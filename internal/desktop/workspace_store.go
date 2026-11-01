package desktop

import "errors"

type storedWorkspace struct {
	SSH        []string `json:"ssh"`
	Files      []string `json:"files"`
	Section    section  `json:"section"`
	ActiveSSH  int      `json:"activeSSH"`
	ActiveFile int      `json:"activeFile"`
}

func (a *app) workspaceSnapshot() *storedWorkspace {
	result := &storedWorkspace{SSH: []string{}, Files: []string{}, Section: a.section, ActiveSSH: a.activeTab, ActiveFile: a.activeFile}
	for _, tab := range a.tabs {
		result.SSH = append(result.SSH, a.hosts[tab.focused.host].session.ID)
	}
	for _, session := range a.fileSessions {
		result.Files = append(result.Files, a.hosts[session.host].session.ID)
	}
	return result
}

func (a *app) restoreWorkspace() {
	saved := a.savedWorkspace
	if !a.prefs.RestoreSessions {
		return
	}
	if saved == nil {
		for _, id := range []hostID{0, 4, 1} {
			if int(id) < len(a.hosts) && a.hosts[id].protocol == "" {
				a.openHost(id)
			}
		}
		return
	}
	activeSSH, activeFile := saved.ActiveSSH, saved.ActiveFile
	for savedIndex, id := range saved.SSH {
		for i, h := range a.hosts {
			if h.session.ID == id && h.protocol == "" {
				a.openHost(hostID(i))
				disconnectSession(a.currentTab().focused)
				if savedIndex == saved.ActiveSSH {
					activeSSH = len(a.tabs) - 1
				}
				break
			}
		}
	}
	for savedIndex, id := range saved.Files {
		for i, h := range a.hosts {
			if h.session.ID == id && (h.protocol == "sftp" || h.protocol == "ftp") {
				a.openNewFileHost(hostID(i))
				a.currentFileSession().state = disconnected
				if savedIndex == saved.ActiveFile {
					activeFile = len(a.fileSessions) - 1
				}
				break
			}
		}
	}
	if len(a.tabs) > 0 {
		a.activeTab = max(0, min(activeSSH, len(a.tabs)-1))
	}
	if len(a.fileSessions) > 0 {
		a.activeFile = max(0, min(activeFile, len(a.fileSessions)-1))
	}
	if saved.Section == sectionFiles {
		a.section = sectionFiles
	}
}

func (a *app) saveWorkspace() error { a.savedWorkspace = a.workspaceSnapshot(); return a.saveCatalog() }

func (a *app) persistBeforeClose() error {
	var catalogErr, prefsErr error
	// A file rejected at load time is never overwritten. It already has a
	// visible error; it must not trap the user in an unclosable window.
	if a.catalogConfig.loadErr == nil {
		catalogErr = a.saveWorkspace()
	}
	if a.prefsLoadErr == nil {
		prefsErr = a.flushAppPreferences()
	}
	return errors.Join(catalogErr, prefsErr)
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
			if len(session.output) > limit {
				// Copy to release strings retained by the discarded backing array.
				session.output = append([]terminalLine(nil), session.output[len(session.output)-limit:]...)
				session.transcript = nil
			}
		}
	}
}
