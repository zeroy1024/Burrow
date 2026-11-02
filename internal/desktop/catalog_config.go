package desktop

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

type catalogConfigState struct {
	path       string
	legacyPath string
	loadErr    error
	migrated   bool
	migration  catalogMigrationState
}

type catalogConfig struct {
	Version   int                  `json:"version"`
	Groups    []catalogConfigGroup `json:"groups"`
	Hosts     []catalogConfigHost  `json:"hosts,omitempty"`
	Sessions  []storedSession      `json:"sessions,omitempty"`
	Keys      []managedKey         `json:"keys,omitempty"`
	Workspace *storedWorkspace     `json:"workspace,omitempty"`
	SSH       legacyCatalogData    `json:"ssh,omitzero"`
	Files     legacyCatalogData    `json:"files,omitzero"`
}

type legacyCatalogData struct {
	NextHostID  hostID               `json:"next_host_id"`
	NextGroupID int                  `json:"next_group_id"`
	Groups      []catalogConfigGroup `json:"groups"`
	Hosts       []catalogConfigHost  `json:"hosts"`
	LocalPaths  map[hostID]string    `json:"local_paths,omitempty"`
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
	Auth     string        `json:"auth,omitempty"`
	KeyPath  string        `json:"key_path,omitempty"`
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
	if a.hosts == nil && a.groups == nil {
		a.emptyCatalog()
	}
	err := a.readCatalog()
	// A failed read must never turn the initial state into an overwrite of the user's file.
	a.catalogConfig.loadErr = err
	return err
}

func (a *app) readCatalog() error {
	directory, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return fmt.Errorf("locate catalog configuration directory: %w", err)
	}
	a.catalogConfig.path = filepath.Join(directory, "sessions.json")
	a.catalogConfig.legacyPath = filepath.Join(directory, "catalog.json")
	return a.readCatalogFile()
}

func (a *app) emptyCatalog() {
	a.hosts, a.keys = []host{}, []managedKey{}
	a.savedWorkspace = nil
	a.groups = []*hostGroup{}
}

func (a *app) readCatalogFile() error {
	a.catalogConfig.migrated = false
	directory := filepath.Dir(a.catalogConfig.path)
	data, err := os.ReadFile(a.catalogConfig.path)
	if errors.Is(err, os.ErrNotExist) {
		return a.importLegacyCatalog(directory)
	}
	if err != nil {
		return fmt.Errorf("read catalog configuration: %w", err)
	}
	var config catalogConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode catalog configuration: %w", err)
	}
	if config.Version != 1 {
		return errors.New("unsupported sessions version")
	}
	return a.applyStoredCatalog(config, directory)
}

func (a *app) applyStoredCatalog(config catalogConfig, directory string) error {
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
	hosts, groups, err := config.catalog(nil)
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
		info, err := os.Lstat(expected)
		if err != nil {
			return fmt.Errorf("read managed private key: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("managed key must be a regular file, not a symbolic link")
		}
		if info.Mode().Perm()&0077 != 0 {
			return errors.New("managed private key permissions must be private")
		}
	}
	if err := validateStoredWorkspace(config.Workspace, hosts, config.Keys); err != nil {
		return fmt.Errorf("validate saved workspace: %w", err)
	}
	a.hosts, a.groups, a.keys = hosts, groups, config.Keys
	a.savedWorkspace = config.Workspace
	return nil
}

