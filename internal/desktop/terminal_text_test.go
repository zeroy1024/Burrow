package desktop

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func TestTerminalPaletteAndTruecolorRemainSemantic(t *testing.T) {
	var state terminalTextState
	var output strings.Builder
	for index := range 256 {
		fmt.Fprintf(&output, "\x1b[38;5;%dmx", index)
	}
	output.WriteString("\x1b[38;2;12;34;56mx\x1b[0mx")
	lines, bell := state.parse(output.String())
	if bell || len(lines) != 1 || len(lines[0]) != 258 {
		t.Fatalf("unexpected parsed output: %d lines, bell=%v", len(lines), bell)
	}
	light, _ := termtheme.Lookup(termtheme.DefaultLight)
	dark, _ := termtheme.Lookup(termtheme.DefaultDark)
	prefs := termtheme.DefaultPrefs()
	for index, run := range lines[0][:256] {
		if run.Foreground.kind != terminalPalette || int(run.Foreground.index) != index {
			t.Fatalf("color %d lost its palette index: %+v", index, run.Foreground)
		}
		for _, scheme := range []*termtheme.Scheme{light, dark} {
			fg, _ := terminalSpanColors(run, scheme, prefs)
			if fg != tokenColor(scheme.Color(index)) {
				t.Fatalf("color %d did not resolve against %s", index, scheme.Name)
			}
		}
	}
	truecolor := lines[0][256]
	if truecolor.Foreground.kind != terminalRGB || truecolor.Foreground.rgb != 0x0c2238 {
		t.Fatalf("truecolor was not preserved: %+v", truecolor.Foreground)
	}
	for _, scheme := range []*termtheme.Scheme{light, dark} {
		fg, _ := terminalSpanColors(truecolor, scheme, prefs)
		if fg != (ui.Color{R: 12, G: 34, B: 56, A: 255}) {
			t.Fatalf("%s changed truecolor: %+v", scheme.Name, fg)
		}
		fg, _ = terminalSpanColors(lines[0][257], scheme, prefs)
		if fg != tokenColor(scheme.Foreground) {
			t.Fatalf("%s did not resolve the default foreground", scheme.Name)
		}
	}
}

func TestTerminalSGRAttributesAcrossLines(t *testing.T) {
	var state terminalTextState
	lines, _ := state.parse("\x1b[1;2;4;7;31;44mfirst\nsecond\n")
	if len(lines) != 2 || lines[0][0].Foreground.index != 1 || lines[0][0].Background.index != 4 {
		t.Fatalf("incorrect basic SGR colors: %+v", lines)
	}
	for _, line := range lines {
		run := line[0]
		if !run.Bold || !run.Dim || !run.Underline || !run.Reverse {
			t.Fatalf("attributes did not survive a newline: %+v", run)
		}
	}
	next, _ := state.parse("next\x1b[22;24;27;39;49mdefault")
	if !next[0][0].Bold || next[0][1] != (terminalSpan{Text: "default"}) {
		t.Fatalf("SGR state or reset was lost across calls: %+v", next)
	}
	bright, _ := state.parse("\x1b[90;107mx\x1b[97;100my")
	if bright[0][0].Foreground.index != 8 || bright[0][0].Background.index != 15 ||
		bright[0][1].Foreground.index != 15 || bright[0][1].Background.index != 8 {
		t.Fatalf("incorrect bright SGR colors: %+v", bright)
	}
	invalid, _ := state.parse("\x1b[38;5;256mA\x1b[48;2;1;2;999mB")
	if invalid[0][0].Foreground.index != 15 || invalid[0][1].Background.index != 8 {
		t.Fatalf("invalid extended colors changed attributes: %+v", invalid)
	}
}

