package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func hostIdentity(address string, remote net.Addr) (string, int, error) {
	if address == "" {
		address = remote.String()
	}
	host, service, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(service)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid SSH port %q", service)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.Unmap().String()
	} else {
		host = strings.ToLower(strings.TrimSuffix(host, "."))
	}
	if host == "" || strings.ContainsAny(host, "\x00\r\n") {
		return "", 0, errors.New("invalid SSH host")
	}
	return host, port, nil
}

func checkHostKey(ctx context.Context, tx *sql.Tx, host string, port int, key ssh.PublicKey) error {
	rows, err := tx.QueryContext(ctx, "SELECT host,public_key,revoked FROM host_keys WHERE (host=? AND port=?) OR (public_key=? AND revoked=1)", host, port, key.Marshal())
	if err != nil {
		return err
	}
	defer rows.Close()
	mismatch := &knownhosts.KeyError{}
	matched := false
	for rows.Next() {
		var name string
		var data []byte
		var revoked bool
		if err := rows.Scan(&name, &data, &revoked); err != nil {
			return err
		}
		stored, err := ssh.ParsePublicKey(data)
		if err != nil {
			return fmt.Errorf("invalid stored SSH host key: %w", err)
		}
		known := knownhosts.KnownKey{Key: stored, Filename: "Burrow SQLite", Line: 0}
		if revoked && bytes.Equal(data, key.Marshal()) {
			return &knownhosts.RevokedError{Revoked: known}
		}
		if !revoked {
			mismatch.Want = append(mismatch.Want, known)
			matched = matched || bytes.Equal(data, key.Marshal())
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if matched {
		return nil
	}
	return mismatch
}

func (s *Store) CheckHostKey(ctx context.Context, address string, remote net.Addr, key ssh.PublicKey) error {
	host, port, err := hostIdentity(address, remote)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return checkHostKey(ctx, tx, host, port, key)
}

// TrustHostKey rechecks inside the write transaction after the UI confirmation.
// A different key accepted by another connection while the dialog was open
// must fail, never replace or add to that host's trust automatically.
func (s *Store) TrustHostKey(ctx context.Context, address string, remote net.Addr, key ssh.PublicKey) error {
	host, port, err := hostIdentity(address, remote)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = checkHostKey(ctx, tx, host, port, key)
	if err == nil {
		return tx.Commit()
	}
	var mismatch *knownhosts.KeyError
	if !errors.As(err, &mismatch) || len(mismatch.Want) > 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO host_keys(host,port,algorithm,public_key,created_at) VALUES(?,?,?,?,?)", host, port, key.Type(), key.Marshal(), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RevokeHostKey(ctx context.Context, key ssh.PublicKey) error {
	_, err := s.db.ExecContext(ctx, "UPDATE host_keys SET revoked=1 WHERE public_key=?", key.Marshal())
	return err
}
