package desktop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func catalogAppForTest(path string) *app {
	a := &app{hosts: demoHosts(), groups: demoGroups(), catalogConfig: catalogConfigState{path: path}}
	for i := range a.hosts {
		initializeSession(&a.hosts[i])
	}
	for _, group := range a.groups[:3] {
		for _, id := range group.hosts {
			h := a.hosts[id]
			h.protocol, h.session = "sftp", sessionConfig{Auth: "agent"}
			initializeSession(&h)
			fileID := hostID(len(a.hosts))
			a.hosts = append(a.hosts, h)
			a.groups[group.id+3].hosts = append(a.groups[group.id+3].hosts, fileID)
		}
	}
	a.files.localPaths = map[string]string{}
	return a
}

func catalogHostForTest(t *testing.T, a *app, name string, files bool) hostID {
	t.Helper()
	id := slices.IndexFunc(a.hosts, func(h host) bool { return h.name == name && (h.protocol == "sftp" || h.protocol == "ftp") == files })
	if id < 0 {
		t.Fatalf("missing profile %q (files=%v)", name, files)
	}
	return hostID(id)
}

func removeCatalogFixtureHost(a *app, id hostID) {
	a.hosts = slices.Delete(a.hosts, int(id), int(id)+1)
	for _, group := range a.groups {
		group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
		for i, candidate := range group.hosts {
			if candidate > id {
				group.hosts[i]--
			}
		}
	}
}

func legacyV2Fixture() catalogConfig {
	old := legacyCatalogFixture()
	data := legacyCatalogData{NextHostID: 2, NextGroupID: 1, Groups: old.Groups, Hosts: old.Hosts, LocalPaths: map[hostID]string{}}
	files := data
	files.Groups, files.Hosts = slices.Clone(data.Groups), slices.Clone(data.Hosts)
	files.LocalPaths = map[hostID]string{}
	return catalogConfig{Version: 2, SSH: data, Files: files}
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

func legacyCatalogFixture() catalogConfig {
	return catalogConfig{
		Groups: []catalogConfigGroup{{ID: 0, Name: "servers", Order: 0, Collapsed: true, FilesCollapsed: false}},
		Hosts: []catalogConfigHost{
			{ID: 0, GroupID: 0, Order: 0, Name: "first", User: "ops", Address: "localhost", Port: 22},
			{ID: 1, GroupID: 0, Order: 1, Name: "second", User: "ops", Address: "localhost", Port: 22},
		},
	}
}

func TestCatalogLegacyMigrationIsIndependentAndPreservesPreferences(t *testing.T) {
	directory := t.TempDir()
	legacyPath, path := filepath.Join(directory, "catalog.json"), filepath.Join(directory, "sessions.json")
	legacy := writeCatalogFixture(t, legacyPath, legacyCatalogFixture())
	prefsPath := filepath.Join(directory, "files.json")
	prefs := writeCatalogFixture(t, prefsPath, filePreferences{LocalRatio: 0.5, LocalPaths: map[string]string{"ops@localhost:22": "~/Projects"}})
	a := catalogAppForTest(path)
	if err := a.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	if !a.catalogConfig.migrated {
		t.Fatal("successful migration did not provide a user-facing completion signal")
	}
	if !a.groups[0].collapsed || a.groups[1].collapsed {
		t.Fatal("migration lost page-specific group collapse state")
	}
	firstSSH, secondSSH := catalogHostForTest(t, a, "first", false), catalogHostForTest(t, a, "second", false)
	firstFile, secondFile := catalogHostForTest(t, a, "first", true), catalogHostForTest(t, a, "second", true)
	deletedID, survivingID := a.hosts[firstSSH].session.ID, a.hosts[secondSSH].session.ID
	fileID := a.hosts[firstFile].session.ID
	if a.hosts[firstFile].session.Files.LocalDir != "~/Projects" || a.hosts[secondFile].session.Files.LocalDir != "~/Projects" {
		t.Fatal("migration did not preserve the legacy address-keyed preference for both sites")
	}
	for _, fixture := range []struct {
		path string
		data []byte
	}{{legacyPath + ".v1.bak", legacy}, {legacyPath, legacy}, {prefsPath + ".v1.bak", prefs}, {prefsPath, prefs}} {
		got, err := os.ReadFile(fixture.path)
		if err != nil || !bytes.Equal(got, fixture.data) {
			t.Fatalf("migration changed protected original data at %s", fixture.path)
		}
	}
	if err := a.commitCatalogMove(func() { removeCatalogFixtureHost(a, firstSSH) }); err != nil {
		t.Fatal(err)
	}
	firstFile = catalogHostForTest(t, a, "first", true)
	if a.hosts[firstFile].session.ID != fileID {
		t.Fatal("removing an SSH profile changed its independent file site")
	}
	if a.groups[0] == a.groups[1] {
		t.Fatal("migrated catalogs share mutable groups")
	}
	a.hosts[firstFile].address, a.groups[1].collapsed = "renamed.example", true
	if err := a.saveCatalog(); err != nil {
		t.Fatal(err)
	}
	restarted := catalogAppForTest(path)
	if err := restarted.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	if restarted.catalogConfig.migrated {
		t.Fatal("reading sessions reported a repeated migration")
	}
	if slices.ContainsFunc(restarted.hosts, func(h host) bool { return h.session.ID == deletedID }) {
		t.Fatal("reading sessions reimported the deleted SSH profile")
	}
	if restarted.hosts[catalogHostForTest(t, restarted, "second", false)].session.ID != survivingID {
		t.Fatal("deleting a profile changed a surviving stable ID")
	}
	fileHost := restarted.hosts[catalogHostForTest(t, restarted, "first", true)]
	file := &fileSession{endpoint: fileHost, config: fileHost.session, local: newFilePane(false), remote: newFilePane(true)}
	restarted.restoreFileSession(file)
	if file.local.path != "~/Projects" {
		t.Fatal("changing a site's address lost its ID-keyed local path")
	}
	temporary := &fileSession{temporary: true, local: newFilePane(false)}
	restarted.restoreFileSession(temporary)
	if temporary.local.path != "~" {
		t.Fatal("temporary file view inherited a saved site's preference")
	}
}

func TestCatalogFailedSaveRollsBackOnlyTheEditedCatalog(t *testing.T) {
	a := catalogAppForTest(t.TempDir())
	original := a.snapshotCatalog()
	group := a.groups[0]
	err := a.commitCatalogMove(func() {
		removeCatalogFixtureHost(a, 0)
		a.groups[0].name = "changed"
		a.hosts[0].session.ID = newSessionID()
	})
	if err == nil {
		t.Fatal("saving over a directory succeeded")
	}
	if !reflect.DeepEqual(a.hosts, original.hosts) || a.groups[0] != group {
		t.Fatal("failed save did not restore data and existing group identity")
	}
	for i, current := range a.groups {
		if !reflect.DeepEqual(*current, original.groupValues[i]) {
			t.Fatal("failed SSH edit changed group data, including the independent file catalog")
		}
	}
}

func TestCatalogDeletingHighestIDDoesNotReuseItAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	a := catalogAppForTest(path)
	deleted := hostID(len(demoHosts()) - 1)
	deletedUUID := a.hosts[deleted].session.ID
	fileUUID := a.hosts[catalogHostForTest(t, a, a.hosts[deleted].name, true)].session.ID
	if err := a.commitCatalogMove(func() { removeCatalogFixtureHost(a, deleted) }); err != nil {
		t.Fatal(err)
	}
	restarted := catalogAppForTest(path)
	if err := restarted.readCatalogFile(); err != nil {
		t.Fatal(err)
	}
	fresh := host{port: 22}
	initializeSession(&fresh)
	if fresh.session.ID == deletedUUID || slices.ContainsFunc(restarted.hosts, func(h host) bool { return h.session.ID == deletedUUID }) {
		t.Fatal("a deleted saved UUID was restored or reused")
	}
	if !slices.ContainsFunc(restarted.hosts, func(h host) bool { return h.session.ID == fileUUID }) {
		t.Fatal("deleting the highest SSH profile changed its independent file site")
	}
}

