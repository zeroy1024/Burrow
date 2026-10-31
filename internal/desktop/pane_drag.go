package desktop

import (
	"fmt"
	"math"
	"slices"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type paneTransfer struct {
	tab     *workspaceTab
	session *remoteSession
	fromTab bool
}

type paneDragKind uint8

const (
	dragPane paneDragKind = iota
	dragTab
	dragHost
)

type paneDragPhase uint8

const (
	dragActive paneDragPhase = iota
	dragReturning
	dragLanded
)

type paneDragState struct {
	kind                         paneDragKind
	phase                        paneDragPhase
	session                      *remoteSession
	originTab                    *workspaceTab
	origin, paneRect, returnRect ui.Rect
	grab                         ui.Point
	paneImage, tabImage          *ui.Bitmap
	detached                     bool
	order                        []*workspaceTab
	returnStarted                time.Time
	appearance                   *theme.Theme
}

func (a *app) paneDragHandle(c *ui.Context, source ui.Element, transfer paneTransfer) {
	t := theme.Current()
	if transfer.session.closing {
		return
	}
	if d := a.drag; d != nil && d.phase == dragActive && transfer.fromTab &&
		d.kind == dragTab && d.originTab == transfer.tab {
		transfer.session = d.session
	}
	// MyGo v0.7.3 fixes its native threshold at 4 DIPs; it exposes no hold-delay option.
	source.Drag(transfer).Cursor(ui.CursorGrab).FocusRing(false)
	if !source.Dragging() {
		return
	}
	a.dragSeen = true
	if a.drag != nil {
		if a.drag.phase == dragActive {
			return
		}
		a.drag = nil
	}
	area := a.workArea.Bounds(c)
	r := transfer.session.frame.Bounds(c)
	kind := dragPane
	if transfer.fromTab {
		kind, r = dragTab, transfer.tab.frame.Bounds(c)
	}
	size := ui.Rect{W: r.W, H: r.H}
	if transfer.fromTab {
		size.W, size.H = min(t.C.Desktop.DragPreviewWidth, area.W), min(t.C.Desktop.DragPreviewHeight, area.H)
	}
	d := &paneDragState{
		kind: kind, session: transfer.session, originTab: transfer.tab, origin: r, paneRect: size,
		grab: ui.Point{X: a.pointer.X - r.X, Y: a.pointer.Y - r.Y}, order: []*workspaceTab{},
	}
	d.appearance = t
	d.paneImage = a.paneSnapshot(transfer.session, size.W, size.H)
	if transfer.fromTab {
		d.order = slices.Clone(a.tabs)
		d.tabImage = a.tabSnapshot(transfer.tab, r.W, r.H)
		bar := a.tabScroll.Bounds(c)
		d.grab = ui.Point{X: a.tabPress.X + bar.X - r.X, Y: a.tabPress.Y + bar.Y - r.Y}
		d.detached = a.pointer.Y > bar.Y+bar.H+t.C.Desktop.TabDetachDistance
	}
	a.drag = d
}

func (a *app) tabStripTarget(c *ui.Context, strip ui.Element) {
	_, over := ui.DragOver[paneTransfer](strip)
	_, released := ui.Drop[paneTransfer](strip)
	d := a.drag
	if d == nil || d.kind != dragTab || d.detached || d.phase != dragActive {
		return
	}
	if !over && !released {
		return
	}
	r := strip.Bounds()
	var delta float32
	t := theme.Current()
	zone := t.C.DropIndicator.AutoScrollZone
	step := t.C.DropIndicator.AutoScrollMax * float32(t.C.Desktop.FrameInterval.Seconds())
	if over && a.pointer.X < r.X+zone {
		depth := min(1, (r.X+zone-a.pointer.X)/zone)
		delta = -step * depth * depth
	}
	if over && a.pointer.X > r.X+r.W-zone {
		depth := min(1, (a.pointer.X-(r.X+r.W-zone))/zone)
		delta = step * depth * depth
	}
	if delta != 0 {
		next := min(max(a.tabScroll.X+delta, 0), a.tabScroll.MaxX)
		if next != a.tabScroll.X {
			a.tabScroll.X = next
			c.After(t.C.Desktop.FrameInterval)
		}
	}

	// Reconstruct the slots from their widths, independent of position animations.
	from := slices.Index(a.tabs, d.originTab)
	to := from
	center := a.pointer.X - d.grab.X + d.origin.W/2
	x := r.X - a.tabScroll.X
	for i, tab := range a.tabs {
		width := tab.frame.Bounds(c).W
		if i < from && center < x+width/2 {
			to = min(to, i)
		}
		if i > from && center > x+width/2 {
			to = max(to, i)
		}
		x += width + t.C.Titlebar.TabsGap
	}
	if to != from {
		active := a.currentTab()
		a.tabs = slices.Delete(a.tabs, from, from+1)
		a.tabs = slices.Insert(a.tabs, to, d.originTab)
		a.activeTab = slices.Index(a.tabs, active)
		c.AnimationFrame()
	}
	if released {
		d.phase, d.returnStarted = dragReturning, c.Now()
		d.returnRect = a.tabSlot(c, d.originTab)
	}
}

func (a *app) tabSlot(c *ui.Context, tab *workspaceTab) ui.Rect {
	t := theme.Current()
	r := tab.frame.Bounds(c)
	r.X = a.tabScroll.Bounds(c).X - a.tabScroll.X
	for _, before := range a.tabs[:slices.Index(a.tabs, tab)] {
		r.X += before.frame.Bounds(c).W + t.C.Titlebar.TabsGap
	}
	return r
}

func (a *app) lifted(session *remoteSession) bool {
	d := a.drag
	return d != nil && d.session == session && (d.phase == dragLanded || d.kind != dragHost && (d.kind == dragPane || d.detached))
}

func (a *app) finishWorkspaceDrag(c *ui.Context) {
	d := a.drag
	if d == nil || d.phase != dragActive || a.dragSeen {
		return
	}
	if d.kind == dragTab {
		active := a.currentTab()
		a.tabs = slices.Clone(d.order)
		a.activeTab = slices.Index(a.tabs, active)
	}
	d.phase, d.returnStarted, d.returnRect = dragReturning, c.Now(), d.origin
	switch d.kind {
	case dragTab:
		d.returnRect = a.tabSlot(c, d.originTab)
	case dragPane:
		d.returnRect = d.session.frame.Bounds(c)
	}
	c.AnimationFrame()
}

func (a *app) dragPreview(c *ui.Context) {
	d := a.drag
	if d == nil {
		return
	}
	t := theme.Current()
	if d.appearance != t {
		d.appearance = t
		d.paneImage = a.paneSnapshot(d.session, d.paneRect.W, d.paneRect.H)
		if d.kind == dragTab {
			d.tabImage = a.tabSnapshot(d.originTab, d.origin.W, d.origin.H)
		}
	}
	r, image := d.paneRect, d.paneImage
	if d.kind == dragTab && !d.detached {
		r, image = d.origin, d.tabImage
	}
	r.X, r.Y = a.pointer.X-d.grab.X, a.pointer.Y-d.grab.Y
	if d.kind == dragTab && d.detached {
		r.X, r.Y = a.pointer.X-min(d.grab.X, r.W/2), a.pointer.Y-t.C.Desktop.DragGrabY
	}
	animation := ui.Animation{Duration: t.C.Desktop.InstantDuration}
	scale, rotate, opacity := t.C.DragLift.Scale, t.C.Desktop.DragRotation, t.C.Desktop.DragOpacity
	duration := t.C.DragLift.LiftDuration
	if d.phase == dragReturning {
		r = d.returnRect
		animation = ui.Animation{Spring: tokenSpring(t.C.ReorderMotion.Cancel)}
		scale, rotate, opacity = 1, 0, 1
		duration = t.C.ReorderMotion.Cancel.Duration
	}
	if d.phase == dragLanded && !d.returnStarted.IsZero() {
		area := a.workArea.Bounds(c)
		r = d.session.layoutRect
		r.X, r.Y = r.X+area.X, r.Y+area.Y
		animation = ui.Animation{Spring: tokenSpring(t.C.ReorderMotion.PaneDrop)}
		scale, rotate, opacity = 1, 0, 1
		duration = t.C.ReorderMotion.PaneDrop.Duration
	}
	transition := ui.ElementTransition{Position: true, Size: true, Animation: &animation}
	ghost := ui.Box(c).Key(d).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Radius(t.C.DragLift.Radius).PassThrough().Transition(transition)
	ghost.Scale(ghost.Animate("lift-scale", scale, t.C.DragLift.LiftDuration), ghost.Animate("lift-scale-y", scale, t.C.DragLift.LiftDuration)).
		Rotate(ghost.Animate("lift-angle", rotate, t.C.DragLift.LiftDuration)).
		Opacity(animateVisual(ghost, "lift-alpha", opacity, t.C.DragLift.LiftDuration))
	applyShadow(ghost, t.C.DragLift.Shadow)
	if d.phase != dragActive && !d.returnStarted.IsZero() {
		elapsed := c.Now().Sub(d.returnStarted)
		if c.Preferences().ReduceMotion {
			progress := min(1, float32(elapsed)/float32(duration))
			ghost.Opacity(t.C.Desktop.DragOpacity + (1-t.C.Desktop.DragOpacity)*progress)
			if progress < 1 {
				c.AnimationFrame()
			}
		}
		if elapsed < duration {
			c.After(duration - elapsed)
		}
	}
	ghost.Children(func() { ui.Image(c, image).Fill().Fit(ui.Cover).Radius(t.C.DragLift.Radius).PassThrough() })
}

func (a *app) paneSnapshot(session *remoteSession, width, height float32) *ui.Bitmap {
	t := theme.Current()
	h := a.hosts[session.host]
	position := session.scroll.offset.Y
	if position >= session.scroll.offset.MaxY {
		position = math.MaxFloat32
	}
	offset := ui.ScrollState{Y: position}
	image := ui.Render(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Radius(t.C.Pane.Radius).Clip().Background(tokenColor(t.C.Pane.Bg)).Children(func() {
			ui.Scroll(c.Key("preview-output")).Fill().TrackScroll(&offset).Padding(
				38,
				14,
				50,
				14,
			).ScrollbarInsets(0, -12, 0, 0).Font("Menlo, monospace").FontSize(11).TextColor(ui.Hex(terminalPrimaryText)).Children(func() {
				a.terminalLines(c, session, false)
			})
			ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(t.C.Pane.HeaderHeight).Padding(0, t.C.Pane.HeaderPaddingLeft).Gap(t.C.Pane.HeaderGap).
				Background(tokenColor(t.C.Pane.Bg).Alpha(t.C.Desktop.PreviewHeaderOpacity)).Children(func() {
				statusDot(c, session.state, t.C.SiderItem.DotSize)
				ui.Text(c, h.name).FontSize(t.C.Pane.HeaderFontSize).FontWeight(t.C.Pane.NameWeight).TextColor(tokenColor(t.C.Pane.Name))
			})
			ui.Row(c).Absolute().Bottom(0).Left(0).Right(0).Height(t.C.StatusBar.HeightPane).Padding(0, t.C.StatusBar.PaddingX).
				Background(tokenColor(t.C.Pane.Bg).Alpha(t.C.Desktop.PreviewHeaderOpacity)).Children(func() {
				ui.Text(c, h.user+"@"+h.address).FontSize(t.C.StatusBar.FontSize).TextColor(tokenColor(t.C.StatusBar.Text))
			})
		})
	}, int(math.Ceil(float64(width))), int(math.Ceil(float64(height))), 1)
	return ui.NewBitmap(image)
}

