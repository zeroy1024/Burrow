package desktop

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	nativeterm "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type section int

type sidebarMotionKey section

type pageHeaderKey section

type pageDialogKey struct {
	section section
	kind    string
}

const (
	sectionSSH section = iota
	sectionFiles
	sectionSettings
)

type pageState struct {
	sidebarCollapsed  bool
	sidebarWidth      float32
	query             string
	search            ui.Handle
	addHostOpen       bool
	draft             hostDraft
	groupDialogOpen   bool
	groupDraft        string
	catalogScroll     ui.ScrollState
	catalogScrollbar  scrollbarState
	catalogItems      map[catalogKey]*catalogItemMotion
	catalogDrag       *catalogDragState
	catalogSlots      []catalogSlot
	catalogFlash      *catalogKey
	catalogFlashSeed  int
	catalogFlashUntil time.Time
}

type app struct {
	ctx              context.Context
	cancel           context.CancelFunc
	section          section
	pages            [sectionSettings + 1]pageState
	appearance       int
	reduceMotion     bool
	theme            *ui.Theme
	sshCatalog       catalogData
	fileCatalog      catalogData
	tabs             []*workspaceTab
	activeTab        int
	nextTab          int
	nextSession      int
	nextConnection   int
	tabScroll        ui.ScrollState
	tabPress         ui.Point
	statusVisible    bool
	window           *mygo.Window
	minimumWindow    [2]int
	workArea         ui.Handle
	pointer          ui.Point
	cellWidth        float32
	cellFont         ui.Font
	cellTracking     float32
	terminal         terminalSettingsState
	terminalError    error
	drag             *paneDragState
	dragSeen         bool
	pendingDock      func()
	dockPreview      *ui.Rect
	resizing         *terminalLayout
	catalogConfig    catalogConfigState
	catalogError     error
	statusScroll     ui.ScrollState
	statusScrollbar  scrollbarState
	fileSessions     []*fileSession
	activeFile       int
	nextFileSession  int
	files            fileWorkspaceState
	filePrefs        filePreferencesState
	transfers        transferQueueState
	settingsCategory int
	sshPrompts       []*sshPrompt
	sshWorkers       sync.WaitGroup
	fileSSHLink      *fileSession
}

func (a *app) page() *pageState {
	return &a.pages[a.section]
}

func (a *app) switchSection(target section) {
	if a.section == target {
		return
	}
	// Drag previews belong to the page being left; catalog data is only changed on drop.
	page := a.page()
	page.catalogDrag, page.catalogFlash = nil, nil
	for _, item := range page.catalogItems {
		item.offset = ui.Point{}
	}
	if d := a.drag; d != nil && d.kind == dragTab && d.phase == dragActive {
		active := a.currentTab()
		a.tabs = slices.Clone(d.order)
		a.activeTab = slices.Index(a.tabs, active)
	}
	a.drag, a.resizing, a.pendingDock, a.dockPreview = nil, nil, nil, nil
	a.files.drag = nil
	a.section = target
}

// Run starts the native desktop application and its main window.
func Run() error {
	if err := nativeterm.Load(); err != nil {
		return fmt.Errorf("load terminal emulator: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &app{
		ctx: ctx, cancel: cancel,
		sshCatalog: demoCatalog(), fileCatalog: demoCatalog(),
		tabs: []*workspaceTab{},
	}
	for i := range a.pages {
		a.pages[i].sidebarWidth = theme.Current().C.Sider.Width
	}
	a.catalogError = a.loadCatalog()
	a.terminalError = a.loadTerminalSettings()
	a.files.ratio = theme.Current().C.FTPSplit.DefaultRatio
	a.files.activeRemote = true
	if err := a.loadFilePreferences(); err != nil {
		a.files.notice = "无法读取文件传输偏好：" + err.Error()
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
	stopTerminal := termtheme.OnChange(func(*termtheme.Scheme) {
		if a.window != nil {
			a.window.Invalidate()
		}
	})
	defer stopTerminal()
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
		a.window.OnClosed(a.closeConnections)
	})
	err := mygo.App.Run()
	a.closeConnections()
	a.sshWorkers.Wait()
	return err
}

