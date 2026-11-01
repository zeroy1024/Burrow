package desktop

import (
	"fmt"
	"math"
	"slices"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type connectionState int

const (
	disconnected connectionState = iota
	connected
	connecting
	connectionFailed
)

type hostID int

type host struct {
	session             sessionConfig
	name, user, address string
	protocol            string
	system, uptime      string
	os, port            int
	latency             int
	cpu, memory, disk   float32
}

type workspaceTab struct {
	id                 int
	root               *terminalLayout
	focused            *remoteSession
	maximized          bool
	frame              ui.Handle
	zoomFrom, zoomMix  float32
	zoomStarted        time.Time
	zoomPending        bool
	layoutDuration     time.Duration
	layoutPending      bool
	layoutUntil        time.Time
	workspaceSize      ui.Point
	workspaceResizedAt time.Time
}

func (tab *workspaceTab) canSeparateSessions() bool {
	sessions := tab.root.sessions()
	if len(sessions) < 2 {
		return false
	}
	for _, session := range sessions {
		if session.closing {
			return false
		}
	}
	return true
}

type remoteSession struct {
	config                      sessionConfig
	id                          int
	host                        hostID
	state                       connectionState
	connectionStarted           time.Time
	connectionStep              int
	scroll                      terminalScrollState
	focus                       ui.Handle
	transcript                  []terminalLine
	output                      []terminalLine
	textState                   terminalTextState
	hasNewOutput                bool
	hasBell                     bool
	cursorStarted               time.Time
	frame                       ui.Handle
	layoutRect                  ui.Rect
	contentWidth, contentHeight float32
	columns, rows               int
	enter                       dropZone
	entering                    bool
	enterStarted                time.Time
	closing                     bool
	closeStarted                time.Time
	revealing                   bool
	revealStarted               time.Time
}

func demoHosts() []host {
	hosts := []host{
		{name: "prod-web-01", user: "root", address: "10.0.1.12", os: 0,
			latency: 18, system: "Ubuntu 22.04.4", uptime: "41 天 6 小时", cpu: 23, memory: 41, disk: 57},
		{name: "prod-web-02", user: "root", address: "10.0.1.13", os: 0,
			latency: 21, system: "Ubuntu 22.04.4", uptime: "28 天 3 小时", cpu: 16, memory: 35, disk: 43},
		{name: "prod-db-master", user: "admin", address: "10.0.2.5", os: 1,
			latency: 9, system: "Debian 12", uptime: "64 天 2 小时", cpu: 12, memory: 62, disk: 71},
		{name: "cdn-origin", user: "deploy", address: "10.0.3.8", os: 2,
			latency: 32, system: "CentOS Stream 9", uptime: "17 天 4 小时", cpu: 8, memory: 24, disk: 38},
		{name: "staging-api", user: "ubuntu", address: "172.16.4.21", os: 0,
			latency: 46, system: "Ubuntu 24.04", uptime: "12 天 4 小时", cpu: 31, memory: 27, disk: 32},
		{name: "bastion", user: "ops", address: "bastion.example.com", os: 3,
			latency: 12, system: "Alpine Linux 3.21", uptime: "23 天 8 小时", cpu: 4, memory: 18, disk: 22},
		{name: "qa-runner", user: "ci", address: "172.16.1.40", os: 4,
			latency: 28, system: "Fedora 42", uptime: "3 天 1 小时", cpu: 38, memory: 48, disk: 44},
		{name: "nas.home", user: "zero", address: "192.168.1.10", os: 5,
			latency: 2, system: "Linux 6.6", uptime: "78 天 9 小时", cpu: 6, memory: 32, disk: 68},
		{name: "vps-tokyo", user: "root", address: "45.77.12.9", os: 6,
			latency: 54, system: "Arch Linux", uptime: "9 天 5 小时", cpu: 11, memory: 29, disk: 35},
	}
	for i := range hosts {
		hosts[i].port = 22
	}
	return hosts
}

func (a *app) currentTab() *workspaceTab {
	if len(a.tabs) == 0 {
		return nil
	}
	return a.tabs[a.activeTab]
}

func (a *app) focusedHost() (hostID, bool) {
	tab := a.currentTab()
	if tab == nil {
		return 0, false
	}
	return tab.focused.host, true
}

func (a *app) newSession(id hostID) *remoteSession {
	a.nextSession++
	session := &remoteSession{
		id: a.nextSession, host: id,
		transcript: []terminalLine{}, output: []terminalLine{},
		scroll: terminalScrollState{offset: ui.ScrollState{Y: math.MaxFloat32}},
	}
	a.connectSession(session)
	return session
}

func (a *app) connectSession(session *remoteSession) {
	h := a.hosts[session.host]
	session.config = h.session
	session.state, session.connectionStarted, session.connectionStep = connecting, time.Time{}, 0
	session.hasNewOutput, session.hasBell = false, false
	session.scroll.offset.Y = math.MaxFloat32
	appendSessionLines(session,
		terminalLine{{Text: fmt.Sprintf("[演示] ssh -p %d %s@%s", h.port, h.user, h.address), Foreground: terminalIndexed(4)}},
		terminalLine{{Text: fmt.Sprintf("正在连接 %s:%d…", h.address, h.port)}},
	)
}

func disconnectSession(session *remoteSession) {
	message := "会话已断开。可从标签菜单重新连接。"
	if session.state == connecting {
		message = "连接已取消。可从标签菜单重新连接。"
	}
	session.state = disconnected
	appendSessionLines(session, terminalLine{{Text: message, Foreground: terminalIndexed(8)}})
}

// The prototype advances connection stages on the UI clock, shared by tabs and Shell text.
func (a *app) updateSSHConnections(c *ui.Context) {
	interval := theme.Current().C.ConnectionIndicator.StageDuration
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if session.state != connecting || session.closing {
				continue
			}
			if session.connectionStarted.IsZero() {
				session.connectionStarted = c.Now()
			}
			h := a.hosts[session.host]
			steps := []terminalLine{
				{{Text: "连接已建立，正在进行 SSH 握手…"}},
				{{Text: fmt.Sprintf("握手完成，正在验证用户 %s…", h.user)}},
				{{Text: "身份验证通过，正在启动远程 Shell…"}},
				{{Text: fmt.Sprintf("已连接到 %s（演示会话）", h.name), Foreground: terminalIndexed(2)}},
			}
			elapsed := c.Now().Sub(session.connectionStarted)
			step := min(int(elapsed/interval), len(steps))
			for session.connectionStep < step {
				appendSessionLines(session, steps[session.connectionStep])
				session.connectionStep++
			}
			if step == len(steps) {
				session.state = connected
				session.cursorStarted = c.Now()
				appendSessionLines(session, terminalLine{})
				appendSessionLines(session, demoTranscript(h)...)
				continue
			}
			c.After(time.Duration(step+1)*interval - elapsed)
		}
	}
}

