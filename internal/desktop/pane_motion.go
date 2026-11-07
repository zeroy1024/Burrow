package desktop

import (
	"math"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

func canSplit(r ui.Rect, zone dropZone) bool {
	t := theme.Current()
	minimumWidth, minimumHeight, gap := t.C.Desktop.PaneMinWidth, t.C.Desktop.PaneMinHeight, t.C.Pane.Gap
	if zone == dropTop || zone == dropBottom {
		return r.H >= 2*minimumHeight+gap && r.W >= minimumWidth
	}
	return r.W >= 2*minimumWidth+gap && r.H >= minimumHeight
}

func (a *app) beginWorkspaceFrame(c *ui.Context) {
	t := theme.Current()
	a.dragSeen = false
	a.dockPreview = nil
	if d := a.drag; d != nil {
		pressed := d.pressed
		d.pressed = false
		if d.phase == dragLanded {
			a.drag = nil
		} else if d.phase == dragReturning && !pressed && c.Now().Sub(d.returnStarted) >= t.C.ReorderMotion.Cancel.Duration {
			a.drag = nil
		} else if d.kind == dragTab && d.phase == dragActive {
			bar := a.tabScroll.Bounds(c)
			d.detached = a.pointer.Y > bar.Y+bar.H+t.C.Desktop.TabDetachDistance
		}
		if d.kind == dragPane && d.phase == dragActive {
			d.overTabs = false
		}
	}
	for _, tab := range a.tabs {
		if tab.layoutPending {
			tab.layoutUntil, tab.layoutPending = c.Now().Add(tab.layoutDuration), false
		}
	}
}

func (a *app) updatePaneClosures(c *ui.Context) {
	t := theme.Current()
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if !session.closing {
				continue
			}
			if session.closeStarted.IsZero() {
				session.closeStarted = c.Now()
			}
			remaining := t.C.Desktop.PaneCloseDuration - c.Now().Sub(session.closeStarted)
			if remaining <= 0 {
				a.removePane(tab, session)
			} else {
				c.After(remaining)
			}
		}
	}
}

func (a *app) markPaneLayout(tab *workspaceTab, duration time.Duration) {
	tab.layoutDuration, tab.layoutPending = duration, true
}

func (a *app) enforcePaneMinimum(c *ui.Context) {
	t := theme.Current()
	if a.window == nil {
		return
	}
	active := a.activeSection()
	minimum := ui.Point{X: t.C.Desktop.PaneMinWidth, Y: t.C.Desktop.PaneMinHeight}
	if active == sectionFiles {
		minimum.X = t.C.FTPSplit.MinRemote
		if a.files.localOpen {
			minimum.X += t.C.FTPSplit.MinLocal + t.C.FTPSplit.Gutter
		}
	} else if active == sectionSSH {
		if tab := a.currentTab(); tab != nil {
			minimum = tab.root.minimum()
		}
	}
	width := minimum.X + t.C.Rail.Width + t.C.Window.Gutter
	if active.hasCatalog() && !a.page().sidebarCollapsed {
		width += a.page().sidebarWidth + t.C.Window.Gutter
	}
	if active == sectionSSH && a.statusVisible {
		width += t.Size.StatsPanel + t.C.Window.Gutter
	}
	size := [2]int{
		max(int(t.C.Desktop.WindowMinWidth), int(math.Ceil(float64(width)))),
		max(int(t.C.Desktop.WindowMinHeight), int(minimum.Y+t.C.Titlebar.Height+t.C.Window.Gutter)),
	}
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
	t := theme.Current()
	if tab.maximized == maximized {
		return
	}
	tab.maximized, tab.zoomFrom, tab.zoomPending = maximized, tab.zoomMix, true
	a.markPaneLayout(tab, t.C.Desktop.ZoomDuration)
}

func (a *app) togglePaneZoom(tab *workspaceTab, session *remoteSession) {
	a.focusSession(tab, session)
	a.setPaneZoom(tab, !tab.maximized)
}

func (a *app) zoomProgress(c *ui.Context, tab *workspaceTab) float32 {
	t := theme.Current()
	if tab.zoomPending {
		tab.zoomStarted, tab.zoomPending = c.Now(), false
	}
	target := float32(0)
	if tab.maximized {
		target = 1
	}
	if tab.zoomStarted.IsZero() || a.reduceMotion(c) {
		tab.zoomMix = target
		return tab.zoomMix
	}
	progress := min(1, float32(c.Now().Sub(tab.zoomStarted))/float32(t.C.Desktop.ZoomDuration))
	curve := t.C.Desktop.MotionEasing
	progress = ui.CubicBezier(curve[0], curve[1], curve[2], curve[3])(progress)
	tab.zoomMix = tab.zoomFrom + (target-tab.zoomFrom)*progress
	if progress < 1 {
		c.AnimationFrame()
	}
	return tab.zoomMix
}

