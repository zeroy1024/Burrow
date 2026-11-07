package uikit

import (
	"slices"
	"strconv"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type InputProps struct {
	Label, Placeholder string
	Handle             *ui.Handle
	Mono               bool
	Leading            IconName
	LeadingSVG         *ui.SVG
	Trailing           func()
	Error              string
	ReadOnly, Disabled bool
	Width              float32
	DeferStyle         bool
	Motion             MotionMode
}

// Input returns the actual text control so Changed, Submitted and Bind retain
// their native meanings, including when decorations surround the field.
func Input(c *ui.Context, value *string, p InputProps) ui.Element {
	t := theme.Current()
	field := t.C.Input
	var input ui.Element
	build := func() {
		input = ui.TextInputBase(c, value).Height(field.Height).FillWidth().MinWidth(0).
			Padding(0, field.PaddingX).Radius(field.Radius).FontSize(field.FontSize).FixedLineHeight(field.LineHeight).
			TextColor(Color(field.Text)).Background(Color(field.Bg)).Label(p.Label).
			ReadOnly(p.ReadOnly).Disabled(p.Disabled).FocusRing(false)
		if p.Handle != nil {
			input.Bind(p.Handle)
		}
		input.Placeholder(p.Placeholder)
		if p.Mono {
			input.Font(strings.Join(field.MonoFont, ", ")).FontSize(field.MonoSize).LetterSpacing(field.MonoTracking)
		}
		if p.Width > 0 && p.Leading == "" && p.LeadingSVG == nil && p.Trailing == nil {
			input.Width(p.Width)
		}
		if p.Disabled {
			input.Background(Color(field.DisabledBg)).TextColor(Color(field.DisabledText))
		}
	}
	if p.Leading != "" || p.LeadingSVG != nil || p.Trailing != nil {
		root := ui.Row(c).Height(field.Height).MinWidth(0)
		if p.Width > 0 {
			root.Width(p.Width)
		} else {
			root.FillWidth()
		}
		root.Children(func() {
			build()
			if p.Leading != "" || p.LeadingSVG != nil {
				input.PaddingStart(field.PaddingX + t.Size.IconSm + field.Gap)
				leading := ui.Row(c).Absolute().Top(0).Bottom(0).Left(field.PaddingX).Center().PassThrough()
				if p.Disabled {
					leading.Opacity(t.Opacity.Disabled)
				}
				leading.Children(func() {
					if p.LeadingSVG != nil {
						ui.Icon(c, p.LeadingSVG).Size(t.Size.IconSm, t.Size.IconSm).TextColor(Color(field.Icon)).PassThrough()
					} else {
						Icon(c, p.Leading, IconProps{Size: SizeSm}).TextColor(Color(field.Icon)).PassThrough()
					}
				})
			}
			if p.Trailing != nil {
				input.PaddingEnd(field.Height)
				StyleDisabled(ui.Row(c).Absolute().Top(0).Bottom(0).Right(0).Center(), p.Disabled).Children(p.Trailing)
			}
		})
	} else {
		build()
	}
	StyleDisabled(input, p.Disabled)
	if !p.DeferStyle {
		StyleFormControl(input, p.Error)
	}
	return input
}

// StyleFormControl is applied once, after this frame's edit and blur validation.
func StyleFormControl(e ui.Element, message string) ui.Element {
	return styleFormControl(e, message, e.FocusVisible())
}

func styleFormControl(e ui.Element, message string, focused bool) ui.Element {
	p := theme.Current().C.Input
	border := p.Border
	if e.Hovered() {
		border = p.BorderHover
	}
	shadow := slices.Clone(p.FocusShadow)
	for index := range shadow {
		shadow[index].Color.A = 0
	}
	if focused {
		shadow = p.FocusShadow
	}
	if message != "" {
		shadow = p.ErrorShadow
		e.Error(message)
	}
	if e.IsDisabled() {
		border = p.DisabledBorder
	}
	e.Border(p.BorderWidth, e.AnimateColor("border", Color(border), Tween(p.Transition)))
	return AnimatedShadow(e, "input-ring", shadow, Tween(p.Transition))
}

type PasswordParts struct {
	Root, Input, Toggle ui.Element
}

func PasswordInput(c *ui.Context, value *string, p InputProps) PasswordParts {
	t := theme.Current()
	parts := PasswordParts{Root: ui.Row(c).FillWidth().MinWidth(0)}
	shown := ui.Local(parts.Root, "uikit.password.reveal", func() bool { return false })
	parts.Root.Children(func() {
		parts.Input = Input(c.Key("input"), value, p).Grow(1).PaddingEnd(t.C.Input.Height)
		label := locales.MessageShowPassword()
		if *shown {
			label = locales.MessageHidePassword()
		}
		icon := IconEye
		if *shown {
			icon = IconEyeOff
		}
		parts.Toggle = IconButton(c, icon, IconButtonProps{Label: label, Key: "reveal", Size: SizeXs, Disabled: p.Disabled}).
			Absolute().Right(t.C.Input.PaddingX / 2).Top((t.C.Input.Height - t.C.IconButton.SizeXs) / 2)
		if parts.Toggle.Clicked() {
			*shown = !*shown
		}
		if !*shown {
			parts.Input.Password()
		}
	})
	return parts
}

// PortInput deliberately retains the raw text, including an empty in-progress
// edit; protocol defaults and persistence belong to the session draft.
func PortInput(c *ui.Context, value *string, p InputProps) ui.Element {
	p.Mono = true
	if *value != "" {
		port, err := strconv.Atoi(*value)
		if err != nil || port < 1 || port > 65535 {
			p.Error = locales.MessagePortMustBeBetweenAnd()
		}
	}
	return Input(c, value, p)
}
