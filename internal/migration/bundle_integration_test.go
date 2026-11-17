//go:build integration

package migration

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/vault"
)

// All values in these fixtures are disposable and are never used by a service.
const (
	testVersion      = "1.12.0"
	testBundlePhrase = "migration-fixture-package"
	testUnlockPhrase = "migration-fixture-unlock"
	testCredentialID = "session/migration-test/credential"
)

func TestMigrationIntegrationRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password bool
	}{
		{name: "automatic"},
		{name: "password", password: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, target := t.TempDir(), t.TempDir()
			seedMigrationData(t, source, "source", tc.password)
			seedMigrationData(t, target, "target", false)
			bundle := exportMigrationData(t, source, tc.password)
			raw, err := os.ReadFile(bundle)
			requireMigrationOK(t, err)
			if bytes.Contains(raw, []byte(`"fixture":"source"`)) {
				t.Fatal("export contains plaintext settings")
			}

			stage := stageMigrationData(t, target, bundle, tc.password)
			assertMigrationData(t, target, "target", false)
			requireMigrationOK(t, Schedule(target, stage))
			restore, err := Apply(target, testVersion)
			requireMigrationOK(t, err)
			backup, err := restore.Commit()
			requireMigrationOK(t, err)

			assertMigrationData(t, target, "source", tc.password)
			assertMigrationData(t, backup, "target", false)
			recovered, err := Recover(target)
			requireMigrationOK(t, err)
			if recovered {
				t.Fatal("a committed migration was rolled back")
			}
		})
	}
}

func TestMigrationIntegrationRejectsInvalidPackages(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	seedMigrationData(t, source, "source", false)
	seedMigrationData(t, target, "target", false)
	bundle := exportMigrationData(t, source, false)
	for _, tc := range []struct {
		name, current string
		upgrade       bool
	}{
		{name: "older receiver", current: "1.9.9", upgrade: true},
		{name: "preview receiver", current: "1.12.0-rc.1", upgrade: true},
		{name: "newer receiver", current: "1.13.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckVersion(bundle, tc.current)
			var mismatch *VersionMismatch
			if !errors.As(err, &mismatch) || mismatch.Upgrade != tc.upgrade ||
				mismatch.Current != tc.current || mismatch.Exported != testVersion {
				t.Fatalf("incorrect mismatch for %s: %v", tc.current, err)
			}
		})
	}
	raw, err := os.ReadFile(bundle)
	requireMigrationOK(t, err)
	damaged := filepath.Join(t.TempDir(), "truncated")
	requireMigrationOK(t, os.WriteFile(damaged, raw[:len(raw)-1], 0600))
	for _, tc := range []struct {
		name, path, password string
	}{
		{name: "wrong package password", path: bundle, password: "fixture-wrong"},
		{name: "damaged ciphertext", path: damaged, password: testBundlePhrase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, err := Extract(t.Context(), ImportOptions{
				Directory: target, Source: tc.path, Version: testVersion, Password: tc.password,
			})
			if err == nil || stage != "" {
				t.Fatal("invalid package was extracted")
			}
			if _, err := os.Stat(filepath.Join(target, stageName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed import left a staging directory: %v", err)
			}
			assertMigrationData(t, target, "target", false)
		})
	}
}

func TestMigrationIntegrationRollback(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	seedMigrationData(t, source, "source", false)
	seedMigrationData(t, target, "target", false)
	stage := stageMigrationData(t, target, exportMigrationData(t, source, false), false)
	requireMigrationOK(t, Schedule(target, stage))
	_, err := Apply(target, testVersion)
	requireMigrationOK(t, err)
	assertMigrationData(t, target, "source", false)

	recovered, err := Recover(target)
	requireMigrationOK(t, err)
	if !recovered {
		t.Fatal("the interrupted switch did not roll back")
	}
	assertMigrationData(t, target, "target", false)
	recovered, err = Recover(target)
	requireMigrationOK(t, err)
	if recovered {
		t.Fatal("the rollback left a pending journal")
	}
}

