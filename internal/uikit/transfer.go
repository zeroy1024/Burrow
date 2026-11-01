package uikit

import (
	"image/color"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func TransferDock(c *ui.Context) ui.Element {
	return ui.Column(c).Key("transfer-dock").Shrink(0)
}

func TransferDockCard(c *ui.Context) ui.Element {
	p := theme.Current().C.TransferDock
	return ui.Column(c).Key("transfer-dock-card").Fill().MinHeight(0).Radius(p.Radius).Background(Color(p.Bg)).Clip()
}

func TransferDockEmpty(c *ui.Context, message string) ui.Element {
	t := theme.Current()
	return Text(c, message, TextProps{Style: TextFootnote, Tone: ToneSecondary}).
		Height(t.C.TransferDock.RowHeight).Padding(t.Space.N12).TextColor(Color(t.C.FileList.Meta))
}

func TransferDockDivider(c *ui.Context, label, hint string) ui.Element {
	p := theme.Current().C.FTPSplit
	return Tip(c, ui.Box(c).Key("transfer-dock-divider").Absolute().Left(0).Right(0).
		Top(-p.Gutter).Height(p.Gutter).Cursor(ui.CursorResizeNS).Focusable().FocusRing(false).Role(ui.RoleSplitter).Label(label), hint)
}

func TransferDockGrip(c *ui.Context, divider ui.Element, active bool) {
	p := theme.Current().C.FTPSplit
	ink := p.Grip
	if active {
		ink = p.GripActive
	}
	divider.Center().Children(func() {
		ui.Box(c).Size(p.GripH, p.GripW).Radius(p.GripW).Background(Color(ink)).PassThrough()
	})
}

func TransferHeader(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Row(c).Height(t.C.TransferDock.HeaderHeight).Shrink(0).Padding(0, t.Space.N8).Gap(t.Space.N6).AlignItems(ui.Center)
}

func TransferHeaderTitle(c *ui.Context, title string, icon *ui.SVG) {
	t := theme.Current()
	ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(Color(t.C.TransferRow.UpIcon))
	Text(c, title, TextProps{Style: TextBodyStrong}).FontWeight(t.Font.Weight.Semibold).SingleLine()
}

func TransferHeaderSummary(c *ui.Context, summary string) ui.Element {
	t := theme.Current()
	return Text(c, summary, TextProps{Style: TextFootnote, Tone: ToneSecondary}).
		TextColor(Color(t.C.FileList.Meta)).SingleLine().Shrink(1)
}

func TransferFilters(c *ui.Context, label string, selected *int, items []SegItem) ui.Element {
	return CountTabs(c, selected, items).Label(label)
}

func TransferAction(c *ui.Context, key, label string, icon *ui.SVG, disabled bool) ui.Element {
	t := theme.Current()
	surface := *t
	surface.C.IconButton.Icon = t.C.IconButton.IconSubtle
	return IconButton(c, icon, IconButtonProps{Key: key, Label: label, Size: SizeSm, Disabled: disabled, Surface: &surface})
}

type transferRowProps struct {
	Key   any
	Zebra bool
}

func transferRowShell(c *ui.Context, props transferRowProps) ui.Element {
	t := theme.Current()
	p := t.C.TransferRow
	row := ui.Row(c).Key(props.Key).Height(t.C.TransferDock.RowHeight).Shrink(0).FillWidth().Padding(0, p.PaddingX).
		Gap(p.Gap).AlignItems(ui.Center).Radius(t.C.FileList.RowRadius)
	if props.Zebra {
		row.Background(Color(t.C.FileList.Zebra))
	}
	return row
}

func TransferIcon(c *ui.Context, icon *ui.SVG, background, foreground color.NRGBA) ui.Element {
	t := theme.Current()
	p := t.C.TransferRow
	return ui.Box(c).Size(p.IconTile, p.IconTile).Shrink(0).Radius(p.IconRadius).Background(Color(background)).Center().Children(func() {
		ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(Color(foreground))
	})
}

func TransferNameColumn(c *ui.Context) ui.Element {
	return ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Current().Space.N2)
}

func TransferName(c *ui.Context, name string) ui.Element {
	t := theme.Current()
	return Text(c, name, TextProps{Style: TextBodyStrong}).FontSize(t.C.TransferRow.NameSize).TextColor(Color(t.C.Pane.Name)).SingleLine()
}

func TransferPath(c *ui.Context, path string) ui.Element {
	t := theme.Current()
	return MiddleLabel(c, path, MiddleLabelProps{
		Font:  ui.Font{Family: strings.Join(t.Font.Family.Mono, ", "), Size: t.C.TransferRow.PathSize},
		Color: Color(t.C.FileList.Meta), Path: true,
	}).FillWidth()
}

func TransferProgressColumn(c *ui.Context, amount, percent string, progress func()) ui.Element {
	t := theme.Current()
	p := t.C.TransferRow
	return ui.Column(c).Width(p.ProgressWidth).Shrink(0).Gap(t.Space.N3).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			TransferDetail(c, amount, t.C.FileList.Meta, false)
			ui.Spacer(c)
			TransferDetail(c, percent, t.C.FileList.Meta, false)
		})
		progress()
	})
}

func TransferSpeedColumn(c *ui.Context) ui.Element {
	return ui.Column(c).Width(theme.Current().C.TransferRow.SpeedWidth).Shrink(0).Gap(theme.Current().Space.N2)
}

func TransferDetail(c *ui.Context, text string, foreground color.NRGBA, tooltip bool) ui.Element {
	item := Text(c, text, TextProps{Style: TextFootnote}).FontSize(theme.Current().C.TransferRow.DetailSize).TextColor(Color(foreground)).SingleLine()
	if tooltip {
		return Tip(c, item, text)
	}
	return item
}

