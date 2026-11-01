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
	return ui.Text(c, message).Height(t.C.TransferDock.RowHeight).Padding(t.Space.N12).
		FontSize(t.Font.Size.Footnote).TextColor(Color(t.C.FileList.Meta))
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
	ui.Text(c, title).FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Semibold).SingleLine()
}

func TransferHeaderSummary(c *ui.Context, summary string) ui.Element {
	t := theme.Current()
	return ui.Text(c, summary).FontSize(t.Font.Size.Footnote).TextColor(Color(t.C.FileList.Meta)).SingleLine().Shrink(1)
}

func TransferFilters(c *ui.Context, label string, selected *int, labels []string) ui.Element {
	p := theme.Current().C.Segmented
	parts := ui.SegmentedBase(c, selected, len(labels))
	parts.Track.Padding(p.Padding).Radius(p.Radius).Background(Color(p.Bg)).Label(label).Children(func() {
		for i, title := range labels {
			segment := parts.Segment(i).Padding(p.ItemPaddingY, p.ItemPaddingX).Radius(p.ItemRadius).
				FontSize(p.FontSize).TextColor(Color(p.Text)).Label(title)
			StyleSegment(segment, p, *selected == i)
			segment.Children(func() { ui.Text(c, title).SingleLine() })
		}
	})
	return parts.Track
}

func TransferAction(c *ui.Context, key, label string, icon *ui.SVG) ui.Element {
	t := theme.Current()
	button := StyleIconButton(ui.ButtonBase(c).Key(key).Label(label), t).
		Size(t.C.IconButton.SizeSm, t.C.IconButton.SizeSm).TextColor(Color(t.C.IconButton.IconSubtle))
	Tip(c, button, label)
	button.Children(func() { ui.Icon(c, icon).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
	return button
}

type TransferRowProps struct {
	Key   any
	Zebra bool
}

func TransferRow(c *ui.Context, props TransferRowProps) ui.Element {
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
	return ui.Text(c, name).FontSize(t.C.TransferRow.NameSize).FontWeight(t.Font.Weight.Medium).TextColor(Color(t.C.Pane.Name)).SingleLine()
}

func TransferPath(c *ui.Context, path string) ui.Element {
	t := theme.Current()
	return ui.Text(c, path).FontSize(t.C.TransferRow.PathSize).Font(strings.Join(t.Font.Family.Mono, ", ")).TextColor(Color(t.C.FileList.Meta)).SingleLine()
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
	item := ui.Text(c, text).FontSize(theme.Current().C.TransferRow.DetailSize).TextColor(Color(foreground)).SingleLine()
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

type TransferBadgeProps struct {
	Label                       string
	Background, Foreground, Dot color.NRGBA
}

func TransferBadge(c *ui.Context, props TransferBadgeProps) ui.Element {
	t := theme.Current()
	p := t.C.Badge
	badge := ui.Row(c).Shrink(0).Padding(p.PaddingYCompact, p.PaddingXCompact).Gap(p.Gap).Radius(p.Radius).AlignItems(ui.Center)
	return badge.Background(badge.AnimateColor("state-bg", Color(props.Background), Tween(t.C.TransferRow.Transition))).Children(func() {
		ui.Box(c).Size(t.C.ActivityBadge.DotSize, t.C.ActivityBadge.DotSize).Radius(p.Radius).Background(Color(props.Dot))
		ui.Text(c, props.Label).FontSize(p.FontSizeCompact).TextColor(Color(props.Foreground)).SingleLine()
	})
}
