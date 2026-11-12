package desktop

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/mcpserver"

	"github.com/egoist/mygo/ui"
)

// A closed agent tab stays closed while its agent keeps working; it returns
// with the first operation after this long a pause, or when reopened.
const agentDismissQuiet = 5 * time.Minute

// The record keeps the latest blocks, and each block its latest output lines.
const (
	agentBlockLimit = 200
	agentLineLimit  = 2000
	// agentPendingLimit ends an output line that has no line feed.
	agentPendingLimit = 16 * 1024
)

// agentView is the read-only record of what one access key does on one
// connection through MCP: one block for each operation. Each call is an
// independent non-PTY command, so there is no remote shell behind it. The
// record keeps growing while its tab is closed, so reopening shows all of it.
type agentView struct {
	keyName   string
	session   *remoteSession // made by the first operation, kept for this run
	open      bool           // a tab shows it
	dismissed bool
	blocks    []*agentBlock
	running   map[string]*agentBlock // by operation ID
	last      time.Time
}

// agentBlock is one operation: its request, its output and how it ended.
type agentBlock struct {
	request mcpserver.Activity
	// lines are the finished output lines of both streams, as they arrived.
	lines   []terminalLine
	streams [2]agentStream
	// dropped counts earlier lines given up to agentLineLimit.
	dropped int
	// results has one entry, or for an accepted transfer its queueing and
	// latest task state, replaced when the task is resumed or changes state.
	results []agentResult
	queued  bool
}

// agentStream is stdout or stderr: what of an escape sequence and of a line
// is still to come, and the colors in force.
type agentStream struct {
	filter  agentFilter
	text    terminalTextState
	pending []byte
}

type agentResult struct {
	status, detail string
	exitCode       *int
	elapsed        time.Duration
}

// agentActivity runs on the window thread for every MCP activity, in order.
func (a *app) agentActivity(event mcpserver.Activity) {
	if a.ctx.Err() != nil {
		return
	}
	id := event.KeyID + "\x00" + event.Connection.DisplayID()
	view := a.agents[id]
	if event.Phase == mcpserver.ActivityStarted {
		if !a.prefs.ShowAgentSessions {
			return
		}
		if view == nil {
			view = &agentView{running: map[string]*agentBlock{}}
			if a.agents == nil {
				a.agents = map[string]*agentView{}
			}
			a.agents[id] = view
		}
		quiet := event.At.Sub(view.last)
		view.last, view.keyName = event.At, event.KeyName
		if view.session == nil {
			a.createAgentSession(view, event)
		}
		if !view.open && !(view.dismissed && quiet < agentDismissQuiet) {
			a.showAgentSession(view)
		}
		block := &agentBlock{request: event}
		view.running[event.OperationID] = block
		view.blocks = append(view.blocks, block)
		// The oldest block that has ended makes room.
		if len(view.blocks) > agentBlockLimit {
			if ended := slices.IndexFunc(view.blocks, func(b *agentBlock) bool { return view.running[b.request.OperationID] != b }); ended >= 0 {
				view.blocks = slices.Delete(view.blocks, ended, ended+1)
			}
		}
		a.agentChanged(view)
		return
	}
	if view == nil {
		return
	}
	block := view.running[event.OperationID]
	if block == nil {
		return
	}
	view.last = event.At
	if event.Phase == mcpserver.ActivityOutput {
		stream := 0
		if event.Stderr {
			stream = 1
		}
		block.write(stream, event.Data)
		a.agentChanged(view)
		return
	}
	block.flush()
	block.results = append(block.results, agentResult{
		status: event.Status, detail: event.Error, exitCode: event.ExitCode, elapsed: event.At.Sub(block.request.At),
	})
	if event.Status == "queued" {
		block.queued = true
	} else {
		delete(view.running, event.OperationID)
	}
	a.agentChanged(view)
}

