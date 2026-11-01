package uikit

import (
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func ChipInput(c *ui.Context) ui.Element {
	t := theme.Current()
	p := t.C.Input
	return ui.Row(c).FillWidth().MinHeight(p.Height).Padding(t.Space.N3).Gap(t.Space.N4).Wrap().Radius(p.Radius).
		Background(Color(p.Bg)).Border(p.BorderWidth, Color(p.Border))
}

type ChipProps struct {
	Key, RemoveKey any
	Invalid        bool
	RemoveLabel    string
	RemoveIcon     *ui.SVG
}

type ChipParts struct {
	Root, Remove ui.Element
}

func Chip(c *ui.Context, value string, props ChipProps) ChipParts {
	t := theme.Current()
	p := t.C.Input
	parts := ChipParts{Root: ui.Row(c).Height(p.ChipHeight).Padding(0, t.Space.N6).Gap(t.Space.N4).
		Radius(p.ChipRadius).Background(Color(p.ChipBg)).AlignItems(ui.Center)}
	if props.Key != nil {
		parts.Root.Key(props.Key)
	}
	if props.Invalid {
		parts.Root.Background(Color(t.Color.Danger.Subtle))
	}
	parts.Root.Children(func() {
		ui.Text(c, value).Font(strings.Join(p.MonoFont, ", ")).FontSize(p.ChipFontSize).SingleLine()
		parts.Remove = StyleIconButton(ui.ButtonBase(c.Key(props.RemoveKey)).Label(props.RemoveLabel), t).
			Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).Padding(0).Center()
		Tip(c, parts.Remove, props.RemoveLabel)
		parts.Remove.Children(func() {
			ui.Icon(c, props.RemoveIcon).Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).PassThrough()
		})
	})
	return parts
}

func ChipEntry(c *ui.Context, value *string, p InputProps) ui.Element {
	t := theme.Current().C.Input
	e := ui.TextInputBase(c, value).Height(t.ChipHeight).MinWidth(t.NumberWidth).Grow(1).
		Font(strings.Join(t.MonoFont, ", ")).FontSize(t.MonoSize).FixedLineHeight(t.LineHeight).
		Placeholder(p.Placeholder).Label(p.Label)
	if p.Handle != nil {
		e.Bind(p.Handle)
	}
	return e
}
