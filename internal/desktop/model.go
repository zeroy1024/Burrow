package desktop

import (
	"math"
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
	name, user, address string
	protocol            string
	system, uptime      string
	os, port            int
	latency             int
	cpu, memory, disk   float32
}

type workspaceTab struct {
	id                int
	root              *terminalLayout
	focused           *remoteSession
	maximized         bool
	frame             ui.Handle
	zoomFrom, zoomMix float32
	zoomStarted       time.Time
	zoomPending       bool
	layoutDuration    time.Duration
	layoutPending     bool
	layoutUntil       time.Time
}

type remoteSession struct {
	id                          int
	host                        hostID
	state                       connectionState
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
	return &remoteSession{
		id: a.nextSession, host: id, state: connected,
		transcript: demoTranscript(a.hosts[id]), output: []terminalLine{},
		scroll: terminalScrollState{offset: ui.ScrollState{Y: math.MaxFloat32}},
	}
}

func (a *app) focusSession(tab *workspaceTab, session *remoteSession) {
	tab.focused = session
	session.focus.Focus()
	session.hasNewOutput, session.hasBell = false, false
	session.cursorStarted = time.Time{}
}

func (a *app) activateTab(index int) {
	a.activeTab = index
	tab := a.tabs[index]
	a.focusSession(tab, tab.focused)
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
	if session.scroll.offset.Y >= session.scroll.offset.MaxY {
		session.scroll.offset.Y = math.MaxFloat32
	}
	session.output = append(session.output, lines...)
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

func (a *app) closeTab(index int) {
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
	if closedActive && len(a.tabs) > 0 {
		a.activateTab(a.activeTab)
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
		if tab == a.currentTab() {
			a.focusSession(tab, tab.root.firstSession())
		} else {
			tab.focused = tab.root.firstSession()
		}
	}
	a.markPaneLayout(tab, theme.Current().C.Desktop.PaneEnterDuration)
	a.setPaneZoom(tab, false)
}
