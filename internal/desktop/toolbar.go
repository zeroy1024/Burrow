package desktop

import (
	"fmt"
	"math"
	"slices"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func iconButton(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	return keyedIconButton(c, label, label, icon)
}

func keyedIconButton(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	return uikit.IconButton(c, icon, uikit.IconButtonProps{Label: label, Key: key})
}

func (a *app) titleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	uikit.TitleBar(c).Children(func() {
		uikit.TitleBarLeading(c, t.C.Rail.Width+a.page().sidebarWidth*shown).Children(func() {
			label := "收起会话栏"
			if a.page().sidebarCollapsed {
				label = "展开会话栏"
			}
			keyedIconButton(c, "sidebar-toggle", label, sidebarIcon).
				Disabled(a.activeSection() == sectionSettings).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		uikit.SessionTabsViewport(c).Children(func() {
			// Clip the native indicator outside the viewport while retaining native scrolling.
			strip := uikit.SessionTabsScroll(c.Key("session-tabs"), &a.tabScroll).HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
					a.tabPress = ui.Point{X: ev.X, Y: ev.Y}
				}
				if ev.Kind != ui.InputScroll || math.Abs(float64(ev.DY)) <= math.Abs(float64(ev.DX)) {
					return false
				}
				a.tabScroll.X = min(max(a.tabScroll.X+ev.DY, 0), a.tabScroll.MaxX)
				return true
			})
			strip.Children(func() {
				uikit.SessionTabsRow(c).Children(func() {
					var pendingClose func()
					for i, tab := range a.tabs {
						if close := a.tabButton(c, i, tab); close != nil {
							pendingClose = close
						}
					}
					iconButton(c, "新建会话", plusIcon).Menu(func(m *ui.Menu) {
						for _, group := range a.catalogGroups() {
							m.Submenu(group.name, func(sub *ui.Menu) {
								for _, id := range group.hosts {
									if a.hosts[id].protocol != "" {
										continue
									}
									if sub.Item(a.hosts[id].name).Chosen() {
										a.openHost(id)
									}
								}
							})
						}
					})
					if pendingClose != nil {
						pendingClose()
					}
				})
			})
			a.tabStripTarget(c, strip)
		})
		uikit.TitleBarActions(c).Children(func() {
			status := iconButton(c, "服务器监控", activityIcon).Expanded(a.statusVisible).
				Disabled(a.activeSection() == sectionSettings)
			if a.statusVisible {
				status.Background(tokenColor(t.C.TitleAction.BgOn))
			}
			status.OnClick(func() { a.statusVisible = !a.statusVisible })
		})
	})
}

func (a *app) tabButton(c *ui.Context, index int, tab *workspaceTab) func() {
	var pendingClose func()
	session := tab.focused
	h := a.hosts[session.host]
	hint := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…（演示）"
	case disconnected:
		hint = "已断开 · 可从菜单重新连接"
	}
	props := uikit.SessionTabProps{
		Key: tab.id, Title: h.name, Hint: hint, Selected: a.activeTab == index,
		State: connectionIndicatorState(session.state), Handle: &tab.frame,
		Extra: tab.root.count() - 1, CloseLabel: "关闭标签 " + h.name, DragLabel: "拖动标签 " + h.name, CloseIcon: closeIcon,
	}
	button := uikit.SessionTab(c, props)
	button.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
	button.ContextMenu(func(m *ui.Menu) {
		current := tab.focused
		if m.Item("编辑会话…").Chosen() {
			a.startEditHostDraft(current.host)
		}
		m.Separator()
		if current.state == connected || current.state == connecting {
			label := "断开当前会话"
			if current.state == connecting {
				label = "取消连接"
			}
			if m.Item(label).Chosen() {
				disconnectSession(current)
			}
		} else if m.Item("连接当前会话").Chosen() {
			a.connectSession(current)
		}
		m.Separator()
		if m.Item("关闭选项卡").Chosen() {
			pendingClose = func() { a.requestCloseTab(slices.Index(a.tabs, tab)) }
		}
	})
	button.Children(func() {
		props.State = connectionIndicatorState(session.state)
		parts := uikit.SessionTabContents(c, button, props)
		parts.Close.OnClick(func() { a.requestCloseTab(slices.Index(a.tabs, tab)) })
		parts.DragHandle.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
		a.paneDragHandle(c, parts.DragHandle, paneTransfer{tab: tab, session: tab.focused, fromTab: true})
	})
	if d := a.drag; d != nil && d.kind == dragTab && d.originTab == tab && d.phase != dragLanded {
		button.Opacity(0)
	}
	return pendingClose
}
