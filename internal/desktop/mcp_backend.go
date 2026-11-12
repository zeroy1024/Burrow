package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/mcpserver"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/wggateway"
)

type mcpDesktop struct {
	a           *app
	service     *mcpserver.Service
	sessions    map[string]*fileSession // Window-thread owned, independent of terminal groups.
	refreshing  bool
	refreshDue  bool
	busy        bool
	keys        []store.MCPKey
	policies    []store.MCPPolicy
	connections []mcpserver.Connection
}

type mcpTransferOwner struct {
	KeyID, OperationID, ConnectionID, Identity, LocalRoot, Conflict string
	Outcome                                                         string
	CleanupOnly                                                     bool
	CreatedAt                                                       time.Time
	PolicyConnectionID                                              string `json:",omitempty"`
}

func mcpOnUI[T any](ctx context.Context, b *mcpDesktop, fn func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	ready := make(chan result, 1)
	b.a.window.Update(func() {
		if err := ctx.Err(); err != nil {
			ready <- result{err: err}
			return
		}
		value, err := fn()
		ready <- result{value, err}
	})
	select {
	case r := <-ready:
		return r.value, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case <-b.a.ctx.Done():
		var zero T
		return zero, b.a.ctx.Err()
	}
}

func (b *mcpDesktop) Available(ctx context.Context) error {
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
		if !b.a.businessReady() {
			return false, errAppUnavailable
		}
		return true, nil
	})
	return err
}

func (b *mcpDesktop) Connections(ctx context.Context) ([]mcpserver.Connection, error) {
	return mcpOnUI(ctx, b, func() ([]mcpserver.Connection, error) {
		if !b.a.businessReady() {
			return nil, errAppUnavailable
		}
		return b.captureConnections()
	})
}

func (b *mcpDesktop) captureConnections() ([]mcpserver.Connection, error) {
	if !b.a.businessReady() && b.a.security.initialized {
		return nil, errAppUnavailable
	}
	connections := make([]mcpserver.Connection, 0, len(b.a.hosts))
	for i, h := range b.a.hosts {
		h = cloneHostEndpoint(h)
		protocol := h.protocol
		if protocol == "" {
			protocol = "ssh"
		}
		type identity struct {
			Protocol, Host, User, Auth, KeyID, Revision, KeyFingerprint string
			Port                                                        int
			Files                                                       *fileOptions
			SOCKS                                                       *socksGateway     `json:",omitempty"`
			WireGuard                                                   *wggateway.Config `json:",omitempty"`
		}
		makeIdentity := func(endpoint host) identity {
			var options *fileOptions
			if endpoint.session.Files != nil {
				copy := *endpoint.session.Files
				copy.LocalDir, copy.RemoteDir = "", ""
				options = &copy
			}
			fingerprint := ""
			for _, key := range b.a.keys {
				if key.ID == endpoint.session.KeyID {
					fingerprint = key.Fingerprint
				}
			}
			return identity{Protocol: endpoint.protocol, Host: endpoint.address, User: endpoint.user,
				Auth: endpoint.session.Auth, KeyID: endpoint.session.KeyID, Revision: endpoint.session.PasswordRevision,
				Port: endpoint.port, Files: options, KeyFingerprint: fingerprint}
		}
		plan, err := b.a.captureSSHPlan(h)
		if err != nil {
			return nil, err
		}
		chain := make([]identity, 0, len(plan.route)+1)
		sshIndex := 0
		for _, hop := range plan.route {
			if hop.WG != nil {
				chain = append(chain, identity{WireGuard: hop.WG})
				continue
			}
			if hop.Kind == "socks5" {
				chain = append(chain, identity{SOCKS: hop.SOCKS})
				continue
			}
			chain = append(chain, makeIdentity(plan.steps[sshIndex].endpoint))
			sshIndex++
		}
		chain = append(chain, makeIdentity(h))

		data, err := json.Marshal(chain)
		if err != nil {
			return nil, err
		}
		digest := store.EndpointID(data)
		session := b.sessions[h.session.ID+":"+digest]
		if session == nil || !sameGatewayRoute(session.endpoint.session.GatewayRoute, &plan.route) {
			route := plan.route
			h.session.GatewayRoute = &route
			fileEndpoint := h
			if protocol == "ssh" {
				// Queue snapshots are file endpoints, even when the grant belongs
				// to an SSH profile. Keep the original identity above unchanged.
				fileEndpoint.protocol = "sftp"
			}
			session = b.a.newFileSession(hostID(i), fileEndpoint, false, nil)
			session.gatewayScope = "mcp:" + session.restoreID
			b.observeGroup(session.connection)
			b.sessions[h.session.ID+":"+digest] = session
		}
		session.host = hostID(i)
		if protocol == "ftp" && h.session.Files != nil && h.session.Files.FTPTLS != "" && h.session.Files.FTPTLS != "none" {
			protocol = "ftps"
		}
		connection := mcpserver.Connection{ID: h.session.ID, Name: h.name, Protocol: protocol,
			Host: h.address, Port: h.port, User: h.user, Identity: digest, Runtime: session}
		if protocol == "ssh" {
			platform := b.a.knownPlatform(h)
			connection.OS = string(platform)
			if facts := b.a.savedHostFacts(h); facts != nil && facts.Platform == platform {
				connection.OSName, connection.Arch, connection.Shell = facts.OSName, facts.Arch, facts.Shell
			}
		}
		connections = append(connections, connection)
	}
	b.groupConnections(connections)
	return connections, nil
}

