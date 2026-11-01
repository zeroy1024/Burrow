package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func Card(c *ui.Context) ui.Element {
	p := theme.Current().C.Card
	return Shadow(ui.Column(c).Radius(p.Radius).Background(Color(p.Bg)), p.Shadow).Clip()
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
	t := theme.Current()
	ink := Color(t.C.Desktop.DividerHandleColor)
	if dragging {
		ink = Color(t.C.DropIndicator.LineColor)
	}
	e.Children(func() {
		ui.Box(c).Size(t.C.Desktop.DividerHandleWidth, t.C.Desktop.DividerHandleHeight).
			Radius(t.C.Desktop.DividerHandleRadius).Background(ink).PassThrough().
			Opacity(Visual(e, "handle", visible, t.C.Desktop.DividerFadeDuration))
	})
}
