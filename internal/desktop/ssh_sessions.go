package desktop

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"time"

	"sshdesk/internal/sshclient"
)

var errSSHConnectCanceled = errors.New("SSH 连接已取消")

// The group outlives each view's request and owns one authenticated transport.
// All group fields and request callbacks are owned by the window thread.
type sshConnectionGroup struct {
	id         int
	endpoint   host
	plan       sshDialPlan
	planErr    error
	connection *sshclient.Connection
	ctx        context.Context
	cancel     context.CancelFunc
	generation int
	connecting bool
	waiters    map[*sshConnectionRequest]struct{}
	queue      []*sshConnectionRequest
	opening    *sshConnectionRequest
	latency    sshLatencyState
	facts      hostFactsProbe
}

type sshConnectionRequest struct {
	mcpOperationID string
	mcpTaskID      int
	ctx            context.Context
	group          *sshConnectionGroup
	connection     *sshclient.Connection
	release        func()
	ready          func(*sshclient.Connection)
	failed         func(error)
}

type sshRuntime struct {
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *sshclient.Conn
	request *sshConnectionRequest
}

func (a *app) newSSHGroup(endpoint host) *sshConnectionGroup {
	a.nextConnection++
	endpoint = sshHostSnapshot(endpoint)
	plan, err := a.captureSSHPlan(endpoint)
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
	}
	ctx, cancel := context.WithCancel(a.ctx)
	run := &sshRuntime{ctx: ctx, cancel: cancel}
	session.ssh, session.state = run, connecting
	session.hasNewOutput, session.hasBell = false, false
	h := session.endpoint
	showSSHMessage(session, "正在连接 "+h.user+"@"+net.JoinHostPort(h.address, strconv.Itoa(h.port))+"…")
	request := &sshConnectionRequest{ctx: ctx, group: session.connection}
	run.request = request
	request.failed = func(err error) {
		if session.ssh != run || ctx.Err() != nil {
			return
		}
		cancel()
		session.state = connectionFailed
		if errors.Is(err, errSSHConnectCanceled) {
			session.state = disconnected
			showSSHMessage(session, err.Error())
		} else {
			showSSHMessage(session, "连接失败："+err.Error())
		}
		showReconnectHint(session, false)
	}
	request.ready = func(connection *sshclient.Connection) {
		channelCtx, cancelChannel := context.WithCancel(ctx)
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
					session.state = connectionFailed
					showSSHMessage(session, err.Error())
					showReconnectHint(session, false)
				}
			})
		}
		a.sshWorkers.Go(func() {
			setupCtx, cancelSetup := context.WithTimeout(channelCtx, 2*time.Minute)
			defer cancelSetup()
			stopSetup := context.AfterFunc(setupCtx, cancelChannel)
			defer stopSetup()
			var conn *sshclient.Conn
			var err error
			for {
				err = connection.WaitChannelReady(setupCtx)
				if err != nil {
					break
				}
				conn, err = connection.OpenShell(channelCtx, sshclient.ShellOptions{
					Columns: columns, Rows: rows, Env: env, InitCommand: initCommand,
					OnError: onError,
				})
				if !errors.Is(err, sshclient.ErrChannelPending) {
					break
				}
			}
			if err != nil {
				setupErr := setupCtx.Err()
				cancelChannel()
				if setupErr != nil {
					err = setupErr
				}
			}
			a.window.Update(func() {
				a.completeSSHRequest(request)
				if session.ssh != run || ctx.Err() != nil {
					if conn != nil {
						conn.Close()
					}
					return
				}
				if err != nil {
					request.failed(err)
					return
				}
				if channelCtx.Err() != nil || session.state == connectionFailed {
					conn.Close()
					return
				}
				run.conn = conn
				a.attachSSH(session, run)
			})
			if err != nil {
				return
			}
			// The reader must attach before a large initial command can finish:
			// its remote echo may exceed the channel's receive window.
			select {
			case <-conn.InitializationDone():
			case <-setupCtx.Done():
			}
			stopSetup()
			if setupCtx.Err() != nil {
				cancelChannel()
				if ctx.Err() == nil {
					onError(setupCtx.Err())
				}
			}
		})
	}
	a.acquireSSH(request)
}

func (a *app) acquireSSH(request *sshConnectionRequest) {
	if !a.businessReady() {
		request.failed(errAppUnavailable)
		return
	}
	group := request.group
	if request.ctx.Err() != nil {
		return
	}
	if connection := group.connection; connection != nil {
		select {
		case <-connection.Done():
			group.connection = nil
		default:
			err := a.deliverSSHConnection(request, connection)
			if err == nil {
				return
			}
			if !errors.Is(err, sshclient.ErrClosed) {
				request.failed(err)
				return
			}
			group.connection = nil
		}
	}
	if !group.connecting || group.ctx.Err() != nil {
		group.generation++
		group.ctx, group.cancel = context.WithCancel(a.ctx)
		group.connecting = true
		group.waiters = make(map[*sshConnectionRequest]struct{})
		group.waiters[request] = struct{}{}
		a.dialSSHGroup(group)
		return
	}
	group.waiters[request] = struct{}{}
}

