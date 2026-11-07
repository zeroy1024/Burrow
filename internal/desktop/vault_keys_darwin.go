package desktop

import (
	"github.com/egoist/mygo"
	"github.com/zeroy1024/Burrow/internal/vault"
)

func platformUnlockKeys() vault.KeyStore {
	// Development and integration bundles have changing signatures and isolated
	// data roots. Only stable-signed release bundles use the login Keychain.
	if !mygo.App.IsPackaged() || mygo.App.Name() != "Burrow" {
		return nil
	}
	return mygo.Secrets.Service("dev.burrow.app.vault-unlock")
}
