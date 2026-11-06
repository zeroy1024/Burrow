package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type Group struct {
	ID, Scope, Name string
	Position        int
	Collapsed       bool
}

type Key struct {
	PublicKey                                                 []byte
	ID, Name, Kind, Fingerprint, SecretID, Comment, CreatedAt string
	Bits                                                      int
}

type Profile struct {
	ID, GroupID, KeyID, GatewayID string
	Position                      int
	Value                         []byte
}

type Gateway struct {
	ID, SSHProfileID, UpstreamID string
	Value                        []byte
}

type Endpoint struct {
	ID, KeyID string
	Value     []byte
}
type Workspace struct {
	Layout    []byte
	Endpoints []Endpoint
}
type Catalog struct {
	Gateways         []Gateway
	DefaultGatewayID string
	Groups           []Group
	Profiles         []Profile
	Keys             []Key
	Workspace        *Workspace
	VaultChanges     *VaultChanges
}

// EndpointID makes immutable connection snapshots independent of profile edits.
func EndpointID(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func (s *Store) ReadCatalog(ctx context.Context) (Catalog, error) {
	var c Catalog
	// One read transaction prevents a catalog/workspace mix from separate commits.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return c, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id,scope,name,position,collapsed FROM catalog_groups ORDER BY position,id")
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Scope, &g.Name, &g.Position, &g.Collapsed); err != nil {
			rows.Close()
			return c, err
		}
		c.Groups = append(c.Groups, g)
	}
	if err := closeRows(rows); err != nil {
		return c, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,group_id,position,coalesce(key_id,''),coalesce(gateway_id,''),value FROM profiles ORDER BY group_id,position")
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.GroupID, &p.Position, &p.KeyID, &p.GatewayID, &p.Value); err != nil {
			rows.Close()
			return c, err
		}
		c.Profiles = append(c.Profiles, p)
	}
	if err := closeRows(rows); err != nil {
		return c, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,coalesce(ssh_profile_id,''),coalesce(upstream_id,''),value FROM gateways ORDER BY rowid")
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var g Gateway
		if err := rows.Scan(&g.ID, &g.SSHProfileID, &g.UpstreamID, &g.Value); err != nil {
			rows.Close()
			return c, err
		}
		c.Gateways = append(c.Gateways, g)
	}
	if err := closeRows(rows); err != nil {
		return c, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(default_id,'') FROM gateway_settings WHERE id=1").Scan(&c.DefaultGatewayID); err != nil {
		return c, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,name,kind,fingerprint,secret_id,comment,bits,created_at,public_key FROM managed_keys ORDER BY created_at,id")
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.Kind, &k.Fingerprint, &k.SecretID, &k.Comment, &k.Bits, &k.CreatedAt, &k.PublicKey); err != nil {
			rows.Close()
			return c, err
		}
		c.Keys = append(c.Keys, k)
	}
	if err := closeRows(rows); err != nil {
		return c, err
	}
	var layout []byte
	err = tx.QueryRowContext(ctx, "SELECT layout FROM workspace WHERE id=1").Scan(&layout)
	if err == nil {
		c.Workspace = &Workspace{Layout: layout}
		rows, err = tx.QueryContext(ctx, "SELECT e.id,coalesce(e.key_id,''),e.value FROM endpoints e JOIN workspace_endpoints w ON w.endpoint_id=e.id")
		if err != nil {
			return c, err
		}
		for rows.Next() {
			var e Endpoint
			if err := rows.Scan(&e.ID, &e.KeyID, &e.Value); err != nil {
				rows.Close()
				return c, err
			}
			c.Workspace.Endpoints = append(c.Workspace.Endpoints, e)
		}
		if err := closeRows(rows); err != nil {
			return c, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	return c, tx.Commit()
}

func closeRows(rows *sql.Rows) error {
	err := rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) SaveCatalog(ctx context.Context, c Catalog) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveVaultChanges(ctx, tx, c.VaultChanges); err != nil {
		return err
	}
	// The catalog is small and its order is a single user operation. Replace it
	// atomically; immutable endpoints and task rows are never cascaded away.
	for _, table := range []string{"profiles", "gateways", "catalog_groups", "managed_keys"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	for _, k := range c.Keys {
		if _, err := tx.ExecContext(ctx, "INSERT INTO managed_keys VALUES(?,?,?,?,?,?,?,?,?)", k.ID, k.Name, k.Kind, k.Fingerprint, k.SecretID, k.Comment, k.Bits, k.CreatedAt, k.PublicKey); err != nil {
			return err
		}
	}
	for _, g := range c.Groups {
		if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_groups VALUES(?,?,?,?,?)", g.ID, g.Scope, g.Name, g.Position, g.Collapsed); err != nil {
			return err
		}
	}
	for _, p := range c.Profiles {
		if _, err := tx.ExecContext(ctx, "INSERT INTO profiles VALUES(?,?,?,?,?,?)", p.ID, p.GroupID, p.Position, nullable(p.KeyID), nullable(p.GatewayID), string(p.Value)); err != nil {
			return err
		}
	}
	for _, g := range c.Gateways {
		if _, err := tx.ExecContext(ctx, "INSERT INTO gateways VALUES(?,?,?,?)", g.ID, nullable(g.SSHProfileID), nullable(g.UpstreamID), string(g.Value)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE gateway_settings SET default_id=? WHERE id=1", nullable(c.DefaultGatewayID)); err != nil {
		return err
	}
	if c.Workspace != nil {
		if err := saveWorkspace(ctx, tx, c.Workspace); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func saveEndpoints(ctx context.Context, tx *sql.Tx, table string, values []Endpoint) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
		return err
	}
	for _, e := range values {
		if e.ID != EndpointID(e.Value) {
			return fmt.Errorf("invalid immutable endpoint %s", e.ID)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO endpoints(id,key_id,value) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", e.ID, nullable(e.KeyID), string(e.Value)); err != nil {
			return err
		}
		if err := saveEndpointKeys(ctx, tx, e.ID, e.Value); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+"(endpoint_id) VALUES(?) ON CONFLICT DO NOTHING", e.ID); err != nil {
			return err
		}
	}
	return nil
}

