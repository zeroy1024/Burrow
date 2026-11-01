package desktop

import (
	"fmt"
	"math"
	"slices"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fileSession struct {
	config      sessionConfig
	id          int
	host        hostID
	state       connectionState
	errorText   string
	connectedAt time.Time
	local       *filePaneState
	remote      *filePaneState
}

func (a *app) currentFileSession() *fileSession {
	if len(a.fileSessions) == 0 {
		return nil
	}
	return a.fileSessions[a.activeFile]
}

func (a *app) activateFileTab(index int) {
	a.settingsUI.previousFocus = ui.Handle{}
	a.closeSettings()
	a.activeFile = index
	a.focusWorkspace()
}

func (a *app) openCatalogHost(id hostID) {
	if a.section == sectionFiles {
		a.openFileHost(id)
		return
	}
	a.openHost(id)
}

func (a *app) openFileHost(id hostID) {
	a.closeSettings()
	for i, session := range a.fileSessions {
		if session.host == id {
			a.activateFileTab(i)
			return
		}
	}
	a.openNewFileHost(id)
}

func (a *app) openNewFileHost(id hostID) {
	a.nextFileSession++
	session := &fileSession{
		config: a.hosts[id].session,
		id:     a.nextFileSession, host: id, state: connecting, connectedAt: time.Now(),
		local: newFilePane(false), remote: newFilePane(true),
	}
	a.restoreFileSession(session)
	a.fileSessions = append(a.fileSessions, session)
	a.activeFile = len(a.fileSessions) - 1
	if a.activeSection() == sectionFiles {
		a.focusWorkspace()
	}
}

func (a *app) reconnectFileSession(session *fileSession) {
	session.config = a.hosts[session.host].session
	a.restoreFileSession(session)
	session.state, session.errorText = connecting, ""
	session.connectedAt = time.Now()
}

func (a *app) closeFileSession(index int) {
	closedActive := index == a.activeFile
	a.fileSessions = slices.Delete(a.fileSessions, index, index+1)
	switch {
	case len(a.fileSessions) == 0:
		a.activeFile = 0
	case index < a.activeFile:
		a.activeFile--
	case a.activeFile >= len(a.fileSessions):
		a.activeFile = len(a.fileSessions) - 1
	}
	if closedActive && len(a.fileSessions) > 0 && a.activeSection() == sectionFiles {
		a.focusWorkspace()
	}
}

func (a *app) updateFileConnections(c *ui.Context) {
	for _, session := range a.fileSessions {
		if session.state != connecting {
			continue
		}
		remaining := theme.Current().C.Tab.SpinnerPeriod - c.Now().Sub(session.connectedAt)
		if remaining <= 0 {
			session.state = connected
			continue
		}
		c.After(remaining)
	}
}

func (a *app) fileTitleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	bar := c.TitleBar()
	ui.Row(c).Height(max(t.C.Titlebar.Height, bar.Height)).Shrink(0).
		Gap(t.Space.N6).DragWindow().Children(func() {
		ui.Row(c).Width(max(t.C.Rail.Width+a.page().sidebarWidth*shown, bar.Left+t.C.Titlebar.Height)).
			Shrink(0).PaddingStart(bar.Left + t.C.Titlebar.LeftGap).DragWindow().Children(func() {
			label := "收起站点栏"
			if a.page().sidebarCollapsed {
				label = "展开站点栏"
			}
			keyedIconButton(c, "sidebar-toggle", label, sidebarIcon).
				Disabled(a.activeSection() == sectionSettings).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		ui.Box(c).Grow(1).MinWidth(0).Height(t.C.Tab.StripHeight).Clip().Children(func() {
			ui.ScrollHorizontal(c.Key("file-session-tabs")).Fill().ScrollbarInsets(
				0,
				0,
				-t.Space.N12,
				0,
			).Children(func() {
				ui.Row(c).Gap(t.C.Titlebar.TabsGap).FillHeight().Children(func() {
					var closing *fileSession
					for index, session := range a.fileSessions {
						if a.fileTabButton(c, index, session) {
							closing = session
						}
					}
					iconButton(c, "新建文件会话", plusIcon).Menu(func(m *ui.Menu) {
						for _, group := range a.catalogGroups() {
							m.Submenu(group.name, func(sub *ui.Menu) {
								for _, id := range group.hosts {
									if sub.Item(a.hosts[id].name).Chosen() {
										a.openFileHost(id)
									}
								}
							})
						}
					})
					if closing != nil {
						a.requestCloseFileTab(slices.Index(a.fileSessions, closing))
					}
				})
			})
		})
	})
}

func (a *app) fileTabButton(c *ui.Context, index int, session *fileSession) bool {
	t := theme.Current()
	h := a.hosts[session.host]
	selected := a.activeFile == index
	foreground := tokenColor(t.C.Tab.Text)
	if selected {
		foreground = tokenColor(t.C.Tab.TextSelected)
	}
	hint := fmt.Sprintf("%s@%s:%d · 演示会话", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…（演示）"
	case disconnected:
		hint = "已断开 · 选择后可重新连接"
		foreground = tokenColor(t.C.Tab.TextDisconnected)
	case connectionFailed:
		hint = session.errorText
		if selected {
			foreground = tokenColor(t.C.Tab.TextErrorSelected)
		}
	}
	button := ui.ButtonBase(c).Key(session).Height(t.C.Tab.Height).
		Padding(0, t.C.Tab.PaddingRight, 0, t.C.Tab.PaddingLeft).
		Gap(t.C.Tab.Gap).Radius(t.C.Tab.Radius).AlignItems(ui.Center).
		Role(ui.RoleTab).Label(h.name).Selected(selected)
	tooltip(c, button, hint)
	button.TextColor(button.AnimateColor("text", foreground, tokenTween(t.C.Tab.StateTransition)))
	background := ui.Transparent
	if selected {
		background = tokenColor(t.C.Tab.BgSelected)
		applyShadow(button, t.C.Tab.ShadowSelected)
	} else if button.Hovered() {
		background = tokenColor(t.C.Tab.BgHover)
	}
	button.Background(button.AnimateColor("background", background, tokenTween(t.C.Tab.StateTransition)))
	button.OnClick(func() {
		a.activateFileTab(slices.Index(a.fileSessions, session))
	})
	closing := false
	button.ContextMenu(func(m *ui.Menu) {
		if m.Item("编辑会话…").Chosen() {
			a.startEditHostDraft(session.host)
		}
		m.Separator()
		if session.state == connected || session.state == connecting {
			if m.Item("断开文件会话").Chosen() {
				session.state = disconnected
			}
		} else if m.Item("重新连接").Chosen() {
			a.reconnectFileSession(session)
		}
		m.Separator()
		if m.Item("模拟连接失败").Chosen() {
			session.state, session.errorText = connectionFailed, "认证被拒绝（演示错误）"
		}
		m.Separator()
		if m.Item("关闭选项卡").Chosen() {
			closing = true
		}
	})
	button.Children(func() {
		ui.Box(c).Width(t.C.Tab.SpinnerSize).Height(t.C.Tab.ContentHeight).Shrink(0).
			Center().Children(func() { tabConnectionIndicator(c, session.state) })
		ui.Text(c, h.name).FontSize(t.C.Tab.FontSize).FontWeight(t.C.Tab.WeightSelected).
			FixedLineHeight(t.C.Tab.ContentHeight).Top(t.C.Tab.TitleOffset).SingleLine()
		close := iconButton(c, "关闭文件标签 "+h.name, closeIcon).Size(t.Size.Icon, t.C.Tab.ContentHeight)
		visible := float32(0)
		if selected || button.Hovered() || close.Focused() {
			visible = 1
		}
		close.Opacity(animateVisual(close, "visible", visible, t.C.Tab.StateTransition)).
			OnClick(func() { a.requestCloseFileTab(slices.Index(a.fileSessions, session)) })
	})
	return closing
}

func tabConnectionIndicator(c *ui.Context, state connectionState) {
	t := theme.Current().C.Tab
	reduceMotion := c.Preferences().ReduceMotion
	color := tokenColor(t.DotConnected)
	switch state {
	case disconnected:
		color = tokenColor(t.RingDisconnected)
	case connectionFailed:
		color = tokenColor(t.DotError)
	case connecting:
		color = tokenColor(t.Spinner)
	}
	indicator := ui.Box(c).Key("connection-indicator").Size(t.SpinnerSize, t.SpinnerSize)
	color = indicator.AnimateColor("state", color, tokenTween(t.StateTransition))
	filledTarget, ringTarget, spinnerTarget := float32(0), float32(0), float32(0)
	switch {
	case state == disconnected:
		ringTarget = 1
	case state == connecting && !reduceMotion:
		spinnerTarget = 1
	default:
		filledTarget = 1
	}
	filled := animateVisual(indicator, "filled", filledTarget, t.StateTransition)
	ring := animateVisual(indicator, "ring", ringTarget, t.StateTransition)
	spinner := animateVisual(indicator, "spinner", spinnerTarget, t.StateTransition)
	indicator.Draw(func(p *ui.Painter, r ui.Rect) {
		alpha := func(color ui.Color, opacity float32) ui.Color {
			color.A = uint8(float32(color.A) * opacity)
			return color
		}
		small := ui.Rect{
			X: r.X + (r.W-t.DotSize)/2, Y: r.Y + (r.H-t.DotSize)/2,
			W: t.DotSize, H: t.DotSize,
		}
		if filled > 0 {
			p.Fill(small, alpha(color, filled), t.DotSize/2)
		}
		if ring > 0 {
			p.Stroke(small, alpha(tokenColor(t.RingDisconnected), ring), t.DotSize/2, t.RingWidth)
		}
		if spinner == 0 {
			return
		}
		var track ui.Path
		track.Circle(r.X+r.W/2, r.Y+r.H/2, (r.W-t.SpinnerStroke)/2)
		p.StrokePath(&track, t.SpinnerStroke, alpha(tokenColor(t.SpinnerTrack), spinner))
		phase := float32(p.Now().UnixNano()%int64(t.SpinnerPeriod)) / float32(t.SpinnerPeriod)
		var arc ui.Path
		cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, (r.W-t.SpinnerStroke)/2
		steps := 16
		for i := range steps + 1 {
			angle := float64((phase + float32(i)/float32(steps)/4) * 2 * math.Pi)
			x, y := cx+radius*float32(math.Cos(angle)), cy+radius*float32(math.Sin(angle))
			if i == 0 {
				arc.MoveTo(x, y)
			} else {
				arc.LineTo(x, y)
			}
		}
		p.StrokePath(&arc, t.SpinnerStroke, alpha(tokenColor(t.Spinner), spinner))
		if state == connecting && !reduceMotion {
			p.AnimationFrame()
		}
	})
}

func (a *app) catalogHostIcon(c *ui.Context, id hostID) {
	t := theme.Current()
	h := a.hosts[id]
	if a.section == sectionFiles {
		p := t.C.ProtocolIcon
		background, foreground, icon := p.SftpBg, p.SftpGlyph, folderLockIcon
		if h.protocol == "ftp" {
			background, foreground, icon = p.FtpBg, p.FtpGlyph, folderIcon
		}
		ui.Box(c).Size(p.Size, p.Size).Shrink(0).Radius(p.Radius).Background(tokenColor(background)).
			Center().Children(func() { ui.Icon(c, icon).Size(p.Glyph, p.Glyph).TextColor(tokenColor(foreground)) })
		return
	}
	style := systems[h.os]
	ui.Box(c).Size(t.C.OsLogo.Size, t.C.OsLogo.Size).Shrink(0).Radius(t.C.OsLogo.Radius).
		Background(tokenColor(t.OSLogoBg(style.color))).Center().Children(func() {
		ui.Image(c, style.icon).Size(t.C.OsLogo.GlyphSize, t.C.OsLogo.GlyphSize).TextColor(tokenColor(style.color))
	})
}

func (a *app) deleteFileSite(id hostID) error {
	deleted := a.hosts[id]
	for _, h := range a.hosts {
		if h.session.SSH != nil && h.session.SSH.JumpHostID != "" && h.session.SSH.JumpHostID == deleted.session.ID {
			return fmt.Errorf("会话正被「%s」用作跳板机，请先修改该会话", h.name)
		}
	}
	if err := a.commitCatalogMove(func() {
		a.hosts = slices.Delete(a.hosts, int(id), int(id)+1)
		for _, group := range a.groups {
			group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
			for i, candidate := range group.hosts {
				if candidate > id {
					group.hosts[i]--
				}
			}
		}
	}); err != nil {
		return err
	}
	// Hidden page drafts still refer to the compacted host array.
	for i := range a.pages {
		page := &a.pages[i]
		if !page.draft.editing && !page.draft.deleting {
			continue
		}
		if page.draft.id == id {
			page.draft.cancelTest()
			page.draft, page.addHostOpen = hostDraft{}, false
		} else if page.draft.id > id {
			page.draft.id--
		}
	}
	for i := len(a.fileSessions) - 1; i >= 0; i-- {
		session := a.fileSessions[i]
		if session.host == id {
			a.closeFileSession(i)
		}
	}
	remainingFiles := map[*fileSession]bool{}
	for _, session := range a.fileSessions {
		remainingFiles[session] = true
	}
	for _, task := range a.transfers.tasks {
		remainingFiles[task.session] = true
	}
	for session := range remainingFiles {
		if session.host > id {
			session.host--
		}
	}
	for i := len(a.tabs) - 1; i >= 0; i-- {
		tab := a.tabs[i]
		for _, session := range tab.root.sessions() {
			if session.host == id {
				tab.root = tab.root.remove(session)
				if tab.focused == session && tab.root != nil {
					tab.focused = tab.root.firstSession()
				}
			} else if session.host > id {
				session.host--
			}
		}
		if tab.root == nil {
			a.closeTab(i)
		}
	}
	a.closedTabs = slices.DeleteFunc(a.closedTabs, func(candidate closedWorkspaceTab) bool { return candidate.host == id })
	for i, candidate := range a.closedTabs {
		if candidate.host > id {
			a.closedTabs[i].host--
		}
	}
	if deleted.session.PasswordSaved {
		if err := mygo.Secrets.Delete("session/" + deleted.session.ID + "/password"); err != nil {
			a.files.notice = "会话已移除，但钥匙串清理失败：" + err.Error()
		}
	}
	return nil
}

func (a *app) deleteSiteDialogContents(c *ui.Context) {
	t := theme.Current()
	id := a.page().draft.id
	h := a.hosts[id]
	ui.Text(c, "删除会话「"+h.name+"」？").FontSize(t.C.Dialog.TitleSize).
		FontWeight(t.Font.Weight.Semibold)
	ui.Text(c, "将从当前页面的目录移除，并关闭此条目的会话。服务器文件不受影响。").
		FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		button(c, "取消").OnClick(func() { a.page().addHostOpen = false })
		button(c, "删除").TextColor(tokenColor(t.C.Tab.TextErrorSelected)).OnClick(func() {
			if a.fileSiteBusy(id) {
				c.Toast("请先取消或清除该会话的传输任务")
				return
			}
			if err := a.deleteFileSite(id); err != nil {
				c.Toast("删除会话失败：" + err.Error())
				return
			}
			a.page().addHostOpen = false
		})
	})
}
