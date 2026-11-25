package desktop

import (
	"github.com/zeroy1024/Burrow/internal/workspace"
	"math"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

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
	id int

	paneTab
	frame              ui.Handle
	zoomFrom, zoomMix  float32
	zoomPane           *remoteSession
	zoomBounds         ui.Rect
	relayout           paneRelayout
	layoutDuration     time.Duration
	layoutPending      bool
	layoutUntil        time.Time
	workspaceSize      ui.Point
	workspaceResizedAt time.Time
}

func (tab *workspaceTab) canSeparateSessions() bool {
	sessions := tab.Root.Sessions()
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
	// agent marks the read-only record of an MCP agent; it has no shell of its own.
	agent                       *agentView
	endpoint                    host
	temporary                   bool
	connection                  *sshConnectionGroup
	ssh                         *sshRuntime
	terminal                    *sshTerminal
	focusPending                bool
	config                      sessionConfig
	id                          int
	host                        hostID
	state                       connectionState
	scroll                      terminalScrollState
	focus                       ui.Handle
	transcript                  []terminalLine
	output                      []terminalLine
	hasNewOutput                bool
	hasBell                     bool
	cursorStarted               time.Time
	frame                       ui.Handle
	layoutRect                  ui.Rect
	contentWidth, contentHeight float32
	columns, rows               int
	closing                     bool
	closeMaximized              bool
	closeStarted                time.Time
	revealing                   bool
	revealStarted               time.Time
	splitFadeStarted            time.Time
}

func demoHosts() []host {
	hosts := []host{
		{name: "prod-web-01", user: "root", address: "192.0.2.10", os: 0,
			latency: 18, system: "Ubuntu 22.04.4", uptime: locales.MessageDaysHours(), cpu: 23, memory: 41, disk: 57},
		{name: "prod-web-02", user: "root", address: "192.0.2.11", os: 0,
			latency: 21, system: "Ubuntu 22.04.4", uptime: locales.MessageDaysHours2(), cpu: 16, memory: 35, disk: 43},
		{name: "prod-db-master", user: "admin", address: "192.0.2.12", os: 1,
			latency: 9, system: "Debian 12", uptime: locales.MessageDaysHours3(), cpu: 12, memory: 62, disk: 71},
		{name: "cdn-origin", user: "deploy", address: "192.0.2.13", os: 2,
			latency: 32, system: "CentOS Stream 9", uptime: locales.MessageDaysHours4(), cpu: 8, memory: 24, disk: 38},
		{name: "staging-api", user: "ubuntu", address: "192.0.2.14", os: 0,
			latency: 46, system: "Ubuntu 24.04", uptime: locales.MessageDaysHours5(), cpu: 31, memory: 27, disk: 32},
		{name: "bastion", user: "ops", address: "192.0.2.15", os: 3,
			latency: 12, system: "Alpine Linux 3.21", uptime: locales.MessageDaysHours6(), cpu: 4, memory: 18, disk: 22},
		{name: "qa-runner", user: "ci", address: "192.0.2.16", os: 4,
			latency: 28, system: "Fedora 42", uptime: locales.MessageDaysHour(), cpu: 38, memory: 48, disk: 44},
		{name: "nas.home", user: "zero", address: "192.0.2.17", os: 5,
			latency: 2, system: "Linux 6.6", uptime: locales.MessageDaysHours7(), cpu: 6, memory: 32, disk: 68},
		{name: "vps-tokyo", user: "root", address: "192.0.2.18", os: 6,
			latency: 54, system: "Arch Linux", uptime: locales.MessageDaysHours8(), cpu: 11, memory: 29, disk: 35},
	}
	for i := range hosts {
		hosts[i].port = 22
	}
	return hosts
}

func (a *app) currentTab() *workspaceTab {
	return a.tabs.Current()
}

func (a *app) newSession(id hostID) *remoteSession {
	return a.newShellSession(id, a.hosts[id], false, nil)
}

func (a *app) newDisconnectedSession(id hostID, endpoint host, temporary bool, connection *sshConnectionGroup) *remoteSession {
	endpoint = cloneHostEndpoint(endpoint)
	endpoint.protocol = ""
	endpoint.session.Files = nil
	if endpoint.session.SSH == nil {
		endpoint.session.SSH = &sshOptions{}
	}
	a.nextSession++
	if connection == nil {
		connection = a.newSSHGroup(endpoint)
	}
	if connection.planErr == nil {
		route := connection.plan.route
		endpoint.session.GatewayRoute = &route
	}
	return &remoteSession{
		id: a.nextSession, host: id, endpoint: endpoint, config: endpoint.session,
		temporary: temporary, connection: connection,
		scroll: terminalScrollState{offset: ui.ScrollState{Y: math.MaxFloat32}},
	}
}

