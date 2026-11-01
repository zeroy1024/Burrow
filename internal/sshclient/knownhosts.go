package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var knownHostsMu sync.Mutex

// KnownHosts verifies an app-owned OpenSSH known_hosts file. Only previously
// unknown hosts can be confirmed; changed and revoked keys always fail. confirm
// runs on the dialing goroutine and must stop waiting when its context ends.
func KnownHosts(path string, confirm func(hostname, fingerprint, keyType string) bool) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		knownHostsMu.Lock()
		err := checkHostKey(path, hostname, remote, key)
		knownHostsMu.Unlock()
		if err == nil || !unknownHost(err) {
			return err
		}
		if confirm == nil || !confirm(hostname, ssh.FingerprintSHA256(key), key.Type()) {
			return fmt.Errorf("SSH host key was not trusted for %s", hostname)
		}
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		// Another pane may have saved a key while this confirmation was open.
		if err := checkHostKey(path, hostname, remote, key); err == nil || !unknownHost(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create SSH host key directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("open SSH host key file: %w", err)
		}
		if err := file.Chmod(0o600); err != nil {
			file.Close()
			return fmt.Errorf("secure SSH host key file: %w", err)
		}
		// Prepending a newline also handles an existing final entry without one.
		_, writeErr := file.WriteString("\n" + knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n")
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("save SSH host key: %w", err)
		}
		return nil
	}
}

func unknownHost(err error) bool {
	var keyErr *knownhosts.KeyError
	return errors.As(err, &keyErr) && len(keyErr.Want) == 0
}

func checkHostKey(path, hostname string, remote net.Addr, key ssh.PublicKey) error {
	files := []string{path}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		files = nil
	} else if err != nil {
		return fmt.Errorf("read SSH host key file: %w", err)
	}
	check, err := knownhosts.New(files...)
	if err != nil {
		return fmt.Errorf("parse SSH host key file: %w", err)
	}
	return check(hostname, remote, key)
}
