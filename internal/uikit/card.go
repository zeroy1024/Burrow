package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
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

type InsetProps struct {
	// Tone tints the panel for a warning or a failure; other tones are neutral.
	Tone Tone
	// Raised uses the card surface, for items placed on a neutral inset.
	Raised bool
	// Compact sets row padding and radius.
	Compact bool
}

// Inset is a bordered panel set into a dialog or card body, such as a rule,
// a task or a per-file result. Callers add its width and gap.
func Inset(c *ui.Context, options ...InsetProps) ui.Element {
	p := theme.Current().C.Inset
	var props InsetProps
	if len(options) != 0 {
		props = options[0]
	}
	bg, border := p.Bg, p.Border
	if props.Raised {
		bg = p.RaisedBg
	}
	switch props.Tone {
	case ToneWarning:
		bg, border = p.WarningBg, p.WarningBorder
	case ToneDanger:
		bg, border = p.DangerBg, p.DangerBorder
	}
	inset := ui.Column(c).Padding(p.Padding).Radius(p.Radius)
	if props.Compact {
		inset.Padding(p.CompactPaddingY, p.CompactPaddingX).Radius(p.CompactRadius)
	}
	return inset.Background(Color(bg)).Border(p.BorderWidth, Color(border))
}

func Workspace(c *ui.Context, key any) ui.Element {
	t := theme.Current()
	return ui.Row(c).Key(key).Fill().MinWidth(0).AlignItems(ui.Stretch).
		Padding(0, t.C.Window.Gutter, t.C.Window.Gutter, 0)
}

// SidebarDivider is the resize target in the gutter after a sidebar.
func SidebarDivider(c *ui.Context, label string, left, shown float32) ui.Element {
	return gutterDivider(c, "resize-sidebar", label).Left(left).
		Width(theme.Current().C.Window.Gutter * shown).Hidden(shown != 1)
}

// DrawerDivider is the resize target in the gutter before a drawer whose
// width is right.
func DrawerDivider(c *ui.Context, key any, label string, right float32) ui.Element {
	return gutterDivider(c, key, label).Right(right).Width(theme.Current().C.Window.Gutter)
}

func gutterDivider(c *ui.Context, key any, label string) ui.Element {
	return ui.Box(c).Key(key).Absolute().Top(0).Bottom(0).
		Cursor(ui.CursorResizeEW).Focusable().FocusRing(false).Center().Role(ui.RoleSplitter).Label(label)
}

// ResizeGutter applies a gutter divider's drag and Left/Right keys to *width
// within [lo, hi] and draws its handle. A trailing panel, such as a drawer,
// grows as the divider moves left. It reports whether the width changed.
func ResizeGutter(c *ui.Context, divider ui.Element, width *float32, lo, hi float32, trailing bool) bool {
	step := theme.Current().C.Desktop.SiderKeyboardStep
	delta, _, dragging := divider.Dragged()
	if dragging {
		divider.Focus()
	}
	if divider.Shortcut(0, ui.KeyLeft) {
		delta -= step
	}
	if divider.Shortcut(0, ui.KeyRight) {
		delta += step
	}
	if trailing {
		delta = -delta
	}
	if delta != 0 {
		*width = min(max(*width+delta, lo), hi)
	}
	divider.Range(float64(lo), float64(hi), float64(*width))
	visible := float32(0)
	if divider.Hovered() || dragging || divider.FocusVisible() {
		visible = 1
	}
	divider.Children(func() { SplitterHandle(c, divider, visible, dragging, false) })
	return delta != 0
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
