package desktop

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

var paneSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}
var zoomSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 240 * time.Millisecond}}

func canSplit(r ui.Rect, zone dropZone) bool {
	if zone == dropTop || zone == dropBottom {
		return r.H >= 2*minPaneHeight+paneSpacing && r.W >= minPaneWidth
	}
	return r.W >= 2*minPaneWidth+paneSpacing && r.H >= minPaneHeight
}

func (a *app) beginWorkspaceFrame(c *ui.Context) {
	a.dragSeen = false
	a.dockPreview = nil
	if d := a.drag; d != nil {
		if d.phase == dragLanded && d.returnStarted.IsZero() {
			d.returnStarted = c.Now()
		}
		if d.phase != dragActive && c.Now().Sub(d.returnStarted) >= 220*time.Millisecond {
			a.drag = nil
		} else if d.kind == dragTab && d.phase == dragActive {
			bar := a.tabScroll.Bounds(c)
			d.detached = a.pointer.Y > bar.Y+bar.H+8
		}
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if !session.closing {
				continue
			}
			if session.closeStarted.IsZero() {
				session.closeStarted = c.Now()
			}
			if c.Now().Sub(session.closeStarted) >= 160*time.Millisecond {
				a.removePane(tab, session)
			} else {
				c.AnimationFrame()
			}
		}
		if tab.layoutPending {
			tab.layoutUntil, tab.layoutPending = c.Now().Add(tab.layoutDuration), false
		}
	}
	a.enforcePaneMinimum(c)
}

func (a *app) markPaneLayout(tab *workspaceTab, duration time.Duration) {
	tab.layoutDuration, tab.layoutPending = duration, true
}

func (a *app) enforcePaneMinimum(c *ui.Context) {
	if a.window == nil {
		return
	}
	minimum := ui.Point{X: minPaneWidth, Y: minPaneHeight}
	if tab := a.currentTab(); tab != nil {
		minimum = tab.root.minimum()
	}
	width := minimum.X + navigationWidth + workspaceSpacing
	if !a.sidebarCollapsed {
		width += a.sidebarWidth + workspaceSpacing
	}
	if a.statusVisible {
		width += statusPanelWidth + workspaceSpacing
	}
	size := [2]int{max(1000, int(math.Ceil(float64(width)))), max(640, int(minimum.Y)+titleBarHeight+workspaceSpacing)}
	if size == a.minimumWindow {
		return
	}
	a.minimumWindow = size
	a.window.SetMinimumSize(size[0], size[1])
	w, h := c.Size()
	if w < float32(size[0]) || h < float32(size[1]) {
		a.window.SetContentSize(max(int(w), size[0]), max(int(h), size[1]))
	}
}

func (a *app) setPaneZoom(tab *workspaceTab, maximized bool) {
	if tab.maximized == maximized {
		return
	}
	tab.maximized, tab.zoomFrom, tab.zoomPending = maximized, tab.zoomMix, true
	a.markPaneLayout(tab, 240*time.Millisecond)
}

func (a *app) togglePaneZoom(tab *workspaceTab, session *remoteSession) {
	a.focusSession(tab, session)
	a.setPaneZoom(tab, !tab.maximized)
}

func (a *app) zoomProgress(c *ui.Context, tab *workspaceTab) float32 {
	if tab.zoomPending {
		tab.zoomStarted, tab.zoomPending = c.Now(), false
	}
	target := float32(0)
	if tab.maximized {
		target = 1
	}
	if tab.zoomStarted.IsZero() {
		return target
	}
	progress := min(1, float32(c.Now().Sub(tab.zoomStarted))/float32(240*time.Millisecond))
	progress = ui.CubicBezier(.2, .8, .2, 1)(progress)
	tab.zoomMix = tab.zoomFrom + (target-tab.zoomFrom)*progress
	if progress < 1 {
		c.AnimationFrame()
	}
	return tab.zoomMix
}

