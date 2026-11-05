package desktop

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

type closedWorkspaceTab struct {
	file       *fileSession
	endpoint   host
	temporary  bool
	connection *sshConnectionGroup
	host       hostID
	section    section
}

type paneCloseRequest struct {
	tab     *workspaceTab
	session *remoteSession
}

func (a *app) navigationSelected(target section) bool {
	return a.activeSection() == target
}

func (a *app) translucencyEnabled(c *ui.Context) bool {
	return a.prefs.Translucency && !a.reduceTransparency
}

func windowVibrancy() mygo.Vibrancy {
	switch runtime.GOOS {
	case "darwin":
		return mygo.VibrancyUnderWindow
	case "windows":
		return mygo.VibrancyMica
	default:
		return mygo.VibrancyTitlebar
	}
}

func (a *app) syncWindowMaterial() {
	if a.window == nil {
		return
	}
	material := mygo.VibrancyNone
	if a.prefs.Translucency && !a.reduceTransparency {
		material = windowVibrancy()
	}
	if material != a.windowMaterial {
		a.windowMaterial = material
		a.window.SetVibrancy(material)
	}
}

func (a *app) glassRadius(c *ui.Context, radius float32) float32 {
	if !a.translucencyEnabled(c) {
		return 0
	}
	return radius
}

func (a *app) positionAnimation(animation ui.Animation) ui.Animation {
	return uikit.Position(animation, a.prefs.ReduceMotion)
}

func (a *app) actionPressed(c *ui.Context, id string) bool {
	binding := a.shortcutBinding(id)
	if binding == "" {
		return false
	}
	mods, key, ok := bindingToKey(binding)
	return ok && c.Shortcut(mods, key)
}

func (a *app) appShortcuts(c *ui.Context) {
	if a.settingsUI.recording != "" {
		return
	}
	run := func(action func()) {
		if !a.deferWorkspaceDragAction(action) {
			action()
		}
	}
	count, index := len(a.tabs), a.activeTab
	if a.section == sectionFiles {
		count, index = len(a.fileSessions), a.activeFile
	}
	selectTab := func(next int) {
		if next < 0 || next >= count {
			return
		}
		if a.section == sectionFiles {
			a.activateFileTab(next)
		} else {
			tab := a.tabs[next]
			run(func() { a.activateTab(slices.Index(a.tabs, tab)) })
		}
	}
	// These keys always navigate tabs, even when the current tab is split.
	if !a.settingsUI.open && count > 0 {
		if c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab) {
			a.cycleTab(-1)
			return
		}
		if c.Shortcut(ui.Ctrl, ui.KeyTab) {
			a.cycleTab(1)
			return
		}
	}
	if a.actionPressed(c, "app.settings") {
		run(a.openSettings)
	}
	if a.actionPressed(c, "mode.terminal") {
		run(func() { a.switchSection(sectionSSH) })
	}
	if a.actionPressed(c, "mode.files") {
		run(func() { a.switchSection(sectionFiles) })
	}
	if a.actionPressed(c, "session.new") {
		run(func() {
			a.closeSettings()
			if a.section == sectionMCP {
				if a.mcp.activeTab == 1 {
					a.openNewMCPPolicyModal()
				} else {
					a.openNewMCPKeyModal()
				}
				return
			}
			a.startHostDraft()
		})
	}
	if a.settingsUI.open {
		if a.actionPressed(c, "host.search") {
			a.settingsUI.search.Focus()
		}
		return
	}
	if a.section == sectionMCP {
		for n := range 3 {
			if a.actionPressed(c, fmt.Sprintf("tab.select.%d", n+1)) {
				a.mcp.activeTab = n
				return
			}
		}
		return
	}
	selectTarget := selectTab
	targetCount, targetIndex := count, index
	if a.section == sectionSSH {
		if tab := a.currentTab(); tab != nil && tab.root.count() > 1 {
			sessions := tab.root.sessions()
			targetCount, targetIndex = len(sessions), slices.Index(sessions, tab.focused)
			selectTarget = func(next int) {
				if next < 0 || next >= len(sessions) {
					return
				}
				run(func() { a.focusSession(tab, sessions[next]) })
			}
		}
	}
	for n := range 4 {
		if a.actionPressed(c, fmt.Sprintf("tab.select.%d", n+1)) {
			selectTarget(n)
			return
		}
	}
	if targetCount > 0 {
		if a.actionPressed(c, "tab.prev") {
			selectTarget((targetIndex + targetCount - 1) % targetCount)
			return
		}
		if a.actionPressed(c, "tab.next") {
			selectTarget((targetIndex + 1) % targetCount)
			return
		}
	}
	if count > 0 && a.actionPressed(c, "tab.close") {
		if a.section == sectionFiles {
			a.requestCloseFileTab(a.activeFile)
		} else if tab := a.currentTab(); tab.root.count() > 1 {
			session := tab.focused
			run(func() { a.requestClosePane(tab, session) })
		} else {
			a.requestCloseTab(a.activeTab)
		}
	}
	if a.actionPressed(c, "host.search") {
		a.page().sidebarCollapsed = false
		a.page().search.Focus()
	}
	if a.actionPressed(c, "tab.reopen") && len(a.closedTabs) > 0 {
		run(func() {
			closed := a.closedTabs[len(a.closedTabs)-1]
			a.closedTabs = a.closedTabs[:len(a.closedTabs)-1]
			a.switchSection(closed.section)
			if closed.section == sectionFiles {
				// Reopening this tab restores its identity and task ownership;
				// opening the same site from the catalog still creates a new one.
				session := closed.file
				a.fileSessions = append(a.fileSessions, session)
				a.activeFile = len(a.fileSessions) - 1
				a.reconnectFileSession(session)
			} else {
				session := a.newShellSession(closed.host, closed.endpoint, closed.temporary, closed.connection)
				a.nextTab++
				a.tabs = append(a.tabs, &workspaceTab{id: a.nextTab, root: &terminalLayout{session: session}, focused: session})
				a.activateTab(len(a.tabs) - 1)
			}
		})
	}
	// Reconnecting only revives a session; it never drops a live one.
	if a.actionPressed(c, "session.reconnect") {
		if a.section == sectionFiles {
			if session := a.currentFileSession(); session != nil && session.state != connected && session.state != connecting {
				a.reconnectFileSession(session)
			}
		} else if tab := a.currentTab(); a.section == sectionSSH && tab != nil && tab.focused.awaitingConnect() {
			session := tab.focused
			run(func() { a.connectSession(session) })
		}
	}
	if a.actionPressed(c, "sidebar.toggle") {
		a.page().sidebarCollapsed = !a.page().sidebarCollapsed
	}
	if a.section == sectionSSH && a.actionPressed(c, "pane.split") && a.currentTab() != nil {
		tab := a.currentTab()
		session := tab.focused
		run(func() {
			bounds := session.layoutRect
			a.splitSession(session, tab.root.count() > 1 && bounds.H > bounds.W)
		})
	}
}

