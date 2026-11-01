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
)

type catalogConfigState struct {
	path     string
	loadErr  error
	migrated bool
}
type catalogConfig struct {
	Version int                  `json:"version"`
	SSH     catalogConfigData    `json:"ssh"`
	Files   catalogConfigData    `json:"files"`
	Groups  []catalogConfigGroup `json:"groups,omitempty"`
	Hosts   []catalogConfigHost  `json:"hosts,omitempty"`
}
type catalogConfigData struct {
	NextHostID  hostID               `json:"next_host_id"`
	NextGroupID int                  `json:"next_group_id"`
	Groups      []catalogConfigGroup `json:"groups"`
	Hosts       []catalogConfigHost  `json:"hosts"`
	LocalPaths  map[hostID]string    `json:"local_paths,omitempty"`
}
type catalogConfigGroup struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Order          int    `json:"order"`
	Collapsed      bool   `json:"collapsed"`
	FilesCollapsed bool   `json:"files_collapsed,omitempty"`
}
type catalogConfigHost struct {
	ID       hostID `json:"id"`
	GroupID  int    `json:"group_id"`
	Order    int    `json:"order"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	OS       int    `json:"os"`
	System   string `json:"system"`
	Protocol string `json:"protocol,omitempty"`
	Auth     string `json:"auth,omitempty"`
	KeyPath  string `json:"key_path,omitempty"`
}
type catalogSnapshot struct {
	target      *catalogData
	value       catalogData
	groupValues []hostGroup
}

func cloneCatalog(catalog catalogData) catalogData {
	cloned := catalog
	cloned.hosts, cloned.localPaths = maps.Clone(catalog.hosts), maps.Clone(catalog.localPaths)
	cloned.groups = make([]*hostGroup, len(catalog.groups))
	for i, group := range catalog.groups {
		copy := *group
		copy.hosts = slices.Clone(group.hosts)
		cloned.groups[i] = &copy
	}
	return cloned
}
func (a *app) snapshotCatalog() catalogSnapshot {
	catalog := a.catalog()
	snapshot := catalogSnapshot{target: catalog, value: *catalog, groupValues: make([]hostGroup, len(catalog.groups))}
	snapshot.value.hosts, snapshot.value.localPaths = maps.Clone(catalog.hosts), maps.Clone(catalog.localPaths)
	snapshot.value.groups = slices.Clone(catalog.groups)
	for i, group := range catalog.groups {
		snapshot.groupValues[i] = *group
		snapshot.groupValues[i].hosts = slices.Clone(group.hosts)
	}
	return snapshot
}
func (a *app) restoreCatalog(snapshot catalogSnapshot) {
	*snapshot.target = snapshot.value
	for i, group := range snapshot.target.groups {
		*group = snapshot.groupValues[i]
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
	// A failed read must never overwrite the user's catalog with initial data.
	a.catalogConfig.loadErr = err
	return err
}
func (a *app) readCatalog() error {
	directory, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate catalog configuration directory: %w", err)
	}
	a.catalogConfig.path = filepath.Join(directory, "SSH Desk", "catalog.json")
	return a.readCatalogFile()
}
func (a *app) readCatalogFile() error {
	data, err := os.ReadFile(a.catalogConfig.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read catalog configuration: %w", err)
	}
	var config catalogConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode catalog configuration: %w", err)
	}
	if config.Version == 0 {
		sshCatalog, fileCatalog, err := migrateLegacyCatalog(config, filepath.Join(filepath.Dir(a.catalogConfig.path), "files.json"))
		if err != nil {
			return fmt.Errorf("migrate catalog configuration: %w", err)
		}
		migrated, err := marshalCatalogs(sshCatalog, fileCatalog)
		if err != nil {
			return err
		}
		if err := backupLegacyConfig(a.catalogConfig.path, data); err != nil {
			return err
		}
		if err := writeCatalog(a.catalogConfig.path, migrated); err != nil {
			return err
		}
		a.sshCatalog, a.fileCatalog = sshCatalog, fileCatalog
		a.catalogConfig.migrated = true
		return nil
	}
	if config.Version != 2 {
		return fmt.Errorf("unsupported catalog configuration version %d", config.Version)
	}
	sshCatalog, err := config.SSH.catalog()
	if err != nil {
		return fmt.Errorf("validate SSH catalog: %w", err)
	}
	fileCatalog, err := config.Files.catalog()
	if err != nil {
		return fmt.Errorf("validate file catalog: %w", err)
	}
	a.sshCatalog, a.fileCatalog = sshCatalog, fileCatalog
	return nil
}
func (config catalogConfigData) catalog() (catalogData, error) {
	if len(config.Groups) == 0 {
		return catalogData{}, errors.New("catalog must contain a group")
	}
	catalog := catalogData{hosts: make(map[hostID]host, len(config.Hosts)), groups: make([]*hostGroup, len(config.Groups)),
		nextHostID: config.NextHostID, nextGroupID: config.NextGroupID, localPaths: map[hostID]string{}}
	byID, names := make(map[int]*hostGroup, len(config.Groups)), map[string]bool{}
	for _, value := range config.Groups {
		if value.ID < 0 || value.Order < 0 || value.Order >= len(config.Groups) || value.ID >= config.NextGroupID {
			return catalogData{}, errors.New("invalid group id, order or next id")
		}
		if byID[value.ID] != nil || catalog.groups[value.Order] != nil {
			return catalogData{}, errors.New("duplicate group id or order")
		}
		if strings.TrimSpace(value.Name) == "" || names[value.Name] {
			return catalogData{}, errors.New("empty or duplicate group name")
		}
		group := &hostGroup{id: value.ID, name: value.Name, collapsed: value.Collapsed, hosts: []hostID{}}
		catalog.groups[value.Order], byID[value.ID], names[value.Name] = group, group, true
	}
	if config.NextHostID < 0 {
		return catalogData{}, errors.New("invalid next host id")
	}
	ordered := make(map[int][]catalogConfigHost, len(config.Groups))
	for _, value := range config.Hosts {
		if value.ID < 0 || value.ID >= config.NextHostID {
			return catalogData{}, errors.New("invalid host id or next id")
		}
		if _, exists := catalog.hosts[value.ID]; exists {
			return catalogData{}, errors.New("duplicate host id")
		}
		if byID[value.GroupID] == nil || value.Order < 0 {
			return catalogData{}, errors.New("invalid host group or order")
		}
		if value.OS < 0 || value.OS >= len(systems) {
			return catalogData{}, errors.New("invalid host operating system")
		}
		if value.Port < 1 || value.Port > 65535 {
			return catalogData{}, errors.New("invalid host port")
		}
		if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.User) == "" || strings.TrimSpace(value.Address) == "" {
			return catalogData{}, errors.New("host connection fields must not be empty")
		}
		auth := value.Auth
		if auth == "" {
			auth = "agent"
		}
		if auth != "agent" && auth != "password" && auth != "key" {
			return catalogData{}, errors.New("invalid SSH authentication method")
		}
		if auth == "key" && strings.TrimSpace(value.KeyPath) == "" {
			return catalogData{}, errors.New("SSH private key path must not be empty")
		}
		if value.Protocol != "" && value.Protocol != "sftp" && value.Protocol != "ftp" {
			return catalogData{}, errors.New("invalid file transfer protocol")
		}
		catalog.hosts[value.ID] = host{name: value.Name, user: value.User, address: value.Address, port: value.Port,
			os: value.OS, system: value.System, protocol: value.Protocol, auth: auth, keyPath: value.KeyPath}
		ordered[value.GroupID] = append(ordered[value.GroupID], value)
	}
	for id, values := range ordered {
		slices.SortFunc(values, func(a, b catalogConfigHost) int { return cmp.Compare(a.Order, b.Order) })
		for order, value := range values {
			if value.Order != order {
				return catalogData{}, errors.New("host order must be consecutive within each group")
			}
			byID[id].hosts = append(byID[id].hosts, value.ID)
		}
	}
	for id, path := range config.LocalPaths {
		if _, exists := catalog.hosts[id]; !exists {
			return catalogData{}, errors.New("local path refers to an unknown file site")
		}
		if path == "" {
			return catalogData{}, errors.New("local path must not be empty")
		}
		catalog.localPaths[id] = path
	}
	return catalog, nil
}
func migrateLegacyCatalog(config catalogConfig, preferencesPath string) (catalogData, catalogData, error) {
	for _, group := range config.Groups {
		if group.ID < 0 || group.ID >= len(config.Groups) {
			return catalogData{}, catalogData{}, errors.New("invalid legacy group id")
		}
	}
	for _, value := range config.Hosts {
		if value.ID < 0 || int(value.ID) >= len(config.Hosts) {
			return catalogData{}, catalogData{}, errors.New("invalid legacy host id")
		}
	}
	legacy := catalogConfigData{NextHostID: hostID(len(config.Hosts)), NextGroupID: len(config.Groups), Groups: config.Groups, Hosts: config.Hosts}
	sshCatalog, err := legacy.catalog()
	if err != nil {
		return catalogData{}, catalogData{}, err
	}
	fileCatalog := cloneCatalog(sshCatalog)
	for _, value := range config.Groups {
		for _, group := range fileCatalog.groups {
			if group.id == value.ID {
				group.collapsed = value.FilesCollapsed
			}
		}
	}
	data, err := os.ReadFile(preferencesPath)
	if errors.Is(err, os.ErrNotExist) {
		return sshCatalog, fileCatalog, nil
	}
	if err != nil {
		return catalogData{}, catalogData{}, fmt.Errorf("read legacy file preferences: %w", err)
	}
	var prefs filePreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return catalogData{}, catalogData{}, fmt.Errorf("decode legacy file preferences: %w", err)
	}
	for id, value := range fileCatalog.hosts {
		key := fmt.Sprintf("%s@%s:%d", value.user, value.address, value.port)
		if local := prefs.LocalPaths[key]; local != "" {
			fileCatalog.localPaths[id] = local
		}
	}
	if err := backupLegacyConfig(preferencesPath, data); err != nil {
		return catalogData{}, catalogData{}, err
	}
	return sshCatalog, fileCatalog, nil
}
func backupLegacyConfig(path string, data []byte) error {
	backup := path + ".v1.bak"
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(backup)
		if readErr != nil {
			return fmt.Errorf("read existing configuration backup: %w", readErr)
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("configuration backup already exists with different contents: %s", backup)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("create configuration backup: %w", err)
	}
	_, writeErr := file.Write(data)
	syncErr, closeErr := file.Sync(), file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write configuration backup: %w", err)
	}
	return nil
}
func (catalog catalogData) configuration() catalogConfigData {
	config := catalogConfigData{NextHostID: catalog.nextHostID, NextGroupID: catalog.nextGroupID,
		Groups: make([]catalogConfigGroup, 0, len(catalog.groups)), Hosts: make([]catalogConfigHost, 0, len(catalog.hosts)), LocalPaths: maps.Clone(catalog.localPaths)}
	for order, group := range catalog.groups {
		config.Groups = append(config.Groups, catalogConfigGroup{ID: group.id, Name: group.name, Order: order, Collapsed: group.collapsed})
		for position, id := range group.hosts {
			h := catalog.hosts[id]
			config.Hosts = append(config.Hosts, catalogConfigHost{ID: id, GroupID: group.id, Order: position, Name: h.name, User: h.user, Address: h.address,
				Port: h.port, OS: h.os, System: h.system, Protocol: h.protocol, Auth: h.auth, KeyPath: h.keyPath})
		}
	}
	return config
}
func marshalCatalogs(sshCatalog, fileCatalog catalogData) ([]byte, error) {
	config := catalogConfig{Version: 2, SSH: sshCatalog.configuration(), Files: fileCatalog.configuration()}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode catalog configuration: %w", err)
	}
	return append(data, '\n'), nil
}
func (a *app) saveCatalog() error {
	if a.catalogConfig.loadErr != nil {
		return fmt.Errorf("catalog configuration was not loaded: %w", a.catalogConfig.loadErr)
	}
	if a.catalogConfig.path == "" {
		return errors.New("catalog configuration has not been loaded")
	}
	data, err := marshalCatalogs(a.sshCatalog, a.fileCatalog)
	if err != nil {
		return err
	}
	return writeCatalog(a.catalogConfig.path, data)
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
