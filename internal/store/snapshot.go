package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// OpenSnapshot reads a standalone database without initializing, migrating or
// opening a WAL. The caller must keep the file unchanged until Close.
func OpenSnapshot(ctx context.Context, name string) (*Store, error) {
	db, err := sql.Open("sqlite", sqliteURI(name)+"?mode=ro&immutable=1&_pragma=trusted_schema(0)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version, application int
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	if err == nil {
		err = db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&application)
	}
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if version != schemaVersion || application != applicationID {
		return nil, errors.Join(fmt.Errorf("invalid Burrow snapshot schema %d (application %x)", version, application), db.Close())
	}
	return &Store{db: db}, nil
}

func (s *Store) ValidateSnapshot(ctx context.Context, snapshot *Store) error {
	want, err := s.schemaObjects(ctx)
	if err != nil {
		return err
	}
	got, err := snapshot.schemaObjects(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(want, got) {
		return errors.New("Burrow snapshot schema does not match the current database")
	}
	var integrity string
	if err := snapshot.db.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("Burrow snapshot failed the SQLite integrity check")
	}
	rows, err := snapshot.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("Burrow snapshot contains invalid foreign keys")
	}
	return rows.Err()
}

func (s *Store) schemaObjects(ctx context.Context) ([][4]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT type, name, tbl_name, sql FROM sqlite_schema
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := [][4]string{}
	for rows.Next() {
		var object [4]string
		if err := rows.Scan(&object[0], &object[1], &object[2], &object[3]); err != nil {
			return nil, err
		}
		objects = append(objects, object)
	}
	return objects, rows.Err()
}
