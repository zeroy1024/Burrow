package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"sshdesk/internal/fileclient"
	"sshdesk/internal/sshclient"
	"sshdesk/internal/uikit"
)

type fileNavigation struct {
	path         string
	historyIndex int        // -1 appends a directory only after it was read successfully.
	link         *fileEntry // Only an explicit Open may resolve a directory link.
}

type fileRuntime struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     fileclient.Client
	request    *sshConnectionRequest
	listing    bool
	sequence   int
	navigation fileNavigation
	initial    string
	reveal     string
}

// Each browser and transfer gets its own FTP control connection or SFTP
// subsystem. The group's transport is shared, but canceling a consumer is not.
// This entry point and its completion callback run on the UI thread.
func (a *app) openFileClient(ctx context.Context, session *fileSession, complete func(fileclient.Client, error)) *sshConnectionRequest {
	h := cloneHostEndpoint(session.endpoint)
	var request *sshConnectionRequest
	finished := false
	finish := func(client fileclient.Client, err error) {
		if finished {
			if client != nil {
				a.sshWorkers.Go(func() { client.Close() })
			}
			return
		}
		finished = true
		a.completeSSHRequest(request)
		if ctx.Err() != nil {
			if client != nil {
				a.sshWorkers.Go(func() { client.Close() })
			}
			complete(nil, ctx.Err())
			return
		}
		complete(client, err)
	}
	deliver := func(client fileclient.Client, err error, release context.CancelFunc) {
		if client != nil {
			a.sshWorkers.Go(func() {
				if release != nil {
					defer release()
				}
				select {
				case <-ctx.Done():
					client.Close()
				case <-client.Done():
				}
			})
		} else if release != nil {
			release()
		}
		a.window.Update(func() { finish(client, err) })
	}
	context.AfterFunc(ctx, func() {
		a.window.Update(func() { cancelSSHRequest(request); finish(nil, ctx.Err()) })
	})
	if h.protocol == "ftp" {
		secret := session.ftpSecret
		a.sshWorkers.Go(func() {
			user := h.user
			var err error
			if h.session.Auth == "anonymous" {
				user, secret = "anonymous", "anonymous@"
			} else {
				if secret == "" {
					secret, err = a.sessionPassword(h)
					if errors.Is(err, mygo.ErrSecretNotFound) {
						err = nil
					}
				}
				if err == nil && secret == "" {
					answer := a.askSSH(ctx, &sshPrompt{title: "输入 FTP 密码", address: user + "@" + net.JoinHostPort(h.address, strconv.Itoa(h.port))})
					if !answer.accepted {
						err = errSSHConnectCanceled
					} else {
						secret = answer.secret
					}
				}
			}
			if err != nil {
				deliver(nil, err, nil)
				return
			}
			options := fileOptions{Encoding: "UTF-8", FTPMode: "passive", FTPTLS: "none"}
			if h.session.Files != nil {
				options = *h.session.Files
			}
			client, err := fileclient.DialFTP(ctx, fileclient.FTPConfig{
				Host: h.address, Port: h.port, User: user, Password: secret,
				TLSMode: options.FTPTLS, Mode: options.FTPMode, Encoding: options.Encoding,
			})
			if err == nil && h.session.Auth != "anonymous" {
				// Kept only for this runtime and its background jobs, never persisted.
				cached := secret
				a.window.Update(func() {
					if ctx.Err() == nil {
						session.ftpSecret = cached
					}
				})
			}
			secret = ""
			deliver(client, err, nil)
		})
		return nil
	}
	request = &sshConnectionRequest{ctx: ctx, group: session.connection, failed: func(err error) { finish(nil, err) }}
	request.ready = func(connection *sshclient.Connection) {
		encoding := "UTF-8"
		if h.session.Files != nil {
			encoding = h.session.Files.Encoding
		}
		a.sshWorkers.Go(func() {
			clientCtx, cancelClient := context.WithCancel(ctx)
			setupCtx, cancelSetup := context.WithTimeout(ctx, 20*time.Second)
			stopSetup := context.AfterFunc(setupCtx, cancelClient)
			var client fileclient.Client
			var err error
			for {
				if err = connection.WaitChannelReady(setupCtx); err != nil {
					break
				}
				if err = fileclient.WaitSFTPReady(setupCtx, connection); err != nil {
					break
				}
				client, err = fileclient.NewSFTP(clientCtx, connection, fileclient.SFTPOptions{Encoding: encoding})
				if !errors.Is(err, sshclient.ErrChannelPending) && !errors.Is(err, fileclient.ErrSFTPInitPending) {
					break
				}
			}
			// The setup budget bounds waits and initialization, while the successful
			// file subsystem keeps its independent lifetime until the caller closes it.
			stopSetup()
			setupErr := setupCtx.Err()
			cancelSetup()
			if setupErr != nil {
				err = setupErr
			}
			if err != nil {
				cancelClient()
				if client != nil {
					err = errors.Join(err, client.Close())
					client = nil
				}
			}
			deliver(client, err, cancelClient)
		})
	}
	a.acquireSSH(request)
	return request
}