func (a *app) importLegacyCatalog(directory string) (result error) {
	legacyPath := a.catalogConfig.legacyPath
	if legacyPath == "" {
		legacyPath = filepath.Join(directory, "catalog.json")
	}
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		a.emptyCatalog()
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy catalog: %w", err)
	}
	var source catalogConfig
	if err := json.Unmarshal(data, &source); err != nil {
		return fmt.Errorf("decode legacy catalog: %w", err)
	}
	if source.Version != 0 && source.Version != 2 {
		return fmt.Errorf("unsupported legacy catalog version %d", source.Version)
	}
	preferencesPath := filepath.Join(filepath.Dir(legacyPath), "files.json")
	prefsData, err := os.ReadFile(preferencesPath)
	prefsPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read legacy file preferences: %w", err)
	}
	var prefs filePreferences
	if prefsPresent {
		if err := json.Unmarshal(prefsData, &prefs); err != nil {
			return fmt.Errorf("decode legacy file preferences: %w", err)
		}
		if prefs.LocalRatio <= 0 || prefs.LocalRatio >= 1 || prefs.TransferFilter < 0 ||
			prefs.TransferFilter > 2 || prefs.TransferDockHeight < 0 {
			return errors.New("invalid legacy file preferences")
		}
	}
	config := catalogConfig{Version: 1, Groups: []catalogConfigGroup{}, Hosts: []catalogConfigHost{}, Keys: []managedKey{}}
	keySources := map[hostID]string{}
	sshSource, fileSource := source.SSH, source.Files
	if source.Version == 0 {
		sshSource = legacyCatalogData{
			NextHostID: hostID(len(source.Hosts)), NextGroupID: len(source.Groups),
			Groups: source.Groups, Hosts: source.Hosts,
		}
		fileSource = sshSource
		fileSource.Groups = slices.Clone(source.Groups)
		for index := range fileSource.Groups {
			fileSource.Groups[index].Collapsed = fileSource.Groups[index].FilesCollapsed
		}
	}
	if err := appendLegacyScope(&config, sshSource, sectionSSH, prefs, keySources); err != nil {
		return fmt.Errorf("migrate SSH catalog: %w", err)
	}
	if err := appendLegacyScope(&config, fileSource, sectionFiles, prefs, keySources); err != nil {
		return fmt.Errorf("migrate file catalog: %w", err)
	}
	backupVersion := 2
	if source.Version == 0 {
		backupVersion = 1
	}
	if err := backupLegacyConfig(legacyPath, data, backupVersion); err != nil {
		return err
	}
	if prefsPresent {
		if err := backupLegacyConfig(preferencesPath, prefsData, backupVersion); err != nil {
			return err
		}
	}
	created := []string{}
	defer func() {
		if result != nil {
			cleanupErr := discardMigrationFiles(created)
			if cleanupErr != nil && a.catalogConfig.migration.plan != nil {
				a.catalogConfig.migration.unpublished = slices.Clone(created)
			}
			result = errors.Join(result, cleanupErr)
		}
	}()
	metadata, err := json.Marshal(config)
	if err != nil {
		return err
	}
	plan := &catalogMigrationPlan{
		directory: directory, destination: a.catalogConfig.path,
		metadata: metadata, keySources: maps.Clone(keySources),
	}
	imported := map[string]managedKey{}
	for index := range config.Hosts {
		path := keySources[hostID(index)]
		if path == "" {
			continue
		}
		key, exists := imported[path]
		if !exists {
			var files []string
			key, files, err = copyLegacyPrivateKey(path, directory, nil)
			created = append(created, files...)
			if errors.Is(err, errLegacyPrivateKeyLocked) {
				a.catalogConfig.migration.plan = plan
				return errCatalogMigrationDeferred
			}
			if err != nil {
				return fmt.Errorf("import legacy private key: %w", err)
			}
			imported[path] = key
			config.Keys = append(config.Keys, key)
		}
		config.Hosts[index].Session.KeyID = key.ID
	}
	candidate := &app{}
	if err := candidate.applyStoredCatalog(config, directory); err != nil {
		return err
	}
	encoded, err := candidate.catalogBytes()
	if err != nil {
		return err
	}
	published, err := createCatalog(a.catalogConfig.path, encoded)
	if published {
		// A pending-file cleanup error must not remove keys referenced by the
		// sessions file that has already been atomically published.
		created = []string{}
	}
	if err != nil {
		return fmt.Errorf("create migrated sessions: %w", err)
	}
	a.hosts, a.groups, a.keys = candidate.hosts, candidate.groups, candidate.keys
	a.savedWorkspace = candidate.savedWorkspace
	a.catalogConfig.migrated = true
	return nil
}