func (b *mcpDesktop) OpenFiles(ctx context.Context, c mcpserver.Connection) (fileclient.Client, error) {
	ready := make(chan transferClientResult, 1)
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
		if !b.a.businessReady() {
			return false, errAppUnavailable
		}
		b.a.openFileClient(ctx, c.Runtime.(*fileSession), func(client fileclient.Client, err error) {
			ready <- transferClientResult{client, err}
		})
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	select {
	case result := <-ready:
		return result.client, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *mcpDesktop) Exec(ctx context.Context, c mcpserver.Connection, command string, observe func(stderr bool, data []byte)) (sshclient.ExecResult, error) {
	type result struct {
		output sshclient.ExecResult
		err    error
	}
	ready := make(chan result, 1)
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
		if !b.a.businessReady() {
			return false, errAppUnavailable
		}
		request := &sshConnectionRequest{ctx: ctx, group: c.Runtime.(*fileSession).connection}
		request.mcpOperationID, request.mcpTaskID = mcpserver.OperationID(ctx)
		request.failed = func(err error) { ready <- result{err: err} }
		request.ready = func(connection *sshclient.Connection) {
			b.a.sshWorkers.Go(func() {
				output, err := connection.ExecObserved(ctx, command, observe)
				ready <- result{output, err}
				b.a.window.Update(func() { b.a.completeSSHRequest(request) })
			})
		}
		context.AfterFunc(ctx, func() { b.a.window.Update(func() { cancelSSHRequest(request) }) })
		b.a.acquireSSH(request)
		return true, nil
	})
	if err != nil {
		return sshclient.ExecResult{}, err
	}
	select {
	case r := <-ready:
		return r.output, r.err
	case <-ctx.Done():
		return sshclient.ExecResult{}, ctx.Err()
	}
}

func (b *mcpDesktop) WriteText(ctx context.Context, c mcpserver.Connection, remotePath, text, mode string,
	check func(context.Context, fileclient.Client, string, bool) error) (result error) {
	name := path.Base(remotePath)
	if !transferSafeName(name) {
		return fs.ErrInvalid
	}
	dir, err := os.MkdirTemp("", "burrow-mcp-text-*")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.Remove(dir)) }()
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.Remove(file.Name())) }()
	_, writeErr := file.WriteString(text)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	local, err := fileclient.OpenLocalSource(ctx, file.Name())
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, local.Close()) }()
	remote, err := b.OpenFiles(ctx, c)
	if err != nil {
		return err
	}
	defer func() {
		if remote != nil {
			result = errors.Join(result, remote.Close())
		}
	}()
	batch := &transferBatch{}
	batch.policy.Store(1)
	job := &transferJob{sourcePath: file.Name(), targetPath: path.Dir(remotePath), name: name,
		policy: 1, batch: batch, temporary: make(map[string]struct{}), destinationKey: fmt.Sprintf("%q/%q/%q/%d", c.Runtime.(*fileSession).endpoint.protocol, c.User, c.Host, c.Port),
		mcpCheck: func(ctx context.Context, source, target fileclient.Client, node *transferNode, destination string) error {
			entry, err := target.Stat(ctx, destination)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err == nil && (entry.Symlink || entry.Directory) {
				return fs.ErrInvalid
			}
			return check(ctx, target, destination, err == nil)
		}}
	run := &transferRun{ctx: ctx}
	task := &transferTask{job: job, run: run}
	var locks *transferLocks
	_, err = mcpOnUI(ctx, b, func() (bool, error) { locks = &b.a.transfers.locks; return true, nil })
	if err != nil {
		return err
	}
	err = b.a.performTransfer(ctx, task, run, job, local, remote, locks)
	if err != nil && len(job.temporary) > 0 {
		err = errors.Join(err, remote.Close())
		remote = nil
		// Cancellation closes the original protocol client. Cleanup owns a new
		// bounded connection and can never restart the business write.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		cleanupClient, cleanupErr := b.OpenFiles(cleanupCtx, c)
		if cleanupErr == nil {
			cleanupErr = errors.Join(cleanupTransferFiles(cleanupCtx, cleanupClient, job), cleanupClient.Close())
		}
		cancel()
		err = errors.Join(err, cleanupErr)
		if len(job.temporary) > 0 {
			err = errors.Join(err, b.keepWriteCleanup(ctx, c, job, dir, err))
		}
	}
	return err
}