func (a *app) tabSnapshot(tab *workspaceTab, width, height float32) *ui.Bitmap {
	t := theme.Current()
	image := ui.Render(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Row(c).Fill().Padding(0, t.C.Tab.PaddingLeft).Gap(t.C.Tab.Gap).Radius(t.C.Tab.Radius).Background(tokenColor(t.C.DragGhost.Bg)).Children(func() {
			statusDot(c, tab.focused.state, t.C.Tab.DotSize)
			ui.Text(c, a.hosts[tab.focused.host].name).FontSize(t.C.Tab.FontSize).FontWeight(t.C.Tab.WeightSelected).TextColor(tokenColor(t.C.Tab.TextSelected))
		})
	}, int(math.Ceil(float64(width))), int(math.Ceil(float64(height))), 1)
	return ui.NewBitmap(image)
}

func (a *app) paneDrop(c *ui.Context, pane ui.Element, target *remoteSession, catalog bool) {
	t := theme.Current()
	payload, over := ui.DragOver[any](pane)
	value, dropped := ui.Drop[any](pane)
	if dropped {
		payload = value
	}
	var source hostDrag
	var catalogOver, catalogDropped bool
	if catalog {
		source, catalogOver, catalogDropped = a.catalogPaneDropTarget(c, target)
		if catalogOver {
			payload, over, dropped = source, true, catalogDropped
		}
	}
	if !over && !dropped {
		return
	}
	tab := a.currentTab()
	x, y, _ := pane.PointerPosition()
	zone := zoneAt(x, y, pane.Bounds().W, pane.Bounds().H, tab.root.count() == 1)
	plan, valid := a.planPaneDrop(c, target, payload, zone)
	if !valid {
		pane.Cursor(ui.CursorNotAllowed)
		return
	}
	pane.Cursor(ui.CursorDefault)
	if catalogOver {
		pane.Cursor(ui.CursorGrabbing)
	}
	if over {
		r := plan.target
		inset := t.C.Desktop.PreviewInset
		r.X, r.Y, r.W, r.H = r.X+inset, r.Y+inset, r.W-2*inset, r.H-2*inset
		switch plan.zone {
		case dropLeft:
			r.W = (r.W - t.C.Pane.Gap) / 2
		case dropRight:
			r.W = (r.W - t.C.Pane.Gap) / 2
			r.X += r.W + t.C.Pane.Gap
		case dropTop:
			r.H = (r.H - t.C.Pane.Gap) / 2
		case dropBottom:
			r.H = (r.H - t.C.Pane.Gap) / 2
			r.Y += r.H + t.C.Pane.Gap
		}
		a.dockPreview = &r
	}
	if dropped {
		a.pendingDock = func() {
			a.dockSession(tab, target, payload, plan)
			if catalogDropped {
				a.catalogDrag.externalDrop = true
			}
		}
	}
}