func (a *app) cycleTab(direction int) {
	count, index := len(a.tabs), a.activeTab
	if a.section == sectionFiles {
		count, index = len(a.fileSessions), a.activeFile
	}
	if count == 0 {
		return
	}
	next := (index + direction + count) % count
	if a.section == sectionFiles {
		a.activateFileTab(next)
		return
	}
	tab := a.tabs[next]
	action := func() { a.activateTab(slices.Index(a.tabs, tab)) }
	if !a.deferWorkspaceDragAction(action) {
		action()
	}
}

func (a *app) installAppMenu() func() {
	settings := &mygo.MenuItem{
		Label: localizeTag(mygo.App.Locale(), "设置…"),
		Click: func(*mygo.MenuItem, *mygo.Window) {
			if !a.businessReady() || a.security.dialog != nil {
				return
			}
			// Native menu callbacks do not pass through the UI modal scope.
			if a.page().addHostOpen || a.page().groupDialogOpen || a.files.dialog != nil ||
				a.files.conflict != nil || a.credentials.draft != nil || a.closeRequest != nil || a.closeFileRequest != nil || a.closePaneRequest != nil {
				return
			}
			if !a.deferWorkspaceDragAction(a.openSettings) {
				a.openSettings()
			}
			a.window.Invalidate()
		},
	}
	cycle := func(direction int) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			if !a.businessReady() || a.security.dialog != nil {
				return
			}
			// AppKit handles Control-Tab before the native view's keyDown.
			// Menu accelerators need their own guard for the UI's modal scope.
			switch {
			case a.settingsUI.open, a.settingsUI.recording != "", a.transfers.quitting:
				return
			case a.page().addHostOpen, a.page().groupDialogOpen:
				return
			case a.files.dialog != nil, a.files.conflict != nil, a.fileSSHLink != nil:
				return
			case a.credentials.draft != nil, len(a.sshPrompts) > 0:
				return
			case a.closeRequest != nil, a.closeFileRequest != nil, a.closePaneRequest != nil:
				return
			}
			a.cycleTab(direction)
			a.window.Invalidate()
		}
	}
	previous := &mygo.MenuItem{
		Label: localizeTag(mygo.App.Locale(), "上一个选项卡"), Accelerator: "Ctrl+Shift+Tab", Click: cycle(-1),
	}
	next := &mygo.MenuItem{
		Label: localizeTag(mygo.App.Locale(), "下一个选项卡"), Accelerator: "Ctrl+Tab", Click: cycle(1),
	}
	tabs := &mygo.MenuItem{
		Label: localizeTag(mygo.App.Locale(), "选项卡"), Submenu: []*mygo.MenuItem{previous, next},
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Burrow", Submenu: []*mygo.MenuItem{
			settings,
			{Type: mygo.MenuItemSeparator}, {Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		tabs,
		{Role: mygo.RoleWindowMenu},
	})
	mygo.App.SetMenu(menu)
	return mygo.App.OnLocaleChanged(func(locale string) {
		settings.SetLabel(localizeTag(locale, "设置…"))
		tabs.SetLabel(localizeTag(locale, "选项卡"))
		previous.SetLabel(localizeTag(locale, "上一个选项卡"))
		next.SetLabel(localizeTag(locale, "下一个选项卡"))
	})
}

