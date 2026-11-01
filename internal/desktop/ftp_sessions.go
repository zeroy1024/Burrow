package desktop

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fileSession struct {
	id         int
	host       hostID
	endpoint   host
	temporary  bool
	connection *sshConnectionGroup
	runtime    *fileRuntime
	sshPeer    *remoteSession
	state      connectionState
	errorText  string
	local      *filePaneState
	remote     *filePaneState
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
		if !session.temporary && session.host == id {
			a.activeFile = i
			return
		}
	}
	session := a.newFileSession(id, a.fileCatalog.hosts[id], false, nil)
	a.restoreFileSession(session)
	a.fileSessions = append(a.fileSessions, session)
	a.activeFile = len(a.fileSessions) - 1
	a.reconnectFileSession(session)
}

func (a *app) newFileSession(id hostID, endpoint host, temporary bool, connection *sshConnectionGroup) *fileSession {
	a.nextFileSession++
	if endpoint.protocol == "" {
		endpoint.protocol = "sftp"
	}
	if endpoint.protocol == "sftp" && connection == nil {
		connection = a.newSSHGroup(endpoint)
	}
	return &fileSession{id: a.nextFileSession, host: id, endpoint: endpoint, temporary: temporary,
		connection: connection, local: newFilePane(false), remote: newFilePane(true)}
}

func (a *app) closeFileSession(index int) {
	a.disconnectFileSession(a.fileSessions[index])
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
						for _, group := range a.fileCatalog.groups {
							m.Submenu(group.name, func(sub *ui.Menu) {
								for _, id := range group.hosts {
									if sub.Item(a.fileCatalog.hosts[id].name).Chosen() {
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
		session := a.currentFileSession()
		iconButton(c, "打开 SSH 会话", terminalIcon).Disabled(session == nil || session.state != connected).
			OnClick(func() { a.openSSHForFiles(session) })
	})
}

func (a *app) fileTabButton(c *ui.Context, index int, session *fileSession) bool {
	t := theme.Current()
	h := session.endpoint
	selected := a.activeFile == index
	foreground := tokenColor(t.C.Tab.Text)
	if selected {
		foreground = tokenColor(t.C.Tab.TextSelected)
	}
	hint := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
	switch session.state {
	case connecting:
		hint = "正在连接 " + h.address + "…"
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
				a.disconnectFileSession(session)
			}
		} else if m.Item("重新连接").Chosen() {
			a.reconnectFileSession(session)
		}
		m.Separator()
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
	h := a.catalog().hosts[id]
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
				a.disconnectFileSession(session)
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
		h := a.fileCatalog.hosts[id]
		protocol := "sftp"
		if h.protocol == "ftp" {
			protocol = "ftp"
		}
		mygo.Clipboard.WriteText(fmt.Sprintf("%s://%s@%s:%d", protocol, h.user, h.address, h.port))
	}
	m.Separator()
	if m.Item("删除站点…").Chosen() {
		a.page().draft = hostDraft{id: id, deleting: true}
		a.page().addHostOpen = true
	}
}

func (a *app) startEditHostDraft(id hostID) {
	h := a.catalog().hosts[id]
	group := ""
	for _, candidate := range a.catalog().groups {
		if slices.Contains(candidate.hosts, id) {
			group = candidate.name
			break
		}
	}
	protocol := "SFTP"
	if h.protocol == "ftp" {
		protocol = "FTP"
	}
	auth := h.auth
	if auth == "" {
		auth = "agent"
	}
	a.page().draft = hostDraft{
		id: id, editing: true, files: a.section == sectionFiles, name: h.name, address: h.address,
		user: h.user, port: strconv.Itoa(h.port), group: group, system: systems[h.os].name, protocol: protocol,
		auth: auth, keyPath: h.keyPath,
	}
	a.page().addHostOpen = true
}

func (a *app) deleteCatalogHost(id hostID) error {
	return a.commitCatalogMove(func() { a.catalog().removeHost(id) })
}

func (a *app) deleteSiteDialogContents(c *ui.Context) {
	t := theme.Current()
	kind := "主机"
	if a.section == sectionFiles {
		kind = "站点"
	}
	id := a.page().draft.id
	h := a.catalog().hosts[id]
	ui.Text(c, "删除"+kind+"「"+h.name+"」？").FontSize(t.C.Dialog.TitleSize).
		FontWeight(t.Font.Weight.Semibold)
	ui.Text(c, "将从当前目录移除此条目，已打开的会话会保留到你手动关闭。").
		FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
	ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
		button(c, "取消").OnClick(func() { a.page().addHostOpen = false })
		button(c, "删除"+kind).TextColor(tokenColor(t.C.Tab.TextErrorSelected)).OnClick(func() {
			if err := a.deleteCatalogHost(id); err != nil {
				c.Toast("删除" + kind + "失败：" + err.Error())
				return
			}
			a.page().addHostOpen = false
		})
	})
}
