package mcpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"sshdesk/internal/fileclient"
	"sshdesk/internal/store"
)

type Input struct {
	ConnectionID string `json:"connection_id"`
	Path         string `json:"path"`
	LocalPath    string `json:"local_path"`
	Command      string `json:"command"`
	Timeout      int    `json:"timeout_seconds"`
	Text         string `json:"text"`
	Mode         string `json:"mode"`
	Conflict     string `json:"conflict"`
	Protocol     string `json:"protocol"`
	State        string `json:"state"`
	Offset       int    `json:"offset"`
	Limit        int    `json:"limit"`
	Length       int    `json:"length"`
	TaskID       int    `json:"task_id"`
}

func decodeInput(name string, raw json.RawMessage) (Input, error) {
	var in Input
	if len(raw) == 0 && len(toolSchema(name)["required"].([]string)) == 0 {
		raw = json.RawMessage("{}")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return in, fail("invalid_argument", "工具参数必须是 JSON 对象")
	}
	schema := toolSchema(name)
	for key := range object {
		if _, ok := schema["properties"].(map[string]any)[key]; !ok {
			return in, fail("invalid_argument", "包含不支持的工具参数")
		}
	}
	for _, key := range schema["required"].([]string) {
		if value, ok := object[key]; !ok || string(value) == "null" {
			return in, fail("invalid_argument", "缺少必需工具参数")
		}
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, fail("invalid_argument", "工具参数类型不正确")
	}
	if in.Timeout < 0 || in.Timeout > 120 || in.Offset < 0 || in.Offset > 1000000 || in.Limit < 0 || in.Limit > 1000 || in.Length < 0 || in.Length > MaxText {
		return in, fail("invalid_argument", "参数超过服务端限制")
	}
	if in.Limit == 0 {
		in.Limit = 100
	}
	if in.Timeout == 0 {
		in.Timeout = 60
	}
	if in.Length == 0 {
		in.Length = 65536
	}
	if len(in.Path) > 4096 || len(in.LocalPath) > 4096 || len(in.Command) > 65536 || len(in.Text) > MaxText || strings.ContainsRune(in.Command, 0) || strings.ContainsRune(in.Path, 0) {
		return in, fail("invalid_argument", "命令、路径或文本超过限制或含无效字符")
	}
	if name == "ssh_exec" && strings.TrimSpace(in.Command) == "" {
		return in, fail("invalid_argument", "命令不能为空")
	}
	if strings.HasPrefix(name, "files_") && (!path.IsAbs(in.Path) || in.Path == "") {
		return in, fail("invalid_argument", "远端路径必须是绝对路径")
	}
	if name == "files_write" && in.Mode != "create" && in.Mode != "replace" {
		return in, fail("invalid_argument", "写入模式必须为 create 或 replace")
	}
	if name == "files_upload" || name == "files_download" {
		if !slices.Contains([]string{"error", "overwrite", "skip", "rename"}, in.Conflict) {
			return in, fail("invalid_argument", "必须指定 error、overwrite、skip 或 rename 冲突策略")
		}
	}
	return in, nil
}

func capability(name string) string {
	if name == "ssh_exec" {
		return "exec"
	}
	if name == "files_write" || name == "files_upload" {
		return "write"
	}
	return "read"
}

