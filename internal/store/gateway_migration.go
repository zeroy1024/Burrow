package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// migrateGateways converts catalog references and immutable snapshots together.
// Endpoint IDs are content hashes, so every reference must move with its value.
func migrateGateways(ctx context.Context, tx *sql.Tx) error {
	type profile struct {
		id, group, key, jump string
		position             int
		value                map[string]any
	}
	profiles := []profile{}
	byID := map[string]map[string]any{}
	rows, err := tx.QueryContext(ctx, "SELECT id,group_id,position,coalesce(key_id,''),coalesce(jump_id,''),value FROM profiles")
	if err != nil {
		return err
	}
	for rows.Next() {
		var p profile
		var raw []byte
		if err := rows.Scan(&p.id, &p.group, &p.position, &p.key, &p.jump, &raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(raw, &p.value); err != nil {
			rows.Close()
			return err
		}
		id, _ := p.value["id"].(string)
		key, _ := p.value["keyId"].(string)
		ssh, _ := p.value["ssh"].(map[string]any)
		jump, _ := ssh["jumpHostId"].(string)
		if id != p.id || key != p.key || jump != p.jump {
			rows.Close()
			return errors.New("gateway migration: profile references do not match database")
		}
		profiles = append(profiles, p)
		byID[p.id] = p.value
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	jump := func(v map[string]any) string {
		ssh, _ := v["ssh"].(map[string]any)
		id, _ := ssh["jumpHostId"].(string)
		return id
	}
	gateways := map[string]bool{}
	for _, p := range profiles {
		if id := jump(p.value); id != "" {
			gateways[id] = true
		}
	}
	clean := func(v map[string]any) map[string]any {
		out := make(map[string]any, len(v))
		for k, value := range v {
			out[k] = value
		}
		if ssh, ok := v["ssh"].(map[string]any); ok {
			options := make(map[string]any, len(ssh))
			for k, value := range ssh {
				if k != "jumpHostId" {
					options[k] = value
				}
			}
			out["ssh"] = options
		}
		if out["protocol"] == "ssh" {
			out["protocol"] = ""
		}
		delete(out, "gatewayRoute")
		delete(out, "gateway")
		return out
	}
	hop := func(v map[string]any) map[string]any {
		id, _ := v["id"].(string)
		return map[string]any{"id": "ssh-" + id, "name": v["name"], "kind": "ssh", "ssh": clean(v)}
	}
	route := func(v map[string]any) ([]any, error) {
		result := []any{}
		seen := map[string]bool{}
		for id := jump(v); id != ""; id = jump(v) {
			if seen[id] {
				return nil, errors.New("gateway migration: SSH jump cycle")
			}
			seen[id] = true
			var ok bool
			v, ok = byID[id]
			if !ok {
				return nil, fmt.Errorf("gateway migration: missing SSH profile %s", id)
			}
			protocol, _ := v["protocol"].(string)
			if protocol != "" && protocol != "ssh" {
				return nil, errors.New("gateway migration: jump target is not an SSH profile")
			}
			result = append([]any{hop(v)}, result...)
		}
		return result, nil
	}
	for _, p := range profiles {
		if _, err := route(p.value); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE gateways (
	 id TEXT PRIMARY KEY, ssh_profile_id TEXT REFERENCES profiles(id) DEFERRABLE INITIALLY DEFERRED,
	 upstream_id TEXT REFERENCES gateways(id) DEFERRABLE INITIALLY DEFERRED,
	 value TEXT NOT NULL CHECK(json_valid(value))) STRICT;
	 CREATE TABLE gateway_settings (id INTEGER PRIMARY KEY CHECK(id=1),
	 default_id TEXT REFERENCES gateways(id) DEFERRABLE INITIALLY DEFERRED) STRICT;
	 INSERT INTO gateway_settings VALUES(1,NULL);
	 CREATE TABLE endpoint_keys (
	 endpoint_id TEXT NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
	 key_id TEXT NOT NULL REFERENCES managed_keys(id) DEFERRABLE INITIALLY DEFERRED,
	 PRIMARY KEY(endpoint_id,key_id)) STRICT;
	 DROP TABLE profiles;
	 CREATE TABLE profiles (
	 id TEXT PRIMARY KEY, group_id TEXT NOT NULL REFERENCES catalog_groups(id) DEFERRABLE INITIALLY DEFERRED,
	 position INTEGER NOT NULL CHECK(position>=0), key_id TEXT REFERENCES managed_keys(id) DEFERRABLE INITIALLY DEFERRED,
	 gateway_id TEXT REFERENCES gateways(id) DEFERRABLE INITIALLY DEFERRED,
	 value TEXT NOT NULL CHECK(json_valid(value)), UNIQUE(group_id,position)) STRICT;
	 CREATE TABLE vault_items_v5 (
	 secret_id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN ('session_password','private_key','key_passphrase','gateway_password')),
	 revision TEXT NOT NULL, crypto_epoch INTEGER NOT NULL CHECK(crypto_epoch>0),
	 nonce BLOB NOT NULL CHECK(length(nonce)=24), ciphertext BLOB NOT NULL CHECK(length(ciphertext)>=16)) STRICT;
	 INSERT INTO vault_items_v5 SELECT * FROM vault_items;
	 DROP TABLE vault_items;
	 ALTER TABLE vault_items_v5 RENAME TO vault_items;`); err != nil {
		return err
	}
	for id := range gateways {
		v, ok := byID[id]
		if !ok {
			return fmt.Errorf("gateway migration: missing SSH profile %s", id)
		}
		// An SSH gateway follows its profile's own gateway, which the profile
		// loop below sets from that profile's jump, keeping multi-hop paths.
		data, err := json.Marshal(map[string]any{"id": "ssh-" + id, "name": v["name"], "kind": "ssh", "sshProfileId": id})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO gateways(id,ssh_profile_id,upstream_id,value) VALUES(?,?,NULL,?)",
			"ssh-"+id, id, string(data)); err != nil {
			return err
		}
	}
	for _, p := range profiles {
		v := clean(p.value)
		gateway := ""
		if id := jump(p.value); id != "" {
			gateway = "ssh-" + id
			v["gateway"] = gateway
		}
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO profiles VALUES(?,?,?,?,?,?)",
			p.id, p.group, p.position, nullable(p.key), nullable(gateway), string(data))
		if err != nil {
			return err
		}
	}
	original := map[string]map[string]any{}
	keys := map[string]string{}
	rows, err = tx.QueryContext(ctx, "SELECT id,coalesce(key_id,''),value FROM endpoints")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, key string
		var raw []byte
		if err := rows.Scan(&id, &key, &raw); err != nil {
			rows.Close()
			return err
		}
		if id != EndpointID(raw) {
			rows.Close()
			return errors.New("gateway migration: immutable endpoint digest mismatch")
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			rows.Close()
			return err
		}
		keyID, _ := v["keyId"].(string)
		if keyID != key {
			rows.Close()
			return errors.New("gateway migration: snapshot key reference mismatch")
		}
		original[id], keys[id] = v, key
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	converted := map[string]string{}
	writeEndpoint := func(old string, hops []any) (string, error) {
		v, exists := original[old]
		if !exists {
			return "", fmt.Errorf("gateway migration: missing endpoint %s", old)
		}
		value := clean(v)
		value["gatewayRoute"] = hops
		if id := jump(v); id != "" {
			value["gateway"] = "ssh-" + id
		}
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		id := EndpointID(data)
		_, err = tx.ExecContext(ctx, "INSERT INTO endpoints VALUES(?,?,?) ON CONFLICT DO NOTHING", id, nullable(keys[old]), string(data))
		if err == nil {
			err = saveEndpointKeys(ctx, tx, id, data)
		}
		return id, err
	}
	// Transfer connections own their captured steps, which can differ from today's catalog.
	connections := map[int]map[string]any{}
	rows, err = tx.QueryContext(ctx, "SELECT id,value FROM transfer_connections")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			rows.Close()
			return err
		}
		connections[id] = v
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	captured := map[string][]any{}
	connectionRoutes := map[int][]any{}
	for id, v := range connections {
		old, ok := v["endpointRef"].(string)
		if !ok {
			return errors.New("gateway migration: invalid connection endpoint reference")
		}
		refs, _ := v["stepRefs"].([]any)
		if len(refs) == 0 {
			continue
		}
		hops := []any{}
		for index, ref := range refs {
			stepID, ok := ref.(string)
			if !ok {
				return errors.New("gateway migration: invalid captured SSH step reference")
			}
			value, exists := original[stepID]
			if !exists {
				return errors.New("gateway migration: missing captured SSH step")
			}
			captured[stepID] = append([]any{}, hops...)
			if index < len(refs)-1 {
				hops = append(hops, hop(value))
			}
		}
		captured[old], connectionRoutes[id] = hops, hops
	}
	for id, value := range original {
		hops, err := route(value)
		if err != nil {
			var exists bool
			hops, exists = captured[id]
			if !exists {
				return err
			}
		}
		converted[id], err = writeEndpoint(id, hops)
		if err != nil {
			return err
		}
	}
	for id, v := range connections {
		if refs, ok := v["stepRefs"].([]any); ok && len(refs) > 0 {
			hops := connectionRoutes[id]
			old, _ := v["endpointRef"].(string)
			endpoint, err := writeEndpoint(old, hops)
			if err != nil {
				return err
			}
			v["endpointRef"] = endpoint
			// Keep each captured SSH step as a direct snapshot; the complete route is on the target.
			steps := []any{}
			for _, ref := range refs {
				step, err := writeEndpoint(ref.(string), []any{})
				if err != nil {
					return err
				}
				steps = append(steps, step)
			}
			v["stepRefs"] = steps
		} else if old, ok := v["endpointRef"].(string); ok {
			v["endpointRef"] = converted[old]
			hops, err := route(original[old])
			if err != nil {
				return err
			}
			connectionRoutes[id] = hops
		}
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE transfer_connections SET value=? WHERE id=?", string(data), id); err != nil {
			return err
		}
	}
	var rewrite func(any) error
	rewrite = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			for k, child := range v {
				if k == "endpointRef" {
					old, ok := child.(string)
					if !ok || converted[old] == "" {
						return errors.New("gateway migration: unknown endpoint reference")
					}
					v[k] = converted[old]
					continue
				}
				if err := rewrite(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := rewrite(child); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for _, table := range []string{"workspace", "transfer_sessions"} {
		column := "value"
		if table == "workspace" {
			column = "layout"
		}
		values := map[int]any{}
		rows, err := tx.QueryContext(ctx, "SELECT id,"+column+" FROM "+table)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int
			var raw []byte
			if err := rows.Scan(&id, &raw); err != nil {
				rows.Close()
				return err
			}
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				rows.Close()
				return err
			}
			var session map[string]any
			var capturedEndpoint string
			if table == "transfer_sessions" {
				object, ok := value.(map[string]any)
				if !ok {
					rows.Close()
					return errors.New("gateway migration: invalid transfer session")
				}
				session, ok = object["session"].(map[string]any)
				if !ok {
					rows.Close()
					return errors.New("gateway migration: invalid transfer endpoint")
				}
				old, _ := session["endpointRef"].(string)
				connection, _ := session["connection"].(float64)
				if hops, exists := connectionRoutes[int(connection)]; exists {
					capturedEndpoint, err = writeEndpoint(old, hops)
					if err != nil {
						rows.Close()
						return err
					}
				}
			}
			if err := rewrite(value); err != nil {
				rows.Close()
				return err
			}
			if capturedEndpoint != "" {
				session["endpointRef"] = capturedEndpoint
			}
			values[id] = value
		}
		if err := closeRows(rows); err != nil {
			return err
		}
		for id, value := range values {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET "+column+"=? WHERE id=?", string(data), id); err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"workspace_endpoints", "transfer_endpoints"} {
		// References are rebuilt from the rewritten owner rows, including transfer-specific routes.
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
		owners := []struct{ table, column string }{{"workspace", "layout"}}
		if table == "transfer_endpoints" {
			owners = []struct{ table, column string }{{"transfer_sessions", "value"}, {"transfer_connections", "value"}}
		}
		for _, owner := range owners {
			query := "INSERT INTO " + table + ` SELECT DISTINCT j.value FROM ` + owner.table +
				`, json_tree(` + owner.table + `.` + owner.column + `) j
				WHERE j.key='endpointRef' OR (j.type='text' AND j.path LIKE '%.stepRefs')
				ON CONFLICT DO NOTHING`
			_, err := tx.ExecContext(ctx, query)
			if err != nil {
				return err
			}
		}
	}
	if err := collectEndpoints(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "PRAGMA user_version=5")
	return err
}
