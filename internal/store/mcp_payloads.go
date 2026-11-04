package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// MCPAuditPayload is loaded only for a selected operation. Nil fields mean
// "not provided" when updating, preserving a request across task progress writes.
type MCPAuditPayload struct {
	Request, Response json.RawMessage
}

func validateMCPPayload(payload MCPAuditPayload) error {
	for _, value := range []json.RawMessage{payload.Request, payload.Response} {
		if value != nil && !json.Valid(value) {
			return errors.New("invalid MCP audit payload JSON")
		}
	}
	return nil
}

func (s *Store) MCPBeginWithPayload(ctx context.Context, op MCPOperation, payload MCPAuditPayload) error {
	if err := validateMCPPayload(payload); err != nil {
		return err
	}
	return s.mcpBegin(ctx, op, nil, &payload)
}

func writeMCPPayload(ctx context.Context, tx *sql.Tx, operationID string, payload MCPAuditPayload) error {
	var request, response any
	if payload.Request != nil {
		request = string(payload.Request)
	}
	if payload.Response != nil {
		response = string(payload.Response)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO mcp_payloads(operation_id,request,response) VALUES(?,?,?)
 ON CONFLICT(operation_id) DO UPDATE SET request=coalesce(excluded.request,mcp_payloads.request),
 response=coalesce(excluded.response,mcp_payloads.response)`, operationID, request, response)
	return err
}

func (s *Store) MCPUpdatePayload(ctx context.Context, operationID string, payload MCPAuditPayload) error {
	if err := validateMCPPayload(payload); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := writeMCPPayload(ctx, tx, operationID, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MCPPayload(ctx context.Context, operationID string) (MCPAuditPayload, error) {
	var request, response []byte
	err := s.db.QueryRowContext(ctx, "SELECT request,response FROM mcp_payloads WHERE operation_id=?", operationID).
		Scan(&request, &response)
	return MCPAuditPayload{Request: request, Response: response}, err
}
