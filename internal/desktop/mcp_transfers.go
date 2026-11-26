package desktop

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/mcpserver"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/transfer"
)

func (b *mcpDesktop) preflightTransfer(ctx context.Context, task *transferTask) error {
	capability, local := "write", task.Job.SourcePath
	if task.SourceRemote {
		capability, local = "read", task.Job.TargetPath
	}
	key, connection, err := b.service.Authorize(ctx, task.MCP.KeyID, task.MCP.ConnectionID, capability)
	if err != nil {
		return err
	}
	if connection.Identity != task.MCP.Identity {
		return &mcpserver.Failure{Code: "permission_denied", Message: locales.MessageTheTaskSConnectionIdentityHasChanged()}
	}
	_, err = mcpserver.AuthorizeLocal(key, local, task.SourceRemote)
	return err
}

func (b *mcpDesktop) checkTransfer(ctx context.Context, task *transferTask, name, side, action string, client fileclient.Client) error {
	owner := task.MCP
	capability := "write"
	if task.SourceRemote {
		capability = "read"
	}
	key, connection, err := b.service.Authorize(ctx, owner.KeyID, owner.ConnectionID, capability)
	if err != nil {
		return err
	}
	if connection.Identity != owner.Identity {
		return &mcpserver.Failure{Code: "permission_denied", Message: locales.MessageTheTransferEndpointIdentityHasChangedAuthorizationIsRequiredAgain()}
	}
	if side == "local" {
		if _, err := mcpserver.AuthorizeLocal(key, name, task.SourceRemote); err != nil {
			return err
		}
	}
	return b.service.CheckFile(ctx, owner.KeyID, owner.ConnectionID, side, name, action, client, owner.PolicyConnectionID, connection.DisplayID())
}

func (b *mcpDesktop) attachTransfer(task *transferTask) {
	b.observeGroup(task.Session.connection)
	job := task.Job
	job.MCPFailConflict = task.MCP.Conflict == "error"
	job.MCPCheck = func(ctx context.Context, source, target fileclient.Client, node *transferNode, destination string) error {
		sourceSide, targetSide := "local", "remote"
		if task.SourceRemote {
			sourceSide, targetSide = "remote", "local"
		}
		sourceAction := "read"
		if node.Entry.Directory {
			sourceAction = "browse"
		}
		node.MCPPath, node.MCPSide, node.MCPAction = node.Source, sourceSide, sourceAction
		if err := b.checkTransfer(ctx, task, node.Source, sourceSide, sourceAction, source); err != nil {
			return err
		}
		existing, err := target.Stat(ctx, destination)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err == nil && existing.Symlink {
			return fs.ErrInvalid
		}
		if err == nil && node.Entry.Directory && existing.Directory {
			node.MCPPath, node.MCPSide, node.MCPAction = destination, targetSide, "merge"
			return nil
		}
		action := "create"
		if err == nil {
			action = "write"
		}
		node.MCPPath, node.MCPSide, node.MCPAction = destination, targetSide, action
		if err := b.checkTransfer(ctx, task, destination, targetSide, action, target); err != nil {
			return err
		}
		auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer auditCancel()
		if err := b.a.store.MCPAddFile(auditCtx, store.MCPFileEvent{OperationID: task.MCP.OperationID,
			Path: destination, Side: targetSide, Action: action, Status: "running", At: time.Now().UTC()}); err != nil {
			return b.service.FailStorage(err)
		}
		return nil
	}
	job.MCPFinish = func(ctx context.Context, node *transferNode, result error) error {
		side, name, action := node.MCPSide, node.MCPPath, node.MCPAction
		if node.Skipped {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			sourceSide := "local"
			if task.SourceRemote {
				sourceSide = "remote"
			}
			if err := b.a.store.MCPAddFiles(ctx, []store.MCPFileEvent{
				{OperationID: task.MCP.OperationID, Path: name, Side: side, Action: action, Status: "skipped", At: time.Now().UTC()},
				{OperationID: task.MCP.OperationID, Path: node.Source, Side: sourceSide, Action: "read", Status: "skipped", At: time.Now().UTC()},
			}); err != nil {
				return b.service.FailStorage(err)
			}
			return nil
		}
		return b.service.FileEvent(ctx, task.MCP.OperationID, name, side, action, result, max(node.Entry.Size, 0))
	}
}

