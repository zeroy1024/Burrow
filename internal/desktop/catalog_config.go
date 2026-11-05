package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"slices"
	"strings"
	"time"

	"sshdesk/internal/store"
)

type catalogConfigState struct{ loadErr error }

type storedSession struct {
	sessionConfig
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	GroupID  int    `json:"groupId"`
	Order    int    `json:"order"`
	OS       int    `json:"os"`
	System   string `json:"system,omitempty"`
}

type catalogSnapshot struct {
	hosts       []host
	groups      []*hostGroup
	groupValues []hostGroup
}

func (a *app) snapshotCatalog() catalogSnapshot {
	snapshot := catalogSnapshot{
		hosts: slices.Clone(a.hosts), groups: slices.Clone(a.groups),
		groupValues: make([]hostGroup, len(a.groups)),
	}
	for i, group := range a.groups {
		snapshot.groupValues[i] = *group
		snapshot.groupValues[i].hosts = slices.Clone(group.hosts)
	}
	return snapshot
}

func (a *app) restoreCatalog(snapshot catalogSnapshot) {
	a.hosts, a.groups = slices.Clone(snapshot.hosts), slices.Clone(snapshot.groups)
	for i, group := range a.groups {
		*group = snapshot.groupValues[i]
		group.hosts = slices.Clone(snapshot.groupValues[i].hosts)
	}
}

func (a *app) beginCatalogMove(change func()) func(error) {
	snapshot := a.snapshotCatalog()
	change()
	return func(err error) {
		if err != nil {
			a.restoreCatalog(snapshot)
		}
	}
}

func (a *app) loadCatalog() error {
	a.emptyCatalog()
	err := a.readCatalog()
	if err != nil {
		a.emptyCatalog()
	}
	a.catalogConfig.loadErr = err
	return err
}

func (a *app) emptyCatalog() {
	a.hosts, a.keys = []host{}, []managedKey{}
	a.groups = []*hostGroup{}
	a.savedWorkspace = nil
}

func (a *app) readCatalog() error {
	c, err := a.store.ReadCatalog(a.ctx)
	if err != nil {
		return err
	}
	groupIndex := map[string]int{}
	for _, g := range c.Groups {
		scope := sectionSSH
		if g.Scope == "files" {
			scope = sectionFiles
		}
		index := len(a.groups)
		groupIndex[g.ID] = index
		a.groups = append(a.groups, &hostGroup{id: index, stableID: g.ID, scope: scope, name: g.Name, collapsed: g.Collapsed})
	}
	for _, p := range c.Profiles {
		var s storedSession
		if err := json.Unmarshal(p.Value, &s); err != nil {
			return err
		}
		if s.ID != p.ID || s.KeyID != p.KeyID {
			return errors.New("profile identity does not match database references")
		}
		jump := ""
		if s.SSH != nil {
			jump = s.SSH.JumpHostID
		}
		if jump != p.JumpID {
			return errors.New("profile jump reference does not match database")
		}
		group, exists := groupIndex[p.GroupID]
		if !exists {
			return errors.New("profile group does not exist")
		}
		h := transferStoredHost(s)
		if h.protocol == "ssh" {
			h.protocol = ""
		}
		if strings.TrimSpace(h.name) == "" || strings.TrimSpace(h.address) == "" || h.port < 1 || h.port > 65535 || h.os < 0 || h.os >= len(systems) {
			return errors.New("invalid profile endpoint")
		}
		scope := sectionSSH
		if h.protocol == "sftp" || h.protocol == "ftp" {
			scope = sectionFiles
		} else if h.protocol != "" {
			return errors.New("invalid profile protocol")
		}
		g := a.groups[group]
		if g.scope != scope || p.Position != len(g.hosts) {
			return errors.New("invalid profile group or order")
		}
		g.hosts = append(g.hosts, hostID(len(a.hosts)))
		a.hosts = append(a.hosts, h)
	}
	for _, k := range c.Keys {
		public, _, _, _, err := ssh.ParseAuthorizedKey(k.PublicKey)
		if err != nil {
			return fmt.Errorf("parse managed public key: %w", err)
		}
		if ssh.FingerprintSHA256(public) != k.Fingerprint {
			return errors.New("managed public key fingerprint mismatch")
		}
		created, err := time.Parse(time.RFC3339Nano, k.CreatedAt)
		if err != nil {
			return err
		}
		if k.SecretID != privateCredentialKey(k.ID) {
			return errors.New("managed key credential reference mismatch")
		}
		a.keys = append(a.keys, managedKey{PublicKey: k.PublicKey, ID: k.ID, Name: k.Name, Type: k.Kind, Fingerprint: k.Fingerprint, SecretID: k.SecretID, Comment: k.Comment, Bits: k.Bits, CreatedAt: created})
	}
	if err := validateSessionCatalog(a.hosts, a.keys); err != nil {
		return err
	}
	if c.Workspace != nil {
		saved, err := decodeWorkspace(*c.Workspace)
		if err != nil {
			return err
		}
		if err := validateStoredWorkspace(saved, a.hosts, a.keys); err != nil {
			return err
		}
		a.savedWorkspace = saved
	}
	return nil
}

func (a *app) catalogSnapshotData() (store.Catalog, error) {
	if a.catalogConfig.loadErr != nil {
		return store.Catalog{}, fmt.Errorf("catalog was not loaded: %w", a.catalogConfig.loadErr)
	}
	if err := validateSessionCatalog(a.hosts, a.keys); err != nil {
		return store.Catalog{}, err
	}
	c := store.Catalog{}
	for _, k := range a.keys {
		c.Keys = append(c.Keys, store.Key{PublicKey: k.PublicKey, ID: k.ID, Name: k.Name, Kind: k.Type, Fingerprint: k.Fingerprint, SecretID: k.SecretID, Comment: k.Comment, Bits: k.Bits, CreatedAt: k.CreatedAt.Format(time.RFC3339Nano)})
	}
	for order, g := range a.groups {
		if g.stableID == "" {
			g.stableID = newSessionID()
		}
		scope := "ssh"
		if g.scope == sectionFiles {
			scope = "files"
		}
		c.Groups = append(c.Groups, store.Group{ID: g.stableID, Scope: scope, Name: g.name, Position: order, Collapsed: g.collapsed})
		for position, id := range g.hosts {
			h := &a.hosts[id]
			initializeSession(h)
			if h.session.UpdatedAt.IsZero() {
				h.session.UpdatedAt = time.Now().UTC()
			}
			endpoint := snapshotWorkspaceSession(*h, h.session, false, 0).Endpoint
			data, err := json.Marshal(endpoint)
			if err != nil {
				return c, err
			}
			jump := ""
			if h.session.SSH != nil {
				jump = h.session.SSH.JumpHostID
			}
			c.Profiles = append(c.Profiles, store.Profile{ID: h.session.ID, GroupID: g.stableID, Position: position, KeyID: h.session.KeyID, JumpID: jump, Value: data})
		}
	}
	if len(c.Profiles) != len(a.hosts) {
		return c, errors.New("every profile must belong to exactly one group")
	}
	saved := a.workspaceSnapshot()
	if err := validateStoredWorkspace(saved, a.hosts, a.keys); err != nil {
		return c, err
	}
	w, err := encodeWorkspace(saved)
	if err != nil {
		return c, err
	}
	c.Workspace = &w
	return c, nil
}
