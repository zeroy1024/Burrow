package termtheme

import (
	"embed"
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed schemes/*
var bundled embed.FS

// Names of the bundled default schemes.
const (
	DefaultLight = "Catppuccin Latte" // background overridden to #fbfbfc (= terminal card)
	DefaultDark  = "Catppuccin Mocha"
)

// ModeKind says how the terminal scheme is chosen.
type ModeKind int

const (
	// FollowApp uses DefaultLight / DefaultDark (or the configured pair)
	// based on the app appearance.
	FollowApp ModeKind = iota
	// FixedScheme always uses one named scheme.
	FixedScheme
)

// Mode is the user's 终端配色 setting.
type Mode struct {
	Kind ModeKind
	Name string // scheme name when Kind == FixedScheme
}

// FollowAppTheme returns the "跟随主题" mode.
func FollowAppTheme() Mode { return Mode{Kind: FollowApp} }

// Fixed returns a mode that always uses the named scheme.
func Fixed(name string) Mode { return Mode{Kind: FixedScheme, Name: name} }

// String encodes the mode for settings storage: "follow" or "fixed:<name>".
func (m Mode) String() string {
	if m.Kind == FixedScheme {
		return "fixed:" + m.Name
	}
	return "follow"
}

// ParseMode decodes Mode.String(); anything unknown means FollowAppTheme.
func ParseMode(s string) Mode {
	if name, ok := strings.CutPrefix(s, "fixed:"); ok && name != "" {
		return Fixed(name)
	}
	return FollowAppTheme()
}

var (
	regMu        sync.RWMutex
	registry     = map[string]*Scheme{}
	bundledNames = map[string]bool{}
	pairMu       sync.RWMutex
	lightDef     = DefaultLight
	darkDef      = DefaultDark
)

func init() {
	files, err := fs.Glob(bundled, "schemes/*")
	if err != nil {
		panic(err)
	}
	for _, p := range files {
		f, err := bundled.Open(p)
		if err != nil {
			panic(err)
		}
		s, err := Parse(f)
		f.Close()
		if err != nil {
			panic(fmt.Sprintf("termtheme: bundled %s: %v", p, err))
		}
		if s.Name == "" {
			s.Name = path.Base(p)
		}
		registry[s.Name] = s
		bundledNames[s.Name] = true
	}
	cur = registry[DefaultLight]
}

// Register adds or replaces a scheme by name (e.g. after importing a file).
func Register(s *Scheme) {
	if s == nil || s.Name == "" {
		return
	}
	regMu.Lock()
	registry[s.Name] = s.Clone()
	regMu.Unlock()
}

// IsBundled reports whether the app ships this scheme.
func IsBundled(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	return bundledNames[name]
}

// Unregister removes an imported scheme after the caller removes its persisted file.
// The caller must first check settings and session references.
func Unregister(name string) error {
	regMu.Lock()
	defer regMu.Unlock()
	if bundledNames[name] {
		return fmt.Errorf("cannot remove bundled scheme %q", name)
	}
	if _, ok := registry[name]; !ok {
		return fmt.Errorf("unknown scheme %q", name)
	}
	delete(registry, name)
	return nil
}

// Import loads a Ghostty theme file and registers it. The caller should
// copy the file into the app's themes directory so it survives restarts,
// then call Import on every file there at startup.
func Import(path string) (*Scheme, error) {
	s, err := Load(path)
	if err != nil {
		return nil, err
	}
	Register(s)
	return s, nil
}

// Lookup returns a copy of the named scheme.
func Lookup(name string) (*Scheme, bool) {
	regMu.RLock()
	s, ok := registry[name]
	regMu.RUnlock()
	if !ok {
		return nil, false
	}
	return s.Clone(), true
}

// Names lists registered schemes, sorted.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// SetFollowPair changes which schemes FollowAppTheme uses.
func SetFollowPair(light, dark string) {
	pairMu.Lock()
	lightDef, darkDef = light, dark
	pairMu.Unlock()
}

// Resolve picks the scheme for a mode and app appearance. A fixed scheme
// that is not registered falls back to the follow pair.
func Resolve(m Mode, appDark bool) *Scheme {
	if m.Kind == FixedScheme {
		if s, ok := Lookup(m.Name); ok {
			return s
		}
	}
	pairMu.RLock()
	name, fallback := lightDef, DefaultLight
	if appDark {
		name, fallback = darkDef, DefaultDark
	}
	pairMu.RUnlock()
	if s, ok := Lookup(name); ok {
		return s
	}
	s, _ := Lookup(fallback)
	return s
}

// Apply resolves the mode and makes it current. Call it at startup, when
// the setting changes and when the app appearance changes.
func Apply(m Mode, appDark bool) *Scheme {
	s := Resolve(m, appDark)
	Set(s)
	return s
}

var (
	curMu     sync.RWMutex
	cur       *Scheme
	listeners = map[int]func(*Scheme){}
	nextID    int
)

// Current returns the active scheme. Treat it as read-only; the renderer
// should call it every frame instead of caching colors.
func Current() *Scheme {
	curMu.RLock()
	defer curMu.RUnlock()
	return cur
}

// Set makes s the active scheme and notifies listeners (outside the lock).
func Set(s *Scheme) {
	if s == nil {
		return
	}
	s = s.Clone()
	curMu.Lock()
	cur = s
	fns := make([]func(*Scheme), 0, len(listeners))
	for _, f := range listeners {
		fns = append(fns, f)
	}
	curMu.Unlock()
	for _, f := range fns {
		f(s)
	}
}

// OnChange registers fn to run after every Set (use it to request a
// redraw). The returned func unregisters it.
func OnChange(fn func(*Scheme)) (cancel func()) {
	curMu.Lock()
	id := nextID
	nextID++
	listeners[id] = fn
	curMu.Unlock()
	return func() {
		curMu.Lock()
		delete(listeners, id)
		curMu.Unlock()
	}
}

// BackgroundSource is the 终端背景 setting: what fills the terminal area
// inside the pane card. The pane's top/bottom fade bars must tint with the
// same color.
type BackgroundSource int

const (
	// BackgroundScheme (default) always uses the scheme's own Background,
	// in every mode and app appearance.
	BackgroundScheme BackgroundSource = iota
	// BackgroundCard always uses the app's pane card color (theme C.Pane.Bg).
	BackgroundCard
)

// String encodes the setting for storage: "scheme" or "card".
func (b BackgroundSource) String() string {
	if b == BackgroundCard {
		return "card"
	}
	return "scheme"
}

// ParseBackgroundSource decodes BackgroundSource.String(); anything unknown
// means BackgroundScheme.
func ParseBackgroundSource(v string) BackgroundSource {
	if v == "card" {
		return BackgroundCard
	}
	return BackgroundScheme
}

// TerminalBackground returns the color that fills the terminal area (and
// tints the pane fade bars): s.Background for BackgroundScheme, card for
// BackgroundCard. Pass theme.Current().C.Pane.Bg as card.
func TerminalBackground(s *Scheme, card color.NRGBA, src BackgroundSource) color.NRGBA {
	if src == BackgroundCard {
		return card
	}
	return s.Background
}

// CursorShape is the terminal cursor shape.
type CursorShape int

const (
	CursorBlock CursorShape = iota
	CursorBar
	CursorUnderline
)

// Cursor metrics belong to terminal rendering, independently of app controls.
const (
	CursorHeightPadding float32 = 3.5
	CursorRadius        float32 = 1
	CursorBottomInset   float32 = 1.5
	CursorOutlineWidth  float32 = 1.2
	CursorBarWidth      float32 = 1.5
	CursorUnderlineSize float32 = 2
	CursorBlinkInterval         = 500 * time.Millisecond
)

// Prefs are terminal font/cursor preferences. They are not part of a
// scheme and do not change when the scheme changes.
type Prefs struct {
	FontFamily  []string // fallback chain
	FontSize    float32  // logical px
	LineHeight  float32  // logical px
	Tracking    float32  // letter spacing, logical px
	CursorShape CursorShape
	CursorBlink bool
	// Background is the 终端背景 option (default BackgroundScheme).
	Background BackgroundSource
	// BoldIsBright renders bold text in ANSI 0–7 with the bright 8–15
	// color (xterm behaviour).
	BoldIsBright bool
}

// DefaultPrefs matches the terminal text in the mockups (ssh-v6.html:
// JetBrains Mono 12.5px / 19px line, -0.1px tracking).
func DefaultPrefs() Prefs {
	return Prefs{
		FontFamily:  []string{"JetBrains Mono", "Menlo", "monospace"},
		FontSize:    12.5,
		LineHeight:  19,
		Tracking:    -0.1,
		CursorShape: CursorBlock,
		CursorBlink: true,
		Background:  BackgroundScheme,
	}
}

func pow(x, y float64) float64 { return math.Pow(x, y) }
