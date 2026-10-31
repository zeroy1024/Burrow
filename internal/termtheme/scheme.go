// Package termtheme holds terminal color schemes for the SSH Tool terminal
// renderer: the background and text colors inside a terminal pane (ANSI 16, 256-color
// table, cursor, selection). It is separate from the app UI tokens (package
// theme), which keep owning the pane container chrome (radius, inner
// hairline, focus ring, title bar). The terminal area inside the card is
// filled with the scheme's own Background by default (Prefs.Background =
// BackgroundScheme) or, if the user picks BackgroundCard, with the app card
// color; see TerminalBackground. The pane's fade bars tint with that color. The package only depends on
// the Go standard library.
//
// Scheme files use the Ghostty theme format, so files exported by
// iTerm2-Color-Schemes (ghostty/ directory) can be imported directly:
//
//	# comment
//	background = #fbfbfc
//	foreground = #4c4f69
//	cursor-color = #dc8a78
//	cursor-text = #fbfbfc
//	selection-background = #d8dae1
//	selection-foreground = #4c4f69
//	palette = 0=#5c5f77
//	...
//	palette = 15=#bcc0cc
//
// Unknown keys are ignored.
package termtheme

import (
	"bufio"
	"bytes"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Scheme is one terminal color scheme.
type Scheme struct {
	Name string
	// Background fills the terminal area (and tints the pane fade bars)
	// unless the user chose BackgroundCard.
	Background  color.NRGBA
	Foreground  color.NRGBA
	Cursor      color.NRGBA
	CursorText  color.NRGBA
	SelectionBg color.NRGBA
	SelectionFg color.NRGBA
	// Bold is the color for bold text when the scheme defines one
	// (Ghostty key "bold-color"); nil means "use the regular color".
	Bold *color.NRGBA
	// Link is the color for detected hyperlinks; nil means the renderer
	// should keep the text color and only underline. Non-standard key
	// "link-color" (Ghostty ignores it).
	Link *color.NRGBA
	// ANSI is the 16-color palette: 0–7 normal, 8–15 bright.
	ANSI [16]color.NRGBA
	// Extended holds optional overrides for palette indexes 16–255
	// ("palette = 123=#rrggbb"). Usually empty.
	Extended map[int]color.NRGBA
	// Comments are the leading "#" comment lines (source/license notes),
	// kept so Format can write them back.
	Comments []string
}

// Default colors used when a file leaves a key out. They match Ghostty's
// built-in defaults; ANSI falls back to the xterm palette.
var (
	defaultBackground = color.NRGBA{0x28, 0x2c, 0x34, 0xff}
	defaultForeground = color.NRGBA{0xff, 0xff, 0xff, 0xff}
)

// XtermANSI is the standard xterm 16-color palette used as fallback.
var XtermANSI = [16]color.NRGBA{
	{0x00, 0x00, 0x00, 0xff}, {0xcd, 0x00, 0x00, 0xff}, {0x00, 0xcd, 0x00, 0xff}, {0xcd, 0xcd, 0x00, 0xff},
	{0x00, 0x00, 0xee, 0xff}, {0xcd, 0x00, 0xcd, 0xff}, {0x00, 0xcd, 0xcd, 0xff}, {0xe5, 0xe5, 0xe5, 0xff},
	{0x7f, 0x7f, 0x7f, 0xff}, {0xff, 0x00, 0x00, 0xff}, {0x00, 0xff, 0x00, 0xff}, {0xff, 0xff, 0x00, 0xff},
	{0x5c, 0x5c, 0xff, 0xff}, {0xff, 0x00, 0xff, 0xff}, {0x00, 0xff, 0xff, 0xff}, {0xff, 0xff, 0xff, 0xff},
}

// Parse reads a Ghostty-format theme. Missing keys get fallbacks:
// background/foreground → Ghostty defaults, cursor-color → foreground,
// cursor-text → background, selection-background → foreground,
// selection-foreground → background (inverse video), ANSI → xterm.
// Name is taken from a "# name: X" comment if present.
func Parse(r io.Reader) (*Scheme, error) {
	s := &Scheme{}
	var have = map[string]bool{}
	var ansiSet [16]bool
	sc := bufio.NewScanner(r)
	ln := 0
	leading := true
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if ln == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if v, ok := cutKey(body, "name"); ok && s.Name == "" {
				s.Name = v
			} else if leading {
				s.Comments = append(s.Comments, body)
			}
			continue
		}
		leading = false
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue // not a key = value line; ignore like unknown keys
		}
		key = strings.TrimSpace(key)
		val = unquote(strings.TrimSpace(val))
		var dst *color.NRGBA
		switch key {
		case "background":
			dst = &s.Background
		case "foreground":
			dst = &s.Foreground
		case "cursor-color":
			dst = &s.Cursor
		case "cursor-text":
			dst = &s.CursorText
		case "selection-background":
			dst = &s.SelectionBg
		case "selection-foreground":
			dst = &s.SelectionFg
		case "bold-color":
			c, err := ParseColor(val)
			if err != nil {
				// Ghostty also accepts "bright" here; treat non-colors as unset.
				continue
			}
			s.Bold = &c
			continue
		case "link-color":
			c, err := ParseColor(val)
			if err != nil {
				return nil, fmt.Errorf("termtheme: line %d: %w", ln, err)
			}
			s.Link = &c
			continue
		case "palette":
			is, cs, ok := strings.Cut(val, "=")
			if !ok {
				return nil, fmt.Errorf("termtheme: line %d: palette needs N=#rrggbb", ln)
			}
			idx, err := strconv.Atoi(strings.TrimSpace(is))
			if err != nil || idx < 0 || idx > 255 {
				return nil, fmt.Errorf("termtheme: line %d: bad palette index %q", ln, is)
			}
			c, err := ParseColor(strings.TrimSpace(cs))
			if err != nil {
				return nil, fmt.Errorf("termtheme: line %d: %w", ln, err)
			}
			if idx < 16 {
				s.ANSI[idx] = c
				ansiSet[idx] = true
			} else {
				if s.Extended == nil {
					s.Extended = map[int]color.NRGBA{}
				}
				s.Extended[idx] = c
			}
			continue
		default:
			continue // unknown key
		}
		c, err := ParseColor(val)
		if err != nil {
			return nil, fmt.Errorf("termtheme: line %d (%s): %w", ln, key, err)
		}
		*dst = c
		have[key] = true
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !have["background"] {
		s.Background = defaultBackground
	}
	if !have["foreground"] {
		s.Foreground = defaultForeground
	}
	if !have["cursor-color"] {
		s.Cursor = s.Foreground
	}
	if !have["cursor-text"] {
		s.CursorText = s.Background
	}
	if !have["selection-background"] {
		s.SelectionBg = s.Foreground
	}
	if !have["selection-foreground"] {
		s.SelectionFg = s.Background
	}
	for i, ok := range ansiSet {
		if !ok {
			s.ANSI[i] = XtermANSI[i]
		}
	}
	return s, nil
}