func (a *app) requestCloseTab(index int) {
	tab := a.tabs[index]
	if a.deferWorkspaceDragAction(func() { a.requestCloseTab(slices.Index(a.tabs, tab)) }) {
		return
	}
	if a.prefs.ConfirmCloseActive && slices.ContainsFunc(tab.root.sessions(), func(s *remoteSession) bool { return s.state == connected || s.state == connecting }) {
		a.closeRequest, a.closeFileRequest, a.closePaneRequest = tab, nil, nil
		return
	}
	a.closeTab(index)
	a.afterLastTabClosed()
}

func (a *app) requestClosePane(tab *workspaceTab, session *remoteSession) {
	if !tab.canClosePane(session) {
		return
	}
	if a.prefs.ConfirmCloseActive && (session.state == connected || session.state == connecting) {
		a.closePaneRequest = &paneCloseRequest{tab: tab, session: session}
		a.closeRequest, a.closeFileRequest = nil, nil
		return
	}
	a.closePane(tab, session)
}

func (a *app) closeTabDialog(c *ui.Context) {
	open := a.closeRequest != nil || a.closeFileRequest != nil || a.closePaneRequest != nil
	if !open {
		return
	}
	t := theme.Current()
	fileRequest := a.closeFileRequest
	paneRequest := a.closePaneRequest
	var key any = a.closeRequest
	detail := "关闭此选项卡将结束其中的会话。"
	button := "关闭选项卡"
	if fileRequest != nil {
		key, detail = fileRequest, "关闭此选项卡将结束文件会话。"
	} else if paneRequest != nil {
		key, detail, button = paneRequest.session, "关闭此窗格将结束其中的会话。", "关闭窗格"
	}
	closeRequested := false
	result := uikit.Modal(c.Key(key), &open, uikit.ModalProps{
		Title: "关闭活动连接？", Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, detail, uikit.TextProps{})
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			closeRequested = uikit.Button(c, button, uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked()
		})
	})
	if closeRequested || result.Submit {
		tabRequest := a.closeRequest
		a.closeRequest, a.closeFileRequest, a.closePaneRequest = nil, nil, nil
		if paneRequest != nil {
			if slices.Contains(a.tabs, paneRequest.tab) {
				a.closePane(paneRequest.tab, paneRequest.session)
			}
		} else if fileRequest != nil {
			if index := slices.Index(a.fileSessions, fileRequest); index >= 0 {
				a.closedTabs = append(a.closedTabs, closedWorkspaceTab{file: fileRequest, section: sectionFiles})
				a.closeFileSession(index)
				a.afterLastTabClosed()
			}
		} else if index := slices.Index(a.tabs, tabRequest); index >= 0 {
			a.closeTab(index)
			a.afterLastTabClosed()
		}
	}
	if !open {
		a.closeRequest, a.closeFileRequest, a.closePaneRequest = nil, nil, nil
	}
}

func (a *app) afterLastTabClosed() {
	count := len(a.tabs) + len(a.fileSessions)
	if count > 0 || a.window == nil {
		return
	}
	switch a.prefs.LastTabClose {
	case "closeWindow":
		a.window.Close()
	case "quit":
		mygo.App.Quit()
	}
}

func (a *app) requestCloseFileTab(index int) {
	session := a.fileSessions[index]
	if a.deferWorkspaceDragAction(func() { a.requestCloseFileTab(slices.Index(a.fileSessions, session)) }) {
		return
	}
	if a.prefs.ConfirmCloseActive && (session.state == connected || session.state == connecting) {
		a.closeFileRequest, a.closeRequest, a.closePaneRequest = session, nil, nil
		return
	}
	a.closedTabs = append(a.closedTabs, closedWorkspaceTab{file: session, section: sectionFiles})
	a.closeFileSession(index)
	a.afterLastTabClosed()
}

func (a *app) sessionScheme(session *remoteSession) *termtheme.Scheme {
	if options := session.config.SSH; options != nil && options.TermScheme != "" {
		return termtheme.Resolve(termtheme.ParseMode(options.TermScheme), theme.Current().IsDark)
	}
	return termtheme.Current()
}
