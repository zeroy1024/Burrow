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
)

type catalogConfigState struct {
	path    string
	loadErr error
}

type catalogConfig struct {
	Groups []catalogConfigGroup `json:"groups"`
	Hosts  []catalogConfigHost  `json:"hosts"`
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
	directory, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate catalog configuration directory: %w", err)
	}
	a.catalogConfig.path = filepath.Join(directory, "SSH Desk", "catalog.json")
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
	hosts, groups, err := config.catalog(a.hosts)
	if err != nil {
		return fmt.Errorf("validate catalog configuration: %w", err)
	}
	a.hosts, a.groups = hosts, groups
	return nil
}

func (config catalogConfig) catalog(runtimeHosts []host) ([]host, []*hostGroup, error) {
	if len(config.Groups) == 0 {
		return nil, nil, errors.New("catalog must contain a group")
	}
	groups := make([]*hostGroup, len(config.Groups))
	byID := make(map[int]*hostGroup, len(groups))
	names := make(map[string]bool, len(groups))
	for _, value := range config.Groups {
		validID := value.ID >= 0 && value.ID < len(groups)
		validOrder := value.Order >= 0 && value.Order < len(groups)
		if !validID || !validOrder {
			return nil, nil, errors.New("invalid group id or order")
		}
		if byID[value.ID] != nil || groups[value.Order] != nil {
			return nil, nil, errors.New("duplicate group id or order")
		}
		if strings.TrimSpace(value.Name) == "" || names[value.Name] {
			return nil, nil, errors.New("empty or duplicate group name")
		}
		group := &hostGroup{
			id: value.ID, name: value.Name, collapsed: value.Collapsed, hosts: []hostID{},
			filesCollapsed: value.FilesCollapsed,
		}
		groups[value.Order], byID[value.ID], names[value.Name] = group, group, true
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
		validUser := strings.TrimSpace(value.User) != ""
		validAddress := strings.TrimSpace(value.Address) != ""
		if !validName || !validUser || !validAddress {
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
		h.protocol = value.Protocol
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
		Groups: make([]catalogConfigGroup, len(a.groups)),
		Hosts:  make([]catalogConfigHost, len(a.hosts)),
	}
	for id, h := range a.hosts {
		config.Hosts[id] = catalogConfigHost{
			ID: hostID(id), Name: h.name, User: h.user, Address: h.address,
			Port: h.port, OS: h.os, System: h.system, Protocol: h.protocol,
		}
	}
	for order, group := range a.groups {
		config.Groups[order] = catalogConfigGroup{
			ID: group.id, Name: group.name, Order: order, Collapsed: group.collapsed,
			FilesCollapsed: group.filesCollapsed,
		}
		for position, id := range group.hosts {
			config.Hosts[id].GroupID, config.Hosts[id].Order = group.id, position
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