func (a *app) reconnectFileSession(session *fileSession) { a.startFileSession(session, true) }

func (a *app) startFileSession(session *fileSession, refresh bool) {
	a.disconnectFileSession(session)
	if refresh {
		endpoint, id, changed := a.latestSessionEndpoint(session.endpoint, session.temporary)
		session.endpoint, session.config, session.host = endpoint, endpoint.session, id
		if changed {
			session.ftpSecret = ""
		}
		if endpoint.protocol == "ftp" {
			session.connection = nil
		} else {
			session.connection = a.reconnectSSHGroup(session.connection, endpoint)
		}
	}
	a.ensureLocalFiles(session)
	ctx, cancel := context.WithCancel(a.ctx)
	run := &fileRuntime{ctx: ctx, cancel: cancel, navigation: fileNavigation{historyIndex: -1}}
	session.runtime, session.state, session.errorText = run, connecting, ""
	session.remote.loading, session.remote.errorText = true, ""
	run.request = a.openFileClient(ctx, session, func(client fileclient.Client, err error) {
		if session.runtime != run || ctx.Err() != nil {
			if client != nil {
				a.sshWorkers.Go(func() { client.Close() })
			}
			return
		}
		if err != nil {
			cancel()
			session.state, session.errorText = connectionFailed, err.Error()
			if errors.Is(err, errSSHConnectCanceled) || errors.Is(err, context.Canceled) {
				session.state, session.errorText = disconnected, ""
			}
			session.remote.loading = false
			return
		}
		run.client = client
		a.sshWorkers.Go(func() {
			initial, err := client.InitialPath(ctx)
			a.window.Update(func() {
				if session.runtime != run || ctx.Err() != nil {
					return
				}
				if err != nil {
					cancel()
					session.state, session.errorText = connectionFailed, err.Error()
					session.remote.loading = false
					return
				}
				run.initial = initial
				session.state, session.errorText, session.connectedAt = connected, "", time.Now()
				navigation := run.navigation
				if navigation.path == "" {
					navigation = fileNavigation{path: session.remote.path, historyIndex: session.remote.historyIndex}
				}
				if navigation.path == "" {
					navigation = fileNavigation{path: initial, historyIndex: -1}
				}
				a.queueFileDirectory(session, session.remote, navigation)
			})
		})
		a.sshWorkers.Go(func() {
			select {
			case <-ctx.Done():
				return
			case <-client.Done():
			}
			a.window.Update(func() {
				if session.runtime == run && ctx.Err() == nil {
					session.state, session.errorText, session.remote.loading = connectionFailed, "文件连接已断开", false
				}
			})
		})
	})
}

func (a *app) ensureLocalFiles(session *fileSession) {
	if session.localRuntime != nil {
		return
	}
	a.ensureLocalFilesIfNeeded(session)
	a.queueFileDirectory(session, session.local, fileNavigation{path: session.local.path, historyIndex: session.local.historyIndex})
}

func (a *app) closeLocalFiles(session *fileSession) {
	if run := session.localRuntime; run != nil {
		session.localRuntime = nil
		run.cancel()
	}
}

func (a *app) disconnectFileSession(session *fileSession) {
	if run := session.runtime; run != nil {
		session.runtime = nil
		run.cancel()
		cancelSSHRequest(run.request)
	}
	session.state, session.errorText, session.remote.loading = disconnected, "", false
}

func (a *app) refreshRemoteFiles(session *fileSession) { a.refreshFilePane(session, session.remote) }
func (a *app) refreshFilePane(session *fileSession, pane *filePaneState) {
	a.queueFileDirectory(session, pane, fileNavigation{path: pane.path, historyIndex: pane.historyIndex})
}
func (a *app) queueRemoteDirectory(session *fileSession, navigation fileNavigation) {
	a.queueFileDirectory(session, session.remote, navigation)
}

