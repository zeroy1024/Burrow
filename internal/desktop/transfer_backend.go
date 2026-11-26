package desktop

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/mcpserver"
	"github.com/zeroy1024/Burrow/internal/profile"
	"github.com/zeroy1024/Burrow/internal/transfer"
)

type transferRun = transfer.Run
type transferJob = transfer.Job
type transferBatch = transfer.Batch
type transferNode = transfer.Node
type transferLocks = transfer.Locks
type transferClientResult = transfer.ClientResult
type mcpTransferOwner = transfer.MCPOwner

type transferConflictChoice struct {
	choice   int
	applyAll bool
}

func transferFraction(task *transferTask) float32 {
	if task.State == transferDone {
		return 1
	}
	if task.Total == 0 {
		return 0
	}
	return float32(min(max(float64(task.Transferred)/float64(task.Total), 0), 1))
}

func (a *app) scheduleTransfers() {
	if !a.businessReady() {
		return
	}
	if a.ctx.Err() != nil {
		return
	}
	a.transfers.Schedule(a.prefs.TransferParallel, a.store != nil, func(s *fileSession) bool {
		return s.endpoint.protocol == "ftp" && s.endpoint.session.Auth != "anonymous" && s.ftpSecret == ""
	}, a.startTransfer)
}

func (a *app) startTransfer(task *transferTask, cleanup bool) {
	base := a.ctx
	if task.MCP != nil {
		base = mcpserver.WithOperation(base, task.MCP.OperationID, task.ID)
	}
	run := task.Begin(base, cleanup)
	ctx := run.Context
	job := task.Job
	run.Journal = func() error { return a.journalTransfer(task.ID, run, job) }
	a.persistTransfers()
	if !cleanup {
		a.mcpTransferChanged(task, nil)
	}
	if job.DestinationKey == "" {
		job.DestinationKey = "local"
		if !job.SourceRemote {
			h := task.Session.endpoint
			job.DestinationKey = fmt.Sprintf("%q/%q/%q/%d", h.protocol, h.user, h.address, h.port)
		}
	}
	a.bindTransferEvents(task, run)
	var wrapSource func(fileclient.Client) fileclient.Client
	if task.MCP != nil && !cleanup {
		wrapSource = func(c fileclient.Client) fileclient.Client { return a.mcpBackend.sourceClient(task, c) }
	}
	locks := &a.transfers.Locks
	// Start the worker before requesting a client: context cancellation must
	// release it even if an application-exit UI callback is never delivered.
	a.sshWorkers.Go(func() {
		var err error
		if task.MCP != nil && !cleanup {
			err = a.mcpBackend.preflightTransfer(ctx, task)
			if err == nil {
				a.window.Update(func() {
					if ctx.Err() != nil || task.Run != run {
						return
					}
					if !a.mcpBackend.service.Accepting() {
						run.AwaitingClient = false
						run.Ready <- transferClientResult{Err: locales.NewError(locales.MessageMcpServiceStopped)}
						return
					}
					a.requestTransferClient(task, run)
				})
			}
		}
		if err == nil {
			err = run.Work(job, locks, cleanup, wrapSource)
		}
		if journalErr := run.Journal(); journalErr != nil {
			err = errors.Join(err, locales.Errorf(locales.MessageSaveTransferCheckpoint, journalErr))
		}
		if a.ctx.Err() != nil {
			return
		}
		a.window.Update(func() { a.finishTransfer(task, run, cleanup, err) })
	})
	if cleanup && job.SourceRemote {
		run.AwaitingClient = false
		return
	}
	if task.MCP != nil && !cleanup {
		return
	}
	a.requestTransferClient(task, run)
}

