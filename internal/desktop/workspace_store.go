package desktop

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/persistence"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/workspace"
)

type storedWorkspace = workspace.Snapshot
type storedWorkspaceConnection = workspace.Connection
type storedWorkspaceTab = workspace.SavedTab
type storedWorkspaceSession = workspace.Session
type storedFileTab = workspace.FileTab

func (a *app) workspaceSnapshot() *storedWorkspace {
	result := &storedWorkspace{
		Version: 2, Tabs: []storedWorkspaceTab{}, FileTabs: []storedFileTab{},
		Section: a.section, ActiveFile: a.activeFile,
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
	for index, tab := range a.tabs.Items {
		// An agent's record belongs to this run; the saved active tab counts
		// only the tabs kept, and falls back to the last of them.
		if tab.Focused.agent != nil {
			continue
		}
		if index <= a.tabs.Active {
			result.ActiveSSH = len(result.Tabs)
		}
		result.Tabs = append(result.Tabs, workspace.SaveTab(tab.paneTab, func(s *remoteSession) *workspace.Session {
			stored := snapshotWorkspaceSession(s.endpoint, s.config, s.temporary, connectionID(s.connection))
			return &stored
		}))
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
		Config: endpoint.session, Protocol: endpoint.protocol, Name: endpoint.name,
		Host: endpoint.address, Port: endpoint.port, User: endpoint.user, OS: endpoint.os, System: endpoint.system,
	}
	profile := ""
	if !temporary {
		profile = endpoint.session.ID
	}
	return storedWorkspaceSession{ProfileID: profile, Endpoint: stored, Temporary: temporary, Connection: connection}
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
			os: value.OS, system: value.System, protocol: value.Protocol, session: value.Config,
		}
		groups[connection.ID] = a.newSSHGroup(endpoint)
	}
	restore := func(stored storedWorkspaceSession) (hostID, host, bool, *sshConnectionGroup) {
		value := stored.Endpoint
		endpoint := host{
			name: value.Name, user: value.User, address: value.Host, port: value.Port,
			os: value.OS, system: value.System, protocol: value.Protocol, session: value.Config,
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
	for _, stored := range saved.Tabs {
		tab := workspace.RestoreTab(stored, func(s *workspace.Session) *remoteSession {
			id, endpoint, temporary, group := restore(*s)
			return a.newDisconnectedSession(id, endpoint, temporary, group)
		})
		a.nextTab++
		a.tabs.Items = append(a.tabs.Items, &workspaceTab{id: a.nextTab, paneTab: tab})
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
	a.tabs.Active, a.activeFile, a.section = saved.ActiveSSH, saved.ActiveFile, saved.Section
	if len(a.tabs.Items) > 0 {
		// Let Enter reach the active pane's connect prompt without a click first.
		a.tabs.Items[a.tabs.Active].Focused.focusPending = true
	}
}

func validateStoredWorkspace(saved *storedWorkspace, keys []managedKey) error {
	keyIDs := map[string]bool{}
	for _, key := range keys {
		keyIDs[key.ID] = true
	}
	return saved.Validate(keyIDs)
}

type workspaceSaveState struct {
	last    []byte
	dirtyAt time.Time
}

func (a *app) tickWorkspace(c *ui.Context) {
	if a.catalogConfig.loadErr != nil || a.catalogSaving || a.quitPending || a.transfers.Quitting {
		return
	}
	saved := a.workspaceSnapshot()
	data, err := json.Marshal(saved)
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveWorkspace, err)
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
	if err := validateStoredWorkspace(saved, a.keys); err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveWorkspace, err)
		return
	}
	value, err := persistence.EncodeWorkspace(saved)
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveWorkspace, err)
		return
	}
	a.persist(func() error { return a.store.SaveWorkspace(a.ctx, value) }, func(err error) {
		if err == nil {
			a.savedWorkspace = saved
		} else {
			a.persistenceNotice(locales.MessageUnableToSaveWorkspace, err)
		}
	})
}

func (a *app) persistBeforeClose(done func(error)) {
	var workspace *store.Workspace
	settings := map[string][]byte{}
	if a.catalogConfig.loadErr == nil {
		saved := a.workspaceSnapshot()
		if err := validateStoredWorkspace(saved, a.keys); err != nil {
			done(err)
			return
		}
		value, err := persistence.EncodeWorkspace(saved)
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
	for _, tab := range a.tabs.Items {
		for _, session := range tab.Root.Sessions() {
			// This pass also runs while settings/files or another tab is visible.
			// Apply the limit in place; rendering must not own terminal lifetime.
			if live := session.terminal; live != nil && live.scrollbackLines != limit {
				if err := live.view.SetScrollbackLines(limit); err != nil {
					a.terminalError = locales.Errorf(locales.MessageUpdateTerminalScrollback, err)
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
