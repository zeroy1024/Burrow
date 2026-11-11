package desktop

import (
	"unicode/utf8"

	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type terminalColorKind uint8

const (
	terminalDefault terminalColorKind = iota
	terminalPalette
)

// Buffer colors retain their source so changing the scheme also recolors history.
type terminalColor struct {
	kind  terminalColorKind
	index uint8
}

func terminalIndexed(index uint8) terminalColor {
	return terminalColor{kind: terminalPalette, index: index}
}

type terminalSpan struct {
	Text                   string
	Foreground, Background terminalColor
	Bold, Dim, Reverse     bool
	Underline              bool
	Link                   string
}

type terminalLine []terminalSpan

func (source terminalColor) resolve(s *termtheme.Scheme, defaultColor ui.Color) ui.Color {
	if source.kind == terminalPalette {
		return uikit.Color(s.Color(int(source.index)))
	}
	return defaultColor
}

func terminalSpanColors(span terminalSpan, scheme *termtheme.Scheme, prefs termtheme.Prefs) (ui.Color, ui.Color) {
	foreground := span.Foreground
	if span.Bold && prefs.BoldIsBright && foreground.kind == terminalPalette && foreground.index < 8 {
		foreground.index += 8
	}
	fg := foreground.resolve(scheme, uikit.Color(scheme.Foreground))
	background := termtheme.TerminalBackground(scheme, theme.Current().C.Pane.Bg, prefs.Background)
	bg := span.Background.resolve(scheme, uikit.Color(background))
	if span.Bold && scheme.Bold != nil {
		fg = uikit.Color(*scheme.Bold)
	}
	if span.Link != "" && scheme.Link != nil {
		fg = uikit.Color(*scheme.Link)
	}
	if span.Reverse {
		fg, bg = bg, fg
	}
	if span.Dim {
		fg = fg.Alpha(0.5)
	}
	return fg, bg
}

func terminalSpans(line terminalLine, scheme *termtheme.Scheme, prefs termtheme.Prefs, start, end int) []ui.Span {
	spans := make([]ui.Span, 0, len(line))
	at := 0
	for _, run := range line {
		fg, bg := terminalSpanColors(run, scheme, prefs)
		span := ui.Span{
			Text: run.Text, Color: fg, Underline: run.Underline || run.Link != "",
		}
		if run.Background.kind != terminalDefault || run.Reverse {
			span.Background = bg
		}
		if run.Bold {
			span.Weight = 700
		}
		length := utf8.RuneCountInString(run.Text)
		from, to := max(0, start-at), min(length, end-at)
		if from >= to {
			spans = append(spans, span)
			at += length
			continue
		}
		runes := []rune(run.Text)
		if from > 0 {
			before := span
			before.Text = string(runes[:from])
			spans = append(spans, before)
		}
		selected := span
		selected.Text, selected.Color = string(runes[from:to]), uikit.Color(scheme.SelectionFg)
		selected.Background = uikit.Color(scheme.SelectionBg)
		spans = append(spans, selected)
		if to < length {
			after := span
			after.Text = string(runes[to:])
			spans = append(spans, after)
		}
		at += length
	}
	return spans
}
