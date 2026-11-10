package desktop

import (
	"slices"

	"github.com/zeroy1024/Burrow/internal/locales"
)

// Owned by the window thread. The queue must finish cancellation and persist
// its removal before the catalog transaction can retire the profile's secret.
type siteDeletion struct {
	profileID  string
	committing bool
	done       func(error)
}

func (a *app) siteTransfers(profileID string) []*transferTask {
	tasks := []*transferTask{}
	for _, task := range a.transfers.tasks {
		if task.session.endpoint.session.ID == profileID {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func transferNeedsCancellation(task *transferTask) bool {
	return task.state != transferDone || task.run != nil || task.cleanupPending || task.removeRequested
}

func (a *app) siteDeletionPending(profileID string) bool {
	return a.siteDeletion != nil && a.siteDeletion.profileID == profileID
}

func (a *app) beginSiteDeletion(profileID string, done func(error)) {
	if a.siteDeletion != nil || a.catalogSaving || a.transfers.quitting {
		done(locales.NewError(locales.MessageProcessing2))
		return
	}
	id := slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == profileID })
	if id < 0 {
		done(locales.NewError(locales.SessionUnavailableForTransfer))
		return
	}
	if err := a.fileSiteDeleteError(hostID(id)); err != nil {
		done(err)
		return
	}
	if err := a.transfers.storeErr; err != nil {
		done(err)
		return
	}
	pending := &siteDeletion{profileID: profileID, committing: true, done: done}
	a.siteDeletion = pending
	for _, task := range a.siteTransfers(profileID) {
		if transferNeedsCancellation(task) {
			a.cancelTransfer(task)
		} else {
			// Completed history is retired without changing its MCP audit outcome.
			a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(t *transferTask) bool { return t == task })
		}
	}
	// Persist even if cancellation was already requested, so retrying after a
	// queue-write failure can clear that error and restart cleanup scheduling.
	a.persistTransfersDone(func(err error) {
		if err != nil {
			a.finishSiteDeletion(err)
			return
		}
		pending.committing = false
		a.advanceSiteDeletion()
	})
}

func (a *app) finishSiteDeletion(err error) {
	pending := a.siteDeletion
	a.siteDeletion = nil
	pending.done(err)
}

func (a *app) advanceSiteDeletion() {
	pending := a.siteDeletion
	if pending == nil || pending.committing {
		return
	}
	if err := a.transfers.storeErr; err != nil {
		a.finishSiteDeletion(err)
		return
	}
	if err := a.transfers.writeErr; err != nil {
		a.finishSiteDeletion(err)
		return
	}
	tasks := a.siteTransfers(pending.profileID)
	for _, task := range tasks {
		if task.run == nil && !task.removeRequested {
			a.finishSiteDeletion(locales.NewError(locales.SessionTransferCleanupBlocked))
			return
		}
	}
	if len(tasks) != 0 {
		return
	}
	pending.committing = true
	a.persistTransfersDone(func(err error) {
		if err != nil {
			a.finishSiteDeletion(err)
			return
		}
		id := slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == pending.profileID })
		if id < 0 {
			a.finishSiteDeletion(locales.NewError(locales.SessionUnavailableForTransfer))
			return
		}
		a.deleteFileSite(hostID(id), a.finishSiteDeletion)
	})
}
