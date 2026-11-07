package mcpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/store"
)

const maxAuditResponse = 32 << 20

func decodeAuditJSON(data []byte, value *any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("MCP audit payload has trailing JSON data")
	}
	return nil
}

type inboundAuditKey struct{}

type inboundAudit struct {
	op      store.MCPOperation
	managed atomic.Bool
	secret  string
}

func redactAuditKey(value []byte, secret string) []byte {
	return bytes.ReplaceAll(value, []byte(secret), []byte(locales.MessageAccessKeyHidden()))
}

type auditResponseWriter struct {
	http.ResponseWriter
	body        bytes.Buffer
	status      int
	bytes       int64
	truncated   bool
	writeFailed bool
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *auditResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.bytes += int64(len(data))
	remaining := maxAuditResponse - w.body.Len()
	if len(data) > remaining {
		w.truncated = true
	}
	w.body.Write(data[:min(len(data), remaining)])
	n, err := w.ResponseWriter.Write(data)
	w.writeFailed = w.writeFailed || err != nil
	return n, err
}

func (w *auditResponseWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
		w.writeFailed = true
	}
}

// auditValue never receives HTTP headers or SDK request extras. File bodies are
// omitted, while commands and their bounded stdout/stderr remain inspectable.
func auditValue(value any, tool string, request bool) any {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			sensitive := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			credential := sensitive == "authorization" || sensitive == "secretkey" || sensitive == "password" ||
				sensitive == "passphrase" || sensitive == "privatekey" || sensitive == "token" || sensitive == "apikey" || sensitive == "secret"
			if credential {
				v[key] = locales.MessageAuthenticationInformationHidden()
				continue
			}
			fileBody := key == "text" && (request && tool == "files_write" || !request && tool == "files_read")
			if fileBody {
				if text, ok := child.(string); ok {
					v["omitted_text_bytes"] = len(text)
				}
				delete(v, key)
				v["file_body_omitted"] = true
				continue
			}
			// StructuredContent contains the same tool result as the encoded text
			// content. Keep one copy, including the file-body omission markers.
			if key == "content" && v["structuredContent"] != nil {
				delete(v, key)
				v["duplicate_content_omitted"] = true
				continue
			}
			v[key] = auditValue(child, tool, request)
		}
	case []any:
		for i, child := range v {
			v[i] = auditValue(child, tool, request)
		}
	}
	return value
}

func auditRequestValue(value any) any {
	if batch, ok := value.([]any); ok {
		for i, message := range batch {
			batch[i] = auditRequestValue(message)
		}
		return batch
	}
	tool := ""
	if message, ok := value.(map[string]any); ok && message["method"] == "tools/call" {
		if params, ok := message["params"].(map[string]any); ok {
			tool, _ = params["name"].(string)
		}
	}
	// Only an RPC envelope determines the tool. Nested argument objects cannot
	// redefine the scope used to omit file text.
	return auditValue(value, tool, true)
}

