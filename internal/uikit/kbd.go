package uikit

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type KbdTone uint8

const (
	KbdKeycap KbdTone = iota
	KbdFlat
	KbdOnInk
	KbdOnAccent
)

type KbdProps struct {
	Tone     KbdTone
	FontSize float32
	Text     *color.NRGBA
}

// Kbd is decorative. Flat hints add no padding or height to the original text.
func Kbd(c *ui.Context, keys string, props KbdProps) ui.Element {
	t := theme.Current()
	p := t.C.Kbd
	fontSize := props.FontSize
	if fontSize == 0 {
		fontSize = p.FontSize
	}
	text, background := p.Text, p.Bg
	switch props.Tone {
	case KbdOnInk:
		text, background = p.InkText, p.InkBg
	case KbdOnAccent:
		text, background = p.AccentText, p.AccentBg
	}
	if props.Text != nil {
		text = *props.Text
	}
	e := ui.Text(c, keys).FontSize(fontSize).TextColor(Color(text)).Role(ui.RoleNone).PassThrough()
	if props.Tone == KbdFlat {
		return e
	}
	e.Height(p.Height).MinWidth(p.MinWidth).Padding(0, p.PaddingX).Radius(p.Radius).Background(Color(background))
	if props.Tone == KbdKeycap {
		e.Border(t.Size.Divider, Color(p.Border))
	}
	return e
}
