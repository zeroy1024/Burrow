package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/localfs"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"

	"github.com/egoist/mygo"
	nativeterm "github.com/egoist/mygo/burrow-terminal"
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
	sectionMCP
	sectionGateways
)

func (s section) hasCatalog() bool {
	return s == sectionSSH || s == sectionFiles
}

type pageState struct {
	lastGroup         *hostGroup
	sidebarCollapsed  bool
	sidebarWidth      float32
	query             string
	search            ui.Handle
	addHostOpen       bool
	draft             hostDraft
	groupDialogOpen   bool
	groupModal        uikit.ModalController
	groupDraft        string
	groupDelete       *hostGroup
	groupRename       *hostGroup
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
	vault                  *vault.Manager
	security               applicationSecurityState
	store                  *store.Store
	persistence            *persistenceWorker
	dataDirectory          string
	catalogSaving          bool
	catalogReady           []func()
	siteDeletion           *siteDeletion
	workspaceSave          workspaceSaveState
	quitReady, quitPending bool
	terminalCapture        terminalCaptureState
	terminalPreviews       map[*remoteSession]*terminalPreview
	monitors               map[*sshConnectionGroup]*monitorState
	hostFacts              hostFactsState
	ctx                    context.Context
	cancel                 context.CancelFunc
	sshWorkers             sync.WaitGroup
	nextConnection         int
	nextCatalogGroupID     int
	sshPrompts             []*sshPrompt
	gateways               []gatewayConfig
	defaultGateway         string
	gatewayUI              gatewayUIState
	gatewayPool            gatewayPool

	section            section
	settingsButton     ui.Handle
	pages              [sectionGateways + 1]pageState
	mcp                mcpUIState
	mcpBackend         *mcpDesktop
	appearance         int
	theme              *ui.Theme
	hosts              []host
	groups             []*hostGroup
	tabs               []*workspaceTab
	activeTab          int
	nextTab            int
	nextSession        int
	tabScroll          ui.ScrollState
	tabPress           ui.Point
	statusVisible      bool
	window             *mygo.Window
	tray               trayState
	quitMenu           *mygo.MenuItem
	backgroundMenu     *mygo.MenuItem
	windowMaterial     mygo.Vibrancy
	minimumWindow      [2]int
	workArea           ui.Handle
	pointer            ui.Point
	cellWidth          float32
	cellFont           ui.Font
	terminal           terminalSettingsState
	terminalError      error
	drag               *paneDragState
	dragSeen           bool
	pendingDock        func()
	dockPreview        *ui.Rect
	resizing           *terminalLayout
	catalogConfig      catalogConfigState
	catalogError       error
	statusScroll       ui.ScrollState
	statusScrollbar    scrollbarState
	statusFactsOpen    bool
	fileSessions       []*fileSession
	fileTabScroll      ui.ScrollState
	activeFile         int
	nextFileSession    int
	files              fileWorkspaceState
	filePrefs          filePreferencesState
	transfers          transferQueueState
	settingsCategory   int
	prefs              appPreferences
	prefsSave          preferenceSaveState
	settingsUI         settingsState
	prefsLoadErr       error
	keys               []managedKey
	credentials        credentialState
	closedTabs         []closedWorkspaceTab
	closeRequest       *workspaceTab
	closeFileRequest   *fileSession
	closePaneRequest   *paneCloseRequest
	closeConfirmed     bool
	savedWorkspace     *storedWorkspace
	reduceTransparency bool
}

func (a *app) page() *pageState {
	return &a.pages[a.section]
}

// Settings is a peer page; section remembers the last workspace while it is open.
func (a *app) activeSection() section {
	if a.settingsUI.open {
		return sectionSettings
	}
	return a.section
}

func (a *app) switchSection(target section) {
	if target == sectionSettings {
		a.openSettings()
		return
	}
	if a.activeSection() == target {
		return
	}
	if a.section == sectionGateways {
		a.cancelGatewayTest()
	}
	if a.settingsUI.open {
		a.settingsUI.previousFocus = ui.Handle{}
		a.closeSettings()
	}
	a.clearWorkspaceGestures()
	a.section = target
	a.focusWorkspace()
}

