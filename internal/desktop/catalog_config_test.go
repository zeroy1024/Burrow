package desktop

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/zeroy1024/Burrow/internal/store"
)

func catalogAppForTest(t *testing.T, path string) *app {
	t.Helper()
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &app{ctx: context.Background(), store: db, dataDirectory: filepath.Dir(path), hosts: demoHosts(), groups: demoGroups()}
	for i := range a.hosts {
		initializeSession(&a.hosts[i])
	}
	for _, g := range a.groups[:3] {
		for _, id := range g.hosts {
			h := a.hosts[id]
			h.protocol, h.session = "sftp", sessionConfig{Auth: "agent"}
			initializeSession(&h)
			fileID := hostID(len(a.hosts))
			a.hosts = append(a.hosts, h)
			a.groups[g.id+3].hosts = append(a.groups[g.id+3].hosts, fileID)
		}
	}
	for _, g := range a.groups {
		g.stableID = newSessionID()
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

func TestCatalogFailedSaveRollsBackOnlyTheEditedCatalog(t *testing.T) {
	a := catalogAppForTest(t, filepath.Join(t.TempDir(), "burrow.db"))
	original := a.snapshotCatalog()
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	group := a.groups[0]
	err := a.commitCatalogMove(func() {
		removeCatalogFixtureHost(a, 0)
		a.groups[0].name = "changed"
		a.hosts[0].session.ID = newSessionID()
	})
	if err == nil {
		t.Fatal("saving to a closed database succeeded")
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
	path := filepath.Join(t.TempDir(), "burrow.db")
	a := catalogAppForTest(t, path)
	deleted := hostID(len(demoHosts()) - 1)
	deletedUUID := a.hosts[deleted].session.ID
	fileUUID := a.hosts[catalogHostForTest(t, a, a.hosts[deleted].name, true)].session.ID
	if err := a.commitCatalogMove(func() { removeCatalogFixtureHost(a, deleted) }); err != nil {
		t.Fatal(err)
	}
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := catalogAppForTest(t, path)
	if err := restarted.readCatalog(); err != nil {
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
func (a *app) saveCatalog() error {
	c, err := a.catalogSnapshotData()
	if err != nil {
		return err
	}
	return a.store.SaveCatalog(a.ctx, c)
}

func (a *app) commitCatalogMove(change func()) error {
	complete := a.beginCatalogMove(change)
	err := a.saveCatalog()
	complete(err)
	return err
}
