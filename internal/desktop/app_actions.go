package desktop

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
)

type closedWorkspaceTab struct {
	host    hostID
	section section
}

func (a *app) navigationSelected(target section) bool {
	return a.activeSection() == target
}

func (a *app) translucencyEnabled(c *ui.Context) bool {
	return a.prefs.Translucency && !a.reduceTransparency
}

func (a *app) glassRadius(c *ui.Context, radius float32) float32 {
	if !a.translucencyEnabled(c) {
		return 0
	}
	return radius
}

func (a *app) positionAnimation(animation ui.Animation) ui.Animation {
	if a.prefs.ReduceMotion == "on" {
		return ui.Animation{Duration: theme.Current().Motion.Duration.InstantNonzero}
	}
	return animation
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
	if a.actionPressed(c, "app.settings") {
		a.openSettings()
	}
	if a.actionPressed(c, "mode.terminal") {
		a.switchSection(sectionSSH)
	}
	if a.actionPressed(c, "mode.files") {
		a.switchSection(sectionFiles)
	}
	if a.actionPressed(c, "session.new") {
		a.closeSettings()
		a.startHostDraft()
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
			a.activateTab(next)
		}
	}
	for n := range 4 {
		if a.actionPressed(c, fmt.Sprintf("tab.select.%d", n+1)) {
			selectTab(n)
		}
	}
	if count > 0 {
		if a.actionPressed(c, "tab.prev") {
			selectTab((index + count - 1) % count)
		}
		if a.actionPressed(c, "tab.next") {
			selectTab((index + 1) % count)
		}
		if a.actionPressed(c, "tab.close") {
			if a.section == sectionFiles {
				a.requestCloseFileTab(a.activeFile)
			} else {
				a.requestCloseTab(a.activeTab)
			}
		}
	}
	if a.actionPressed(c, "host.search") {
		if a.settingsUI.open {
			a.settingsUI.search.Focus()
		} else {
			a.page().sidebarCollapsed = false
			a.page().search.Focus()
		}
	}
	if a.actionPressed(c, "tab.reopen") && len(a.closedTabs) > 0 {
		closed := a.closedTabs[len(a.closedTabs)-1]
		a.closedTabs = a.closedTabs[:len(a.closedTabs)-1]
		a.switchSection(closed.section)
		if closed.section == sectionFiles {
			a.openNewFileHost(closed.host)
		} else {
			a.openHost(closed.host)
		}
	}
	if a.settingsUI.open {
		return
	}
	if a.actionPressed(c, "sidebar.toggle") {
		a.page().sidebarCollapsed = !a.page().sidebarCollapsed
	}
	if a.section == sectionSSH && a.actionPressed(c, "pane.split") && a.currentTab() != nil {
		tab := a.currentTab()
		bounds := tab.focused.scroll.offset.Bounds(c)
		a.splitSession(tab.focused, tab.root.count() > 1 && bounds.H > bounds.W)
	}
}

func (a *app) installAppMenu() func() {
	settings := &mygo.MenuItem{
		Label: localizeTag(mygo.App.Locale(), "设置…"),
		Click: func(*mygo.MenuItem, *mygo.Window) {
			// Native menu callbacks do not pass through the UI modal scope.
			if a.page().addHostOpen || a.page().groupDialogOpen || a.files.dialog != nil ||
				a.files.conflict != nil || a.credentials.draft != nil || a.closeRequest != nil {
				return
			}
			a.openSettings()
			a.window.Invalidate()
		},
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "SSH Desk", Submenu: []*mygo.MenuItem{
			settings,
			{Type: mygo.MenuItemSeparator}, {Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
	mygo.App.SetMenu(menu)
	return mygo.App.OnLocaleChanged(func(locale string) {
		settings.SetLabel(localizeTag(locale, "设置…"))
	})
}

func (a *app) requestCloseTab(index int) {
	tab := a.tabs[index]
	if a.prefs.ConfirmCloseActive && slices.ContainsFunc(tab.root.sessions(), func(s *remoteSession) bool { return s.state == connected || s.state == connecting }) {
		a.closeRequest = tab
		return
	}
	a.closeTab(index)
	a.afterLastTabClosed()
}

func (a *app) closeTabDialog(c *ui.Context) {
	open := a.closeRequest != nil
	if !open {
		return
	}
	modal(c, &open, func() {
		t := theme.Current()
		ui.Column(c).Width(t.C.Dialog.GroupWidth).Gap(t.Space.N14).Children(func() {
			ui.Text(c, "关闭活动连接？").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "关闭此选项卡将结束其中的会话。").FontSize(t.Font.Size.Body)
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				button(c, "取消").OnClick(func() { a.closeRequest = nil })
				primaryButton(c, "关闭选项卡").OnClick(func() {
					index := slices.Index(a.tabs, a.closeRequest)
					a.closeRequest = nil
					if index >= 0 {
						a.closeTab(index)
						a.afterLastTabClosed()
					}
				})
			})
		})
	})
	if !open {
		a.closeRequest = nil
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
	if a.prefs.ConfirmCloseActive && (session.state == connected || session.state == connecting) {
		// Native confirmation is used here so an active file tab has the same
		// contract as a terminal tab, without changing the transfer conflict dialog.
		go func() {
			result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Type: mygo.MessageQuestion, Message: "关闭活动连接？", Detail: "关闭此选项卡将结束文件会话。", Buttons: []string{"取消", "关闭选项卡"}})
			mygo.RunOnMain(func() {
				if err != nil {
					a.files.notice = err.Error()
				} else if result.Button == 1 {
					if i := slices.Index(a.fileSessions, session); i >= 0 {
						a.closedTabs = append(a.closedTabs, closedWorkspaceTab{host: session.host, section: sectionFiles})
						a.closeFileSession(i)
						a.afterLastTabClosed()
					}
				}
				if a.window != nil {
					a.window.Invalidate()
				}
			})
		}()
		return
	}
	a.closedTabs = append(a.closedTabs, closedWorkspaceTab{host: session.host, section: sectionFiles})
	a.closeFileSession(index)
	a.afterLastTabClosed()
}

func (a *app) sessionScheme(session *remoteSession) *termtheme.Scheme {
	if options := session.config.SSH; options != nil && options.TermScheme != "" {
		return termtheme.Resolve(termtheme.ParseMode(options.TermScheme), theme.Current().IsDark)
	}
	return termtheme.Current()
}
