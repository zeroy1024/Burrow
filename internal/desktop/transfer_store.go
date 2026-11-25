package desktop

import (
	"errors"
	"slices"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/persistence"
	"github.com/zeroy1024/Burrow/internal/transfer"
)

type storedTransferQueue = transfer.Snapshot
type storedTransferTask = transfer.SavedTask
type storedTransferBatch = transfer.SavedBatch
type storedTransferSession = transfer.SavedSession
type storedTransferConnection = transfer.SavedConnection

// prepareTransferQuit leaves the application context alive until every transfer
// worker has relinquished its manifest. Saving then requires no authentication
// or filesystem/remote cleanup, and a failed write leaves usable paused rows.
func (a *app) prepareTransferQuit(done func(error)) {
	q := &a.transfers
	if q.Quitting {
		if !q.storing && len(q.quitDone) == 0 {
			done(nil)
			return
		}
		q.quitDone = append(q.quitDone, done)
		return
	}
	q.quitDone = append(q.quitDone, done)
	q.Quitting = true
	for _, task := range q.Tasks {
		if task.State == transferRunning || task.State == transferQueued {
			task.State, task.Speed = transferPaused, 0
		}
		if task.Run != nil {
			task.Run.Cancel()
			task.Run.Request.Cancel()
		}
	}
	q.displaySpeed = 0
	a.advanceTransferConflicts()
	a.finishTransferQuitIfReady()
}

func (a *app) finishTransferQuitIfReady() {
	q := &a.transfers
	if !q.Quitting || q.storing {
		return
	}
	for _, task := range q.Tasks {
		if task.Run != nil {
			return
		}
	}
	finish := func(err error) {
		q.storing = false
		if err != nil {
			q.Quitting = false
		}
		callbacks := q.quitDone
		q.quitDone = nil
		for _, done := range callbacks {
			done(err)
		}
	}
	if q.StoreErr != nil {
		if len(q.Tasks) == 0 {
			// Preserve a journal rejected at load without trapping an otherwise
			// empty application in an exit/save loop.
			finish(nil)
			return
		}
		finish(locales.Errorf(locales.MessageTransferHistoryCouldNotBeReadOverwritingBlocked, q.StoreErr))
		return
	}
	if a.store == nil {
		finish(locales.NewError(locales.MessageTheTransferHistoryDirectoryIsUnavailable))
		return
	}
	snapshot := a.transferSnapshot()
	if err := snapshot.Validate(); err != nil {
		finish(locales.Errorf(locales.MessageInvalidTransferHistoryState, err))
		return
	}
	q.storing = true
	a.persist(func() error {
		saved, err := persistence.EncodeTransfers(snapshot)
		if err != nil {
			return err
		}
		return a.store.SaveTransfers(a.ctx, saved)
	}, finish)
}

func (a *app) transferSnapshot() *storedTransferQueue {
	result := &storedTransferQueue{Version: 4, NextID: a.transfers.NextID, NextBatchID: a.transfers.NextBatchID,
		Sessions: []storedTransferSession{}, Connections: []storedTransferConnection{}, Tasks: []storedTransferTask{}}
	groups := map[*sshConnectionGroup]int{}
	groupID := func(group *sshConnectionGroup) int {
		if group == nil {
			return 0
		}
		if id := groups[group]; id != 0 {
			return id
		}
		id := len(groups) + 1
		groups[group] = id
		stored := storedTransferConnection{ID: id,
			Endpoint: snapshotWorkspaceSession(group.endpoint, group.endpoint.session, true, 0).Endpoint}
		result.Connections = append(result.Connections, stored)
		return id
	}
	sessions := map[*fileSession]int{}
	batches := map[*transferBatch]bool{}
	for _, task := range a.transfers.Tasks {
		// Completed rows belong to a restorable tab, not a global history.
		// Keep unfinished checkpoints even when their browser was closed.
		if task.MCP == nil && task.State == transferDone && (!a.prefs.RestoreSessions || !slices.Contains(a.fileSessions, task.View)) {
			continue
		}
		sessionID := sessions[task.Session]
		if sessionID == 0 {
			sessionID = len(sessions) + 1
			sessions[task.Session] = sessionID
			session := task.Session
			result.Sessions = append(result.Sessions, storedTransferSession{
				ID: sessionID, ViewID: session.restoreID,
				Session:   snapshotWorkspaceSession(session.endpoint, session.config, session.temporary, groupID(session.connection)),
				LocalPath: session.local.path, RemotePath: session.remote.path,
			})
		}
		job := task.Job
		if !batches[job.Batch] {
			batches[job.Batch] = true
			result.Batches = append(result.Batches, storedTransferBatch{ID: job.Batch.ID, Policy: int(job.Batch.Policy.Load())})
		}
		result.Tasks = append(result.Tasks, task.Snapshot(sessionID))
	}
	return result
}

