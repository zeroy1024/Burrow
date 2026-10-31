package desktop

import (
	"fmt"
	"image/color"
	"path"
	"slices"
	"strings"
	"time"

	"sshdesk/internal/theme"

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
	transferUpIcon     = lineIcon(`<path d="M12 16V3m-5 5 5-5 5 5M4 13v6a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6"/>`)
	transferDownIcon   = lineIcon(`<path d="M12 3v13m-5-5 5 5 5-5M4 13v6a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6"/>`)
	transferPauseIcon  = lineIcon(`<path d="M9 5v14M15 5v14"/>`)
	transferPlayIcon   = lineIcon(`<path d="m8 4 12 8-12 8Z"/>`)
	transferRetryIcon  = lineIcon(`<path d="M3 10a9 9 0 1 1 2 8M3 3v7h7"/>`)
	transferTrashIcon  = lineIcon(`<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/>`)
	transferCheckIcon  = lineIcon(`<path d="m4 12 5 5L20 6"/>`)
	transferErrorIcon  = lineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v6M12 17h.01"/>`)
	transferFolderIcon = lineIcon(`<path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>`)
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
		if running >= int(theme.Current().C.TransferDock.Concurrency) {
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
	dock := ui.Column(c).Key("transfer-dock").Shrink(0)
	rows := a.filteredTransfers()
	rowsTarget := float32(1)
	if q.collapsed {
		rowsTarget = 0
	}
	rowsShown := dock.AnimateTo("rows", rowsTarget, ui.Animation{Spring: tokenSpring(p.Collapse)})
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
		ui.Column(c).Key("transfer-dock-card").Fill().MinHeight(0).Radius(p.Radius).
			Background(tokenColor(p.Bg)).Clip().Children(func() {
			a.transferHeader(c)
			ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&q.dockScroll).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					if len(rows) == 0 {
						message := "此分类暂无传输"
						if len(q.tasks) == 0 {
							message = "暂无传输任务"
						}
						ui.Text(c, message).Height(p.RowHeight).Padding(t.Space.N12).
							FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.C.FileList.Meta))
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
		divider := ui.Box(c).Key("transfer-dock-divider").Absolute().Left(0).Right(0).
			Top(-t.C.FTPSplit.Gutter).Height(t.C.FTPSplit.Gutter).Cursor(ui.CursorResizeNS).
			Focusable().FocusRing(false).Role(ui.RoleSplitter).Label("调整传输面板高度").
			Tooltip("拖动调整高度 · 双击恢复默认")
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
			color := t.C.FTPSplit.Grip
			if dragging {
				color = t.C.FTPSplit.GripActive
			}
			divider.Center().Children(func() {
				ui.Box(c).Size(t.C.FTPSplit.GripH, t.C.FTPSplit.GripW).Radius(t.C.FTPSplit.GripW).
					Background(tokenColor(color)).PassThrough()
			})
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
	header := ui.Row(c).Height(t.C.TransferDock.HeaderHeight).Shrink(0).
		Padding(0, t.Space.N8).Gap(t.Space.N6).AlignItems(ui.Center)
	compact := header.Bounds().W < t.C.TransferDock.CompactBreakpoint
	header.Children(func() {
		ui.Icon(c, filesIcon).Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(t.C.TransferRow.UpIcon))
		ui.Text(c, "传输").FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Semibold).SingleLine()
		a.transferFilters(c, s)
		ui.Spacer(c)
		if !compact {
			ui.Text(c, "总速度 "+transferBytes(int64(q.displaySpeed))+"/s · "+transferRemaining(s.total-s.bytes, s.speed)).
				FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.C.FileList.Meta)).SingleLine().Shrink(1)
		}
		pauseIcon, pauseLabel := transferPauseIcon, "全部暂停"
		if s.running+s.queued == 0 && s.paused > 0 {
			pauseIcon, pauseLabel = transferPlayIcon, "全部继续"
		}
		transferAction(c, pauseLabel, pauseIcon).Disabled(s.running+s.queued+s.paused == 0).
			OnClick(func() { a.pauseAllTransfers(c) })
		transferAction(c, "清除已完成", transferTrashIcon).Disabled(s.done == 0).OnClick(func() {
			q.tasks = slices.DeleteFunc(q.tasks, func(task *transferTask) bool { return task.state == transferDone })
		})
		label := "折叠传输列表"
		if q.collapsed {
			label = "展开传输列表"
		}
		button := transferAction(c, label, chevronIcon).Expanded(!q.collapsed)
		rotation := float32(0)
		if q.collapsed {
			rotation = 180
		}
		button.AnimatePose(ui.Pose{Rotate: rotation}, ui.Animation{Spring: tokenSpring(t.C.TransferDock.Collapse)})
		button.OnClick(func() {
			q.collapsed = !q.collapsed
			a.saveFilePreferences(c)
		})
	})
}

