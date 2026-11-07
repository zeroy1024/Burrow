package desktop

import (
	"context"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const (
	sshLatencyInterval = 5 * time.Second
	sshLatencyTimeout  = 3 * time.Second
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
	seen := map[*sshConnectionGroup]bool{}
	for _, session := range tab.root.sessions() {
		if session.closing || session.state != connected || session.ssh == nil || session.ssh.conn == nil ||
			tab.maximized && session != tab.focused {
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
		ctx, cancel := context.WithTimeout(a.ctx, sshLatencyTimeout)
		a.sshWorkers.Go(func() {
			defer cancel()
			elapsed, err := connection.ProbeLatency(ctx)
			a.window.Update(func() {
				if a.ctx.Err() != nil || group.connection != connection || state.connection != connection {
					return
				}
				state.pending = false
				// A later caller can join an earlier unanswered protocol request.
				// Its eventual late reply must not restore an expired measurement.
				if err != nil || elapsed > sshLatencyTimeout {
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
