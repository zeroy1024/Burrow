package desktop

import (
	"fmt"
	"path"
	"slices"
	"time"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

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

const minimumDemoTransferTime = 8 * time.Second

type transferTask struct {
	id                     int
	session                *fileSession
	entry                  fileEntry
	sourceRemote           bool
	sourcePath, targetPath string
	state                  transferState
	transferred, total     int64
	fraction               float64
	speed                  float64
	started, completed     time.Time
	errorText              string
}

type transferQueueState struct {
	tasks             []*transferTask
	nextID            int
	collapsed         bool
	filter            int
	lastTick, speedAt time.Time
	displaySpeed      float64
	dockScroll        ui.ScrollState
	dockHeight        float32
	dockResizing      bool
}

type transferSummary struct {
	running, queued, paused, done int
	bytes, total                  int64
	speed                         float64
}

var (
	transferUpIcon     = uikit.LineIcon(`<path d="M12 16V3m-5 5 5-5 5 5M4 13v6a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6"/>`)
	transferDownIcon   = uikit.LineIcon(`<path d="M12 3v13m-5-5 5 5 5-5M4 13v6a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6"/>`)
	transferPauseIcon  = uikit.LineIcon(`<path d="M9 5v14M15 5v14"/>`)
	transferPlayIcon   = uikit.LineIcon(`<path d="m8 4 12 8-12 8Z"/>`)
	transferRetryIcon  = uikit.LineIcon(`<path d="M3 10a9 9 0 1 1 2 8M3 3v7h7"/>`)
	transferTrashIcon  = uikit.LineIcon(`<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/>`)
	transferCheckIcon  = uikit.LineIcon(`<path d="m4 12 5 5L20 6"/>`)
	transferErrorIcon  = uikit.LineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v6M12 17h.01"/>`)
	transferFolderIcon = uikit.LineIcon(`<path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>`)
)

func (q *transferQueueState) summary() transferSummary {
	s := transferSummary{}
	for _, task := range q.tasks {
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

// The prototype advances only its in-memory queue; no host or local file is written.
func (a *app) enqueueFiles(
	c *ui.Context,
	session *fileSession,
	sourceRemote bool,
	entries []fileEntry,
	targetPath string,
) {
	q := &a.transfers
	for _, entry := range entries {
		q.nextID++
		source := session.local.path
		if sourceRemote {
			source = session.remote.path
		}
		sourcePath := entry.path
		if sourcePath == "" {
			sourcePath = path.Join(source, entry.name)
		}
		total := max(entry.size, 1)
		speed := min(
			float64(theme.Current().C.TransferRow.DemoBytesPerSecond),
			float64(total)/minimumDemoTransferTime.Seconds(),
		)
		q.tasks = append(q.tasks, &transferTask{
			id: q.nextID, session: session, entry: entry, sourceRemote: sourceRemote,
			sourcePath: sourcePath, targetPath: targetPath,
			state: transferQueued, total: total, speed: speed,
		})
	}
	q.lastTick = c.Now()
	c.AnimationFrame()
}

func (a *app) tickTransfers(c *ui.Context) {
	q := &a.transfers
	now := c.Now()
	elapsed := now.Sub(q.lastTick)
	if q.lastTick.IsZero() {
		elapsed = 0
	}
	q.lastTick = now
	running := 0
	for _, task := range q.tasks {
		if task.state != transferRunning {
			continue
		}
		if task.session.state != connected {
			task.state, task.errorText = transferFailed, "连接已断开（演示）"
			continue
		}
		progress := task.fraction + task.speed*elapsed.Seconds()
		bytes := int64(progress)
		task.fraction = progress - float64(bytes)
		task.transferred = min(task.total, task.transferred+bytes)
		if task.transferred == task.total {
			task.state, task.completed = transferDone, now
			a.completeFileTransfer(task)
			continue
		}
		running++
	}
	for _, task := range q.tasks {
		if running >= a.prefs.TransferParallel {
			break
		}
		if task.state == transferQueued {
			if task.session.state != connected {
				task.state, task.errorText = transferFailed, "连接已断开（演示）"
				continue
			}
			task.state = transferRunning
			if task.started.IsZero() {
				task.started = now
			}
			running++
		}
	}
	summary := q.summary()
	if summary.speed == 0 || now.Sub(q.speedAt) >= theme.Current().C.TransferRow.SpeedInterval {
		q.displaySpeed, q.speedAt = summary.speed, now
	}
	if running > 0 {
		c.AnimationFrame()
	}
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
		return "剩余 —"
	}
	seconds := float64(bytes) / speed
	if seconds < 1 {
		return "即将完成"
	}
	n := int(seconds)
	return fmt.Sprintf("剩余约 %02d:%02d", n/60, n%60)
}

func (a *app) transferDock(c *ui.Context, maxHeight float32) {
	t, q := theme.Current(), &a.transfers
	p := t.C.TransferDock
	dock := uikit.TransferDock(c)
	rows := a.filteredTransfers()
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
		uikit.TransferDockCard(c).Children(func() {
			a.transferHeader(c)
			ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&q.dockScroll).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					if len(rows) == 0 {
						message := "此分类暂无传输"
						if len(q.tasks) == 0 {
							message = "暂无传输任务"
						}
						uikit.TransferDockEmpty(c, message)
					}
					for _, task := range rows {
						a.transferRow(c, task)
					}
				})
			})
		})
		if q.collapsed || !dock.Settled("rows") {
			return
		}
		// Keep the gap outside the clipped card so it remains a full-height hit target.
		divider := uikit.TransferDockDivider(c, "调整传输面板高度", "拖动调整高度 · 双击恢复默认")
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

