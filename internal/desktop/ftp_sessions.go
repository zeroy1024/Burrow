package desktop

import (
	"fmt"
	"slices"
	"time"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fileSession struct {
	config      sessionConfig
	id          int
	host        hostID
	state       connectionState
	errorText   string
	connectedAt time.Time
	local       *filePaneState
	remote      *filePaneState
}

func (a *app) currentFileSession() *fileSession {
	if len(a.fileSessions) == 0 {
		return nil
	}
	return a.fileSessions[a.activeFile]
}

func (a *app) activateFileTab(index int) {
	a.selectFileTab(index)
	a.focusWorkspace()
}

func (a *app) selectFileTab(index int) {
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

func (a *app) openNewFileHost(id hostID) {
	a.nextFileSession++
	session := &fileSession{
		config: a.hosts[id].session,
		id:     a.nextFileSession, host: id, state: connecting, connectedAt: time.Now(),
		local: newFilePane(false), remote: newFilePane(true),
	}
	a.restoreFileSession(session)
	a.fileSessions = append(a.fileSessions, session)
	a.activeFile = len(a.fileSessions) - 1
	if a.activeSection() == sectionFiles {
		a.focusWorkspace()
	}
}

func (a *app) reconnectFileSession(session *fileSession) {
	session.config = a.hosts[session.host].session
	a.restoreFileSession(session)
	session.state, session.errorText = connecting, ""
	session.connectedAt = time.Now()
}

func (a *app) closeFileSession(index int) {
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

func (a *app) updateFileConnections(c *ui.Context) {
	for _, session := range a.fileSessions {
		if session.state != connecting {
			continue
		}
		remaining := theme.Current().C.Tab.SpinnerPeriod - c.Now().Sub(session.connectedAt)
		if remaining <= 0 {
			session.state = connected
			continue
		}
		c.After(remaining)
	}
}

func (a *app) fileTitleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	uikit.TitleBar(c).PaddingEnd(t.C.Window.Gutter).Children(func() {
		uikit.TitleBarLeading(c, t.C.Rail.Width+a.page().sidebarWidth*shown).Children(func() {
			label := "收起站点栏"
			if a.page().sidebarCollapsed {
				label = "展开站点栏"
			}
			uikit.IconButton(c, sidebarIcon, uikit.IconButtonProps{Key: "sidebar-toggle", Label: label, Variant: uikit.IconOnWindow, Disabled: a.activeSection() == sectionSettings}).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		tabs := uikit.SessionTabs(c, nil, nil, uikit.SessionTabsProps{
			Scroll: &a.fileTabScroll, ScrollKey: "file-session-tabs", Motion: uikit.MotionOverride(a.reduceMotion(c)),
			BuildTabs: func(list ui.Element) ui.Element {
				var closing *fileSession
				for index, session := range a.fileSessions {
					if a.fileTabButton(c, list, index, session) {
						closing = session
					}
				}
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
				add := uikit.IconButton(c, plusIcon, uikit.IconButtonProps{Key: "新建文件会话", Label: "新建文件会话", Variant: uikit.IconOnWindow})
				uikit.MenuOn(c, add, menu)
				if closing != nil {
					a.requestCloseFileTab(slices.Index(a.fileSessions, closing))
				}
				return add
			},
		})
		if tabs.Select >= 0 {
			a.selectFileTab(tabs.Select)
		}
	})
}

func (a *app) fileTabButton(c *ui.Context, list ui.Element, index int, session *fileSession) bool {
	h := a.hosts[session.host]
	selected := a.activeFile == index
	hint := fmt.Sprintf("%s@%s:%d · 演示会话", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…（演示）"
	case disconnected:
		hint = "已断开 · 选择后可重新连接"
	case connectionFailed:
		hint = session.errorText
	}
	props := uikit.SessionTabProps{
		Key: session, Title: h.name, Hint: hint, Selected: selected, Variant: uikit.SessionTabFiles,
		State: connectionIndicatorState(session.state), CloseLabel: "关闭文件标签 " + h.name, CloseIcon: closeIcon,
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
	button.ContextMenu(func(m *ui.Menu) {
		if m.Item("编辑会话…").Chosen() {
			a.startEditHostDraft(session.host)
		}
		m.Separator()
		if session.state == connected || session.state == connecting {
			if m.Item("断开文件会话").Chosen() {
				session.state = disconnected
			}
		} else if m.Item("重新连接").Chosen() {
			a.reconnectFileSession(session)
		}
		m.Separator()
		if m.Item("模拟连接失败").Chosen() {
			session.state, session.errorText = connectionFailed, "认证被拒绝（演示错误）"
		}
		m.Separator()
		if m.Item("关闭选项卡").Chosen() {
			closing = true
		}
	})
	button.Children(func() {
		props.State = connectionIndicatorState(session.state)
		parts := uikit.SessionTabContents(c, button, props)
		parts.Close.OnClick(func() { a.requestCloseFileTab(slices.Index(a.fileSessions, session)) })
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
	style := systems[h.os]
	uikit.OsLogo(c, style.name, uikit.SizeDefault)
}

func (a *app) deleteFileSite(id hostID) error {
	deleted := a.hosts[id]
	for _, h := range a.hosts {
		if h.session.SSH != nil && h.session.SSH.JumpHostID != "" && h.session.SSH.JumpHostID == deleted.session.ID {
			return fmt.Errorf("会话正被「%s」用作跳板机，请先修改该会话", h.name)
		}
	}
	if err := a.commitCatalogMove(func() {
		a.hosts = slices.Delete(a.hosts, int(id), int(id)+1)
		for _, group := range a.groups {
			group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
			for i, candidate := range group.hosts {
				if candidate > id {
					group.hosts[i]--
				}
			}
		}
	}); err != nil {
		return err
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
		if session.host == id {
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
	for session := range remainingFiles {
		if session.host > id {
			session.host--
		}
	}
	for i := len(a.tabs) - 1; i >= 0; i-- {
		tab := a.tabs[i]
		for _, session := range tab.root.sessions() {
			if session.host == id {
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
	a.closedTabs = slices.DeleteFunc(a.closedTabs, func(candidate closedWorkspaceTab) bool { return candidate.host == id })
	for i, candidate := range a.closedTabs {
		if candidate.host > id {
			a.closedTabs[i].host--
		}
	}
	if deleted.session.PasswordSaved {
		if err := mygo.Secrets.Delete("session/" + deleted.session.ID + "/password"); err != nil {
			a.files.notice = &uikit.ToastProps{ID: "会话已移除，但钥匙串清理失败：" + err.Error(), Tone: uikit.ToneDanger, Title: "会话已移除，但钥匙串清理失败", Detail: err.Error()}
		}
	}
	return nil
}

func (a *app) deleteSiteDialogContents(c *ui.Context, parts uikit.ModalParts) bool {
	t := theme.Current()
	var confirm bool
	uikit.Text(c, "将从当前页面的目录移除，并关闭此条目的会话。服务器文件不受影响。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
		confirm = uikit.Button(c, "删除", uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked()
	})
	return confirm
}

func (a *app) confirmDeleteSite(c *ui.Context, id hostID) {
	if a.fileSiteBusy(id) {
		uikit.Notify(c, "请先取消或清除该会话的传输任务")
		return
	}
	if err := a.deleteFileSite(id); err != nil {
		uikit.Toast(c, uikit.ToastProps{ID: "删除会话失败：" + err.Error(), Tone: uikit.ToneDanger, Title: "删除会话失败", Detail: err.Error()})
		return
	}
	a.page().addHostOpen = false
}