func (s *Service) Call(parent context.Context, keyID, name string, raw json.RawMessage) (out map[string]any, callErr error) {
	out = make(map[string]any)
	inbound, _ := parent.Value(inboundAuditKey{}).(*inboundAudit)
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return out, fail("service_stopped", "MCP 服务已停止")
	}
	s.operations.Add(1)
	s.mu.Unlock()
	defer s.operations.Done()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		busy := store.MCPOperation{ID: rand.Text(), KeyID: keyID, Action: name, Status: "failed", ErrorCode: "busy", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
		if inbound != nil {
			inbound.managed.Store(true)
			busy.ID, busy.KeyName = inbound.op.ID, inbound.op.KeyName
		}
		auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer auditCancel()
		var err error
		if inbound != nil {
			err = s.Store.MCPFinish(auditCtx, busy)
		} else {
			err = s.Store.MCPBegin(auditCtx, busy, nil)
		}
		if err != nil {
			return out, s.storageFault(err)
		}
		out["operation_id"] = busy.ID
		return out, fail("busy", "MCP 同时执行的调用已达到上限")
	}
	op := store.MCPOperation{ID: rand.Text(), KeyID: keyID, Action: name, Status: "running", StartedAt: time.Now().UTC(), Timeline: []store.MCPEvent{{Stage: "accepted", At: time.Now().UTC()}}}
	if inbound != nil {
		op = inbound.op
	}
	out["operation_id"] = op.ID
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	ctx = WithOperation(ctx, op.ID, 0)
	defer cancel()
	s.mu.Lock()
	s.active[op.ID] = activeOperation{keyID: keyID, cancel: cancel}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.active, op.ID)
		s.mu.Unlock()
	}()
	key, err := s.Store.MCPKey(ctx, keyID)
	if err != nil {
		return out, err
	}
	op.KeyName = key.Name
	if key.State == "active" && strings.HasPrefix(name, "tasks_") && s.fault.Load() {
		return s.recoveryTool(ctx, key, name, raw, op, out)
	}
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer auditCancel()
	if inbound == nil {
		err = s.Store.MCPBegin(auditCtx, op, nil)
	}
	if err != nil {
		if key.State == "active" && strings.HasPrefix(name, "tasks_") {
			s.storageFault(err)
			return s.recoveryTool(ctx, key, name, raw, op, out)
		}
		return out, s.storageFault(err)
	}
	if inbound != nil {
		inbound.managed.Store(true)
	}
	background := false
	defer func() {
		if background {
			s.Changed()
			return
		}
		op.Status, op.FinishedAt = "completed", time.Now().UTC()
		if callErr != nil {
			op.Status, op.ErrorCode, op.Error = "failed", errorCode(callErr), callErr.Error()
			var failure *Failure
			if errors.As(callErr, &failure) {
				op.ErrorCode, op.Hits = failure.Code, failure.Hits
				if failure.Code == "intercepted" || failure.Code == "permission_denied" {
					op.Status = failure.Code
				}
			}
			if errors.Is(callErr, context.Canceled) {
				op.Status, op.ErrorCode = "cancelled", "cancelled"
			} else if errors.Is(callErr, context.DeadlineExceeded) {
				op.Status, op.ErrorCode = "timed_out", "timed_out"
			}
		}
		out["status"] = op.Status
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer finishCancel()
		saved, readErr := s.Store.MCPOperation(finishCtx, op.ID)
		if readErr != nil {
			callErr = errors.Join(callErr, s.storageFault(readErr))
			return
		}
		op.Timeline = append(saved.Timeline, store.MCPEvent{Stage: op.Status, At: op.FinishedAt, Detail: op.Error})
		if inbound != nil {
			op.Error = string(redactAuditKey([]byte(op.Error), inbound.secret))
			for i := range op.Timeline {
				op.Timeline[i].Detail = string(redactAuditKey([]byte(op.Timeline[i].Detail), inbound.secret))
			}
		}
		if err := s.Store.MCPFinish(finishCtx, op); err != nil {
			callErr = errors.Join(callErr, s.storageFault(err))
		}
		s.Changed()
	}()
	if key.State != "active" || s.fault.Load() {
		return out, fail("permission_denied", "授权密钥无效或 MCP 存储不可用")
	}
	if !s.Accepting() && !strings.HasPrefix(name, "tasks_") {
		return out, fail("service_stopped", "MCP 服务已停止")
	}
	if toolDescription(name) == "" {
		return out, fail("unknown_tool", "不支持的工具")
	}
	in, err := decodeInput(name, raw)
	if err != nil {
		return out, err
	}
	op.ConnectionID = in.ConnectionID
	// Body text and command output never enter the audit database.
	requestSummary := map[string]any{"command": in.Command, "path": in.Path,
		"local_path": in.LocalPath, "mode": in.Mode, "conflict": in.Conflict,
		"offset": in.Offset, "length": in.Length, "text_bytes": len(in.Text), "task_id": in.TaskID, "state": in.State}
	op.Request, err = json.Marshal(requestSummary)
	if err != nil {
		return out, err
	}
	if inbound != nil {
		op.Request = redactAuditKey(op.Request, inbound.secret)
	}
	s.mu.Lock()
	active := s.active[op.ID]
	active.connectionID, active.capability, active.command = in.ConnectionID, capability(name), in.Command
	s.active[op.ID] = active
	s.mu.Unlock()
	if name == "connections_list" {
		connections, err := s.Backend.Connections(ctx)
		if err != nil {
			return out, err
		}
		visible := []map[string]any{}
		for _, c := range connections {
			for _, g := range key.Grants {
				if g.ConnectionID == c.ID && g.EndpointDigest == c.Identity && (g.Read || g.Write || g.Exec) && (in.Protocol == "" || c.Protocol == in.Protocol) {
					visible = append(visible, map[string]any{"connection": c, "files_read": g.Read, "files_write": g.Write, "ssh_exec": g.Exec})
				}
			}
		}
		from := min(in.Offset, len(visible))
		out["connections"], out["total"] = visible[from:min(from+in.Limit, len(visible))], len(visible)
		return out, nil
	}
	if strings.HasPrefix(name, "tasks_") {
		return s.taskTool(ctx, key.ID, name, in, out)
	}
	key, connection, err := s.Authorize(ctx, key.ID, in.ConnectionID, capability(name))
	if err != nil {
		return out, err
	}
	policies, err := s.Store.MCPPolicies(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, s.storageFault(err)
	}
	versions := make(map[string]int)
	category := "files"
	if connection.Protocol == "ssh" {
		category = "ssh"
	}
	for _, p := range policies {
		if applies(p, key.ID, connection.ID, category) {
			versions[p.ID] = p.Version
		}
	}
	requestSummary["accepted_policy_versions"] = versions
	op.Request, err = json.Marshal(requestSummary)
	if err != nil {
		return out, err
	}
	if inbound != nil {
		op.Request = redactAuditKey(op.Request, inbound.secret)
	}
	s.mu.Lock()
	active = s.active[op.ID]
	active.identity = connection.Identity
	s.active[op.ID] = active
	s.mu.Unlock()
	op.Target, err = json.Marshal(connection)
	if err != nil {
		return out, err
	}
	op.Timeline = append(op.Timeline, store.MCPEvent{Stage: "validated", At: time.Now().UTC()})
	// Persist the validated target and request before opening any channel.
	if err := s.Store.MCPFinish(ctx, op); err != nil {
		return out, s.storageFault(err)
	}
	if name == "ssh_exec" {
		if connection.Protocol != "ssh" {
			return out, fail("invalid_argument", "该连接不支持 SSH 命令")
		}
		if err := s.CheckCommand(ctx, key.ID, connection.ID, in.Command); err != nil {
			return out, err
		}
		execCtx, execCancel := context.WithTimeout(ctx, time.Duration(in.Timeout)*time.Second)
		defer execCancel()
		result, err := s.Backend.Exec(execCtx, connection, in.Command)
		out["stdout"], out["stderr"], out["exit_code"], out["truncated"] = string(result.Stdout), string(result.Stderr), result.ExitCode, result.Truncated
		out["stdout_lossy_utf8"], out["stderr_lossy_utf8"] = !utf8.Valid(result.Stdout), !utf8.Valid(result.Stderr)
		out["timed_out"], out["cancelled"] = errors.Is(execCtx.Err(), context.DeadlineExceeded), errors.Is(ctx.Err(), context.Canceled)
		var stdoutBytes, stderrBytes *int
		if result.ExitCode != nil || result.Stdout != nil || result.Stderr != nil {
			stdout, stderr := len(result.Stdout), len(result.Stderr)
			stdoutBytes, stderrBytes = &stdout, &stderr
		}
		out["output_complete"] = result.ExitCode != nil && !result.Truncated && execCtx.Err() == nil
		var encodeErr error
		op.Result, encodeErr = json.Marshal(map[string]any{"stdout_bytes": stdoutBytes, "stderr_bytes": stderrBytes, "exit_code": result.ExitCode, "truncated": result.Truncated, "output_complete": out["output_complete"]})
		if encodeErr != nil {
			return out, errors.Join(err, encodeErr)
		}
		if result.Truncated {
			return out, fail("output_limit", "命令输出超过服务端限制，输出已截断")
		}
		return out, err
	}
	if connection.Protocol == "ssh" {
		return out, fail("invalid_argument", "文件工具仅适用于已保存的文件连接")
	}
	if name == "files_upload" || name == "files_download" {
		download := name == "files_download"
		localPath, err := AuthorizeLocal(key, in.LocalPath, download)
		if err != nil {
			return out, err
		}
		task, err := s.Backend.Enqueue(ctx, TransferRequest{Operation: op, Connection: connection, Key: key,
			RemotePath: path.Clean(in.Path), LocalPath: localPath, Download: download, Conflict: in.Conflict})
		if err != nil {
			return out, err
		}
		out["task_id"], out["status"] = task.ID, task.State
		background = true
		return out, nil
	}
	if name == "files_write" {
		err := s.Backend.WriteText(ctx, connection, path.Clean(in.Path), in.Text, in.Mode,
			func(checkCtx context.Context, client fileclient.Client, target string, exists bool) error {
				if _, _, err := s.Authorize(checkCtx, key.ID, connection.ID, "write"); err != nil {
					return err
				}
				if in.Mode == "create" && exists {
					return fs.ErrExist
				}
				if in.Mode == "replace" && !exists {
					return fs.ErrNotExist
				}
				action := "create"
				if exists {
					action = "write"
				}
				return s.CheckFile(checkCtx, key.ID, connection.ID, "remote", target, action, client)
			})
		out["requested_bytes"] = len(in.Text)
		out["commit_status"] = "unknown"
		committedBytes := int64(0)
		if err == nil {
			out["commit_status"], out["bytes"] = "committed", len(in.Text)
			committedBytes = int64(len(in.Text))
		}
		op.Result, callErr = json.Marshal(map[string]any{"requested_bytes": len(in.Text), "commit_status": out["commit_status"]})
		if callErr != nil {
			return out, errors.Join(err, callErr)
		}
		if fileErr := s.FileEvent(context.WithoutCancel(ctx), op.ID, in.Path, "remote", "write", err, committedBytes); fileErr != nil {
			return out, errors.Join(err, fileErr)
		}
		return out, err
	}
	client, err := s.Backend.OpenFiles(ctx, connection)
	if err != nil {
		return out, err
	}
	defer func() { callErr = errors.Join(callErr, client.Close()) }()
	action := "browse"
	if name == "files_read" {
		action = "read"
	}
	if err := s.CheckFile(ctx, key.ID, connection.ID, "remote", in.Path, action, client); err != nil {
		return out, err
	}
	entry, err := client.Stat(ctx, in.Path)
	if err != nil {
		return out, err
	}
	if entry.Symlink && name != "files_stat" {
		return out, fail("path_unverifiable", "文件工具不跟随符号链接")
	}
	observedBytes := int64(0)
	switch name {
	case "files_stat":
		out["entry"] = entry
	case "files_list":
		entries, err := client.ReadDir(ctx, in.Path)
		if err != nil {
			return out, err
		}
		slices.SortFunc(entries, func(a, b fileclient.Entry) int { return strings.Compare(a.Name, b.Name) })
		from := min(in.Offset, len(entries))
		out["entries"], out["total"], out["has_more"] = entries[from:min(from+in.Limit, len(entries))], len(entries), from+in.Limit < len(entries)
	case "files_read":
		if entry.Directory || entry.ModeKnown && !entry.Mode.IsRegular() {
			return out, fail("invalid_argument", "只能读取普通文本文件")
		}
		reader, err := client.OpenRead(ctx, in.Path, int64(in.Offset))
		if err != nil {
			return out, err
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, int64(in.Length)+1))
		if err := errors.Join(readErr, reader.Close()); err != nil {
			return out, err
		}
		truncated := len(data) > in.Length
		if truncated {
			end := in.Length
			for end > 0 && !utf8.RuneStart(data[end]) {
				end--
			}
			data = data[:end]
		}
		if !utf8.Valid(data) {
			return out, fail("invalid_argument", "读取范围不是有效的 UTF-8 文本")
		}
		// Permissions and rules may have tightened during a slow read. Do not
		// deliver already-buffered content after that access has been removed.
		if _, _, err := s.Authorize(ctx, key.ID, connection.ID, "read"); err != nil {
			return out, err
		}
		if err := s.CheckFile(ctx, key.ID, connection.ID, "remote", in.Path, "read", client); err != nil {
			return out, err
		}
		out["text"], out["offset"], out["bytes"], out["truncated"] = string(data), in.Offset, len(data), truncated || int64(in.Offset+len(data)) < entry.Size
		observedBytes = int64(len(data))
	default:
		return out, fail("unknown_tool", "不支持的工具")
	}
	op.Result, err = json.Marshal(map[string]any{"bytes": observedBytes, "offset": in.Offset, "truncated": out["truncated"], "total": out["total"]})
	if err != nil {
		return out, err
	}
	return out, s.FileEvent(context.WithoutCancel(ctx), op.ID, in.Path, "remote", action, nil, observedBytes)
}