func TestCatalogV2RejectsInvalidIdentityWithoutOverwriting(t *testing.T) {
	for _, name := range []string{"unknown_version", "duplicate_id", "reused_next_id", "unknown_site_preference"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			legacyPath, path := filepath.Join(directory, "catalog.json"), filepath.Join(directory, "sessions.json")
			config := legacyV2Fixture()
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
			original := writeCatalogFixture(t, legacyPath, config)
			a := catalogAppForTest(path)
			if err := a.readCatalogFile(); err == nil {
				t.Fatal("invalid catalog was accepted")
			}
			got, err := os.ReadFile(legacyPath)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("rejecting invalid configuration overwrote the original")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("rejecting invalid configuration published sessions")
			}
		})
	}
}

func TestCatalogMigrationFailurePreservesOriginals(t *testing.T) {
	for _, name := range []string{"malformed_preferences", "existing_different_backup"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			legacyPath, path := filepath.Join(directory, "catalog.json"), filepath.Join(directory, "sessions.json")
			original := writeCatalogFixture(t, legacyPath, legacyCatalogFixture())
			if name == "malformed_preferences" {
				if err := os.WriteFile(filepath.Join(directory, "files.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(legacyPath+".v1.bak", []byte("previous backup"), 0o600); err != nil {
				t.Fatal(err)
			}
			a := catalogAppForTest(path)
			if err := a.readCatalogFile(); err == nil {
				t.Fatal("migration unexpectedly succeeded")
			}
			got, err := os.ReadFile(legacyPath)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("failed migration overwrote the original catalog")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("failed migration published sessions")
			}
		})
	}
}
