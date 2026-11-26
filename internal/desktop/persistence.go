package desktop

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/persistence"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/transfer"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

func (a *app) persist(work func() error, done func(error)) {
	a.persistence.Submit(work, func(err error) {
		if done != nil {
			done(err)
		}
	})
}

func (a *app) saveCatalogAsync(done func(error)) { a.saveCatalogSecretsAsync(nil, done) }

func (a *app) saveCatalogSecretsAsync(changes *store.VaultChanges, done func(error)) {
	c, err := a.catalogSnapshotData()
	if err != nil {
		done(err)
		return
	}
	c.VaultChanges = changes
	a.catalogSaving = true
	a.persist(func() error { return a.store.SaveCatalog(a.ctx, c) }, func(err error) {
		a.catalogSaving = false
		done(err)
		ready := a.catalogReady
		a.catalogReady = nil
		for _, next := range ready {
			next()
		}
	})
}

func (a *app) commitCatalogMoveAsync(change func(), done func(error)) {
	a.commitCatalogSecretsAsync(nil, change, done)
}

func (a *app) commitCatalogSecretsAsync(changes *store.VaultChanges, change func(), done func(error)) {
	complete := a.beginCatalogMove(change)
	a.saveCatalogSecretsAsync(changes, func(err error) {
		complete(err)
		done(err)
	})
}

func (a *app) persistenceNotice(title func() string, err error) {
	if err == nil {
		return
	}
	a.files.notice = &uikit.ToastProps{ID: "database-write-error", Tone: uikit.ToneDanger, Title: locales.Deferred(
		title,
	), Detail: locales.ErrorText(
		err,
	)}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *app) persistTransfers() {
	a.persistTransfersDone(nil)
}

func (a *app) persistTransfersDone(done func(error)) {
	if a.store == nil || a.transfers.Quitting || a.transfers.StoreErr != nil {
		if done != nil {
			done(locales.NewError(locales.MessageTransferStorageIsUnavailable))
		}
		return
	}
	saved := a.transferSnapshot()
	if err := saved.Validate(); err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveTransferHistory, err)
		if done != nil {
			done(err)
		}
		return
	}
	q, err := persistence.EncodeTransfers(saved)
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveTransferHistory, err)
		if done != nil {
			done(err)
		}
		return
	}
	a.transfers.persistenceRevision++
	revision := a.transfers.persistenceRevision
	active := map[int]bool{}
	for _, task := range a.transfers.Tasks {
		active[task.ID] = task.Run != nil
		if task.Run == nil {
			task.Persisted = false
		}
	}
	for i := range q.Tasks {
		q.Tasks[i].PreserveManifest = active[q.Tasks[i].ID]
	}
	owners := []store.MCPTask{}
	for _, task := range a.transfers.Tasks {
		if task.MCP != nil {
			owners = append(owners, a.mcpTaskSnapshot(task, nil))
		}
	}
	a.persist(func() error { return a.store.SaveTransfersWithMCP(a.ctx, q, owners) }, func(err error) {
		defer a.advanceSiteDeletion()
		if done != nil {
			defer done(err)
		}
		if err != nil {
			a.transfers.WriteErr = err
			a.persistenceNotice(locales.MessageUnableToSaveTransferHistory, err)
			return
		}
		a.transfers.WriteErr = nil
		if revision != a.transfers.persistenceRevision {
			return
		}
		for _, task := range a.transfers.Tasks {
			for _, saved := range q.Tasks {
				if task.ID == saved.ID {
					task.Persisted = true
					break
				}
			}
		}
		a.scheduleTransfers()
	})
}

func (a *app) journalTransfer(taskID int, run *transferRun, job *transferJob) error {
	m := transfer.SnapshotManifest(job)
	run.Manifest.Store(m)
	if a.store == nil {
		return nil
	}
	root, err := json.Marshal(m.Root)
	if err != nil {
		return err
	}
	temporary, err := json.Marshal(m.Temporary)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	name := job.Name
	if m.Root != nil && m.Root.Destination != "" {
		name = transfer.Base(!job.SourceRemote, m.Root.Destination)
	}
	if err := a.store.SaveManifest(ctx, taskID, store.Manifest{Root: root, Temporary: temporary, Total: m.Total, Done: m.Done, SourcePath: m.SourcePath, TargetPath: m.TargetPath, EntryName: name}); err != nil {
		return err
	}
	if run.MCP != nil {
		return a.store.MCPProgress(ctx, taskID, max(run.Bytes.Load(), 0), max(run.Total.Load(), 0))
	}
	return nil
}

func (a *app) finishPersistence() error {
	if a.persistence != nil {
		a.persistence.Close()
	}
	return a.store.Close()
}
