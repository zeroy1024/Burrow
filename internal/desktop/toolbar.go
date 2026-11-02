package desktop

import (
	"fmt"
	"slices"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) titleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	uikit.TitleBar(c).Children(func() {
		uikit.TitleBarLeading(c, t.C.Rail.Width+(a.page().sidebarWidth+t.C.Window.Gutter)*shown).Children(func() {
			label := "收起会话栏"
			if a.page().sidebarCollapsed {
				label = "展开会话栏"
			}
			uikit.IconButton(c, sidebarIcon, uikit.IconButtonProps{Key: "sidebar-toggle", Label: label, Variant: uikit.IconOnWindow,
				Disabled: a.activeSection() == sectionSettings}).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		var add ui.Element
		tabs := uikit.SessionTabs(c, nil, nil, uikit.SessionTabsProps{
			Scroll: &a.tabScroll, ScrollKey: "session-tabs", Motion: uikit.MotionOverride(a.reduceMotion(c)),
			StripInput: func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
					a.tabPress = ui.Point{X: ev.X, Y: ev.Y}
					a.captureTerminalTabPress(ev)
				}
				return false
			},
			BuildTabs: func(list ui.Element) ui.Element {
				var pendingAction func()
				for i, tab := range a.tabs {
					if action := a.tabButton(c, list, i, tab); action != nil {
						pendingAction = action
					}
				}
				var menu []uikit.MenuEntry
				for _, group := range a.catalogGroups() {
					entry := uikit.MenuEntry{ID: fmt.Sprintf("group-%d", group.id), Label: group.name}
					for _, id := range group.hosts {
						if a.hosts[id].protocol != "" {
							continue
						}
						entry.Children = append(entry.Children, uikit.MenuEntry{ID: fmt.Sprintf("host-%d", id), Label: a.hosts[id].name,
							OnChoose: func() { a.openHost(id) }})
					}
					entry.Disabled = len(entry.Children) == 0
					menu = append(menu, entry)
				}
				add = uikit.IconButton(c, plusIcon, uikit.IconButtonProps{Key: "新建会话", Label: "新建会话", Variant: uikit.IconOnWindow})
				uikit.MenuOn(c, add, menu)
				if pendingAction != nil {
					pendingAction()
				}
				return add
			},
		})
		tabs.Scroller.DrawOver(func(_ *ui.Painter, _ ui.Rect) { a.terminalCapture.strip = tabs.Scroller.Bounds() })
		a.tabStripTarget(c, tabs, add)
		if tabs.Select >= 0 {
			a.selectTab(tabs.Select)
		}
		uikit.TitleBarActions(c).Children(func() {
			uikit.IconButton(c, activityIcon, uikit.IconButtonProps{Key: "服务器监控", Label: "服务器监控", Variant: uikit.IconOnWindow,
				On: &a.statusVisible, Disabled: a.activeSection() == sectionSettings}).Expanded(a.statusVisible)
		})
	})
}

func (a *app) tabButton(c *ui.Context, list ui.Element, index int, tab *workspaceTab) func() {
	t := theme.Current()
	var pendingAction func()
	session := tab.focused
	h := session.endpoint
	hint := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…"
	case disconnected:
		hint = "已断开 · 可从菜单重新连接"
	}
	props := uikit.SessionTabProps{
		Key: tab.id, Title: h.name, Hint: hint, Selected: a.activeTab == index,
		State: connectionIndicatorState(session.state), Handle: &tab.frame,
		ReduceMotion: a.reduceMotion(c), PositionAnimation: a.positionAnimation(ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.TabDisplace)}),
		Extra: tab.root.count() - 1, CloseLabel: "关闭标签 " + h.name, DragLabel: "拖动标签 " + h.name, CloseIcon: closeIcon,
	}
	button := uikit.SessionTab(c, props)
	input := uikit.SessionTabInput(c, list, button, index, len(a.tabs))
	if input.Select >= 0 {
		a.selectTab(input.Select)
	}
	if input.Close {
		pendingAction = func() { a.requestCloseTab(slices.Index(a.tabs, tab)) }
	}
	button.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
	menu := []uikit.MenuEntry{}
	if tab.root.count() > 1 {
		menu = append(menu, uikit.MenuEntry{
			ID: "ungroup", Label: localize(c, "拆分会话"), Icon: uikit.IconGrid,
			Disabled: a.drag != nil || !tab.canSeparateSessions(), OnChoose: func() {
				pendingAction = func() { a.ungroupTab(tab) }
			},
		})
	} else {
		menu = append(menu, uikit.MenuEntry{
			ID: "edit", Label: localize(c, "编辑会话…"), Icon: uikit.IconEdit,
			Disabled: session.temporary || session.host < 0, OnChoose: func() { a.startEditHostDraft(session.host) },
		}, uikit.MenuEntry{ID: "after-edit", Separator: true})
		if session.state == connected || session.state == connecting {
			label, icon := "断开当前会话", uikit.IconPause
			if session.state == connecting {
				label, icon = "取消连接", uikit.IconClose
			}
			menu = append(menu, uikit.MenuEntry{
				ID: "disconnect", Label: localize(c, label), Icon: icon,
				OnChoose: func() { disconnectSession(session) },
			})
		} else {
			menu = append(menu, uikit.MenuEntry{
				ID: "connect", Label: localize(c, "连接当前会话"), Icon: uikit.IconTerminal,
				OnChoose: func() { a.connectSession(session) },
			})
		}
	}
	menu = append(menu, uikit.MenuEntry{ID: "before-close", Separator: true}, uikit.MenuEntry{
		ID: "close", Label: localize(c, "关闭选项卡"), Icon: uikit.IconClose, Danger: true,
		OnChoose: func() { pendingAction = func() { a.requestCloseTab(slices.Index(a.tabs, tab)) } },
	})
	button.Children(func() {
		props.State = connectionIndicatorState(session.state)
		parts := uikit.SessionTabContents(c, button, props)
		parts.Close.OnClick(func() { a.requestCloseTab(slices.Index(a.tabs, tab)) })
		parts.DragHandle.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
		a.paneDragHandle(c, parts.DragHandle, paneTransfer{tab: tab, session: tab.focused, fromTab: true})
		uikit.ContextMenuOn(
			c,
			button,
			menu,
			parts.DragHandle,
			parts.Close,
		)
	})
	if d := a.drag; d != nil && d.kind == dragTab && d.originTab == tab && d.phase != dragLanded {
		button.Opacity(0)
	}
	if d := a.drag; d != nil && d.phase == dragTabLanding && d.landingTab == tab {
		button.Opacity(0)
	}
	return pendingAction
}
