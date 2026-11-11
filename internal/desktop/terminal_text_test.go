package desktop

import (
	"github.com/zeroy1024/Burrow/internal/uikit"
	"image/color"
	"testing"

	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
)

func TestTerminalPaletteRemainsSemantic(t *testing.T) {
	light, _ := termtheme.Lookup(termtheme.DefaultLight)
	dark, _ := termtheme.Lookup(termtheme.DefaultDark)
	prefs := termtheme.DefaultPrefs()
	for _, scheme := range []*termtheme.Scheme{light, dark} {
		for index := range 256 {
			fg, _ := terminalSpanColors(terminalSpan{Foreground: terminalIndexed(uint8(index))}, scheme, prefs)
			if fg != uikit.Color(scheme.Color(index)) {
				t.Fatalf("color %d did not resolve against %s", index, scheme.Name)
			}
		}
		fg, _ := terminalSpanColors(terminalSpan{}, scheme, prefs)
		if fg != uikit.Color(scheme.Foreground) {
			t.Fatalf("%s did not resolve the default foreground", scheme.Name)
		}
	}
}

func TestTerminalBoldDimReverseAndBackground(t *testing.T) {
	scheme, _ := termtheme.Lookup(termtheme.DefaultLight)
	prefs := termtheme.DefaultPrefs()
	prefs.BoldIsBright = true
	run := terminalSpan{Foreground: terminalIndexed(1), Bold: true}
	fg, _ := terminalSpanColors(run, scheme, prefs)
	if fg != uikit.Color(scheme.ANSI[9]) {
		t.Fatalf("bold ANSI color did not brighten: %+v", fg)
	}
	bold := color.NRGBA{R: 18, G: 52, B: 86, A: 255}
	scheme.Bold = &bold
	fg, _ = terminalSpanColors(run, scheme, prefs)
	if fg != uikit.Color(bold) {
		t.Fatalf("explicit bold color was ignored: %+v", fg)
	}
	run.Background, run.Reverse, run.Dim = terminalIndexed(4), true, true
	fg, bg := terminalSpanColors(run, scheme, prefs)
	if fg != uikit.Color(scheme.ANSI[4]).Alpha(0.5) || bg != uikit.Color(bold) {
		t.Fatalf("reverse and dim resolved incorrectly: fg=%+v bg=%+v", fg, bg)
	}
	for _, source := range []termtheme.BackgroundSource{termtheme.BackgroundScheme, termtheme.BackgroundCard} {
		prefs.Background = source
		run = terminalSpan{Reverse: true}
		fg, bg = terminalSpanColors(run, scheme, prefs)
		expected := termtheme.TerminalBackground(scheme, theme.Current().C.Pane.Bg, source)
		if fg != uikit.Color(expected) || bg != uikit.Color(scheme.Foreground) {
			t.Fatalf("reverse did not use background source %s", source)
		}
	}
}

func TestTerminalLinksUnderlineAndUseLinkColor(t *testing.T) {
	line := terminalLine{{Text: "link", Link: "https://example.com"}, {Text: " plain"}}
	scheme, _ := termtheme.Lookup(termtheme.DefaultLight)
	prefs := termtheme.DefaultPrefs()
	spans := terminalSpans(line, scheme, prefs, 0, 0)
	if !spans[0].Underline || spans[1].Underline {
		t.Fatalf("link underlines did not follow the span link: %+v", spans)
	}
	link := color.NRGBA{R: 1, G: 2, B: 3, A: 255}
	scheme.Link = &link
	fg, _ := terminalSpanColors(line[0], scheme, prefs)
	if fg != uikit.Color(link) {
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
		if spans[index].Color != uikit.Color(scheme.SelectionFg) ||
			spans[index].Background != uikit.Color(scheme.SelectionBg) {
			t.Fatalf("selected span %d did not use the scheme colors", index)
		}
	}
	if spans[0].Color != uikit.Color(scheme.ANSI[1]) || spans[3].Color != uikit.Color(scheme.ANSI[2]) {
		t.Fatal("selection changed colors outside its range")
	}
}
