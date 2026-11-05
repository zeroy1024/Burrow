package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/egoist/mygo"
	"sshdesk/internal/store"
	"sshdesk/internal/uikit"
)

type persistenceRequest struct {
	work func() error
	done func(error)
}
type persistenceWorker struct {
	requests chan persistenceRequest
	stopped  chan struct{}
}

func newPersistenceWorker(window func() *mygo.Window) *persistenceWorker {
	p := &persistenceWorker{requests: make(chan persistenceRequest, 64), stopped: make(chan struct{})}
	go func() {
		defer close(p.stopped)
		for r := range p.requests {
			err := r.work()
			if r.done != nil {
				window().Update(func() { r.done(err) })
			}
		}
	}()
	return p
}
func (p *persistenceWorker) close() { close(p.requests); <-p.stopped }

func (a *app) persist(work func() error, done func(error)) {
	a.persistence.requests <- persistenceRequest{work: work, done: func(err error) {
		if done != nil {
			done(err)
		}
	}}
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

func (a *app) persistenceNotice(title string, err error) {
	if err == nil {
		return
	}
	a.files.notice = &uikit.ToastProps{ID: "database-write-error", Tone: uikit.ToneDanger, Title: title, Detail: err.Error()}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *app) persistTransfers() {
	a.persistTransfersDone(nil)
}

func (a *app) persistTransfersDone(done func(error)) {
	if a.store == nil || a.transfers.quitting || a.transfers.storeErr != nil {
		if done != nil {
			done(errors.New("传输存储不可用"))
		}
		return
	}
	saved := a.transferSnapshot()
	if err := validateTransferQueue(saved); err != nil {
		a.persistenceNotice("保存传输记录失败", err)
		if done != nil {
			done(err)
		}
		return
	}
	q, err := encodeTransfers(saved)
	if err != nil {
		a.persistenceNotice("保存传输记录失败", err)
		if done != nil {
			done(err)
		}
		return
	}
	a.transfers.persistenceRevision++
	revision := a.transfers.persistenceRevision
	active := map[int]bool{}
	for _, task := range a.transfers.tasks {
		active[task.id] = task.run != nil
		if task.run == nil {
			task.persisted = false
		}
	}
	for i := range q.Tasks {
		q.Tasks[i].PreserveManifest = active[q.Tasks[i].ID]
	}
	owners := []store.MCPTask{}
	for _, task := range a.transfers.tasks {
		if task.mcp != nil {
			owners = append(owners, a.mcpTaskSnapshot(task, nil))
		}
	}
	a.persist(func() error { return a.store.SaveTransfersWithMCP(a.ctx, q, owners) }, func(err error) {
		if done != nil {
			defer done(err)
		}
		if err != nil {
			a.transfers.writeErr = err
			a.persistenceNotice("保存传输记录失败", err)
			return
		}
		a.transfers.writeErr = nil
		if revision != a.transfers.persistenceRevision {
			return
		}
		for _, task := range a.transfers.tasks {
			for _, saved := range q.Tasks {
				if task.id == saved.ID {
					task.persisted = true
					break
				}
			}
		}
		a.scheduleTransfers()
	})
}

type transferManifestSnapshot struct {
	Root                   *storedTransferNode
	Temporary              []string
	Total, Done            int64
	SourcePath, TargetPath string
}

func transferManifest(job *transferJob) *transferManifestSnapshot {
	m := &transferManifestSnapshot{Root: snapshotTransferNode(job.root), Total: job.total, Done: job.doneBytes, Temporary: []string{}, SourcePath: job.sourcePath, TargetPath: job.targetPath}
	for name := range job.temporary {
		m.Temporary = append(m.Temporary, name)
	}
	return m
}

func (a *app) journalTransfer(taskID int, run *transferRun, job *transferJob) error {
	m := transferManifest(job)
	run.manifest.Store(m)
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
	name := job.name
	if m.Root != nil && m.Root.Destination != "" {
		name = transferBase(!job.sourceRemote, m.Root.Destination)
	}
	if err := a.store.SaveManifest(ctx, taskID, store.Manifest{Root: root, Temporary: temporary, Total: m.Total, Done: m.Done, SourcePath: m.SourcePath, TargetPath: m.TargetPath, EntryName: name}); err != nil {
		return err
	}
	if run.mcp != nil {
		return a.store.MCPProgress(ctx, taskID, max(run.bytes.Load(), 0), max(run.total.Load(), 0))
	}
	return nil
}

func (a *app) finishPersistence() error {
	if a.persistence != nil {
		a.persistence.close()
	}
	return a.store.Close()
}