func (a *app) transferFilters(c *ui.Context, summary transferSummary) {
	t, q := theme.Current().C.Segmented, &a.transfers
	labels := []string{
		fmt.Sprintf("全部 %d", len(q.tasks)),
		fmt.Sprintf("进行中 %d", summary.running+summary.queued+summary.paused),
		fmt.Sprintf("已完成 %d", summary.done),
	}
	parts := ui.SegmentedBase(c, &q.filter, len(labels))
	parts.Track.Padding(t.Padding).Radius(t.Radius).Background(tokenColor(t.Bg)).Label("传输分类").Children(func() {
		for i, label := range labels {
			segment := parts.Segment(i).Padding(t.ItemPaddingY, t.ItemPaddingX).Radius(t.ItemRadius).
				FontSize(t.FontSize).TextColor(tokenColor(t.Text)).Label(label)
			if q.filter == i {
				applyShadow(segment.Background(tokenColor(t.BgOn)).TextColor(tokenColor(t.TextOn)), t.ShadowOn)
			}
			segment.Children(func() { ui.Text(c, label).SingleLine() })
		}
	})
	if parts.Track.Changed() {
		a.saveFilePreferences(c)
	}
}

func transferAction(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	t := theme.Current()
	button := ui.ButtonBase(c).Key(label).Size(t.C.IconButton.SizeSm, t.C.IconButton.SizeSm).
		Radius(t.C.IconButton.Radius).Label(label).Tooltip(label).TextColor(tokenColor(t.C.IconButton.IconSubtle))
	if button.Hovered() {
		button.Background(tokenColor(t.C.IconButton.BgHover))
	}
	button.Children(func() { ui.Icon(c, icon).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
	return button
}

func (a *app) transferRow(c *ui.Context, task *transferTask) {
	t := theme.Current()
	p := t.C.TransferRow
	height := t.C.TransferDock.RowHeight
	row := ui.Row(c).Key(task.id).Height(height).Shrink(0).FillWidth().Padding(0, p.PaddingX).
		Gap(p.Gap).AlignItems(ui.Center).Radius(t.C.FileList.RowRadius)
	compact := row.Bounds().W < t.C.TransferDock.CompactBreakpoint
	if task.id%2 == 0 {
		row.Background(tokenColor(t.C.FileList.Zebra))
	}
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
		ui.Box(c).Size(p.IconTile, p.IconTile).Shrink(0).Radius(p.IconRadius).Background(tokenColor(bg)).Center().
			Children(func() { ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(foreground)) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
			ui.Text(c, task.entry.name).FontSize(p.NameSize).FontWeight(t.Font.Weight.Medium).
				TextColor(tokenColor(t.C.Pane.Name)).SingleLine()
			if compact {
				a.transferCompactDetail(c, task)
				if task.state != transferDone && task.state != transferFailed {
					a.taskProgress(c, task)
				}
				return
			}
			ui.Text(c, task.sourcePath+" → "+path.Join(task.targetPath, task.entry.name)).FontSize(p.PathSize).
				Font(strings.Join(t.Font.Family.Mono, ", ")).TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
		})
		if !compact {
			ui.Column(c).Width(p.ProgressWidth).Shrink(0).Gap(t.Space.N3).Children(func() {
				ui.Row(c).FillWidth().Children(func() {
					ui.Text(c, transferBytes(task.transferred)+" / "+transferBytes(task.total)).FontSize(p.DetailSize).
						TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
					ui.Spacer(c)
					ui.Text(c, fmt.Sprintf("%d%%", task.transferred*100/task.total)).FontSize(p.DetailSize).
						TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
				})
				a.taskProgress(c, task)
			})
			ui.Column(c).Width(p.SpeedWidth).Shrink(0).Gap(t.Space.N2).Children(func() {
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
				ui.Text(c, speed).FontSize(p.DetailSize).TextColor(tokenColor(p.SpeedText)).SingleLine()
				ui.Text(c, remaining).FontSize(p.DetailSize).TextColor(tokenColor(t.C.FileList.Meta)).SingleLine().Tooltip(remaining)
			})
		}
		ui.Row(c).Width(p.StatusWidth).Shrink(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			if !compact || task.state != transferRunning {
				a.transferBadge(c, task)
			} else {
				ui.Text(c, transferBytes(int64(task.speed))+"/s").FontSize(p.DetailSize).
					TextColor(tokenColor(p.SpeedText)).SingleLine()
			}
		})
		ui.Row(c).Key("transfer-actions").Width(2*t.C.IconButton.SizeSm + p.Gap).Shrink(0).
			Gap(p.Gap).Justify(ui.End).AlignItems(ui.Center).
			Children(func() { a.transferTaskActions(c, task) })
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
	ui.Text(c, text).FontSize(t.C.TransferRow.DetailSize).TextColor(tokenColor(foreground)).SingleLine().Tooltip(text)
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
	transferProgress(c, float32(task.transferred)/float32(task.total), t.C.Progress.Height, t.C.Progress.Track, fill)
}

