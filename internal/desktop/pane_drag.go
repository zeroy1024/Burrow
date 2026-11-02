package desktop

import (
	"fmt"
	"slices"
	"time"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

type paneTransfer struct {
	tab     *workspaceTab
	session *remoteSession
	fromTab bool
}

func paneDropCanSwap(value any) bool {
	source, ok := value.(paneTransfer)
	return ok && !source.fromTab
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
	dragTabLanding
)

type paneDragState struct {
	afterCancel        func()
	pressed            bool
	kind               paneDragKind
	phase              paneDragPhase
	session            *remoteSession
	originTab          *workspaceTab
	origin, returnRect ui.Rect
	grab               ui.Point
	detached           bool
	order              []*workspaceTab
	returnStarted      time.Time
	overTabs           bool
	tabRect, paneRect  ui.Rect
	landingTab         *workspaceTab
	morphSeed          bool
	viewport           terminalDragViewport
}

const paneTabSlotMotion = "pane-tab-slot"

// MyGo has no public value-drag cancellation API. Keep its pressed source
// mounted until release, reject the drop, then run one requested action.
func (a *app) deferWorkspaceDragAction(action func()) bool {
	if d := a.drag; d != nil && d.kind != dragHost && (d.phase == dragActive || d.pressed) {
		if d.afterCancel == nil {
			d.afterCancel = action
		}
		d.phase = dragActive
		return true
	}
	if d := a.files.drag; d != nil {
		if d.afterCancel == nil {
			d.afterCancel = action
		}
		return true
	}
	return false
}

func (a *app) paneDragHandle(c *ui.Context, source ui.Element, transfer paneTransfer) {
	if a.activeSection() != sectionSSH {
		return
	}
	t := theme.Current()
	if transfer.session.closing {
		return
	}
	if d := a.drag; d != nil && d.phase == dragActive && transfer.fromTab &&
		d.kind == dragTab && d.originTab == transfer.tab {
		transfer.session = d.session
	}
	if d := a.drag; d != nil &&
		(d.kind == dragTab && transfer.fromTab && d.originTab == transfer.tab ||
			d.kind == dragPane && !transfer.fromTab && d.session == transfer.session) {
		d.pressed = source.Pressed()
	}
	// MyGo v0.7.3 fixes its native threshold at 4 DIPs; it exposes no hold-delay option.
	source.Drag(transfer).Cursor(ui.CursorDefault).FocusRing(false)
	if transfer.fromTab {
		source.DrawOver(func(_ *ui.Painter, _ ui.Rect) {
			if transfer.tab == a.currentTab() {
				a.terminalCapture.tabHandle, a.terminalCapture.tabSource = source.Bounds(), transfer.session
			}
		})
	} else {
		source.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
				a.captureTerminalPress(transfer.session, false)
			}
			if ev.Kind == ui.InputPointerUp && ev.Button == 0 && a.drag == nil {
				a.clearTerminalPreviews()
			}
			return false
		})
	}
	if capture := &a.terminalCapture; capture.pressed && capture.session == transfer.session && capture.fromTab == transfer.fromTab {
		source.Bind(&capture.handle)
		capture.bound = true
	}
	if !source.Dragging() {
		return
	}
	source.Cursor(ui.CursorGrabbing)
	a.dragSeen = true
	if a.drag != nil {
		if a.drag.phase == dragActive {
			return
		}
		a.drag = nil
	}
	r := transfer.session.frame.Bounds(c)
	kind := dragPane
	if transfer.fromTab {
		kind, r = dragTab, transfer.tab.frame.Bounds(c)
	}
	d := &paneDragState{
		kind: kind, session: transfer.session, originTab: transfer.tab, origin: r,
		grab: ui.Point{X: a.pointer.X - r.X, Y: a.pointer.Y - r.Y}, order: []*workspaceTab{}, morphSeed: kind == dragPane,
	}
	if kind == dragPane {
		d.viewport = terminalDragViewportFor(transfer.session)
	}
	if transfer.fromTab {
		d.order = slices.Clone(a.tabs)
		bar := a.tabScroll.Bounds(c)
		d.grab = ui.Point{X: a.tabPress.X + bar.X - r.X, Y: a.tabPress.Y + bar.Y - r.Y}
		d.detached = a.pointer.Y > bar.Y+bar.H+t.C.Desktop.TabDetachDistance
	}
	a.drag = d
	a.terminalCapture.pressed = false
}

