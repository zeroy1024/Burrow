package desktop

import (
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type section int

const (
	sectionSSH section = iota
	sectionFiles
	sectionSettings
)

type app struct {
	section           section
	appearance        int
	theme             *ui.Theme
	sidebarCollapsed  bool
	sidebarWidth      float32
	hosts             []host
	groups            []*hostGroup
	tabs              []*workspaceTab
	activeTab         int
	nextTab           int
	nextSession       int
	query             string
	search            ui.Handle
	tabScroll         ui.ScrollState
	tabPress          ui.Point
	statusVisible     bool
	addHostOpen       bool
	draft             hostDraft
	groupDialogOpen   bool
	groupDraft        string
	window            *mygo.Window
	minimumWindow     [2]int
	workArea          ui.Handle
	pointer           ui.Point
	cellWidth         float32
	drag              *paneDragState
	dragSeen          bool
	pendingDock       func()
	dockPreview       *ui.Rect
	resizing          *terminalLayout
	catalogConfig     catalogConfigState
	catalogError      error
	catalogScroll     ui.ScrollState
	catalogItems      map[catalogKey]*catalogItemMotion
	catalogDrag       *catalogDragState
	catalogSlots      []catalogSlot
	catalogFlash      *catalogKey
	catalogFlashSeed  int
	catalogFlashUntil time.Time
}

// Run starts the native desktop application and its main window.
func Run() error {
	a := &app{
		sidebarWidth:  theme.Current().C.Sider.Width,
		hosts:         demoHosts(),
		groups:        demoGroups(),
		tabs:          []*workspaceTab{},
		statusVisible: true,
	}
	a.catalogError = a.loadCatalog()
	for _, id := range []hostID{0, 4, 1} {
		if int(id) < len(a.hosts) {
			a.openHost(id)
		}
	}
	if len(a.tabs) > 0 {
		a.activateTab(0)
	}
	mygo.Theme.SetSource(mygo.ThemeLight)
	stopTokens := theme.OnChange(func(t *theme.Theme) {
		if a.window != nil {
			if err := a.window.SetBackgroundColor(windowColor(t.C.Window.Bg)); err != nil {
				panic(err)
			}
			a.window.Invalidate()
		}
	})
	defer stopTokens()
	stopAppearance := mygo.Theme.OnUpdated(func() { a.syncAppearance() })
	defer stopAppearance()
	mygo.App.WhenReady(func() {
		t := theme.Current()
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title:                "SSH Desk",
			Width:                int(t.C.Desktop.WindowWidth),
			Height:               int(t.C.Desktop.WindowHeight),
			MinWidth:             int(t.C.Desktop.WindowMinWidth),
			MinHeight:            int(t.C.Desktop.WindowMinHeight),
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: int(t.C.Desktop.TrafficLightX), Y: int(t.C.Desktop.TrafficLightY)},
			BackgroundColor:      windowColor(t.C.Window.Bg),
			Content:              ui.View(a.view),
		})
	})
	return mygo.App.Run()
}

func (a *app) view(c *ui.Context) {
	t := theme.Current()
	a.theme = nativeTheme(t)
	c.SetTheme(a.theme)
	c.Root().Background(tokenColor(t.C.Window.Bg))
	root := ui.Column(c).Fill()
	x, y, _ := root.PointerPosition()
	a.pointer = ui.Point{X: x, Y: y}
	a.beginWorkspaceFrame(c)
	a.beginCatalogFrame(c)
	target := float32(1)
	if a.sidebarCollapsed {
		target = 0
	}
	shown := root.Animate("sidebar", target, t.MotionDuration(t.C.Desktop.SidebarDuration, c.Preferences().ReduceMotion))
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
			if a.section == sectionSettings {
				a.settings(c)
			} else {
				a.workspace(c, shown)
			}
		})
		if a.pendingDock != nil {
			a.pendingDock()
			a.pendingDock = nil
			c.AnimationFrame()
		}
		a.finishWorkspaceDrag(c)
		a.dragPreview(c)
		a.finishCatalogDrag(c)
		a.catalogDragPreview(c)
	})
	if a.catalogError != nil {
		c.Toast("无法加载主机配置：" + a.catalogError.Error())
		a.catalogError = nil
	}
	a.hostDialog(c)
	a.groupDialog(c)
}

