package desktop

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/zeroy1024/Burrow/assets"
	"github.com/zeroy1024/Burrow/internal/locales"
)

type trayState struct {
	icon                                 *mygo.Tray
	open, status, toggle, settings, quit *mygo.MenuItem
	err                                  error
	// panel is the open flyout of a click on the icon, nil while closed;
	// panelHeight is its last measured height, which it opens at again.
	panel       *mygo.Window
	panelHeight int
}

// trayPanelSupported reports whether the icon reports clicks, so a click can
// open the panel. Linux's AppIndicator only shows menus.
func trayPanelSupported() bool { return runtime.GOOS != "linux" }

func (a *app) syncTray() error {
	if !a.prefs.ShowTray {
		a.destroyTray()
		return nil
	}
	if a.tray.icon != nil {
		a.refreshTray()
		return nil
	}
	a.tray.open = &mygo.MenuItem{Click: func(*mygo.MenuItem, *mygo.Window) { a.showMainWindow() }}
	a.tray.status = &mygo.MenuItem{Disabled: true}
	a.tray.toggle = &mygo.MenuItem{Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleMCPFromTray() }}
	a.tray.settings = &mygo.MenuItem{Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettingsFromMenu() }}
	a.tray.quit = &mygo.MenuItem{Role: mygo.RoleQuit}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		a.tray.open,
		mygo.Separator(),
		a.tray.status,
		a.tray.toggle,
		mygo.Separator(),
		a.tray.settings,
		a.tray.quit,
	})
	options := mygo.TrayOptions{
		ID: "burrow", Icon: assets.IconPNG, ToolTip: "Burrow", Menu: menu,
	}
	if runtime.GOOS == "darwin" {
		options.Icon = assets.TrayTemplatePNG
		options.IconIsTemplate = true
	}
	if trayPanelSupported() {
		// Without a menu the click reaches OnClick; the native menu stays
		// on the secondary click.
		options.Menu = nil
	}
	icon, err := mygo.NewTray(options)
	a.tray.err = err
	if err != nil {
		return err
	}
	a.tray.icon = icon
	if trayPanelSupported() {
		icon.OnClick(a.toggleTrayPanel)
		icon.OnRightClick(func() {
			a.closeTrayPanel()
			// Set for this popup only, so that clicks keep toggling the panel.
			icon.SetMenu(menu)
			defer icon.SetMenu(nil)
			icon.PopUpMenu()
		})
	}
	a.syncQuitShortcut()
	a.refreshTray()
	return nil
}

func (a *app) destroyTray() {
	a.closeTrayPanel()
	if a.tray.icon != nil {
		a.tray.icon.Destroy()
	}
	a.tray = trayState{}
}

func (a *app) refreshTray() {
	if a.tray.icon == nil {
		return
	}
	if a.tray.panel != nil {
		a.tray.panel.Invalidate()
	}
	a.tray.open.SetLabel(locales.MessageOpenBurrow())
	a.tray.settings.SetLabel(locales.MessageSettings())
	a.tray.settings.SetEnabled(a.businessReady())
	status := locales.MessageMcpUnavailable()
	running := false
	available := a.mcpBackend != nil && a.mcpBackend.service != nil
	if !a.security.ready {
		status = locales.MessageWaitingForAppUnlock()
	} else if available {
		_, active, _, err := a.mcpBackend.service.Status()
		running = active
		status = locales.MessageOff()
		if running {
			status = locales.MessageRunning()
		}
		if err != nil {
			status = locales.MessageMcpServiceError()
		}
	}
	a.tray.status.SetLabel(fmt.Sprintf(locales.MessageTrayMcpStatus(), status))
	label := locales.MessageStartMcpService()
	if running {
		label = locales.MessageStopMcpService()
	}
	a.tray.toggle.SetLabel(label)
	a.tray.toggle.SetEnabled(available && a.businessReady() && !a.mcpBusy())
}

func (a *app) toggleMCPFromTray() {
	if !a.businessReady() || a.mcpBusy() {
		return
	}
	service := a.mcpBackend.service
	cfg, running, _, _ := service.Status()
	cfg.Enabled = !running
	a.mcpWork(func() error { return service.Configure(a.ctx, cfg) }, func(err error) {
		if err != nil {
			a.showMainWindow()
		}
	})
}

func (a *app) backgroundEnabled() bool {
	return a.prefs.WindowClose == "background" || runtime.GOOS == "darwin" && a.prefs.CmdQBackground
}

func (a *app) setBackgroundPreferences(closeAction string, showTray, cmdQBackground bool) {
	previousClose, previousTray, previousCmdQ := a.prefs.WindowClose, a.prefs.ShowTray, a.prefs.CmdQBackground
	a.prefs.WindowClose, a.prefs.CmdQBackground = closeAction, cmdQBackground
	if a.backgroundEnabled() {
		showTray = true
	}
	a.prefs.ShowTray = showTray
	trayChanged := showTray != previousTray
	needsBackgroundIcon := a.backgroundEnabled() && a.tray.icon == nil
	if trayChanged || needsBackgroundIcon {
		if err := a.syncTray(); err != nil {
			a.prefs.WindowClose, a.prefs.ShowTray = previousClose, previousTray
			a.prefs.CmdQBackground = previousCmdQ
			a.reportTrayError(err)
			return
		}
	}
	a.syncQuitShortcut()
	a.preferencesChanged()
}

func (a *app) reportTrayError(err error) {
	a.settingsUI.notice = locales.Concat(
		locales.Deferred(locales.MessageUnableToCreateTrayIcon),
		locales.Raw(": "),
		locales.ErrorText(err),
	)
	a.persistenceNotice(locales.MessageUnableToCreateTrayIcon, err)
}

func (a *app) showMainWindow() {
	if a.window != nil {
		if runtime.GOOS == "darwin" && mygo.App.ActivationPolicy() != mygo.ActivationPolicyRegular {
			mygo.App.Dock.Show()
		}
		a.window.Show()
		a.refreshTray()
	}
}

func (a *app) hideMainWindow() {
	if a.quitPending {
		return
	}
	if a.tray.icon == nil {
		a.reportTrayError(a.tray.err)
		return
	}
	a.window.Hide()
	if runtime.GOOS == "darwin" && a.prefs.HideDockInBackground {
		mygo.App.Dock.Hide()
	}
	if !a.security.initialized {
		return
	}
	// Hidden windows need no frames. Flush the same snapshot as quitting,
	// after an in-flight catalog write, without stopping any services.
	save := func() {
		a.persistBeforeClose(func(err error) {
			if err != nil {
				a.persistenceNotice(locales.MessageUnableToSaveBeforeHiding, err)
				a.showMainWindow()
			}
		})
	}
	if a.catalogSaving {
		a.catalogReady = append(a.catalogReady, save)
		return
	}
	save()
}