func (a *app) deliverSSHConnection(request *sshConnectionRequest, connection *sshclient.Connection) error {
	release, err := connection.Retain()
	if err != nil {
		return err
	}
	request.release = release
	request.connection = connection
	request.group.queue = append(request.group.queue, request)
	a.nextSSHRequest(request.group)
	return nil
}

func (a *app) nextSSHRequest(group *sshConnectionGroup) {
	if group.opening != nil {
		return
	}
	for len(group.queue) > 0 {
		request := group.queue[0]
		group.queue = group.queue[1:]
		if request.ctx.Err() != nil {
			request.release()
			continue
		}
		group.opening = request
		request.ready(request.connection)
		return
	}
}

func (a *app) completeSSHRequest(request *sshConnectionRequest) {
	if request == nil {
		return
	}
	if request.release != nil {
		request.release()
	}
	group := request.group
	if group.opening == request {
		group.opening = nil
		a.nextSSHRequest(group)
	}
}

func (a *app) dialSSHGroup(group *sshConnectionGroup) {
	ctx, generation, plan, planErr := group.ctx, group.generation, group.plan, group.planErr
	cancel := group.cancel
	a.sshWorkers.Go(func() {
		var connection *sshclient.Connection
		err := planErr
		if err == nil {
			connection, err = a.dialSSHPlan(ctx, plan, sshSecretsOverride{}, nil)
		}
		a.window.Update(func() {
			if group.generation != generation || ctx.Err() != nil {
				cancel()
				if connection != nil {
					connection.Close()
				}
				return
			}
			group.connecting = false
			waiters := group.waiters
			group.waiters = nil
			if err != nil {
				cancel()
				for request := range waiters {
					if request.ctx.Err() == nil {
						request.failed(err)
					}
				}
				return
			}
			// Reserve the transport while every still-wanted child gets its lease.
			guard, err := connection.Retain()
			if err != nil {
				connection.Close()
				cancel()
				for request := range waiters {
					if request.ctx.Err() == nil {
						request.failed(err)
					}
				}
				return
			}
			group.connection = connection
			for request := range waiters {
				if request.ctx.Err() == nil {
					if err := a.deliverSSHConnection(request, connection); err != nil {
						request.failed(err)
					}
				}
			}
			guard()
		})
		if connection != nil {
			connection.Wait()
			cancel()
			a.window.Update(func() {
				if group.generation == generation && group.connection == connection {
					group.connection = nil
				}
			})
		}
	})
}

func cancelSSHRequest(request *sshConnectionRequest) {
	if request == nil {
		return
	}
	if request.release != nil {
		request.release()
	}
	group := request.group
	delete(group.waiters, request)
	for i, queued := range group.queue {
		if queued == request {
			group.queue = append(group.queue[:i], group.queue[i+1:]...)
			break
		}
	}
	if group.connecting && len(group.waiters) == 0 {
		group.cancel()
	}
}

// Invalidate the view before canceling so late callbacks cannot reattach a channel.
func stopSSH(session *remoteSession) {
	if run := session.ssh; run != nil {
		session.ssh = nil
		run.cancel()
		cancelSSHRequest(run.request)
		if run.conn != nil {
			run.conn.Close()
		}
	}
}

func (a *app) closeConnections() {
	a.cancel()
	for _, session := range a.fileSessions {
		a.disconnectFileSession(session)
		a.closeLocalFiles(session)
		session.ftpSecret = ""
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			releaseSSH(session)
		}
	}
	for _, prompt := range a.sshPrompts {
		prompt.secret = ""
	}
	a.sshPrompts = nil
	a.fileSSHLink = nil
	a.fileSSHLinkAfterClose = nil
}

// A reconnect may see changed jump credentials while the leaf is unchanged.
// Refresh the complete immutable plan for this consumer; siblings keep theirs.
func (a *app) reconnectSSHGroup(previous *sshConnectionGroup, endpoint host) *sshConnectionGroup {
	plan, err := a.captureSSHPlan(endpoint)
	if err == nil && previous != nil && previous.planErr == nil && sameSSHPlan(previous.plan, plan) {
		return previous
	}
	a.nextConnection++
	return &sshConnectionGroup{id: a.nextConnection, endpoint: cloneHostEndpoint(endpoint), plan: plan, planErr: err}
}

func sameSSHPlan(left, right sshDialPlan) bool {
	if left.trust != right.trust || len(left.steps) != len(right.steps) {
		return false
	}
	keepalive := func(h host) int {
		if h.session.SSH != nil {
			return h.session.SSH.KeepAlive
		}
		return 0
	}
	for index, one := range left.steps {
		two := right.steps[index]
		a, b := one.endpoint, two.endpoint
		if a.address != b.address || a.port != b.port || a.user != b.user || a.session.ID != b.session.ID ||
			a.session.Auth != b.session.Auth || a.session.KeyID != b.session.KeyID || a.session.PasswordSaved != b.session.PasswordSaved || a.session.PasswordRevision != b.session.PasswordRevision ||
			!a.session.UpdatedAt.Equal(b.session.UpdatedAt) || keepalive(a) != keepalive(b) ||
			one.key.ID != two.key.ID || one.key.SecretID != two.key.SecretID || one.key.Fingerprint != two.key.Fingerprint {
			return false
		}
	}
	return true
}