func (a *app) requestTransferClient(task *transferTask, run *transferRun) {
	ctx := run.Context
	if task.Session.endpoint.protocol == "ftp" && task.Session.ftpSecret == "" && sameTransferView(task.View, task.Session) {
		task.Session.ftpSecret = task.View.ftpSecret
	}
	run.Request = a.openFileClient(ctx, task.Session, func(client fileclient.Client, err error) {
		if ctx.Err() != nil {
			if client != nil {
				client.Close()
			}
			return
		}
		run.AwaitingClient = false
		run.Ready <- transferClientResult{Client: client, Err: err}
		a.scheduleTransfers()
	})
}

// A browser's explicit reconnect may adopt edited saved settings. Jobs retain
// this private snapshot and the original shared transport group independently.
func (a *app) snapshotTransferSession(view *fileSession) *fileSession {
	snapshot := a.newFileSession(view.host, view.endpoint, view.temporary, view.connection)
	snapshot.restoreID = view.restoreID
	snapshot.gatewayScope = view.gatewayScope
	snapshot.ftpSecret = view.ftpSecret
	snapshot.local.path, snapshot.remote.path = view.local.path, view.remote.path
	snapshot.local.history, snapshot.remote.history = slices.Clone(view.local.history), slices.Clone(view.remote.history)
	snapshot.local.historyIndex, snapshot.remote.historyIndex = view.local.historyIndex, view.remote.historyIndex
	return snapshot
}

func sameTransferView(view, snapshot *fileSession) bool {
	if view == nil {
		return false
	}
	one, two := view.endpoint, snapshot.endpoint
	if one.address != two.address || one.port != two.port || one.user != two.user || one.protocol != two.protocol ||
		one.session.ID != two.session.ID || one.session.Auth != two.session.Auth || one.session.KeyID != two.session.KeyID ||
		one.session.PasswordSaved != two.session.PasswordSaved || one.session.PasswordRevision != two.session.PasswordRevision || !one.session.UpdatedAt.Equal(two.session.UpdatedAt) {
		return false
	}
	if (one.session.Files == nil) != (two.session.Files == nil) ||
		one.session.Files != nil && *one.session.Files != *two.session.Files {
		return false
	}
	if !profile.SameRoute(one.session.GatewayRoute, two.session.GatewayRoute) {
		return false
	}
	return one.protocol == "ftp" || view.connection == snapshot.connection
}

func (a *app) finishTransfer(task *transferTask, run *transferRun, cleanup bool, err error) {
	if task.Run != run {
		return
	}
	if a.transfers.Finish(task, run, cleanup, err) {
		a.completeFileTransfer(task)
	}
	a.transfers.displaySpeed = a.transfers.summary().Speed
	a.advanceTransferConflicts()
	setMCPTransferOutcome(task, err)
	a.persistTransfers()
	a.scheduleTransfers()
	a.finishTransferQuitIfReady()
	a.mcpTransferChanged(task, err)
	if task.MCP != nil && task.State == transferDone {
		a.transfers.Tasks = slices.DeleteFunc(a.transfers.Tasks, func(t *transferTask) bool { return t == task })
		a.persistTransfers()
	}
	a.advanceSiteDeletion()
}

func (a *app) pauseTransfer(task *transferTask) {
	if !task.Pause() {
		return
	}
	a.transfers.displaySpeed = a.transfers.summary().Speed
	a.advanceTransferConflicts()
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
}

func (a *app) resumeTransfer(task *transferTask) {
	if a.transfers.Quitting || a.siteDeletionPending(task.Session.endpoint.session.ID) {
		return
	}
	if task.CleanupPending {
		a.cancelTransfer(task)
		return
	}
	if !task.Resume() {
		return
	}
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
	a.scheduleTransfers()
}

func (a *app) cancelTransfer(task *transferTask) {
	if !a.transfers.Cancel(task) {
		return
	}
	a.advanceTransferConflicts()
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
}

