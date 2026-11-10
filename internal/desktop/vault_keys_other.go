//go:build !darwin

package desktop

import "github.com/zeroy1024/Burrow/internal/vault"

func legacyUnlockKeys() vault.KeyStore { return nil }
