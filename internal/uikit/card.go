package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type CardProps struct {
	Padding float32
	Clip    bool
}

func Card(c *ui.Context, options ...CardProps) ui.Element {
	p := theme.Current().C.Card
	props := CardProps{Clip: true}
	if len(options) != 0 {
		props = options[0]
	}
	card := Shadow(ui.Column(c).Radius(p.Radius).Background(Color(p.Bg)).Padding(props.Padding), p.Shadow)
	if props.Clip {
		card.Clip()
	}
	return card
}

func Workspace(c *ui.Context, key any) ui.Element {
	t := theme.Current()
	return ui.Row(c).Key(key).Fill().MinWidth(0).AlignItems(ui.Stretch).
		Padding(0, t.C.Window.Gutter, t.C.Window.Gutter, 0)
}

func SidebarDivider(c *ui.Context, label string, left, shown float32) ui.Element {
	t := theme.Current()
	return ui.Box(c).Key("resize-sidebar").Absolute().Top(0).Bottom(0).
		Left(left).Width(t.C.Window.Gutter * shown).
		Hidden(shown != 1).Cursor(ui.CursorResizeEW).Focusable().FocusRing(false).Center().Role(ui.RoleSplitter).
		Label(label)
}

func StyleSidebarDivider(c *ui.Context, e ui.Element, visible float32, dragging bool) {
	e.Children(func() { SplitterHandle(c, e, visible, dragging, false) })
}

func SplitterHandle(c *ui.Context, owner ui.Element, visible float32, dragging, horizontal bool) ui.Element {
	t := theme.Current()
	ink := Color(t.C.Desktop.DividerHandleColor)
	if dragging {
		ink = Color(t.C.DropIndicator.LineColor)
	}
	width, height := t.C.Desktop.DividerHandleWidth, t.C.Desktop.DividerHandleHeight
	if horizontal {
		width, height = height, width
	}
	return ui.Box(c).Size(width, height).Radius(t.C.Desktop.DividerHandleRadius).Background(ink).PassThrough().
		Opacity(Visual(owner, "handle", visible, t.C.Desktop.DividerFadeDuration))
}