func (s *Service) auditHTTP(w http.ResponseWriter, r *http.Request, key store.MCPKey, next http.Handler) {
	raw, readErr := io.ReadAll(r.Body)
	readErr = errors.Join(readErr, r.Body.Close())
	var request any
	valid := readErr == nil && decodeAuditJSON(raw, &request) == nil
	method, tool := "", ""
	if object, ok := request.(map[string]any); ok {
		method, _ = object["method"].(string)
		if params, ok := object["params"].(map[string]any); ok && method == "tools/call" {
			tool, _ = params["name"].(string)
		}
	}
	action := "mcp." + method
	if method == "" || len(method) > 128 {
		action = "mcp.invalid_request"
	}
	if method == "tools/call" && tool != "" && len(tool) <= 128 {
		action = tool
	}
	if !valid {
		request = map[string]any{"http_method": r.Method, "body_bytes": len(raw), "body_omitted": locales.MessageTheRequestBodyIsNotValidJsonOrExceedsTheSizeLimit()}
	}
	requestJSON, err := json.Marshal(auditRequestValue(request))
	if err != nil {
		http.Error(w, "MCP audit encoding failed", http.StatusInternalServerError)
		return
	}
	secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	requestJSON = redactAuditKey(requestJSON, secret)
	op := store.MCPOperation{ID: rand.Text(), KeyID: key.ID, KeyName: key.Name, Action: action,
		Status: "running", StartedAt: time.Now().UTC()}
	op.Timeline = []store.MCPEvent{{Stage: "accepted", At: op.StartedAt}}
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	err = s.Store.MCPBeginWithPayload(auditCtx, op, store.MCPAuditPayload{Request: requestJSON})
	auditCancel()
	if err != nil {
		s.storageFault(err)
		if method == "tools/call" && strings.HasPrefix(tool, "tasks_") {
			r.Body = io.NopCloser(bytes.NewReader(raw))
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "MCP audit storage unavailable", http.StatusServiceUnavailable)
		return
	}
	entry := &inboundAudit{op: op, secret: secret}
	capture := &auditResponseWriter{ResponseWriter: w}
	if readErr != nil {
		http.Error(capture, "MCP request body exceeds limit or could not be read", http.StatusRequestEntityTooLarge)
	} else {
		r.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(capture, r.WithContext(context.WithValue(r.Context(), inboundAuditKey{}, entry)))
	}
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	var body any
	if !capture.truncated && decodeAuditJSON(capture.body.Bytes(), &body) == nil {
		body = auditValue(body, tool, false)
	} else if capture.truncated {
		body = map[string]any{"body_omitted": locales.MessageTheResponseExceedsTheAuditSizeLimitItsIncompleteBodyWasNotSaved()}
	} else if strings.HasPrefix(capture.Header().Get("Content-Type"), "text/event-stream") {
		events := []any{}
		for _, line := range bytes.Split(capture.body.Bytes(), []byte("\n")) {
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			var event any
			if decodeAuditJSON(bytes.TrimSpace(line[5:]), &event) == nil {
				events = append(events, auditValue(event, tool, false))
			}
		}
		body = map[string]any{"sse_events": events}
	} else if capture.body.Len() > 0 {
		body = map[string]any{"text": capture.body.String()}
	}
	responseJSON, encodeErr := json.Marshal(map[string]any{
		"http_status": capture.status, "data": body, "response_bytes": capture.bytes,
		"capture_truncated": capture.truncated,
		"write_failed":      capture.writeFailed,
	})
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer finishCancel()
	if encodeErr != nil {
		s.storageFault(encodeErr)
		return
	}
	responseJSON = redactAuditKey(responseJSON, secret)
	if !entry.managed.Load() {
		op.FinishedAt, op.Status = time.Now().UTC(), "completed"
		if capture.status >= 400 {
			op.Status, op.ErrorCode, op.Error = "failed", "protocol_error", http.StatusText(capture.status)
		}
		if object, ok := body.(map[string]any); ok && object["error"] != nil {
			op.Status, op.ErrorCode = "failed", "protocol_error"
			op.Error = locales.MessageTheMcpProtocolRequestFailedSeeResponseDetails()
		}
		if object, ok := body.(map[string]any); ok {
			if result, ok := object["result"].(map[string]any); ok && result["isError"] == true {
				op.Status, op.ErrorCode, op.Error = "failed", "tool_error", locales.MessageTheToolRequestFailedSeeResponseDetails()
			}
		}
		op.Timeline = append(op.Timeline, store.MCPEvent{Stage: op.Status, At: op.FinishedAt, Detail: op.Error})
		if err := s.Store.MCPFinish(finishCtx, op); err != nil {
			s.storageFault(err)
			return
		}
	}
	if err := s.Store.MCPUpdatePayload(finishCtx, op.ID, store.MCPAuditPayload{Response: responseJSON}); err != nil {
		s.storageFault(err)
		return
	}
	s.Changed()
}