func (a *app) view(c *ui.Context) {
	a.reduceMotion = c.Preferences().ReduceMotion
	t := theme.Current()
	a.theme = nativeTheme(t)
	c.SetTheme(a.theme)
	c.Root().Background(tokenColor(t.C.Window.Bg))
	root := ui.Column(c).Fill()
	x, y, _ := root.PointerPosition()
	a.pointer = ui.Point{X: x, Y: y}
	if a.section == sectionSSH {
		a.beginWorkspaceFrame(c)
	}
	if a.section != sectionSettings {
		a.beginCatalogFrame(c)
	}
	a.enforcePaneMinimum(c)
	a.files.dragSeen = false
	a.tickTransfers(c)
	target := float32(1)
	if a.page().sidebarCollapsed {
		target = 0
	}
	shown := root.AnimateTo(sidebarMotionKey(a.section), target, tokenTween(t.C.Desktop.SidebarDuration))
	if a.section == sectionSSH {
		a.paneShortcuts(c)
	}
	if a.section != sectionSettings && c.Shortcut(ui.Super, ui.KeyK) {
		a.page().sidebarCollapsed = false
		a.page().search.Focus()
	}
	if a.section == sectionSSH && c.Shortcut(ui.Super, ui.KeyD) && a.currentTab() != nil {
		tab := a.currentTab()
		bounds := tab.focused.scroll.offset.Bounds(c)
		stacked := tab.root.count() > 1 && bounds.H > bounds.W
		a.splitSession(tab.focused, stacked)
	}
	root.Children(func() {
		ui.Column(c).Key(pageHeaderKey(a.section)).FillWidth().Shrink(0).Children(func() {
			switch a.section {
			case sectionSSH:
				a.titleBar(c, shown)
			case sectionFiles:
				a.fileTitleBar(c, shown)
			case sectionSettings:
				a.settingsTitleBar(c, shown)
			}
		})
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.navigation(c)
			a.workspace(c, shown)
		})
		if a.section == sectionSSH {
			if a.pendingDock != nil {
				a.pendingDock()
				a.pendingDock = nil
				c.AnimationFrame()
			}
			a.finishWorkspaceDrag(c)
			a.dragPreview(c)
		}
		if a.section != sectionSettings {
			a.finishCatalogDrag(c)
			a.catalogDragPreview(c)
		}
	})
	if a.catalogError != nil {
		c.Toast("无法加载主机配置：" + a.catalogError.Error())
		a.catalogError = nil
	}
	if a.catalogConfig.migrated {
		c.Toast("已将旧主机目录迁移为 SSH 与文件两份独立目录，原配置已备份。")
		a.catalogConfig.migrated = false
	}
	if a.terminalError != nil {
		c.Toast("终端设置：" + a.terminalError.Error())
		a.terminalError = nil
	}
	if a.section != sectionSettings {
		if a.page().addHostOpen {
			a.hostDialog(c.Key(pageDialogKey{section: a.section, kind: "host"}))
		}
		if a.page().groupDialogOpen {
			a.groupDialog(c.Key(pageDialogKey{section: a.section, kind: "group"}))
		}
	}
	if a.section == sectionFiles {
		a.fileDialogs(c)
	}
	if a.files.drag != nil && !a.files.dragSeen {
		a.files.drag = nil
		c.AnimationFrame()
	}
	if a.files.notice != "" {
		c.Toast(a.files.notice)
		a.files.notice = ""
	}
	a.sshDialogs(c)
	a.fileSSHLinkDialog(c)
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
		TextColor(tokenColor(r.Icon))
	button.OnClick(func() {
		a.switchSection(target)
	})
	background := ui.Transparent
	switch {
	case button.Pressed():
		background = tokenColor(t.C.Desktop.RailPressed)
	case a.section == target:
		background = tokenColor(r.BgOn)
	case button.Hovered():
		background = tokenColor(t.C.Desktop.RailHover)
	}
	button.Background(button.AnimateColor("background", background, tokenTween(r.Transition)))
	button.Children(func() {
		ui.Icon(c, icon).Size(t.Size.IconMd, t.Size.IconMd)
	})
	ui.TooltipBase(c, button, func(tip ui.Element) {
		p := t.C.Tooltip
		styleTooltip(tip).AttachTo(button, ui.AnchorRight, ui.AnchorLeft).
			MarginStart(p.RailOffset).Children(func() { ui.Text(c, label) })
	})
}