// agentTransferChanged updates the retained card even after a failed attempt
// has left running. A retry belongs to the same operation and card.
func (a *app) agentTransferChanged(operationID, status, detail string) {
	for _, view := range a.agents {
		index := slices.IndexFunc(view.blocks, func(block *agentBlock) bool {
			return block.request.OperationID == operationID && block.queued
		})
		if index < 0 {
			continue
		}
		block := view.blocks[index]
		switch status {
		case "queued", "running", "cancel_requested":
			view.running[operationID] = block
		default:
			delete(view.running, operationID)
		}
		view.last = time.Now()
		block.results = block.results[:1]
		if status != "queued" {
			block.results = append(block.results, agentResult{status: status, detail: detail, elapsed: view.last.Sub(block.request.At)})
		}
		a.agentChanged(view)
		return
	}
}

// write adds a chunk of one stream: its finished lines join the block, and
// the rest waits for its line feed.
func (b *agentBlock) write(stream int, data []byte) {
	s := &b.streams[stream]
	s.pending = append(s.pending, s.filter.filter(data)...)
	end := bytes.LastIndexByte(s.pending, '\n') + 1
	if end == 0 && len(s.pending) > agentPendingLimit {
		end = len(s.pending)
	}
	if end == 0 {
		return
	}
	b.add(stream, s.pending[:end])
	s.pending = append(s.pending[:0], s.pending[end:]...)
}

// flush ends the lines left unfinished when the operation ends.
func (b *agentBlock) flush() {
	for stream := range b.streams {
		if s := &b.streams[stream]; len(s.pending) > 0 {
			b.add(stream, s.pending)
			s.pending = nil
		}
	}
}

func (b *agentBlock) add(stream int, text []byte) {
	lines, _ := b.streams[stream].text.parse(strings.ToValidUTF8(string(text), "�"))
	if stream == 1 {
		agentMarkErrors(lines)
	}
	b.lines = append(b.lines, lines...)
	if over := len(b.lines) - agentLineLimit; over > 0 {
		// Copy to release the text of the lines given up.
		b.lines, b.dropped = slices.Clone(b.lines[over:]), b.dropped+over
	}
}

// tail is the unfinished line of each stream, shown while its rest is awaited.
func (b *agentBlock) tail() []terminalLine {
	var lines []terminalLine
	for stream := range b.streams {
		s := b.streams[stream]
		if len(s.pending) == 0 {
			continue
		}
		// s is a copy: parsing ahead must not move the stream's own colors.
		parsed, _ := s.text.parse(strings.ToValidUTF8(string(s.pending), "�"))
		if stream == 1 {
			agentMarkErrors(parsed)
		}
		lines = append(lines, parsed...)
	}
	return lines
}

// agentMarkErrors gives stderr text that has no color of its own the palette's red.
func agentMarkErrors(lines []terminalLine) {
	for _, line := range lines {
		for i := range line {
			if line[i].Foreground.kind == terminalDefault {
				line[i].Foreground = terminalIndexed(1)
			}
		}
	}
}

// agentChanged follows new content when the record was scrolled to its end.
func (a *app) agentChanged(view *agentView) {
	session := view.session
	if session.scroll.offset.Y >= session.scroll.offset.MaxY {
		session.scroll.offset.Y = math.MaxFloat32
	}
	if view.open && !a.sshSessionFocused(session) {
		session.hasNewOutput = true
	}
}

func (a *app) createAgentSession(view *agentView, event mcpserver.Activity) {
	connection := event.Connection
	// The saved endpoint finds the host's facts for its logo; the name and
	// login shown are those the agent was given.
	endpoint := cloneHostEndpoint(connection.Runtime.(*fileSession).endpoint)
	endpoint.protocol, endpoint.session.Files = "", nil
	endpoint.name, endpoint.user, endpoint.address, endpoint.port = connection.Name, connection.User, connection.Host, connection.Port
	a.nextSession++
	view.session = &remoteSession{
		id: a.nextSession, host: -1, endpoint: endpoint, temporary: true, state: connected, agent: view,
		scroll: terminalScrollState{offset: ui.ScrollState{Y: math.MaxFloat32}},
	}
}

