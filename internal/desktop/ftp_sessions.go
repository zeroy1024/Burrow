package desktop

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fileSession struct {
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

func (a *app) openCatalogHost(id hostID) {
	if a.section == sectionFiles {
		a.openFileHost(id)
		return
	}
	a.openHost(id)
}

func (a *app) openFileHost(id hostID) {
	for i, session := range a.fileSessions {
		if session.host == id {
			a.activeFile = i
			return
		}
	}
	a.nextFileSession++
	session := &fileSession{
		id: a.nextFileSession, host: id, state: connecting, connectedAt: time.Now(),
		local: newFilePane(false), remote: newFilePane(true),
	}
	a.restoreFileSession(session)
	a.fileSessions = append(a.fileSessions, session)
	a.activeFile = len(a.fileSessions) - 1
}

func (a *app) reconnectFileSession(session *fileSession) {
	session.state, session.errorText = connecting, ""
	session.connectedAt = time.Now()
}

func (a *app) closeFileSession(index int) {
	a.fileSessions = slices.Delete(a.fileSessions, index, index+1)
	switch {
	case len(a.fileSessions) == 0:
		a.activeFile = 0
	case index < a.activeFile:
		a.activeFile--
	case a.activeFile >= len(a.fileSessions):
		a.activeFile = len(a.fileSessions) - 1
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
	a.updateFileConnections(c)
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
						for _, group := range a.groups {
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
						a.closeFileSession(slices.Index(a.fileSessions, closing))
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
	button.OnClick(func() { a.activeFile = slices.Index(a.fileSessions, session) })
	closing := false
	button.ContextMenu(func(m *ui.Menu) {
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
			OnClick(func() { a.closeFileSession(slices.Index(a.fileSessions, session)) })
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

func (a *app) fileSiteMenu(c *ui.Context, m *ui.Menu, id hostID) {
	sessionIndex := slices.IndexFunc(a.fileSessions, func(session *fileSession) bool { return session.host == id })
	if sessionIndex < 0 {
		if m.Item("连接站点").Chosen() {
			a.openFileHost(id)
		}
	} else {
		session := a.fileSessions[sessionIndex]
		if m.Item("切换到文件会话").Chosen() {
			a.activeFile = sessionIndex
		}
		if session.state == connected || session.state == connecting {
			if m.Item("断开连接").Chosen() {
				session.state = disconnected
			}
		} else if m.Item("重新连接").Chosen() {
			a.reconnectFileSession(session)
		}
	}
	m.Separator()
	if m.Item("编辑站点").Chosen() {
		a.startEditHostDraft(id)
	}
	if m.Item("复制连接地址").Chosen() {
		h := a.hosts[id]
		protocol := "sftp"
		if h.protocol == "ftp" {
			protocol = "ftp"
		}
		mygo.Clipboard.WriteText(fmt.Sprintf("%s://%s@%s:%d", protocol, h.user, h.address, h.port))
	}
	m.Separator()
	if m.Item("删除站点…").Chosen() {
		if a.fileSiteBusy(id) {
			c.Toast("请先取消或清除该站点的传输任务")
			return
		}
		a.page().draft = hostDraft{id: id, deleting: true}
		a.page().addHostOpen = true
	}
}

func (a *app) startEditHostDraft(id hostID) {
	h := a.hosts[id]
	group := ""
	for _, candidate := range a.groups {
		if slices.Contains(candidate.hosts, id) {
			group = candidate.name
			break
		}
	}
	protocol := "SFTP"
	if h.protocol == "ftp" {
		protocol = "FTP"
	}
	a.page().draft = hostDraft{
		id: id, editing: true, files: true, name: h.name, address: h.address,
		user: h.user, port: strconv.Itoa(h.port), group: group, system: systems[h.os].name, protocol: protocol,
	}
	a.page().addHostOpen = true
}

func (a *app) deleteFileSite(id hostID) error {
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
	return nil
}

func (a *app) deleteSiteDialogContents(c *ui.Context) {
	t := theme.Current()
	id := a.page().draft.id
	h := a.hosts[id]
	ui.Text(c, "删除站点「"+h.name+"」？").FontSize(t.C.Dialog.TitleSize).
		FontWeight(t.Font.Weight.Semibold)
	ui.Text(c, "将从共享主机目录移除，并关闭此主机的 SSH 与文件会话。服务器文件不受影响。").
		FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		button(c, "取消").OnClick(func() { a.page().addHostOpen = false })
		button(c, "删除站点").TextColor(tokenColor(t.C.Tab.TextErrorSelected)).OnClick(func() {
			if a.fileSiteBusy(id) {
				c.Toast("请先取消或清除该站点的传输任务")
				return
			}
			if err := a.deleteFileSite(id); err != nil {
				c.Toast("删除站点失败：" + err.Error())
				return
			}
			a.page().addHostOpen = false
		})
	})
}
