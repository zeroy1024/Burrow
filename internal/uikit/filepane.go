package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func FileWorkspace(c *ui.Context) ui.Element {
	return ui.Column(c).Key("file-workspace").Grow(1).MinWidth(0).MinHeight(0)
}

func FilePanels(c *ui.Context) ui.Element {
	return ui.Box(c).Key("file-panels").Grow(1).FillWidth().MinWidth(0).MinHeight(0).ClipX()
}

func FilePane(c *ui.Context, key any, label string, focus *ui.Handle) ui.Element {
	t := theme.Current()
	card := ui.Box(c).Key(key).Fill().Radius(t.C.Card.Radius).Background(Color(t.C.Card.Bg)).Clip().
		Focusable().FocusRing(false).Bind(focus).Label(label)
	card.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		p.Stroke(rect, Color(t.Color.Border.Card), t.C.Card.Radius, t.Size.Divider)
	})
	return card
}

func FilePaneHeader(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Row(c).Height(t.C.FilePane.HeaderHeight).Padding(0, t.Space.N8).
		Gap(t.C.FilePane.ToolGap).FillWidth()
}

type PaneToggleProps struct {
	Label        string
	Icon, IconOn *ui.SVG
}

func PaneToggle(c *ui.Context, on *bool, props PaneToggleProps) ui.Element {
	t := theme.Current()
	p := t.C.PaneToggle
	toggle := Tip(c, ui.ToggleBase(c, on).Size(p.Size, p.Size).Radius(p.Radius).Label(props.Label), props.Label)
	bg, ink, glyph := ui.Transparent, Color(p.Icon), props.Icon
	if toggle.Hovered() {
		bg = Color(p.BgHover)
	}
	if *on {
		bg, ink, glyph = Color(p.BgOn), Color(p.IconOn), props.IconOn
	}
	animation := Tween(p.Transition)
	return toggle.Background(toggle.AnimateColor("bg", bg, animation)).TextColor(toggle.AnimateColor("fg", ink, animation)).
		Children(func() { ui.Icon(c, glyph).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
}

func FilePaneFooter(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Row(c).Absolute().Bottom(0).Left(0).Right(0).Height(t.C.FilePane.StatusHeight).
		Padding(0, t.C.StatusBar.PaddingX).Gap(t.Space.N8)
}

func FileDivider(c *ui.Context, left float32) ui.Element {
	t := theme.Current()
	return ui.Box(c).Key("file-divider").Absolute().Left(left).Top(0).Bottom(0).
		Width(t.C.FTPSplit.Gutter).Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).Label("调整本地面板宽度")
}

func StyleFileDivider(c *ui.Context, divider ui.Element, dragging bool) {
	t := theme.Current()
	p := t.C.FTPSplit
	if divider.Hovered() || dragging || divider.FocusVisible() {
		ink := p.Grip
		if dragging {
			ink = p.GripActive
		}
		divider.Center().Children(func() {
			ui.Box(c).Size(p.GripW, p.GripH).Radius(p.GripW).Background(Color(ink)).PassThrough()
		})
	}
}

func PaneLabel(c *ui.Context, label string, localOpen, active bool) ui.Element {
	t := theme.Current()
	focus := t.C.FilePaneFocus
	bg, ink, weight := ui.Transparent, Color(focus.LabelTextInactive), focus.LabelWeightInactive
	if localOpen && active {
		bg, ink, weight = Color(focus.LabelBgActive), Color(focus.LabelTextActive), focus.LabelWeightActive
	}
	if !localOpen {
		ink, weight = Color(t.Color.Text.Primary), focus.LabelWeightActive
	}
	badge := ui.Row(c).Height(t.C.Breadcrumb.Height).Padding(0, focus.LabelPaddingX).Radius(focus.LabelRadius).Shrink(0)
	animation := Tween(focus.Transition)
	return badge.Background(badge.AnimateColor("label-bg", bg, animation)).Children(func() {
		text := ui.Text(c, label).FontSize(t.Font.Size.Body).FontWeight(weight).SingleLine()
		text.TextColor(text.AnimateColor("label-fg", ink, animation))
	})
}

type HistoryParts struct {
	Back, Forward ui.Element
}

func HistoryButtons(c *ui.Context, canBack, canForward bool, backIcon, forwardIcon *ui.SVG) HistoryParts {
	return HistoryParts{
		Back:    IconButton(c, backIcon, IconButtonProps{Key: "后退", Label: "后退"}).Disabled(!canBack),
		Forward: IconButton(c, forwardIcon, IconButtonProps{Key: "前进", Label: "前进"}).Disabled(!canForward),
	}
}