func (a *app) tabStripTarget(c *ui.Context, parts uikit.TabsResult, add ui.Element) {
	if a.activeSection() != sectionSSH {
		return
	}
	strip := parts.Scroller
	t := theme.Current()
	animation := a.positionAnimation(ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.TabDisplace)})
	// Keep the transition registered outside a drag as well, so yielding and
	// returning to the original position use the same continuous spring.
	add.Transition(ui.ElementTransition{Position: true, Animation: &animation})
	if a.drag != nil && a.drag.afterCancel != nil {
		return
	}
	source, over := ui.DragOver[paneTransfer](strip)
	dropped, released := ui.Drop[paneTransfer](strip)
	if released {
		source = dropped
	}
	canExtract := (over || released) && !source.fromTab && source.tab.canSeparateSessions()
	paneWidth := float32(0)
	if canExtract {
		paneWidth = uikit.SessionTabWidth(a.paneTabProps(c, source.session))
	}
	index, slotX := a.paneTabInsertion(c, parts, add, paneWidth)
	if !over && !released {
		return
	}
	d := a.drag
	if source.fromTab && (d == nil || d.kind != dragTab || d.detached || d.phase != dragActive) {
		return
	}
	if !source.fromTab && !canExtract {
		strip.Cursor(ui.CursorNotAllowed)
		return
	}
	r := strip.Bounds()
	var delta float32
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
	if !source.fromTab {
		if d == nil || d.phase != dragActive {
			origin := source.session.frame.Bounds(c)
			d = &paneDragState{kind: dragPane, session: source.session, originTab: source.tab, origin: origin,
				grab: ui.Point{X: t.C.DragGhost.Width / 2, Y: t.C.Desktop.DragGrabY}, morphSeed: true,
				paneRect: ui.Rect{X: origin.X, Y: origin.Y, W: t.C.DragGhost.Width, H: t.C.DragGhost.Height},
				viewport: terminalDragViewportFor(source.session)}
			a.drag = d
		}
		d.overTabs = true
		d.tabRect = ui.Rect{X: slotX,
			Y: r.Y + (r.H-t.C.Tab.Height)/2, W: paneWidth, H: t.C.Tab.Height}
		strip.Cursor(ui.CursorGrabbing)
		if released {
			a.pendingDock = func() {
				tab := a.detachPaneToTab(source.tab, source.session, index, a.workArea.Bounds(c))
				if tab == nil {
					return
				}
				// The inserted real tab now owns the space; do not retain a
				// second animated tail after the model has been committed.
				parts.Row.Jump(paneTabSlotMotion, 0)
				startPaneReveal(source.session)
				d.phase, d.landingTab = dragTabLanding, tab
			}
		}
		return
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

func (a *app) paneTabInsertion(c *ui.Context, parts uikit.TabsResult, add ui.Element, width float32) (int, float32) {
	t := theme.Current()
	index, x := len(a.tabs), parts.Scroller.Bounds().X-a.tabScroll.X
	slotX, total := x, add.Bounds().W
	for i, tab := range a.tabs {
		w := tab.frame.Bounds(c).W
		if index == len(a.tabs) && a.pointer.X < x+w/2 {
			index, slotX = i, x
		}
		x += w + t.C.Titlebar.TabsGap
		total += w + t.C.Titlebar.TabsGap
	}
	if index == len(a.tabs) {
		slotX = x
	}
	space := float32(0)
	if width > 0 {
		space = width + t.C.Titlebar.TabsGap
		for i, tab := range a.tabs {
			if i >= index {
				c.Resolve(tab.frame).Left(space)
			}
		}
		add.Left(space)
	}
	// Keep the scroll range while the same spring restores the neighbours.
	// Removing it immediately would clamp scrollX and visibly jump the strip.
	animation := a.positionAnimation(ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.TabDisplace)})
	gap := parts.Row.AnimateTo(paneTabSlotMotion, space, animation)
	if space > 0 || !parts.Row.Settled(paneTabSlotMotion) {
		parts.Row.Width(total + max(0, gap)).Shrink(0)
	}
	return index, slotX
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
	return d != nil && d.phase == dragActive && d.session == session &&
		d.kind != dragHost && (d.kind == dragPane || d.detached)
}

