package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"slices"
	"sshdesk/internal/store"
	"strings"
	"time"
)

type storedWorkspace struct {
	Version     int                         `json:"version,omitempty"`
	Tabs        []storedWorkspaceTab        `json:"tabs,omitempty"`
	FileTabs    []storedFileTab             `json:"fileTabs,omitempty"`
	Connections []storedWorkspaceConnection `json:"connections,omitempty"`
	Section     section                     `json:"section"`
	ActiveSSH   int                         `json:"activeSSH"`
	ActiveFile  int                         `json:"activeFile"`
}

type storedWorkspaceConnection struct {
	EndpointRef string        `json:"endpointRef,omitempty"`
	ID          int           `json:"id"`
	Endpoint    storedSession `json:"endpoint,omitzero"`
}

type storedWorkspaceTab struct {
	Root      *storedWorkspaceLayout `json:"root"`
	Focused   int                    `json:"focused"`
	Maximized bool                   `json:"maximized,omitempty"`
}

type storedWorkspaceLayout struct {
	Session *storedWorkspaceSession `json:"session,omitempty"`
	First   *storedWorkspaceLayout  `json:"first,omitempty"`
	Second  *storedWorkspaceLayout  `json:"second,omitempty"`
	Stacked bool                    `json:"stacked,omitempty"`
	Ratio   float32                 `json:"ratio,omitempty"`
}

type storedWorkspaceSession struct {
	EndpointRef string        `json:"endpointRef,omitempty"`
	ProfileID   string        `json:"profileId,omitempty"`
	Endpoint    storedSession `json:"endpoint,omitzero"`
	Temporary   bool          `json:"temporary,omitempty"`
	Connection  int           `json:"connection,omitempty"`
}

type storedFileTab struct {
	ID         string                 `json:"id,omitempty"`
	Session    storedWorkspaceSession `json:"session"`
	LocalPath  string                 `json:"localPath"`
	RemotePath string                 `json:"remotePath"`
}

func (a *app) workspaceSnapshot() *storedWorkspace {
	result := &storedWorkspace{
		Version: 2, Tabs: []storedWorkspaceTab{}, FileTabs: []storedFileTab{},
		Section: a.section, ActiveSSH: a.activeTab, ActiveFile: a.activeFile,
	}
	groups := map[*sshConnectionGroup]int{}
	connectionID := func(group *sshConnectionGroup) int {
		if group == nil {
			return 0
		}
		if id, exists := groups[group]; exists {
			return id
		}
		id := len(groups) + 1
		groups[group] = id
		stored := snapshotWorkspaceSession(group.endpoint, group.endpoint.session, true, 0)
		result.Connections = append(result.Connections, storedWorkspaceConnection{ID: id, Endpoint: stored.Endpoint})
		return id
	}
	for _, tab := range a.tabs {
		result.Tabs = append(result.Tabs, storedWorkspaceTab{
			Root:    snapshotWorkspaceLayout(tab.root, connectionID),
			Focused: slices.Index(tab.root.sessions(), tab.focused), Maximized: tab.maximized,
		})
	}
	for _, session := range a.fileSessions {
		result.FileTabs = append(result.FileTabs, storedFileTab{
			ID:        session.restoreID,
			Session:   snapshotWorkspaceSession(session.endpoint, session.config, session.temporary, connectionID(session.connection)),
			LocalPath: session.local.path, RemotePath: session.remote.path,
		})
	}
	return result
}

func snapshotWorkspaceSession(endpoint host, config sessionConfig, temporary bool, connection int) storedWorkspaceSession {
	endpoint.session = config
	endpoint = cloneHostEndpoint(endpoint)
	stored := storedSession{
		sessionConfig: endpoint.session, Protocol: endpoint.protocol, Name: endpoint.name,
		Host: endpoint.address, Port: endpoint.port, User: endpoint.user, OS: endpoint.os, System: endpoint.system,
	}
	profile := ""
	if !temporary {
		profile = endpoint.session.ID
	}
	return storedWorkspaceSession{ProfileID: profile, Endpoint: stored, Temporary: temporary, Connection: connection}
}

