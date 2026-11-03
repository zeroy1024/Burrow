package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyStore separates SSH verification from the app's persistence backend.
// TrustHostKey must atomically recheck before persisting an unknown key.
type HostKeyStore interface {
	CheckHostKey(context.Context, string, net.Addr, ssh.PublicKey) error
	TrustHostKey(context.Context, string, net.Addr, ssh.PublicKey) error
}

// KnownHosts confirms only unknown hosts. Changed and revoked keys always fail;
// no transaction or database connection is held while confirm waits for the UI.
func KnownHosts(ctx context.Context, store HostKeyStore, confirm func(hostname, fingerprint, keyType string) bool) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := store.CheckHostKey(ctx, hostname, remote, key)
		var unknown *knownhosts.KeyError
		if err == nil || !errors.As(err, &unknown) || len(unknown.Want) > 0 {
			return err
		}
		if confirm == nil || !confirm(hostname, ssh.FingerprintSHA256(key), key.Type()) {
			return fmt.Errorf("SSH host key was not trusted for %s", hostname)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return store.TrustHostKey(ctx, hostname, remote, key)
	}
}
