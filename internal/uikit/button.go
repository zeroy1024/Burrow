package uikit

import (
	"runtime"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ButtonVariant uint8

const (
	ButtonSecondary ButtonVariant = iota
	ButtonPrimary
	ButtonPlain
	ButtonDanger
	ButtonDangerPlain
)

type ButtonProps struct {
	Variant  ButtonVariant
	Size     Size
	Icon     IconName
	Leading  *ui.SVG
	Kbd      string
	Loading  bool
	Disabled bool
	Motion   MotionMode
}

func Button(c *ui.Context, label string, props ButtonProps) ui.Element {
	t := theme.Current()
	p := t.C.Button
	height, padding := p.Height, p.PaddingX
	switch props.Size {
	case SizeSm, SizeXs:
		height, padding = p.HeightSm, p.PaddingXSm
	case SizeLg:
		height, padding = p.HeightLg, p.PaddingXLg
	}
	disabled := props.Disabled || props.Loading
	e := ui.ButtonBase(c).Height(height).Padding(0, padding).Gap(p.Gap).
		Radius(p.Radius).FontSize(p.FontSize).Label(label).Disabled(disabled).FocusRing(false)
	bg, hover, pressed, fg := p.SecondaryBg, p.SecondaryBgHover, p.BgPressed, p.Text
	switch props.Variant {
	case ButtonPrimary:
		bg, hover, pressed, fg = p.PrimaryBg, p.PrimaryBgHover, p.PrimaryBgPressed, p.PrimaryText
	case ButtonPlain:
		bg, hover, pressed = t.Color.Fill.Quiet, p.PlainBgHover, p.BgPressed
		bg.A = 0
	case ButtonDanger:
		bg, hover, pressed, fg = p.DangerBg, p.DangerBgHover, p.DangerBgPressed, p.DangerText
	case ButtonDangerPlain:
		bg, hover, pressed, fg = p.DangerPlainBgHover, p.DangerPlainBgHover, p.DangerPlainBgHover, p.DangerPlainText
		bg.A = 0
	}
	if !disabled {
		if e.Pressed() {
			bg = pressed
		} else if e.Hovered() {
			bg = hover
		}
	}
	if props.Variant == ButtonSecondary {
		e.Border(p.BorderWidth, Color(p.Border))
	}
	e.Background(e.AnimateColor("background", Color(bg), Tween(p.Transition))).TextColor(Color(fg))
	if e.FocusVisible() && !disabled {
		Shadow(e, p.FocusShadow)
	}
	if props.Kbd != "" {
		e.Description(label + " " + props.Kbd)
	}
	e.Children(func() {
		if props.Loading {
			Spinner(c, SpinnerProps{Size: SizeSm, Tone: buttonContentTone(props.Variant), Label: locales.MessageProcessing2(), Motion: props.Motion}).PassThrough()
		} else if props.Icon != "" {
			ui.Icon(c, IconSVG(props.Icon)).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize).PassThrough()
		} else if props.Leading != nil {
			ui.Icon(c, props.Leading).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize).PassThrough()
		}
		if label != "" {
			ui.Text(c, label).SingleLine().PassThrough()
		}
		if props.Kbd != "" {
			tone := KbdKeycap
			if props.Variant == ButtonPrimary || props.Variant == ButtonDanger {
				tone = KbdOnAccent
			}
			Kbd(c, props.Kbd, KbdProps{Tone: tone}).MarginStart(p.KbdGap - p.Gap)
		}
	})
	return StyleDisabled(e, disabled)
}

func buttonContentTone(variant ButtonVariant) Tone {
	if variant == ButtonPrimary || variant == ButtonDanger {
		return ToneOnAccent
	}
	if variant == ButtonDangerPlain {
		return ToneDanger
	}
	return ToneNeutral
}

func StyleDisabled(e ui.Element, disabled bool) ui.Element {
	e.Disabled(false)
	if disabled && !e.IsDisabled() {
		e.Disabled(true)
		// MyGo v0.7.3 already paints Disabled at 0.5; compensate to the token's total.
		// A disabled ancestor owns the fade for its entire subtree.
		e.Opacity(theme.Current().Opacity.Disabled / 0.5)
	}
	return e
}

func StyleIconButton(e ui.Element, t *theme.Theme) ui.Element {
	return styleIconButton(e, t, IconPlain, false)
}

func styleIconButton(e ui.Element, t *theme.Theme, variant IconVariant, on bool) ui.Element {
	p := t.C.IconButton
	bg, ink := ui.Transparent, Color(p.Icon)
	if variant == IconFilled {
		bg = Color(p.BgFilled)
	}
	if on {
		bg, ink = Color(p.BgOn), Color(p.IconOn)
	}
	hover, pressed := Color(p.BgHover), Color(p.BgPressed)
	if variant == IconOnWindow {
		hover = Color(t.Color.State.OnWindow)
		if runtime.GOOS == "windows" {
			hover, pressed = Color(t.C.Desktop.RailHover), Color(t.C.Desktop.RailPressed)
			if on {
				bg, ink = Color(t.Color.Accent.Tint10), Color(t.Color.Accent.Default)
				pressed = Color(t.Color.Accent.Tint15)
			}
		}
	}
	if !e.IsDisabled() {
		switch {
		case e.Pressed():
			bg = pressed
		case e.Hovered():
			if !on {
				bg = hover
				ink = Color(p.IconHover)
			}
		}
	}
	e.Size(p.Size, p.Size).Radius(p.Radius).FocusRing(false).
		TextColor(e.AnimateColor("icon", ink, Tween(p.Transition))).
		Background(e.AnimateColor("background", bg, Tween(p.Transition)))
	if e.FocusVisible() && !e.IsDisabled() {
		Shadow(e, p.FocusShadow)
	}
	return e
}

func StyleSegment(e ui.Element, p theme.SegmentedTokens, selected bool, showPill ...bool) ui.Element {
	pill := len(showPill) == 0 || showPill[0]
	bg, fg, shadow := ui.Transparent, Color(p.Text), float32(0)
	if selected {
		fg = Color(p.TextOn)
		if pill {
			bg, shadow = Color(p.BgOn), 1
		}
	} else if e.Hovered() {
		bg = Color(p.BgHover)
	}
	animation := Tween(p.Transition)
	e.TextColor(e.AnimateColor("text", fg, animation))
	if selected && pill {
		// On initial mount before the track measures bounds, paint synchronously
		// so switching to the track's sliding pill leaves no fading background behind.
		e.Background(bg)
		return ShadowOpacity(e, p.ShadowOn, 1)
	}
	e.Background(e.AnimateColor("background", bg, animation))
	return ShadowOpacity(e, p.ShadowOn, Visual(e, "shadow", shadow, p.Transition))
}
