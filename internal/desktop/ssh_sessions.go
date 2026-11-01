package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	connection *sshclient.Connection
	ctx        context.Context
	cancel     context.CancelFunc
	generation int
	connecting bool
	waiters    map[*sshConnectionRequest]struct{}
	queue      []*sshConnectionRequest
	opening    *sshConnectionRequest
}

type sshConnectionRequest struct {
	ctx        context.Context
	group      *sshConnectionGroup
	connection *sshclient.Connection
	release    func()
	ready      func(*sshclient.Connection)
	failed     func(error)
}

type sshRuntime struct {
	ctx     context.Context
	cancel  context.CancelFunc
	conn    *sshclient.Conn
	request *sshConnectionRequest
}

func (a *app) newSSHGroup(endpoint host) *sshConnectionGroup {
	a.nextConnection++
	return &sshConnectionGroup{id: a.nextConnection, endpoint: endpoint}
}

func (a *app) connectSession(session *remoteSession) {
	stopSSH(session)
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
			return
		}
		showSSHMessage(session, "连接失败："+err.Error())
	}
	request.ready = func(connection *sshclient.Connection) {
		channelCtx, cancelChannel := context.WithCancel(ctx)
		a.sshWorkers.Go(func() {
			setupCtx, cancelSetup := context.WithTimeout(channelCtx, 2*time.Minute)
			defer cancelSetup()
			stopSetup := context.AfterFunc(setupCtx, cancelChannel)
			var conn *sshclient.Conn
			err := connection.WaitChannelReady(setupCtx)
			if err == nil {
				conn, err = connection.OpenShell(channelCtx, sshclient.ShellOptions{
					Columns: 80, Rows: 24,
					OnError: func(err error) {
						a.window.Update(func() {
							if session.ssh == run && ctx.Err() == nil {
								session.state = connectionFailed
								showSSHMessage(session, err.Error())
							}
						})
					},
				})
			}
			if !stopSetup() && setupCtx.Err() != nil {
				err = setupCtx.Err()
				if conn != nil {
					conn.Close()
				}
			}
			if err != nil {
				cancelChannel()
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
				run.conn = conn
				a.attachSSH(session, run)
			})
		})
	}
	a.acquireSSH(request)
}

func (a *app) acquireSSH(request *sshConnectionRequest) {
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
	request.release()
	group := request.group
	if group.opening == request {
		group.opening = nil
		a.nextSSHRequest(group)
	}
}

func (a *app) dialSSHGroup(group *sshConnectionGroup) {
	ctx, generation, h := group.ctx, group.generation, group.endpoint
	cancel := group.cancel
	a.sshWorkers.Go(func() {
		connection, err := a.authenticateSSH(ctx, h)
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
			a.sshWorkers.Go(func() {
				connection.Wait()
				cancel()
				a.window.Update(func() {
					if group.generation == generation && group.connection == connection {
						group.connection = nil
					}
				})
			})
		})
	})
}

func (a *app) authenticateSSH(ctx context.Context, h host) (*sshclient.Connection, error) {
	cfg := sshclient.Config{Host: h.address, Port: h.port, User: h.user, Auth: h.auth, KeyPath: h.keyPath}
	if cfg.Auth == "" {
		cfg.Auth = "agent"
	}
	if cfg.Auth == "password" || cfg.Auth == "key" {
		title := "输入 SSH 密码"
		if cfg.Auth == "key" {
			title = "私钥口令（未加密可留空）"
		}
		answer := a.askSSH(ctx, &sshPrompt{title: title,
			address: h.user + "@" + net.JoinHostPort(h.address, strconv.Itoa(h.port))})
		if !answer.accepted {
			return nil, errSSHConnectCanceled
		}
		if cfg.Auth == "password" {
			cfg.Password = answer.secret
		} else {
			cfg.Passphrase = answer.secret
		}
		answer.secret = ""
	}
	defer func() { cfg.Password, cfg.Passphrase = "", "" }()
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate SSH configuration: %w", err)
	}
	// A socket deadline alone cannot interrupt a host-key prompt waiting for input.
	setupCtx, cancelSetup := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelSetup()
	cfg.HostKey = sshclient.KnownHosts(filepath.Join(directory, "SSH Desk", "known_hosts"), func(hostname, fingerprint, keyType string) bool {
		return a.askSSH(setupCtx, &sshPrompt{title: "信任服务器主机密钥？", address: hostname,
			fingerprint: fingerprint, keyType: keyType}).accepted
	})
	connection, err := sshclient.Dial(ctx, cfg)
	if err != nil && setupCtx.Err() != nil {
		return nil, setupCtx.Err()
	}
	return connection, err
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
}
