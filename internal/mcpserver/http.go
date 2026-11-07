package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sshdesk/internal/locales"
	"sshdesk/internal/store"
)

func (s *Service) Configure(ctx context.Context, cfg Config) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.RetentionDays < 1 || cfg.RetentionDays > 3650 {
		return fail("invalid_argument", locales.MessageInvalidMcpListenerOrRetentionConfiguration())
	}
	previous, running, _, _ := s.Status()
	restart := running != cfg.Enabled || previous.Host != cfg.Host || previous.Port != cfg.Port || s.fault.Load()
	if restart {
		if err := s.stop(ctx); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.config, s.lastErr = cfg, nil
	s.mu.Unlock()
	if cfg.Enabled && restart {
		if err := s.start(ctx); err != nil {
			return err
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.Store.MCPConfigure(ctx, data); err != nil {
		return s.storageFault(err)
	}
	s.Changed()
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	return s.start(ctx)
}

func (s *Service) start(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.mu.Lock()
	if s.server != nil {
		s.mu.Unlock()
		return nil
	}
	cfg := s.config
	s.mu.Unlock()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", strings.Trim(cfg.Host, "[]"))
	if err == nil && len(ips) == 0 {
		err = errors.New("MCP host has no address")
	}
	for _, ip := range ips {
		if !ip.Unmap().IsLoopback() {
			err = locales.NewError(locales.MessageMcpCurrentlyOnlySupportsListeningOnALocalLoopbackAddress)
			break
		}
	}
	var listener net.Listener
	if err == nil {
		listener, err = (&net.ListenConfig{}).Listen(s.ctx, "tcp", net.JoinHostPort(ips[0].String(), strconv.Itoa(cfg.Port)))
	}
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		s.Changed()
		return err
	}
	srv := &http.Server{Handler: s.Handler(cfg), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 130 * time.Second, WriteTimeout: 135 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024,
		BaseContext: func(net.Listener) context.Context { return s.ctx }}
	s.mu.Lock()
	s.server, s.started, s.lastErr = srv, time.Now(), nil
	s.fault.Store(false)
	s.accepting.Store(true)
	s.mu.Unlock()
	s.serveWG.Go(func() {
		err := srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.lastErr = err
			s.accepting.Store(false)
			if s.server == srv {
				s.server = nil
			}
			s.mu.Unlock()
			s.cancelKey("")
			s.Changed()
		}
	})
	s.Changed()
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	return s.stop(ctx)
}

// Close is the final application shutdown path; it does not dispatch work to
// a window whose event loop may already have stopped.
func (s *Service) Close() error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	s.accepting.Store(false)
	s.mu.Lock()
	s.closing = true
	srv := s.server
	s.server = nil
	s.mu.Unlock()
	s.cancelKey("")
	var err error
	if srv != nil {
		err = srv.Close()
	}
	s.serveWG.Wait()
	s.requests.Wait()
	s.operations.Wait()
	return err
}

func (s *Service) stop(ctx context.Context) error {
	s.accepting.Store(false)
	s.mu.Lock()
	srv := s.server
	s.server = nil
	s.mu.Unlock()
	s.cancelKey("")
	if srv == nil {
		return nil
	}
	err := s.Backend.Quiesce(ctx, "", true)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		err = errors.Join(err, shutdownErr, srv.Close())
	}
	s.serveWG.Wait()
	s.Changed()
	return err
}