func (a *app) focusWorkspace() {
	switch a.activeSection() {
	case sectionSSH:
		if tab := a.currentTab(); tab != nil {
			a.focusSession(tab, tab.focused)
		}
	case sectionFiles:
		if session := a.currentFileSession(); session != nil {
			pane := session.remote
			if a.files.localOpen && !a.files.activeRemote {
				pane = session.local
			}
			pane.focus.Focus()
		}
	}
}

func (a *app) clearWorkspaceGestures() {
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
}

func appVersion() string {
	if version := mygo.App.Version(); version != "" {
		return version
	}
	// Plain go run has no package version metadata.
	return "dev"
}

// Run starts the native desktop application and its main window.
func Run() (runErr error) {
	directory, err := configureDataDirectory()
	if err != nil {
		return err
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	if err := localfs.MkdirAll(directory, 0700); err != nil {
		return locales.Errorf(locales.MessageProtectAppDataDirectory, err)
	}
	if err := nativeterm.Load(); err != nil {
		return locales.Errorf(locales.MessageLoadNativeTerminal, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	db, err := store.Open(ctx, filepath.Join(directory, "burrow.db"))
	if err != nil {
		cancel()
		return locales.Errorf(locales.MessageOpenAppDatabase, err)
	}

	a := &app{ctx: ctx, cancel: cancel, store: db, dataDirectory: directory, tabs: []*workspaceTab{}, statusFactsOpen: true}
	defer func() {
		a.cancel()
		if a.mcpBackend != nil && a.mcpBackend.service != nil {
			runErr = errors.Join(runErr, a.mcpBackend.service.Close())
		}
		a.sshWorkers.Wait()
		if a.vault != nil {
			a.vault.Close()
		}
		a.clearSecurityInputs()
		runErr = errors.Join(runErr, a.finishPersistence())
	}()
	a.persistence = newPersistenceWorker(func() *mygo.Window { return a.window })
	for i := range a.pages {
		a.pages[i].sidebarWidth = theme.Current().C.Sider.Width
	}
	a.terminalError = a.loadTerminalSettings()
	if err := a.loadAppPreferences(); err != nil {
		a.security.errorText = locales.ErrorText(err)
	}
	a.vault, err = vault.OpenWithKeyStore(ctx, db, directory, legacyUnlockKeys())
	if err != nil {
		a.security.errorText = locales.ErrorText(err)
		a.security.status = "blocked"
	} else {
		a.security.migration = a.vault.State() == "pending_migration"
		a.security.requirePassword = a.vault.RequiresPassword()
	}
	mygo.Theme.SetSource([]mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeSystem}[a.appearance])
	stopTokens := theme.OnChange(func(t *theme.Theme) {
		updateTerminalSurfaceAccent(t)
		if a.window != nil {
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
	var stopMenuLocale func()
	defer func() {
		if stopMenuLocale != nil {
			stopMenuLocale()
		}
	}()
	openWindow := func() {
		a.syncAppearance()
		t := theme.Current()
		material := windowVibrancy()
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title:                "Burrow",
			Hidden:               true,
			Width:                int(t.C.Desktop.WindowWidth),
			Height:               int(t.C.Desktop.WindowHeight),
			MinWidth:             int(t.C.Desktop.WindowMinWidth),
			MinHeight:            int(t.C.Desktop.WindowMinHeight),
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       int(t.C.Titlebar.Height),
			TrafficLightPosition: &mygo.Point{X: int(t.C.Desktop.TrafficLightX), Y: int(t.C.Desktop.TrafficLightY)},
			BackgroundColor:      "#00000000",
			Transparent:          true,
			Vibrancy:             material,
			Content:              ui.View(a.view),
		})
		a.windowMaterial = material
		a.syncWindowMaterial()
		a.window.OnClose(func(e *mygo.CloseEvent) {
			if a.quitReady {
				return
			}
			e.PreventDefault()
			if a.prefs.WindowClose == "background" {
				a.hideMainWindow()
				return
			}
			mygo.App.Quit()
		})
		if stopMenuLocale != nil {
			stopMenuLocale()
		}
		stopMenuLocale = a.installAppMenu()
		if err := a.syncTray(); err != nil {
			a.reportTrayError(err)
		}
		// Windows can build the first frame synchronously when shown. Finish
		// assigning and setting up a.window before that frame starts UI work.
		a.showMainWindow()
		a.beginStartup()
	}
	mygo.App.WhenReady(openWindow)
	stopSecondInstance := mygo.App.OnSecondInstance(func([]string, string) { a.showMainWindow() })
	defer stopSecondInstance()
	stopActivate := mygo.App.OnActivate(func(visible bool) {
		if !visible && a.ctx.Err() == nil {
			a.showMainWindow()
		}
	})
	defer stopActivate()
	stopQuit := mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.security.initialized {
			a.quitReady = true
			a.cancel()
			return
		}
		if a.security.changing {
			e.PreventDefault()
			return
		}
		if a.quitPending {
			e.PreventDefault()
			return
		}
		if a.mcpBusy() || a.credentials.busy || a.terminal.importing || a.savingSession() || a.catalogSaving || a.files.dialog != nil && a.files.dialog.busy {
			a.resetQuitAttempt()
			e.PreventDefault()
			a.files.notice = &uikit.ToastProps{ID: "work-before-quit", Tone: uikit.ToneWarning, Title: locales.Deferred(
				locales.MessageAFileOrCredentialOperationIsInProgressWaitForItToFinish,
			)}
			if a.window != nil {
				a.showMainWindow()
				a.window.Invalidate()
			}
			return
		}
		if !a.quitReady {
			e.PreventDefault()
			a.quitPending = true
			a.refreshTray()
			a.persistBeforeClose(func(err error) {
				if err != nil {
					a.quitPending = false
					a.persistenceNotice(locales.MessageUnableToSaveBeforeQuitting, err)
					a.showMainWindow()
					return
				}
				a.stopMCPForQuit(func(err error) {
					if err != nil {
						a.quitPending = false
						a.persistenceNotice(locales.MessageUnableToStopTheMcpService, err)
						a.showMainWindow()
						return
					}
					a.prepareTransferQuit(func(err error) {
						a.quitPending = false
						if err != nil {
							a.persistenceNotice(locales.MessageUnableToSaveTransferHistory, err)
							a.showMainWindow()
							return
						}
						a.quitReady = true
						a.window.Update(func() { mygo.App.Quit() })
					})
				})
			})
			return
		}
		a.cancelCredentialWork()
		for i := range a.pages {
			a.pages[i].draft.cancelTest()
		}
	})
	defer stopQuit()
	stopShutdown := mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		a.destroyTray()
		a.clearTerminalPreviews()
		a.stopMonitors()
		a.closeConnections()
	})
	defer stopShutdown()
	return mygo.App.Run()
}

