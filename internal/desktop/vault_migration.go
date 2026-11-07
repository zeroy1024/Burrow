package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"golang.org/x/crypto/ssh"
	"sshdesk/internal/locales"
	"sshdesk/internal/localfs"
	"sshdesk/internal/vault"
)

type migrationPassphraseError struct{ ID, Name string }

func (e *migrationPassphraseError) Error() string {
	return locales.MessageMigratingPrivateKey() + e.Name + locales.MessageRequiresItsOriginalPassphrase()
}

type migrationCredentialError struct{ ID, Name string }

func (e *migrationCredentialError) Error() string {
	return locales.MessageMigratingSession() + e.Name + locales.MessageRequiresTheMissingSavedPasswordToBeEnteredAgain()
}

func (a *app) migrateVault(ctx context.Context, temporary map[string]string) error {
	catalog, err := a.store.ReadCatalog(ctx)
	if err != nil {
		return err
	}
	profiles, err := a.store.LegacyCredentialProfiles(ctx)
	if err != nil {
		return err
	}
	source := legacyCredentialStore()
	items := []vault.PlainItem{}
	defer func() {
		for _, item := range items {
			clear(item.Data)
		}
	}()
	latest := map[string]storedSession{}
	for _, profile := range catalog.Profiles {
		var endpoint storedSession
		if err := json.Unmarshal(profile.Value, &endpoint); err != nil {
			return err
		}
		latest[endpoint.ID] = endpoint
	}
	seen := map[string]bool{}
	for _, data := range profiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		var endpoint storedSession
		if err := json.Unmarshal(data, &endpoint); err != nil {
			return err
		}
		if !endpoint.PasswordSaved || seen[endpoint.ID] {
			continue
		}
		if current, present := latest[endpoint.ID]; present && current.PasswordSaved {
			endpoint = current
		}
		id := sessionCredentialKey(endpoint.ID)
		var value []byte
		if explicit, present := temporary[id]; present {
			if explicit == "" || endpoint.PasswordRevision == "" {
				return locales.NewError(locales.MessageTheReEnteredPasswordOrPreviousCredentialVersionIsInvalid)
			}
			value, err = json.Marshal(sessionCredential{Revision: endpoint.PasswordRevision, Secret: explicit})
		} else {
			value, err = source.Get(id)
		}
		if errors.Is(err, mygo.ErrSecretNotFound) {
			return &migrationCredentialError{ID: id, Name: endpoint.Name}
		}
		if err != nil {
			return locales.Errorf(locales.MessageMigratePasswordForSession, endpoint.Name, err)
		}
		var credential sessionCredential
		if err := json.Unmarshal(value, &credential); err != nil || credential.Revision == "" || credential.Secret == "" {
			clear(value)
			return locales.NewError(locales.MessageInvalidLegacySessionCredentialFormat)
		}
		// The saved latest revision remains authoritative. Immutable stale snapshots
		// continue to fail their normal revision comparison after migration.
		items = append(items, vault.PlainItem{ID: sessionCredentialKey(endpoint.ID), Kind: "session_password", Revision: credential.Revision, Data: value})
		seen[endpoint.ID] = true
	}
	keys := map[string]string{}
	for _, key := range catalog.Keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		expected := filepath.Join(a.dataDirectory, "keys", key.ID)
		if !localfs.ValidName(key.ID) || filepath.Clean(key.SecretID) != expected {
			return locales.NewError(locales.MessageInvalidLegacyManagedPrivateKeyPath)
		}
		info, err := os.Lstat(expected)
		if err != nil {
			return locales.Errorf(locales.MessageMigratePrivateKey, key.Name, err)
		}
		if !info.Mode().IsRegular() {
			return locales.NewError(locales.MessageALegacyManagedPrivateKeyMustBeARegularFile)
		}
		if err := localfs.CheckPrivate(expected, info); err != nil {
			return err
		}
		data, err := os.ReadFile(expected)
		if err != nil {
			return err
		}
		id := privateCredentialKey(key.ID)
		items = append(items, vault.PlainItem{ID: id, Kind: "private_key", Revision: key.ID, Data: data})
		phrase, err := source.Get("key/" + key.ID + "/passphrase")
		saved := err == nil
		if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
			return err
		}
		password := phrase
		if explicit, present := temporary[key.ID]; present {
			password = []byte(explicit)
			defer clear(password)
		}
		signer, parseErr := ssh.ParsePrivateKey(data)
		var missing *ssh.PassphraseMissingError
		if errors.As(parseErr, &missing) {
			if len(password) == 0 {
				return &migrationPassphraseError{ID: key.ID, Name: key.Name}
			}
			signer, parseErr = ssh.ParsePrivateKeyWithPassphrase(data, password)
			if parseErr != nil {
				clear(phrase)
				return &migrationPassphraseError{ID: key.ID, Name: key.Name}
			}
		}
		if saved {
			savedPhrase := bytes.Clone(password)
			clear(phrase)
			items = append(items, vault.PlainItem{ID: "key/" + key.ID + "/passphrase", Kind: "key_passphrase", Revision: key.ID, Data: savedPhrase})
		}
		if parseErr != nil {
			return locales.Errorf(locales.MessageVerifyPrivateKey, key.Name, parseErr)
		}
		public, _, _, _, err := ssh.ParseAuthorizedKey(key.PublicKey)
		if err != nil {
			return err
		}
		if !bytes.Equal(public.Marshal(), signer.PublicKey().Marshal()) || ssh.FingerprintSHA256(public) != key.Fingerprint {
			return locales.NewError(locales.MessageTheMigratedPublicKeyDoesNotMatchTheSavedFingerprint)
		}
		keys[key.ID] = id
	}
	if err := a.vault.Initialize(ctx, items, keys); err != nil {
		return err
	}
	return nil
}