func (a *app) paneMotion(c *ui.Context, pane ui.Element, tab *workspaceTab, session *remoteSession) {
	t := theme.Current()
	animation := ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.PaneDisplace)}
	if tab.layoutDuration == t.C.Desktop.ZoomDuration {
		animation = ui.Animation{Spring: uikit.Spring(t.C.Desktop.ZoomSpring)}
	}
	animation = a.positionAnimation(animation)
	transition := ui.ElementTransition{Position: true, Size: true, Animation: &animation}
	instant := ui.Animation{Duration: t.C.Desktop.InstantDuration}
	if a.resizing != nil {
		transition.Animation = &instant
	}
	// The workspace already animates its size; a second spring would lag behind it.
	if !session.revealing && !tab.workspaceResizedAt.Equal(c.Now()) {
		pane.Transition(transition)
	}
	alpha := float32(1)
	if tab.focused != session {
		alpha *= 1 - tab.zoomMix
	} else if !session.revealing && a.reduceMotion(c) && !tab.zoomStarted.IsZero() {
		alpha *= min(1, float32(c.Now().Sub(tab.zoomStarted))/float32(t.C.Desktop.ZoomDuration))
	}
	pose := ui.Pose{}
	if session.closing {
		pose.Fade = 1
		if !a.reduceMotion(c) {
			pose.Scale = t.C.Desktop.PaneCloseScale
		}
	}
	pane.AnimatePose(pose, uikit.Tween(t.C.Desktop.PaneCloseDuration))
	pane.Opacity(alpha)
	if tab.maximized && tab.focused != session && tab.zoomMix >= 1 {
		pane.Hidden(true)
	}
}

func startPaneReveal(session *remoteSession) {
	session.revealing, session.revealStarted = true, time.Time{}
}

func paneReveal(c *ui.Context, card ui.Element, session *remoteSession) {
	if !session.revealing {
		return
	}
	t := theme.Current()
	reveal := t.C.PaneReveal
	card.Children(func() {
		ui.Box(c).Key("drop-reveal").Absolute().Top(0).Right(0).Bottom(0).Left(0).
			PassThrough().Draw(func(p *ui.Painter, rect ui.Rect) {
			// The final-size pane content is already painted beneath this overlay.
			if session.revealStarted.IsZero() {
				session.revealStarted = p.Now()
			}
			progress := min(1, float32(p.Now().Sub(session.revealStarted))/float32(reveal.Duration))
			if progress == 1 {
				session.revealing = false
				c.Invalidate()
				return
			}
			blur := reveal.Blur * (1 - uikit.Easing(reveal.Easing)(progress))
			radius := t.C.Pane.Radius
			glass.Blur{Radius: blur}.PaintMaterial(p, rect, [4]float32{radius, radius, radius, radius})
			p.AnimationFrame()
		})
	})
}

func (a *app) terminalDivider(c *ui.Context, split splitPlacement) {
	t := theme.Current()
	node, r := split.node, split.rect
	edge := ui.Box(c).Key(node).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Center().Focusable().FocusRing(false).Role(ui.RoleSplitter).Label(locales.MessageResizeTerminalPanes())
	axis, delta := split.parent.W-t.C.Pane.Gap, float32(0)
	firstMin, secondMin := node.first.minimum().X, node.second.minimum().X
	firstSize := r.X - split.parent.X
	edge.Cursor(ui.CursorResizeEW)
	dx, dy, held := edge.Dragged()
	if node.stacked {
		edge.Cursor(ui.CursorResizeNS)
		axis, delta = split.parent.H-t.C.Pane.Gap, dy
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
	animation := ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.PaneDisplace)}
	if held {
		animation = ui.Animation{Duration: t.C.Desktop.InstantDuration}
	}
	animation = a.positionAnimation(animation)
	if !a.currentTab().workspaceResizedAt.Equal(c.Now()) {
		edge.Transition(ui.ElementTransition{Position: true, Size: true, Animation: &animation})
	}
	if edge.DoubleClicked() {
		node.ratio = .5
		a.markPaneLayout(a.currentTab(), t.C.Desktop.PaneEnterDuration)
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
			node.ratio = min(max(node.ratio+direction*t.C.Desktop.DividerKeyboardStep/axis, firstMin/axis), 1-secondMin/axis)
			a.markPaneLayout(a.currentTab(), t.C.Desktop.PaneEnterDuration)
		}
	}
	target := float32(0)
	if edge.Hovered() || held || edge.FocusVisible() {
		target = 1
	}
	edge.Children(func() { uikit.SplitterHandle(c, edge, target, held, node.stacked) })
}