type mcpSourceClient struct {
	fileclient.Client
	b       *mcpDesktop
	task    *transferTask
	side    string
	entries int
}

func (b *mcpDesktop) sourceClient(task *transferTask, client fileclient.Client) fileclient.Client {
	side := "local"
	if task.SourceRemote {
		side = "remote"
	}
	return &mcpSourceClient{Client: client, b: b, task: task, side: side}
}

func (c *mcpSourceClient) ResolveDirectory(ctx context.Context, name string) (string, error) {
	resolver, ok := c.Client.(fileclient.DirectoryResolver)
	if !ok {
		return "", fs.ErrInvalid
	}
	return resolver.ResolveDirectory(ctx, name)
}

func (c *mcpSourceClient) ResolveCanonicalDirectory(ctx context.Context, name string) (string, error) {
	resolver, ok := c.Client.(fileclient.CanonicalDirectoryResolver)
	if !ok {
		return "", fs.ErrInvalid
	}
	return resolver.ResolveCanonicalDirectory(ctx, name)
}

func (c *mcpSourceClient) ReadDir(ctx context.Context, name string) ([]fileclient.Entry, error) {
	if err := c.b.checkTransfer(ctx, c.task, name, c.side, "browse", c.Client); err != nil {
		return nil, err
	}
	entries, err := c.Client.ReadDir(ctx, name)
	c.entries += len(entries)
	if len(entries) > 10000 || c.entries > 100000 {
		err = locales.NewError(locales.MessageMcpDirectoryLimitExceededEntriesPerDirectoryPerTask)
	}
	if auditErr := c.b.service.FileEvent(context.WithoutCancel(ctx), c.task.MCP.OperationID, name, c.side, "browse", err, 0); auditErr != nil {
		err = errors.Join(err, auditErr)
	}
	if err == nil {
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		events := make([]store.MCPFileEvent, 0, len(entries))
		for _, entry := range entries {
			status := "pending"
			if entry.Symlink {
				status = "skipped"
			}
			events = append(events, store.MCPFileEvent{OperationID: c.task.MCP.OperationID,
				Path: transfer.Join(c.task.SourceRemote, name, entry.Name), Side: c.side, Action: "enumerated", Status: status, At: time.Now().UTC()})
		}
		if addErr := c.b.a.store.MCPAddFiles(auditCtx, events); addErr != nil {
			return nil, c.b.service.FailStorage(addErr)
		}
	}
	return entries, err
}

func (c *mcpSourceClient) OpenRead(ctx context.Context, name string, offset int64) (io.ReadCloser, error) {
	if err := c.b.checkTransfer(ctx, c.task, name, c.side, "read", c.Client); err != nil {
		return nil, err
	}
	reader, err := c.Client.OpenRead(ctx, name, offset)
	if err != nil {
		return nil, err
	}
	return &mcpReadStream{ReadCloser: reader, done: func(size int64, result error) error {
		return c.b.service.FileEvent(context.WithoutCancel(ctx), c.task.MCP.OperationID, name, c.side, "read", result, size)
	}}, nil
}

type mcpReadStream struct {
	io.ReadCloser
	bytes int64
	err   error
	done  func(int64, error) error
}

func (r *mcpReadStream) Read(data []byte) (int, error) {
	n, err := r.ReadCloser.Read(data)
	r.bytes += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}

func (r *mcpReadStream) Close() error {
	err := errors.Join(r.err, r.ReadCloser.Close())
	return errors.Join(err, r.done(r.bytes, err))
}