func appendSessionLines(session *remoteSession, lines ...terminalLine) {
	if session.scroll.offset.Y >= session.scroll.offset.MaxY {
		session.scroll.offset.Y = math.MaxFloat32
	}
	session.output = append(session.output, lines...)
}

func (a *app) focusSession(tab *workspaceTab, session *remoteSession) {
	tab.focused = session
	session.focus.Focus()
	session.markViewed()
}

func (session *remoteSession) markViewed() {
	session.hasNewOutput, session.hasBell = false, false
	session.cursorStarted = time.Time{}
}

func (a *app) activateTab(index int) {
	a.selectTab(index)
	a.tabs[index].focused.focus.Focus()
}

func (a *app) selectTab(index int) {
	a.settingsUI.previousFocus = ui.Handle{}
	a.closeSettings()
	a.activeTab = index
	a.tabs[index].focused.markViewed()
}

// The prototype receives complete output lines; BEL is an activity event, not printable text.
func (a *app) receiveSessionOutput(session *remoteSession, text string) {
	background := a.currentTab().focused != session
	lines, bell := session.textState.parse(text)
	if background && bell {
		session.hasBell = true
	}
	if len(lines) == 0 {
		return
	}
	appendSessionLines(session, lines...)
	if background {
		session.hasNewOutput = true
	}
}

func (a *app) openHost(id hostID) {
	a.nextTab++
	session := a.newSession(id)
	a.tabs = append(a.tabs, &workspaceTab{
		id: a.nextTab, root: &terminalLayout{session: session}, focused: session,
	})
	a.activateTab(len(a.tabs) - 1)
}