func (a *app) finishWorkspaceDrag(c *ui.Context) {
	d := a.drag
	if d == nil || d.phase != dragActive {
		return
	}
	if d.afterCancel != nil {
		if d.pressed {
			return
		}
		if d.kind == dragTab {
			active := a.currentTab()
			a.tabs = slices.Clone(d.order)
			a.activeTab = slices.Index(a.tabs, active)
		}
		a.drag = nil
		a.clearTerminalPreviews()
		d.afterCancel()
		c.AnimationFrame()
		return
	}
	if a.dragSeen {
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
	if d == nil || d.phase == dragLanded {
		return
	}
	if d.kind == dragPane {
		a.paneTabDragPreview(c, d)
		return
	}
	t := theme.Current()
	p := t.C.DragGhost
	tabPreview := d.kind == dragTab && !d.detached
	r := ui.Rect{W: p.Width, H: p.Height}
	grab := ui.Point{X: min(d.grab.X, r.W/2), Y: t.C.Desktop.DragGrabY}
	radius := p.Radius
	if tabPreview {
		r, grab, radius = d.origin, d.grab, t.C.Tab.Radius
	}
	r.X, r.Y = a.pointer.X-grab.X, a.pointer.Y-grab.Y
	animation := ui.Animation{Duration: t.C.Desktop.InstantDuration}
	visible := float32(1)
	if d.phase == dragReturning {
		r.X, r.Y = d.returnRect.X, d.returnRect.Y
		animation = ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.Cancel)}
		visible = 0
	}
	// Return the compact preview to its source without stretching it into a pane.
	ghost := uikit.DragGhost(c, uikit.DragGhostProps{Key: d, Rect: r, Radius: radius, Animation: animation})
	uikit.StyleDragGhost(ghost, visible, t.C.ReorderMotion.Cancel.Duration)
	ghost.Children(func() {
		if tabPreview {
			uikit.DragTabGhost(c, radius).Children(func() {
				if d.originTab.root.count() > 1 {
					uikit.Icon(c, uikit.IconGrid, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneSecondary}).PassThrough()
				} else {
					uikit.ConnectionDot(c, connectionIndicatorState(d.session.state), t.C.Tab.DotSize)
				}
				uikit.DragTabTitle(c, d.session.endpoint.name)
			})
			return
		}
		a.paneDragPreview(c, d.session)
	})
}

func (a *app) paneTabProps(c *ui.Context, session *remoteSession) uikit.SessionTabProps {
	return uikit.SessionTabProps{
		Key: session.id, Title: session.endpoint.name, Selected: true,
		State: connectionIndicatorState(session.state), CloseIcon: closeIcon,
		ReduceMotion: a.reduceMotion(c),
	}
}

