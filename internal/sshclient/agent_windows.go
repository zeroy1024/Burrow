package sshclient

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	winio "github.com/Microsoft/go-winio"
)

// DialAgent uses a local named pipe. An explicit SSH_AUTH_SOCK overrides the
// Windows OpenSSH agent endpoint; remote pipes are never used for credentials.
func DialAgent(ctx context.Context) (net.Conn, error) {
	address := os.Getenv("SSH_AUTH_SOCK")
	if address == "" {
		address = `\\.\pipe\openssh-ssh-agent`
	}
	address = filepath.FromSlash(address)
	if !strings.HasPrefix(strings.ToLower(address), `\\.\pipe\`) {
		return nil, errors.New("Windows SSH_AUTH_SOCK must name a local named pipe")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return winio.DialPipeContext(ctx, address)
}
