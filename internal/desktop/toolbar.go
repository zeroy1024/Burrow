package desktop

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

const sessionTabGap = 4

func iconButton(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	button := ui.ButtonBase(c).Key(label).Size(28, 28).Radius(7).
		Label(label).Tooltip(label).TextColor(c.Theme().TextMuted)
	switch {
	case button.Pressed():
		button.Background(ui.RGB(0, 0, 0).Alpha(0.1))
	case button.Hovered():
		button.Background(ui.RGB(0, 0, 0).Alpha(0.055))
	}
	button.Children(func() { ui.Icon(c, icon).Size(16, 16) })
	return button
}

func (a *app) titleBar(c *ui.Context, shown float32) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(titleBarHeight, bar.Height)).Shrink(0).Gap(6).DragWindow().Children(func() {
		ui.Row(c).Width(max(navigationWidth+a.sidebarWidth*shown, bar.Left+44)).Shrink(0).
			PaddingStart(bar.Left + 8).DragWindow().Children(func() {
			label := "收起会话栏"
			if a.sidebarCollapsed {
				label = "展开会话栏"
			}
			iconButton(c, label, sidebarIcon).OnClick(func() { a.sidebarCollapsed = !a.sidebarCollapsed })
		})
		ui.Box(c).Grow(1).MinWidth(0).Height(36).Clip().Children(func() {
			// Clip the native indicator outside the viewport while retaining native scrolling.
			strip := ui.ScrollHorizontal(c.Key("session-tabs")).Fill().TrackScroll(&a.tabScroll).ScrollbarInsets(
				0,
				0,
				-12,
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
				ui.Row(c).Gap(sessionTabGap).FillHeight().Children(func() {
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
		ui.Row(c).Padding(0, 8).Shrink(0).Children(func() {
			status := iconButton(c, "服务器监控", activityIcon).Expanded(a.statusVisible)
			if a.statusVisible {
				status.Background(ui.RGB(0, 0, 0).Alpha(0.065))
			}
			status.OnClick(func() { a.statusVisible = !a.statusVisible })
		})
	})
}

func (a *app) tabButton(c *ui.Context, index int, tab *workspaceTab) func() {
	const contentHeight = 20
	var pendingClose func()
	session := tab.focused
	h := a.hosts[session.host]
	button := ui.ButtonBase(c).Key(tab.id).Height(32).Padding(0, 9).Gap(6).Radius(8).
		Bind(&tab.frame).Transition(ui.ElementTransition{Position: true, Duration: 150 * time.Millisecond}).
		AlignItems(ui.Center).
		Role(ui.RoleTab).Label(h.name).Selected(a.activeTab == index).TextColor(a.theme.Text).
		Tooltip(fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port))
	if a.activeTab == index {
		button.Background(ui.Hex(cardBackground)).Shadow(
			0,
			0,
			0,
			0.5,
			ui.RGB(0, 0, 0).Alpha(0.06),
		)
	} else if button.Hovered() {
		button.Background(ui.RGB(255, 255, 255).Alpha(0.45))
	}
	button.OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
	button.ContextMenu(func(m *ui.Menu) {
		current := tab.focused
		if current.state == connected {
			if m.Item("断开当前会话").Chosen() {
				current.state = disconnected
			}
		} else if m.Item("连接当前会话").Chosen() {
			current.state = connected
		}
		m.Separator()
		if m.Item("关闭选项卡").Chosen() {
			pendingClose = func() { a.closeTab(slices.Index(a.tabs, tab)) }
		}
	})
	button.Children(func() {
		ui.Box(c).Size(16, contentHeight).Shrink(0).Center().Children(func() {
			if tab.root.count() > 1 {
				ui.Icon(c, gridIcon).Size(12, 12).TextColor(a.theme.TextMuted)
				return
			}
			statusDot(c, session.state, 6)
		})
		// Center the visible glyphs as well as their font line boxes.
		ui.Text(c, h.name).FontSize(12).FontWeight(550).FixedLineHeight(contentHeight).Top(-1.5).SingleLine()
		if tab.root.count() > 1 {
			ui.Text(c, "+"+strconv.Itoa(tab.root.count()-1)).FontSize(11).
				FixedLineHeight(contentHeight).Top(-0.5).TextColor(a.theme.TextMuted)
		}
		iconButton(c, "关闭标签 "+h.name, closeIcon).Size(16, contentHeight).
			OnClick(func() { a.closeTab(slices.Index(a.tabs, tab)) })
		handle := ui.Box(c).Key("tab-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(30).
			Label("拖动标签 " + h.name).OnClick(func() { a.activateTab(slices.Index(a.tabs, tab)) })
		a.paneDragHandle(c, handle, paneTransfer{tab: tab, session: tab.focused, fromTab: true})
	})
	if d := a.drag; d != nil && d.kind == dragTab && d.originTab == tab && d.phase != dragLanded {
		button.Opacity(0)
	}
	return pendingClose
}