func (a *app) queueFileDirectory(session *fileSession, pane *filePaneState, navigation fileNavigation) {
	if !pane.remote {
		a.ensureLocalFilesIfNeeded(session)
	}
	run := session.localRuntime
	if pane.remote {
		run = session.runtime
	}
	if run == nil {
		pane.errorText = "文件会话未连接"
		return
	}
	run.sequence++
	run.navigation = navigation
	reveal := pane.reveal
	if reveal == "" {
		reveal = run.reveal
	}
	if reveal != "" && path.Dir(reveal) == path.Clean(navigation.path) {
		run.reveal = reveal
	} else {
		run.reveal = ""
	}
	pane.reveal = ""
	pane.loading, pane.errorText = true, ""
	if run.client != nil && (!pane.remote || session.state == connected) && !run.listing {
		a.readFileDirectory(session, pane, run)
	}
}

// No recursive call to queueFileDirectory: it owns the first request as well.
func (a *app) ensureLocalFilesIfNeeded(session *fileSession) {
	if session.localRuntime != nil {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	client := fileclient.NewLocal()
	session.localRuntime = &fileRuntime{ctx: ctx, cancel: cancel, client: client}
	a.sshWorkers.Go(func() { <-ctx.Done(); client.Close() })
}

func (a *app) readFileDirectory(session *fileSession, pane *filePaneState, run *fileRuntime) {
	run.listing = true
	sequence, navigation := run.sequence, run.navigation
	a.sshWorkers.Go(func() {
		location := navigation.path
		var err error
		if pane.remote {
			location = remoteFileLocation(run.initial, location)
		} else {
			location, err = localFileLocation(location)
		}
		var entries []fileclient.Entry
		if err == nil && navigation.link != nil {
			resolver, ok := run.client.(fileclient.DirectoryResolver)
			if !ok {
				err = errors.New("此文件连接不支持解析目录链接")
			} else {
				location, err = resolver.ResolveDirectory(run.ctx, location)
			}
		}
		if err == nil {
			entries, err = run.client.ReadDir(run.ctx, location)
		}
		a.window.Update(func() {
			current := session.localRuntime
			if pane.remote {
				current = session.runtime
			}
			if current != run || run.ctx.Err() != nil {
				return
			}
			run.listing = false
			if sequence != run.sequence {
				a.readFileDirectory(session, pane, run)
				return
			}
			pane.loading = false
			if navigation.link != nil && errors.Is(err, fileclient.ErrNotDirectory) {
				a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "preview", preview: *navigation.link}
				return
			}
			if err != nil {
				pane.errorText = err.Error()
				a.files.notice = &uikit.ToastProps{ID: "directory-error", Tone: uikit.ToneDanger, Title: "读取目录失败", Detail: err.Error()}
				return
			}
			files := make([]fileEntry, 0, len(entries))
			for _, entry := range entries {
				name := path.Join(location, entry.Name)
				if !pane.remote {
					name = filepath.Join(location, entry.Name)
				}
				files = append(files, fileEntry{name: entry.Name, path: name, size: entry.Size, dir: entry.Directory, symlink: entry.Symlink, modified: entry.Modified, permissions: entry.Permissions, owner: entry.Owner})
			}
			changed := pane.path != location
			if len(pane.history) == 0 {
				pane.history, pane.historyIndex = []string{location}, 0
			} else if navigation.historyIndex >= 0 && navigation.historyIndex < len(pane.history) {
				pane.historyIndex = navigation.historyIndex
				pane.history[pane.historyIndex] = location
			} else if changed || len(pane.history) == 0 {
				pane.history = append(pane.history[:pane.historyIndex+1], location)
				pane.historyIndex = len(pane.history) - 1
			}
			pane.path, pane.directories, pane.errorText = location, map[string][]fileEntry{location: files}, ""
			if changed {
				pane.query = ""
				pane.table.Clear()
				pane.table.List.ScrollTo(0, 0)
				pane.scroll.Y = 0
			}
			if run.reveal != "" && path.Dir(run.reveal) == location {
				pane.reveal = run.reveal
				pane.table.Select(pane.reveal)
				run.reveal = ""
			}
			if !pane.remote {
				if key := a.fileSiteKey(session); key != "" {
					a.files.localPaths[key] = location
				}
				if err := a.writeFilePreferences(); err != nil {
					pane.errorText = "保存目录历史：" + err.Error()
				}
			}
		})
	})
}

func remoteFileLocation(initial, location string) string {
	if location == "" || location == "~" {
		return path.Clean(initial)
	}
	if strings.HasPrefix(location, "~/") {
		return path.Join(initial, location[2:])
	}
	if !path.IsAbs(location) {
		return path.Join(initial, location)
	}
	return path.Clean(location)
}

func localFileLocation(location string) (string, error) {
	if location == "~" || strings.HasPrefix(location, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		location = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(location, "~"), "/"))
	}
	if location == "" {
		return "", fmt.Errorf("目录不能为空")
	}
	return filepath.Abs(location)
}
