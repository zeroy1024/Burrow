package desktop

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
)

func TestTerminalPaneRelayoutAnimatesSurvivor(t *testing.T) {
	a := &app{prefs: appPreferences{ReduceMotion: "off"}}
	a.terminal.prefs = termtheme.DefaultPrefs()
	top := a.newDisconnectedSession(0, host{name: "Shell", port: 22}, false, nil)
	top.config.SSH.TermScheme = termtheme.Fixed(termtheme.DefaultDark).String()
	tab := &workspaceTab{root: &terminalLayout{session: top}, focused: top}
	a.tabs = []*workspaceTab{tab}
	workspace := ui.Point{X: 660, Y: 416}
	var bottom *remoteSession
	var shown, bottomShown ui.Rect
	var split, closeBottom, resizeOnRemoval bool
	tt := ui.NewTester(func(c *ui.Context) {
		if split {
			split = false
			a.splitSession(top, true)
			bottom = tab.root.sessions()[1]
			workspace.Y -= 16
		}
		if closeBottom {
			closeBottom = false
			a.closePane(tab, tab.focused)
		}
		a.updatePaneClosures(c)
		if resizeOnRemoval && tab.root.count() == 1 {
			resizeOnRemoval = false
			workspace.Y += 16
		}
		a.beginWorkspaceFrame(c)
		c.Root().Background(ui.RGB(210, 212, 216))
		root := ui.Column(c).Fill()
		root.Children(func() { a.terminals(c, workspace) })
		root.DrawOver(func(_ *ui.Painter, _ ui.Rect) {
			shown = top.frame.Bounds(c)
			if bottom != nil {
				bottomShown = bottom.frame.Bounds(c)
			}
		})
	}, 660, 416)
	tt.Advance(32 * time.Millisecond)
	full := shown
	split = true
	tt.Frame()
	if tab.root.count() != 2 || shown != full {
		t.Fatalf("split started at %v with %d panes, want the visible geometry %v", shown, tab.root.count(), full)
	}
	half := (workspace.Y - theme.Current().C.Pane.Gap) / 2
	tt.Advance(80 * time.Millisecond)
	if shown.Y != full.Y || shown.H <= half || shown.H >= full.H {
		t.Fatalf("existing pane did not retract through an intermediate frame: %v", shown)
	}
	if bottomShown.H != 0 || tab.focused != top {
		t.Fatalf("new pane was visible or focused during retraction: %v, focused pane %d", bottomShown, tab.focused.id)
	}
	const sampleX, sampleY = 530, 365
	empty := tt.Image().RGBAAt(sampleX, sampleY)
	for i := 0; i < 36 && bottomShown.H == 0; i++ {
		tt.Advance(16 * time.Millisecond)
	}
	if shown.H != half || bottomShown.H != half || tab.focused != bottom {
		t.Fatalf("reveal must start after the layout settles: top=%v bottom=%v focus=%d", shown, bottomShown, tab.focused.id)
	}
	tt.Advance(48 * time.Millisecond)
	fading := tt.Image().RGBAAt(sampleX, sampleY)
	tt.Advance(240 * time.Millisecond)
	opaque := tt.Image().RGBAAt(sampleX, sampleY)
	if fading == empty || fading == opaque {
		t.Fatalf("new pane skipped its opacity transition: empty=%v fade=%v solid=%v", empty, fading, opaque)
	}

	closeBottom, resizeOnRemoval = true, true
	tt.Frame()
	tt.Advance(80 * time.Millisecond)
	if tab.root.count() != 2 || shown.H != half {
		t.Fatalf("survivor moved while the closed pane was fading: %v", shown)
	}
	tt.Advance(144 * time.Millisecond)
	if tab.root.count() != 1 || shown.Y != full.Y || shown.H <= half || shown.H >= full.H {
		t.Fatalf("survivor did not expand through an intermediate frame: %v", shown)
	}
	tt.Advance(560 * time.Millisecond)
	if shown != full {
		t.Fatalf("survivor settled at %v, want %v", shown, full)
	}
}

func TestTerminalPaneDropKeepsTargetFarEdgeAnchored(t *testing.T) {
	a := &app{prefs: appPreferences{ReduceMotion: "off"}}
	a.terminal.prefs = termtheme.DefaultPrefs()
	top := a.newDisconnectedSession(0, host{name: "Top", port: 22}, false, nil)
	middle := a.newDisconnectedSession(1, host{name: "Middle", port: 22}, false, nil)
	bottom := a.newDisconnectedSession(2, host{name: "Bottom", port: 22}, false, nil)
	tab := &workspaceTab{
		root: &terminalLayout{
			first: &terminalLayout{
				first: &terminalLayout{session: top}, second: &terminalLayout{session: middle}, stacked: true, ratio: .5,
			},
			second: &terminalLayout{session: bottom}, stacked: true, ratio: 2.0 / 3,
		},
		focused: middle,
	}
	a.tabs = []*workspaceTab{tab}
	workspace := ui.Point{X: 660, Y: 600}
	var targetBounds, movedBounds ui.Rect
	var drop bool
	tt := ui.NewTester(func(c *ui.Context) {
		if drop {
			drop = false
			source := paneTransfer{session: middle, tab: tab}
			plan, ok := a.planPaneDrop(c, top, source, dropLeft)
			if !ok || plan.zone != dropLeft {
				t.Fatal("left edge drop did not create the requested split")
			}
			a.dockSession(tab, top, source, plan)
		}
		a.beginWorkspaceFrame(c)
		root := ui.Column(c).Fill()
		root.Children(func() { a.terminals(c, workspace) })
		root.DrawOver(func(_ *ui.Painter, _ ui.Rect) {
			targetBounds, movedBounds = top.frame.Bounds(c), middle.frame.Bounds(c)
		})
	}, 660, 600)
	tt.Advance(32 * time.Millisecond)
	start := targetBounds
	drop = true
	tt.Frame()
	if targetBounds != start || movedBounds.W != 0 {
		t.Fatalf("drop jumped before retraction: target=%v moved=%v start=%v", targetBounds, movedBounds, start)
	}
	for range 5 {
		tt.Advance(16 * time.Millisecond)
		if math.Abs(float64(targetBounds.X+targetBounds.W-(start.X+start.W))) > .1 || movedBounds.W != 0 {
			t.Fatalf("target lost its right anchor or revealed the moved pane early: target=%v moved=%v", targetBounds, movedBounds)
		}
	}
	if targetBounds.X <= start.X || targetBounds.W >= start.W || tab.focused != top {
		t.Fatalf("target did not retract in place with focus: %v", targetBounds)
	}
	tt.Advance(650 * time.Millisecond)
	if gap := targetBounds.X - (movedBounds.X + movedBounds.W); gap != theme.Current().C.Pane.Gap || tab.focused != middle {
		t.Fatalf("drop did not settle in the left slot: target=%v moved=%v", targetBounds, movedBounds)
	}
}
