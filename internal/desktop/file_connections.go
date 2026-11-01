package desktop

import (
	"context"
	"errors"
	"net"
	"path"
	"strconv"

	"sshdesk/internal/fileclient"
	"sshdesk/internal/sshclient"
)

type fileNavigation struct {
	path         string
	historyIndex int // -1 appends a successfully visited directory
}

type fileRuntime struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     fileclient.Client
	request    *sshConnectionRequest
	listing    bool
	sequence   int
	navigation fileNavigation
}

func (a *app) reconnectFileSession(session *fileSession) {
	a.disconnectFileSession(session)
	ctx, cancel := context.WithCancel(a.ctx)
	run := &fileRuntime{ctx: ctx, cancel: cancel}
	session.runtime, session.state, session.errorText = run, connecting, ""
	session.remote.loading, session.remote.errorText = true, ""
	failed := func(err error) {
		if session.runtime != run || ctx.Err() != nil {
			return
		}
		cancel()
		session.state, session.errorText = connectionFailed, err.Error()
		if errors.Is(err, errSSHConnectCanceled) {
			session.state, session.errorText = disconnected, ""
		}
		session.remote.loading = false
	}
	if session.endpoint.protocol == "ftp" {
		a.sshWorkers.Go(func() {
			h := session.endpoint
			answer := a.askSSH(ctx, &sshPrompt{title: "输入 FTP 密码",
				address: h.user + "@" + net.JoinHostPort(h.address, strconv.Itoa(h.port))})
			if !answer.accepted {
				a.window.Update(func() { failed(errSSHConnectCanceled) })
				return
			}
			client, err := fileclient.DialFTP(ctx, fileclient.FTPConfig{Host: h.address, Port: h.port, User: h.user, Password: answer.secret})
			answer.secret = ""
			a.finishFileConnection(session, run, client, err, failed)
		})
		return
	}
	request := &sshConnectionRequest{ctx: ctx, group: session.connection, failed: failed}
	run.request = request
	request.ready = func(connection *sshclient.Connection) {
		a.sshWorkers.Go(func() {
			var client fileclient.Client
			err := connection.WaitChannelReady(ctx)
			if err == nil {
				client, err = fileclient.NewSFTP(ctx, connection)
			}
			a.finishFileConnection(session, run, client, err, failed)
		})
	}
	a.acquireSSH(request)
}

func (a *app) finishFileConnection(session *fileSession, run *fileRuntime, client fileclient.Client, err error, failed func(error)) {
	var initial string
	if err == nil {
		initial, err = client.InitialPath(run.ctx)
	}
	a.window.Update(func() {
		a.completeSSHRequest(run.request)
		if session.runtime != run || run.ctx.Err() != nil {
			if client != nil {
				client.Close()
			}
			return
		}
		if err != nil {
			if client != nil {
				client.Close()
			}
			failed(err)
			return
		}
		run.client = client
		session.state, session.errorText = connected, ""
		pane := session.remote
		if pane.path == "" {
			pane.path, pane.history, pane.historyIndex = initial, []string{initial}, 0
		}
		a.queueRemoteDirectory(session, fileNavigation{path: pane.path, historyIndex: pane.historyIndex})
		a.sshWorkers.Go(func() {
			<-client.Done()
			a.window.Update(func() {
				if session.runtime == run && run.ctx.Err() == nil {
					session.state = connectionFailed
					if session.errorText == "" {
						session.errorText = "文件连接已断开"
					}
					session.remote.loading = false
				}
			})
		})
	})
}

func (a *app) disconnectFileSession(session *fileSession) {
	if run := session.runtime; run != nil {
		session.runtime = nil
		run.cancel()
		cancelSSHRequest(run.request)
		if run.client != nil {
			run.client.Close()
		}
	}
	session.state, session.errorText = disconnected, ""
	session.remote.loading = false
}

func (a *app) refreshRemoteFiles(session *fileSession) {
	a.queueRemoteDirectory(session, fileNavigation{path: session.remote.path, historyIndex: session.remote.historyIndex})
}

func (a *app) queueRemoteDirectory(session *fileSession, navigation fileNavigation) {
	run := session.runtime
	if run == nil || run.client == nil || session.state != connected {
		return
	}
	run.sequence++
	run.navigation = navigation
	session.remote.loading, session.remote.errorText = true, ""
	if !run.listing {
		a.readRemoteDirectory(session, run)
	}
}

// Only one directory operation runs per view. New navigation replaces the
// pending destination without canceling an active protocol write or response.
func (a *app) readRemoteDirectory(session *fileSession, run *fileRuntime) {
	run.listing = true
	sequence, navigation := run.sequence, run.navigation
	a.sshWorkers.Go(func() {
		entries, err := run.client.ReadDir(run.ctx, navigation.path)
		a.window.Update(func() {
			if session.runtime != run || run.ctx.Err() != nil {
				return
			}
			run.listing = false
			if sequence != run.sequence {
				select {
				case <-run.client.Done():
					session.state, session.errorText = connectionFailed, "文件连接已断开"
					session.remote.loading = false
				default:
					a.readRemoteDirectory(session, run)
				}
				return
			}
			pane := session.remote
			pane.loading = false
			if err != nil {
				pane.errorText = err.Error()
				select {
				case <-run.client.Done():
					session.state, session.errorText = connectionFailed, err.Error()
				default:
				}
				return
			}
			files := make([]fileEntry, 0, len(entries))
			for _, entry := range entries {
				files = append(files, fileEntry{name: entry.Name, path: path.Join(navigation.path, entry.Name),
					size: entry.Size, dir: entry.Directory, modified: entry.Modified,
					permissions: entry.Permissions, owner: entry.Owner})
			}
			if navigation.historyIndex >= 0 {
				pane.historyIndex = navigation.historyIndex
			} else if pane.path != navigation.path {
				pane.history = append(pane.history[:pane.historyIndex+1], navigation.path)
				pane.historyIndex = len(pane.history) - 1
			}
			pane.path = navigation.path
			pane.directories[navigation.path] = files
			pane.query, pane.anchor, pane.errorText = "", "", ""
			pane.selected, pane.scroll.Y = map[string]bool{}, 0
		})
	})
}