func (a *app) ungroupTab(tab *workspaceTab) {
	index := slices.Index(a.tabs, tab)
	if index < 0 || !tab.canSeparateSessions() {
		return
	}
	active, focused := a.currentTab(), tab.focused
	sessions := tab.root.sessions()
	separated := make([]*workspaceTab, 0, len(sessions))
	*tab = workspaceTab{
		id: tab.id, frame: tab.frame,
		root: &terminalLayout{session: sessions[0]}, focused: sessions[0],
	}
	separated = append(separated, tab)
	for _, session := range sessions[1:] {
		a.nextTab++
		separated = append(separated, &workspaceTab{
			id: a.nextTab, root: &terminalLayout{session: session}, focused: session,
		})
	}
	a.tabs = slices.Replace(a.tabs, index, index+1, separated...)
	if active != tab {
		a.activeTab = slices.Index(a.tabs, active)
		return
	}
	a.resizing = nil
	a.activateTab(index + slices.Index(sessions, focused))
}

func (a *app) detachPaneToTab(source *workspaceTab, session *remoteSession, index int, area ui.Rect) *workspaceTab {
	if !source.canSeparateSessions() || slices.Index(a.tabs, source) < 0 {
		return nil
	}
	if source.root.find(session) == nil {
		return nil
	}
	if index < 0 || index > len(a.tabs) {
		return nil
	}
	minimum := source.root.minimum()
	canvas := ui.Rect{W: max(area.W, minimum.X), H: max(area.H, minimum.Y)}
	remaining := source.root.copyAt(canvas).remove(session)
	focused := source.focused
	if focused == session {
		focused = remaining.firstSession()
	}
	*source = workspaceTab{
		id: source.id, frame: source.frame, root: remaining, focused: focused,
	}
	a.markPaneLayout(source, theme.Current().C.ReorderMotion.PaneDrop.Duration)
	a.nextTab++
	tab := &workspaceTab{
		id: a.nextTab, root: &terminalLayout{session: session}, focused: session,
	}
	a.tabs = slices.Insert(a.tabs, index, tab)
	a.resizing = nil
	a.activateTab(index)
	return tab
}

func (a *app) closeTab(index int) {
	if a.tabs[index].root != nil {
		a.closedTabs = append(a.closedTabs, closedWorkspaceTab{host: a.tabs[index].focused.host, section: sectionSSH})
	}
	closedActive := index == a.activeTab
	a.tabs = append(a.tabs[:index], a.tabs[index+1:]...)
	switch {
	case len(a.tabs) == 0:
		a.activeTab = 0
	case index < a.activeTab:
		a.activeTab--
	case a.activeTab >= len(a.tabs):
		a.activeTab = len(a.tabs) - 1
	}
	if closedActive && len(a.tabs) > 0 && a.activeSection() == sectionSSH {
		a.focusWorkspace()
	}
}

func (a *app) splitSession(session *remoteSession, stacked bool) {
	tab := a.currentTab()
	if tab.root.count() >= maxTerminalPanes {
		return
	}
	zone := dropRight
	if stacked {
		zone = dropBottom
	}
	if !canSplit(session.layoutRect, zone) {
		if stacked {
			zone = dropRight
		} else {
			zone = dropBottom
		}
		if !canSplit(session.layoutRect, zone) {
			return
		}
	}
	child := a.newSession(session.host)
	node := tab.root.find(session)
	node.insert(child, zone)
	child.enter, child.entering = zone, true
	a.markPaneLayout(tab, theme.Current().C.Desktop.PaneEnterDuration)
	a.focusSession(tab, child)
	a.setPaneZoom(tab, false)
}

func (a *app) closePane(session *remoteSession) {
	available := 0
	for _, candidate := range a.currentTab().root.sessions() {
		if !candidate.closing {
			available++
		}
	}
	if available <= 1 || session.closing {
		return
	}
	session.closing = true
}

func (a *app) removePane(tab *workspaceTab, session *remoteSession) {
	tab.root = tab.root.remove(session)
	if tab.focused == session {
		if tab == a.currentTab() && a.activeSection() == sectionSSH {
			a.focusSession(tab, tab.root.firstSession())
		} else {
			tab.focused = tab.root.firstSession()
		}
	}
	a.markPaneLayout(tab, theme.Current().C.Desktop.PaneEnterDuration)
	a.setPaneZoom(tab, false)
}