func snapshotWorkspaceLayout(node *terminalLayout, connectionID func(*sshConnectionGroup) int) *storedWorkspaceLayout {
	if node.session != nil {
		session := node.session
		stored := snapshotWorkspaceSession(session.endpoint, session.config, session.temporary, connectionID(session.connection))
		return &storedWorkspaceLayout{Session: &stored}
	}
	return &storedWorkspaceLayout{
		First: snapshotWorkspaceLayout(node.first, connectionID), Second: snapshotWorkspaceLayout(node.second, connectionID),
		Stacked: node.stacked, Ratio: node.ratio,
	}
}

func (a *app) restoreWorkspace() {
	if a.prefs.RestoreSessions && a.savedWorkspace != nil {
		a.restoreWorkspaceTabs(a.savedWorkspace)
	}
}

func (a *app) restoreWorkspaceTabs(saved *storedWorkspace) {
	groups := map[int]*sshConnectionGroup{}
	for _, connection := range saved.Connections {
		value := connection.Endpoint
		endpoint := host{
			name: value.Name, user: value.User, address: value.Host, port: value.Port,
			os: value.OS, system: value.System, protocol: value.Protocol, session: value.sessionConfig,
		}
		groups[connection.ID] = a.newSSHGroup(endpoint)
	}
	restore := func(stored storedWorkspaceSession) (hostID, host, bool, *sshConnectionGroup) {
		value := stored.Endpoint
		endpoint := host{
			name: value.Name, user: value.User, address: value.Host, port: value.Port,
			os: value.OS, system: value.System, protocol: value.Protocol, session: value.sessionConfig,
		}
		initializeSession(&endpoint)
		id := hostID(slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == stored.ProfileID }))
		temporary := stored.Temporary || id < 0
		var group *sshConnectionGroup
		if stored.Connection > 0 {
			group = groups[stored.Connection]
		}
		return id, endpoint, temporary, group
	}
	var layout func(*storedWorkspaceLayout) *terminalLayout
	layout = func(stored *storedWorkspaceLayout) *terminalLayout {
		if stored.Session != nil {
			id, endpoint, temporary, group := restore(*stored.Session)
			return &terminalLayout{session: a.newDisconnectedSession(id, endpoint, temporary, group)}
		}
		return &terminalLayout{
			first: layout(stored.First), second: layout(stored.Second), stacked: stored.Stacked, ratio: stored.Ratio,
		}
	}
	for _, stored := range saved.Tabs {
		root := layout(stored.Root)
		a.nextTab++
		a.tabs = append(a.tabs, &workspaceTab{
			id: a.nextTab, root: root, focused: root.sessions()[stored.Focused], maximized: stored.Maximized,
		})
	}
	for _, stored := range saved.FileTabs {
		id, endpoint, temporary, group := restore(stored.Session)
		session := a.newFileSession(id, endpoint, temporary, group)
		if stored.ID != "" {
			session.restoreID = stored.ID
		}
		if stored.LocalPath != "" {
			session.local.path, session.local.history = stored.LocalPath, []string{stored.LocalPath}
			session.local.historyIndex = 0
		}
		session.remote.path, session.remote.history, session.remote.historyIndex = "", []string{}, -1
		if stored.RemotePath != "" {
			session.remote.path, session.remote.history = stored.RemotePath, []string{stored.RemotePath}
			session.remote.historyIndex = 0
		}
		a.fileSessions = append(a.fileSessions, session)
	}
	a.activeTab, a.activeFile, a.section = saved.ActiveSSH, saved.ActiveFile, saved.Section
	if len(a.tabs) > 0 {
		// Let Enter reach the active pane's connect prompt without a click first.
		a.tabs[a.activeTab].focused.focusPending = true
	}
}

