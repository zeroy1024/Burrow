package desktop

import (
	"math"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

// trayPanelRefresh redraws the open panel for values that change without an
// event of their own: transfer progress and latency samples.
const trayPanelRefresh = time.Second

// trayPanelHeight is the height the panel first opens at, before it has
// measured its content.
const trayPanelHeight = 240

func (a *app) toggleTrayPanel() {
	if a.tray.panel != nil {
		a.closeTrayPanel()
		return
	}
	t := theme.Current()
	height := a.tray.panelHeight
	if height == 0 {
		height = trayPanelHeight
	}
	panel := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         a.tray.icon,
		Placement:    mygo.PlacementBottom,
		Gap:          int(t.Space.N4),
		Width:        int(t.C.TrayPanel.Width),
		Height:       height,
		Focusable:    true,
		Shadow:       true,
		CornerRadius: int(t.C.Popover.Radius),
		Content:      ui.View(a.trayPanelView),
	})
	a.tray.panel = panel
	panel.OnClosed(func() {
		if a.tray.panel == panel {
			a.tray.panel = nil
		}
	})
}

func (a *app) closeTrayPanel() {
	if a.tray.panel != nil {
		a.tray.panel.Close()
		a.tray.panel = nil
	}
}

func (a *app) trayPanelView(c *ui.Context) {
	t := theme.Current()
	uikit.Install(c)
	p := t.C.Popover
	c.Root().Background(ui.Transparent)
	sessions := a.trayPanelSessions()
	content := ui.Column(c).Width(t.C.TrayPanel.Width).Shrink(0).Radius(p.Radius).
		Background(uikit.Color(p.Bg)).Label("Burrow").Children(func() {
		a.trayPanelMCP(c)
		if len(sessions) > 0 {
			uikit.Divider(c)
			a.trayPanelSessionList(c, sessions)
		}
		a.trayPanelTransfers(c)
		uikit.Divider(c)
		a.trayPanelActions(c)
	})
	// Native UI does not fit a flyout to its content: follow the column's
	// height as sessions and transfers come and go.
	if height := int(math.Ceil(float64(content.Bounds().H))); height > 0 && height != a.tray.panelHeight {
		a.tray.panelHeight = height
		if a.tray.panel != nil {
			a.tray.panel.SetSize(int(t.C.TrayPanel.Width), height)
		}
	}
	if a.ctx.Err() == nil {
		live := make([]*remoteSession, 0, len(sessions))
		for _, item := range sessions {
			live = append(live, item.session)
		}
		a.sampleSSHLatency(c, live)
	}
	c.After(trayPanelRefresh)
}

func (a *app) trayPanelMCP(c *ui.Context) {
	t := theme.Current()
	status, tone := locales.MessageMcpUnavailable(), uikit.ToneNeutral
	available := a.mcpBackend != nil && a.mcpBackend.service != nil
	running, address := false, ""
	if !a.security.ready {
		status = locales.MessageWaitingForAppUnlock()
	} else if available {
		cfg, active, _, err := a.mcpBackend.service.Status()
		running = active
		status = locales.MessageOff()
		if running {
			status, tone = locales.MessageRunning(), uikit.ToneSuccess
			address = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		}
		if err != nil {
			status, tone = locales.MessageMcpServiceError(), uikit.ToneDanger
		}
	}
	ui.Column(c).FillWidth().Padding(t.Space.N12, t.Space.N14).Gap(t.Space.N12).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N10).Children(func() {
			on := running
			toggle := uikit.Switch(c.Key("tray-mcp"), &on, uikit.ToggleProps{
				Label:    locales.MessageEnableMcpService(),
				Disabled: !available || !a.businessReady() || a.mcpBusy(),
				Motion:   uikit.MotionOverride(a.reduceMotion(c)),
			})
			if toggle.Changed() {
				a.toggleMCPFromTray()
			}
			uikit.Text(c, locales.MessageMcpService(), uikit.TextProps{Style: uikit.TextBodyStrong})
			uikit.Badge(c, status, uikit.BadgeProps{Tone: tone, Dot: true}).Role(ui.RoleStatus).Label(status)
		})
		if !running {
			return
		}
		ui.Row(c).FillWidth().Gap(t.Space.N12).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N4).Children(func() {
				addressCopy := uikit.CopyField(c, address, uikit.CopyFieldProps{
					Key: "tray-mcp-address", Label: locales.MessageCopyListeningAddress(), Plain: true,
				})
				addressCopy.Root.AlignSelf(ui.Start).MinWidth(0)
				if addressCopy.Copied {
					mygo.Clipboard.WriteText("http://" + address + "/mcp")
				}
			})
			calls := locales.MessageCallsToday() + " " + strconv.Itoa(a.mcp.todayCalls)
			uikit.Badge(c, calls, uikit.BadgeProps{Tone: uikit.ToneMuted}).
				Radius(t.Radius.ControlSm).FontFeatures("tnum").Margin(0, 0, t.Space.N2, 0)
		})
	})
}

type trayPanelSession struct {
	tab     *workspaceTab
	session *remoteSession
}

