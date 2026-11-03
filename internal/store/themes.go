package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

type Theme struct {
	Name, Filename string
	Source         []byte
}

func (s *Store) Themes(ctx context.Context) ([]Theme, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name,filename,source FROM themes ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var themes []Theme
	for rows.Next() {
		var t Theme
		if err := rows.Scan(&t.Name, &t.Filename, &t.Source); err != nil {
			return nil, err
		}
		themes = append(themes, t)
	}
	return themes, rows.Err()
}

func (s *Store) SaveTheme(ctx context.Context, t Theme) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT name,filename,source FROM themes WHERE name=? OR filename=?", t.Name, t.Filename)
	if err != nil {
		return err
	}
	for rows.Next() {
		var old Theme
		if err := rows.Scan(&old.Name, &old.Filename, &old.Source); err != nil {
			rows.Close()
			return err
		}
		if old.Name != t.Name {
			rows.Close()
			return fmt.Errorf("已存在同名配色文件 %q", t.Filename)
		}
		if old.Filename != t.Filename {
			rows.Close()
			return fmt.Errorf("配色名称 %q 已由文件 %q 使用", t.Name, old.Filename)
		}
		if !bytes.Equal(old.Source, t.Source) {
			rows.Close()
			return fmt.Errorf("已存在同名配色文件 %q，请重命名后导入", t.Filename)
		}
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO themes VALUES(?,?,?) ON CONFLICT(name) DO NOTHING", t.Name, t.Filename, t.Source); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteTheme(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM themes WHERE name=?", name)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("配色不存在")
	}
	return nil
}