// Catalog drags use Draggable rather than Drag, so their own preview and
// edge scrolling remain in control while the same hosts can dock in a pane.
func (a *app) catalogHostDropTarget(element ui.Element) (hostDrag, bool, bool) {
	source, active, released := a.catalogHostTransfer()
	if !active {
		return source, false, false
	}
	r := element.Bounds()
	inside := a.pointer.X >= r.X && a.pointer.X < r.X+r.W &&
		a.pointer.Y >= r.Y && a.pointer.Y < r.Y+r.H
	return source, inside, inside && released
}

func (a *app) catalogPaneDropTarget(c *ui.Context, target *remoteSession) (hostDrag, bool, bool) {
	source, active, released := a.catalogHostTransfer()
	if !active {
		return source, false, false
	}
	tab := a.currentTab()
	sessions := tab.root.sessions()
	focused := slices.Index(sessions, tab.focused)
	sessions = slices.Delete(sessions, focused, focused+1)
	sessions = append(sessions, tab.focused)
	// The focused wrapper paints last. Select one target in reverse paint
	// order, using persistent wrappers rather than newly created drop shields.
	for i := len(sessions) - 1; i >= 0; i-- {
		candidate := sessions[i]
		if tab.maximized && candidate != tab.focused {
			continue
		}
		r := candidate.frame.Bounds(c)
		insideX := a.pointer.X >= r.X && a.pointer.X < r.X+r.W
		insideY := a.pointer.Y >= r.Y && a.pointer.Y < r.Y+r.H
		if !insideX || !insideY {
			continue
		}
		selected := candidate == target
		return source, selected, selected && released
	}
	return source, false, false
}