// Handler authenticates every HTTP request. Sessions and client-supplied names
// are never credentials; each request gets a server with its current tool view.
func (s *Service) Handler(cfg Config) http.Handler {
	type principal struct{}
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		key := r.Context().Value(principal{}).(store.MCPKey)
		return s.protocolServer(key, r.Context())
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, DisableLocalhostProtection: true,
		// Synchronous work follows its HTTP request. Accepted transfers are
		// detached by the desktop queue before returning their task identity.
		PropagateRequestCancellation: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			http.Error(w, "MCP stopped", http.StatusServiceUnavailable)
			return
		}
		s.requests.Add(1)
		s.mu.Unlock()
		defer s.requests.Done()
		if err := s.Backend.Available(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || port != strconv.Itoa(cfg.Port) || !slices.Contains([]string{cfg.Host, "127.0.0.1", "::1", "localhost"}, host) || r.URL.RawQuery != "" {
			http.Error(w, "Invalid MCP request target", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "http" || u.Host != r.Host || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
				http.Error(w, "Invalid MCP origin", http.StatusForbidden)
				return
			}
		}
		token := r.Header.Get("Authorization")
		var key store.MCPKey
		if strings.HasPrefix(token, "Bearer ") && len(r.Header.Values("Authorization")) == 1 {
			key, err = s.Authenticate(r.Context(), strings.TrimPrefix(token, "Bearer "))
		} else {
			err = fail("authentication_failed", locales.MessageMissingAccessKey())
		}
		if err != nil {
			op := store.MCPOperation{ID: rand.Text(), Action: "authentication", Status: "authentication_failed",
				KeyID: key.ID, KeyName: key.Name, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), ErrorCode: "authentication_failed"}
			if op.KeyName == "" {
				op.KeyName = locales.MessageUnknownClient()
			}
			auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer auditCancel()
			payload := store.MCPAuditPayload{
				Request:  json.RawMessage(`{"body_omitted":"Authentication failed; authentication request body omitted"}`),
				Response: json.RawMessage(`{"http_status":401,"data":{"error":{"code":"authentication_failed","message":"Authentication failed"}}}`),
			}
			if auditErr := s.Store.MCPBeginWithPayload(auditCtx, op, payload); auditErr != nil {
				s.storageFault(auditErr)
				http.Error(w, "MCP storage unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="Burrow MCP"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			s.Changed()
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2*MaxText)
		s.auditHTTP(w, r.WithContext(context.WithValue(r.Context(), principal{}, key)), key, transport)
	})
}

func (s *Service) protocolServer(key store.MCPKey, requestContext context.Context) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "Burrow", Version: s.version}, nil)
	read, write, exec := false, false, false
	for _, g := range key.Grants {
		read, write, exec = read || g.Read, write || g.Write, exec || g.Exec
	}
	allowed := func(name string) bool {
		return !(name == "ssh_exec" && !exec || slices.Contains([]string{"files_list", "files_stat", "files_read", "files_download"}, name) && !read || slices.Contains([]string{"files_write", "files_upload"}, name) && !write)
	}
	handle := func(ctx context.Context, name string, arguments json.RawMessage) (*mcp.CallToolResult, error) {
		// Legacy stateless MCP sessions do not inherit a dropped POST's
		// cancellation. A synchronous Burrow operation still belongs to that
		// HTTP call; Enqueue explicitly detaches an accepted background task.
		ctx, cancel := context.WithCancel(ctx)
		stopHTTP := context.AfterFunc(requestContext, cancel)
		defer stopHTTP()
		defer cancel()
		if requestContext.Err() != nil {
			cancel()
		}
		result, err := s.Call(ctx, key.ID, name, arguments)
		if err != nil {
			detail := map[string]any{"code": errorCode(err), "message": err.Error()}
			var failure *Failure
			if errors.As(err, &failure) && len(failure.Hits) > 0 {
				detail["hits"], detail["hint"] = agentHits(failure), locales.McpInterceptionHint()
			}
			result["error"] = detail
		}
		data, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode MCP result: %w", marshalErr)
		}
		return &mcp.CallToolResult{IsError: err != nil, Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}, StructuredContent: result}, nil
	}
	for _, name := range []string{"connections_list", "ssh_exec", "files_list", "files_stat", "files_read", "files_write", "files_upload", "files_download", "tasks_list", "tasks_get", "tasks_cancel"} {
		if !allowed(name) {
			continue
		}
		toolName := name
		server.AddTool(&mcp.Tool{Name: name, Description: toolDescription(name), InputSchema: toolSchema(name)}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handle(ctx, toolName, req.Params.Arguments)
		})
	}
	// Keep all SDK discovery variants consistent by registering only visible
	// tools, while routing direct calls to hidden/unknown tools through audit.
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				params := req.(*mcp.CallToolRequest).Params
				if toolDescription(params.Name) == "" || !allowed(params.Name) {
					return handle(ctx, params.Name, params.Arguments)
				}
			}
			return next(ctx, method, req)
		}
	})
	return server
}