func (a *app) readSavedTransfers() (*storedTransferQueue, error) {
	data, err := a.store.ReadTransfers(a.ctx)
	if err != nil {
		return nil, err
	}
	decoded, err := persistence.DecodeTransfers(data)
	if err != nil {
		return nil, err
	}
	return decoded, decoded.Validate()
}

func (a *app) restoreTransfers() (err error) {
	q := &a.transfers
	defer func() { q.StoreErr = err }()
	saved, err := a.readSavedTransfers()
	if err != nil {
		return err
	}
	groups := map[int]*sshConnectionGroup{}
	for _, stored := range saved.Connections {
		endpoint := transferStoredHost(stored.Endpoint)
		group := a.newSSHGroup(endpoint)
		if len(stored.Steps) > 0 {
			group.plan, group.planErr = sshDialPlan{trust: a.store, route: group.plan.route}, nil
			for _, value := range stored.Steps {
				step := sshDialStep{endpoint: transferStoredHost(value)}
				if step.endpoint.session.Auth == "key" {
					var keyErr error
					step.key, keyErr = a.keyByID(step.endpoint.session.KeyID)
					group.planErr = errors.Join(group.planErr, keyErr)
				}
				group.plan.steps = append(group.plan.steps, step)
			}
		}
		groups[stored.ID] = group
	}
	views := map[string]*fileSession{}
	if a.prefs.RestoreSessions {
		for _, session := range a.fileSessions {
			views[session.restoreID] = session
		}
	}
	sessions := map[int]*fileSession{}
	owners := map[int]*fileSession{}
	for _, stored := range saved.Sessions {
		endpoint := transferStoredHost(stored.Session.Endpoint)
		profile := hostID(slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == stored.Session.ProfileID }))
		session := a.newFileSession(profile, endpoint, stored.Session.Temporary || profile < 0, groups[stored.Session.Connection])
		// Ownership follows the original tab identity, independently of profile edits.
		session.restoreID = stored.ViewID
		if stored.ViewID != "" {
			owners[stored.ID] = views[stored.ViewID]
		}
		session.local.path, session.remote.path = stored.LocalPath, stored.RemotePath
		if stored.LocalPath != "" {
			session.local.history, session.local.historyIndex = []string{stored.LocalPath}, 0
		}
		if stored.RemotePath != "" {
			session.remote.history, session.remote.historyIndex = []string{stored.RemotePath}, 0
		}
		sessions[stored.ID] = session
	}
	batches := map[int]*transferBatch{}
	for _, stored := range saved.Batches {
		batch := &transferBatch{ID: stored.ID}
		batch.Policy.Store(int32(stored.Policy))
		batches[stored.ID] = batch
	}
	for _, stored := range saved.Tasks {
		task := transfer.RestoreTask(stored, batches[stored.Batch], sessions[stored.Session], owners[stored.Session])
		q.Tasks = append(q.Tasks, task)
		if task.MCP != nil && a.mcpBackend != nil {
			task.Session.gatewayScope = "mcp:" + task.Session.restoreID
			a.mcpBackend.observeGroup(task.Session.connection)
			owner, ownerErr := a.store.MCPTask(a.ctx, task.ID)
			if ownerErr != nil {
				return ownerErr
			}
			if owner.KeyID != task.MCP.KeyID || owner.OperationID != task.MCP.OperationID || owner.ConnectionID != task.MCP.ConnectionID {
				return locales.NewError(locales.MessageMcpTransferOwnershipDoesNotMatchTheQueue)
			}
			if task.State == transferFailed && (owner.State == "intercepted" || owner.State == "partial_intercepted") {
				task.MCP.Outcome = owner.State
			}
			a.mcpBackend.attachTransfer(task)
		}
	}
	q.NextID, q.NextBatchID = saved.NextID, saved.NextBatchID
	return nil
}

func transferStoredHost(value storedSession) host {
	return cloneHostEndpoint(host{name: value.Name, address: value.Host, port: value.Port, user: value.User,
		protocol: value.Protocol, os: value.OS, system: value.System, session: value.Config})
}