// Load parses a theme file. When the file has no "# name:" comment the
// file's base name (without extension) is used, as Ghostty does.
func Load(path string) (*Scheme, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Name == "" {
		base := filepath.Base(path)
		if ext := filepath.Ext(base); ext == ".ghostty" || ext == ".conf" {
			base = strings.TrimSuffix(base, ext)
		}
		s.Name = base
	}
	return s, nil
}

// Format writes the scheme in Ghostty format. The output can be read back
// by Parse and by Ghostty itself (except the non-standard link-color).
func Format(w io.Writer, s *Scheme) error {
	var b bytes.Buffer
	if s.Name != "" {
		fmt.Fprintf(&b, "# name: %s\n", s.Name)
	}
	for _, c := range s.Comments {
		fmt.Fprintf(&b, "# %s\n", c)
	}
	for i, c := range s.ANSI {
		fmt.Fprintf(&b, "palette = %d=%s\n", i, Hex(c))
	}
	if len(s.Extended) > 0 {
		keys := make([]int, 0, len(s.Extended))
		for k := range s.Extended {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "palette = %d=%s\n", k, Hex(s.Extended[k]))
		}
	}
	fmt.Fprintf(&b, "background = %s\n", Hex(s.Background))
	fmt.Fprintf(&b, "foreground = %s\n", Hex(s.Foreground))
	fmt.Fprintf(&b, "cursor-color = %s\n", Hex(s.Cursor))
	fmt.Fprintf(&b, "cursor-text = %s\n", Hex(s.CursorText))
	fmt.Fprintf(&b, "selection-background = %s\n", Hex(s.SelectionBg))
	fmt.Fprintf(&b, "selection-foreground = %s\n", Hex(s.SelectionFg))
	if s.Bold != nil {
		fmt.Fprintf(&b, "bold-color = %s\n", Hex(*s.Bold))
	}
	if s.Link != nil {
		fmt.Fprintf(&b, "link-color = %s\n", Hex(*s.Link))
	}
	_, err := w.Write(b.Bytes())
	return err
}