type paneDropPlan struct {
	zone      dropZone
	target    ui.Rect
	remaining *terminalLayout
}

func (a *app) planPaneDrop(c *ui.Context, target *remoteSession, value any, zone dropZone) (paneDropPlan, bool) {
	tab := a.currentTab()
	plan := paneDropPlan{zone: zone, target: target.layoutRect}
	if target.closing {
		return plan, false
	}
	var local *remoteSession
	switch source := value.(type) {
	case paneTransfer:
		if source.session == target || source.session.closing {
			return plan, false
		}
		if source.fromTab && a.drag != nil && !a.drag.detached {
			return plan, false
		}
		if source.tab == tab {
			local = source.session
		}
	case hostDrag:
	default:
		return plan, false
	}
	if zone == dropCenter {
		return plan, true
	}
	if local != nil {
		area, minimum := a.workArea.Bounds(c), tab.root.minimum()
		canvas := ui.Rect{W: max(area.W, minimum.X), H: max(area.H, minimum.Y)}
		remaining := tab.root.copyAt(canvas).remove(local)
		panes, splits := []panePlacement{}, []splitPlacement{}
		remaining.placements(canvas, &panes, &splits)
		for _, pane := range panes {
			if pane.session == target {
				plan.target = pane.rect
				break
			}
		}
		plan.remaining = remaining
	}
	if !canSplit(plan.target, zone) {
		return paneDropPlan{zone: dropCenter, target: target.layoutRect}, true
	}
	return plan, local != nil || tab.root.count() < maxTerminalPanes
}