func validateStoredWorkspace(saved *storedWorkspace, hosts []host, keys []managedKey) error {
	if saved == nil {
		return nil
	}
	if saved.Version != 2 {
		return errors.New("unsupported workspace version")
	}
	switch saved.Section {
	case sectionSSH, sectionFiles, sectionMCP:
	default:
		return errors.New("invalid workspace section")
	}
	sshCount, fileCount := len(saved.Tabs), len(saved.FileTabs)
	if saved.ActiveSSH < 0 || saved.ActiveSSH >= max(1, sshCount) || saved.ActiveFile < 0 || saved.ActiveFile >= max(1, fileCount) {
		return errors.New("invalid active workspace tab")
	}
	profiles := map[string]host{}
	for _, profile := range hosts {
		profiles[profile.session.ID] = profile
	}
	keyIDs := map[string]bool{}
	for _, key := range keys {
		keyIDs[key.ID] = true
	}
	validate := func(stored storedWorkspaceSession, files bool) error {
		h := stored.Endpoint
		if stored.Connection < 0 || strings.TrimSpace(h.Name) == "" || strings.TrimSpace(h.Host) == "" ||
			h.Port < 1 || h.Port > 65535 || h.OS < 0 || h.OS >= len(systems) {
			return errors.New("invalid saved workspace endpoint")
		}
		protocol := h.Protocol
		if protocol == "ssh" {
			protocol = ""
		}
		if files && protocol != "sftp" && protocol != "ftp" || !files && protocol != "" {
			return errors.New("workspace endpoint protocol does not match its view")
		}
		if stored.ProfileID != "" && stored.ProfileID != h.ID {
			return errors.New("workspace profile id differs from its endpoint")
		}
		if err := validateSessionProfile(host{protocol: protocol, session: h.sessionConfig}, profiles, keyIDs); err != nil {
			return err
		}
		return nil
	}
	connections := map[int]bool{}
	for _, connection := range saved.Connections {
		if connection.ID <= 0 || connections[connection.ID] {
			return errors.New("invalid or duplicate workspace connection id")
		}
		connections[connection.ID] = true
		files := connection.Endpoint.Protocol == "sftp"
		if connection.Endpoint.Protocol == "ftp" {
			return errors.New("FTP cannot use a shared SSH connection")
		}
		if err := validate(storedWorkspaceSession{Endpoint: connection.Endpoint, Temporary: true}, files); err != nil {
			return err
		}
	}
	validateConnection := func(stored storedWorkspaceSession) error {
		if stored.Connection > 0 && !connections[stored.Connection] {
			return errors.New("workspace session references an unknown connection")
		}
		if stored.Endpoint.Protocol == "ftp" && stored.Connection != 0 {
			return errors.New("FTP workspace has an SSH connection reference")
		}
		return nil
	}
	for _, tab := range saved.Tabs {
		leaves, nodes := 0, 0
		var walk func(*storedWorkspaceLayout) error
		walk = func(node *storedWorkspaceLayout) error {
			nodes++
			if node == nil || nodes > 2*maxTerminalPanes-1 {
				return errors.New("invalid workspace layout tree")
			}
			if node.Session != nil {
				if node.First != nil || node.Second != nil {
					return errors.New("workspace leaf contains child layouts")
				}
				leaves++
				if err := validateConnection(*node.Session); err != nil {
					return err
				}
				return validate(*node.Session, false)
			}
			if !(node.Ratio > 0 && node.Ratio < 1) {
				return errors.New("invalid workspace split ratio")
			}
			if err := walk(node.First); err != nil {
				return err
			}
			return walk(node.Second)
		}
		if err := walk(tab.Root); err != nil {
			return err
		}
		if leaves > maxTerminalPanes || tab.Focused < 0 || tab.Focused >= leaves {
			return errors.New("invalid workspace focus or pane count")
		}
	}
	fileIDs := map[string]bool{}
	for _, file := range saved.FileTabs {
		if saved.Version >= 2 && file.ID == "" {
			return errors.New("missing file workspace identity")
		}
		if file.ID != "" {
			if strings.TrimSpace(file.ID) != file.ID || fileIDs[file.ID] {
				return errors.New("invalid or duplicate file workspace identity")
			}
			fileIDs[file.ID] = true
		}
		if err := validateConnection(file.Session); err != nil {
			return err
		}
		if err := validate(file.Session, true); err != nil {
			return fmt.Errorf("file workspace: %w", err)
		}
		if strings.ContainsAny(file.LocalPath+file.RemotePath, "\x00") {
			return errors.New("workspace path contains NUL")
		}
	}
	return nil
}

