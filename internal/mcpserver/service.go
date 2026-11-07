// Package mcpserver owns the authenticated local MCP service. Desktop adapters
// supply immutable connection snapshots and the application's existing queue.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/store"
)

const MaxText = 1024 * 1024

type Connection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	// OS is the SSH host's pinned or last detected platform (linux, darwin,
	// windows or other); empty until detected.
	OS string `json:"os,omitempty"`
	// OSName, Arch and Shell come from the last facts collected for that
	// platform; Shell is set for Windows hosts.
	OSName   string `json:"os_name,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Shell    string `json:"shell,omitempty"`
	Identity string `json:"-"`
	Runtime  any    `json:"-"`
	// CanonicalID groups equivalent SSH/SFTP profiles for display. Grants,
	// policies and accepted work retain the actual source ID and identity.
	CanonicalID string `json:"-"`
}

type TransferRequest struct {
	Operation  store.MCPOperation
	Connection Connection
	Key        store.MCPKey
	RemotePath string
	LocalPath  string
	Download   bool
	Conflict   string
}

type operationContextKey struct{}
type operationContext struct {
	ID     string
	TaskID int
}

func WithOperation(ctx context.Context, id string, taskID int) context.Context {
	return context.WithValue(ctx, operationContextKey{}, operationContext{id, taskID})
}

func OperationID(ctx context.Context) (string, int) {
	value, _ := ctx.Value(operationContextKey{}).(operationContext)
	return value.ID, value.TaskID
}

// All methods may be called from HTTP workers. The adapter is responsible for
// dispatching access to desktop-owned state onto the window thread.
type Backend interface {
	Available(context.Context) error
	Connections(context.Context) ([]Connection, error)
	OpenFiles(context.Context, Connection) (fileclient.Client, error)
	Exec(context.Context, Connection, string) (sshclient.ExecResult, error)
	WriteText(context.Context, Connection, string, string, string, func(context.Context, fileclient.Client, string, bool) error) error
	Enqueue(context.Context, TransferRequest) (store.MCPTask, error)
	ControlTask(context.Context, int, string) error
	Quiesce(context.Context, string, bool) error
}

type Config struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	RetentionDays int    `json:"retention_days"`
}

type activeOperation struct {
	keyID                                       string
	connectionID, capability, identity, command string
	cancel                                      context.CancelFunc
}

type Service struct {
	Store      *store.Store
	Backend    Backend
	ctx        context.Context
	version    string
	notify     func()
	mu         sync.Mutex
	lifecycle  sync.Mutex
	config     Config
	server     *http.Server
	started    time.Time
	lastErr    error
	active     map[string]activeOperation
	fault      atomic.Bool
	accepting  atomic.Bool
	slots      chan struct{}
	serveWG    sync.WaitGroup
	requests   sync.WaitGroup
	operations sync.WaitGroup
	closing    bool
}

type Failure struct {
	Code    string
	Message string
	Hits    []store.MCPHit
	// Matches[i] is the part of the request that triggered Hits[i]. It goes
	// back to the calling agent and is retained in the captured response;
	// audit hits stay deduplicated per rule.
	Matches []Match
}

// Match is the matched command text, or the canonical path for a file rule.
// Context is the whitespace-delimited command word containing Text for text
// rules. A command rule reports the launchers it ran through (Via), or why it
// fell back to keyword matching.
type Match struct {
	Text, Context, Via, Fallback string
}

func (e *Failure) Error() string { return e.Message }

func fail(code, message string) error { return &Failure{Code: code, Message: message} }

func errorCode(err error) string {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Code
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(err, fs.ErrExist):
		return "conflict"
	case errors.Is(err, fs.ErrNotExist):
		return "not_found"
	default:
		return "operation_failed"
	}
}

func New(
	ctx context.Context,
	db *store.Store,
	backend Backend,
	version string,
	changed func(),
) (*Service, error) {
	s := &Service{Store: db, Backend: backend, ctx: ctx, version: version, notify: changed,
		config: Config{Host: "127.0.0.1", Port: 7801, RetentionDays: 30},
		active: make(map[string]activeOperation), slots: make(chan struct{}, 8)}
	var data []byte
	found, err := db.ReadSetting(ctx, "mcp", &data)
	if err != nil {
		return nil, err
	}
	if found {
		if err := json.Unmarshal(data, &s.config); err != nil {
			return nil, fmt.Errorf("read MCP settings: %w", err)
		}
	}
	if s.config.Host == "" || s.config.Port < 1 || s.config.Port > 65535 || s.config.RetentionDays < 1 {
		return nil, errors.New("invalid saved MCP settings")
	}
	if err := db.MCPInterrupt(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Changed() {
	if s.notify != nil {
		s.notify()
	}
}

func (s *Service) Status() (Config, bool, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config, s.server != nil && !s.fault.Load(), s.started, s.lastErr
}

func (s *Service) storageFault(err error) error {
	s.fault.Store(true)
	s.accepting.Store(false)
	s.mu.Lock()
	s.lastErr = fmt.Errorf("MCP storage: %w", err)
	for _, op := range s.active {
		op.cancel()
	}
	s.mu.Unlock()
	s.Changed()
	return fail("audit_unavailable", locales.MessageMcpStorageIsUnavailableNewOperationsAreBlocked())
}

func (s *Service) FailStorage(err error) error { return s.storageFault(err) }

func (s *Service) Accepting() bool { return s.accepting.Load() && !s.fault.Load() }

func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	if err := s.Store.MCPDeletePolicy(ctx, id); err != nil {
		return s.storageFault(err)
	}
	s.Changed()
	return nil
}

func (s *Service) Authenticate(ctx context.Context, secret string) (store.MCPKey, error) {
	key, err := s.verifySecret(ctx, secret)
	if err != nil {
		return key, err
	}
	if !s.fault.Load() {
		if err := s.Store.MCPTouchKey(ctx, key.ID); err != nil {
			if ctx.Err() != nil {
				return store.MCPKey{}, ctx.Err()
			}
			if errors.Is(err, sql.ErrNoRows) {
				return store.MCPKey{}, fail("authentication_failed", locales.MessageInvalidAccessKey())
			}
			// A readable key still authenticates recovery controls. Call rejects
			// new business operations after this write fault.
			s.storageFault(err)
		}
	}
	return key, nil
}

func (s *Service) verifySecret(ctx context.Context, secret string) (store.MCPKey, error) {
	var key store.MCPKey
	id, _, ok := strings.Cut(strings.TrimPrefix(secret, "brw_mcp_"), ".")
	if !ok || len(id) != 26 || len(secret) > 128 || !strings.HasPrefix(secret, "brw_mcp_") {
		return key, fail("authentication_failed", locales.MessageInvalidAccessKey())
	}
	key, err := s.Store.MCPKey(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return key, fail("authentication_failed", locales.MessageInvalidAccessKey())
	}
	if err != nil {
		if ctx.Err() != nil {
			return key, ctx.Err()
		}
		return key, s.storageFault(err)
	}
	digest := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(key.Digest, digest[:]) != 1 {
		return store.MCPKey{}, fail("authentication_failed", locales.MessageInvalidAccessKey())
	}
	if key.State != "active" {
		return key, fail("authentication_failed", locales.MessageInvalidAccessKey())
	}
	return key, nil
}

func (s *Service) CreateKey(ctx context.Context, key store.MCPKey) (store.MCPKey, string, error) {
	key.ID, key.State, key.CreatedAt = rand.Text(), "active", time.Now().UTC()
	secret := "brw_mcp_" + key.ID + "." + base64.RawURLEncoding.EncodeToString(randBytes())
	digest := sha256.Sum256([]byte(secret))
	key.Digest, key.Prefix = digest[:], "brw_mcp_"+key.ID[:8]+"…"
	if err := s.validateKey(ctx, &key); err != nil {
		return store.MCPKey{}, "", err
	}
	if err := s.Store.MCPCreateKey(ctx, key); err != nil {
		return store.MCPKey{}, "", s.storageFault(err)
	}
	s.Changed()
	return key, secret, nil
}

func randBytes() []byte {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return data
}

func (s *Service) validateKey(ctx context.Context, key *store.MCPKey) error {
	key.Grants = slices.Clone(key.Grants)
	key.UploadDirs, key.DownloadDirs = slices.Clone(key.UploadDirs), slices.Clone(key.DownloadDirs)
	key.Name = strings.TrimSpace(key.Name)
	if key.Name == "" || len(key.Name) > 200 {
		return fail("invalid_argument", locales.MessageEnterAValidKeyNameUpToBytes())
	}
	connections, err := s.Backend.Connections(ctx)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for i := range key.Grants {
		g := &key.Grants[i]
		index := slices.IndexFunc(connections, func(c Connection) bool { return c.ID == g.ConnectionID })
		if index < 0 || seen[g.ConnectionID] {
			return fail("invalid_argument", locales.MessageAnAuthorizedConnectionIsMissingOrDuplicated())
		}
		seen[g.ConnectionID] = true
		c := connections[index]
		if g.Read && !c.Supports("read") || g.Write && !c.Supports("write") || g.Exec && !c.Supports("exec") {
			return fail("invalid_argument", locales.MessageTheConnectionProtocolDoesNotMatchTheGrantedCapabilities())
		}
		if g.EndpointDigest == "" {
			g.EndpointDigest = c.Identity
		} else if g.EndpointDigest != c.Identity {
			return fail("invalid_argument", locales.MessageConnectionDetailsChangedConfirmAuthorizationAgain())
		}
	}
	for _, dirs := range [][]string{key.UploadDirs, key.DownloadDirs} {
		for i, dir := range dirs {
			absolute, err := LocalPath(dir)
			if err != nil {
				return err
			}
			dirs[i] = absolute
		}
	}
	return nil
}

func (s *Service) UpdateKey(ctx context.Context, key store.MCPKey) error {
	old, err := s.Store.MCPKey(ctx, key.ID)
	if err != nil {
		return err
	}
	if old.State == "revoked" {
		return fail("invalid_argument", locales.MessageARevokedKeyCannotBeRestored())
	}
	if key.State != "active" && key.State != "disabled" && key.State != "revoked" {
		return fail("invalid_argument", locales.MessageInvalidKeyState())
	}
	key.Digest, key.CreatedAt, key.Prefix, key.LastUsedAt = old.Digest, old.CreatedAt, old.Prefix, old.LastUsedAt
	if key.State == "revoked" {
		key.Grants, key.UploadDirs, key.DownloadDirs = nil, nil, nil
	} else if err := s.validateKey(ctx, &key); err != nil {
		return err
	}
	if err := s.Store.MCPUpdateKey(ctx, key); err != nil {
		if errors.Is(err, store.ErrMCPKeyRevoked) {
			return fail("invalid_argument", locales.MessageARevokedKeyCannotBeRestored())
		}
		return s.storageFault(err)
	}
	return s.keyChanged(ctx, key)
}

func (s *Service) SetKeyState(ctx context.Context, id, state string) error {
	if state != "active" && state != "disabled" && state != "revoked" {
		return fail("invalid_argument", locales.MessageInvalidKeyState())
	}
	key, err := s.Store.MCPSetKeyState(ctx, id, state)
	if errors.Is(err, sql.ErrNoRows) {
		return fail("not_found", locales.MessageTheKeyNoLongerExistsRefreshAndTryAgain())
	}
	if errors.Is(err, store.ErrMCPKeyRevoked) {
		return fail("invalid_argument", locales.MessageARevokedKeyCannotBeRestored())
	}
	if err != nil {
		return s.storageFault(err)
	}
	return s.keyChanged(ctx, key)
}

func (s *Service) keyChanged(ctx context.Context, key store.MCPKey) error {
	// Registered operations are cancelled after the durable change. A request
	// registers before re-reading its current key, closing the admission race.
	s.mu.Lock()
	for _, op := range s.active {
		if op.keyID != key.ID {
			continue
		}
		allowed := key.State == "active" && op.connectionID == ""
		for _, g := range key.Grants {
			if key.State == "active" && g.ConnectionID == op.connectionID && (op.identity == "" || g.EndpointDigest == op.identity) &&
				(op.capability == "read" && g.Read || op.capability == "write" && g.Write || op.capability == "exec" && g.Exec) {
				allowed = true
			}
		}
		if !allowed {
			op.cancel()
		}
	}
	s.mu.Unlock()
	if err := s.Backend.Quiesce(ctx, key.ID, false); err != nil {
		return err
	}
	s.Changed()
	return nil
}

func (s *Service) cancelKey(keyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, op := range s.active {
		if keyID == "" || op.keyID == keyID {
			op.cancel()
		}
	}
}

func (s *Service) SavePolicy(ctx context.Context, p store.MCPPolicy) error {
	if err := ValidatePolicy(p); err != nil {
		return err
	}
	// Legacy policies may contain inactive rules from the other editor.
	if p.Category == "ssh" {
		p.FileRules = nil
	} else {
		p.CommandRules = nil
	}
	connections, err := s.Backend.Connections(ctx)
	if err != nil {
		return err
	}
	var scoped []string
	for _, id := range p.ConnectionIDs {
		i := slices.IndexFunc(connections, func(c Connection) bool { return c.ID == id })
		if i < 0 || p.Category == "ssh" && !connections[i].Supports("exec") || p.Category == "files" && !connections[i].Supports("read") {
			return fail("invalid_argument", locales.MessageAPolicyConnectionDoesNotExistOrHasAnIncompatibleProtocol())
		}
		if !slices.Contains(scoped, id) {
			scoped = append(scoped, id)
		}
		// A newly selected merged file connection protects its members too.
		// Persist those IDs so later regrouping cannot drop the restriction.
		// Explicit legacy member scopes remain unchanged.
		if p.Category == "files" && connections[i].DisplayID() == id {
			for _, c := range connections {
				if c.DisplayID() == id && !slices.Contains(scoped, c.ID) {
					scoped = append(scoped, c.ID)
				}
			}
		}
	}
	p.ConnectionIDs = scoped
	if len(p.ConnectionIDs) > 1000 {
		return fail("invalid_argument", locales.MessageTheNumberOfPolicyConnectionsExceedsTheLimit())
	}
	if p.ID == "" {
		p.ID, p.Version = rand.Text(), 1
	}
	if err := s.Store.MCPSavePolicy(ctx, p); err != nil {
		return s.storageFault(err)
	}
	s.recheckActiveCommands(ctx)
	s.Changed()
	return nil
}

func (s *Service) SetPolicyEnabled(ctx context.Context, id string, enabled bool) error {
	if err := s.Store.MCPSetPolicyEnabled(ctx, id, enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail("not_found", locales.MessageThePolicyNoLongerExistsRefreshAndTryAgain())
		}
		return s.storageFault(err)
	}
	s.recheckActiveCommands(ctx)
	s.Changed()
	return nil
}

func (s *Service) recheckActiveCommands(ctx context.Context) {
	s.mu.Lock()
	active := make([]activeOperation, 0, len(s.active))
	for _, op := range s.active {
		active = append(active, op)
	}
	s.mu.Unlock()
	for _, op := range active {
		if op.capability == "exec" && op.command != "" {
			if err := s.CheckCommand(ctx, op.keyID, op.connectionID, op.command); err != nil {
				op.cancel()
			}
		}
	}
}

func (s *Service) Authorize(ctx context.Context, keyID, connectionID, capability string) (store.MCPKey, Connection, error) {
	key, err := s.Store.MCPKey(ctx, keyID)
	if err != nil {
		return key, Connection{}, err
	}
	if key.State != "active" || !s.Accepting() {
		return key, Connection{}, fail("permission_denied", locales.MessageTheAccessKeyIsNoLongerValidOrTheServiceHasStoppedAccepting())
	}
	connections, err := s.Backend.Connections(ctx)
	if err != nil {
		return key, Connection{}, err
	}
	index := slices.IndexFunc(connections, func(c Connection) bool { return c.ID == connectionID })
	if index >= 0 {
		requested := connections[index]
		if grantedConnection(key, requested, capability) {
			return key, requested, nil
		}
		// The merged entry can use an existing member grant, but the returned
		// source keeps that grant's credentials, policies and task identity.
		// A legacy member ID never inherits permissions from another member.
		if requested.ID == requested.DisplayID() {
			for _, c := range connections {
				if c.DisplayID() == requested.ID && grantedConnection(key, c, capability) {
					return key, c, nil
				}
			}
		}
	}
	return key, Connection{}, fail("permission_denied", locales.MessageTheConnectionIsUnauthorizedPermissionsAreInsufficientOrTheEndpointIdentityHasChanged())
}

func LocalPath(name string) (string, error) {
	if strings.ContainsRune(name, 0) || name == "" {
		return "", fail("invalid_argument", locales.MessageLocalPathCannotBeEmpty())
	}
	if name == "~" || strings.HasPrefix(name, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if name == "~" {
			name = home
		} else {
			name = filepath.Join(home, strings.TrimPrefix(name, "~/"))
		}
	}
	if !filepath.IsAbs(name) {
		return "", fail("invalid_argument", locales.MessageLocalPathMustBeAbsolute())
	}
	return filepath.Clean(name), nil
}