func (a *app) filteredTransfers() []*transferTask {
	q := &a.transfers
	rows := []*transferTask{}
	for _, task := range q.tasks {
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

func (a *app) transferHeader(c *ui.Context) {
	t, q := theme.Current(), &a.transfers
	s := q.summary()
	header := uikit.TransferHeader(c)
	compact := header.Bounds().W < t.C.TransferDock.CompactBreakpoint
	header.Children(func() {
		uikit.TransferHeaderTitle(c, "传输", filesIcon)
		a.transferFilters(c, s)
		ui.Spacer(c)
		if !compact {
			uikit.TransferHeaderSummary(c, "总速度 "+transferBytes(int64(q.displaySpeed))+"/s · "+transferRemaining(s.total-s.bytes, s.speed))
		}
		pauseIcon, pauseLabel := transferPauseIcon, "全部暂停"
		if s.running+s.queued == 0 && s.paused > 0 {
			pauseIcon, pauseLabel = transferPlayIcon, "全部继续"
		}
		uikit.TransferAction(c, "pause-all", pauseLabel, pauseIcon).Disabled(s.running+s.queued+s.paused == 0).
			OnClick(func() { a.pauseAllTransfers(c) })
		uikit.TransferAction(c, "clear-done", "清除已完成", transferTrashIcon).Disabled(s.done == 0).OnClick(func() {
			q.tasks = slices.DeleteFunc(q.tasks, func(task *transferTask) bool { return task.state == transferDone })
		})
		label := "折叠传输列表"
		if q.collapsed {
			label = "展开传输列表"
		}
		button := uikit.TransferAction(c, "toggle-dock", label, chevronIcon).Expanded(!q.collapsed)
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

func (a *app) transferFilters(c *ui.Context, summary transferSummary) {
	q := &a.transfers
	labels := []string{
		fmt.Sprintf("全部 %d", len(q.tasks)),
		fmt.Sprintf("进行中 %d", summary.running+summary.queued+summary.paused),
		fmt.Sprintf("已完成 %d", summary.done),
	}
	if uikit.TransferFilters(c, "传输分类", &q.filter, labels).Changed() {
		a.saveFilePreferences(c)
	}
}

func (a *app) transferRow(c *ui.Context, task *transferTask) {
	t := theme.Current()
	p := t.C.TransferRow
	row := uikit.TransferRow(c, uikit.TransferRowProps{Key: task.id, Zebra: task.id%2 == 0})
	compact := row.Bounds().W < t.C.TransferDock.CompactBreakpoint
	if row.DoubleClicked() {
		a.revealTransferredFile(c, task)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if task.state == transferRunning && m.Item("模拟权限错误").Chosen() {
			task.state, task.errorText = transferFailed, "权限不足：目标目录不可写（演示）"
		}
		if m.Item("在文件夹中显示").Chosen() {
			a.revealTransferredFile(c, task)
		}
	})
	row.Children(func() {
		icon, bg, foreground := transferUpIcon, p.UpBg, p.UpIcon
		if task.sourceRemote {
			icon, bg, foreground = transferDownIcon, p.DownBg, p.DownIcon
		}
		switch task.state {
		case transferDone:
			icon, bg, foreground = transferCheckIcon, p.DoneBg, p.DoneIcon
		case transferFailed:
			icon, bg, foreground = transferErrorIcon, p.ErrBg, p.ErrIcon
		}
		uikit.TransferIcon(c, icon, bg, foreground)
		uikit.TransferNameColumn(c).Children(func() {
			uikit.TransferName(c, task.entry.name)
			if compact {
				a.transferCompactDetail(c, task)
				if task.state != transferDone && task.state != transferFailed {
					a.taskProgress(c, task)
				}
				return
			}
			uikit.TransferPath(c, task.sourcePath+" → "+path.Join(task.targetPath, task.entry.name))
		})
		if !compact {
			uikit.TransferProgressColumn(c, transferBytes(task.transferred)+" / "+transferBytes(task.total), fmt.Sprintf("%d%%", task.transferred*100/task.total), func() { a.taskProgress(c, task) })
			uikit.TransferSpeedColumn(c).Children(func() {
				speed, remaining := "0 B/s", "剩余 —"
				if task.state == transferRunning {
					speed = transferBytes(int64(task.speed)) + "/s"
					remaining = transferRemaining(task.total-task.transferred, task.speed)
				}
				if task.state == transferDone {
					speed = fmt.Sprintf("用时 %.1f 秒", task.completed.Sub(task.started).Seconds())
					remaining = task.completed.Format("15:04:05") + " 完成"
				}
				if task.state == transferFailed {
					speed, remaining = "传输失败", task.errorText
				}
				uikit.TransferDetail(c, speed, p.SpeedText, false)
				uikit.TransferDetail(c, remaining, t.C.FileList.Meta, true)
			})
		}
		uikit.TransferStatus(c).Children(func() {
			if !compact || task.state != transferRunning {
				a.transferBadge(c, task)
			} else {
				uikit.TransferDetail(c, transferBytes(int64(task.speed))+"/s", p.SpeedText, false)
			}
		})
		uikit.TransferActions(c).Children(func() { a.transferTaskActions(c, task) })
	})
}

func (a *app) transferCompactDetail(c *ui.Context, task *transferTask) {
	t := theme.Current()
	text := transferBytes(task.transferred) + " / " + transferBytes(task.total)
	text += fmt.Sprintf(" · %d%%", task.transferred*100/task.total)
	foreground := t.C.FileList.Meta
	switch task.state {
	case transferRunning:
		text += " · " + transferRemaining(task.total-task.transferred, task.speed)
	case transferFailed:
		text, foreground = task.errorText, t.C.TransferRow.ErrorText
	case transferDone:
		text = transferBytes(task.total) + " · " + task.targetPath
	}
	uikit.TransferDetail(c, text, foreground, true)
}

func (a *app) taskProgress(c *ui.Context, task *transferTask) {
	t := theme.Current()
	fill := t.C.Progress.Fill
	switch task.state {
	case transferPaused, transferQueued:
		fill = t.C.Progress.FillPaused
	case transferDone:
		fill = t.C.Progress.FillDone
	case transferFailed:
		fill = t.C.TransferRow.ErrIcon
	}
	uikit.Progress(c, uikit.ProgressProps{Value: float32(task.transferred) / float32(task.total), Height: t.C.Progress.Height, Track: t.C.Progress.Track, Fill: fill, Animation: ui.Animation{Spring: uikit.Spring(t.C.TransferDock.Grow)}})
}

func (a *app) transferBadge(c *ui.Context, task *transferTask) {
	p := theme.Current().C.TransferRow
	label, bg, foreground, dot := "排队中", p.PausedBg, p.PausedText, p.PausedDot
	switch task.state {
	case transferRunning:
		label, bg, foreground, dot = "上传中", p.RunBg, p.RunText, p.RunDot
		if task.sourceRemote {
			label = "下载中"
		}
	case transferPaused:
		label = "已暂停"
	case transferDone:
		label, bg, foreground, dot = "已完成", p.DoneBadgeBg, p.DoneBadgeText, p.DoneDot
	case transferFailed:
		label, bg, foreground, dot = "失败", p.ErrBadgeBg, p.ErrBadgeText, p.ErrDot
	}
	uikit.TransferBadge(c, uikit.TransferBadgeProps{Label: label, Background: bg, Foreground: foreground, Dot: dot})
}

func (a *app) transferTaskActions(c *ui.Context, task *transferTask) {
	q := &a.transfers
	if task.state == transferDone {
		uikit.TransferAction(c, "reveal", "在文件夹中显示", transferFolderIcon).OnClick(func() { a.revealTransferredFile(c, task) })
		return
	}
	icon, label := transferPauseIcon, "暂停传输"
	switch task.state {
	case transferPaused:
		icon, label = transferPlayIcon, "继续传输"
	case transferFailed:
		icon, label = transferRetryIcon, "重试传输"
	}
	uikit.TransferAction(c, "toggle-transfer", label, icon).OnClick(func() {
		switch task.state {
		case transferRunning, transferQueued:
			task.state = transferPaused
		case transferPaused, transferFailed:
			task.state, task.errorText = transferQueued, ""
			q.lastTick = c.Now()
		}
		c.AnimationFrame()
	})
	label = "取消传输"
	if task.state == transferFailed {
		label = "移除失败任务"
	}
	uikit.TransferAction(c, "remove-transfer", label, closeIcon).OnClick(func() {
		q.tasks = slices.DeleteFunc(q.tasks, func(candidate *transferTask) bool { return candidate == task })
	})
}

func (a *app) pauseAllTransfers(c *ui.Context) {
	q := &a.transfers
	s := q.summary()
	resume := s.running+s.queued == 0 && s.paused > 0
	for _, task := range q.tasks {
		if resume && task.state == transferPaused {
			task.state = transferQueued
			continue
		}
		if !resume && (task.state == transferRunning || task.state == transferQueued) {
			task.state = transferPaused
		}
	}
	q.lastTick = c.Now()
	c.AnimationFrame()
}