func (a *app) view(c *ui.Context) {
	if !a.security.ready {
		a.startupView(c)
		return
	}
	a.tickMCPBackend(c)
	t := theme.Current()
	a.theme = uikit.Install(c)
	background := uikit.Color(t.C.Window.Bg)
	headerBackground, contentBackground := ui.Transparent, ui.Transparent
	if a.translucencyEnabled(c) && c.Vibrancy() {
		switch runtime.GOOS {
		case "darwin":
			// Tint the native material once across the whole window, so the
			// header and content share a continuous background.
			background = background.Alpha(t.C.Window.TintOpacity)
		case "windows":
			// Mica supplies the base color; content cards keep their own fills.
			background = ui.Transparent
		default:
			headerBackground = background.Alpha(t.C.Titlebar.TintOpacity)
			contentBackground = background
			background = ui.Transparent
		}
	}
	c.Root().Background(background)
	root := ui.Column(c).Fill()
	x, y, _ := root.PointerPosition()
	a.pointer = ui.Point{X: x, Y: y}
	if !a.interactionBlocked() {
		a.appShortcuts(c)
	}
	if a.drag == nil && !a.terminalCapturePressed(c) {
		a.clearTerminalPreviews()
	}
	active := a.activeSection()
	a.updatePaneClosures(c)
	if active == sectionSSH {
		a.beginWorkspaceFrame(c)
	}
	if active.hasCatalog() {
		a.beginCatalogFrame(c)
	}
	a.enforcePaneMinimum(c)
	a.files.dragSeen = false
	a.enforceScrollback()
	a.tickTransfers(c)
	a.updateSSHLatency(c)
	a.updateMonitor(c)
	a.tickPreferences(c)
	a.tickFilePreferences(c)
	a.tickWorkspace(c)
	target := float32(1)
	if a.page().sidebarCollapsed {
		target = 0
	}
	shown := root.AnimateTo(sidebarMotionKey(a.section), target, a.positionAnimation(uikit.Tween(t.C.Desktop.SidebarDuration)))
	root.Children(func() {
		ui.Column(c).Key(pageHeaderKey(active)).FillWidth().Shrink(0).Background(headerBackground).Children(func() {
			switch active {
			case sectionSSH:
				a.titleBar(c, shown)
			case sectionFiles:
				a.fileTitleBar(c, shown)
			case sectionMCP, sectionSettings, sectionGateways:
				uikit.TitleBar(c).FillWidth()
			}
		})
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Background(contentBackground).Children(func() {
			a.navigation(c)
			ui.Box(c).Key(active).Grow(1).MinWidth(0).FillHeight().Children(func() {
				switch active {
				case sectionSSH, sectionFiles:
					a.workspace(c, shown)
				case sectionMCP:
					ui.Box(c).Fill().Padding(0, t.C.Window.Gutter, t.C.Window.Gutter, 0).
						Children(func() { a.mcpPage(c) })
				case sectionGateways:
					ui.Box(c).Fill().Padding(0, t.C.Window.Gutter, t.C.Window.Gutter, 0).Children(func() { a.gatewayPage(c) })
				case sectionSettings:
					ui.Box(c).Fill().Padding(0, t.C.Window.Gutter, t.C.Window.Gutter, 0).
						Children(func() { a.settingsPage(c) })
				}
			})
		})
		if active == sectionSSH {
			if a.pendingDock != nil {
				a.pendingDock()
				a.pendingDock = nil
				c.AnimationFrame()
			}
			a.finishWorkspaceDrag(c)
			a.dragPreview(c)
		}
		if active.hasCatalog() {
			a.finishCatalogDrag(c)
			a.catalogDragPreview(c)
		}
	})
	a.showCatalogError(c)
	if a.terminalError != nil {
		uikit.Toast(
			c,
			uikit.ToastProps{Title: locales.Deferred(
				locales.MessageTerminalSettings,
			), Detail: locales.ErrorText(
				a.terminalError,
			), Tone: uikit.ToneDanger},
		)
		a.terminalError = nil
	}
	if active.hasCatalog() {
		if a.page().addHostOpen {
			a.hostDialog(c.Key(pageDialogKey{section: a.section, kind: "host"}))
		}
		if a.page().groupDialogOpen {
			a.groupDialog(c.Key(pageDialogKey{section: a.section, kind: "group"}))
		}
	}
	if active == sectionFiles || a.files.conflict != nil {
		a.fileDialogs(c)
	}
	if active == sectionMCP {
		a.mcpDialogs(c)
	}
	a.gatewayDialog(c)
	a.sshDialogs(c)
	a.credentialDialogs(c)
	a.applicationSecurityDialog(c)
	a.closeTabDialog(c)
	if a.files.drag != nil && !a.files.dragSeen {
		drag := a.files.drag
		a.files.drag = nil
		if drag.publishDirectory != nil {
			drag.publishDirectory()
		}
		if drag.publishDisconnect != nil {
			drag.publishDisconnect()
		}
		if drag.afterCancel != nil {
			drag.afterCancel()
		}
		c.AnimationFrame()
	}
	if a.files.notice != nil {
		uikit.Toast(c, *a.files.notice)
		a.files.notice = nil
	}
	uikit.ToastViewport(c, uikit.MotionOverride(a.reduceMotion(c)))
	if a.interactionBlocked() {
		ui.Overlay(c.Key("application-input-shield"), func() {
			uikit.InputShield(c).Modal().AutoFocus().Role(ui.RoleStatus).Label(locales.MessageProcessing2())
		})
	}
}

