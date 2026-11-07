//go:build !darwin

package desktop

import "github.com/zeroy1024/Burrow/internal/vault"

func platformUnlockKeys() vault.KeyStore { return nil }