type workspaceSaveState struct {
	last    []byte
	dirtyAt time.Time
}

func (a *app) tickWorkspace(c *ui.Context) {
	if a.catalogConfig.loadErr != nil || a.catalogSaving || a.quitPending || a.transfers.quitting {
		return
	}
	saved := a.workspaceSnapshot()
	data, err := json.Marshal(saved)
	if err != nil {
		a.persistenceNotice("保存工作区失败", err)
		return
	}
	if !bytes.Equal(data, a.workspaceSave.last) {
		a.workspaceSave.last = data
		a.workspaceSave.dirtyAt = c.Now()
	}
	if a.workspaceSave.dirtyAt.IsZero() {
		return
	}
	remaining := 300*time.Millisecond - c.Now().Sub(a.workspaceSave.dirtyAt)
	if remaining > 0 {
		c.After(remaining)
		return
	}
	a.workspaceSave.dirtyAt = time.Time{}
	if err := validateStoredWorkspace(saved, a.hosts, a.keys); err != nil {
		a.persistenceNotice("保存工作区失败", err)
		return
	}
	value, err := encodeWorkspace(saved)
	if err != nil {
		a.persistenceNotice("保存工作区失败", err)
		return
	}
	a.persist(func() error { return a.store.SaveWorkspace(a.ctx, value) }, func(err error) {
		if err == nil {
			a.savedWorkspace = saved
		} else {
			a.persistenceNotice("保存工作区失败", err)
		}
	})
}

func (a *app) persistBeforeClose(done func(error)) {
	var workspace *store.Workspace
	settings := map[string][]byte{}
	if a.catalogConfig.loadErr == nil {
		saved := a.workspaceSnapshot()
		if err := validateStoredWorkspace(saved, a.hosts, a.keys); err != nil {
			done(err)
			return
		}
		value, err := encodeWorkspace(saved)
		if err != nil {
			done(err)
			return
		}
		workspace = &value
	}
	if a.prefsLoadErr == nil {
		data, err := a.preferencesData()
		if err != nil {
			done(err)
			return
		}
		settings["preferences"] = data
	}
	if a.filePrefs.loadErr == nil {
		data, err := a.filePreferencesData()
		if err != nil {
			done(err)
			return
		}
		settings["files"] = data
	}
	a.persist(func() error { return a.store.SaveDesktopState(a.ctx, workspace, settings) }, done)
}

func (a *app) savingSession() bool {
	for i := range a.pages {
		if a.pages[i].draft.saving {
			return true
		}
	}
	return false
}

func (a *app) enforceScrollback() {
	limit := a.prefs.ScrollbackLines
	if limit == 0 {
		return
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			// This pass also runs while settings/files or another tab is visible.
			// Apply the limit in place; rendering must not own terminal lifetime.
			if live := session.terminal; live != nil && live.scrollbackLines != limit {
				if err := live.view.SetScrollbackLines(limit); err != nil {
					a.terminalError = fmt.Errorf("更新终端回滚行数：%w", err)
					return
				}
				live.scrollbackLines = limit
			}
			if len(session.output) > limit {
				// Copy to release strings retained by the discarded backing array.
				session.output = append([]terminalLine(nil), session.output[len(session.output)-limit:]...)
				session.transcript = nil
			}
		}
	}
}
