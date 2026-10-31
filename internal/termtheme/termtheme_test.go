package termtheme

import (
	"bytes"
	"image/color"
	"strings"
	"testing"
)

func TestBundled(t *testing.T) {
	for _, n := range []string{DefaultLight, DefaultDark, "Dracula", "Nord"} {
		if _, ok := Lookup(n); !ok {
			t.Fatalf("missing bundled scheme %q (have %v)", n, Names())
		}
	}
	l, _ := Lookup(DefaultLight)
	if Hex(l.Background) != "#fbfbfc" || Hex(l.Foreground) != "#4c4f69" || Hex(l.ANSI[4]) != "#1e66f5" {
		t.Fatalf("latte: %s %s %s", Hex(l.Background), Hex(l.Foreground), Hex(l.ANSI[4]))
	}
	d, _ := Lookup(DefaultDark)
	if Hex(d.Background) != "#1e1e2e" || Hex(d.ANSI[1]) != "#f38ba8" || !d.IsDark() || l.IsDark() {
		t.Fatalf("mocha: %s %s", Hex(d.Background), Hex(d.ANSI[1]))
	}
}

func TestParseFallbacksAndUnknown(t *testing.T) {
	src := "# a comment\nfont-size = 14\nbackground = 101010\nforeground = \"#eeeeee\"\npalette = 1=#f00\npalette = 200=#123456\nnot a pair\n"
	s, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if Hex(s.Background) != "#101010" || Hex(s.ANSI[1]) != "#ff0000" {
		t.Fatal("parse", Hex(s.Background), Hex(s.ANSI[1]))
	}
	if s.Cursor != s.Foreground || s.CursorText != s.Background || s.SelectionBg != s.Foreground || s.SelectionFg != s.Background {
		t.Fatal("fallbacks")
	}
	if s.ANSI[2] != XtermANSI[2] || Hex(s.Color(200)) != "#123456" {
		t.Fatal("ansi fallback / extended")
	}
	if _, err := Parse(strings.NewReader("background = nope\n")); err == nil {
		t.Fatal("want error for bad color")
	}
}

func TestRoundTrip(t *testing.T) {
	a, _ := Lookup(DefaultDark)
	blue := color.NRGBA{0, 0, 255, 255}
	a.Link = &blue
	var buf bytes.Buffer
	if err := Format(&buf, a); err != nil {
		t.Fatal(err)
	}
	b, err := Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != a.Name || b.ANSI != a.ANSI || b.Background != a.Background || b.SelectionBg != a.SelectionBg || *b.Link != blue {
		t.Fatal("round trip mismatch")
	}
}

func TestColor256(t *testing.T) {
	s, _ := Lookup(DefaultLight)
	cases := map[int]string{3: Hex(s.ANSI[3]), 16: "#000000", 21: "#0000ff", 196: "#ff0000", 231: "#ffffff", 232: "#080808", 244: "#808080", 255: "#eeeeee"}
	for i, want := range cases {
		if got := Hex(s.Color(i)); got != want {
			t.Errorf("Color(%d)=%s want %s", i, got, want)
		}
	}
}

func TestModeAndSwitch(t *testing.T) {
	if ParseMode(Fixed("Nord").String()) != Fixed("Nord") || ParseMode("x") != FollowAppTheme() {
		t.Fatal("mode encoding")
	}
	if Resolve(FollowAppTheme(), true).Name != DefaultDark || Resolve(FollowAppTheme(), false).Name != DefaultLight {
		t.Fatal("follow")
	}
	if Resolve(Fixed("Nord"), false).Name != "Nord" || Resolve(Fixed("missing"), true).Name != DefaultDark {
		t.Fatal("fixed")
	}
	got := ""
	cancel := OnChange(func(s *Scheme) { got = s.Name })
	Apply(Fixed("Dracula"), false)
	if got != "Dracula" || Current().Name != "Dracula" {
		t.Fatal("listener / current")
	}
	cancel()
	Apply(FollowAppTheme(), false)
	if got != "Dracula" || Current().Name != DefaultLight {
		t.Fatal("cancel")
	}
	// The terminal fill is always the scheme's own background.
	if Hex(Resolve(FollowAppTheme(), true).Background) != "#1e1e2e" {
		t.Fatal("dark follow should fill with Mocha #1e1e2e")
	}
	if Hex(Resolve(FollowAppTheme(), false).Background) != "#fbfbfc" {
		t.Fatal("light follow should fill with Latte override #fbfbfc")
	}
	if Hex(Resolve(Fixed("Dracula"), false).Background) != "#282a36" {
		t.Fatal("fixed Dracula in a light app should fill with Dracula bg")
	}
	// 终端背景 option.
	card := color.NRGBA{0x20, 0x20, 0x22, 0xff}
	mocha := Resolve(FollowAppTheme(), true)
	if TerminalBackground(mocha, card, DefaultPrefs().Background) != mocha.Background {
		t.Fatal("default should be scheme background")
	}
	if TerminalBackground(mocha, card, BackgroundCard) != card {
		t.Fatal("card option should use card color")
	}
	if ParseBackgroundSource(BackgroundCard.String()) != BackgroundCard || ParseBackgroundSource("x") != BackgroundScheme {
		t.Fatal("background encoding")
	}
}
