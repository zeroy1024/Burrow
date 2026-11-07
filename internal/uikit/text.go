package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type TextStyle uint8

const (
	TextBody TextStyle = iota
	TextBodyStrong
	TextHeadline
	TextTitle
	TextSmall
	TextFootnote
	TextCaption
	TextMono
	TextNumeric
	// TextLabel names a figure or a group in a dense panel.
	TextLabel
	TextCaption2
)

type TextProps struct {
	Style TextStyle
	Tone  Tone
	// Mono sets the style's size in the monospaced UI family.
	Mono bool
}

// TextFont shares the typography with rich-text slots and measured labels.
func TextFont(style TextStyle) ui.Font {
	t := theme.Current()
	size, weight := t.Font.Size.Body, t.Font.Weight.Regular
	switch style {
	case TextBodyStrong:
		weight = t.Font.Weight.Medium
	case TextHeadline:
		size, weight = t.Font.Size.BodyLg, t.Font.Weight.Semibold
	case TextTitle:
		size, weight = t.Font.Size.Title, t.Font.Weight.Semibold
	case TextSmall:
		size = t.Font.Size.Small
	case TextFootnote, TextMono:
		size = t.Font.Size.Footnote
	case TextCaption:
		size = t.Font.Size.Caption
	case TextLabel:
		size, weight = t.Font.Size.Small, t.Font.Weight.Medium
	case TextCaption2:
		size = t.Font.Size.Caption2
	}
	family := t.Font.Family.Ui
	if style == TextMono {
		family = t.Font.Family.Mono
	}
	return ui.Font{Family: strings.Join(family, ", "), Size: size, Weight: weight}
}

func Text(c *ui.Context, text string, props TextProps) ui.Element {
	font := TextFont(props.Style)
	e := ui.Text(c, text).FontSize(font.Size).FontWeight(font.Weight).TextColor(ToneColor(props.Tone))
	if props.Style == TextMono || props.Mono {
		e.Font(strings.Join(theme.Current().Font.Family.Mono, ", "))
	}
	if props.Style == TextNumeric {
		e.FontFeatures("tnum")
	}
	return e
}

func ToneColor(tone Tone) ui.Color {
	t := theme.Current()
	ink := t.Color.Text.Primary
	switch tone {
	case ToneSecondary:
		ink = t.Color.Text.Secondary
	case ToneTertiary:
		ink = t.Color.Text.Tertiary
	case ToneAccent:
		ink = t.Color.Text.Accent
	case ToneSuccess:
		ink = t.Color.Success.Text
	case ToneWarning:
		ink = t.Color.Warning.TextStrong
	case ToneDanger:
		ink = t.Color.Danger.Text
	case ToneMuted:
		ink = t.Color.Text.Muted
	case ToneOnAccent:
		ink = t.Color.Text.OnAccent
	}
	return Color(ink)
}
