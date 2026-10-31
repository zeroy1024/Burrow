package desktop

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	titleBarHeight      = 48
	navigationWidth     = 52
	workspaceSpacing    = 6
	minSidebarWidth     = 240
	maxSidebarWidth     = 320
	defaultSidebarWidth = 260
	statusPanelWidth    = 220
	windowBackground    = "#e7e7e8"
	cardBackground      = "#ffffff"
	primaryText         = "#1d1d1f"
	secondaryText       = "#8e8e93"
)

type section int

const (
	sectionSSH section = iota
	sectionFiles
	sectionSettings
)

type app struct {
	section          section
	theme            *ui.Theme
	sidebarCollapsed bool
	sidebarWidth     float32
	hosts            []host
	groups           []*hostGroup
	tabs             []*workspaceTab
	activeTab        int
	nextTab          int
	nextSession      int
	query            string
	search           ui.Handle
	tabScroll        ui.ScrollState
	statusVisible    bool
	addHostOpen      bool
	draft            hostDraft
	groupDialogOpen  bool
	groupDraft       string
	window           *mygo.Window
	minimumWindow    [2]int
	workArea         ui.Handle
	pointer          ui.Point
	cellWidth        float32
	drag             *paneDragState
	dragSeen         bool
	pendingDock      func()
	dockPreview      *ui.Rect
	pendingTabMove   func()
	resizing         *terminalLayout
}

// Run starts the native desktop application and its main window.
func Run() error {
	a := &app{
		theme:         ui.LightTheme(),
		sidebarWidth:  defaultSidebarWidth,
		hosts:         demoHosts(),
		groups:        demoGroups(),
		tabs:          []*workspaceTab{},
		statusVisible: true,
	}
	a.theme.Text = ui.Hex(primaryText)
	a.theme.TextMuted = ui.Hex(secondaryText)
	a.theme.Border = ui.Hex("#eeeef0")
	a.theme.Accent = ui.Hex("#0a84ff")
	for _, id := range []hostID{0, 4, 1} {
		a.openHost(id)
	}
	a.activateTab(0)
	mygo.Theme.SetSource(mygo.ThemeLight)
	mygo.App.WhenReady(func() {
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title:                "SSH Desk",
			Width:                1180,
			Height:               780,
			MinWidth:             1000,
			MinHeight:            640,
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 12, Y: 17},
			BackgroundColor:      windowBackground,
			Content:              ui.View(a.view),
		})
	})
	return mygo.App.Run()
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	c.Root().Background(ui.Hex(windowBackground))
	root := ui.Column(c).Fill()
	x, y, _ := root.PointerPosition()
	a.pointer = ui.Point{X: x, Y: y}
	a.beginWorkspaceFrame(c)
	target := float32(1)
	if a.sidebarCollapsed {
		target = 0
	}
	shown := root.Animate("sidebar", target, 220*time.Millisecond)
	a.paneShortcuts(c)
	if c.Shortcut(ui.Super, ui.KeyK) {
		a.sidebarCollapsed = false
		a.search.Focus()
	}
	if c.Shortcut(ui.Super, ui.KeyD) && a.currentTab() != nil {
		tab := a.currentTab()
		bounds := tab.focused.scroll.offset.Bounds(c)
		stacked := tab.root.count() > 1 && bounds.H > bounds.W
		a.splitSession(tab.focused, stacked)
	}
	root.Children(func() {
		a.titleBar(c, shown)
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.navigation(c)
			a.workspace(c, shown)
		})
		if a.pendingDock != nil {
			a.pendingDock()
			a.pendingDock = nil
			c.AnimationFrame()
		}
		a.finishWorkspaceDrag(c)
		a.dragPreview(c)
	})
	a.hostDialog(c)
	a.groupDialog(c)
}