func (a *app) navigation(c *ui.Context) {
	t := theme.Current()
	r := t.C.Rail
	ui.Column(c).Width(r.Width).Shrink(0).Padding(
		r.PaddingTop,
		t.Space.N6,
		r.PaddingBottom,
		t.Space.N6,
	).Gap(r.Gap).AlignItems(ui.Center).Label("主导航").Children(func() {
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
	t := theme.Current()
	r := t.C.Rail
	button := ui.ButtonBase(c).Key(label).Size(r.ButtonSize, r.ButtonSize).Radius(r.ButtonRadius).
		Label(label).Description(label).Selected(a.section == target).
		TextColor(tokenColor(r.Icon)).Disabled(target == sectionFiles)
	button.OnClick(func() {
		a.section = target
	})
	switch {
	case button.Pressed():
		button.Background(tokenColor(t.C.Desktop.RailPressed))
	case a.section == target:
		button.Background(tokenColor(r.BgOn))
	case button.Hovered():
		button.Background(tokenColor(t.C.Desktop.RailHover))
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(t.Size.IconMd, t.Size.IconMd)
	})
	ui.TooltipBase(c, button, func(tip ui.Element) {
		p := t.C.Tooltip
		tip.AttachTo(button, ui.AnchorRight, ui.AnchorLeft).MarginStart(t.Space.N8).
			Padding(p.PaddingY, p.PaddingX).Radius(p.Radius).
			Background(tokenColor(p.Bg)).TextColor(tokenColor(p.Text)).FontSize(p.FontSize)
		applyShadow(tip, p.Shadow).Children(func() { ui.Text(c, label) })
	})
}

func (a *app) workspace(c *ui.Context, shown float32) {
	t := theme.Current()
	panel := ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).
		Margin(
			0,
			t.C.Window.Gutter,
			t.C.Window.Gutter,
			0,
		)
	statusTarget := float32(1)
	if !a.statusVisible {
		statusTarget = 0
	}
	statusShown := panel.Animate("status", statusTarget, t.MotionDuration(t.C.Desktop.SidebarDuration, c.Preferences().ReduceMotion))
	statusOpacity := animateVisual(panel, "status-opacity", statusTarget, t.C.Desktop.SidebarDuration)
	panel.Children(func() {
		workspaceCard(c).Key("sessions").Width(a.sidebarWidth * shown).Shrink(0).
			MarginEnd(t.C.Window.Gutter * shown).Hidden(shown == 0).Children(func() {
			a.sidebar(c, shown)
		})
		a.terminals(c)
		status := workspaceCard(c).Key("status").Width(t.Size.StatsPanel * statusShown).Shrink(0).
			MarginStart(t.C.Window.Gutter * statusShown).Hidden(statusShown == 0 && statusOpacity == 0)
		if c.Preferences().ReduceMotion && !a.statusVisible && statusOpacity > 0 {
			// Geometry changes immediately; the old panel can still fade over the
			// expanded workspace without animating its width.
			status.Absolute().Top(0).Right(0).Bottom(0).Width(t.Size.StatsPanel)
		}
		status.Opacity(statusOpacity).Children(func() {
			ui.Column(c).Width(t.Size.StatsPanel).FillHeight().
				Children(func() { a.serverStatus(c) })
		})
		a.sidebarResizeHandle(c, shown)
	})
}

func workspaceCard(c *ui.Context) ui.Element {
	t := theme.Current().C.Card
	return applyShadow(ui.Column(c).Radius(t.Radius).Background(tokenColor(t.Bg)), t.Shadow).Clip()
}

func (a *app) sidebarResizeHandle(c *ui.Context, shown float32) {
	t := theme.Current()
	// The transparent resize target covers the gap above both cards.
	edge := ui.Box(c).Key("resize-sidebar").Absolute().Top(0).Bottom(0).
		Left(a.sidebarWidth * shown).Width(t.C.Window.Gutter * shown).
		Hidden(shown != 1).Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).
		Label("调整会话栏宽度")
	var delta float32
	if dx, _, dragging := edge.Dragged(); dragging {
		edge.Focus()
		delta = dx
	}
	if edge.Shortcut(0, ui.KeyLeft) {
		delta -= t.C.Desktop.SiderKeyboardStep
	}
	if edge.Shortcut(0, ui.KeyRight) {
		delta += t.C.Desktop.SiderKeyboardStep
	}
	if delta != 0 {
		a.sidebarWidth = min(max(a.sidebarWidth+delta, t.C.Desktop.SiderMinWidth), t.C.Desktop.SiderMaxWidth)
	}
	edge.Range(float64(t.C.Desktop.SiderMinWidth), float64(t.C.Desktop.SiderMaxWidth), float64(a.sidebarWidth))
}
