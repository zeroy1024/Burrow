package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type MCPGrant struct {
	ConnectionID      string `json:"connection_id"`
	EndpointDigest    string `json:"endpoint_digest"`
	Read, Write, Exec bool
}

type MCPKey struct {
	ID, Name, Prefix, State  string
	Digest                   []byte `json:"-"`
	CreatedAt, LastUsedAt    time.Time
	Grants                   []MCPGrant
	UploadDirs, DownloadDirs []string
}

type MCPCommandRule struct {
	Mode, Pattern string
	// IgnoreCase folds argument case for "command" rules; program names always
	// compare case-insensitively.
	IgnoreCase bool `json:",omitempty"`
}

type MCPFileRule struct {
	Side, Kind, Path string
	IncludeSubdirs   bool
	DeniedActions    map[string]bool
}

type MCPPolicy struct {
	ID, Name, Description, Category  string
	Enabled, AllConnections, AllKeys bool
	Version                          int
	ConnectionIDs, KeyIDs            []string
	CommandRules                     []MCPCommandRule
	FileRules                        []MCPFileRule
}

type MCPHit struct {
	PolicyID, Name, Category, Rule, Action string
	Version                                int
}

type MCPEvent struct {
	Stage, Detail string
	At            time.Time
}

type MCPOperation struct {
	ID, KeyID, KeyName, ConnectionID, Action, Status, ErrorCode, Error string
	StartedAt, FinishedAt                                              time.Time
	Target, Request, Result                                            json.RawMessage
	Hits                                                               []MCPHit
	Timeline                                                           []MCPEvent
	TaskID                                                             int
}

type MCPFileEvent struct {
	ID                                             int64
	OperationID, Path, Side, Action, Status, Error string
	Bytes                                          int64
	At                                             time.Time
}

type MCPTask struct {
	ID                                             int
	OperationID, KeyID, ConnectionID, State, Error string
	Done, Total                                    int64
	CreatedAt, UpdatedAt                           time.Time
}

type MCPOperationFilter struct {
	KeyID, Status, Action  string
	PolicyID, ActionPrefix string
	ToolsOnly              bool
	ConnectionIDs          []string
	Statuses               []string
	Since, Until           time.Time
	Limit, Offset          int
}

const mcpDateLayout = "2006-01-02T15:04:05.000000000Z"

var ErrMCPKeyRevoked = errors.New("revoked MCP key cannot be reactivated")

func mcpDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(mcpDateLayout)
}

func mcpParseDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

type mcpScanner interface{ Scan(...any) error }

const mcpKeyColumns = "id,name,prefix,state,digest,created_at,last_used_at,grants,upload_dirs,download_dirs"

func scanMCPKey(row mcpScanner) (MCPKey, error) {
	var key MCPKey
	var created, used string
	var grants, upload, download []byte
	err := row.Scan(&key.ID, &key.Name, &key.Prefix, &key.State, &key.Digest, &created, &used, &grants, &upload, &download)
	if err != nil {
		return key, err
	}
	if key.CreatedAt, err = mcpParseDate(created); err != nil {
		return key, err
	}
	if key.LastUsedAt, err = mcpParseDate(used); err != nil {
		return key, err
	}
	if err := json.Unmarshal(grants, &key.Grants); err != nil {
		return key, err
	}
	if err := json.Unmarshal(upload, &key.UploadDirs); err != nil {
		return key, err
	}
	return key, json.Unmarshal(download, &key.DownloadDirs)
}

