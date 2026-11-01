package desktop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func catalogAppForTest(path string) *app {
	return &app{sshCatalog: demoCatalog(), fileCatalog: demoCatalog(), catalogConfig: catalogConfigState{path: path}}
}

func writeCatalogFixture(t *testing.T, path string, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func legacyCatalogFixture() any {
	return struct {
		Groups []catalogConfigGroup `json:"groups"`
		Hosts  []catalogConfigHost  `json:"hosts"`
	}{
		Groups: []catalogConfigGroup{{ID: 0, Name: "servers", Order: 0, Collapsed: true, FilesCollapsed: false}},
		Hosts: []catalogConfigHost{
			{ID: 0, GroupID: 0, Order: 0, Name: "first", User: "ops", Address: "localhost", Port: 22},
			{ID: 1, GroupID: 0, Order: 1, Name: "second", User: "ops", Address: "localhost", Port: 22},
		},
	}
}

func TestCatalogLegacyMigrationIsIndependentAndPreservesPreferences(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "catalog.json")
	legacy := writeCatalogFixture(t, path, legacyCatalogFixture())
	prefsPath := filepath.Join(directory, "files.json")
	prefs := writeCatalogFixture(t, prefsPath, filePreferences{
		LocalRatio: 0.5, LocalPaths: map[string]string{"ops@localhost:22": "~/Projects"},
	})
	a := catalogAppForTest(path)
	if err := a.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	if !a.catalogConfig.migrated {
		t.Fatal("successful migration did not provide a user-facing completion signal")
	}
	if !a.sshCatalog.groups[0].collapsed || a.fileCatalog.groups[0].collapsed {
		t.Fatal("migration lost page-specific group collapse state")
	}
	if a.fileCatalog.localPaths[0] != "~/Projects" || a.fileCatalog.localPaths[1] != "~/Projects" {
		t.Fatal("migration did not preserve the legacy address-keyed preference for both sites")
	}
	for _, fixture := range []struct {
		path string
		data []byte
	}{{path + ".v1.bak", legacy}, {prefsPath + ".v1.bak", prefs}, {prefsPath, prefs}} {
		got, err := os.ReadFile(fixture.path)
		if err != nil || !bytes.Equal(got, fixture.data) {
			t.Fatalf("migration changed protected original data at %s", fixture.path)
		}
	}
	a.section = sectionSSH
	if err := a.commitCatalogMove(func() { a.sshCatalog.removeHost(0) }); err != nil {
		t.Fatal(err)
	}
	if _, exists := a.fileCatalog.hosts[0]; !exists {
		t.Fatal("removing an SSH profile removed the independent file site")
	}
	if a.sshCatalog.groups[0] == a.fileCatalog.groups[0] {
		t.Fatal("migrated catalogs share mutable groups")
	}
	changed := a.fileCatalog.hosts[0]
	changed.address = "renamed.example"
	a.fileCatalog.hosts[0] = changed
	a.fileCatalog.groups[0].collapsed = true
	if err := a.saveCatalog(); err != nil {
		t.Fatal(err)
	}
	restarted := catalogAppForTest(path)
	if err := restarted.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	if restarted.catalogConfig.migrated {
		t.Fatal("reading v2 reported a repeated migration")
	}
	if _, reimported := restarted.sshCatalog.hosts[0]; reimported {
		t.Fatal("reading v2 reimported the deleted SSH profile")
	}
	if restarted.sshCatalog.hosts[1].name != "second" || restarted.sshCatalog.nextHostID != 2 {
		t.Fatal("deleting a profile changed a surviving ID or reused the next ID")
	}
	file := &fileSession{host: 0, endpoint: restarted.fileCatalog.hosts[0], local: newFilePane(false)}
	restarted.restoreFileSession(file)
	if file.local.path != "~/Projects" {
		t.Fatal("changing a site's address lost its ID-keyed local path")
	}
	temporary := &fileSession{host: 0, temporary: true, local: newFilePane(false)}
	restarted.restoreFileSession(temporary)
	if temporary.local.path != "~" {
		t.Fatal("temporary file view inherited a saved site's preference")
	}
}

func TestCatalogFailedSaveRollsBackOnlyTheEditedCatalog(t *testing.T) {
	// Replacing a directory with the pending file fails on every supported platform.
	a := catalogAppForTest(t.TempDir())
	original := cloneCatalog(a.sshCatalog)
	other := cloneCatalog(a.fileCatalog)
	group := a.sshCatalog.groups[0]
	err := a.commitCatalogMove(func() {
		a.sshCatalog.removeHost(0)
		a.sshCatalog.groups[0].name = "changed"
		a.sshCatalog.nextHostID++
	})
	if err == nil {
		t.Fatal("saving over a directory succeeded")
	}
	if !reflect.DeepEqual(a.sshCatalog, original) || a.sshCatalog.groups[0] != group {
		t.Fatal("failed save did not restore data and existing group identity")
	}
	if !reflect.DeepEqual(a.fileCatalog, other) {
		t.Fatal("failed SSH edit changed the file catalog")
	}
}

func TestCatalogDeletingHighestIDDoesNotReuseItAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	a := catalogAppForTest(path)
	deleted := a.sshCatalog.nextHostID - 1
	if err := a.commitCatalogMove(func() { a.sshCatalog.removeHost(deleted) }); err != nil {
		t.Fatal(err)
	}
	restarted := catalogAppForTest(path)
	if err := restarted.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	if restarted.sshCatalog.nextHostID <= deleted {
		t.Fatal("the next profile would reuse a deleted saved ID after restart")
	}
	if _, exists := restarted.fileCatalog.hosts[deleted]; !exists {
		t.Fatal("deleting the highest SSH ID changed the independent file namespace")
	}
}

func TestCatalogV2RejectsInvalidIdentityWithoutOverwriting(t *testing.T) {
	for _, name := range []string{"unknown_version", "duplicate_id", "reused_next_id", "unknown_site_preference"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "catalog.json")
			config := catalogConfig{Version: 2, SSH: demoCatalog().configuration(), Files: demoCatalog().configuration()}
			switch name {
			case "unknown_version":
				config.Version = 3
			case "duplicate_id":
				config.SSH.Hosts[1].ID = config.SSH.Hosts[0].ID
			case "reused_next_id":
				config.SSH.NextHostID = 0
			case "unknown_site_preference":
				config.Files.LocalPaths[1000] = "~/unrelated"
			}
			original := writeCatalogFixture(t, path, config)
			a := catalogAppForTest(path)
			if err := a.readCatalogFile(); err == nil {
				t.Fatal("invalid catalog was accepted")
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("rejecting invalid configuration overwrote the original")
			}
		})
	}
}

func TestCatalogMigrationFailurePreservesOriginals(t *testing.T) {
	for _, name := range []string{"malformed_preferences", "existing_different_backup"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "catalog.json")
			original := writeCatalogFixture(t, path, legacyCatalogFixture())
			if name == "malformed_preferences" {
				if err := os.WriteFile(filepath.Join(directory, "files.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path+".v1.bak", []byte("previous backup"), 0o600); err != nil {
				t.Fatal(err)
			}
			a := catalogAppForTest(path)
			if err := a.readCatalogFile(); err == nil {
				t.Fatal("migration unexpectedly succeeded")
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("failed migration overwrote the original catalog")
			}
		})
	}
}