func (a *app) paneTabDragPreview(c *ui.Context, d *paneDragState) {
	t := theme.Current()
	p := t.C.DragGhost
	targetKnown := false
	if d.phase == dragTabLanding {
		if slices.Index(a.tabs, d.landingTab) < 0 {
			a.drag = nil
			return
		}
		// The newly inserted tab gets its first committed bounds after layout.
		// Until then retain the real insertion slot shown during the drag.
		if rect := d.landingTab.frame.Bounds(c); rect.W > 0 && rect.H > 0 {
			d.tabRect, targetKnown = rect, true
		}
	}
	pane := ui.Rect{W: p.Width, H: p.Height}
	switch d.phase {
	case dragActive:
		pane.X, pane.Y = a.pointer.X-min(d.grab.X, p.Width/2), a.pointer.Y-t.C.Desktop.DragGrabY
		d.paneRect = pane
	case dragReturning:
		pane.X, pane.Y = d.returnRect.X, d.returnRect.Y
	case dragTabLanding:
		pane = d.paneRect
	}
	tabTarget := d.overTabs || d.phase == dragTabLanding
	tab := d.tabRect
	if d.phase == dragActive {
		width := uikit.SessionTabWidth(a.paneTabProps(c, d.session))
		// Hovering changes only the shape held at the pointer. Insertion and
		// neighbour movement begin on release, as for a tab dragged into Shell.
		tab = ui.Rect{X: a.pointer.X - min(d.grab.X, width/2),
			Y: a.pointer.Y - min(d.grab.Y, t.C.Tab.Height/2), W: width, H: t.C.Tab.Height}
	}
	seed := d.morphSeed
	if seed {
		// A fast native drop can arrive before the first drag preview frame.
		// Seed its real pane view once so it still morphs instead of popping in.
		tabTarget, d.morphSeed = false, false
		c.AnimationFrame()
	}
	ghost := uikit.DragMorphGhost(c, uikit.DragMorphGhostProps{
		Key: d, PaneRect: pane, TabRect: tab, Tab: tabTarget,
		Settling: d.phase != dragActive, Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func() {
		background := termtheme.TerminalBackground(a.sessionScheme(d.session), t.C.Pane.Bg, a.terminal.prefs.Background)
		surface := terminalSurface(background)
		uikit.DragPaneGhost(c, background, surface).Children(func() {
			uikit.DragPaneHeader(c, surface).Children(func() {
				uikit.StatusDot(c, connectionIndicatorState(d.session.state), uikit.StatusDotProps{
					Motion: uikit.MotionOverride(a.reduceMotion(c)),
				}).PassThrough()
				uikit.DragPaneTitle(c, d.session.endpoint.name, surface)
			})
			width := p.Width - p.PaddingLeft - p.PaddingRight
			height := p.Height - 2*p.PaddingY - p.HeaderHeight - t.Space.N8
			a.terminalDragContents(c, d.session, d.viewport, width, height)
		})
	}, func() {
		uikit.SessionTabPreview(c, a.paneTabProps(c, d.session)).FillWidth()
	})
	visible := float32(1)
	if d.phase == dragReturning {
		visible = 0
	}
	uikit.StyleDragGhost(ghost, visible, t.C.ReorderMotion.Cancel.Duration)
	if d.phase == dragTabLanding && !seed && targetKnown && uikit.DragMorphSettled(ghost) {
		d.phase = dragLanded
		c.AnimationFrame()
	}
}

func (a *app) paneDragPreview(c *ui.Context, session *remoteSession) {
	t := theme.Current()
	background := termtheme.TerminalBackground(a.sessionScheme(session), t.C.Pane.Bg, a.terminal.prefs.Background)
	surface := terminalSurface(background)
	// PassThrough is per element in MyGo; every preview descendant must opt out of hit testing.
	uikit.DragPaneGhost(c, background, surface).Children(func() {
		uikit.DragPaneHeader(c, surface).Children(func() {
			uikit.ConnectionDot(c, connectionIndicatorState(session.state), t.C.Tab.DotSize)
			uikit.DragPaneTitle(c, session.endpoint.name, surface)
		})
		uikit.DragSkeleton(c, surface)
	})
}

func (a *app) paneDrop(c *ui.Context, pane ui.Element, target *remoteSession, catalog bool) {
	if a.drag != nil && a.drag.afterCancel != nil {
		return
	}
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
	edgeOnly := tab.root.count() == 1 || !paneDropCanSwap(payload)
	zone := zoneAt(x, y, pane.Bounds().W, pane.Bounds().H, edgeOnly)
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
				a.page().catalogDrag.externalDrop = true
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
		return plan, paneDropCanSwap(value)
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
		// Header and sidebar drops must preserve every destination session.
		// Only an existing Shell pane may fall back to swapping positions.
		return paneDropPlan{zone: dropCenter, target: target.layoutRect}, paneDropCanSwap(value)
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
		tab.root.find(target).insert(session, zone)
	case paneTransfer:
		session = source.session
		if zone == dropCenter && !source.fromTab {
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
			tab.root.find(target).insert(session, zone)
		}
	}
	a.focusSession(tab, session)
	a.setPaneZoom(tab, false)
	a.markPaneLayout(tab, t.C.ReorderMotion.PaneDrop.Duration)
	startPaneReveal(session)
	if a.drag != nil {
		a.drag.session, a.drag.phase = session, dragLanded
	}
}

func (a *app) paneSizeLabel(session *remoteSession) string {
	return fmt.Sprintf("%d × %d", session.columns, session.rows)
}
