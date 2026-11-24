package desktop

import (
	"math"
	"slices"
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
	reduced := a.reduceMotion(c)
	for _, tab := range a.tabs {
		if reduced {
			tab.layoutUntil, tab.layoutPending = time.Time{}, false
			continue
		}
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

type paneRelayout struct {
	pending   bool
	entering  []*remoteSession
	focusFrom *remoteSession
	from      map[*remoteSession]ui.Rect
}

func (a *app) startPaneRelayout(tab *workspaceTab, entering *remoteSession) {
	if a.currentTab() != tab || a.activeSection() != sectionSSH {
		return
	}
	a.resizing = nil
	tab.relayout.pending = true
	if entering != nil {
		tab.relayout.entering = append(tab.relayout.entering, entering)
		tab.relayout.focusFrom = tab.focused
	}
}

func (a *app) finishPaneRelayout(c *ui.Context, tab *workspaceTab) {
	relayout := tab.relayout
	tab.relayout = paneRelayout{}
	var focus *remoteSession
	for _, session := range relayout.entering {
		if !session.closing && tab.root.find(session) != nil {
			session.splitFadeStarted = c.Now()
			focus = session
		}
	}
	if focus != nil && tab.focused == relayout.focusFrom {
		a.focusSession(tab, focus)
	}
}

func (a *app) paneRelayoutProgress(c *ui.Context, area ui.Element, tab *workspaceTab, panes []panePlacement) float32 {
	const motion = "pane-relayout"
	relayout := &tab.relayout
	owner := ui.Local(area, "pane-relayout-tab", func() int { return -1 })
	if *owner != tab.id {
		*owner = tab.id
		if !relayout.pending {
			a.finishPaneRelayout(c, tab)
		}
	}
	if a.reduceMotion(c) || slices.Contains(relayout.entering, tab.focused) {
		a.finishPaneRelayout(c, tab)
		return 1
	}
	if relayout.pending {
		workspace := a.workArea.Bounds(c)
		relayout.from = make(map[*remoteSession]ui.Rect, len(panes))
		for _, placement := range panes {
			session := placement.session
			if session.closing || slices.Contains(relayout.entering, session) {
				continue
			}
			visible := visiblePaneBounds(c, session, workspace)
			if visible.W > 0 && visible.H > 0 && visible != placement.rect {
				relayout.from[session] = visible
			}
		}
		relayout.pending = false
		area.Jump(motion, 0)
	}
	if len(relayout.from) == 0 {
		a.finishPaneRelayout(c, tab)
		return 1
	}
	animation := ui.Animation{Spring: uikit.Spring(theme.Current().Motion.Spring.Smooth)}
	progress := area.AnimateTo(motion, 1, a.positionAnimation(animation))
	if area.Settled(motion) {
		a.finishPaneRelayout(c, tab)
		return 1
	}
	return min(max(progress, 0), 1)
}

func visiblePaneBounds(c *ui.Context, session *remoteSession, workspace ui.Rect) ui.Rect {
	visible := session.frame.Bounds(c)
	visible.X -= workspace.X
	visible.Y -= workspace.Y
	return visible
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
	if active == sectionSSH && a.forwardUI.visible && a.forwardSession() != nil {
		minimum.Y += forwardMinimumHeight() + t.C.Window.Gutter
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
	tab.maximized, tab.zoomFrom, tab.zoomPane = maximized, tab.zoomMix, nil
	a.markPaneLayout(tab, t.C.Desktop.ZoomDuration)
}

func (a *app) togglePaneZoom(c *ui.Context, tab *workspaceTab, session *remoteSession) {
	visible := visiblePaneBounds(c, session, a.workArea.Bounds(c))
	a.focusSession(tab, session)
	a.setPaneZoom(tab, !tab.maximized)
	if visible.W > 0 && visible.H > 0 {
		tab.zoomPane, tab.zoomBounds = session, visible
	}
}

func (a *app) zoomProgress(c *ui.Context, area ui.Element, tab *workspaceTab) float32 {
	const motion = "pane-zoom"
	target := float32(0)
	if tab.maximized {
		target = 1
	}
	owner := ui.Local(area, "pane-zoom-tab", func() int { return -1 })
	focusChanged := tab.zoomPane != nil && tab.zoomPane != tab.focused
	if *owner != tab.id || a.reduceMotion(c) || focusChanged {
		*owner = tab.id
		tab.zoomMix, tab.zoomPane = area.Jump(motion, target), nil
		return 1
	}
	animation := ui.Animation{Spring: uikit.Spring(theme.Current().Motion.Spring.Smooth)}
	tab.zoomMix = min(max(area.AnimateTo(motion, target, a.positionAnimation(animation)), 0), 1)
	if area.Settled(motion) {
		tab.zoomPane = nil
		return 1
	}
	span := target - tab.zoomFrom
	if span == 0 {
		return 1
	}
	return min(max((tab.zoomMix-tab.zoomFrom)/span, 0), 1)
}

func interpolatePaneBounds(from, to ui.Rect, fraction float32) ui.Rect {
	return ui.Rect{
		X: from.X + (to.X-from.X)*fraction,
		Y: from.Y + (to.Y-from.Y)*fraction,
		W: max(1, from.W+(to.W-from.W)*fraction),
		H: max(1, from.H+(to.H-from.H)*fraction),
	}
}

func (a *app) paneMotion(c *ui.Context, pane ui.Element, tab *workspaceTab, placement panePlacement) {
	t := theme.Current()
	session := placement.session
	reduced := a.reduceMotion(c)
	if reduced {
		session.revealing = false
	}
	animation := ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.PaneDisplace)}
	if tab.layoutDuration == t.C.Desktop.ZoomDuration {
		spring := uikit.Spring(t.Motion.Spring.Smooth)
		// Settle at subpixel distance so the terminal can adopt its final grid promptly.
		spring.Precision = .5
		animation = ui.Animation{Spring: spring}
	}
	animation = a.positionAnimation(animation)
	transition := ui.ElementTransition{Position: true, Size: true, Animation: &animation}
	instant := ui.Animation{Duration: t.C.Desktop.InstantDuration}
	if a.resizing != nil {
		transition.Animation = &instant
	}
	// The workspace already animates its size; a second spring would lag behind it.
	if !placement.resizing && session != tab.zoomPane && !session.revealing && !tab.workspaceResizedAt.Equal(c.Now()) {
		pane.Transition(transition)
	}
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
			glass.Blur{Radius: blur}.PaintMaterial(p, rect, [4]float32{})
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
		c.AnimationFrame()
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
