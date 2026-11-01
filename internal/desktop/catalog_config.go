package desktop

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

type catalogConfigState struct {
	path    string
	loadErr error
}

type catalogConfig struct {
	Version   int                  `json:"version"`
	Groups    []catalogConfigGroup `json:"groups"`
	Hosts     []catalogConfigHost  `json:"hosts,omitempty"`
	Sessions  []storedSession      `json:"sessions,omitempty"`
	Keys      []managedKey         `json:"keys,omitempty"`
	Workspace *storedWorkspace     `json:"workspace,omitempty"`
}

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

type catalogConfigGroup struct {
	ID             int    `json:"id"`
	Scope          string `json:"scope"`
	Name           string `json:"name"`
	Order          int    `json:"order"`
	Collapsed      bool   `json:"collapsed"`
	FilesCollapsed bool   `json:"files_collapsed,omitempty"`
}

type catalogConfigHost struct {
	Session  sessionConfig `json:"session,omitempty"`
	ID       hostID        `json:"id"`
	GroupID  int           `json:"group_id"`
	Order    int           `json:"order"`
	Name     string        `json:"name"`
	User     string        `json:"user"`
	Address  string        `json:"address"`
	Port     int           `json:"port"`
	OS       int           `json:"os"`
	System   string        `json:"system"`
	Protocol string        `json:"protocol,omitempty"`
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

func (a *app) commitCatalogMove(change func()) error {
	snapshot := a.snapshotCatalog()
	change()
	if err := a.saveCatalog(); err != nil {
		a.restoreCatalog(snapshot)
		return err
	}
	return nil
}

func (a *app) loadCatalog() error {
	err := a.readCatalog()
	// A failed read must never turn the demo data into an overwrite of the user's file.
	a.catalogConfig.loadErr = err
	return err
}

func (a *app) readCatalog() error {
	directory, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return fmt.Errorf("locate catalog configuration directory: %w", err)
	}
	a.catalogConfig.path = filepath.Join(directory, "sessions.json")
	data, err := os.ReadFile(a.catalogConfig.path)
	if errors.Is(err, os.ErrNotExist) {
		// Read the existing prototype catalog once; subsequent writes use sessions.json.
		legacy, pathErr := os.UserConfigDir()
		if pathErr != nil {
			return pathErr
		}
		data, err = os.ReadFile(filepath.Join(legacy, "SSH Desk", "catalog.json"))
		if errors.Is(err, os.ErrNotExist) {
			for i := range a.hosts {
				initializeSession(&a.hosts[i])
			}
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("read catalog configuration: %w", err)
	}
	var config catalogConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode catalog configuration: %w", err)
	}
	if config.Version != 0 && config.Version != 1 {
		return errors.New("unsupported sessions version")
	}
	if config.Sessions != nil {
		config.Hosts = make([]catalogConfigHost, len(config.Sessions))
		ids := map[string]bool{}
		for i, s := range config.Sessions {
			if s.ID == "" || ids[s.ID] {
				return errors.New("empty or duplicate session id")
			}
			ids[s.ID] = true
			protocol := s.Protocol
			if protocol == "ssh" {
				protocol = ""
			}
			config.Hosts[i] = catalogConfigHost{ID: hostID(i), GroupID: s.GroupID, Order: s.Order, Name: s.Name, User: s.User, Address: s.Host, Port: s.Port, OS: s.OS, System: s.System, Protocol: protocol, Session: s.sessionConfig}
		}
	}
	hosts, groups, err := config.catalog(a.hosts)
	if err != nil {
		return fmt.Errorf("validate catalog configuration: %w", err)
	}
	for i := range hosts {
		initializeSession(&hosts[i])
	}
	if err := validateSessionCatalog(hosts, config.Keys); err != nil {
		return err
	}
	for i := range config.Keys {
		expected := filepath.Join(directory, "keys", config.Keys[i].ID)
		if filepath.Clean(config.Keys[i].Path) != expected {
			return errors.New("managed key path must be inside the application's keys directory")
		}
		if info, err := os.Lstat(expected); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("managed key must not be a symbolic link")
		}
	}
	a.hosts, a.groups, a.keys = hosts, groups, config.Keys
	a.savedWorkspace = config.Workspace
	return nil
}