func TestTerminalBoldDimReverseAndBackground(t *testing.T) {
	scheme, _ := termtheme.Lookup(termtheme.DefaultLight)
	prefs := termtheme.DefaultPrefs()
	prefs.BoldIsBright = true
	run := terminalSpan{Foreground: terminalIndexed(1), Bold: true}
	fg, _ := terminalSpanColors(run, scheme, prefs)
	if fg != tokenColor(scheme.ANSI[9]) {
		t.Fatalf("bold ANSI color did not brighten: %+v", fg)
	}
	bold := color.NRGBA{R: 18, G: 52, B: 86, A: 255}
	scheme.Bold = &bold
	fg, _ = terminalSpanColors(run, scheme, prefs)
	if fg != tokenColor(bold) {
		t.Fatalf("explicit bold color was ignored: %+v", fg)
	}
	run.Background, run.Reverse, run.Dim = terminalIndexed(4), true, true
	fg, bg := terminalSpanColors(run, scheme, prefs)
	if fg != tokenColor(scheme.ANSI[4]).Alpha(0.5) || bg != tokenColor(bold) {
		t.Fatalf("reverse and dim resolved incorrectly: fg=%+v bg=%+v", fg, bg)
	}
	for _, source := range []termtheme.BackgroundSource{termtheme.BackgroundScheme, termtheme.BackgroundCard} {
		prefs.Background = source
		run = terminalSpan{Reverse: true}
		fg, bg = terminalSpanColors(run, scheme, prefs)
		expected := termtheme.TerminalBackground(scheme, theme.Current().C.Pane.Bg, source)
		if fg != tokenColor(expected) || bg != tokenColor(scheme.Foreground) {
			t.Fatalf("reverse did not use background source %s", source)
		}
	}
}

func TestTerminalOSCLinksAndBell(t *testing.T) {
	var state terminalTextState
	lines, bell := state.parse("\x1b]8;;https://example.com\alink\x1b[0m stays linked\x1b]8;;\x1b\\ plain\a")
	if !bell || len(lines) != 1 || len(lines[0]) != 3 {
		t.Fatalf("incorrect OSC/BEL parsing: %+v bell=%v", lines, bell)
	}
	if lines[0][0].Link != "https://example.com" || lines[0][1].Link != "https://example.com" ||
		lines[0][2].Link != "" {
		t.Fatalf("OSC link state did not open/reset/close correctly: %+v", lines[0])
	}
	_, bell = state.parse("\x1b]8;;https://example.com\aquiet\x1b]8;;\a")
	if bell {
		t.Fatal("OSC terminators were treated as activity bells")
	}
	scheme, _ := termtheme.Lookup(termtheme.DefaultLight)
	prefs := termtheme.DefaultPrefs()
	spans := terminalSpans(lines[0], scheme, prefs, 0, 0)
	if !spans[0].Underline || !spans[1].Underline || spans[2].Underline {
		t.Fatalf("link underlines did not follow OSC state: %+v", spans)
	}
	link := color.NRGBA{R: 1, G: 2, B: 3, A: 255}
	scheme.Link = &link
	fg, _ := terminalSpanColors(lines[0][0], scheme, prefs)
	if fg != tokenColor(link) {
		t.Fatal("explicit link color was ignored")
	}
}

func TestTerminalSelectionUsesRuneOffsets(t *testing.T) {
	scheme, _ := termtheme.Lookup(termtheme.DefaultLight)
	line := terminalLine{
		{Text: "a你b", Foreground: terminalIndexed(1)},
		{Text: "好c", Foreground: terminalIndexed(2)},
	}
	spans := terminalSpans(line, scheme, termtheme.DefaultPrefs(), 1, 4)
	if len(spans) != 4 || spans[0].Text != "a" || spans[1].Text != "你b" ||
		spans[2].Text != "好" || spans[3].Text != "c" {
		t.Fatalf("selection split byte offsets instead of rune offsets: %+v", spans)
	}
	for _, index := range []int{1, 2} {
		if spans[index].Color != tokenColor(scheme.SelectionFg) ||
			spans[index].Background != tokenColor(scheme.SelectionBg) {
			t.Fatalf("selected span %d did not use the scheme colors", index)
		}
	}
	if spans[0].Color != tokenColor(scheme.ANSI[1]) || spans[3].Color != tokenColor(scheme.ANSI[2]) {
		t.Fatal("selection changed colors outside its range")
	}
}
