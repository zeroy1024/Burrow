package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ButtonVariant uint8

const (
	ButtonSecondary ButtonVariant = iota
	ButtonPrimary
	ButtonPlain
)

type ButtonProps struct {
	Variant  ButtonVariant
	Disabled bool
}

func Button(c *ui.Context, label string, props ButtonProps) ui.Element {
	p := theme.Current().C.Button
	if props.Variant == ButtonPlain {
		return ui.ButtonBase(c).Height(p.Height).Padding(0, p.PaddingX).FontSize(p.FontSize).Label(label).
			Children(func() { ui.Text(c, label) }).Disabled(props.Disabled)
	}
	e := ui.ButtonBase(c).Height(p.Height).Padding(0, p.PaddingX).Gap(p.Gap).
		Radius(p.Radius).FontSize(p.FontSize).Label(label)
	bg, fg := p.Bg, p.Text
	if props.Variant == ButtonPrimary {
		bg, fg = p.PrimaryBg, p.PrimaryText
	} else {
		e.Border(p.BorderWidth, Color(p.Border))
		switch {
		case e.Pressed():
			bg = p.BgPressed
		case e.Hovered():
			bg = p.BgHover
		}
	}
	e.Background(e.AnimateColor("background", Color(bg), Tween(p.Transition))).
		TextColor(Color(fg)).Children(func() { ui.Text(c, label).SingleLine() })
	return e.Disabled(props.Disabled)
}

func StyleIconButton(e ui.Element, t *theme.Theme) ui.Element {
	p := t.C.IconButton
	bg := ui.Transparent
	switch {
	case e.Pressed():
		bg = Color(p.BgPressed)
	case e.Hovered():
		bg = Color(p.BgHover)
	}
	return e.Size(p.Size, p.Size).Radius(p.Radius).TextColor(Color(p.Icon)).
		Background(e.AnimateColor("background", bg, Tween(p.Transition)))
}

func StyleSegment(e ui.Element, p theme.SegmentedTokens, selected bool) ui.Element {
	bg, fg, shadow := ui.Transparent, Color(p.Text), float32(0)
	if selected {
		bg, fg, shadow = Color(p.BgOn), Color(p.TextOn), 1
	} else if e.Hovered() {
		bg = Color(p.BgHover)
	}
	animation := Tween(p.Transition)
	e.Background(e.AnimateColor("background", bg, animation)).
		TextColor(e.AnimateColor("text", fg, animation))
	return ShadowOpacity(e, p.ShadowOn, Visual(e, "shadow", shadow, p.Transition))
}
