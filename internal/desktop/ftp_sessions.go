package desktop

import (
	"fmt"
	"slices"
	"time"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

type fileSession struct {
	endpoint       host
	temporary      bool
	connection     *sshConnectionGroup
	gatewayErr     error
	gatewayScope   string
	runtime        *fileRuntime
	localRuntime   *fileRuntime
	sshPeer        *remoteSession
	ftpSecret      string
	config         sessionConfig
	id             int
	restoreID      string
	transferScroll ui.ScrollState
	host           hostID
	state          connectionState
	errorText      locales.Text
	connectedAt    time.Time
	local          *filePaneState
	remote         *filePaneState
}

func (a *app) currentFileSession() *fileSession {
	if len(a.fileSessions) == 0 {
		return nil
	}
	return a.fileSessions[a.activeFile]
}

func (a *app) activateFileTab(index int) {
	session := a.fileSessions[index]
	if index != a.activeFile && a.deferWorkspaceDragAction(func() { a.activateFileTab(slices.Index(a.fileSessions, session)) }) {
		return
	}
	a.selectFileTab(index)
	a.focusWorkspace()
}

func (a *app) selectFileTab(index int) {
	session := a.fileSessions[index]
	if index != a.activeFile && a.deferWorkspaceDragAction(func() { a.selectFileTab(slices.Index(a.fileSessions, session)) }) {
		return
	}
	a.settingsUI.previousFocus = ui.Handle{}
	a.closeSettings()
	a.activeFile = index
}

func (a *app) openCatalogHost(id hostID) {
	if a.section == sectionFiles {
		a.openFileHost(id)
		return
	}
	a.openHost(id)
}

func (a *app) openFileHost(id hostID) {
	a.closeSettings()
	a.openNewFileHost(id)
}

func (a *app) newFileSession(id hostID, endpoint host, temporary bool, connection *sshConnectionGroup) *fileSession {
	var gatewayErr error
	endpoint = cloneHostEndpoint(endpoint)
	if endpoint.session.Files == nil {
		endpoint.session.Files = &fileOptions{Encoding: "UTF-8"}
	}
	a.nextFileSession++
	if connection == nil && endpoint.protocol != "ftp" {
		connection = a.newSSHGroup(endpoint)
	}
	if connection != nil && connection.planErr == nil {
		route := connection.plan.route
		endpoint.session.GatewayRoute = &route
	} else if endpoint.protocol == "ftp" && endpoint.session.GatewayRoute == nil {
		endpoint, gatewayErr = a.freezeGatewayRoute(endpoint)
	}
	session := &fileSession{
		id: a.nextFileSession, restoreID: newSessionID(), host: id, endpoint: endpoint, config: endpoint.session,
		temporary: temporary, connection: connection, gatewayErr: gatewayErr,
		local: newFilePane(false), remote: newFilePane(true),
	}
	a.restoreFileSession(session)
	return session
}

func (a *app) openNewFileHost(id hostID) {
	session := a.newFileSession(id, a.hosts[id], false, nil)
	a.fileSessions = append(a.fileSessions, session)
	a.activeFile = len(a.fileSessions) - 1
	a.startFileSession(session, false)
	if a.activeSection() == sectionFiles {
		a.focusWorkspace()
	}
}

func (a *app) closeFileSession(index int) {
	a.disconnectFileSession(a.fileSessions[index])
	a.closeLocalFiles(a.fileSessions[index])
	closedActive := index == a.activeFile
	a.fileSessions = slices.Delete(a.fileSessions, index, index+1)
	switch {
	case len(a.fileSessions) == 0:
		a.activeFile = 0
	case index < a.activeFile:
		a.activeFile--
	case a.activeFile >= len(a.fileSessions):
		a.activeFile = len(a.fileSessions) - 1
	}
	if closedActive && len(a.fileSessions) > 0 && a.activeSection() == sectionFiles {
		a.focusWorkspace()
	}
}

func (a *app) fileTitleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	uikit.TitleBar(c).PaddingEnd(t.C.Window.Gutter).Children(func() {
		uikit.TitleBarLeading(c, t.C.Rail.Width+(a.page().sidebarWidth+t.C.Window.Gutter)*shown).Children(func() {
			label := locales.MessageCollapseSiteSidebar()
			if a.page().sidebarCollapsed {
				label = locales.MessageExpandSiteSidebar()
			}
			uikit.IconButton(c, sidebarIcon, uikit.IconButtonProps{Key: "sidebar-toggle", Label: label, Variant: uikit.IconOnWindow, Disabled: a.activeSection() == sectionSettings}).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		tabs := uikit.SessionTabs(c, nil, nil, uikit.SessionTabsProps{
			Scroll: &a.fileTabScroll, ScrollKey: "file-session-tabs", Motion: uikit.MotionOverride(a.reduceMotion(c)),
			BuildTabs: func(list ui.Element) {
				var closing *fileSession
				for index, session := range a.fileSessions {
					if a.fileTabButton(c, list, index, session) {
						closing = session
					}
				}
				if closing != nil {
					a.requestCloseFileTab(slices.Index(a.fileSessions, closing))
				}
			},
			BuildAdd: func() {
				var menu []uikit.MenuEntry
				for _, group := range a.catalogGroups() {
					entry := uikit.MenuEntry{ID: fmt.Sprintf("group-%d", group.id), Label: group.name}
					for _, id := range group.hosts {
						entry.Children = append(entry.Children, uikit.MenuEntry{ID: fmt.Sprintf("host-%d", id), Label: a.hosts[id].name,
							OnChoose: func() { a.openFileHost(id) }})
					}
					entry.Disabled = len(entry.Children) == 0
					menu = append(menu, entry)
				}
				add := uikit.IconButton(
					c,
					plusIcon,
					uikit.IconButtonProps{Key: locales.MessageNewFileSession(), Label: locales.MessageNewFileSession(), Variant: uikit.IconOnWindow},
				)
				uikit.MenuOn(c, add, menu)
			},
		})
		if tabs.Select >= 0 {
			a.selectFileTab(tabs.Select)
		}
	})
}

func (a *app) fileTabButton(c *ui.Context, list ui.Element, index int, session *fileSession) bool {
	h := session.endpoint
	selected := a.activeFile == index
	hint := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = locales.MessageConnectingTo() + h.address + "…"
	case disconnected:
		hint = locales.MessageDisconnectedSelectToReconnect()
	case connectionFailed:
		hint = session.errorText.String()
	}
	props := uikit.SessionTabProps{
		Key: session, Title: h.name, Hint: hint, Selected: selected, Variant: uikit.SessionTabFiles,
		State: connectionIndicatorState(session.state), CloseLabel: locales.MessageCloseFileTab() + h.name, CloseIcon: closeIcon,
		ReduceMotion: a.reduceMotion(c),
	}
	button := uikit.SessionTab(c, props)
	input := uikit.SessionTabInput(c, list, button, index, len(a.fileSessions))
	if input.Select >= 0 {
		a.selectFileTab(input.Select)
	}
	button.OnClick(func() {
		a.activateFileTab(slices.Index(a.fileSessions, session))
	})
	closing := input.Close
	menu := []uikit.MenuEntry{
		{ID: "edit", Label: locales.MessageEditSession(), Icon: uikit.IconEdit,
			Disabled: session.temporary || session.host < 0, OnChoose: func() { a.startEditHostDraft(session.host) }},
		{ID: "after-edit", Separator: true},
	}
	if session.state == connected || session.state == connecting {
		menu = append(menu, uikit.MenuEntry{
			ID: "disconnect", Label: locales.MessageDisconnectFileSession(), Icon: uikit.IconPause,
			OnChoose: func() { a.disconnectFileSession(session) },
		})
	} else {
		menu = append(menu, uikit.MenuEntry{
			ID: "reconnect", Label: locales.MessageReconnect(), Icon: uikit.IconRetry,
			OnChoose: func() { a.reconnectFileSession(session) },
		})
	}
	menu = append(menu, uikit.MenuEntry{ID: "before-close", Separator: true}, uikit.MenuEntry{
		ID: "close", Label: locales.MessageCloseTab(), Icon: uikit.IconClose, Danger: true,
		OnChoose: func() { closing = true },
	})
	button.Children(func() {
		props.State = connectionIndicatorState(session.state)
		parts := uikit.SessionTabContents(c, button, props)
		parts.Close.OnClick(func() { a.requestCloseFileTab(slices.Index(a.fileSessions, session)) })
		uikit.ContextMenuOn(
			c,
			button,
			menu,
			parts.Close,
		)
	})
	return closing
}

func connectionIndicatorState(state connectionState) uikit.ConnState {
	return [...]uikit.ConnState{
		disconnected:     uikit.ConnDisconnected,
		connected:        uikit.ConnConnected,
		connecting:       uikit.ConnConnecting,
		connectionFailed: uikit.ConnError,
	}[state]
}

func (a *app) catalogHostIcon(c *ui.Context, id hostID) {
	h := a.hosts[id]
	if a.section == sectionFiles {
		protocol := uikit.ProtocolSFTP
		if h.protocol == "ftp" {
			protocol = uikit.ProtocolFTP
		}
		uikit.ProtocolIcon(c, protocol, uikit.SizeDefault)
		return
	}
	style, _ := a.hostSystem(h)
	uikit.OsLogo(c, style.name, uikit.SizeDefault)
}

func (a *app) deleteFileSite(id hostID, done func(error)) {
	deleted := a.hosts[id]
	for _, g := range a.gateways {
		if g.SSHProfileID == deleted.session.ID {
			done(locales.Errorf(locales.MessageThisHostIsReferencedByGatewayUpdateTheGatewayFirst, g.Name))
			return
		}
	}
	for _, group := range a.runtimeSSHGroups() {
		for _, hop := range group.plan.route {
			if hop.SSH != nil && hop.SSH.ID == deleted.session.ID {
				done(locales.Errorf(locales.MessageThisHostIsUsedByAGatewayConnectionCloseRelatedSessionsAndTransfers))
				return
			}
		}
	}
	previousWorkspace := a.savedWorkspace
	a.savedWorkspace = a.workspaceSnapshot()
	changes, err := a.vault.Prepare(nil, []string{sessionCredentialKey(deleted.session.ID)})
	if err != nil {
		done(err)
		return
	}
	a.commitCatalogSecretsAsync(changes, func() {
		a.hosts = slices.Delete(a.hosts, int(id), int(id)+1)
		for _, group := range a.groups {
			group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
			for i, candidate := range group.hosts {
				if candidate > id {
					group.hosts[i]--
				}
			}
		}
	}, func(err error) {
		if err != nil {
			a.savedWorkspace = previousWorkspace
			done(err)
			return
		}

		// Hidden page drafts still refer to the compacted host array.
		for i := range a.pages {
			page := &a.pages[i]
			if !page.draft.editing && !page.draft.deleting {
				continue
			}
			if page.draft.id == id {
				page.draft.cancelTest()
				page.draft, page.addHostOpen = hostDraft{}, false
			} else if page.draft.id > id {
				page.draft.id--
			}
		}
		for i := len(a.fileSessions) - 1; i >= 0; i-- {
			session := a.fileSessions[i]
			if !session.temporary && session.endpoint.session.ID == deleted.session.ID {
				a.closeFileSession(i)
			}
		}
		remainingFiles := map[*fileSession]bool{}
		for _, session := range a.fileSessions {
			remainingFiles[session] = true
		}
		for _, task := range a.transfers.tasks {
			remainingFiles[task.session] = true
		}
		for _, closed := range a.closedTabs {
			if closed.section == sectionFiles {
				remainingFiles[closed.file] = true
			}
		}
		for _, tab := range a.tabs {
			for _, session := range tab.root.sessions() {
				if session.filePeer != nil {
					remainingFiles[session.filePeer] = true
				}
			}
		}
		a.closedTabs = slices.DeleteFunc(a.closedTabs, func(candidate closedWorkspaceTab) bool {
			if candidate.section == sectionFiles {
				return !candidate.file.temporary && candidate.file.endpoint.session.ID == deleted.session.ID
			}
			return !candidate.temporary && candidate.endpoint.session.ID == deleted.session.ID
		})
		for session := range remainingFiles {
			if !session.temporary && session.endpoint.session.ID == deleted.session.ID {
				session.host, session.temporary = -1, true
			} else if session.host > id {
				session.host--
			}
		}
		for i := len(a.tabs) - 1; i >= 0; i-- {
			tab := a.tabs[i]
			for _, session := range tab.root.sessions() {
				if !session.temporary && session.endpoint.session.ID == deleted.session.ID {
					releaseSSH(session)
					tab.root = tab.root.remove(session)
					if tab.focused == session && tab.root != nil {
						tab.focused = tab.root.firstSession()
					}
				} else if session.host > id {
					session.host--
				}
			}
			if tab.root == nil {
				a.closeTab(i)
			}
		}
		for i, candidate := range a.closedTabs {
			if candidate.section != sectionFiles && candidate.host > id {
				a.closedTabs[i].host--
			}
		}
		done(nil)
	})
}

func (a *app) deleteSiteDialogContents(c *ui.Context, parts uikit.ModalParts) bool {
	t := theme.Current()
	var confirm bool
	uikit.Text(
		c,
		locales.MessageRemoveThisEntryFromTheCurrentCatalogAndCloseItsSessionsServerFiles(),
		uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
	)
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
		confirm = uikit.Button(c, locales.CommonDelete(), uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked()
	})
	return confirm
}

func (a *app) confirmDeleteSite(c *ui.Context, id hostID) {
	if a.fileSiteBusy(id) {
		uikit.Notify(c, locales.MessageCancelOrRemoveThisSessionSTransfersFirst())
		return
	}
	page := a.page()
	a.deleteFileSite(id, func(err error) {
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToDeleteSession, err)
			return
		}
		page.draft.modal.Close()
	})
}
