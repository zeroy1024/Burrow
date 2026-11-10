package desktop

import (
	"fmt"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

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

func (a *app) fileSiteDeleteError(id hostID) error {
	deleted := a.hosts[id]
	for _, g := range a.gateways {
		if g.SSHProfileID == deleted.session.ID {
			return locales.Errorf(locales.MessageThisHostIsReferencedByGatewayUpdateTheGatewayFirst, g.Name)
		}
	}
	for _, group := range a.runtimeSSHGroups() {
		for _, hop := range group.plan.route {
			if hop.SSH != nil && hop.SSH.ID == deleted.session.ID {
				return locales.Errorf(locales.MessageThisHostIsUsedByAGatewayConnectionCloseRelatedSessionsAndTransfers)
			}
		}
	}
	return nil
}

func (a *app) deleteFileSite(id hostID, done func(error)) {
	deleted := a.hosts[id]
	if err := a.fileSiteDeleteError(id); err != nil {
		done(err)
		return
	}
	if len(a.siteTransfers(deleted.session.ID)) != 0 {
		done(locales.NewError(locales.MessageCancelOrRemoveThisSessionSTransfersFirst))
		return
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
				if page.draft.deleting && page.draft.saving {
					continue
				}
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
		if a.mcpBackend != nil {
			for _, session := range a.mcpBackend.sessions {
				remainingFiles[session] = true
			}
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
	d := &a.page().draft
	tasks := a.siteTransfers(d.deleteProfileID)
	needsCancellation := slices.ContainsFunc(tasks, transferNeedsCancellation)
	var confirm bool
	uikit.Text(
		c,
		locales.MessageRemoveThisEntryFromTheCurrentCatalogAndCloseItsSessionsServerFiles(),
		uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
	)
	if len(tasks) > 0 {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
			uikit.Text(c, locales.SessionRelatedTransfers(len(tasks)), uikit.TextProps{Style: uikit.TextBodyStrong})
			ui.Spacer(c)
			uikit.Button(c, locales.SessionViewTransfers(), uikit.ButtonProps{Size: uikit.SizeSm, Disabled: d.saving}).OnClick(func() {
				d.viewDeleteTransfers = true
				parts.RequestDismiss()
			})
		})
		list := ui.Scroll(c).FillWidth().MinHeight(0).MaxHeight(parts.Bounds.MaxHeight / 2).TrackScroll(&d.scroll).Children(func() {
			for _, task := range tasks {
				ui.Column(c).Key(task.id).FillWidth().Gap(t.Space.N4).Padding(t.Space.N8, 0).Children(func() {
					ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
						source := locales.SessionManualTransfer()
						if task.mcp != nil {
							source = "MCP"
						}
						uikit.Badge(c, source, uikit.BadgeProps{})
						uikit.Text(c, task.entry.name, uikit.TextProps{}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						state := [...]func() string{
							transferQueued: locales.MessageQueued, transferRunning: locales.MessageRunning,
							transferPaused: locales.MessagePaused, transferDone: locales.MessageCompleted,
							transferFailed: locales.MessageTransferFailed2,
						}[task.state]()
						if task.removeRequested {
							state = locales.MessageCancellationRequested()
						} else if task.cleanupPending {
							state = locales.SessionTransferCleanupPending()
						}
						uikit.Badge(c, state, uikit.BadgeProps{Tone: uikit.ToneMuted})
					})
					uikit.Text(c, task.sourcePath+" → "+task.targetPath, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
					if task.errorText.String() != "" {
						uikit.Text(c, task.errorText.String(), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneDanger})
					}
				})
			}
		})
		uikit.Scrollbars(c, parts.Panel, list, &d.scroll, uikit.ScrollbarProps{Label: locales.MessageTransferListScrollbar()})
	}
	if d.deleteError.String() != "" {
		uikit.Banner(c, d.deleteError.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
	} else if d.saving {
		status := locales.SessionWaitingForTransfers()
		if len(tasks) == 0 {
			status = locales.SessionDeleting()
		}
		uikit.Text(c, status, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
	} else if needsCancellation {
		uikit.Text(c, locales.SessionCancelTransfersNotice(), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
	}
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{Disabled: d.saving}).OnClick(parts.RequestDismiss)
		label := locales.CommonDelete()
		if needsCancellation {
			label = locales.SessionCancelTransfersAndDelete()
		}
		if d.saving {
			label = locales.MessageProcessing2()
		}
		confirm = uikit.Button(c, label, uikit.ButtonProps{Variant: uikit.ButtonDanger, Disabled: d.saving}).Clicked()
	})
	return confirm
}

func (a *app) confirmDeleteSite() {
	page := a.page()
	d := &page.draft
	d.saving, d.deleteError = true, locales.Text{}
	a.beginSiteDeletion(d.deleteProfileID, func(err error) {
		d.saving = false
		if err != nil {
			d.deleteError = locales.ErrorText(err)
			return
		}
		page.draft.modal.Close()
	})
}
