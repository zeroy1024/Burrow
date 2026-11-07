package desktop

import (
	"strconv"
	"strings"
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
	terminalRGB
)

// Buffer colors retain their source so changing the scheme also recolors history.
// RGB stores only SGR truecolor input, never a resolved palette entry.
type terminalColor struct {
	kind  terminalColorKind
	index uint8
	rgb   uint32
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

type terminalTextState struct {
	style terminalSpan
}

// parse handles the prototype's complete output lines, including SGR and OSC 8.
// Cursor movement and screen editing belong to a future terminal emulator.
func (state *terminalTextState) parse(text string) ([]terminalLine, bool) {
	lines := []terminalLine{}
	line := terminalLine{}
	var pending strings.Builder
	bell := false
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		span := state.style
		span.Text = pending.String()
		line = append(line, span)
		pending.Reset()
	}
	ended := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	for i := 0; i < len(text); {
		switch text[i] {
		case '\n':
			flush()
			lines = append(lines, line)
			line = terminalLine{}
			i++
		case '\a':
			bell = true
			i++
		case '\r':
			i++
		case '\x1b':
			flush()
			i = state.escape(text, i)
		default:
			pending.WriteByte(text[i])
			i++
		}
	}
	flush()
	if len(line) > 0 || strings.ContainsRune(text, '\n') || ended {
		lines = append(lines, line)
	}
	return lines, bell
}

func (state *terminalTextState) escape(text string, start int) int {
	if start+1 >= len(text) {
		return len(text)
	}
	switch text[start+1] {
	case '[':
		for end := start + 2; end < len(text); end++ {
			if text[end] >= 0x40 && text[end] <= 0x7e {
				if text[end] == 'm' {
					state.sgr(text[start+2 : end])
				}
				return end + 1
			}
		}
	case ']':
		for end := start + 2; end < len(text); end++ {
			length := 0
			if text[end] == '\a' {
				length = 1
			}
			if text[end] == '\x1b' && end+1 < len(text) && text[end+1] == '\\' {
				length = 2
			}
			if length != 0 {
				if command, ok := strings.CutPrefix(text[start+2:end], "8;"); ok {
					if _, link, ok := strings.Cut(command, ";"); ok {
						state.style.Link = link
					}
				}
				return end + length
			}
		}
	default:
		return start + 2
	}
	return len(text)
}

func (state *terminalTextState) sgr(sequence string) {
	parts := strings.Split(sequence, ";")
	values := make([]int, len(parts))
	for i, part := range parts {
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return
		}
		values[i] = value
	}
	for i := 0; i < len(values); i++ {
		value := values[i]
		switch {
		case value == 0:
			link := state.style.Link
			state.style = terminalSpan{Link: link}
		case value == 1:
			state.style.Bold = true
		case value == 2:
			state.style.Dim = true
		case value == 4:
			state.style.Underline = true
		case value == 7:
			state.style.Reverse = true
		case value == 22:
			state.style.Bold, state.style.Dim = false, false
		case value == 24:
			state.style.Underline = false
		case value == 27:
			state.style.Reverse = false
		case value == 39:
			state.style.Foreground = terminalColor{}
		case value == 49:
			state.style.Background = terminalColor{}
		case value >= 30 && value <= 37:
			state.style.Foreground = terminalIndexed(uint8(value - 30))
		case value >= 40 && value <= 47:
			state.style.Background = terminalIndexed(uint8(value - 40))
		case value >= 90 && value <= 97:
			state.style.Foreground = terminalIndexed(uint8(value - 90 + 8))
		case value >= 100 && value <= 107:
			state.style.Background = terminalIndexed(uint8(value - 100 + 8))
		case value == 38 || value == 48:
			color, consumed, valid := terminalExtendedColor(values[i+1:])
			i += consumed
			if valid {
				if value == 38 {
					state.style.Foreground = color
				} else {
					state.style.Background = color
				}
			}
		}
	}
}

func terminalExtendedColor(values []int) (terminalColor, int, bool) {
	if len(values) == 0 {
		return terminalColor{}, 0, false
	}
	switch values[0] {
	case 5:
		if len(values) < 2 {
			return terminalColor{}, len(values), false
		}
		if values[1] > 255 {
			return terminalColor{}, 2, false
		}
		return terminalIndexed(uint8(values[1])), 2, true
	case 2:
		if len(values) < 4 {
			return terminalColor{}, len(values), false
		}
		if values[1] > 255 || values[2] > 255 || values[3] > 255 {
			return terminalColor{}, 4, false
		}
		rgb := uint32(values[1])<<16 | uint32(values[2])<<8 | uint32(values[3])
		return terminalColor{kind: terminalRGB, rgb: rgb}, 4, true
	}
	return terminalColor{}, 1, false
}

func (source terminalColor) resolve(s *termtheme.Scheme, defaultColor ui.Color) ui.Color {
	switch source.kind {
	case terminalPalette:
		return uikit.Color(s.Color(int(source.index)))
	case terminalRGB:
		return ui.Color{R: uint8(source.rgb >> 16), G: uint8(source.rgb >> 8), B: uint8(source.rgb), A: 255}
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