func TransferStatus(c *ui.Context) ui.Element {
	return ui.Row(c).Width(theme.Current().C.TransferRow.StatusWidth).Shrink(0).Justify(ui.End).AlignItems(ui.Center)
}

func TransferActions(c *ui.Context) ui.Element {
	t := theme.Current()
	p := t.C.TransferRow
	return ui.Row(c).Key("transfer-actions").Width(2*t.C.IconButton.SizeSm + p.Gap).Shrink(0).Gap(p.Gap).Justify(ui.End).AlignItems(ui.Center)
}

type TransferState uint8

const (
	TransferQueued TransferState = iota
	TransferRunning
	TransferPaused
	TransferDone
	TransferFailed
)

// Transfer is a display snapshot. Queue ownership, elapsed time and task
// operations remain in the application that consumes TransferRowResult.
type Transfer struct {
	Key                                                          any
	Name, SourcePath, TargetPath                                 string
	Amount, Percent, Speed, Remaining, Elapsed, Completed, Error string
	Value                                                        float32
	State                                                        TransferState
	Download, Zebra, Disabled                                    bool
	Motion                                                       MotionMode
}

type TransferRowResult struct {
	Root                                 ui.Element
	Pause, Resume, Retry, Reveal, Cancel bool
}

func TransferRow(c *ui.Context, transfer *Transfer) TransferRowResult {
	t := theme.Current()
	p := t.C.TransferRow
	result := TransferRowResult{Root: transferRowShell(c, transferRowProps{Key: transfer.Key, Zebra: transfer.Zebra}).Disabled(transfer.Disabled)}
	StyleDisabled(result.Root, transfer.Disabled)
	row := result.Root
	compact := row.Bounds().W < t.C.TransferDock.CompactBreakpoint
	progressTone, status := ProgressPaused, StatusPaused
	icon, bg, ink := IconUpload, p.UpBg, p.UpIcon
	if transfer.Download {
		icon, bg, ink = IconDownload, p.DownBg, p.DownIcon
	}
	switch transfer.State {
	case TransferRunning:
		progressTone, status = ProgressRunning, StatusRunning
	case TransferPaused:
		progressTone, status = ProgressPaused, StatusPaused
	case TransferDone:
		progressTone, status = ProgressDone, StatusDone
		icon, bg, ink = IconCheck, p.DoneBg, p.DoneIcon
	case TransferFailed:
		progressTone, status = ProgressError, StatusError
		icon, bg, ink = IconAlertCircle, p.ErrBg, p.ErrIcon
	}
	progress := func() {
		Progress(c, ProgressProps{Value: transfer.Value, Tone: progressTone, Motion: transfer.Motion, Label: transfer.Name + "传输进度", Animation: ui.Animation{Spring: Spring(t.C.TransferDock.Grow)}})
	}
	row.Children(func() {
		TransferIcon(c, IconSVG(icon), bg, ink)
		TransferNameColumn(c).Children(func() {
			TransferName(c, transfer.Name)
			if !compact {
				TransferPath(c, transfer.SourcePath+" → "+transfer.TargetPath)
				return
			}
			detail := transfer.Amount + " · " + transfer.Percent
			foreground := t.Color.Text.Secondary
			switch transfer.State {
			case TransferRunning:
				detail += " · " + transfer.Remaining
			case TransferDone:
				detail = transfer.TargetPath
			case TransferFailed:
				detail, foreground = transfer.Error, t.Color.Danger.Text
			}
			TransferDetail(c, detail, foreground, true)
			if transfer.State != TransferDone && transfer.State != TransferFailed {
				progress()
			}
		})
		if !compact {
			TransferProgressColumn(c, transfer.Amount, transfer.Percent, progress)
			TransferSpeedColumn(c).Children(func() {
				primary, secondary, foreground := transfer.Speed, transfer.Remaining, t.Color.Text.Secondary
				switch transfer.State {
				case TransferRunning:
					foreground = t.Color.Text.Accent
				case TransferDone:
					primary, secondary = transfer.Elapsed, transfer.Completed
				case TransferFailed:
					primary, secondary = "传输失败", transfer.Error
				}
				TransferDetail(c, primary, foreground, false)
				TransferDetail(c, secondary, t.Color.Text.Secondary, true)
			})
		}
		TransferStatus(c).Children(func() {
			if compact && transfer.State == TransferRunning {
				TransferDetail(c, transfer.Speed, t.Color.Text.Accent, false)
			} else if transfer.State == TransferQueued {
				Badge(c, "排队中", BadgeProps{Tone: ToneMuted, Dot: true})
			} else {
				StatusPill(c, status)
			}
		})
		TransferActions(c).Children(func() {
			if transfer.State == TransferDone {
				result.Reveal = TransferAction(c, "reveal", "在文件夹中显示", IconSVG(IconFolder), transfer.Disabled).Clicked()
				return
			}
			icon, label := IconPause, "暂停传输"
			if transfer.State == TransferPaused {
				icon, label = IconPlay, "继续传输"
			}
			if transfer.State == TransferFailed {
				icon, label = IconRetry, "重试传输"
			}
			if TransferAction(c, "toggle-transfer", label, IconSVG(icon), transfer.Disabled).Clicked() {
				switch transfer.State {
				case TransferPaused:
					result.Resume = true
				case TransferFailed:
					result.Retry = true
				default:
					result.Pause = true
				}
			}
			label = "取消传输"
			if transfer.State == TransferFailed {
				label = "移除失败任务"
			}
			result.Cancel = TransferAction(c, "remove-transfer", label, IconSVG(IconClose), transfer.Disabled).Clicked()
		})
	})
	return result
}
