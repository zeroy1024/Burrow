package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type VaultHeader struct {
	Version                 int
	ID, Mode, LocalKeyID    string
	KeyStorage              string
	Epoch                   int64
	Salt, Nonce, WrappedKey []byte
	Memory, Iterations      uint32
	Parallelism             uint8
}

type VaultItem struct {
	ID, Kind, Revision string
	Epoch              int64
	Nonce, Ciphertext  []byte
}

type VaultChanges struct {
	Epoch  int64
	Put    []VaultItem
	Delete []string
}

type VaultSnapshot struct {
	State  string
	Header VaultHeader
	Items  []VaultItem
}

func (s *Store) ReadVault(ctx context.Context) (VaultSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return VaultSnapshot{}, err
	}
	defer tx.Rollback()
	var result VaultSnapshot
	var data string
	if err := tx.QueryRowContext(ctx, "SELECT state,value FROM vault_meta WHERE id=1").Scan(&result.State, &data); err != nil {
		return result, fmt.Errorf("read vault header: %w", err)
	}
	if err := json.Unmarshal([]byte(data), &result.Header); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT secret_id,kind,revision,crypto_epoch,nonce,ciphertext FROM vault_items ORDER BY secret_id")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item VaultItem
		if err := rows.Scan(&item.ID, &item.Kind, &item.Revision, &item.Epoch, &item.Nonce, &item.Ciphertext); err != nil {
			rows.Close()
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	if err := closeRows(rows); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func (s *Store) ReadVaultItem(ctx context.Context, id string) (VaultItem, error) {
	var item VaultItem
	err := s.db.QueryRowContext(ctx,
		"SELECT secret_id,kind,revision,crypto_epoch,nonce,ciphertext FROM vault_items WHERE secret_id=?",
		id).Scan(&item.ID, &item.Kind, &item.Revision, &item.Epoch, &item.Nonce, &item.Ciphertext)
	return item, err
}

func checkVaultEpoch(ctx context.Context, tx *sql.Tx, epoch int64) error {
	var state, data string
	if err := tx.QueryRowContext(ctx, "SELECT state,value FROM vault_meta WHERE id=1").Scan(&state, &data); err != nil {
		return err
	}
	var header VaultHeader
	if err := json.Unmarshal([]byte(data), &header); err != nil {
		return err
	}
	if state != "ready" || header.Epoch != epoch {
		return errors.New("vault changed before commit")
	}
	return nil
}

func saveVaultChanges(ctx context.Context, tx *sql.Tx, changes *VaultChanges) error {
	if changes == nil {
		return nil
	}
	if err := checkVaultEpoch(ctx, tx, changes.Epoch); err != nil {
		return err
	}
	for _, item := range changes.Put {
		if item.Epoch != changes.Epoch {
			return errors.New("invalid vault item epoch")
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO vault_items VALUES(?,?,?,?,?,?) ON CONFLICT(secret_id) DO UPDATE SET kind=excluded.kind,revision=excluded.revision,crypto_epoch=excluded.crypto_epoch,nonce=excluded.nonce,ciphertext=excluded.ciphertext",
			item.ID, item.Kind, item.Revision, item.Epoch, item.Nonce, item.Ciphertext); err != nil {
			return err
		}
	}
	for _, id := range changes.Delete {
		if _, err := tx.ExecContext(ctx, "DELETE FROM vault_items WHERE secret_id=?", id); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceVault commits a complete new key generation. Migration updates private
// key references in this same transaction, after all external reads succeeded.
func (s *Store) ReplaceVault(ctx context.Context, previous VaultSnapshot, next VaultSnapshot, keys map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, data string
	if err := tx.QueryRowContext(ctx, "SELECT state,value FROM vault_meta WHERE id=1").Scan(&state, &data); err != nil {
		return err
	}
	var current VaultHeader
	if err := json.Unmarshal([]byte(data), &current); err != nil {
		return err
	}
	if state != previous.State || current.ID != previous.Header.ID || current.Epoch != previous.Header.Epoch {
		return errors.New("vault changed before replacement")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM vault_items"); err != nil {
		return err
	}
	for _, item := range next.Items {
		if item.Epoch != next.Header.Epoch {
			return errors.New("invalid replacement epoch")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO vault_items VALUES(?,?,?,?,?,?)",
			item.ID, item.Kind, item.Revision, item.Epoch, item.Nonce, item.Ciphertext); err != nil {
			return err
		}
	}
	for id, secretID := range keys {
		result, err := tx.ExecContext(ctx, "UPDATE managed_keys SET secret_id=? WHERE id=?", secretID, id)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("migration private key reference disappeared")
		}
	}
	encoded, err := json.Marshal(next.Header)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE vault_meta SET state='ready',value=? WHERE id=1", string(encoded)); err != nil {
		return err
	}
	return tx.Commit()
}

// LegacyCredentialProfiles includes immutable endpoints, whose originating
// catalog entry may already have been removed.
func (s *Store) LegacyCredentialProfiles(ctx context.Context) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT value FROM profiles UNION SELECT value FROM endpoints")
	if err != nil {
		return nil, err
	}
	values := [][]byte{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			rows.Close()
			return nil, err
		}
		values = append(values, data)
	}
	return values, closeRows(rows)
}
