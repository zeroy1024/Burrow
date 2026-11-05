//go:build !darwin

package desktop

import "sshdesk/internal/vault"

func platformUnlockKeys() vault.KeyStore { return nil }