// showAgentSession opens the record's tab behind the current one.
func (a *app) showAgentSession(view *agentView) *workspaceTab {
	a.closedTabs = slices.DeleteFunc(a.closedTabs, func(closed closedWorkspaceTab) bool { return closed.agent == view })
	a.nextTab++
	tab := &workspaceTab{id: a.nextTab, root: &terminalLayout{session: view.session}, focused: view.session}
	a.tabs = append(a.tabs, tab)
	// A tab drag puts back the order it started with when it ends.
	if d := a.drag; d != nil && d.kind == dragTab {
		d.order = append(d.order, tab)
	}
	view.open, view.dismissed = true, false
	return tab
}

// agentViewFor finds the record of an audited operation's key and connection,
// if this run has one.
func (a *app) agentViewFor(keyID, connectionID string) *agentView {
	display := connectionID
	for _, connection := range a.mcpBackend.connections {
		if connection.ID == connectionID {
			display = connection.DisplayID()
			break
		}
	}
	if view := a.agents[keyID+"\x00"+display]; view != nil && view.session != nil {
		return view
	}
	return nil
}

// revealAgentSession brings the record's tab to the front, opening it if closed.
func (a *app) revealAgentSession(view *agentView) {
	index := slices.IndexFunc(a.tabs, func(tab *workspaceTab) bool { return tab.focused == view.session })
	if index < 0 {
		index = slices.Index(a.tabs, a.showAgentSession(view))
	}
	a.switchSection(sectionSSH)
	a.activateTab(index)
}

// closeAgentSession takes the record off screen when its tab closes. The
// agent's operations continue, and the record with them, for reopening.
func (a *app) closeAgentSession(view *agentView) {
	view.open, view.dismissed = false, true
	a.closedTabs = append(a.closedTabs, closedWorkspaceTab{agent: view, host: -1, temporary: true, section: sectionSSH})
}

// agentText keeps control characters out of request text.
func agentText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r >= 0x7f && r < 0xa0 {
			return -1
		}
		return r
	}, value)
}

// agentRequestText is what the operation was asked to run, read or write.
func agentRequestText(event mcpserver.Activity) string {
	if event.Tool == "ssh_exec" {
		return agentText(event.Command)
	}
	target := agentText(event.Path)
	if event.LocalPath != "" {
		target += "  ⇄  " + agentText(event.LocalPath)
	}
	return target
}

// agentFilter passes command output on with its colors but nothing else that
// is not text: of the escape sequences only SGR survives, and of the control
// characters only tab, line feed and carriage return. A sequence may be split
// between chunks.
type agentFilter struct {
	state   int
	pending []byte
}

const (
	agentGround = iota
	agentEscape
	agentCSI
	agentString // OSC, DCS, APC, PM and SOS run to BEL or ST
	agentStringEscape
)

func (f *agentFilter) filter(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch f.state {
		case agentGround:
			switch {
			case b == 0x1b:
				f.state, f.pending = agentEscape, append(f.pending[:0], b)
			case b == '\n' || b == '\r' || b == '\t' || b >= 0x20 && b != 0x7f:
				out = append(out, b)
			}
		case agentEscape:
			switch b {
			case '[':
				f.state, f.pending = agentCSI, append(f.pending, b)
			case ']', 'P', '_', '^', 'X':
				f.state = agentString
			default:
				f.state = agentGround
			}
		case agentCSI:
			switch {
			case b >= 0x40 && b <= 0x7e:
				if b == 'm' {
					out = append(append(out, f.pending...), b)
				}
				f.state = agentGround
			case b == 0x1b:
				f.state, f.pending = agentEscape, append(f.pending[:0], b)
			case len(f.pending) < 64:
				f.pending = append(f.pending, b)
			default:
				f.state = agentGround
			}
		case agentString:
			switch b {
			case 0x07:
				f.state = agentGround
			case 0x1b:
				f.state = agentStringEscape
			}
		case agentStringEscape:
			if b == '\\' {
				f.state = agentGround
			} else if b != 0x1b {
				f.state = agentString
			}
		}
	}
	return out
}