func (a *app) newShellSession(id hostID, endpoint host, temporary bool, connection *sshConnectionGroup) *remoteSession {
	session := a.newDisconnectedSession(id, endpoint, temporary, connection)
	a.startSSHSession(session, false)
	return session
}

func disconnectSession(session *remoteSession) {
	running := terminalRunning(session)
	stopSSH(session)
	session.state = disconnected
	if running {
		// The terminal reports its end with [Disconnected], and its OnExit adds the hint.
		return
	}
	showSSHMessage(session, locales.MessageSessionDisconnected2())
	showReconnectHint(session, false)
}

// awaitingConnect reports whether the pane offers a new connection attempt.
func (session *remoteSession) awaitingConnect() bool {
	return !session.closing && (session.state == disconnected || session.state == connectionFailed)
}

func appendSessionLines(session *remoteSession, lines ...terminalLine) {
	if session.scroll.offset.Y >= session.scroll.offset.MaxY {
		session.scroll.offset.Y = math.MaxFloat32
	}
	session.output = append(session.output, lines...)
}

func (a *app) focusSession(tab *workspaceTab, session *remoteSession) {
	if session.closing {
		return
	}
	tab.Focused = session
	// The target may still be waiting for its terminal view to be created.
	session.focusPending = true
	session.focus.Focus()
	session.markViewed()
}

func (session *remoteSession) markViewed() {
	session.hasNewOutput, session.hasBell = false, false
	session.cursorStarted = time.Time{}
}

func (a *app) activateTab(index int) {
	a.selectTab(index)
	a.tabs.Items[index].Focused.focus.Focus()
}

func (a *app) selectTab(index int) {
	a.settingsUI.previousFocus = ui.Handle{}
	a.closeSettings()
	a.tabs.Active = index
	a.tabs.Items[index].Focused.markViewed()
}

func (a *app) openHost(id hostID) {
	a.nextTab++
	session := a.newSession(id)
	a.tabs.Items = append(a.tabs.Items, &workspaceTab{id: a.nextTab, paneTab: workspace.NewTab(session)})
	a.activateTab(len(a.tabs.Items) - 1)
}

func (a *app) ungroupTab(tab *workspaceTab) {
	index := slices.Index(a.tabs.Items, tab)
	if index < 0 || !tab.canSeparateSessions() {
		return
	}
	active, focused := a.currentTab(), tab.Focused
	sessions := tab.Root.Sessions()
	separated := make([]*workspaceTab, 0, len(sessions))
	*tab = workspaceTab{id: tab.id, frame: tab.frame, paneTab: workspace.NewTab(sessions[0])}
	separated = append(separated, tab)
	for _, session := range sessions[1:] {
		a.nextTab++
		separated = append(separated, &workspaceTab{id: a.nextTab, paneTab: workspace.NewTab(session)})
	}
	a.tabs.Items = slices.Replace(a.tabs.Items, index, index+1, separated...)
	if active != tab {
		a.tabs.Active = slices.Index(a.tabs.Items, active)
		return
	}
	a.resizing = nil
	a.activateTab(index + slices.Index(sessions, focused))
}

func (a *app) detachPaneToTab(source *workspaceTab, session *remoteSession, index int, area ui.Rect) *workspaceTab {
	if !source.canSeparateSessions() || slices.Index(a.tabs.Items, source) < 0 {
		return nil
	}
	if source.Root.Find(session) == nil {
		return nil
	}
	if index < 0 || index > len(a.tabs.Items) {
		return nil
	}
	minimum := minimumLayoutSize(source.Root)
	canvas := ui.Rect{W: max(area.W, minimum.X), H: max(area.H, minimum.Y)}
	remaining := copyLayoutAt(source.Root, canvas).Remove(session)
	focused := source.Focused
	if focused == session {
		focused = remaining.FirstSession()
	}
	*source = workspaceTab{id: source.id, frame: source.frame, paneTab: paneTab{Root: remaining, Focused: focused}}
	a.markPaneLayout(source, theme.Current().C.ReorderMotion.PaneDrop.Duration)
	a.nextTab++
	tab := &workspaceTab{id: a.nextTab, paneTab: workspace.NewTab(session)}
	a.tabs.Items = slices.Insert(a.tabs.Items, index, tab)
	a.resizing = nil
	a.activateTab(index)
	return tab
}

func (a *app) closeTab(index int) {
	if tab := a.tabs.Items[index]; tab.Root != nil {
		// An agent's record outlives its tab; a shell's tab reopens as a new shell.
		if session := tab.Focused; session.agent != nil {
			a.closeAgentSession(session.agent)
		} else {
			for _, session := range tab.Root.Sessions() {
				releaseSSH(session)
			}
			a.closedTabs = append(a.closedTabs, closedWorkspaceTab{host: session.host, section: sectionSSH,
				endpoint: cloneHostEndpoint(session.endpoint), temporary: session.temporary, connection: session.connection})
		}
	}
	closedActive := index == a.tabs.Active
	a.tabs.Remove(index)
	if closedActive && len(a.tabs.Items) > 0 && a.activeSection() == sectionSSH {
		a.focusWorkspace()
	}
}