func (b *mcpDesktop) keepWriteCleanup(ctx context.Context, c mcpserver.Connection, job *transferJob, root string, cause error) error {
	// This is already-owned remote work, so service/key shutdown or the normal
	// business queue limit must not discard its cleanup journal.
	saveCtx, cancel := context.WithTimeout(b.a.ctx, 30*time.Second)
	defer cancel()
	operationID, _ := mcpserver.OperationID(ctx)
	op, err := b.a.store.MCPOperation(saveCtx, operationID)
	if err != nil {
		return b.service.FailStorage(err)
	}
	ready := make(chan error, 1)
	_, err = mcpOnUI(saveCtx, b, func() (bool, error) {
		a := b.a
		a.transfers.nextID++
		a.transfers.nextBatchID++
		job.batch.id = a.transfers.nextBatchID
		owner := &mcpTransferOwner{KeyID: op.KeyID, OperationID: op.ID, ConnectionID: c.ID, Identity: c.Identity, PolicyConnectionID: c.CanonicalID,
			LocalRoot: root, Conflict: "overwrite", CleanupOnly: true, CreatedAt: time.Now().UTC()}
		task := &transferTask{id: a.transfers.nextID, mcp: owner, job: job,
			session: a.snapshotTransferSession(c.Runtime.(*fileSession)),
			state:   transferFailed, cleanupPending: true, errorText: locales.Concat(
				locales.Deferred(locales.MessageTemporaryFileAwaitingCleanupAfterWriting),
				locales.ErrorText(cause),
			),
			sourcePath: job.sourcePath, targetPath: job.targetPath, total: job.total, transferred: job.doneBytes,
			entry: fileEntry{name: job.name, path: job.sourcePath, size: job.root.entry.Size}}
		a.transfers.tasks = append(a.transfers.tasks, task)
		a.persistTransfersDone(func(err error) {
			ready <- err
		})
		return true, nil
	})
	if err == nil {
		select {
		case err = <-ready:
		case <-saveCtx.Done():
			err = saveCtx.Err()
		}
	}
	if err != nil {
		return b.service.FailStorage(err)
	}
	return locales.NewError(locales.MessageAnOwnedTemporaryFileStillNeedsCleanupACleanupTaskHasBeenSaved)
}