func (s *Service) FileEvent(ctx context.Context, operationID, name, side, action string, result error, size int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	event := store.MCPFileEvent{OperationID: operationID, Path: name, Side: side, Action: action, Bytes: size, At: time.Now().UTC(), Status: "completed"}
	if result != nil {
		event.Status, event.Error = "failed", result.Error()
		var failure *Failure
		if errors.As(result, &failure) && failure.Code == "intercepted" {
			event.Status = "intercepted"
		}
	}
	if err := s.Store.MCPAddFile(ctx, event); err != nil {
		return s.storageFault(err)
	}
	return nil
}

func (s *Service) taskTool(ctx context.Context, keyID, name string, in Input, out map[string]any) (map[string]any, error) {
	if name == "tasks_list" {
		tasks, total, err := s.Store.MCPTasksPage(ctx, keyID, in.State, in.Limit, in.Offset)
		if err != nil {
			return out, err
		}
		out["tasks"], out["total"] = tasks, total
		return out, nil
	}
	task, err := s.Store.MCPTask(ctx, in.TaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, fail("permission_denied", "任务不存在或不属于当前密钥")
		}
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, s.storageFault(err)
	}
	if task.KeyID != keyID {
		return out, fail("permission_denied", "任务不存在或不属于当前密钥")
	}
	if name == "tasks_cancel" {
		if err := s.Backend.ControlTask(ctx, task.ID, "cancel"); err != nil {
			return out, err
		}
		out["cancel_requested"] = true
		task, err = s.Store.MCPTask(ctx, in.TaskID)
		if err != nil {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			return out, s.storageFault(err)
		}
	}
	out["task"] = task
	return out, nil
}

func (s *Service) recoveryTool(ctx context.Context, key store.MCPKey, name string, raw json.RawMessage, op store.MCPOperation, out map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	in, err := decodeInput(name, raw)
	if err != nil {
		return out, err
	}
	out["audit_persisted"] = false
	out["audit_warning"] = "审计存储故障；本次查询或取消未能落盘"
	return s.taskTool(ctx, key.ID, name, in, out)
}