func (a *app) workspace(c *ui.Context, shown float32) {
	t := theme.Current()
	panel := ui.Row(c).Key(a.section).Grow(1).MinWidth(0).AlignItems(ui.Stretch).
		Margin(
			0,
			t.C.Window.Gutter,
			t.C.Window.Gutter,
			0,
		)
	statusTarget := float32(1)
	if a.section != sectionSSH || !a.statusVisible {
		statusTarget = 0
	}
	statusShown := panel.AnimateTo("status", statusTarget, tokenTween(t.C.Desktop.SidebarDuration))
	statusOpacity := animateVisual(panel, "status-opacity", statusTarget, t.C.Desktop.SidebarDuration)
	panel.Children(func() {
		workspaceCard(c).Key("sessions").Width(a.page().sidebarWidth * shown).Shrink(0).
			MarginEnd(t.C.Window.Gutter * shown).Hidden(shown == 0).Children(func() {
			if a.section == sectionSettings {
				a.settingsSidebar(c, shown)
			} else {
				a.sidebar(c, shown)
			}
		})
		if a.section == sectionSettings {
			a.settings(c)
			a.sidebarResizeHandle(c, shown)
			return
		}
		if a.section == sectionFiles {
			a.fileWorkspace(c)
			a.sidebarResizeHandle(c, shown)
			return
		}
		width, height := c.Size()
		// Both cards and the terminal use this frame's sidebar/monitor progress.
		sidebarWidth := (a.page().sidebarWidth + t.C.Window.Gutter) * shown
		statusWidth := (t.Size.StatsPanel + t.C.Window.Gutter) * statusShown
		a.terminals(c, ui.Point{
			X: width - t.C.Rail.Width - t.C.Window.Gutter - sidebarWidth - statusWidth,
			Y: height - max(t.C.Titlebar.Height, c.TitleBar().Height) - t.C.Window.Gutter,
		})
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
	label := "调整会话栏宽度"
	switch a.section {
	case sectionFiles:
		label = "调整站点栏宽度"
	case sectionSettings:
		label = "调整设置栏宽度"
	}
	// The transparent resize target covers the gap above both cards.
	edge := ui.Box(c).Key("resize-sidebar").Absolute().Top(0).Bottom(0).
		Left(a.page().sidebarWidth * shown).Width(t.C.Window.Gutter * shown).
		Hidden(shown != 1).Cursor(ui.CursorResizeEW).Focusable().FocusRing(false).Center().Role(ui.RoleSplitter).
		Label(label)
	var delta float32
	dx, _, dragging := edge.Dragged()
	if dragging {
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
		a.page().sidebarWidth = min(max(a.page().sidebarWidth+delta, t.C.Desktop.SiderMinWidth), t.C.Desktop.SiderMaxWidth)
	}
	edge.Range(float64(t.C.Desktop.SiderMinWidth), float64(t.C.Desktop.SiderMaxWidth), float64(a.page().sidebarWidth))
	visible := float32(0)
	if edge.Hovered() || dragging || edge.FocusVisible() {
		visible = 1
	}
	color := tokenColor(t.C.Desktop.DividerHandleColor)
	if dragging {
		color = tokenColor(t.C.DropIndicator.LineColor)
	}
	edge.Children(func() {
		ui.Box(c).Size(t.C.Desktop.DividerHandleWidth, t.C.Desktop.DividerHandleHeight).
			Radius(t.C.Desktop.DividerHandleRadius).Background(color).PassThrough().
			Opacity(animateVisual(edge, "handle", visible, t.C.Desktop.DividerFadeDuration))
	})
}
