package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func authentication(ctx context.Context, cfg Config) (ssh.AuthMethod, net.Conn, error) {
	switch cfg.Auth {
	case "password":
		return ssh.Password(cfg.Password), nil, nil
	case "key":
		keyPath := cfg.KeyPath
		if strings.HasPrefix(keyPath, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, nil, fmt.Errorf("locate SSH key directory: %w", err)
			}
			keyPath = filepath.Join(home, keyPath[2:])
		}
		if keyPath == "" {
			return nil, nil, errors.New("SSH private key path is required")
		}
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read SSH private key: %w", err)
		}
		defer clear(data)
		var signer ssh.Signer
		if cfg.Passphrase == "" {
			signer, err = ssh.ParsePrivateKey(data)
		} else {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(cfg.Passphrase))
		}
		if err != nil {
			return nil, nil, fmt.Errorf("parse SSH private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil, nil
	case "agent":
		conn, err := DialAgent(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("connect SSH agent: %w", err)
		}
		if err := conn.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
			conn.Close()
			return nil, nil, fmt.Errorf("set SSH agent deadline: %w", err)
		}
		return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), conn, nil
	default:
		return nil, nil, fmt.Errorf("unsupported SSH authentication method %q", cfg.Auth)
	}
}
