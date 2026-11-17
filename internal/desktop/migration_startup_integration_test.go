//go:build integration

package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/zeroy1024/Burrow/internal/migration"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/vault"
)

func TestMigrationIntegrationStartupRollsBackUnopenableDatabase(t *testing.T) {
	ctx := t.Context()
	version := mygo.App.Version()
	mygo.App.SetVersion("1.2.3")
	t.Cleanup(func() { mygo.App.SetVersion(version) })

	requireOK := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	create := func(directory, marker string) (*store.Store, *vault.Manager) {
		t.Helper()
		db, err := store.Open(ctx, filepath.Join(directory, "burrow.db"))
		requireOK(err)
		manager, err := vault.OpenWithKeyStore(ctx, db, directory, nil)
		requireOK(err)
		requireOK(manager.Initialize(ctx, nil, nil))
		requireOK(db.SaveSetting(ctx, "migration-fixture", []byte(marker)))
		return db, manager
	}

	source, target := t.TempDir(), t.TempDir()
	archive := filepath.Join(t.TempDir(), "migration-fixture")
	const fixturePhrase = "{{SECRET_w60NxHok}}"
	sourceDB, sourceVault := create(source, `"source"`)
	requireOK(migration.Export(ctx, sourceVault, migration.ExportOptions{
		Directory: source, Destination: archive, Version: appVersion(), Password: fixturePhrase,
	}))
	sourceVault.Close()
	requireOK(sourceDB.Close())

	targetDB, targetVault := create(target, `"target"`)
	stage, err := migration.Extract(ctx, migration.ImportOptions{
		Directory: target, Source: archive, Version: appVersion(), Password: fixturePhrase,
	})
	requireOK(err)
	view := &app{ctx: ctx, store: targetDB}
	_, err = migration.Preflight(ctx, targetDB, stage, view.validateMigrationRecords)
	requireOK(err)

	// Corrupt only the validated staging file, as if it were damaged between
	// confirmation and restart. Startup must still open the receiving data.
	requireOK(os.WriteFile(filepath.Join(stage, "incoming", "burrow.db"), []byte("corrupt"), 0600))
	requireOK(migration.Schedule(target, stage))
	targetVault.Close()
	requireOK(targetDB.Close())

	data, err := openDesktopData(ctx, target)
	requireOK(err)
	defer func() { requireOK(data.db.Close()) }()
	requireOK(data.vaultErr)
	defer data.vault.Close()
	if !data.vault.Unlocked() || data.notice == nil || data.notice.ID != "migration-rolled-back" {
		t.Fatal("startup did not report recovery to the original unlockable vault")
	}
	var marker []byte
	found, err := data.db.ReadSetting(ctx, "migration-fixture", &marker)
	requireOK(err)
	if !found || string(marker) != `"target"` {
		t.Fatal("startup did not recover the original database")
	}
}