func (a *app) paneMotion(c *ui.Context, pane ui.Element, tab *workspaceTab, session *remoteSession) {
	animation := &paneSpring
	if tab.layoutDuration == 240*time.Millisecond {
		animation = &zoomSpring
	}
	transition := ui.ElementTransition{Position: true, Size: true, Animation: animation}
	instant := ui.Animation{Duration: time.Nanosecond}
	if a.resizing != nil {
		transition.Animation = &instant
	}
	pane.Transition(transition)
	alpha := float32(1)
	if tab.focused != session {
		alpha *= 1 - tab.zoomMix
	} else if c.Preferences().ReduceMotion && !tab.zoomStarted.IsZero() {
		alpha *= min(1, float32(c.Now().Sub(tab.zoomStarted))/float32(240*time.Millisecond))
	}
	if session.entering {
		if session.enterStarted.IsZero() {
			session.enterStarted = c.Now()
		}
		progress := min(1, float32(c.Now().Sub(session.enterStarted))/float32(220*time.Millisecond))
		progress = ui.CubicBezier(.2, .8, .2, 1)(progress)
		alpha *= progress
		if !c.Preferences().ReduceMotion {
			x, y := float32(0), float32(0)
			switch session.enter {
			case dropLeft:
				x = -12
			case dropRight:
				x = 12
			case dropTop:
				y = -12
			case dropBottom:
				y = 12
			}
			pane.Translate(x*(1-progress), y*(1-progress))
		}
		if progress >= 1 {
			session.entering = false
		} else {
			c.AnimationFrame()
		}
	}
	if session.closing {
		progress := min(1, float32(c.Now().Sub(session.closeStarted))/float32(160*time.Millisecond))
		alpha *= 1 - progress
		if !c.Preferences().ReduceMotion {
			pane.Scale(1-.03*progress, 1-.03*progress)
		}
	}
	pane.Opacity(alpha)
	if tab.maximized && tab.focused != session && tab.zoomMix >= 1 {
		pane.Hidden(true)
	}
}

func (a *app) terminalDivider(c *ui.Context, split splitPlacement) {
	node, r := split.node, split.rect
	edge := ui.Box(c).Key(node).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Center().Focusable().FocusRing(false).Role(ui.RoleSplitter).Label("调整终端分屏比例")
	axis, delta := split.parent.W-paneSpacing, float32(0)
	firstMin, secondMin := node.first.minimum().X, node.second.minimum().X
	firstSize := r.X - split.parent.X
	edge.Cursor(ui.CursorResizeEW)
	dx, dy, held := edge.Dragged()
	if node.stacked {
		edge.Cursor(ui.CursorResizeNS)
		axis, delta = split.parent.H-paneSpacing, dy
		firstSize = r.Y - split.parent.Y
		firstMin, secondMin = node.first.minimum().Y, node.second.minimum().Y
	} else {
		delta = dx
	}
	if held {
		a.resizing = node
		if delta != 0 {
			node.ratio = min(max((firstSize+delta)/axis, firstMin/axis), 1-secondMin/axis)
		}
	} else if a.resizing == node {
		a.resizing = nil
	}
	animation := paneSpring
	if held {
		animation = ui.Animation{Duration: time.Nanosecond}
	}
	edge.Transition(ui.ElementTransition{Position: true, Size: true, Animation: &animation})
	if edge.DoubleClicked() {
		node.ratio = .5
		a.markPaneLayout(a.currentTab(), 220*time.Millisecond)
	}
	if edge.Focused() {
		var direction float32
		if edge.Shortcut(0, ui.KeyLeft) || edge.Shortcut(0, ui.KeyUp) {
			direction = -1
		}
		if edge.Shortcut(0, ui.KeyRight) || edge.Shortcut(0, ui.KeyDown) {
			direction = 1
		}
		if direction != 0 {
			node.ratio = min(max(node.ratio+direction*8/axis, firstMin/axis), 1-secondMin/axis)
			a.markPaneLayout(a.currentTab(), 220*time.Millisecond)
		}
	}
	target := float32(0)
	if edge.Hovered() || held || edge.FocusVisible() {
		target = 1
	}
	color := ui.Hex("#c7c7cc")
	if held {
		color = a.theme.Accent
	}
	edge.Children(func() {
		handle := ui.Box(c).Size(3, 28).Radius(1.5).Background(color).
			Opacity(edge.Animate("handle", target, 120*time.Millisecond)).PassThrough()
		if node.stacked {
			handle.Size(28, 3)
		}
	})
}