func (a *app) bindTransferEvents(task *transferTask, run *transferRun) {
	run.DestinationChanged = func(remote bool, destination string) { a.publishTransferDestination(task, run, remote, destination) }
	run.ResolveConflict = func(ctx context.Context, dir string, entry fileclient.Entry) (int, error) {
		choice, err := a.askTransferConflict(ctx, task, dir, fileEntry{name: entry.Name, size: entry.Size, dir: entry.Directory})
		return choice.choice, err
	}
	run.ProgressChanged = func(bytes, total int64, speed float64) {
		a.window.Update(func() {
			if task.Run != run || run.Context.Err() != nil {
				return
			}
			task.Transferred, task.Total, task.Speed = bytes, total, speed
			a.transfers.displaySpeed = a.transfers.summary().Speed
		})
	}
}

func (a *app) publishTransferDestination(task *transferTask, run *transferRun, sourceRemote bool, destination string) {
	name, directory := transfer.Base(!sourceRemote, destination), transfer.Dir(!sourceRemote, destination)
	a.window.Update(func() {
		if task.Run == run {
			task.Entry.Name, task.TargetPath = name, directory
		}
	})
}

func (a *app) askTransferConflict(ctx context.Context, task *transferTask, target string, entry fileEntry) (transferConflictChoice, error) {
	conflict := &fileConflict{session: task.Session, sourceRemote: task.SourceRemote, target: target,
		entries: []fileEntry{entry}, reply: make(chan transferConflictChoice, 1), ctx: ctx, task: task}
	a.window.Update(func() {
		if ctx.Err() != nil {
			return
		}
		if policy := int(task.Job.Batch.Policy.Load()); policy != 0 {
			conflict.reply <- transferConflictChoice{choice: policy}
			return
		}
		a.transfers.conflicts = append(a.transfers.conflicts, conflict)
		a.advanceTransferConflicts()
	})
	select {
	case answer := <-conflict.reply:
		if answer.choice == 0 {
			return answer, transfer.ErrConflictCanceled
		}
		return answer, nil
	case <-ctx.Done():
		return transferConflictChoice{}, ctx.Err()
	}
}

func (a *app) advanceTransferConflicts() {
	q := &a.transfers
	session := a.activeTransferSession()
	if a.files.conflict != nil && (session == nil || a.files.conflict.task.View != session) {
		a.files.conflict = nil
	}
	q.conflicts = slices.DeleteFunc(q.conflicts, func(conflict *fileConflict) bool {
		if conflict.ctx.Err() != nil {
			if a.files.conflict == conflict {
				a.files.conflict = nil
			}
			return true
		}
		if policy := int(conflict.task.Job.Batch.Policy.Load()); policy != 0 {
			select {
			case conflict.reply <- transferConflictChoice{choice: policy}:
			default:
			}
			if a.files.conflict == conflict {
				a.files.conflict = nil
			}
			return true
		}
		return false
	})
	if session != nil && a.files.dialog == nil && a.files.conflict == nil {
		for _, conflict := range q.conflicts {
			if conflict.task.View == session {
				a.files.conflict = conflict
				break
			}
		}
	}
}

func (a *app) cancelFileConflict() {
	conflict := a.files.conflict
	if conflict == nil {
		return
	}
	batch := conflict.task.Job.Batch
	tasks := slices.Clone(a.transfers.Tasks)
	// Individual removals may schedule other work. Pause this batch's queued
	// rows first so no remaining selection starts during the cancellation loop.
	for _, task := range tasks {
		if task.Job.Batch == batch && task.State == transferQueued {
			task.State = transferPaused
		}
	}
	select {
	case conflict.reply <- transferConflictChoice{}:
	default:
	}
	a.transfers.conflicts = slices.DeleteFunc(a.transfers.conflicts, func(candidate *fileConflict) bool {
		return candidate.task.Job.Batch == batch
	})
	a.files.conflict = nil
	for _, task := range tasks {
		if task.Job.Batch == batch && task.State != transferDone {
			a.cancelTransfer(task)
		}
	}
	a.advanceTransferConflicts()
}
