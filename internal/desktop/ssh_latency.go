package desktop

import (
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const (
	sshLatencyInterval = 5 * time.Second
	sshLatencyMaxAge   = 2 * sshLatencyInterval
)

// Window-thread state belongs to the connection group, not a focused Shell or
// the monitor panel. Moving panes keeps the same sample and transport.
type sshLatencyState struct {
	connection *sshclient.Connection
	pending    bool
	next       time.Time
	updated    time.Time
	elapsed    time.Duration
}

func (a *app) updateSSHLatency(c *ui.Context) {
	if a.activeSection() != sectionSSH || a.window == nil || !a.window.IsVisible() || a.ctx.Err() != nil {
		return
	}
	tab := a.currentTab()
	if tab == nil {
		return
	}
	var sessions []*remoteSession
	for _, session := range tab.root.sessions() {
		if !tab.maximized || session == tab.focused {
			sessions = append(sessions, session)
		}
	}
	a.sampleSSHLatency(c, sessions)
}

// sampleSSHLatency reads passive TCP RTT for shown connections, once per
// interval for each connection group, whichever window shows them.
func (a *app) sampleSSHLatency(c *ui.Context, sessions []*remoteSession) {
	seen := map[*sshConnectionGroup]bool{}
	for _, session := range sessions {
		if session.closing || session.state != connected || session.ssh == nil || session.ssh.conn == nil {
			continue
		}
		group := session.connection
		if group == nil || group.connection == nil || seen[group] {
			continue
		}
		seen[group] = true
		connection := group.connection
		state := &group.latency
		if state.connection != connection {
			*state = sshLatencyState{connection: connection}
		}
		if remaining := state.next.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
			continue
		}
		if state.pending {
			continue
		}
		state.pending, state.next = true, c.Now().Add(sshLatencyInterval)
		c.After(sshLatencyInterval)
		a.sshWorkers.Go(func() {
			elapsed, err := connection.TCPRTT()
			a.window.Update(func() {
				if a.ctx.Err() != nil || group.connection != connection || state.connection != connection {
					return
				}
				state.pending = false
				if err != nil {
					state.updated = time.Time{}
					return
				}
				state.elapsed, state.updated = elapsed, time.Now()
			})
		})
	}
}

func sshLatencyMS(session *remoteSession, now time.Time) int {
	if session.state != connected || session.connection == nil {
		return -1
	}
	state := &session.connection.latency
	if state.connection != session.connection.connection || state.updated.IsZero() || now.Sub(state.updated) >= sshLatencyMaxAge {
		return -1
	}
	return int(state.elapsed.Milliseconds())
}
