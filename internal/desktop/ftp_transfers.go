package desktop

import (
	"fmt"
	"path"
	"slices"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/transfer"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type transferTask = transfer.Task[fileSession]
type transferState = transfer.State
type transferSummary = transfer.Summary

const (
	transferQueued  = transfer.Queued
	transferRunning = transfer.Running
	transferPaused  = transfer.Paused
	transferDone    = transfer.Done
	transferFailed  = transfer.Failed
)

type transferQueueState struct {
	transfer.Queue[fileSession]
	persistenceRevision uint64

	collapsed    bool
	filter       int
	displaySpeed float64
	dockScroll   ui.ScrollState
	dockHeight   float32
	dockResizing bool
	conflicts    []*fileConflict

	storing  bool
	quitDone []func(error)
}

var (
	transferUpIcon    = uikit.IconSVG(uikit.IconUpload)
	transferPauseIcon = uikit.IconSVG(uikit.IconPause)
	transferPlayIcon  = uikit.IconSVG(uikit.IconPlay)
	transferTrashIcon = uikit.IconSVG(uikit.IconTrash)
)

func (q *transferQueueState) summary() transferSummary {
	return transfer.Summarize(q.Tasks)
}

func (a *app) activeTransferSession() *fileSession {
	if a.activeSection() != sectionFiles {
		return nil
	}
	return a.currentFileSession()
}

func (q *transferQueueState) sessionTasks(session *fileSession) []*transferTask {
	tasks := []*transferTask{}
	if session == nil {
		return tasks
	}
	for _, task := range q.Tasks {
		if task.View == session {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (a *app) detachedTransferCount() int {
	count := 0
	for _, task := range a.transfers.Tasks {
		if !slices.Contains(a.fileSessions, task.View) {
			count++
		}
	}
	return count
}

func (a *app) recoverTransferTabs(c *ui.Context, tasks []*transferTask) {
	recovered := map[string]*fileSession{}
	var first *fileSession
	for _, task := range tasks {
		if slices.Contains(a.fileSessions, task.View) {
			continue
		}
		snapshot := task.Session
		view := task.View
		if view == nil {
			view = recovered[snapshot.restoreID]
			if view == nil {
				view = a.snapshotTransferSession(snapshot)
				if view.restoreID == "" {
					view.restoreID = newSessionID()
				}
			}
		}
		if !slices.Contains(a.fileSessions, view) {
			a.fileSessions = append(a.fileSessions, view)
		}
		// Keep every private endpoint snapshot, while persisting the recovered
		// tab identity for all batches that belonged to the same original tab.
		recovered[view.restoreID] = view
		task.View, snapshot.restoreID = view, view.restoreID
		if first == nil {
			first = view
		}
	}
	if first == nil {
		if len(tasks) == 0 {
			return
		}
		first = tasks[0].View
	}
	a.closedTabs = slices.DeleteFunc(a.closedTabs, func(closed closedWorkspaceTab) bool {
		return closed.section == sectionFiles && slices.Contains(a.fileSessions, closed.file)
	})
	a.transfers.filter, a.transfers.collapsed = 0, false
	a.switchSection(sectionFiles)
	a.activateFileTab(slices.Index(a.fileSessions, first))
	a.advanceTransferConflicts()
	a.saveFilePreferences(c)
}

func (a *app) enqueueFiles(
	c *ui.Context,
	session *fileSession,
	sourceRemote bool,
	entries []fileEntry,
	targetPath string,
) {
	if a.siteDeletionPending(session.endpoint.session.ID) {
		uikit.Notify(c, "site-deleting", locales.SessionDeleting)
		return
	}
	var snapshot *fileSession
	var batch *transferBatch
	for _, entry := range entries {
		if !transfer.SafeName(entry.name) {
			uikit.Toast(c, uikit.ToastProps{ID: "transfer-invalid-name", Tone: uikit.ToneDanger,
				Title: locales.Deferred(
					locales.MessageTheFilenameCannotBeSafelyUsedInTheDestinationPath,
				), Detail: locales.Raw(
					entry.name,
				)})
			continue
		}
		if snapshot == nil {
			snapshot = a.snapshotTransferSession(session)
			batch = a.transfers.NewBatch(transfer.ParsePolicy(a.prefs.ConflictPolicy))
		}
		sourcePath := entry.path
		if sourcePath == "" {
			pane := session.local
			if sourceRemote {
				pane = session.remote
			}
			sourcePath = transfer.Join(sourceRemote, pane.path, entry.name)
		}
		task := &transferTask{
			Session: snapshot, View: session, SourceRemote: sourceRemote, SourcePath: sourcePath, TargetPath: targetPath,
			Entry: fileclient.Entry{
				Name: entry.name, Size: entry.size, Directory: entry.dir, Symlink: entry.symlink,
				Modified: entry.modified, Owner: entry.owner, Permissions: entry.permissions,
			},
		}
		a.transfers.Enqueue(task, batch)
	}
	a.persistTransfers()
	a.scheduleTransfers()
}

func (a *app) tickTransfers(c *ui.Context) {
	a.advanceTransferConflicts()
	session := a.activeTransferSession()
	for _, task := range a.transfers.Tasks {
		if task.FailureNotice && session != nil && task.View == session {
			task.FailureNotice = false
			a.transferFailureNotice(c, task)
		}
	}
	a.scheduleTransfers()
}

func transferBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, suffix := float64(bytes), "B"
	for _, label := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		suffix = label
		if value < unit {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

func transferRemaining(bytes int64, speed float64) string {
	if speed == 0 {
		return locales.MessageRemaining()
	}
	seconds := float64(bytes) / speed
	if seconds < 1 {
		return locales.MessageAlmostDone()
	}
	n := int(seconds)
	return fmt.Sprintf(locales.MessageAboutRemaining(), n/60, n%60)
}

func (a *app) transferDock(c *ui.Context, maxHeight float32) {
	t, q := theme.Current(), &a.transfers
	p := t.C.TransferDock
	session := a.activeTransferSession()
	tasks := q.sessionTasks(session)
	dock := uikit.TransferDock(c).Key(session)
	rows := a.filteredTransfers(tasks)
	scroll := &q.dockScroll
	if session != nil {
		scroll = &session.transferScroll
	}
	rowsTarget := float32(1)
	if q.collapsed {
		rowsTarget = 0
	}
	rowsShown := dock.AnimateTo("rows", rowsTarget, a.positionAnimation(ui.Animation{Spring: uikit.Spring(p.Collapse)}))
	minHeight := p.HeaderHeight + p.RowHeight
	maxHeight = max(minHeight, maxHeight)
	height := p.HeaderHeight + min(float32(max(len(rows), 1)), p.MaxRows)*p.RowHeight
	if q.dockHeight > 0 {
		height = q.dockHeight
	}
	height = min(max(height, minHeight), maxHeight)
	setHeight := func() {
		dock.Height(p.HeaderHeight + (height-p.HeaderHeight)*max(0, rowsShown))
	}
	setHeight()
	dock.Margin(t.C.FTPSplit.Gutter, 0, 0, 0).Children(func() {
		card := uikit.TransferDockCard(c)
		card.Children(func() {
			a.transferHeader(c, session, tasks)
			if !q.collapsed {
				// Only the inert exit copy remains while the dock closes.
				list := ui.Local(dock, "transfer-list-state", func() ui.ListState { return ui.ListState{KeepOffset: true} })
				scroller := ui.List(c.Key("transfer-rows"), list, max(1, len(rows))).Grow(1).MinHeight(0).TrackScroll(scroll).
					Transition(ui.ElementTransition{Duration: t.Motion.Duration.Focus, Exit: &ui.Motion{}}).
					ItemKey(func(index int) any {
						if len(rows) == 0 {
							return "empty"
						}
						return rows[index].ID
					}).Rows(func(row ui.ListRow) {
					if len(rows) != 0 {
						a.transferRow(row.Context, rows[row.Index])
						return
					}
					message := locales.MessageNoTransfersInThisCategory()
					if len(tasks) == 0 {
						message = locales.MessageNoTransfersYet()
					}
					uikit.TransferDockEmpty(row.Context, message)
				})
				uikit.Scrollbars(c, card, scroller, scroll, uikit.ScrollbarProps{Label: locales.MessageTransferListScrollbar()})
			}
		})
		if q.collapsed || !dock.Settled("rows") {
			return
		}
		// Keep the gap outside the clipped card so it remains a full-height hit target.
		divider := uikit.TransferDockDivider(
			c,
			locales.MessageResizeTransferPanel(),
			locales.MessageDragToResizeDoubleClickToReset(),
		)
		_, dy, dragging := divider.Dragged()
		if dragging && dy != 0 {
			height = min(max(height-dy, minHeight), maxHeight)
			q.dockHeight, q.dockResizing = height, true
			setHeight()
		}
		if q.dockResizing && !dragging {
			q.dockResizing = false
			a.saveFilePreferences(c)
		}
		if divider.DoubleClicked() {
			q.dockHeight = 0
			a.saveFilePreferences(c)
		}
		step := float32(0)
		if divider.Shortcut(0, ui.KeyUp) {
			step = t.C.Desktop.SiderKeyboardStep
		}
		if divider.Shortcut(0, ui.KeyDown) {
			step = -t.C.Desktop.SiderKeyboardStep
		}
		if step != 0 {
			height = min(max(height+step, minHeight), maxHeight)
			q.dockHeight = height
			setHeight()
			a.saveFilePreferences(c)
		}
		divider.Range(float64(minHeight), float64(maxHeight), float64(height))
		if divider.Hovered() || dragging || divider.FocusVisible() {
			uikit.TransferDockGrip(c, divider, dragging)
		}
	})
}

func (a *app) filteredTransfers(tasks []*transferTask) []*transferTask {
	q := &a.transfers
	rows := []*transferTask{}
	for _, task := range tasks {
		if q.filter == 1 && (task.State == transferDone || task.State == transferFailed) {
			continue
		}
		if q.filter == 2 && task.State != transferDone {
			continue
		}
		rows = append(rows, task)
	}
	return rows
}

func (a *app) transferHeader(c *ui.Context, session *fileSession, tasks []*transferTask) {
	t, q := theme.Current(), &a.transfers
	s := transfer.Summarize(tasks)
	header := uikit.TransferHeader(c)
	compact := header.Bounds().W < t.C.TransferDock.CompactBreakpoint
	header.Children(func() {
		uikit.TransferHeaderTitle(c, locales.MessageTransfers(), filesIcon)
		a.transferFilters(c, s, len(tasks))
		ui.Spacer(c)
		if count := a.detachedTransferCount(); count > 0 {
			button := uikit.Button(c, locales.TransferRecoverCount(count), uikit.ButtonProps{
				Variant: uikit.ButtonPlain, Size: uikit.SizeSm,
			}).Key("recover-transfers").Shrink(0)
			uikit.Tip(c, button, locales.MessageReopenTheTransferSTabWithoutConnectingOrResumingTheTransfer())
			button.OnClick(func() { a.recoverTransferTabs(c, q.Tasks) })
		} else if !compact {
			uikit.TransferHeaderSummary(
				c,
				locales.MessageTotalSpeed()+transferBytes(int64(s.Speed))+"/s · "+transferRemaining(s.Total-s.Bytes, s.Speed),
			)
		}
		pauseIcon, pauseLabel := transferPauseIcon, locales.MessagePauseAll()
		if s.Running+s.Queued == 0 && s.Paused > 0 {
			pauseIcon, pauseLabel = transferPlayIcon, locales.MessageResumeAll()
		}
		uikit.TransferAction(c, "pause-all", pauseLabel, pauseIcon, s.Running+s.Queued+s.Paused == 0).
			OnClick(func() { a.pauseAllTransfers(c, session) })
		uikit.TransferAction(
			c,
			"clear-done",
			locales.MessageClearCompleted(),
			transferTrashIcon,
			s.Done == 0,
		).OnClick(func() {
			q.Tasks = slices.DeleteFunc(q.Tasks, func(task *transferTask) bool {
				return session != nil && task.View == session && task.State == transferDone
			})
		})
		label := locales.MessageCollapseTransferList()
		if q.collapsed {
			label = locales.MessageExpandTransferList()
		}
		button := uikit.TransferAction(c, "toggle-dock", label, chevronIcon, false).Expanded(!q.collapsed)
		rotation := float32(0)
		if q.collapsed {
			rotation = 180
		}
		button.AnimatePose(ui.Pose{Rotate: rotation}, a.positionAnimation(ui.Animation{Spring: uikit.Spring(t.C.TransferDock.Collapse)}))
		button.OnClick(func() {
			q.collapsed = !q.collapsed
			a.saveFilePreferences(c)
		})
	})
}

func (a *app) transferFilters(c *ui.Context, summary transferSummary, total int) {
	q := &a.transfers
	active, done := summary.Running+summary.Queued+summary.Paused, summary.Done
	items := []uikit.SegItem{{Label: locales.MessageAll(), Count: &total}, {Label: locales.MessageInProgress(), Count: &active}, {Label: locales.MessageCompleted(), Count: &done}}
	if uikit.TransferFilters(c, locales.MessageTransferCategory(), &q.filter, items, uikit.MotionOverride(a.reduceMotion(c))).Changed() {
		a.saveFilePreferences(c)
	}
}

func (a *app) transferRow(c *ui.Context, task *transferTask) {
	states := [...]uikit.TransferState{
		transferQueued: uikit.TransferQueued, transferRunning: uikit.TransferRunning,
		transferPaused: uikit.TransferPaused, transferDone: uikit.TransferDone, transferFailed: uikit.TransferFailed,
	}
	view := uikit.Transfer{
		Key: task.ID, Name: task.Entry.Name, SourcePath: task.SourcePath,
		TargetPath: path.Join(task.TargetPath, task.Entry.Name),
		Amount:     transferBytes(task.Transferred) + " / " + transferBytes(task.Total),
		Percent:    fmt.Sprintf("%d%%", int(transferFraction(task)*100)),
		Value:      transferFraction(task),
		State:      states[task.State], Download: task.SourceRemote, Zebra: task.ID%2 == 0,
		Speed: "0 B/s", Remaining: locales.MessageRemaining(), Error: task.ErrorText.String(),
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}
	if task.State == transferRunning {
		view.Speed = transferBytes(int64(task.Speed)) + "/s"
		view.Remaining = transferRemaining(task.Total-task.Transferred, task.Speed)
	}
	if task.State == transferDone {
		view.Elapsed = fmt.Sprintf(locales.MessageCompletedInS(), task.Completed.Sub(task.Started).Seconds())
		view.Completed = localizedDate(task.Completed, i18n.NoStyle, i18n.MediumStyle) + locales.MessageCompleted2()
	}
	result := uikit.TransferRow(c, &view)
	menu := []uikit.MenuEntry{}
	menu = append(menu, uikit.MenuEntry{ID: "reveal", Label: locales.MessageShowInFolder(), Icon: uikit.IconFolder, OnChoose: func() { a.revealTransferredFile(c, task) }})
	uikit.ContextMenuOn(c, result.Root, menu, uikit.MotionOverride(a.reduceMotion(c)))
	if result.Root.DoubleClicked() || result.Reveal {
		a.revealTransferredFile(c, task)
	}
	if result.Pause {
		a.pauseTransfer(task)
	}
	if result.Resume || result.Retry {
		a.resumeTransfer(task)
	}
	if result.Cancel {
		a.cancelTransfer(task)
	}
}

func (a *app) transferFailureNotice(c *ui.Context, task *transferTask) {
	uikit.Toast(c, uikit.ToastProps{
		ID: fmt.Sprintf("transfer-failed-%d", task.ID), Title: locales.Concat(
			locales.Raw(task.Entry.Name),
			locales.Deferred(locales.MessageTransferFailed),
		), Detail: task.ErrorText,
		Tone: uikit.ToneDanger, Action: locales.Deferred(locales.MessageRetry), OnAction: func() {
			if !slices.Contains(a.transfers.Tasks, task) {
				return
			}
			a.resumeTransfer(task)
		},
	})
}

func (a *app) pauseAllTransfers(c *ui.Context, session *fileSession) {
	tasks := a.transfers.sessionTasks(session)
	s := transfer.Summarize(tasks)
	resume := s.Running+s.Queued == 0 && s.Paused > 0
	for _, task := range tasks {
		if resume && task.State == transferPaused {
			a.resumeTransfer(task)
		} else if !resume && (task.State == transferRunning || task.State == transferQueued) {
			a.pauseTransfer(task)
		}
	}
	a.scheduleTransfers()
}