func (a *app) interactionBlocked() bool {
	return a.security.changing || a.transfers.quitting || a.catalogSaving || a.quitPending || a.terminal.importing
}

func (a *app) navigation(c *ui.Context) {
	uikit.Rail(c).Children(func() {
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
		a.navigationButton(
			c,
			sectionGateways,
			locales.GatewaysTitle(),
			routeIcon,
		)
		a.navigationButton(
			c,
			sectionMCP,
			"MCP",
			mcpIcon,
		)
		ui.Spacer(c).DragWindow()
		a.navigationButton(
			c,
			sectionSettings,
			locales.SettingsTitle(),
			settingsIcon,
		)
	})
}

func (a *app) navigationButton(c *ui.Context, target section, label string, icon *ui.SVG) {
	props := uikit.RailButtonProps{Key: label, Label: label, Icon: icon, Selected: a.navigationSelected(target)}
	if target == sectionSettings {
		props.Handle = &a.settingsButton
	}
	uikit.RailButton(c, props).OnClick(func() {
		opening := target == sectionSettings && !a.settingsUI.open
		a.switchSection(target)
		if opening {
			a.settingsUI.previousFocus = a.settingsButton
		}
	})
}

func (a *app) workspace(c *ui.Context, shown float32) {
	t := theme.Current()
	panel := uikit.Workspace(c, a.section)
	statusTarget := float32(1)
	if a.section != sectionSSH || !a.statusVisible {
		statusTarget = 0
	}
	statusShown := panel.AnimateTo("status", statusTarget, a.positionAnimation(uikit.Tween(t.C.Desktop.SidebarDuration)))
	statusOpacity := uikit.Visual(panel, "status-opacity", statusTarget, t.C.Desktop.SidebarDuration)
	panel.Children(func() {
		uikit.Card(c).Key("sessions").Width(a.page().sidebarWidth * shown).Shrink(0).
			MarginEnd(t.C.Window.Gutter * shown).Hidden(shown == 0).Children(func() {
			a.sidebar(c, shown)
		})
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
		status := uikit.Card(c).Key("status").Width(t.Size.StatsPanel * statusShown).Shrink(0).
			MarginStart(t.C.Window.Gutter * statusShown).Hidden(statusShown == 0 && statusOpacity == 0)
		if a.reduceMotion(c) && !a.statusVisible && statusOpacity > 0 {
			// Geometry changes immediately; the old panel can still fade over the
			// expanded workspace without animating its width.
			status.Absolute().Top(0).Right(0).Bottom(0).Width(t.Size.StatsPanel)
		}
		status.Opacity(statusOpacity).Children(func() {
			uikit.SidePanelBody(c).Children(func() { a.serverStatus(c) })
		})
		a.sidebarResizeHandle(c, shown)
	})
}

func (a *app) sidebarResizeHandle(c *ui.Context, shown float32) {
	t := theme.Current()
	label := locales.MessageResizeSessionSidebar()
	switch a.section {
	case sectionFiles:
		label = locales.MessageResizeSiteSidebar()
	case sectionSettings:
		label = locales.MessageResizeSettingsSidebar()
	}
	// The transparent resize target covers the gap above both cards.
	edge := uikit.SidebarDivider(c, label, a.page().sidebarWidth*shown, shown)
	uikit.ResizeGutter(c, edge, &a.page().sidebarWidth, t.C.Desktop.SiderMinWidth, t.C.Desktop.SiderMaxWidth, false)
}

func (a *app) resetQuitAttempt() {
	a.quitReady, a.quitPending, a.transfers.quitting = false, false, false
	a.refreshTray()
}
