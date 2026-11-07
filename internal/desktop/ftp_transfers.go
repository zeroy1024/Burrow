package desktop

import (
	"fmt"
	"path"
	"slices"
	"time"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

type transferState int

const (
	transferQueued transferState = iota
	transferRunning
	transferPaused
	transferDone
	transferFailed
)

type transferTask struct {
	mcp       *mcpTransferOwner
	id        int
	persisted bool
	session   *fileSession
	// view is the owning tab; destination reveal never transfers ownership.
	view                   *fileSession
	entry                  fileEntry
	sourceRemote           bool
	sourcePath, targetPath string
	state                  transferState
	transferred, total     int64
	speed                  float64
	started, completed     time.Time
	errorText              locales.Text
	job                    *transferJob
	run                    *transferRun
	removeRequested        bool
	failureNotice          bool
	cleanupPending         bool
}

type transferQueueState struct {
	persistenceRevision uint64
	tasks               []*transferTask
	nextID              int
	nextBatchID         int
	collapsed           bool
	filter              int
	displaySpeed        float64
	dockScroll          ui.ScrollState
	dockHeight          float32
	dockResizing        bool
	conflicts           []*fileConflict
	locks               transferLocks
	storeErr            error
	writeErr            error
	quitting            bool
	storing             bool
	quitDone            []func(error)
}

type transferSummary struct {
	running, queued, paused, done int
	bytes, total                  int64
	speed                         float64
}

var (
	transferUpIcon    = uikit.IconSVG(uikit.IconUpload)
	transferPauseIcon = uikit.IconSVG(uikit.IconPause)
	transferPlayIcon  = uikit.IconSVG(uikit.IconPlay)
	transferTrashIcon = uikit.IconSVG(uikit.IconTrash)
)

func (q *transferQueueState) summary() transferSummary {
	return summarizeTransfers(q.tasks)
}

func summarizeTransfers(tasks []*transferTask) transferSummary {
	s := transferSummary{}
	for _, task := range tasks {
		s.bytes += task.transferred
		s.total += task.total
		switch task.state {
		case transferRunning:
			s.running++
			s.speed += task.speed
		case transferQueued:
			s.queued++
		case transferPaused:
			s.paused++
		case transferDone:
			s.done++
		}
	}
	return s
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
	for _, task := range q.tasks {
		if task.view == session {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (a *app) detachedTransferCount() int {
	count := 0
	for _, task := range a.transfers.tasks {
		if !slices.Contains(a.fileSessions, task.view) {
			count++
		}
	}
	return count
}

func (a *app) recoverTransferTabs(c *ui.Context) {
	recovered := map[string]*fileSession{}
	var first *fileSession
	for _, task := range a.transfers.tasks {
		if slices.Contains(a.fileSessions, task.view) {
			continue
		}
		snapshot := task.session
		view := task.view
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
		task.view, snapshot.restoreID = view, view.restoreID
		if first == nil {
			first = view
		}
	}
	if first == nil {
		return
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
	var snapshot *fileSession
	var batch *transferBatch
	for _, entry := range entries {
		if !transferSafeName(entry.name) {
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
			a.transfers.nextBatchID++
			batch = &transferBatch{id: a.transfers.nextBatchID}
			batch.policy.Store(int32(transferPolicy(a.prefs.ConflictPolicy)))
		}
		a.transfers.nextID++
		sourcePath := entry.path
		if sourcePath == "" {
			pane := session.local
			if sourceRemote {
				pane = session.remote
			}
			sourcePath = transferJoin(sourceRemote, pane.path, entry.name)
		}
		task := &transferTask{id: a.transfers.nextID, session: snapshot, view: session, entry: entry,
			sourceRemote: sourceRemote, sourcePath: sourcePath, targetPath: targetPath, state: transferQueued}
		task.job = &transferJob{sourceRemote: sourceRemote, sourcePath: sourcePath,
			targetPath: targetPath, name: entry.name, policy: transferPolicy(a.prefs.ConflictPolicy), batch: batch,
			temporary: map[string]struct{}{}}
		a.transfers.tasks = append(a.transfers.tasks, task)
	}
	a.persistTransfers()
	a.scheduleTransfers()
}

func (a *app) tickTransfers(c *ui.Context) {
	a.advanceTransferConflicts()
	session := a.activeTransferSession()
	for _, task := range a.transfers.tasks {
		if task.failureNotice && session != nil && task.view == session {
			task.failureNotice = false
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
				scroller := ui.Scroll(c.Key("transfer-rows")).Grow(1).MinHeight(0).TrackScroll(scroll).
					Transition(ui.ElementTransition{Duration: t.Motion.Duration.Focus, Exit: &ui.Motion{}}).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						if len(rows) == 0 {
							message := locales.MessageNoTransfersInThisCategory()
							if len(tasks) == 0 {
								message = locales.MessageNoTransfersYet()
							}
							uikit.TransferDockEmpty(c, message)
						}
						for _, task := range rows {
							a.transferRow(c, task)
						}
					})
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
		if q.filter == 1 && (task.state == transferDone || task.state == transferFailed) {
			continue
		}
		if q.filter == 2 && task.state != transferDone {
			continue
		}
		rows = append(rows, task)
	}
	return rows
}

func (a *app) transferHeader(c *ui.Context, session *fileSession, tasks []*transferTask) {
	t, q := theme.Current(), &a.transfers
	s := summarizeTransfers(tasks)
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
			button.OnClick(func() { a.recoverTransferTabs(c) })
		} else if !compact {
			uikit.TransferHeaderSummary(
				c,
				locales.MessageTotalSpeed()+transferBytes(int64(s.speed))+"/s · "+transferRemaining(s.total-s.bytes, s.speed),
			)
		}
		pauseIcon, pauseLabel := transferPauseIcon, locales.MessagePauseAll()
		if s.running+s.queued == 0 && s.paused > 0 {
			pauseIcon, pauseLabel = transferPlayIcon, locales.MessageResumeAll()
		}
		uikit.TransferAction(c, "pause-all", pauseLabel, pauseIcon, s.running+s.queued+s.paused == 0).
			OnClick(func() { a.pauseAllTransfers(c, session) })
		uikit.TransferAction(
			c,
			"clear-done",
			locales.MessageClearCompleted(),
			transferTrashIcon,
			s.done == 0,
		).OnClick(func() {
			q.tasks = slices.DeleteFunc(q.tasks, func(task *transferTask) bool {
				return session != nil && task.view == session && task.state == transferDone
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
	active, done := summary.running+summary.queued+summary.paused, summary.done
	items := []uikit.SegItem{{Label: locales.MessageAll(), Count: &total}, {Label: locales.MessageInProgress(), Count: &active}, {Label: locales.MessageCompleted(), Count: &done}}
	if uikit.TransferFilters(c, locales.MessageTransferCategory(), &q.filter, items).Changed() {
		a.saveFilePreferences(c)
	}
}

func (a *app) transferRow(c *ui.Context, task *transferTask) {
	states := [...]uikit.TransferState{
		transferQueued: uikit.TransferQueued, transferRunning: uikit.TransferRunning,
		transferPaused: uikit.TransferPaused, transferDone: uikit.TransferDone, transferFailed: uikit.TransferFailed,
	}
	view := uikit.Transfer{
		Key: task.id, Name: task.entry.name, SourcePath: task.sourcePath,
		TargetPath: path.Join(task.targetPath, task.entry.name),
		Amount:     transferBytes(task.transferred) + " / " + transferBytes(task.total),
		Percent:    fmt.Sprintf("%d%%", int(transferFraction(task)*100)),
		Value:      transferFraction(task),
		State:      states[task.state], Download: task.sourceRemote, Zebra: task.id%2 == 0,
		Speed: "0 B/s", Remaining: locales.MessageRemaining(), Error: task.errorText.String(),
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}
	if task.state == transferRunning {
		view.Speed = transferBytes(int64(task.speed)) + "/s"
		view.Remaining = transferRemaining(task.total-task.transferred, task.speed)
	}
	if task.state == transferDone {
		view.Elapsed = fmt.Sprintf(locales.MessageCompletedInS(), task.completed.Sub(task.started).Seconds())
		view.Completed = localizedDate(task.completed, i18n.NoStyle, i18n.MediumStyle) + locales.MessageCompleted2()
	}
	result := uikit.TransferRow(c, &view)
	menu := []uikit.MenuEntry{}
	menu = append(menu, uikit.MenuEntry{ID: "reveal", Label: locales.MessageShowInFolder(), Icon: uikit.IconFolder, OnChoose: func() { a.revealTransferredFile(c, task) }})
	uikit.ContextMenuOn(c, result.Root, menu)
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
		ID: fmt.Sprintf("transfer-failed-%d", task.id), Title: locales.Concat(
			locales.Raw(task.entry.name),
			locales.Deferred(locales.MessageTransferFailed),
		), Detail: task.errorText,
		Tone: uikit.ToneDanger, Action: locales.Deferred(locales.MessageRetry), OnAction: func() {
			if !slices.Contains(a.transfers.tasks, task) {
				return
			}
			a.resumeTransfer(task)
		},
	})
}

func (a *app) pauseAllTransfers(c *ui.Context, session *fileSession) {
	tasks := a.transfers.sessionTasks(session)
	s := summarizeTransfers(tasks)
	resume := s.running+s.queued == 0 && s.paused > 0
	for _, task := range tasks {
		if resume && task.state == transferPaused {
			a.resumeTransfer(task)
		} else if !resume && (task.state == transferRunning || task.state == transferQueued) {
			a.pauseTransfer(task)
		}
	}
	a.scheduleTransfers()
}
