package desktop

import (
	"github.com/egoist/mygo"
	"github.com/zeroy1024/Burrow/internal/vault"
)

func legacyUnlockKeys() vault.KeyStore {
	// Only old release vaults need this reader. File-backed and password vaults
	// never call it, and migrated Keychain items remain available to old backups.
	if !mygo.App.IsPackaged() || mygo.App.Name() != "Burrow" {
		return nil
	}
	return mygo.Secrets.Service("dev.burrow.app.vault-unlock")
}
