//go:build !windows

package sshclient

import (
	"context"
	"errors"
	"net"
	"os"
	"time"
)

// DialAgent connects to the current account's SSH_AUTH_SOCK.
func DialAgent(ctx context.Context) (net.Conn, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, errors.New("SSH agent is unavailable: SSH_AUTH_SOCK is not set")
	}
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "unix", socket)
}
