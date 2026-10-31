package desktop

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func iconButton(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	return keyedIconButton(c, label, label, icon)
}

func keyedIconButton(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	t := theme.Current()
	button := styleIconButton(ui.ButtonBase(c).Key(key).Label(label), t)
	tooltip(c, button, label)
	button.Children(func() { ui.Icon(c, icon).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
	return button
}

func (a *app) titleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	bar := c.TitleBar()
	ui.Row(c).Height(max(t.C.Titlebar.Height, bar.Height)).Shrink(0).Gap(t.Space.N6).DragWindow().Children(func() {
		ui.Row(c).Width(max(t.C.Rail.Width+a.page().sidebarWidth*shown, bar.Left+t.C.Titlebar.Height)).Shrink(0).
			PaddingStart(bar.Left + t.C.Titlebar.LeftGap).DragWindow().Children(func() {
			label := "收起会话栏"
			if a.page().sidebarCollapsed {
				label = "展开会话栏"
			}
			keyedIconButton(c, "sidebar-toggle", label, sidebarIcon).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		ui.Box(c).Grow(1).MinWidth(0).Height(t.C.Tab.StripHeight).Clip().Children(func() {
			// Clip the native indicator outside the viewport while retaining native scrolling.
			strip := ui.ScrollHorizontal(c.Key("session-tabs")).Fill().TrackScroll(&a.tabScroll).ScrollbarInsets(
				0,
				0,
				-t.Space.N12,
				0,
			).HandleInput(func(ev ui.InputEvent) bool {
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
				ui.Row(c).Gap(t.C.Titlebar.TabsGap).FillHeight().Children(func() {
					var pendingClose func()
					for i, tab := range a.tabs {
						if close := a.tabButton(c, i, tab); close != nil {
							pendingClose = close
						}
					}
					iconButton(c, "新建会话", plusIcon).Menu(func(m *ui.Menu) {
						for _, group := range a.groups {
							m.Submenu(group.name, func(sub *ui.Menu) {
								for _, id := range group.hosts {
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
		ui.Row(c).Padding(0, t.C.Titlebar.ActionsPaddingRight).Gap(t.C.Titlebar.ActionsGap).Shrink(0).Children(func() {
			status := iconButton(c, "服务器监控", activityIcon).Expanded(a.statusVisible)
			if a.statusVisible {
				status.Background(tokenColor(t.C.TitleAction.BgOn))
			}
			status.OnClick(func() { a.statusVisible = !a.statusVisible })
		})
	})
}

func (a *app) tabButton(c *ui.Context, index int, tab *workspaceTab) func() {
	t := theme.Current()
	contentHeight := t.C.Tab.ContentHeight
	var pendingClose func()
	session := tab.focused
	h := a.hosts[session.host]
	button := ui.ButtonBase(c).Key(tab.id).Height(t.C.Tab.Height).
		Padding(0, t.C.Tab.PaddingRight, 0, t.C.Tab.PaddingLeft).Gap(t.C.Tab.Gap).Radius(t.C.Tab.Radius).
		Bind(&tab.frame).Transition(ui.ElementTransition{
		Position:  true,
		Animation: &ui.Animation{Spring: tokenSpring(t.C.ReorderMotion.TabDisplace)},
	}).
		AlignItems(ui.Center).
		Role(ui.RoleTab).Label(h.name).Selected(a.activeTab == index)
	background, foreground := ui.Transparent, tokenColor(t.C.Tab.Text)
	hint := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
	if a.activeTab == index {
		background, foreground = tokenColor(t.C.Tab.BgSelected), tokenColor(t.C.Tab.TextSelected)
		applyShadow(button, t.C.Tab.ShadowSelected)
	} else if button.Hovered() {
		background = tokenColor(t.C.Tab.BgHover)
	}
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…（演示）"
	case disconnected:
		foreground, hint = tokenColor(t.C.Tab.TextDisconnected), "已断开 · 可从菜单重新连接"
	}
	animation := tokenTween(t.C.Tab.StateTransition)
	button.Background(button.AnimateColor("background", background, animation)).
		TextColor(button.AnimateColor("text", foreground, animation))
	tooltip(c, button, hint)
	button.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
	button.ContextMenu(func(m *ui.Menu) {
		current := tab.focused
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
			pendingClose = func() { a.closeTab(slices.Index(a.tabs, tab)) }
		}
	})
	button.Children(func() {
		ui.Box(c).Width(t.C.Tab.SpinnerSize).Height(contentHeight).Shrink(0).Center().Children(func() {
			tabConnectionIndicator(c, session.state)
		})
		// Center the visible glyphs as well as their font line boxes.
		ui.Text(c, h.name).FontSize(t.C.Tab.FontSize).FontWeight(t.C.Tab.WeightSelected).FixedLineHeight(contentHeight).Top(t.C.Tab.TitleOffset).SingleLine()
		if tab.root.count() > 1 {
			ui.Text(c, "+"+strconv.Itoa(tab.root.count()-1)).FontSize(t.Font.Size.Caption).
				FixedLineHeight(contentHeight).Top(t.C.Tab.CountOffset).TextColor(tokenColor(t.C.Tab.Text))
		}
		close := iconButton(c, "关闭标签 "+h.name, closeIcon).Size(t.Size.Icon, contentHeight)
		closeVisible := float32(0)
		if a.activeTab == index || button.Hovered() || close.Focused() {
			closeVisible = 1
		}
		close.Opacity(animateVisual(close, "visible", closeVisible, t.C.Tab.StateTransition)).
			OnClick(func() { a.closeTab(slices.Index(a.tabs, tab)) })
		handle := ui.Box(c).Key("tab-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(t.C.Tab.DragHandleInset).
			Label("拖动标签 " + h.name).OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
		a.paneDragHandle(c, handle, paneTransfer{tab: tab, session: tab.focused, fromTab: true})
	})
	if d := a.drag; d != nil && d.kind == dragTab && d.originTab == tab && d.phase != dragLanded {
		button.Opacity(0)
	}
	return pendingClose
}