// Write saves the scheme to path in Ghostty format.
func Write(path string, s *Scheme) error {
	var b bytes.Buffer
	if err := Format(&b, s); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// Clone returns a deep copy, so callers can tweak a scheme safely.
func (s *Scheme) Clone() *Scheme {
	c := *s
	if s.Bold != nil {
		b := *s.Bold
		c.Bold = &b
	}
	if s.Link != nil {
		l := *s.Link
		c.Link = &l
	}
	if s.Extended != nil {
		c.Extended = make(map[int]color.NRGBA, len(s.Extended))
		for k, v := range s.Extended {
			c.Extended[k] = v
		}
	}
	c.Comments = append([]string(nil), s.Comments...)
	return &c
}

// Color returns the color for a 256-color palette index: 0–15 from the
// scheme, 16–231 the xterm 6×6×6 cube, 232–255 the xterm grayscale ramp.
// Explicit "palette = N=" overrides win. Out-of-range indexes return the
// foreground.
func (s *Scheme) Color(index int) color.NRGBA {
	if c, ok := s.Extended[index]; ok {
		return c
	}
	switch {
	case index >= 0 && index < 16:
		return s.ANSI[index]
	case index >= 16 && index < 232:
		return Xterm256(index)
	case index >= 232 && index < 256:
		return Xterm256(index)
	}
	return s.Foreground
}

// Xterm256 returns the standard xterm color for index 16–255 (and the
// xterm ANSI color for 0–15).
func Xterm256(index int) color.NRGBA {
	switch {
	case index < 0:
		return XtermANSI[0]
	case index < 16:
		return XtermANSI[index]
	case index < 232:
		i := index - 16
		lv := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + 40*v)
		}
		return color.NRGBA{lv(i / 36), lv(i / 6 % 6), lv(i % 6), 0xff}
	case index < 256:
		g := uint8(8 + 10*(index-232))
		return color.NRGBA{g, g, g, 0xff}
	}
	return XtermANSI[15]
}

// IsDark reports whether the scheme background is dark (relative
// luminance below 0.5).
func (s *Scheme) IsDark() bool { return Luminance(s.Background) < 0.5 }

// Luminance is the WCAG relative luminance of c (alpha ignored).
func Luminance(c color.NRGBA) float64 {
	ch := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// ParseColor accepts "#rrggbb", "rrggbb", "#rgb", "rgb", and
// "#rrggbbaa" / "rrggbbaa".
func ParseColor(v string) (color.NRGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(v), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "ff"
	}
	if len(h) != 8 {
		return color.NRGBA{}, fmt.Errorf("bad color %q", v)
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("bad color %q", v)
	}
	return color.NRGBA{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}

// Hex formats c as "#rrggbb" (or "#rrggbbaa" when not opaque).
func Hex(c color.NRGBA) string {
	if c.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func cutKey(body, key string) (string, bool) {
	for _, sep := range []string{":", "="} {
		if k, v, ok := strings.Cut(body, sep); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}