func saveEndpointKeys(ctx context.Context, tx *sql.Tx, id string, value []byte) error {
	var endpoint struct {
		GatewayRoute []struct {
			SSH *struct {
				KeyID string `json:"keyId"`
			} `json:"ssh"`
		} `json:"gatewayRoute"`
	}
	if err := json.Unmarshal(value, &endpoint); err != nil {
		return err
	}
	for _, hop := range endpoint.GatewayRoute {
		if hop.SSH == nil || hop.SSH.KeyID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO endpoint_keys VALUES(?,?) ON CONFLICT DO NOTHING", id, hop.SSH.KeyID); err != nil {
			return err
		}
	}
	return nil
}

func saveWorkspace(ctx context.Context, tx *sql.Tx, w *Workspace) error {
	if err := saveEndpoints(ctx, tx, "workspace_endpoints", w.Endpoints); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO workspace(id,version,layout) VALUES(1,1,?) ON CONFLICT(id) DO UPDATE SET layout=excluded.layout", string(w.Layout))
	if err != nil {
		return err
	}
	return collectEndpoints(ctx, tx)
}

func collectEndpoints(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM endpoints WHERE id NOT IN (SELECT endpoint_id FROM workspace_endpoints UNION SELECT endpoint_id FROM transfer_endpoints)")
	return err
}

func (s *Store) SaveWorkspace(ctx context.Context, w Workspace) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveWorkspace(ctx, tx, &w); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveDesktopState(ctx context.Context, w *Workspace, settings map[string][]byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if w != nil {
		if err := saveWorkspace(ctx, tx, w); err != nil {
			return err
		}
	}
	for key, value := range settings {
		if _, err := tx.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, string(value)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