func (a *app) splitSession(session *remoteSession, stacked bool) {
	tab := a.currentTab()
	if session.agent != nil || tab.Root.Count() >= maxTerminalPanes {
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
	child := a.newShellSession(session.host, session.endpoint, session.temporary, session.connection)
	node := tab.Root.Find(session)
	node.Insert(child, zone)
	if tab.Focused != session {
		a.focusSession(tab, session)
	}
	a.startPaneRelayout(tab, child)
	startPaneReveal(child)
	a.markPaneLayout(tab, theme.Current().C.PaneReveal.Duration)
	a.setPaneZoom(tab, false)
}

func (tab *workspaceTab) canClosePane(session *remoteSession) bool {
	if session.closing || tab.Root.Find(session) == nil {
		return false
	}
	available := 0
	for _, candidate := range tab.Root.Sessions() {
		if !candidate.closing {
			available++
		}
	}
	return available > 1
}

func (a *app) closePane(tab *workspaceTab, session *remoteSession) {
	if !tab.canClosePane(session) {
		return
	}
	session.closing = true
	// Close the channel now. The terminal renderer stays alive for the exit copy.
	stopSSH(session)
	if tab.Focused != session {
		return
	}
	session.closeMaximized = tab.Maximized
	a.setPaneZoom(tab, false)
	a.focusRemainingPane(tab)
}

func (a *app) focusRemainingPane(tab *workspaceTab) {
	for _, next := range tab.Root.Sessions() {
		if next.closing {
			continue
		}
		if tab == a.currentTab() && a.activeSection() == sectionSSH {
			a.focusSession(tab, next)
		} else {
			tab.Focused = next
		}
		return
	}
}

func (a *app) removePane(tab *workspaceTab, session *remoteSession) {
	releaseSSH(session)
	tab.Root = tab.Root.Remove(session)
	a.startPaneRelayout(tab, nil)
	if tab.Focused == session {
		a.focusRemainingPane(tab)
		a.setPaneZoom(tab, false)
	}
	a.markPaneLayout(tab, theme.Current().C.Desktop.PaneEnterDuration)
}

// Runtime endpoints are snapshots; catalog edits cannot retarget a live channel.
func cloneHostEndpoint(endpoint host) host {
	// Catalog values are not live samples; SSH latency belongs to the transport group.
	endpoint.latency = -1
	if options := endpoint.session.SSH; options != nil {
		copy := *options
		copy.Env = slices.Clone(options.Env)
		copy.Forwards = slices.Clone(options.Forwards)
		endpoint.session.SSH = &copy
	}
	if options := endpoint.session.Files; options != nil {
		copy := *options
		endpoint.session.Files = &copy
	}
	return endpoint
}

// Explicit reconnect adopts saved changes. Existing channels and background jobs
// keep their captured endpoint; the caller replaces only its own transport group.
func (a *app) latestSessionEndpoint(current host, temporary bool) (host, hostID, bool) {
	if temporary {
		return current, -1, false
	}
	currentFiles := current.protocol == "sftp" || current.protocol == "ftp"
	for index, saved := range a.hosts {
		savedFiles := saved.protocol == "sftp" || saved.protocol == "ftp"
		if saved.session.ID == current.session.ID && savedFiles == currentFiles {
			changed := saved.protocol != current.protocol || !saved.session.UpdatedAt.Equal(current.session.UpdatedAt)
			endpoint := cloneHostEndpoint(saved)
			endpoint.session.GatewayRoute = nil
			return endpoint, hostID(index), changed
		}
	}
	return current, -1, false
}

func (a *app) runtimeSSHGroups() []*sshConnectionGroup {
	groups := map[*sshConnectionGroup]struct{}{}
	for _, tab := range a.tabs.Items {
		for _, session := range tab.Root.Sessions() {
			if session.connection != nil {
				groups[session.connection] = struct{}{}
			}
		}
	}
	for _, session := range a.fileSessions {
		if session.connection != nil {
			groups[session.connection] = struct{}{}
		}
	}
	for _, task := range a.transfers.tasks {
		if task.session.connection != nil {
			groups[task.session.connection] = struct{}{}
		}
	}
	// Reopening a closed tab restores its captured plan after catalog edits.
	for _, closed := range a.closedTabs {
		group := closed.connection
		if closed.section == sectionFiles {
			group = closed.file.connection
		}
		if group != nil {
			groups[group] = struct{}{}
		}
	}
	result := make([]*sshConnectionGroup, 0, len(groups))
	for group := range groups {
		result = append(result, group)
	}
	return result
}
