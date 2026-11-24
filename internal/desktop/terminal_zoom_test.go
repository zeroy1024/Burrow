package desktop

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

func TestTerminalPaneZoomSurvivesWorkspaceResize(t *testing.T) {
	a := &app{prefs: appPreferences{ReduceMotion: "off"}}
	a.terminal.prefs = termtheme.DefaultPrefs()
	left := a.newDisconnectedSession(0, host{name: "Left", port: 22}, false, nil)
	below := a.newDisconnectedSession(1, host{name: "Below", port: 22}, false, nil)
	right := a.newDisconnectedSession(2, host{name: "Right", port: 22}, false, nil)
	tab := &workspaceTab{
		root: &terminalLayout{
			first: &terminalLayout{
				first: &terminalLayout{session: left}, second: &terminalLayout{session: below}, stacked: true, ratio: .5,
			},
			second: &terminalLayout{session: right}, ratio: .5,
		},
		focused: left,
	}
	a.tabs = []*workspaceTab{tab}
	workspace := ui.Point{X: 660, Y: 400}
	var shown ui.Rect
	var zoomAndResize bool
	var sidebarProgress float32
	tt := ui.NewTester(func(c *ui.Context) {
		if zoomAndResize {
			zoomAndResize = false
			a.togglePaneZoom(c, tab, left)
			workspace.X -= 16
		}
		a.beginWorkspaceFrame(c)
		root := ui.Column(c).Fill()
		target := float32(0)
		if tab.maximized {
			target = 1
		}
		spring := uikit.Spring(theme.Current().Motion.Spring.Smooth)
		sidebarProgress = root.AnimateTo("sidebar-reference", target, ui.Animation{Spring: spring})
		root.Children(func() { a.terminals(c, workspace) })
		root.DrawOver(func(_ *ui.Painter, _ ui.Rect) { shown = left.frame.Bounds(c) })
	}, 660, 400)
	tt.Advance(32 * time.Millisecond)
	start := shown
	zoomAndResize = true
	tt.Frame()
	if shown != start {
		t.Fatalf("first zoom frame is %v, want the previous visible bounds %v", shown, start)
	}
	tt.Advance(64 * time.Millisecond)
	if shown.W <= start.W || shown.W >= workspace.X || shown.H <= start.H || shown.H >= workspace.Y {
		t.Fatalf("zoom skipped its intermediate geometry: start=%v shown=%v target=%v", start, shown, workspace)
	}
	progress := (shown.W - start.W) / (workspace.X - start.W)
	if math.Abs(float64(progress-sidebarProgress)) > .001 {
		t.Fatalf("zoom progress = %g, want sidebar progress %g", progress, sidebarProgress)
	}
	if left.contentWidth != start.W || left.contentHeight != start.H {
		t.Fatalf("terminal reflowed during zoom: %g×%g", left.contentWidth, left.contentHeight)
	}
}
