package desktop

import (
	"fmt"
	"math"
	"slices"
	"time"

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
}

func (a *app) paneDragHandle(c *ui.Context, source ui.Element, transfer paneTransfer) {
	if transfer.session.closing {
		return
	}
	if d := a.drag; d != nil && transfer.fromTab && d.kind == dragTab && d.originTab == transfer.tab {
		transfer.session = d.session
	}
	source.Drag(transfer).Cursor(ui.CursorGrab).FocusRing(false)
	if !source.Dragging() {
		return
	}
	a.dragSeen = true
	if a.drag != nil {
		return
	}
	area := a.workArea.Bounds(c)
	r := transfer.session.frame.Bounds(c)
	kind := dragPane
	if transfer.fromTab {
		kind, r = dragTab, transfer.tab.frame.Bounds(c)
	}
	size := ui.Rect{W: r.W, H: r.H}
	if transfer.fromTab {
		size.W, size.H = min(360, area.W), min(260, area.H)
	}
	d := &paneDragState{
		kind: kind, session: transfer.session, originTab: transfer.tab, origin: r, paneRect: size,
		grab: ui.Point{X: a.pointer.X - r.X, Y: a.pointer.Y - r.Y}, order: []*workspaceTab{},
	}
	d.paneImage = a.paneSnapshot(transfer.session, size.W, size.H)
	if transfer.fromTab {
		d.order = slices.Clone(a.tabs)
		d.tabImage = a.tabSnapshot(transfer.tab, r.W, r.H)
		bar := a.tabScroll.Bounds(c)
		d.grab = ui.Point{X: a.tabPress.X + bar.X - r.X, Y: a.tabPress.Y + bar.Y - r.Y}
		d.detached = a.pointer.Y > bar.Y+bar.H+8
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
	if over && a.pointer.X < r.X+28 {
		delta = -4
	}
	if over && a.pointer.X > r.X+r.W-28 {
		delta = 4
	}
	if delta != 0 {
		next := min(max(a.tabScroll.X+delta, 0), a.tabScroll.MaxX)
		if next != a.tabScroll.X {
			a.tabScroll.X = next
			c.After(16 * time.Millisecond)
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
		x += width + sessionTabGap
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
	r := tab.frame.Bounds(c)
	r.X = a.tabScroll.Bounds(c).X - a.tabScroll.X
	for _, before := range a.tabs[:slices.Index(a.tabs, tab)] {
		r.X += before.frame.Bounds(c).W + sessionTabGap
	}
	return r
}

func (a *app) hostDragHandle(c *ui.Context, source ui.Element, value hostDrag) {
	source.Drag(value).Cursor(ui.CursorGrab).FocusRing(false)
	if !source.Dragging() {
		return
	}
	a.dragSeen = true
	if a.drag != nil {
		return
	}
	area, r := a.workArea.Bounds(c), source.Bounds()
	session := &remoteSession{host: value.id, state: connected, output: []string{}}
	size := ui.Rect{W: min(360, area.W), H: min(260, area.H)}
	a.drag = &paneDragState{
		kind: dragHost, session: session, origin: r, paneRect: size, detached: true,
		grab: ui.Point{X: min(30, size.W/2), Y: 18}, order: []*workspaceTab{},
		paneImage: a.paneSnapshot(session, size.W, size.H),
	}
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
	r, image := d.paneRect, d.paneImage
	if d.kind == dragTab && !d.detached {
		r, image = d.origin, d.tabImage
	}
	r.X, r.Y = a.pointer.X-d.grab.X, a.pointer.Y-d.grab.Y
	if d.kind == dragTab && d.detached {
		r.X, r.Y = a.pointer.X-min(d.grab.X, r.W/2), a.pointer.Y-18
	}
	animation := ui.Animation{Duration: time.Nanosecond}
	scale, rotate, opacity := float32(.96), float32(-1), float32(.9)
	if d.phase == dragReturning {
		r, animation, scale, rotate, opacity = d.returnRect, paneSpring, 1, 0, 1
		c.After(220*time.Millisecond - min(c.Now().Sub(d.returnStarted), 220*time.Millisecond))
	}
	if d.phase == dragLanded && !d.returnStarted.IsZero() {
		area := a.workArea.Bounds(c)
		r = d.session.layoutRect
		r.X, r.Y = r.X+area.X, r.Y+area.Y
		animation, scale, rotate, opacity = paneSpring, 1, 0, 1
		c.After(220*time.Millisecond - min(c.Now().Sub(d.returnStarted), 220*time.Millisecond))
	}
	transition := ui.ElementTransition{Position: true, Size: true, Animation: &animation}
	ghost := ui.Box(c).Key("workspace-drag-preview").Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Radius(terminalPaneRadius).PassThrough().Transition(transition)
	ghost.Scale(ghost.Animate("lift-scale", scale, 220*time.Millisecond), ghost.Animate("lift-scale-y", scale, 220*time.Millisecond)).
		Rotate(ghost.Animate("lift-angle", rotate, 220*time.Millisecond)).
		Opacity(ghost.Animate("lift-alpha", opacity, 220*time.Millisecond)).Shadow(
		0,
		16,
		40,
		0,
		ui.RGB(0, 0, 0).Alpha(.18),
	)
	if c.Preferences().ReduceMotion && d.phase != dragActive && !d.returnStarted.IsZero() {
		progress := min(1, float32(c.Now().Sub(d.returnStarted))/float32(220*time.Millisecond))
		ghost.Opacity(.9 + .1*progress)
		if progress < 1 {
			c.AnimationFrame()
		}
	}
	ghost.Children(func() { ui.Image(c, image).Fill().Fit(ui.Cover).Radius(terminalPaneRadius).PassThrough() })
}

func (a *app) paneSnapshot(session *remoteSession, width, height float32) *ui.Bitmap {
	h := a.hosts[session.host]
	position := session.scroll.offset.Y
	if position >= session.scroll.offset.MaxY {
		position = math.MaxFloat32
	}
	offset := ui.ScrollState{Y: position}
	image := ui.Render(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Column(c).Fill().Radius(terminalPaneRadius).Clip().Background(ui.Hex("#fbfbfc")).Children(func() {
			ui.Scroll(c.Key("preview-output")).Fill().TrackScroll(&offset).Padding(
				38,
				14,
				50,
				14,
			).ScrollbarInsets(0, -12, 0, 0).Font("Menlo, monospace").FontSize(11).Children(func() {
				a.terminalLines(c, session, false)
			})
			ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(36).Padding(0, 12).Gap(6).
				Background(ui.Hex(cardBackground).Alpha(.96)).Children(func() {
				statusDot(c, session.state, 6)
				ui.Text(c, h.name).FontSize(12).FontWeight(600)
			})
			ui.Row(c).Absolute().Bottom(0).Left(0).Right(0).Height(28).Padding(0, 12).
				Background(ui.Hex(cardBackground).Alpha(.96)).Children(func() {
				ui.Text(c, h.user+"@"+h.address).FontSize(10).TextColor(a.theme.TextMuted)
			})
		})
	}, int(math.Ceil(float64(width))), int(math.Ceil(float64(height))), 1)
	return ui.NewBitmap(image)
}

func (a *app) tabSnapshot(tab *workspaceTab, width, height float32) *ui.Bitmap {
	image := ui.Render(func(c *ui.Context) {
		c.SetTheme(a.theme)
		ui.Row(c).Fill().Padding(0, 9).Gap(6).Radius(8).Background(ui.Hex(cardBackground)).Children(func() {
			statusDot(c, tab.focused.state, 6)
			ui.Text(c, a.hosts[tab.focused.host].name).FontSize(12).FontWeight(550)
		})
	}, int(math.Ceil(float64(width))), int(math.Ceil(float64(height))), 1)
	return ui.NewBitmap(image)
}

func (a *app) paneDrop(c *ui.Context, pane ui.Element, target *remoteSession) {
	payload, over := ui.DragOver[any](pane)
	value, dropped := ui.Drop[any](pane)
	if dropped {
		payload = value
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
	if over {
		r := plan.target
		r.X, r.Y, r.W, r.H = r.X+4, r.Y+4, r.W-8, r.H-8
		switch plan.zone {
		case dropLeft:
			r.W = (r.W - paneSpacing) / 2
		case dropRight:
			r.W = (r.W - paneSpacing) / 2
			r.X += r.W + paneSpacing
		case dropTop:
			r.H = (r.H - paneSpacing) / 2
		case dropBottom:
			r.H = (r.H - paneSpacing) / 2
			r.Y += r.H + paneSpacing
		}
		a.dockPreview = &r
	}
	if dropped {
		a.pendingDock = func() { a.dockSession(tab, target, payload, plan) }
	}
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
	a.markPaneLayout(tab, 220*time.Millisecond)
	if a.drag != nil {
		a.drag.session, a.drag.phase = session, dragLanded
	}
}

func (a *app) dragPlaceholder(pane ui.Element) {
	pane.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, ui.RGB(0, 0, 0).Alpha(.02), 10)
		p.StrokeDashed(r, ui.RGB(0, 0, 0).Alpha(.12), 10, 1.5)
	})
}

func (a *app) paneSizeLabel(session *remoteSession) string {
	return fmt.Sprintf("%d × %d · 演示", session.columns, session.rows)
}