func (config catalogConfig) catalog(runtimeHosts []host) ([]host, []*hostGroup, error) {
	if len(config.Groups) == 0 {
		return nil, nil, errors.New("catalog must contain a group")
	}
	groups := make([]*hostGroup, len(config.Groups))
	byID := make(map[int]*hostGroup, len(groups))
	type groupName struct {
		scope section
		name  string
	}
	names := make(map[groupName]bool, len(groups))
	legacyGroups := make(map[int]bool)
	legacyFilesCollapsed := make(map[int]bool)
	for _, value := range config.Groups {
		validID := value.ID >= 0 && value.ID < len(groups)
		validOrder := value.Order >= 0 && value.Order < len(groups)
		if !validID || !validOrder {
			return nil, nil, errors.New("invalid group id or order")
		}
		if byID[value.ID] != nil || groups[value.Order] != nil {
			return nil, nil, errors.New("duplicate group id or order")
		}
		var scope section
		switch value.Scope {
		case "":
			scope = sectionSSH
			legacyGroups[value.ID] = true
			legacyFilesCollapsed[value.ID] = value.FilesCollapsed
		case "ssh":
			scope = sectionSSH
		case "files":
			scope = sectionFiles
		default:
			return nil, nil, errors.New("invalid group scope")
		}
		name := groupName{scope: scope, name: value.Name}
		if strings.TrimSpace(value.Name) == "" || names[name] {
			return nil, nil, errors.New("empty or duplicate group name")
		}
		group := &hostGroup{
			id: value.ID, scope: scope, name: value.Name, collapsed: value.Collapsed, hosts: []hostID{},
		}
		groups[value.Order], byID[value.ID], names[name] = group, group, true
	}
	hosts := make([]host, len(config.Hosts))
	seen := make([]bool, len(hosts))
	ordered := make(map[int][]catalogConfigHost, len(groups))
	for _, value := range config.Hosts {
		if value.ID < 0 || int(value.ID) >= len(hosts) {
			return nil, nil, errors.New("invalid host id")
		}
		if seen[value.ID] {
			return nil, nil, errors.New("duplicate host id")
		}
		if byID[value.GroupID] == nil || value.Order < 0 {
			return nil, nil, errors.New("invalid host group or order")
		}
		if value.OS < 0 || value.OS >= len(systems) {
			return nil, nil, errors.New("invalid host operating system")
		}
		if value.Port < 1 || value.Port > 65535 {
			return nil, nil, errors.New("invalid host port")
		}
		validName := strings.TrimSpace(value.Name) != ""
		validAddress := strings.TrimSpace(value.Address) != ""
		if !validName || !validAddress {
			return nil, nil, errors.New("host connection fields must not be empty")
		}
		if int(value.ID) < len(runtimeHosts) {
			hosts[value.ID] = runtimeHosts[value.ID]
		}
		h := &hosts[value.ID]
		h.name, h.user, h.address = value.Name, value.User, value.Address
		h.port, h.os, h.system = value.Port, value.OS, value.System
		if value.Protocol != "" && value.Protocol != "sftp" && value.Protocol != "ftp" {
			return nil, nil, errors.New("invalid file transfer protocol")
		}
		scope := sectionSSH
		if value.Protocol == "sftp" || value.Protocol == "ftp" {
			scope = sectionFiles
		}
		if !legacyGroups[value.GroupID] && byID[value.GroupID].scope != scope {
			return nil, nil, errors.New("host protocol must match its group scope")
		}
		h.protocol = value.Protocol
		h.session = value.Session
		seen[value.ID] = true
		ordered[value.GroupID] = append(ordered[value.GroupID], value)
	}
	for id, values := range ordered {
		slices.SortFunc(values, func(a, b catalogConfigHost) int { return cmp.Compare(a.Order, b.Order) })
		for order, value := range values {
			if value.Order != order {
				return nil, nil, errors.New("host order must be consecutive within each group")
			}
			byID[id].hosts = append(byID[id].hosts, value.ID)
		}
	}
	for _, group := range groups {
		if !legacyGroups[group.id] {
			continue
		}
		name := groupName{scope: sectionFiles, name: group.name}
		if names[name] {
			return nil, nil, errors.New("duplicate file transfer group name")
		}
		filesGroup := &hostGroup{
			id: len(groups), scope: sectionFiles, name: group.name,
			collapsed: legacyFilesCollapsed[group.id], hosts: []hostID{},
		}
		// Split the old shared group without duplicating sessions or their credentials.
		sshHosts := make([]hostID, 0, len(group.hosts))
		for _, id := range group.hosts {
			if hosts[id].protocol == "sftp" || hosts[id].protocol == "ftp" {
				filesGroup.hosts = append(filesGroup.hosts, id)
			} else {
				sshHosts = append(sshHosts, id)
			}
		}
		group.hosts = sshHosts
		groups = append(groups, filesGroup)
		names[name] = true
	}
	sshGroups, filesGroups := 0, 0
	for _, group := range groups {
		if group.scope == sectionFiles {
			filesGroups++
		} else {
			sshGroups++
		}
	}
	if sshGroups == 0 || filesGroups == 0 {
		return nil, nil, errors.New("catalog must contain a group for SSH and file transfer")
	}
	return hosts, groups, nil
}

