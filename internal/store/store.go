// Package store owns Burrow's local database. Callers pass immutable values;
// filesystem, keychain and network operations stay outside its transactions.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
	"sshdesk/internal/localfs"
)

//go:embed schema.sql
var schema string

//go:embed mcp_schema.sql
var mcpSchema string

//go:embed mcp_payload_schema.sql
var mcpPayloadSchema string

//go:embed vault_schema.sql
var vaultSchema string

type Store struct {
	db   *sql.DB
	lock *os.File
}

func Open(ctx context.Context, name string) (*Store, error) {
	if err := localfs.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return nil, err
	}
	// A second app instance has stale in-memory catalog/task state even when
	// SQLite serializes its SQL. Keep one owner per data directory, including
	// plain go run and dev bundles sharing the same directory.
	lock, err := localfs.OpenFile(name+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockDatabase(lock); err != nil {
		lock.Close()
		return nil, fmt.Errorf("lock Burrow database (another instance may be running): %w", err)
	}
	opened := false
	defer func() {
		if !opened {
			lock.Close()
		}
	}()
	file, err := localfs.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(localfs.Protect(file), file.Close()); err != nil {
		return nil, err
	}
	// URI escaping prevents a filename containing ? or # from becoming options.
	path := filepath.ToSlash(name)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	s := &Store{db: db, lock: lock}
	var originalVersion, originalApplication int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&originalVersion); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&originalApplication); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if originalVersion >= 1 && originalVersion <= 3 && originalApplication == 0x42525731 {
		backup := name + ".before-vault-" + time.Now().UTC().Format("20060102T150405.000000000") + ".bak"
		if err := s.Backup(ctx, backup); err != nil {
			return nil, errors.Join(fmt.Errorf("backup before vault upgrade: %w", err), db.Close())
		}
	}
	if err := s.initialize(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	opened = true
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version, application int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&application); err != nil {
		return err
	}
	const applicationID = 0x42525731 // BRW1
	fresh := version == 0 && application == 0
	if version == 0 && application == 0 {
		var tables int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return errors.New("database is not an empty Burrow database")
		}
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("create database schema: %w", err)
		}
		version = 1
	} else if (version < 1 || version > 4) || application != applicationID {
		return fmt.Errorf("unsupported Burrow database schema %d (application %x)", version, application)
	}
	if version == 1 {
		if _, err := tx.ExecContext(ctx, mcpSchema); err != nil {
			return fmt.Errorf("upgrade database schema to version 2: %w", err)
		}
		version = 2
	}
	if version == 2 {
		if _, err := tx.ExecContext(ctx, mcpPayloadSchema); err != nil {
			return fmt.Errorf("upgrade database schema to version 3: %w", err)
		}
		version = 3
	}
	if version == 3 {
		if _, err := tx.ExecContext(ctx, vaultSchema); err != nil {
			return fmt.Errorf("upgrade database schema to version 4: %w", err)
		}
		state := "pending_migration"
		if fresh {
			state = "pending_init"
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO vault_meta VALUES(1,?,?)", state, "{}"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return errors.Join(s.db.Close(), s.lock.Close()) }

// Backup creates a consistent standalone database, including committed WAL
// contents. SQLite refuses to overwrite a nonempty destination.
func (s *Store) Backup(ctx context.Context, destination string) error {
	file, err := localfs.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err := errors.Join(localfs.Protect(file), file.Close()); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "VACUUM INTO ?", destination)
	return err
}

func (s *Store) ReadSetting(ctx context.Context, key string, value *[]byte) (bool, error) {
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) SaveSetting(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, string(value))
	return err
}
