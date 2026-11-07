package desktop

import (
	"encoding/json"
	"maps"
	"time"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type filePreferencesState struct {
	save    preferenceSaveState
	loadErr error
}

type filePreferences struct {
	LocalPaneOpen         bool              `json:"localPaneOpen"`
	LocalRatio            float32           `json:"localRatio"`
	LocalPaths            map[string]string `json:"localPaths"`
	TransferDockCollapsed bool              `json:"transferDockCollapsed"`
	TransferFilter        int               `json:"transferFilter"`
	TransferDockHeight    float32           `json:"transferDockHeight,omitempty"`
}

func (a *app) fileSiteKey(session *fileSession) string {
	if session.temporary {
		return ""
	}
	return session.endpoint.session.ID
}

func (a *app) restoreFileSession(session *fileSession) {
	// New tabs use configured directories; workspace restoration reapplies saved paths separately.
	local := a.prefs.DefaultLocalDir
	if options := session.config.Files; options != nil {
		if options.LocalDir != "" {
			local = options.LocalDir
		}
		if options.RemoteDir != "" {
			session.remote.path = options.RemoteDir
			session.remote.history = []string{options.RemoteDir}
			session.remote.historyIndex = 0
		}
	}
	if local != "" {
		session.local.path = local
		session.local.history = []string{local}
		session.local.historyIndex = 0
	}
}

func (a *app) refreshFileInitialDirectories(session *fileSession, previous fileOptions) {
	options := session.config.Files
	reset := func(pane *filePaneState, location string) {
		pane.path, pane.history, pane.historyIndex = location, nil, -1
		pane.directories = map[string][]fileEntry{}
		pane.query, pane.errorText, pane.reveal, pane.hoverFolder = "", locales.Text{}, "", ""
		pane.table.Clear()
		pane.table.List.ScrollTo(0, 0)
		pane.scroll = ui.ScrollState{Handle: pane.scroll.Handle}
	}
	if options.RemoteDir != previous.RemoteDir {
		reset(session.remote, options.RemoteDir)
	}
	if options.LocalDir != previous.LocalDir {
		// Cancel an outstanding listing before the old runtime can publish its path.
		a.closeLocalFiles(session)
		location := options.LocalDir
		if location == "" {
			location = a.prefs.DefaultLocalDir
		}
		if location == "" {
			location = "~"
		}
		reset(session.local, location)
		if key := a.fileSiteKey(session); key != "" {
			a.files.localPaths[key] = location
		}
	}
}

func (a *app) loadFilePreferences() error {
	err := a.readFilePreferences()
	a.filePrefs.loadErr = err
	return err
}

func (a *app) readFilePreferences() error {
	var data []byte
	found, err := a.store.ReadSetting(a.ctx, "files", &data)
	if err != nil || !found {
		return err
	}
	prefs := filePreferences{LocalRatio: theme.Current().C.FTPSplit.DefaultRatio}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return err
	}
	if prefs.LocalRatio <= 0 || prefs.LocalRatio >= 1 || prefs.TransferFilter < 0 || prefs.TransferFilter > 2 || prefs.TransferDockHeight < 0 {
		return locales.Errorf(locales.MessageInvalidFilePaneSettings)
	}
	a.files.localOpen, a.files.ratio = prefs.LocalPaneOpen, prefs.LocalRatio
	a.files.localPaths = maps.Clone(prefs.LocalPaths)
	if a.files.localPaths == nil {
		a.files.localPaths = map[string]string{}
	}
	a.transfers.collapsed, a.transfers.filter = prefs.TransferDockCollapsed, prefs.TransferFilter
	a.transfers.dockHeight = prefs.TransferDockHeight
	return nil
}

func (a *app) saveFilePreferences(c *ui.Context) {
	if err := a.writeFilePreferences(); err != nil {
		uikit.Toast(
			c,
			uikit.ToastProps{ID: locales.MessageUnableToSaveFileTransferPreferences() + err.Error(), Tone: uikit.ToneDanger, Title: locales.Deferred(
				locales.MessageUnableToSaveFileTransferPreferences2,
			), Detail: locales.ErrorText(
				err,
			)},
		)
	}
}

func (a *app) writeFilePreferences() error {
	if a.filePrefs.loadErr != nil {
		return locales.Errorf(locales.MessageTheOriginalConfigurationCouldNotBeReadOverwritingBlocked, a.filePrefs.loadErr)
	}
	a.filePrefs.save = preferenceSaveState{dirty: true}
	if a.window != nil {
		a.window.Invalidate()
	}
	return nil
}

func (a *app) tickFilePreferences(c *ui.Context) {
	save := &a.filePrefs.save
	if !save.dirty {
		return
	}
	if save.dirtyAt.IsZero() {
		save.dirtyAt = c.Now()
	}
	remaining := 300*time.Millisecond - c.Now().Sub(save.dirtyAt)
	if remaining > 0 {
		c.After(remaining)
		return
	}
	save.dirty = false
	data, err := a.filePreferencesData()
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveFileTransferPreferences2, err)
		return
	}
	a.persist(
		func() error { return a.store.SaveSetting(a.ctx, "files", data) },
		func(err error) { a.persistenceNotice(locales.MessageUnableToSaveFileTransferPreferences2, err) },
	)
}

func (a *app) filePreferencesData() ([]byte, error) {
	if a.filePrefs.loadErr != nil {
		return nil, locales.Errorf(
			locales.MessageTheOriginalConfigurationCouldNotBeReadOverwritingBlocked,
			a.filePrefs.loadErr,
		)
	}
	prefs := filePreferences{LocalPaneOpen: a.files.localOpen, LocalRatio: a.files.ratio, LocalPaths: a.files.localPaths,
		TransferDockCollapsed: a.transfers.collapsed, TransferFilter: a.transfers.filter, TransferDockHeight: a.transfers.dockHeight}
	return json.Marshal(prefs)
}