// Policies guard against mistakes rather than isolate the agent, so the hit
// explains what to change; the hint keeps that from reading as a puzzle to
// rewrite around.

func agentHits(failure *Failure) []map[string]any {
	hits := make([]map[string]any, 0, len(failure.Hits))
	for i, h := range failure.Hits {
		hit := map[string]any{"policy_id": h.PolicyID, "policy_name": h.Name, "policy_version": h.Version,
			"category": h.Category, "rule": h.Rule, "action": h.Action, "matched": failure.Matches[i].Text}
		m := failure.Matches[i]
		if m.Context != "" && m.Context != m.Text {
			hit["context"] = m.Context
		}
		if m.Via != "" {
			hit["via"] = m.Via
		}
		if m.Fallback != "" {
			hit["fallback"] = m.Fallback
		}
		hits = append(hits, hit)
	}
	return hits
}

func toolDescription(name string) string {
	return map[string]string{
		"connections_list": "List connections authorized to this key, with stable IDs; never returns credentials. Equivalent SSH/SFTP profiles are shown once. Use files_read, files_write and ssh_exec flags to choose operations: SSH connections can provide SFTP file tools. Explicit protocol filters retain original profile IDs. SSH connections include os (linux, darwin, windows or other), os_name and arch once known, and shell for Windows.",
		"ssh_exec":         "Run one non-PTY command in the remote user's default shell (check connections_list os and, on Windows, shell). stdout, stderr and exit_code are separate; cancellation never reruns the command.",
		"files_list":       "List one remote directory with bounded pagination; symlinks are not traversed.",
		"files_stat":       "Read remote entry metadata without following symlinks.",
		"files_read":       "Read a bounded UTF-8 text range from a remote regular file.",
		"files_write":      "Atomically create or replace one UTF-8 text file. mode must be create or replace.",
		"files_upload":     "Queue an upload from an authorized absolute local path; returns a recoverable task ID.",
		"files_download":   "Queue a download into an authorized absolute local directory; returns a recoverable task ID.",
		"tasks_list":       "List only this key's persistent background tasks.",
		"tasks_get":        "Read one task owned by this key.",
		"tasks_cancel":     "Request cancellation of one owned task; remote effects already committed are preserved.",
	}[name]
}

func toolSchema(name string) map[string]any {
	props := map[string]any{}
	required := []string{}
	add := func(key, kind string, mandatory bool) {
		props[key] = map[string]any{"type": kind}
		if mandatory {
			required = append(required, key)
		}
	}
	if name == "connections_list" || name == "files_list" || name == "tasks_list" {
		add("offset", "integer", false)
		add("limit", "integer", false)
	}
	if name == "connections_list" {
		add("protocol", "string", false)
	} else if name == "tasks_list" {
		add("state", "string", false)
	} else if strings.HasPrefix(name, "tasks_") {
		add("task_id", "integer", true)
	} else {
		add("connection_id", "string", true)
		if name == "ssh_exec" {
			add("command", "string", true)
			add("timeout_seconds", "integer", false)
		} else {
			add("path", "string", true)
		}
		if name == "files_read" {
			add("offset", "integer", false)
			add("length", "integer", false)
		}
		if name == "files_write" {
			add("text", "string", true)
			add("mode", "string", true)
		}
		if name == "files_upload" || name == "files_download" {
			add("local_path", "string", true)
			add("conflict", "string", true)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
