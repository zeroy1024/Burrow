package desktop

import (
	"slices"
	"strings"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func catalogDisplaceAnimation() ui.Animation {
	return ui.Animation{Spring: tokenSpring(theme.Current().C.ReorderMotion.Displace)}
}

func catalogLandingAnimation() ui.Animation {
	return ui.Animation{Spring: tokenSpring(theme.Current().C.ReorderMotion.Drop)}
}

func catalogColorAnimation() ui.Animation {
	return tokenTween(theme.Current().C.SiderItem.Transition)
}

type catalogDragKind uint8

const (
	catalogHostDrag catalogDragKind = iota
	catalogGroupDrag
)

type catalogDragPhase uint8

const (
	catalogDragging catalogDragPhase = iota
	catalogSettling
	catalogCanceling
)

type catalogKey struct {
	kind catalogDragKind
	id   int
}

type catalogItemMotion struct {
	offset ui.Point
	count  int
	down   bool
}

type catalogSlot struct {
	key         catalogKey
	group       *hostGroup
	rect        ui.Rect
	headerRect  ui.Rect
	placeholder bool
}

type catalogDragState struct {
	kind             catalogDragKind
	phase            catalogDragPhase
	key              catalogKey
	host             hostDrag
	group            *hostGroup
	origin           ui.Rect
	originContentY   float32
	visualY          float32
	velocityY        float32
	targetGroup      *hostGroup
	targetIndex      int // Index after removing the source, in the unfiltered group.
	held             bool
	pressed          bool
	released         bool
	awaitingRelease  bool
	externalDrop     bool
	started          time.Time
	lastFrame        time.Time
	hoverGroup       *hostGroup
	hoverStarted     time.Time
	autoExpanded     map[*hostGroup]bool
	offset           ui.Point
	settleSeed       bool
	settleBase       ui.Rect
	settleStarted    time.Time
	finished         bool
	placeholderReady bool
	collapseFrames   int
	collapseSample   time.Time
	workspacePreview *paneDragState
}

func (a *app) catalogMotion(key catalogKey) *catalogItemMotion {
	if a.page().catalogItems == nil {
		a.page().catalogItems = map[catalogKey]*catalogItemMotion{}
	}
	if a.page().catalogItems[key] == nil {
		a.page().catalogItems[key] = &catalogItemMotion{}
	}
	return a.page().catalogItems[key]
}

func (a *app) beginCatalogFrame(c *ui.Context) {
	if a.page().catalogFlash != nil && !a.page().catalogFlashUntil.IsZero() && !c.Now().Before(a.page().catalogFlashUntil) {
		a.page().catalogFlash = nil
		a.page().catalogFlashSeed, a.page().catalogFlashUntil = 0, time.Time{}
	}
	d := a.page().catalogDrag
	if d == nil {
		return
	}
	d.held, d.pressed, d.released = false, false, false
	if d.phase == catalogDragging && c.Shortcut(0, ui.KeyEscape) {
		a.settleCatalogDrag(c, false)
	}
}

func (a *app) catalogHostTransfer() (hostDrag, bool, bool) {
	d := a.page().catalogDrag
	if d == nil || d.kind != catalogHostDrag || d.phase != catalogDragging {
		return hostDrag{}, false, false
	}
	return d.host, true, d.released
}

func (a *app) syncCatalogWorkspacePreview(c *ui.Context) {
	t := theme.Current()
	d := a.page().catalogDrag
	if d == nil || d.kind != catalogHostDrag || d.phase != catalogDragging {
		return
	}
	if containsCatalogPoint(a.page().catalogScroll.Bounds(c), a.pointer) {
		if d.workspacePreview != nil && a.drag == d.workspacePreview {
			a.drag = nil
		}
		return
	}
	if d.workspacePreview == nil {
		session := &remoteSession{
			host: d.host.id, endpoint: a.sshCatalog.hosts[d.host.id], state: disconnected,
		}
		d.workspacePreview = &paneDragState{
			kind: dragHost, session: session, origin: d.origin, detached: true,
			grab: ui.Point{X: t.C.Desktop.DragGrabX, Y: t.C.Desktop.DragGrabY}, order: []*workspaceTab{},
		}
	}
	a.drag, a.dragSeen = d.workspacePreview, d.held
}

func (a *app) trackCatalogDrag(c *ui.Context, row ui.Element, slot catalogSlot) {
	t := theme.Current()
	motion := a.catalogMotion(slot.key)
	d := a.page().catalogDrag
	active := d != nil && d.key == slot.key
	_, _, held := row.Dragged()
	_, releaseVelocityY, released := row.DragEnded()
	settlingSource := active && d.phase != catalogDragging
	blockGesture := settlingSource && d.awaitingRelease
	if settlingSource {
		d.pressed = held
		if blockGesture && !held {
			d.awaitingRelease = false
		}
	}
	idle := !held && !released
	options := ui.DraggableOptions{Axis: ui.Vertical, Animation: catalogLandingAnimation()}
	if idle || blockGesture {
		// Only the root overlay animates a landing or return. Keep the native
		// gesture current, and clear its own offset before another press catches it.
		motion.offset = ui.Point{}
		options.Bounds, options.Rigid = &ui.Rect{}, true
		options.Animation = ui.Animation{Duration: t.C.Desktop.InstantDuration}
	}
	// MyGo v0.7.3 exposes neither a threshold nor a hold delay in DraggableOptions.
	row.Draggable(&motion.offset, options)
	offset := row.DragOffset()
	if settlingSource && (blockGesture || idle) {
		return
	}
	dragging := row.Dragging()
	// DragOffset also includes spring motion after release. Only an actual
	// release can identify a quick drag that began and ended between builds.
	continuing := active && !settlingSource
	endedDrag := released && (continuing || offset.Y != 0)
	if !dragging && !endedDrag {
		return
	}
	if d != nil && d.phase != catalogDragging {
		// A new gesture takes over from a previous landing or return flight.
		a.catalogMotion(d.key).offset = ui.Point{}
		a.page().catalogDrag, d = nil, nil
	}
	if d == nil {
		if a.drag != nil && a.drag.phase != dragActive {
			a.drag = nil
		}
		origin := row.Bounds()
		viewport := a.page().catalogScroll.Bounds(c)
		d = &catalogDragState{
			kind: slot.key.kind, key: slot.key, group: slot.group,
			origin: origin, originContentY: origin.Y - viewport.Y - t.C.Catalog.ListPaddingY + a.page().catalogScroll.Y,
			visualY:     origin.Y + offset.Y,
			targetGroup: slot.group, started: c.Now(), lastFrame: c.Now(),
			autoExpanded: map[*hostGroup]bool{},
		}
		if d.kind == catalogHostDrag {
			d.host = hostDrag{id: hostID(slot.key.id), group: slot.group}
			d.targetIndex = slices.Index(slot.group.hosts, d.host.id)
		} else {
			d.targetIndex = slices.Index(a.catalog().groups, slot.group)
			if c.Preferences().ReduceMotion {
				d.collapseFrames = 2
			}
		}
		a.page().catalogDrag = d
		c.AnimationFrame()
	}
	if d.key == slot.key && d.phase == catalogDragging {
		d.held, d.pressed, d.released = dragging, dragging, released
		if dragging {
			row.Cursor(ui.CursorGrabbing)
			_, d.velocityY = row.DragVelocity()
		} else {
			d.velocityY = releaseVelocityY
		}
		// The source remains anchored in content coordinates while the scroll
		// moves beneath it. Its gesture offset is independent of preview order.
		d.visualY = d.origin.Y + offset.Y
	}
}

func (a *app) catalogExpanded(group *hostGroup) bool {
	d := a.page().catalogDrag
	if d != nil && d.phase == catalogDragging {
		if d.kind == catalogGroupDrag && d.collapseFrames >= 2 {
			return false
		}
		if d.autoExpanded[group] {
			return true
		}
	}
	return !a.groupCollapsed(group) || strings.TrimSpace(a.page().query) != ""
}

func (a *app) catalogMatches(group *hostGroup, id hostID) bool {
	query := strings.ToLower(strings.TrimSpace(a.page().query))
	h := a.catalog().hosts[id]
	text := h.name + " " + h.user + " " + h.address + " " + systems[h.os].name + " " + group.name
	return strings.Contains(strings.ToLower(text), query)
}

func (a *app) catalogPreviewHosts(group *hostGroup) []hostID {
	ids := slices.Clone(group.hosts)
	d := a.page().catalogDrag
	if d != nil && d.kind == catalogHostDrag && d.phase == catalogDragging && group == d.host.group {
		index := slices.Index(ids, d.host.id)
		ids = slices.Delete(ids, index, index+1)
	}
	return ids
}

// Every header and host is a sibling of the same Column. MyGo keys include
// their parent; nesting hosts in group columns would lose identity across groups.
func (a *app) catalogPlan(c *ui.Context) []catalogSlot {
	t := theme.Current()
	viewport := a.page().catalogScroll.Bounds(c)
	y := viewport.Y + t.C.Catalog.ListPaddingY - a.page().catalogScroll.Y
	width := max(0, viewport.W-2*t.C.Catalog.ListPaddingX)
	slots := []catalogSlot{}
	d := a.page().catalogDrag
	draggingGroup := d != nil && d.phase == catalogDragging && d.kind == catalogGroupDrag
	groups := slices.Clone(a.catalog().groups)
	if draggingGroup {
		index := slices.Index(groups, d.group)
		groups = slices.Delete(groups, index, index+1)
		groups = slices.Insert(groups, d.targetIndex, d.group)
	}
	for _, group := range groups {
		ids := a.catalogPreviewHosts(group)
		visible := slices.ContainsFunc(group.hosts, func(id hostID) bool { return a.catalogMatches(group, id) })
		if !visible && strings.TrimSpace(a.page().query) != "" && (d == nil || d.targetGroup != group) {
			continue
		}
		if len(slots) > 0 {
			y += t.C.Catalog.GroupGap
		}
		slot := catalogSlot{
			key: catalogKey{kind: catalogGroupDrag, id: group.id}, group: group,
			rect:        ui.Rect{X: viewport.X + t.C.Catalog.ListPaddingX, Y: y, W: width, H: t.C.GroupHeader.Height},
			placeholder: draggingGroup && group == d.group,
		}
		slot.headerRect = slot.rect
		slots = append(slots, slot)
		y += t.C.GroupHeader.Height
		if !a.catalogExpanded(group) {
			continue
		}
		for position := 0; position <= len(ids); position++ {
			if d != nil && d.phase == catalogDragging && d.kind == catalogHostDrag &&
				d.targetGroup == group && d.targetIndex == position {
				y += t.C.Catalog.RowGap
				slots = append(slots, catalogSlot{
					key: d.key, group: group, placeholder: true,
					rect: ui.Rect{X: viewport.X + t.C.Catalog.ListPaddingX, Y: y, W: width, H: t.C.Catalog.RowHeight},
				})
				y += t.C.Catalog.RowHeight
			}
			if position == len(ids) || !a.catalogMatches(group, ids[position]) {
				continue
			}
			y += t.C.Catalog.RowGap
			slots = append(slots, catalogSlot{
				key: catalogKey{kind: catalogHostDrag, id: int(ids[position])}, group: group,
				rect: ui.Rect{X: viewport.X + t.C.Catalog.ListPaddingX, Y: y, W: width, H: t.C.Catalog.RowHeight},
			})
			y += t.C.Catalog.RowHeight
		}
	}
	return slots
}

func containsCatalogPoint(r ui.Rect, p ui.Point) bool {
	return p.X >= r.X && p.X <= r.X+r.W && p.Y >= r.Y && p.Y <= r.Y+r.H
}

func (a *app) updateCatalogDestination(c *ui.Context) {
	d := a.page().catalogDrag
	if !containsCatalogPoint(a.page().catalogScroll.Bounds(c), a.pointer) {
		d.hoverGroup, d.hoverStarted = nil, time.Time{}
		return
	}
	// The first drag frame still has the pre-drag slots. Exclude the source
	// by its key even before the preview replaces it with a placeholder.
	if d.kind == catalogGroupDrag {
		groups := slices.Clone(a.catalog().groups)
		from := slices.Index(groups, d.group)
		groups = slices.Delete(groups, from, from+1)
		index := d.targetIndex
		for _, slot := range a.page().catalogSlots {
			if slot.placeholder || slot.key == d.key {
				continue
			}
			if slot.key.kind != catalogGroupDrag {
				continue
			}
			candidate := slices.Index(groups, slot.group)
			if a.pointer.Y < slot.rect.Y+slot.rect.H/2 {
				index = min(index, candidate)
				break
			}
			index = max(index, candidate+1)
		}
		d.targetIndex = index
		return
	}
	var group *hostGroup
	var header *hostGroup
	var first *hostGroup
	var firstY, groupY float32
	for _, slot := range a.page().catalogSlots {
		if slot.key.kind != catalogGroupDrag {
			continue
		}
		if first == nil || slot.headerRect.Y < firstY {
			first, firstY = slot.group, slot.headerRect.Y
		}
		above := slot.headerRect.Y <= a.pointer.Y
		if above && (group == nil || slot.headerRect.Y >= groupY) {
			group, groupY = slot.group, slot.headerRect.Y
		}
		if containsCatalogPoint(slot.headerRect, a.pointer) {
			header = slot.group
		}
	}
	if header != nil {
		group = header
	}
	if group == nil {
		group = first
	}
	if group == nil {
		return
	}
	if d.hoverGroup != header {
		d.hoverGroup, d.hoverStarted = header, c.Now()
	}
	if header != nil && a.groupCollapsed(header) && !d.autoExpanded[header] {
		if c.Now().Sub(d.hoverStarted) >= theme.Current().C.GroupHeader.ExpandDelay {
			d.autoExpanded[header] = true
		} else {
			c.AnimationFrame()
		}
	}
	ids := a.catalogPreviewHosts(group)
	position := len(ids)
	if header == nil && a.catalogExpanded(group) {
		// Keep the position among hidden rows until a visible neighbor's
		// midpoint is crossed; filtering alone must not reorder the catalog.
		if group == d.targetGroup {
			position = d.targetIndex
		} else if group == d.host.group {
			position = slices.Index(group.hosts, d.host.id)
		}
		for _, slot := range a.page().catalogSlots {
			if slot.placeholder || slot.key == d.key {
				continue
			}
			if slot.group != group || slot.key.kind != catalogHostDrag {
				continue
			}
			candidate := slices.Index(ids, hostID(slot.key.id))
			if a.pointer.Y < slot.rect.Y+slot.rect.H/2 {
				position = min(position, candidate)
				break
			}
			position = max(position, candidate+1)
		}
	}
	d.targetGroup, d.targetIndex = group, position
}

func (a *app) scrollCatalogDrag(c *ui.Context) {
	t := theme.Current()
	d := a.page().catalogDrag
	now := c.Now()
	dt := float32(now.Sub(d.lastFrame).Seconds())
	d.lastFrame = now
	r := a.page().catalogScroll.Bounds(c)
	if !containsCatalogPoint(r, a.pointer) {
		return
	}
	zone := t.C.DropIndicator.AutoScrollZone
	var depth float32
	if a.pointer.Y < r.Y+zone {
		depth = -min(1, (r.Y+zone-a.pointer.Y)/zone)
	} else if a.pointer.Y > r.Y+r.H-zone {
		depth = min(1, (a.pointer.Y-(r.Y+r.H-zone))/zone)
	}
	speed := t.C.DropIndicator.AutoScrollMax * depth * depth
	if depth < 0 {
		speed = -speed
	}
	previous := a.page().catalogScroll.Y
	a.page().catalogScroll.Y = min(max(previous+speed*dt, 0), a.page().catalogScroll.MaxY)
	if a.page().catalogScroll.Y != previous {
		delta := a.page().catalogScroll.Y - previous
		for i := range a.page().catalogSlots {
			a.page().catalogSlots[i].rect.Y -= delta
			if a.page().catalogSlots[i].key.kind == catalogGroupDrag {
				a.page().catalogSlots[i].headerRect.Y -= delta
			}
		}
		c.AnimationFrame()
	}
}

func (a *app) finishCatalogDrag(c *ui.Context) {
	d := a.page().catalogDrag
	if d == nil {
		return
	}
	if d.phase != catalogDragging {
		if d.finished && !d.pressed {
			a.page().catalogDrag = nil
			c.AnimationFrame()
		}
		return
	}
	if d.held {
		a.scrollCatalogDrag(c)
		a.updateCatalogDestination(c)
		c.AnimationFrame()
		return
	}
	if !d.released {
		a.settleCatalogDrag(c, false)
		return
	}
	inside := containsCatalogPoint(a.page().catalogScroll.Bounds(c), a.pointer)
	if inside && !d.externalDrop {
		// Recompute at the release point, including a final pointer move that
		// arrived in the same frame as the release.
		a.updateCatalogDestination(c)
	}
	a.settleCatalogDrag(c, inside && !d.externalDrop)
}

func (a *app) settleCatalogDrag(c *ui.Context, commit bool) {
	t := theme.Current()
	d := a.page().catalogDrag
	d.phase = catalogCanceling
	d.awaitingRelease = !d.released
	if commit {
		err := a.commitCatalogMove(func() {
			if d.kind == catalogHostDrag {
				moveHost(d.host, d.targetGroup, d.targetIndex)
			} else {
				a.moveGroup(d.group, d.targetIndex)
			}
			for group := range d.autoExpanded {
				a.setGroupCollapsed(group, false)
			}
		})
		if err != nil {
			c.Toast("保存排序失败，已恢复原位置：" + err.Error())
		} else {
			d.phase = catalogSettling
		}
	}
	// Collapsed flags were never changed while dragging; cancellation simply
	// discards the temporary expansion map and preview order.
	d.hoverGroup, d.hoverStarted = nil, time.Time{}
	d.offset, d.settleSeed, d.settleStarted = ui.Point{}, true, c.Now()
	d.settleBase = d.origin
	for _, slot := range a.catalogPlan(c) {
		if slot.key == d.key {
			d.settleBase = slot.rect
			break
		}
		if d.kind == catalogHostDrag && slot.group == d.targetGroup && slot.key.kind == catalogGroupDrag {
			d.settleBase = ui.Rect{X: slot.rect.X, Y: slot.rect.Y, W: slot.rect.W, H: t.C.Catalog.RowHeight}
		}
	}
	d.offset.Y = d.visualY - d.settleBase.Y
	a.catalogMotion(d.key).offset = ui.Point{}
	if d.workspacePreview != nil && a.drag == d.workspacePreview {
		// The workspace preview handles returning to the source. A successful
		// drop restores the catalog row without starting a second row flight.
		if !d.externalDrop {
			a.drag.phase, a.drag.returnStarted = dragReturning, c.Now()
			a.drag.returnRect = d.settleBase
		}
		d.finished = true
	}
	c.AnimationFrame()
}