func seedMigrationData(t *testing.T, directory, value string, password bool) {
	t.Helper()
	db, manager := openMigrationData(t, directory)
	defer func() { requireMigrationOK(t, db.Close()) }()
	defer manager.Close()
	ctx := t.Context()
	requireMigrationOK(t, manager.Initialize(ctx, []vault.PlainItem{{
		ID: testCredentialID, Kind: "session_password", Revision: "fixture", Data: []byte(value),
	}}, nil))
	if password {
		_, err := manager.ChangeProtection(ctx, "", testUnlockPhrase, true)
		requireMigrationOK(t, err)
	}
	requireMigrationOK(t, db.SaveSetting(ctx, "migration-test", []byte(`{"fixture":"`+value+`"}`)))
	updater := []byte(`{"automaticChecks":false,"skippedVersion":"` + value + `"}`)
	requireMigrationOK(t, os.WriteFile(filepath.Join(directory, updateFile), updater, 0600))
}

func openMigrationData(t *testing.T, directory string) (*store.Store, *vault.Manager) {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(directory, databaseFile))
	requireMigrationOK(t, err)
	manager, err := vault.OpenWithKeyStore(t.Context(), db, directory, nil)
	if err != nil {
		t.Fatal(errors.Join(err, db.Close()))
	}
	return db, manager
}

func exportMigrationData(t *testing.T, directory string, password bool) string {
	t.Helper()
	db, manager := openMigrationData(t, directory)
	defer func() { requireMigrationOK(t, db.Close()) }()
	defer manager.Close()
	ctx := t.Context()
	if password {
		requireMigrationOK(t, manager.Unlock(ctx, []byte(testUnlockPhrase)))
	}
	bundle := filepath.Join(t.TempDir(), "test."+Extension)
	requireMigrationOK(t, Export(ctx, manager, ExportOptions{
		Directory: directory, Destination: bundle, Version: testVersion, Password: testBundlePhrase,
	}))
	return bundle
}

func stageMigrationData(t *testing.T, directory, bundle string, password bool) string {
	t.Helper()
	ctx := t.Context()
	stage, err := Extract(ctx, ImportOptions{
		Directory: directory, Source: bundle, Version: testVersion, Password: testBundlePhrase,
	})
	requireMigrationOK(t, err)
	live, err := store.Open(ctx, filepath.Join(directory, databaseFile))
	requireMigrationOK(t, err)
	defer func() { requireMigrationOK(t, live.Close()) }()
	requiresPassword, err := Preflight(ctx, live, stage, func(*store.Store) error { return nil })
	requireMigrationOK(t, err)
	if requiresPassword != password {
		t.Fatal("preflight did not preserve the source unlock mode")
	}
	if password {
		if err := VerifyPassword(ctx, stage, []byte("fixture-wrong")); !errors.Is(err, vault.ErrUnlock) {
			t.Fatalf("wrong unlock password returned %v", err)
		}
		requireMigrationOK(t, VerifyPassword(ctx, stage, []byte(testUnlockPhrase)))
	}
	return stage
}

func assertMigrationData(t *testing.T, directory, value string, password bool) {
	t.Helper()
	db, manager := openMigrationData(t, directory)
	defer func() { requireMigrationOK(t, db.Close()) }()
	defer manager.Close()
	ctx := t.Context()
	var setting []byte
	found, err := db.ReadSetting(ctx, "migration-test", &setting)
	requireMigrationOK(t, err)
	if !found || string(setting) != `{"fixture":"`+value+`"}` {
		t.Fatal("the SQLite setting did not match the expected device")
	}
	if manager.RequiresPassword() != password {
		t.Fatal("the vault unlock mode changed")
	}
	if password {
		requireMigrationOK(t, manager.Unlock(ctx, []byte(testUnlockPhrase)))
	}
	credential, err := manager.Get(ctx, testCredentialID)
	requireMigrationOK(t, err)
	if string(credential) != value {
		t.Fatal("the saved credential did not match the expected device")
	}
	updater, err := os.ReadFile(filepath.Join(directory, updateFile))
	requireMigrationOK(t, err)
	if string(updater) != `{"automaticChecks":false,"skippedVersion":"`+value+`"}` {
		t.Fatal("the update settings did not match the expected device")
	}
}

func requireMigrationOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