func transferProgress(c *ui.Context, value, height float32, track, fill color.NRGBA) {
	t := theme.Current()
	bar := ui.Box(c).Height(height).Radius(t.C.Progress.Radius).Background(tokenColor(track)).Clip().
		Role(ui.RoleProgress).Range(0, 1, float64(value))
	bar.FillWidth()
	shown := bar.AnimateTo("progress", value, ui.Animation{Spring: tokenSpring(t.C.TransferDock.Grow)})
	bar.Children(func() {
		ui.Box(c).Height(height).WidthPercent(min(max(shown, 0), 1) * 100).Radius(t.C.Progress.Radius).
			Background(tokenColor(fill))
	})
}

func (a *app) transferBadge(c *ui.Context, task *transferTask) {
	t, p := theme.Current(), theme.Current().C.TransferRow
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
	badge := ui.Row(c).Shrink(0).Padding(t.C.Badge.PaddingYCompact, t.C.Badge.PaddingXCompact).
		Gap(t.C.Badge.Gap).Radius(t.C.Badge.Radius).AlignItems(ui.Center)
	badge.Background(badge.AnimateColor("state-bg", tokenColor(bg), ui.Animation{Duration: p.Transition})).Children(func() {
		ui.Box(c).Size(t.C.ActivityBadge.DotSize, t.C.ActivityBadge.DotSize).Radius(t.C.Badge.Radius).
			Background(tokenColor(dot))
		ui.Text(c, label).FontSize(t.C.Badge.FontSizeCompact).TextColor(tokenColor(foreground)).SingleLine()
	})
}

func (a *app) transferTaskActions(c *ui.Context, task *transferTask) {
	q := &a.transfers
	if task.state == transferDone {
		transferAction(c, "在文件夹中显示", transferFolderIcon).OnClick(func() { a.revealTransferredFile(c, task) })
		return
	}
	icon, label := transferPauseIcon, "暂停传输"
	switch task.state {
	case transferPaused:
		icon, label = transferPlayIcon, "继续传输"
	case transferFailed:
		icon, label = transferRetryIcon, "重试传输"
	}
	transferAction(c, label, icon).OnClick(func() {
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
	transferAction(c, label, closeIcon).OnClick(func() {
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