func (a *app) mcpTaskSnapshot(task *transferTask, result error) store.MCPTask {
	owner := task.MCP
	state := map[transferState]string{transferQueued: "queued", transferRunning: "running", transferPaused: "paused", transferDone: "completed", transferFailed: "failed"}[task.State]
	if task.State == transferFailed && owner.Outcome != "" {
		state = owner.Outcome
	}
	if task.RemoveRequested {
		state = "cancelled"
		if task.Run != nil {
			state = "cancel_requested"
		}
	}
	if owner.CleanupOnly {
		state = "cleanup_pending"
		if task.Run != nil {
			state = "cleaning"
		} else if len(task.Job.Temporary) == 0 {
			state = "completed"
		}
	}
	message := task.ErrorText.String()
	if result != nil && task.State != transferPaused {
		message = result.Error()
	}
	if owner.CleanupOnly && state == "completed" {
		message = ""
	}
	done, total := task.Transferred, task.Total
	if task.Run != nil {
		done, total = task.Run.Bytes.Load(), task.Run.Total.Load()
	}
	return store.MCPTask{ID: task.ID, KeyID: owner.KeyID, OperationID: owner.OperationID,
		ConnectionID: owner.ConnectionID, State: state, Error: message, Done: max(done, 0),
		Total: max(total, done), CreatedAt: owner.CreatedAt, UpdatedAt: time.Now().UTC()}
}

func (a *app) mcpTransferChanged(task *transferTask, result error) {
	if task.MCP == nil || a.mcpBackend == nil {
		return
	}
	if errors.Is(result, context.Canceled) && (task.State == transferQueued || task.State == transferPaused || task.State == transferRunning) {
		result = nil
	}
	setMCPTransferOutcome(task, result)
	snapshot := a.mcpTaskSnapshot(task, result)
	cleanupOnly := task.MCP.CleanupOnly
	if !cleanupOnly {
		a.agentTransferChanged(snapshot.OperationID, snapshot.State, snapshot.Error)
	}
	a.persist(func() error {
		if cleanupOnly {
			return a.store.MCPSaveTaskEvent(a.ctx, snapshot)
		}
		if err := a.store.MCPSaveTask(a.ctx, snapshot); err != nil {
			return err
		}
		op, err := a.store.MCPOperation(a.ctx, snapshot.OperationID)
		if err != nil {
			return err
		}
		op.Status, op.Error = snapshot.State, snapshot.Error
		if snapshot.State == "queued" || snapshot.State == "running" || snapshot.State == "paused" {
			op.FinishedAt, op.ErrorCode = time.Time{}, ""
		}
		if result != nil {
			var failure *mcpserver.Failure
			if errors.As(result, &failure) {
				op.ErrorCode = failure.Code
				for _, hit := range failure.Hits {
					if !slices.Contains(op.Hits, hit) {
						op.Hits = append(op.Hits, hit)
					}
				}
			}
		}
		if snapshot.State == "completed" || snapshot.State == "failed" || snapshot.State == "cancelled" || snapshot.State == "intercepted" || snapshot.State == "partial_intercepted" {
			op.FinishedAt = time.Now().UTC()
		}
		op.Timeline = append(op.Timeline, store.MCPEvent{Stage: op.Status, Detail: op.Error, At: time.Now().UTC()})
		return a.store.MCPFinish(a.ctx, op)
	}, func(err error) {
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToSaveMcpTask, err)
			a.mcpBackend.service.FailStorage(err)
		}
		a.refreshMCPBackend()
	})
}

func setMCPTransferOutcome(task *transferTask, result error) {
	if task.MCP == nil {
		return
	}
	if task.State == transferQueued || task.State == transferRunning {
		task.MCP.Outcome = ""
	}
	var failure *mcpserver.Failure
	if task.Run == nil && errors.As(result, &failure) && failure.Code == "intercepted" {
		task.MCP.Outcome = "intercepted"
		if task.Job.DoneBytes > 0 {
			task.MCP.Outcome = "partial_intercepted"
		}
	}
}
