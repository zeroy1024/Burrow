package desktop

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"

	"github.com/zeroy1024/Burrow/internal/locales"
	sshsession "github.com/zeroy1024/Burrow/internal/session"
	"github.com/zeroy1024/Burrow/internal/sshclient"
)

var errSSHConnectCanceled = locales.NewError(locales.MessageSshConnectionCancelled)

// Connection settings and published UI snapshots belong to the desktop owner.
type sshConnectionGroup struct {
	transport  sshsession.Group
	id         int
	endpoint   host
	plan       sshDialPlan
	planErr    error
	latency    sshLatencyState
	facts      hostFactsProbe
	monitor    *monitorState
	forwarding *sshForwardState
}

type sshRuntime struct {
	*sshsession.Shell
	group      *sshConnectionGroup
	monitor    *monitorState
	forwarding *sshForwardState
}

func (a *app) newSSHGroup(endpoint host) *sshConnectionGroup {
	a.nextConnection++
	endpoint = sshHostSnapshot(endpoint)
	plan, err := a.captureSSHPlan(endpoint)
	if err == nil {
		route := plan.route
		endpoint.session.GatewayRoute = &route
	}
	return &sshConnectionGroup{id: a.nextConnection, endpoint: endpoint, plan: plan, planErr: err}
}

func (a *app) connectSession(session *remoteSession) { a.startSSHSession(session, true) }

func (a *app) startSSHSession(session *remoteSession, refresh bool) {
	if !a.businessReady() {
		return
	}
	stopSSH(session)
	if refresh {
		endpoint, id, _ := a.latestSessionEndpoint(session.endpoint, session.temporary)
		session.endpoint, session.config, session.host = endpoint, endpoint.session, id
		session.connection = a.reconnectSSHGroup(session.connection, endpoint)
		if session.connection.planErr == nil {
			route := session.connection.plan.route
			session.endpoint.session.GatewayRoute = &route
			session.config = session.endpoint.session
		}
	}
	run := &sshRuntime{Shell: sshsession.NewShell(a.ctx), group: session.connection}
	ctx := run.Context()
	session.ssh, session.state = run, connecting
	session.hasNewOutput, session.hasBell = false, false
	h := session.endpoint
	showSSHMessage(session, locales.MessageConnectingTo()+h.user+"@"+net.JoinHostPort(h.address, strconv.Itoa(h.port))+"…")
	request := &run.Request
	request.Failed = func(err error) {
		if session.ssh != run || ctx.Err() != nil {
			return
		}
		run.Close()
		session.state = connectionFailed
		if errors.Is(err, errSSHConnectCanceled) {
			session.state = disconnected
			showSSHMessage(session, err.Error())
		} else {
			showSSHMessage(session, locales.MessageConnectionFailed2()+err.Error())
		}
		showReconnectHint(session, false)
	}
	request.Ready = func(connection *sshclient.Connection) {
		columns, rows := session.columns, session.rows
		if columns == 0 {
			columns = 80
		}
		if rows == 0 {
			rows = 24
		}
		var env []string
		var initCommand string
		if options := session.config.SSH; options != nil {
			env, initCommand = slices.Clone(options.Env), options.InitCommand
		}
		onError := func(err error) {
			a.window.Update(func() {
				if session.ssh == run && ctx.Err() == nil && session.state != connectionFailed {
					run.detachServices()
					session.state = connectionFailed
					showSSHMessage(session, err.Error())
					showReconnectHint(session, false)
				}
			})
		}
		opts := sshclient.ShellOptions{Columns: columns, Rows: rows, Env: env, InitCommand: initCommand, OnError: onError}
		a.sshWorkers.Go(func() {
			run.Open(connection, opts, a.window.Update, func() {
				if session.ssh != run || session.state == connectionFailed {
					run.Close()
					return
				}
				a.attachSSH(session, run)
			})
		})
	}
	a.acquireSSH(session.connection, request)
}

func (a *app) acquireSSH(group *sshConnectionGroup, request *sshsession.Request) {
	if !a.businessReady() {
		request.Failed(errAppUnavailable)
		return
	}
	attempt := group.transport.Acquire(a.ctx, request)
	if attempt == nil {
		return
	}
	plan, planErr := group.plan, group.planErr
	a.sshWorkers.Go(func() {
		attempt.Run(func(ctx context.Context) (*sshclient.Connection, error) {
			if planErr != nil {
				return nil, planErr
			}
			return a.dialSSHPlan(ctx, plan, sshSecretsOverride{}, nil)
		}, a.window.Update)
	})
}

// detachServices drops any presentation state belonging to a finished generation.
func (run *sshRuntime) detachServices() {
	run.EndServices()
	if m := run.monitor; m != nil && m.Context().Err() != nil && run.group.monitor == m {
		run.group.monitor = nil
	}
	if state := run.forwarding; state != nil && state.Context().Err() != nil && run.group.forwarding == state {
		run.group.forwarding = nil
	}
	run.monitor, run.forwarding = nil, nil
}

// Invalidate the view before canceling so late callbacks cannot reattach a channel.
func stopSSH(session *remoteSession) {
	if run := session.ssh; run != nil {
		session.ssh = nil
		run.detachServices()
		run.Close()
	}
}

func (a *app) closeConnections() {
	a.cancel()
	for _, session := range a.fileSessions {
		a.disconnectFileSession(session)
		a.closeLocalFiles(session)
		session.ftpSecret = ""
	}
	for _, tab := range a.tabs.Items {
		for _, session := range tab.Root.Sessions() {
			releaseSSH(session)
		}
	}
	for _, prompt := range a.sshPrompts {
		prompt.secret = ""
	}
	a.sshPrompts = nil
}

// A reconnect may see changed jump credentials while the leaf is unchanged.
// Refresh the complete immutable plan for this consumer; siblings keep theirs.
func (a *app) reconnectSSHGroup(previous *sshConnectionGroup, endpoint host) *sshConnectionGroup {
	plan, err := a.captureSSHPlan(endpoint)
	if err == nil && previous != nil && previous.planErr == nil && sameSSHPlan(previous.plan, plan) {
		return previous
	}
	a.nextConnection++
	if err == nil {
		route := plan.route
		endpoint.session.GatewayRoute = &route
	}
	return &sshConnectionGroup{id: a.nextConnection, endpoint: cloneHostEndpoint(endpoint), plan: plan, planErr: err}
}

func sameSSHPlan(left, right sshDialPlan) bool {
	if gatewayRuntimeKey(left.route, left.steps, left.gatewayScope) != gatewayRuntimeKey(right.route, right.steps, right.gatewayScope) {
		return false
	}
	if left.trust != right.trust || len(left.steps) != len(right.steps) {
		return false
	}
	liveness := func(h host) [2]int {
		if h.session.SSH != nil {
			return [2]int{h.session.SSH.KeepAlive, h.session.SSH.HealthCheck}
		}
		return [2]int{}
	}
	for index, one := range left.steps {
		two := right.steps[index]
		a, b := one.endpoint, two.endpoint
		if a.address != b.address || a.port != b.port || a.user != b.user || a.session.ID != b.session.ID ||
			a.session.Auth != b.session.Auth || a.session.KeyID != b.session.KeyID || a.session.PasswordSaved != b.session.PasswordSaved || a.session.PasswordRevision != b.session.PasswordRevision ||
			!a.session.UpdatedAt.Equal(b.session.UpdatedAt) || liveness(a) != liveness(b) ||
			one.key.ID != two.key.ID || one.key.SecretID != two.key.SecretID || one.key.Fingerprint != two.key.Fingerprint {
			return false
		}
	}
	return true
}