func (s *Store) MCPKeys(ctx context.Context) ([]MCPKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+mcpKeyColumns+" FROM mcp_keys ORDER BY created_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []MCPKey{}
	for rows.Next() {
		key, err := scanMCPKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, closeRows(rows)
}

func (s *Store) MCPKey(ctx context.Context, id string) (MCPKey, error) {
	return scanMCPKey(s.db.QueryRowContext(ctx, "SELECT "+mcpKeyColumns+" FROM mcp_keys WHERE id=?", id))
}

func (s *Store) MCPCreateKey(ctx context.Context, key MCPKey) error {
	grants, err := json.Marshal(key.Grants)
	if err != nil {
		return err
	}
	upload, err := json.Marshal(key.UploadDirs)
	if err != nil {
		return err
	}
	download, err := json.Marshal(key.DownloadDirs)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO mcp_keys(`+mcpKeyColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		key.ID, key.Name, key.Prefix, key.State, key.Digest, mcpDate(key.CreatedAt), mcpDate(key.LastUsedAt),
		string(grants), string(upload), string(download))
	if err != nil {
		return err
	}
	if err := mcpManagement(ctx, tx, "key.create", key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPConfigure(ctx context.Context, config []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('mcp',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(config)); err != nil {
		return err
	}
	if err := mcpManagement(ctx, tx, "service.configure", json.RawMessage(config)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPUpdateKey(ctx context.Context, key MCPKey) error {
	grants, err := json.Marshal(key.Grants)
	if err != nil {
		return err
	}
	upload, err := json.Marshal(key.UploadDirs)
	if err != nil {
		return err
	}
	download, err := json.Marshal(key.DownloadDirs)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldState string
	if err := tx.QueryRowContext(ctx, "SELECT state FROM mcp_keys WHERE id=?", key.ID).Scan(&oldState); err != nil {
		return err
	}
	if oldState == "revoked" && key.State != "revoked" {
		return ErrMCPKeyRevoked
	}
	// Authentication touches last_used_at concurrently. A settings edit never
	// overwrites that timestamp, the creation time, or the immutable secret digest.
	_, err = tx.ExecContext(ctx, `UPDATE mcp_keys SET name=?,state=?,grants=?,upload_dirs=?,download_dirs=? WHERE id=?`,
		key.Name, key.State, string(grants), string(upload), string(download), key.ID)
	if err != nil {
		return err
	}
	action := "key.update"
	if oldState != key.State {
		action = "key." + key.State
	}
	if err := mcpManagement(ctx, tx, action, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPSetKeyState(ctx context.Context, id, state string) (MCPKey, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPKey{}, err
	}
	defer tx.Rollback()
	key, err := scanMCPKey(tx.QueryRowContext(ctx, "SELECT "+mcpKeyColumns+" FROM mcp_keys WHERE id=?", id))
	if err != nil {
		return MCPKey{}, err
	}
	if key.State == "revoked" && state != "revoked" {
		return MCPKey{}, ErrMCPKeyRevoked
	}
	key.State = state
	if state == "revoked" {
		key.Grants, key.UploadDirs, key.DownloadDirs = nil, nil, nil
		_, err = tx.ExecContext(ctx, "UPDATE mcp_keys SET state=?,grants='null',upload_dirs='null',download_dirs='null' WHERE id=?", state, id)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE mcp_keys SET state=? WHERE id=?", state, id)
	}
	if err != nil {
		return MCPKey{}, err
	}
	if err := mcpManagement(ctx, tx, "key."+state, key); err != nil {
		return MCPKey{}, err
	}
	if err := tx.Commit(); err != nil {
		return MCPKey{}, err
	}
	return key, nil
}

func (s *Store) MCPTouchKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE mcp_keys SET last_used_at=? WHERE id=? AND state='active'", mcpDate(time.Now()), id)
	return mcpChanged(result, err)
}

func (s *Store) MCPPolicies(ctx context.Context) ([]MCPPolicy, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT value FROM mcp_policies ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	policies := []MCPPolicy{}
	for rows.Next() {
		var value []byte
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		var policy MCPPolicy
		if err := json.Unmarshal(value, &policy); err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, closeRows(rows)
}

func (s *Store) MCPSavePolicy(ctx context.Context, policy MCPPolicy) error {
	value, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	err = tx.QueryRowContext(ctx, "SELECT version FROM mcp_policies WHERE id=?", policy.ID).Scan(&version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	action := "policy.create"
	if err == nil {
		if policy.Version <= version {
			return errors.New("MCP policy version must increase")
		}
		action = "policy.update"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mcp_policies(id,version,enabled,category,value) VALUES(?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET version=excluded.version,enabled=excluded.enabled,category=excluded.category,value=excluded.value`,
		policy.ID, policy.Version, policy.Enabled, policy.Category, string(value))
	if err != nil {
		return err
	}
	if err := mcpManagement(ctx, tx, action, policy); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPSetPolicyEnabled(ctx context.Context, id string, enabled bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var value []byte
	if err := tx.QueryRowContext(ctx, `UPDATE mcp_policies
 SET enabled=?,value=json_set(value,'$.Enabled',json(?)) WHERE id=? RETURNING value`,
		enabled, strconv.FormatBool(enabled), id).Scan(&value); err != nil {
		return err
	}
	action := "policy.disable"
	if enabled {
		action = "policy.enable"
	}
	if err := mcpManagement(ctx, tx, action, json.RawMessage(value)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPDeletePolicy(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var value []byte
	if err := tx.QueryRowContext(ctx, "SELECT value FROM mcp_policies WHERE id=?", id).Scan(&value); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM mcp_policies WHERE id=?", id); err != nil {
		return err
	}
	if err := mcpManagement(ctx, tx, "policy.delete", json.RawMessage(value)); err != nil {
		return err
	}
	return tx.Commit()
}

func mcpManagement(ctx context.Context, tx *sql.Tx, action string, target any) error {
	value, err := json.Marshal(target)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return mcpInsertOperation(ctx, tx, MCPOperation{
		ID: "op_" + rand.Text(), KeyName: "Burrow 本地用户", Action: action, Status: "completed",
		StartedAt: now, FinishedAt: now, Target: value,
		Timeline: []MCPEvent{{Stage: "completed", Detail: action, At: now}},
	})
}

func mcpRaw(value json.RawMessage) string {
	if len(value) == 0 {
		return "null"
	}
	return string(value)
}

const mcpOperationColumns = `id,key_id,key_name,connection_id,action,status,error_code,error,
 started_at,finished_at,target,request,result,hits,timeline,task_id`

func mcpInsertOperation(ctx context.Context, tx *sql.Tx, op MCPOperation) error {
	hits, err := json.Marshal(op.Hits)
	if err != nil {
		return err
	}
	timeline, err := json.Marshal(op.Timeline)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO mcp_operations(`+mcpOperationColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		op.ID, op.KeyID, op.KeyName, op.ConnectionID, op.Action, op.Status, op.ErrorCode, op.Error,
		mcpDate(op.StartedAt), mcpDate(op.FinishedAt), mcpRaw(op.Target), mcpRaw(op.Request), mcpRaw(op.Result),
		string(hits), string(timeline), op.TaskID)
	return err
}

func (s *Store) MCPBegin(ctx context.Context, op MCPOperation, task *MCPTask) error {
	return s.mcpBegin(ctx, op, task, nil)
}

func (s *Store) mcpBegin(ctx context.Context, op MCPOperation, task *MCPTask, payload *MCPAuditPayload) error {
	if task != nil {
		if task.OperationID != op.ID || task.KeyID != op.KeyID || task.ConnectionID != op.ConnectionID {
			return errors.New("MCP task identity does not match its operation")
		}
		op.TaskID = task.ID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := mcpInsertOperation(ctx, tx, op); err != nil {
		return err
	}
	if payload != nil {
		if err := writeMCPPayload(ctx, tx, op.ID, *payload); err != nil {
			return err
		}
	}
	if task != nil {
		_, err := tx.ExecContext(ctx, `INSERT INTO mcp_tasks(id,operation_id,key_id,connection_id,state,error,done,total,created_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?)`, task.ID, task.OperationID, task.KeyID, task.ConnectionID, task.State, task.Error,
			task.Done, task.Total, mcpDate(task.CreatedAt), mcpDate(task.UpdatedAt))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MCPFinish(ctx context.Context, op MCPOperation) error {
	hits, err := json.Marshal(op.Hits)
	if err != nil {
		return err
	}
	timeline, err := json.Marshal(op.Timeline)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE mcp_operations SET connection_id=?,target=?,request=?,status=?,error_code=?,error=?,finished_at=?,result=?,hits=?,timeline=? WHERE id=?`,
		op.ConnectionID, mcpRaw(op.Target), mcpRaw(op.Request), op.Status, op.ErrorCode, op.Error,
		mcpDate(op.FinishedAt), mcpRaw(op.Result), string(hits), string(timeline), op.ID)
	return mcpChanged(result, err)
}

func (s *Store) MCPOperation(ctx context.Context, id string) (MCPOperation, error) {
	return scanMCPOperation(s.db.QueryRowContext(ctx, "SELECT "+mcpOperationColumns+" FROM mcp_operations WHERE id=?", id))
}

func scanMCPOperation(row mcpScanner) (MCPOperation, error) {
	var op MCPOperation
	var started, finished string
	var target, request, result, hits, timeline []byte
	err := row.Scan(&op.ID, &op.KeyID, &op.KeyName, &op.ConnectionID, &op.Action, &op.Status, &op.ErrorCode, &op.Error,
		&started, &finished, &target, &request, &result, &hits, &timeline, &op.TaskID)
	if err != nil {
		return op, err
	}
	op.Target, op.Request, op.Result = target, request, result
	if op.StartedAt, err = mcpParseDate(started); err != nil {
		return op, err
	}
	if op.FinishedAt, err = mcpParseDate(finished); err != nil {
		return op, err
	}
	if err := json.Unmarshal(hits, &op.Hits); err != nil {
		return op, err
	}
	return op, json.Unmarshal(timeline, &op.Timeline)
}

func mcpPage(limit, offset int) (int, error) {
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 1000 || offset < 0 {
		return 0, errors.New("MCP page requires a limit from 1 to 1000 and a nonnegative offset")
	}
	return limit, nil
}

func (s *Store) MCPOperations(ctx context.Context, filter MCPOperationFilter) ([]MCPOperation, int, error) {
	limit, err := mcpPage(filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	where := []string{"1=1"}
	args := []any{}
	for _, field := range []struct{ column, value string }{
		{"key_id", filter.KeyID}, {"status", filter.Status}, {"action", filter.Action},
	} {
		if field.value != "" {
			where = append(where, field.column+"=?")
			args = append(args, field.value)
		}
	}
	if len(filter.ConnectionIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(filter.ConnectionIDs)), ",")
		where = append(where, "connection_id IN ("+placeholders+")")
		for _, id := range filter.ConnectionIDs {
			args = append(args, id)
		}
	}
	if len(filter.Statuses) > 0 {
		where = append(where, "status IN ("+strings.TrimSuffix(strings.Repeat("?,", len(filter.Statuses)), ",")+")")
		for _, status := range filter.Statuses {
			args = append(args, status)
		}
	}
	if filter.PolicyID != "" {
		where = append(where, "EXISTS (SELECT 1 FROM json_each(mcp_operations.hits) hit WHERE json_extract(hit.value,'$.PolicyID')=?)")
		args = append(args, filter.PolicyID)
	}
	if filter.ActionPrefix != "" {
		where = append(where, "substr(action,1,length(?))=?")
		args = append(args, filter.ActionPrefix, filter.ActionPrefix)
	}
	if filter.ToolsOnly {
		where = append(where, "key_id<>''", "action<>'authentication'", "substr(action,1,4)<>'mcp.'")
	}
	if !filter.Since.IsZero() {
		where = append(where, "started_at>=?")
		args = append(args, mcpDate(filter.Since))
	}
	if !filter.Until.IsZero() {
		where = append(where, "started_at<=?")
		args = append(args, mcpDate(filter.Until))
	}
	condition := strings.Join(where, " AND ")
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM mcp_operations WHERE "+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+mcpOperationColumns+" FROM mcp_operations WHERE "+condition+" ORDER BY started_at DESC,id DESC LIMIT ? OFFSET ?", append(args, limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	ops := []MCPOperation{}
	for rows.Next() {
		op, err := scanMCPOperation(rows)
		if err != nil {
			return nil, 0, err
		}
		ops = append(ops, op)
	}
	if err := closeRows(rows); err != nil {
		return nil, 0, err
	}
	return ops, total, tx.Commit()
}

func (s *Store) MCPAddFile(ctx context.Context, event MCPFileEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_files(operation_id,path,side,action,status,error,bytes,at) VALUES(?,?,?,?,?,?,?,?)`,
		event.OperationID, event.Path, event.Side, event.Action, event.Status, event.Error, event.Bytes, mcpDate(event.At))
	return err
}

func (s *Store) MCPAddFiles(ctx context.Context, events []MCPFileEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO mcp_files(operation_id,path,side,action,status,error,bytes,at) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, event := range events {
		if _, err := statement.ExecContext(ctx, event.OperationID, event.Path, event.Side, event.Action, event.Status, event.Error, event.Bytes, mcpDate(event.At)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const mcpFileColumns = "id,operation_id,path,side,action,status,error,bytes,at"

func scanMCPFile(row mcpScanner) (MCPFileEvent, error) {
	var file MCPFileEvent
	var at string
	if err := row.Scan(&file.ID, &file.OperationID, &file.Path, &file.Side, &file.Action, &file.Status, &file.Error, &file.Bytes, &at); err != nil {
		return file, err
	}
	var err error
	file.At, err = mcpParseDate(at)
	return file, err
}

func (s *Store) MCPFiles(ctx context.Context, operationID string, limit, offset int) ([]MCPFileEvent, error) {
	limit, err := mcpPage(limit, offset)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, "SELECT "+mcpFileColumns+" FROM mcp_files WHERE operation_id=? ORDER BY id LIMIT ? OFFSET ?", operationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []MCPFileEvent{}
	for rows.Next() {
		file, err := scanMCPFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, closeRows(rows)
}

func (s *Store) MCPFileSummary(ctx context.Context, operationID string, limit, offset int) ([]MCPFileEvent, int, map[string]int, error) {
	limit, err := mcpPage(limit, offset)
	if err != nil {
		return nil, 0, nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, nil, err
	}
	defer tx.Rollback()
	const latest = `WITH latest AS (
 SELECT f.* FROM mcp_files f JOIN (
  SELECT max(id) id FROM mcp_files WHERE operation_id=? GROUP BY side,path
 ) grouped ON grouped.id=f.id
) `
	rows, err := tx.QueryContext(ctx, latest+"SELECT status,count(*) FROM latest GROUP BY status", operationID)
	if err != nil {
		return nil, 0, nil, err
	}
	counts, total := map[string]int{}, 0
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, 0, nil, err
		}
		counts[status] = count
		total += count
	}
	if err := closeRows(rows); err != nil {
		return nil, 0, nil, err
	}
	rows, err = tx.QueryContext(ctx, latest+"SELECT "+mcpFileColumns+" FROM latest ORDER BY side,path LIMIT ? OFFSET ?", operationID, limit, offset)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	files := []MCPFileEvent{}
	for rows.Next() {
		file, err := scanMCPFile(rows)
		if err != nil {
			return nil, 0, nil, err
		}
		files = append(files, file)
	}
	if err := closeRows(rows); err != nil {
		return nil, 0, nil, err
	}
	return files, total, counts, tx.Commit()
}

const mcpTaskColumns = "id,operation_id,key_id,connection_id,state,error,done,total,created_at,updated_at"

func scanMCPTask(row mcpScanner) (MCPTask, error) {
	var task MCPTask
	var created, updated string
	err := row.Scan(&task.ID, &task.OperationID, &task.KeyID, &task.ConnectionID, &task.State, &task.Error, &task.Done, &task.Total, &created, &updated)
	if err != nil {
		return task, err
	}
	if task.CreatedAt, err = mcpParseDate(created); err != nil {
		return task, err
	}
	task.UpdatedAt, err = mcpParseDate(updated)
	return task, err
}

func (s *Store) MCPTasks(ctx context.Context, keyID string) ([]MCPTask, error) {
	query := "SELECT " + mcpTaskColumns + " FROM mcp_tasks"
	args := []any{}
	if keyID != "" {
		query += " WHERE key_id=?"
		args = append(args, keyID)
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []MCPTask{}
	for rows.Next() {
		task, err := scanMCPTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, closeRows(rows)
}

func (s *Store) MCPTasksPage(ctx context.Context, keyID, state string, limit, offset int) ([]MCPTask, int, error) {
	if limit < 1 || limit > 1000 || offset < 0 {
		return nil, 0, errors.New("MCP task page requires a limit from 1 to 1000 and a nonnegative offset")
	}
	where := "key_id=?"
	args := []any{keyID}
	if state != "" {
		where += " AND state=?"
		args = append(args, state)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM mcp_tasks WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+mcpTaskColumns+" FROM mcp_tasks WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	tasks := []MCPTask{}
	for rows.Next() {
		task, err := scanMCPTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, task)
	}
	if err := closeRows(rows); err != nil {
		return nil, 0, err
	}
	return tasks, total, tx.Commit()
}

func (s *Store) MCPTask(ctx context.Context, id int) (MCPTask, error) {
	return scanMCPTask(s.db.QueryRowContext(ctx, "SELECT "+mcpTaskColumns+" FROM mcp_tasks WHERE id=?", id))
}

// MCPProgress updates counters without replaying a stale UI lifecycle state.
func (s *Store) MCPProgress(ctx context.Context, id int, done, total int64) error {
	result, err := s.db.ExecContext(ctx, "UPDATE mcp_tasks SET done=?,total=?,updated_at=? WHERE id=?", done, max(total, done), mcpDate(time.Now()), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (s *Store) MCPSaveTask(ctx context.Context, task MCPTask) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := mcpSaveTask(ctx, tx, task); err != nil {
		return err
	}
	return tx.Commit()
}

// MCPSaveTaskEvent changes a cleanup task without replacing the parent
// operation's business result, which may still be finishing on another worker.
func (s *Store) MCPSaveTaskEvent(ctx context.Context, task MCPTask) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := mcpSaveTask(ctx, tx, task); err != nil {
		return err
	}
	event, err := json.Marshal(MCPEvent{Stage: task.State, Detail: task.Error, At: task.UpdatedAt})
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE mcp_operations SET timeline=json_insert(coalesce(nullif(timeline,'null'),'[]'),'$[#]',json(?)) WHERE id=?`, string(event), task.OperationID)
	if err := mcpChanged(result, err); err != nil {
		return err
	}
	return tx.Commit()
}

func mcpSaveTask(ctx context.Context, tx *sql.Tx, task MCPTask) error {
	result, err := tx.ExecContext(ctx, `UPDATE mcp_operations SET task_id=? WHERE id=? AND key_id=? AND connection_id=? AND task_id IN (0,?)`,
		task.ID, task.OperationID, task.KeyID, task.ConnectionID, task.ID)
	if err := mcpChanged(result, err); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `INSERT INTO mcp_tasks(`+mcpTaskColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET state=excluded.state,error=excluded.error,done=excluded.done,total=excluded.total,updated_at=excluded.updated_at
 WHERE mcp_tasks.operation_id=excluded.operation_id AND mcp_tasks.key_id=excluded.key_id AND mcp_tasks.connection_id=excluded.connection_id`,
		task.ID, task.OperationID, task.KeyID, task.ConnectionID, task.State, task.Error,
		task.Done, task.Total, mcpDate(task.CreatedAt), mcpDate(task.UpdatedAt))
	return mcpChanged(result, err)
}

func mcpChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MCPInterrupt(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	event, err := json.Marshal(MCPEvent{Stage: "interrupted", Detail: "application stopped before completion; result requires verification", At: now})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE mcp_operations SET status='interrupted',error_code='interrupted',error='application stopped before completion; result requires verification',finished_at=?,timeline=json_insert(coalesce(nullif(timeline,'null'),'[]'),'$[#]',json(?)) WHERE finished_at='' AND status IN ('queued','running','waiting','waiting_user','cancelling','cancel_requested','pausing')`, mcpDate(now), string(event))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE mcp_tasks SET state='interrupted',error='application stopped before completion; requires manual resume',updated_at=? WHERE state IN ('queued','running','waiting','waiting_user','cancelling','cancel_requested','pausing')`, mcpDate(now))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_tasks SET state='cleanup_pending',error='temporary file cleanup interrupted; requires manual retry',updated_at=? WHERE state='cleaning'`, mcpDate(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_files SET status='interrupted',error='application stopped before completion' WHERE status IN ('queued','running','started','reading','writing')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPPrune(ctx context.Context, before time.Time) error {
	if before.IsZero() {
		return errors.New("MCP audit cleanup requires an explicit cutoff")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM mcp_operations WHERE finished_at<>'' AND finished_at<?
 AND status IN ('completed','done','success','failed','cancelled','canceled','intercepted','denied','auth_failed','authentication_failed','permission_denied','timed_out','partial')
 AND NOT EXISTS (SELECT 1 FROM mcp_tasks t WHERE t.operation_id=mcp_operations.id AND t.state NOT IN ('completed','done','success','failed','cancelled','canceled','intercepted','partial_intercepted'))
 AND NOT EXISTS (SELECT 1 FROM mcp_tasks t JOIN transfer_tasks q ON q.id=t.id WHERE t.operation_id=mcp_operations.id)`, mcpDate(before))
	return err
}
