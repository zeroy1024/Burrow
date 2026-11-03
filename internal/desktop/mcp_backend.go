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

	"sshdesk/internal/fileclient"
	"sshdesk/internal/mcpserver"
	"sshdesk/internal/sshclient"
	"sshdesk/internal/store"
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
	CreatedAt                                                       time.Time
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

func (b *mcpDesktop) Connections(ctx context.Context) ([]mcpserver.Connection, error) {
	return mcpOnUI(ctx, b, func() ([]mcpserver.Connection, error) { return b.captureConnections() })
}

func (b *mcpDesktop) captureConnections() ([]mcpserver.Connection, error) {
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
		chain := []identity{makeIdentity(h)}
		if protocol != "ftp" {
			plan, err := b.a.captureSSHPlan(h)
			if err != nil {
				return nil, err
			}
			chain = nil
			for _, step := range plan.steps {
				chain = append(chain, makeIdentity(step.endpoint))
			}
		}
		data, err := json.Marshal(chain)
		if err != nil {
			return nil, err
		}
		digest := store.EndpointID(data)
		session := b.sessions[h.session.ID+":"+digest]
		if session == nil {
			session = b.a.newFileSession(hostID(i), h, false, nil)
			b.observeGroup(session.connection)
			b.sessions[h.session.ID+":"+digest] = session
		}
		if protocol == "ftp" && h.session.Files != nil && h.session.Files.FTPTLS != "" && h.session.Files.FTPTLS != "none" {
			protocol = "ftps"
		}
		connections = append(connections, mcpserver.Connection{ID: h.session.ID, Name: h.name, Protocol: protocol,
			Host: h.address, Port: h.port, User: h.user, Identity: digest, Runtime: session})
	}
	return connections, nil
}

func (b *mcpDesktop) OpenFiles(ctx context.Context, c mcpserver.Connection) (fileclient.Client, error) {
	ready := make(chan transferClientResult, 1)
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
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

func (b *mcpDesktop) Exec(ctx context.Context, c mcpserver.Connection, command string) (sshclient.ExecResult, error) {
	type result struct {
		output sshclient.ExecResult
		err    error
	}
	ready := make(chan result, 1)
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
		request := &sshConnectionRequest{ctx: ctx, group: c.Runtime.(*fileSession).connection}
		request.mcpOperationID, request.mcpTaskID = mcpserver.OperationID(ctx)
		request.failed = func(err error) { ready <- result{err: err} }
		request.ready = func(connection *sshclient.Connection) {
			b.a.sshWorkers.Go(func() {
				output, err := connection.ExecCommand(ctx, command)
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
	file, err := os.CreateTemp("", "burrow-mcp-text-*")
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
	defer func() { result = errors.Join(result, remote.Close()) }()
	batch := &transferBatch{}
	batch.policy.Store(1)
	job := &transferJob{sourcePath: file.Name(), targetPath: path.Dir(remotePath), name: path.Base(remotePath),
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
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = errors.Join(err, cleanupTransferFiles(cleanupCtx, remote, job))
	}
	return err
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
		if !b.service.Accepting() || a.quitPending || len(a.transfers.tasks) >= 1000 || a.transfers.quitting || a.transfers.storeErr != nil || a.transfers.writeErr != nil {
			return false, errors.New("传输队列不可用或已达到容量限制")
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
			ConnectionID: request.Connection.ID, Identity: request.Connection.Identity, LocalRoot: root,
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
		return errors.New("MCP 服务未开启，不能继续任务")
	}
	_, err := mcpOnUI(ctx, b, func() (bool, error) {
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
			if t.mcp == nil || keyID != "" && t.mcp.KeyID != keyID || t.state == transferDone {
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