func (a *app) dockSession(tab *workspaceTab, target *remoteSession, value any, plan paneDropPlan) {
	t := theme.Current()
	zone := plan.zone
	var session *remoteSession
	switch source := value.(type) {
	case hostDrag:
		session = a.newSession(source.id)
		if zone == dropCenter {
			tab.root.find(target).session = session
		}
	case paneTransfer:
		session = source.session
		if zone == dropCenter {
			source.tab.root.find(session).session, tab.root.find(target).session = target, session
			if source.tab != tab && source.tab.focused == session {
				source.tab.focused = target
			}
		} else {
			if source.tab == tab {
				tab.root = plan.remaining
			} else {
				source.tab.root = source.tab.root.remove(session)
				if source.tab.root == nil {
					index := slices.Index(a.tabs, source.tab)
					a.tabs = slices.Delete(a.tabs, index, index+1)
					a.activeTab = slices.Index(a.tabs, tab)
				} else if source.tab.focused == session {
					source.tab.focused = source.tab.root.firstSession()
				}
			}
		}
	}
	if zone != dropCenter {
		tab.root.find(target).insert(session, zone)
	}
	a.focusSession(tab, session)
	a.setPaneZoom(tab, false)
	a.markPaneLayout(tab, t.C.ReorderMotion.PaneDrop.Duration)
	if a.drag != nil {
		a.drag.session, a.drag.phase = session, dragLanded
	}
}

func (a *app) dragPlaceholder(pane ui.Element) {
	pane.Draw(func(p *ui.Painter, r ui.Rect) {
		t := theme.Current()
		p.Fill(r, tokenColor(t.C.DropIndicator.PlaceholderBg), t.C.Pane.Radius)
		layer := t.C.DropIndicator.PlaceholderShadow[0]
		p.StrokeDashed(r, tokenColor(layer.Color), t.C.Pane.Radius, layer.Spread)
	})
}

func (a *app) paneSizeLabel(session *remoteSession) string {
	return fmt.Sprintf("%d × %d · 演示", session.columns, session.rows)
}
