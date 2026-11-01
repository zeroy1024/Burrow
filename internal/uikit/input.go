package uikit

import (
	"slices"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type InputProps struct {
	Label, Placeholder string
	Handle             *ui.Handle
	Mono               bool
	DeferStyle         bool
}

// Input returns the text control itself so callers can handle edits before
// applying form validation with StyleFormControl.
func Input(c *ui.Context, value *string, p InputProps) ui.Element {
	t := theme.Current().C.Input
	e := ui.TextInputBase(c, value).Height(t.Height).
		Padding(0, t.PaddingX).Radius(t.Radius).FontSize(t.FontSize).FixedLineHeight(t.LineHeight).
		TextColor(Color(t.Text)).Background(Color(t.Bg)).Label(p.Label)
	if p.Handle != nil {
		e.Bind(p.Handle)
	}
	if p.Placeholder != "" {
		e.Placeholder(p.Placeholder)
	}
	if p.Mono {
		e.Font(strings.Join(t.MonoFont, ", ")).FontSize(t.MonoSize).LetterSpacing(t.MonoTracking)
	}
	if p.DeferStyle {
		return e
	}
	border := t.Border
	switch {
	case e.Focused():
		border = t.BorderFocus
	case e.Hovered():
		border = t.BorderHover
	}
	return e.Border(t.BorderWidth, e.AnimateColor("border", Color(border), Tween(t.Transition)))
}

// StyleFormControl is applied once, after this frame's edit and blur validation.
func StyleFormControl(e ui.Element, message string) ui.Element {
	p := theme.Current().C.Input
	border := p.Border
	if e.Hovered() {
		border = p.BorderHover
	}
	shadow := slices.Clone(p.FocusShadow)
	for index := range shadow {
		shadow[index].Color.A = 0
	}
	if e.Focused() {
		shadow = p.FocusShadow
	}
	if message != "" {
		shadow = p.ErrorShadow
		e.Error(message)
	}
	e.Border(p.BorderWidth, e.AnimateColor("border", Color(border), Tween(p.Transition)))
	return AnimatedShadow(e, "input-ring", shadow, Tween(p.Transition))
}

type PasswordProps struct {
	Label, Placeholder string
	Handle             *ui.Handle
	Shown              bool
	EyeIcon            *ui.SVG
	ToggleKey          any
	InputKey           any
}

type PasswordParts struct {
	Input, Toggle ui.Element
}

// PasswordInput keeps reveal state and validation with its caller.
func PasswordInput(c *ui.Context, value *string, p PasswordProps) PasswordParts {
	t := theme.Current()
	var parts PasswordParts
	ui.Row(c).FillWidth().Children(func() {
		parts.Input = Input(c.Key(p.InputKey), value, InputProps{
			Label: p.Label, Placeholder: p.Placeholder, Handle: p.Handle, Mono: true, DeferStyle: true,
		}).FillWidth().MinWidth(0).Grow(1).PaddingEnd(t.C.Input.Height)
		if !p.Shown {
			parts.Input.Password()
		}
		parts.Toggle = IconButton(c, p.EyeIcon, IconButtonProps{Label: "显示或隐藏" + p.Label, Key: p.ToggleKey}).
			Absolute().Right(0).Top(0).Size(t.C.Input.Height, t.C.Input.Height).TextColor(Color(t.C.Input.Icon))
	})
	return parts
}