func (a *app) saveCatalog() error {
	if a.catalogConfig.loadErr != nil {
		return fmt.Errorf("catalog configuration was not loaded: %w", a.catalogConfig.loadErr)
	}
	if a.catalogConfig.path == "" {
		return errors.New("catalog configuration has not been loaded")
	}
	config := catalogConfig{
		Version: 1, Keys: a.keys, Workspace: a.savedWorkspace,
		Groups:   make([]catalogConfigGroup, len(a.groups)),
		Sessions: make([]storedSession, len(a.hosts)),
	}
	for id := range a.hosts {
		h := &a.hosts[id]
		initializeSession(h)
		if h.session.UpdatedAt.IsZero() {
			h.session.UpdatedAt = time.Now().UTC()
		}
		protocol := h.protocol
		if protocol == "" {
			protocol = "ssh"
		}
		config.Sessions[id] = storedSession{sessionConfig: h.session, Name: h.name, User: h.user, Host: h.address, Port: h.port, OS: h.os, System: h.system, Protocol: protocol}
	}
	for order, group := range a.groups {
		scope := "ssh"
		if group.scope == sectionFiles {
			scope = "files"
		}
		config.Groups[order] = catalogConfigGroup{
			ID: group.id, Scope: scope, Name: group.name, Order: order, Collapsed: group.collapsed,
		}
		for position, id := range group.hosts {
			config.Sessions[id].GroupID, config.Sessions[id].Order = group.id, position
		}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode catalog configuration: %w", err)
	}
	return writeCatalog(a.catalogConfig.path, append(data, '\n'))
}

func writeCatalog(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create catalog configuration directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".catalog-*.json")
	if err != nil {
		return fmt.Errorf("create pending catalog configuration: %w", err)
	}
	closed, committed := false, false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close pending catalog configuration: %w", closeErr))
			}
		}
		if !committed {
			if removeErr := os.Remove(file.Name()); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("remove pending catalog configuration: %w", removeErr))
			}
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write pending catalog configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync pending catalog configuration: %w", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("close pending catalog configuration: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace catalog configuration: %w", err)
	}
	committed = true
	return nil
}