func (a *app) navigation(c *ui.Context) {
	ui.Column(c).Width(navigationWidth).Shrink(0).Padding(
		6,
		6,
		8,
		6,
	).Gap(6).AlignItems(ui.Center).Label("主导航").Children(func() {
		a.navigationButton(
			c,
			sectionSSH,
			"SSH",
			terminalIcon,
		)
		a.navigationButton(
			c,
			sectionFiles,
			"SFTP / FTP",
			filesIcon,
		)
		ui.Spacer(c).DragWindow()
		a.navigationButton(
			c,
			sectionSettings,
			"设置",
			settingsIcon,
		)
	})
}

func (a *app) navigationButton(c *ui.Context, target section, label string, icon *ui.SVG) {
	button := ui.ButtonBase(c).Key(label).Size(32, 32).Radius(8).
		Label(label).Description(label).Selected(a.section == target).
		TextColor(ui.Hex("#77777c")).Disabled(target != sectionSSH)
	button.OnClick(func() {
		a.section = target
	})
	switch {
	case button.Pressed():
		button.Background(ui.RGB(0, 0, 0).Alpha(0.12))
	case a.section == target:
		button.Background(ui.RGB(0, 0, 0).Alpha(0.08))
	case button.Hovered():
		button.Background(ui.RGB(0, 0, 0).Alpha(0.045))
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(18, 18)
	})
	ui.TooltipBase(c, button, func(tip ui.Element) {
		tip.AttachTo(button, ui.AnchorRight, ui.AnchorLeft).MarginStart(8).
			Padding(5, 8).Radius(5).Background(a.theme.Text).TextColor(a.theme.Background).FontSize(12).
			Shadow(
				0,
				2,
				8,
				0,
				ui.RGB(0, 0, 0).Alpha(0.2),
			).Children(func() {
			ui.Text(c, label)
		})
	})
}

func (a *app) workspace(c *ui.Context, shown float32) {
	panel := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).
		Margin(
			0,
			workspaceSpacing,
			workspaceSpacing,
			0,
		)
	statusTarget := float32(1)
	if !a.statusVisible {
		statusTarget = 0
	}
	statusShown := panel.Animate("status", statusTarget, 220*time.Millisecond)
	panel.Children(func() {
		workspaceCard(c).Key("sessions").Width(a.sidebarWidth * shown).Shrink(0).
			MarginEnd(workspaceSpacing * shown).Hidden(shown == 0).Children(func() {
			a.sidebar(c, shown)
		})
		a.terminals(c)
		workspaceCard(c).Key("status").Width(statusPanelWidth * statusShown).Shrink(0).
			MarginStart(workspaceSpacing * statusShown).Hidden(statusShown == 0).Children(func() {
			ui.Column(c).Width(statusPanelWidth).FillHeight().Opacity(statusShown).
				Children(func() { a.serverStatus(c) })
		})
		a.sidebarResizeHandle(c, shown)
	})
}

func workspaceCard(c *ui.Context) ui.Element {
	return ui.Column(c).Radius(12).Background(ui.Hex(cardBackground)).
		Shadow(
			0,
			0,
			0,
			0.5,
			ui.RGB(0, 0, 0).Alpha(0.06),
		).Clip()
}

func (a *app) sidebarResizeHandle(c *ui.Context, shown float32) {
	// The transparent resize target covers the gap above both cards.
	edge := ui.Box(c).Key("resize-sidebar").Absolute().Top(0).Bottom(0).
		Left(a.sidebarWidth * shown).Width(workspaceSpacing * shown).
		Hidden(shown != 1).Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).
		Label("调整会话栏宽度")
	var delta float32
	if dx, _, dragging := edge.Dragged(); dragging {
		edge.Focus()
		delta = dx
	}
	if edge.Shortcut(0, ui.KeyLeft) {
		delta -= 10
	}
	if edge.Shortcut(0, ui.KeyRight) {
		delta += 10
	}
	if delta != 0 {
		a.sidebarWidth = min(max(a.sidebarWidth+delta, minSidebarWidth), maxSidebarWidth)
	}
	edge.Range(minSidebarWidth, maxSidebarWidth, float64(a.sidebarWidth))
}
