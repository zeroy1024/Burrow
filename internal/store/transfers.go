package store

import (
	"context"
	"database/sql"
	"errors"
)

type TransferBatch struct{ ID, Policy int }
type TransferValue struct {
	ID    int
	Value []byte
}
type TransferTask struct {
	ID, BatchID, SessionID int
	Value                  []byte
	PreserveManifest       bool
}
type Transfers struct {
	NextID, NextBatchID   int
	Batches               []TransferBatch
	Sessions, Connections []TransferValue
	Tasks                 []TransferTask
	Endpoints             []Endpoint
}

func (s *Store) ReadTransfers(ctx context.Context) (Transfers, error) {
	var q Transfers
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return q, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "SELECT next_id,next_batch_id FROM transfer_state WHERE id=1").Scan(&q.NextID, &q.NextBatchID)
	if errors.Is(err, sql.ErrNoRows) {
		return q, nil
	}
	if err != nil {
		return q, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,policy FROM transfer_batches ORDER BY id")
	if err != nil {
		return q, err
	}
	for rows.Next() {
		var b TransferBatch
		if err := rows.Scan(&b.ID, &b.Policy); err != nil {
			rows.Close()
			return q, err
		}
		q.Batches = append(q.Batches, b)
	}
	if err := closeRows(rows); err != nil {
		return q, err
	}
	for _, target := range []struct {
		table  string
		values *[]TransferValue
	}{{"transfer_sessions", &q.Sessions}, {"transfer_connections", &q.Connections}} {
		rows, err := tx.QueryContext(ctx, "SELECT id,value FROM "+target.table+" ORDER BY id")
		if err != nil {
			return q, err
		}
		for rows.Next() {
			var v TransferValue
			if err := rows.Scan(&v.ID, &v.Value); err != nil {
				rows.Close()
				return q, err
			}
			*target.values = append(*target.values, v)
		}
		if err := closeRows(rows); err != nil {
			return q, err
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,batch_id,session_id,value FROM transfer_tasks ORDER BY id")
	if err != nil {
		return q, err
	}
	for rows.Next() {
		var t TransferTask
		if err := rows.Scan(&t.ID, &t.BatchID, &t.SessionID, &t.Value); err != nil {
			rows.Close()
			return q, err
		}
		q.Tasks = append(q.Tasks, t)
	}
	if err := closeRows(rows); err != nil {
		return q, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT e.id,coalesce(e.key_id,''),e.value FROM endpoints e JOIN transfer_endpoints t ON t.endpoint_id=e.id")
	if err != nil {
		return q, err
	}
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(&e.ID, &e.KeyID, &e.Value); err != nil {
			rows.Close()
			return q, err
		}
		q.Endpoints = append(q.Endpoints, e)
	}
	if err := closeRows(rows); err != nil {
		return q, err
	}
	return q, tx.Commit()
}

func (s *Store) SaveTransfers(ctx context.Context, q Transfers) error {
	return s.SaveTransfersWithMCP(ctx, q, nil)
}

// SaveTransfersWithMCP commits the recoverable queue and MCP ownership together.
// A task is not accepted while only its public identity is durable.
func (s *Store) SaveTransfersWithMCP(ctx context.Context, q Transfers, tasks []MCPTask) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Only changed task/checkpoint payloads are updated. UI-only progress never
	// creates a write; workers publish acknowledged checkpoints at safe points.
	rows, err := tx.QueryContext(ctx, "SELECT id FROM transfer_tasks")
	if err != nil {
		return err
	}
	var old []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		old = append(old, id)
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	keep := map[int]bool{}
	for _, t := range q.Tasks {
		keep[t.ID] = true
	}
	for _, id := range old {
		if !keep[id] {
			if _, err := tx.ExecContext(ctx, "DELETE FROM transfer_tasks WHERE id=?", id); err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"transfer_batches", "transfer_sessions", "transfer_connections"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	for _, b := range q.Batches {
		if _, err := tx.ExecContext(ctx, "INSERT INTO transfer_batches VALUES(?,?)", b.ID, b.Policy); err != nil {
			return err
		}
	}
	for _, target := range []struct {
		table  string
		values []TransferValue
	}{{"transfer_sessions", q.Sessions}, {"transfer_connections", q.Connections}} {
		for _, v := range target.values {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+target.table+" VALUES(?,?)", v.ID, string(v.Value)); err != nil {
				return err
			}
		}
	}
	for _, t := range q.Tasks {
		value := "excluded.value"
		if t.PreserveManifest {
			value = `json_set(excluded.value,
 '$.root',json_extract(transfer_tasks.value,'$.root'),
 '$.temporary',json_extract(transfer_tasks.value,'$.temporary'),
 '$.total',json_extract(transfer_tasks.value,'$.total'),
 '$.doneBytes',json_extract(transfer_tasks.value,'$.doneBytes'),
 '$.sourcePath',json_extract(transfer_tasks.value,'$.sourcePath'),
 '$.targetPath',json_extract(transfer_tasks.value,'$.targetPath'),
 '$.entry.Name',json_extract(transfer_tasks.value,'$.entry.Name'))`
		}
		query := `INSERT INTO transfer_tasks VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET batch_id=excluded.batch_id,session_id=excluded.session_id,value=` + value + ` WHERE transfer_tasks.batch_id!=excluded.batch_id OR
 transfer_tasks.session_id!=excluded.session_id OR transfer_tasks.value!=` + value
		if _, err := tx.ExecContext(ctx, query, t.ID, t.BatchID, t.SessionID, string(t.Value)); err != nil {
			return err
		}
	}
	if err := saveEndpoints(ctx, tx, "transfer_endpoints", q.Endpoints); err != nil {
		return err
	}
	if err := collectEndpoints(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO transfer_state VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET next_id=excluded.next_id,next_batch_id=excluded.next_batch_id", q.NextID, q.NextBatchID); err != nil {
		return err
	}
	for _, task := range tasks {
		if !keep[task.ID] {
			return errors.New("MCP task has no recoverable transfer row")
		}
		if err := mcpSaveTask(ctx, tx, task); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Manifest contains only fields owned by the active transfer worker.
type Manifest struct {
	Root, Temporary                   []byte
	Total, Done                       int64
	SourcePath, TargetPath, EntryName string
}

// SaveManifest atomically publishes a checkpoint and its associated paths.
func (s *Store) SaveManifest(ctx context.Context, id int, m Manifest) error {
	result, err := s.db.ExecContext(ctx, `UPDATE transfer_tasks SET value=json_set(value,'$.root',json(?),'$.temporary',json(?),'$.total',?,'$.doneBytes',?,'$.sourcePath',?,'$.targetPath',?,'$.entry.Name',?) WHERE id=?`, string(m.Root), string(m.Temporary), m.Total, m.Done, m.SourcePath, m.TargetPath, m.EntryName, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("transfer task is not persisted")
	}
	return nil
}