// trayPanelSessions lists the open SSH sessions of every tab, one row for each
// connection, which the panes of a split share.
func (a *app) trayPanelSessions() []trayPanelSession {
	var list []trayPanelSession
	seen := map[any]bool{}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if session.closing || session.state == disconnected {
				continue
			}
			key := any(session)
			if session.connection != nil {
				key = session.connection
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			list = append(list, trayPanelSession{tab: tab, session: session})
		}
	}
	return list
}

func (a *app) trayPanelSessionList(c *ui.Context, sessions []trayPanelSession) {
	t := theme.Current()
	ui.Row(c).FillWidth().Padding(t.Space.N8, t.Space.N14, t.Space.N4, t.Space.N14).AlignItems(ui.Center).Children(func() {
		uikit.Text(c, locales.TrayActiveSessions(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Grow(1)
		uikit.Count(c, len(sessions), uikit.CountProps{})
	})
	ui.Column(c).FillWidth().Padding(0, t.Space.N6, t.Space.N6, t.Space.N6).Children(func() {
		for _, item := range sessions {
			session, h := item.session, item.session.endpoint
			row := uikit.ListRow(c, uikit.ListRowProps{
				Key: session.id, Label: h.name, PaddingY: t.Space.N6,
			}).AlignItems(ui.Center)
			row.Children(func() {
				style, _ := a.hostSystem(h)
				uikit.OsLogo(c, style.name, uikit.SizeDefault)
				uikit.SiderRowLabel(c, h.name, h.user+"@"+h.address)
				if session.state == connecting {
					uikit.StatusPill(c, uikit.StatusConnecting)
				} else {
					uikit.Latency(c, sshLatencyMS(session, c.Now()), uikit.LatencyProps{})
				}
			})
			if row.Clicked() {
				a.openSessionFromTray(item.tab, session)
			}
		}
	})
}

func (a *app) openSessionFromTray(tab *workspaceTab, session *remoteSession) {
	a.closeTrayPanel()
	a.showMainWindow()
	if !a.businessReady() || a.security.dialog != nil || a.windowDialogOpen() {
		return
	}
	focus := func() {
		// A drag that finishes first may have reordered or closed the tab.
		index := slices.Index(a.tabs, tab)
		if index < 0 || session.closing {
			return
		}
		a.switchSection(sectionSSH)
		a.selectTab(index)
		a.focusSession(tab, session)
	}
	if !a.deferWorkspaceDragAction(focus) {
		focus()
	}
	a.window.Invalidate()
}

func (a *app) trayPanelTransfers(c *ui.Context) {
	t := theme.Current()
	active := slices.DeleteFunc(slices.Clone(a.transfers.tasks), func(task *transferTask) bool {
		return task.state == transferDone || task.state == transferFailed
	})
	if len(active) == 0 {
		return
	}
	s := summarizeTransfers(active)
	fraction := float32(0)
	if s.total > 0 {
		fraction = float32(float64(s.bytes) / float64(s.total))
	}
	tone := uikit.ProgressRunning
	if s.running == 0 {
		tone = uikit.ProgressPaused
	}
	uikit.Divider(c)
	ui.Column(c).FillWidth().Padding(t.Space.N10, t.Space.N14).Gap(t.Space.N6).Children(func() {
		label := locales.TrayTransfersInProgress(len(active))
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
			uikit.Text(c, label, uikit.TextProps{Style: uikit.TextFootnote}).Grow(1).SingleLine()
			detail := strconv.Itoa(int(fraction*100)) + "%"
			if s.running > 0 {
				detail += " · " + transferBytes(int64(s.speed)) + "/s"
			}
			uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).
				SingleLine().FontFeatures("tnum")
		})
		uikit.Progress(c, uikit.ProgressProps{
			Value: fraction, Tone: tone, Label: label, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		})
	})
}

func (a *app) trayPanelActions(c *ui.Context) {
	t := theme.Current()
	m := t.C.Menu
	ui.Row(c).FillWidth().Padding(t.Space.N6).Gap(t.Space.N2).AlignItems(ui.Center).Children(func() {
		label := locales.MessageOpenBurrow()
		open := ui.ButtonBase(c.Key("tray-open")).Label(label).Height(m.ItemHeight).Grow(1).MinWidth(0).
			Padding(0, m.ItemPaddingX).Radius(m.ItemRadius).FocusRing(false).Justify(ui.Start)
		background := ui.Transparent
		if open.Hovered() || open.FocusVisible() {
			background = uikit.Color(m.ItemHover)
		}
		open.Background(background).TextColor(uikit.Color(m.ItemHoverText)).Children(func() {
			ui.Text(c, label).FontSize(m.FontSize).SingleLine().PassThrough()
		})
		if open.Clicked() {
			a.closeTrayPanel()
			a.showMainWindow()
		}
		if uikit.IconButton(c, uikit.IconSettings, uikit.IconButtonProps{
			Key: "tray-settings", Label: locales.SettingsTitle(), Disabled: !a.businessReady(),
		}).Clicked() {
			a.closeTrayPanel()
			a.openSettingsFromMenu()
		}
		// Quit runs the same quit sequence as the native menu's Quit role.
		if uikit.IconButton(c, uikit.IconPower, uikit.IconButtonProps{
			Key: "tray-quit", Label: locales.MessageQuitApp(),
		}).Clicked() {
			a.closeTrayPanel()
			mygo.App.Quit()
		}
	})
}