func appendLegacyScope(
	target *catalogConfig,
	source legacyCatalogData,
	scope section,
	prefs filePreferences,
	keySources map[hostID]string,
) error {
	if len(source.Groups) == 0 || source.NextHostID < 0 {
		return errors.New("legacy catalog must contain groups and valid next IDs")
	}
	groups := make([]catalogConfigGroup, len(source.Groups))
	byID := map[int]int{}
	names := map[string]bool{}
	for _, group := range source.Groups {
		validOrder := group.Order >= 0 && group.Order < len(groups)
		if group.ID < 0 || group.ID >= source.NextGroupID || !validOrder {
			return errors.New("invalid legacy group id or order")
		}
		if _, exists := byID[group.ID]; exists || groups[group.Order].Name != "" {
			return errors.New("duplicate legacy group id or order")
		}
		if strings.TrimSpace(group.Name) == "" || names[group.Name] {
			return errors.New("empty or duplicate legacy group name")
		}
		groups[group.Order], byID[group.ID], names[group.Name] = group, 0, true
	}
	for _, group := range groups {
		id := len(target.Groups)
		byID[group.ID] = id
		group.ID, group.Order = id, id
		group.Scope = "ssh"
		if scope == sectionFiles {
			group.Scope = "files"
		}
		target.Groups = append(target.Groups, group)
	}
	seen := map[hostID]bool{}
	ordered := map[int][]catalogConfigHost{}
	for _, h := range source.Hosts {
		if h.ID < 0 || h.ID >= source.NextHostID || seen[h.ID] {
			return errors.New("invalid or duplicate legacy host id or next id")
		}
		if _, exists := byID[h.GroupID]; !exists || h.Order < 0 {
			return errors.New("invalid legacy host group or order")
		}
		if h.OS < 0 || h.OS >= len(systems) || h.Port < 1 || h.Port > 65535 {
			return errors.New("invalid legacy host system or port")
		}
		if strings.TrimSpace(h.Name) == "" || strings.TrimSpace(h.Address) == "" || strings.TrimSpace(h.User) == "" {
			return errors.New("legacy host connection fields must not be empty")
		}
		if h.Protocol != "" && h.Protocol != "sftp" && h.Protocol != "ftp" {
			return errors.New("invalid legacy file protocol")
		}
		if h.Auth == "" {
			h.Auth = "agent"
		}
		if !slices.Contains([]string{"agent", "password", "key"}, h.Auth) {
			return errors.New("invalid legacy authentication method")
		}
		if h.Auth == "key" && strings.TrimSpace(h.KeyPath) == "" {
			return errors.New("legacy private key path is empty")
		}
		seen[h.ID] = true
		ordered[h.GroupID] = append(ordered[h.GroupID], h)
	}
	for id, local := range source.LocalPaths {
		if !seen[id] || local == "" {
			return errors.New("legacy local path refers to an unknown site or is empty")
		}
	}
	for _, group := range groups {
		values := ordered[group.ID]
		slices.SortFunc(values, func(a, b catalogConfigHost) int { return cmp.Compare(a.Order, b.Order) })
		for order, value := range values {
			if value.Order != order {
				return errors.New("legacy host order must be consecutive within each group")
			}
			oldID := value.ID
			value.ID, value.GroupID = hostID(len(target.Hosts)), byID[value.GroupID]
			value.Session = sessionConfig{Auth: value.Auth}
			if scope == sectionSSH {
				value.Protocol = ""
			} else if value.Protocol == "" {
				value.Protocol = "sftp"
			}
			if value.Protocol == "ftp" {
				value.Session.Auth = "password"
			}
			profile := host{protocol: value.Protocol, session: value.Session}
			initializeSession(&profile)
			value.Session = profile.session
			if value.Protocol == "ftp" {
				value.Session.Files.FTPTLS = "none"
			}
			if scope == sectionFiles {
				local := source.LocalPaths[oldID]
				if local == "" {
					local = prefs.LocalPaths[fmt.Sprintf("%s@%s:%d", value.User, value.Address, value.Port)]
				}
				value.Session.Files.LocalDir = local
			}
			if value.Session.Auth == "key" {
				keySources[value.ID] = filepath.Clean(value.KeyPath)
			}
			value.Auth, value.KeyPath = "", ""
			target.Hosts = append(target.Hosts, value)
		}
	}
	return nil
}

func backupLegacyConfig(path string, data []byte, version int) error {
	backup := fmt.Sprintf("%s.v%d.bak", path, version)
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(backup)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("configuration backup already exists with different contents: %s", backup)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("create legacy configuration backup: %w", err)
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return errors.Join(err, os.Remove(backup))
	}
	return nil
}

func (config catalogConfig) catalog(runtimeHosts []host) ([]host, []*hostGroup, error) {
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
	return hosts, groups, nil
}

func (a *app) saveCatalog() error {
	if a.catalogConfig.loadErr != nil {
		return fmt.Errorf("catalog configuration was not loaded: %w", a.catalogConfig.loadErr)
	}
	if a.catalogConfig.path == "" {
		return errors.New("catalog configuration has not been loaded")
	}
	data, err := a.catalogBytes()
	if err != nil {
		return err
	}
	return writeCatalog(a.catalogConfig.path, data)
}

func (a *app) catalogBytes() ([]byte, error) {
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
	// Persist dense IDs required by this format without renumbering the live
	// group keys used by drag gestures, transitions and page state.
	for order, group := range a.groups {
		scope := "ssh"
		if group.scope == sectionFiles {
			scope = "files"
		}
		config.Groups[order] = catalogConfigGroup{
			ID: order, Scope: scope, Name: group.name, Order: order, Collapsed: group.collapsed,
		}
		for position, id := range group.hosts {
			config.Sessions[id].GroupID, config.Sessions[id].Order = order, position
		}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode catalog configuration: %w", err)
	}
	return append(data, '\n'), nil
}

// Migration creates a new destination only. Linking a fully synced pending
// file publishes it atomically and cannot replace sessions from another process.
func createCatalog(path string, data []byte) (published bool, result error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sessions-*.json")
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, os.Remove(file.Name())) }()
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return false, err
	}
	if err := os.Link(file.Name(), path); err != nil {
		return false, err
	}
	return true, nil
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