func (b *mcpDesktop) Enqueue(ctx context.Context, request mcpserver.TransferRequest) (store.MCPTask, error) {
	var entry fileclient.Entry
	var err error
	if request.Download {
		client, openErr := b.OpenFiles(ctx, request.Connection)
		if openErr != nil {
			return store.MCPTask{}, openErr
		}
		entry, err = client.Stat(ctx, request.RemotePath)
		err = errors.Join(err, client.Close())
	} else {
		info, statErr := os.Lstat(request.LocalPath)
		if statErr != nil {
			return store.MCPTask{}, statErr
		}
		entry = fileclient.Entry{Name: info.Name(), Size: info.Size(), Directory: info.IsDir(), Symlink: info.Mode()&os.ModeSymlink != 0,
			Mode: info.Mode(), ModeKnown: true, Modified: info.ModTime()}
	}
	if err != nil {
		return store.MCPTask{}, err
	}
	if entry.Symlink || !entry.Directory && entry.ModeKnown && !entry.Mode.IsRegular() {
		return store.MCPTask{}, fs.ErrInvalid
	}
	root := ""
	dirs := request.Key.UploadDirs
	if request.Download {
		dirs = request.Key.DownloadDirs
	}
	for _, dir := range dirs {
		rel, relErr := filepath.Rel(dir, request.LocalPath)
		if relErr == nil && filepath.IsLocal(rel) {
			root = dir
			break
		}
	}
	if root == "" {
		return store.MCPTask{}, fs.ErrPermission
	}
	// Once inserted into the application queue, acceptance outlives the HTTP
	// request. The reply is delivered only after the queue and ownership commit.
	acceptCtx, cancel := context.WithTimeout(b.a.ctx, 30*time.Second)
	defer cancel()
	ready := make(chan error, 1)
	var saved store.MCPTask
	_, err = mcpOnUI(acceptCtx, b, func() (bool, error) {
		a := b.a
		if a.siteDeletionPending(request.Connection.ID) || !slices.ContainsFunc(a.hosts, func(h host) bool { return h.session.ID == request.Connection.ID }) {
			return false, locales.NewError(locales.SessionUnavailableForTransfer)
		}
		if !a.businessReady() || !b.service.Accepting() || a.quitPending || len(a.transfers.tasks) >= 1000 || a.transfers.quitting || a.transfers.storeErr != nil || a.transfers.writeErr != nil {
			return false, locales.NewError(locales.MessageTheTransferQueueIsUnavailableOrAtCapacity)
		}
		a.transfers.nextID++
		a.transfers.nextBatchID++
		batch := &transferBatch{id: a.transfers.nextBatchID}
		policy := transferPolicy(request.Conflict)
		batch.policy.Store(int32(policy))
		source, target := request.LocalPath, request.RemotePath
		if request.Download {
			source, target = request.RemotePath, request.LocalPath
		}
		owner := &mcpTransferOwner{KeyID: request.Key.ID, OperationID: request.Operation.ID,
			ConnectionID: request.Connection.ID, Identity: request.Connection.Identity, PolicyConnectionID: request.Connection.CanonicalID, LocalRoot: root,
			Conflict: request.Conflict, CreatedAt: time.Now().UTC()}
		session := a.snapshotTransferSession(request.Connection.Runtime.(*fileSession))
		task := &transferTask{id: a.transfers.nextID, session: session, sourceRemote: request.Download,
			sourcePath: source, targetPath: target, state: transferQueued, mcp: owner,
			entry: fileEntry{name: transferBase(request.Download, source), path: source, size: entry.Size,
				dir: entry.Directory, symlink: entry.Symlink, modified: entry.Modified, permissions: entry.Permissions, owner: entry.Owner}}
		task.job = &transferJob{sourceRemote: request.Download, sourcePath: source, targetPath: target,
			name: task.entry.name, policy: policy, batch: batch, temporary: make(map[string]struct{})}
		a.transfers.tasks = append(a.transfers.tasks, task)
		b.attachTransfer(task)
		saved = store.MCPTask{ID: task.id, KeyID: owner.KeyID, OperationID: owner.OperationID, ConnectionID: owner.ConnectionID,
			State: "queued", Total: max(entry.Size, 0), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		a.persistTransfersDone(func(err error) {
			if err != nil {
				a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(t *transferTask) bool { return t == task })
			}
			if err == nil {
				a.mcpTransferChanged(task, nil)
			}
			ready <- err
		})
		return true, nil
	})
	if err != nil {
		return store.MCPTask{}, err
	}
	select {
	case err = <-ready:
		return saved, err
	case <-acceptCtx.Done():
		return store.MCPTask{}, acceptCtx.Err()
	}
}

func (b *mcpDesktop) ControlTask(ctx context.Context, id int, action string) error {
	if action == "resume" && !b.service.Accepting() {
		return locales.NewError(locales.MessageTheMcpServiceIsOffThisTaskCannotBeResumed)
	}
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
		if action == "resume" && !b.a.businessReady() {
			return false, errAppUnavailable
		}
		for _, t := range b.a.transfers.tasks {
			if t.id != id || t.mcp == nil {
				continue
			}
			switch action {
			case "cancel":
				b.a.cancelTransfer(t)
			case "pause":
				b.a.pauseTransfer(t)
			case "resume":
				b.a.resumeTransfer(t)
			default:
				return false, errors.New("unknown task action")
			}
			return true, nil
		}
		return true, nil // A finished task remains queryable in its audit identity.
	})
	return err
}

func (b *mcpDesktop) Quiesce(ctx context.Context, keyID string, pause bool) error {
	var key store.MCPKey
	var err error
	if keyID != "" {
		key, err = b.a.store.MCPKey(ctx, keyID)
		if err != nil {
			return err
		}
	}
	_, err = mcpOnUI(ctx, b, func() (bool, error) {
		for _, t := range slices.Clone(b.a.transfers.tasks) {
			if t.mcp == nil || t.mcp.CleanupOnly || keyID != "" && t.mcp.KeyID != keyID || t.state == transferDone {
				continue
			}
			if keyID != "" && key.State == "active" && mcpTaskAllowed(key, t) {
				continue
			}
			if pause && t.state != transferQueued && t.state != transferRunning {
				continue
			}
			if pause {
				b.a.pauseTransfer(t)
			} else {
				b.a.cancelTransfer(t)
			}
		}
		return true, nil
	})
	return err
}

func mcpTaskAllowed(key store.MCPKey, task *transferTask) bool {
	for _, g := range key.Grants {
		if g.ConnectionID == task.mcp.ConnectionID && g.EndpointDigest == task.mcp.Identity && (task.sourceRemote && g.Read || !task.sourceRemote && g.Write) {
			local := task.sourcePath
			if task.sourceRemote {
				local = task.targetPath
			}
			dirs := key.UploadDirs
			if task.sourceRemote {
				dirs = key.DownloadDirs
			}
			for _, dir := range dirs {
				rel, err := filepath.Rel(dir, local)
				if err == nil && filepath.IsLocal(rel) {
					return true
				}
			}
		}
	}
	return false
}
