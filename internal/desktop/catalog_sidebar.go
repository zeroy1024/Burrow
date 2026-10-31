package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) catalogTransition(kind catalogDragKind) ui.ElementTransition {
	animation := &displace
	if d := a.catalogDrag; d != nil && d.kind == catalogGroupDrag && kind == catalogHostDrag {
		duration := 180 * time.Millisecond
		if d.phase != catalogDragging {
			duration = 220 * time.Millisecond
		}
		animation = &ui.Animation{Duration: duration, Ease: ui.EaseOut}
	}
	return ui.ElementTransition{
		Position: true, Size: true, Animation: animation,
		Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	}
}

func (a *app) catalogList(c *ui.Context) {
	ui.Scroll(c.Key("session-list")).TrackScroll(&a.catalogScroll).
		Grow(1).MinHeight(0).Padding(12, 8).Children(func() {
		ui.Column(c).Key("catalog-items").FillWidth().Children(func() {
			a.catalogSlots = a.catalogPlan(c)
			for i, slot := range a.catalogSlots {
				gap := float32(catalogRowGap)
				if slot.key.kind == catalogGroupDrag {
					gap = catalogGroupGap
					if i == 0 {
						gap = 0
					}
				}
				if slot.placeholder {
					a.catalogPlaceholder(c, slot).Margin(gap, 0, 0, 0)
					continue
				}
				item := a.catalogItem(c, slot).Margin(gap, 0, 0, 0)
				if slot.key.kind == catalogGroupDrag {
					if bounds := item.Bounds(); bounds.W > 0 && bounds.H > 0 {
						a.catalogSlots[i].headerRect = bounds
					}
				}
			}
			if d := a.catalogDrag; d != nil && d.phase == catalogDragging {
				// Keep the actual pressed node alive, under the same parent/key,
				// while the placeholder alone takes part in the preview layout.
				built := slices.ContainsFunc(a.catalogSlots, func(slot catalogSlot) bool {
					return slot.key == d.key && !slot.placeholder
				})
				if !built {
					slot := catalogSlot{key: d.key, group: d.group, rect: d.origin}
					a.catalogItem(c, slot).Absolute().Left(0).Top(d.originContentY).FillWidth().Margin(0)
				}
				if d.kind == catalogGroupDrag && d.collapseFrames < 2 {
					// Prime every row's 180ms Exit before removing it, including
					// rows built before the source header in the first drag frame.
					newFrame := d.collapseFrames == 0 || !c.Now().Equal(d.collapseSample)
					if newFrame {
						d.collapseFrames++
						d.collapseSample = c.Now()
					}
					c.AnimationFrame()
				}
			}
			if len(a.catalogSlots) == 0 && strings.TrimSpace(a.query) != "" {
				ui.Text(c, "没有匹配的主机").FontSize(12).TextColor(a.theme.TextMuted).Padding(12)
			}
		})
	})
	a.syncCatalogWorkspacePreview(c)
}

func (a *app) catalogItem(c *ui.Context, slot catalogSlot) ui.Element {
	var row ui.Element
	if slot.key.kind == catalogGroupDrag {
		row = a.catalogHeader(c, slot)
	} else {
		row = a.catalogHostRow(c, slot)
	}
	d := a.catalogDrag
	if d != nil && d.key == slot.key && !d.finished {
		row.Opacity(0)
	} else {
		row.Transition(a.catalogTransition(slot.key.kind))
	}
	return row
}

func (a *app) catalogPlaceholder(c *ui.Context, slot catalogSlot) ui.Element {
	row := ui.Box(c).Key("catalog-placeholder").FillWidth().Height(slot.rect.H).Shrink(0).Radius(8).
		Transition(ui.ElementTransition{Position: true, Size: true, Animation: &displace,
			Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}})
	background := ui.RGB(0, 0, 0).Alpha(.035)
	border := ui.RGB(0, 0, 0).Alpha(.05)
	if a.catalogDrag.placeholderReady && containsCatalogPoint(a.catalogScroll.Bounds(c), a.pointer) {
		background = a.theme.Accent.Alpha(.06)
		border = a.theme.Accent.Alpha(.28)
	}
	row.Background(row.AnimateColor("placeholder-bg", background, catalogColor))
	stroke := row.AnimateColor("placeholder-stroke", border, catalogColor)
	row.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, stroke, 8, 1) })
	if !a.catalogDrag.placeholderReady {
		a.catalogDrag.placeholderReady = true
		c.AnimationFrame()
	}
	return row
}

func (a *app) catalogHeader(c *ui.Context, slot catalogSlot) ui.Element {
	group := slot.group
	expanded := a.catalogExpanded(group)
	header := ui.ButtonBase(c).Key(slot.key).Height(catalogHeaderHeight).Shrink(0).
		FillWidth().Padding(0, 8).Gap(5).Radius(8).
		Label(group.name).Expanded(expanded).TextColor(a.theme.TextMuted)
	a.trackCatalogDrag(c, header, slot)
	highlight := a.catalogDrag != nil && a.catalogDrag.phase == catalogDragging &&
		a.catalogDrag.kind == catalogHostDrag && a.catalogDrag.hoverGroup == group
	background, text, chevron := ui.Transparent, a.theme.TextMuted, a.theme.TextMuted
	if highlight {
		background, text, chevron = a.theme.Accent.Alpha(.08), ui.Hex("#0a6fd6"), a.theme.Accent
	}
	header.Background(header.AnimateColor("header-bg", background, catalogColor))
	if a.catalogDrag == nil {
		header.OnClick(func() {
			if err := a.commitCatalogMove(func() { group.collapsed = !group.collapsed }); err != nil {
				c.Toast("保存分组状态失败：" + err.Error())
			}
		})
		header.ContextMenu(func(m *ui.Menu) {
			if m.Item("添加主机").Chosen() {
				a.startHostDraft()
				a.draft.group = group.name
			}
			if m.Item("新建分组").Chosen() {
				a.startGroupDraft()
			}
		})
	}
	header.Children(func() {
		rotation := float32(-90)
		if expanded {
			rotation = 0
		}
		arrow := ui.Icon(c, chevronIcon).Key("chevron").Size(11, 11)
		arrow.TextColor(arrow.AnimateColor("color", chevron, catalogColor)).
			AnimatePose(ui.Pose{Rotate: rotation}, displace)
		label := ui.Text(c, group.name).FontSize(11)
		label.TextColor(label.AnimateColor("color", text, catalogColor))
		ui.Spacer(c)
		motion := a.catalogMotion(slot.key)
		count := len(group.hosts)
		if count != motion.count {
			motion.down, motion.count = count < motion.count, count
		}
		ui.Text(c, strconv.Itoa(count)).Key("count").FontSize(11).
			ContentTransition(ui.ContentTransition{Numeric: true, Down: motion.down})
		if highlight && group.collapsed && !a.catalogDrag.autoExpanded[group] {
			progress := min(1, float32(c.Now().Sub(a.catalogDrag.hoverStarted))/float32(500*time.Millisecond))
			ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(2).
				Background(a.theme.Accent.Alpha(.15)).PassThrough().Children(func() {
				ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).WidthPercent(progress * 100).
					Background(a.theme.Accent).PassThrough()
			})
		}
	})
	return header
}

func (a *app) catalogHostMenu(c *ui.Context, id hostID) func(*ui.Menu) {
	h := a.hosts[id]
	return func(m *ui.Menu) {
		if m.Item("新建会话").Chosen() {
			a.openHost(id)
		}
		if m.Item("复制连接地址").Chosen() {
			mygo.Clipboard.WriteText(fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port))
		}
	}
}

func (a *app) catalogHostRow(c *ui.Context, slot catalogSlot) ui.Element {
	id, h := hostID(slot.key.id), a.hosts[slot.key.id]
	row := ui.ButtonBase(c).Key(slot.key).Height(catalogRowHeight).Shrink(0).FillWidth().
		Padding(0, 8).Gap(6).Radius(8).
		Role(ui.RoleListItem).Label(h.name).Description(h.user + "@" + h.address).TextColor(a.theme.Text)
	a.trackCatalogDrag(c, row, slot)
	background := ui.Transparent
	if a.catalogDrag == nil && row.Hovered() {
		background = ui.Hex("#f7f7f9")
	}
	animation := catalogColor
	colorKey := "row-bg"
	flashing := a.catalogFlash != nil && *a.catalogFlash == slot.key
	seeding := flashing && a.catalogFlashSeed > 0
	if flashing {
		colorKey = "landing-bg"
		background = ui.Transparent
		animation = ui.Animation{Duration: 600 * time.Millisecond, Ease: ui.EaseOut}
		if seeding {
			background = a.theme.Accent.Alpha(.10)
			animation = ui.Animation{Duration: time.Nanosecond}
		}
	}
	shownBackground := row.AnimateColor(colorKey, background, animation)
	row.Background(shownBackground)
	if seeding {
		if shownBackground == background {
			a.catalogFlashSeed = 0
			a.catalogFlashUntil = c.Now().Add(600 * time.Millisecond)
		}
		c.AnimationFrame()
	}
	if a.catalogDrag == nil {
		row.OnClick(func() { a.openHost(id) })
		row.ContextMenu(a.catalogHostMenu(c, id))
		if row.Focused() && row.Shortcut(0, ui.KeyEnter) {
			a.openHost(id)
		}
	}
	row.Children(func() {
		a.catalogHostContents(c, id)
		action := ui.ButtonBase(c).Key("host-actions").Size(24, 24).Shrink(0).Radius(7).
			TextColor(a.theme.TextMuted).
			Label("更多操作 " + h.name).Description(h.system).Tooltip("更多操作")
		actionBackground := ui.Transparent
		if a.catalogDrag == nil && action.Hovered() {
			actionBackground = ui.Hex("#ededf0")
		}
		action.Background(action.AnimateColor("background", actionBackground, catalogColor))
		target := float32(0)
		if a.catalogDrag == nil && (row.Hovered() || action.FocusVisible()) {
			target = 1
		}
		action.AnimatePose(ui.Pose{Fade: 1 - target}, catalogColor).
			Disabled(a.catalogDrag != nil).Children(func() { ui.Icon(c, moreIcon).Size(16, 16) })
		if a.catalogDrag == nil {
			action.Menu(a.catalogHostMenu(c, id))
		}
	})
	return row
}

func (a *app) catalogHostContents(c *ui.Context, id hostID) {
	h := a.hosts[id]
	style := systems[h.os]
	ui.Box(c).Size(24, 24).Shrink(0).Radius(7).Background(ui.Hex(style.tint)).Center().
		Children(func() {
			ui.Image(c, style.icon).Size(16, 16).TextColor(ui.Hex(style.color))
		})
	ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
		ui.Text(c, h.name).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…")
		ui.Text(c, h.user+"@"+h.address).Font("Menlo, monospace").FontSize(10.5).
			TextColor(a.theme.TextMuted).SingleLine().Ellipsis("…")
	})
}

func (a *app) catalogDragPreview(c *ui.Context) {
	d := a.catalogDrag
	if d == nil || d.finished {
		return
	}
	if d.workspacePreview != nil && a.drag == d.workspacePreview {
		return
	}
	// This root sibling sits above the clipped sidebar and never takes layout
	// space or pointer input. The native source node only drives the gesture.
	ghost := ui.Row(c).Key(d).Absolute().Radius(9).
		Height(d.origin.H).Width(d.origin.W).Padding(0, 8).Gap(6).
		Background(ui.Hex("#ffffff")).TextColor(a.theme.Text).PassThrough()
	scale, shadow := float32(1), float32(0)
	if d.phase == catalogDragging {
		ghost.Left(d.origin.X).Top(d.visualY)
		if !c.Now().Equal(d.started) {
			scale, shadow = 1.03, 1
		}
	} else {
		animation := catalogLanding
		if d.phase == catalogCanceling {
			animation = ui.Smooth
			animation.Spring.Duration = 300 * time.Millisecond
		}
		animation.Spring.Velocity = d.velocityY
		ghost.Left(d.settleBase.X).Top(d.settleBase.Y).Width(d.settleBase.W).
			Draggable(&d.offset, ui.DraggableOptions{Axis: ui.Vertical, Animation: animation})
		if d.settleSeed {
			d.settleSeed = false
			d.offset = ui.Point{}
			c.AnimationFrame()
		} else if !ghost.Transitioning() && c.Now().Sub(d.settleStarted) >= 120*time.Millisecond {
			d.finished = true
			if d.phase == catalogSettling && d.kind == catalogHostDrag {
				key := d.key
				a.catalogFlash, a.catalogFlashSeed = &key, 2
				a.catalogFlashUntil = c.Now().Add(600 * time.Millisecond)
			}
			c.AnimationFrame()
		}
		ghost.DragOffset()
		if !d.finished {
			c.AnimationFrame()
		}
	}
	lift := ui.Snappy
	lift.Spring.Duration = 120 * time.Millisecond
	ghost.AnimatePose(ui.Pose{Scale: scale}, lift)
	alpha := float32(ghost.AnimateColor("shadow", ui.RGB(0, 0, 0).Alpha(shadow), catalogColor).A) / 255
	ghost.Shadow(0, 10, 28, 0, ui.RGB(0, 0, 0).Alpha(.16*alpha)).
		Shadow(0, 2, 6, 0, ui.RGB(0, 0, 0).Alpha(.08*alpha)).
		Shadow(0, 0, 0, .5, ui.RGB(0, 0, 0).Alpha(.10*alpha))
	ghost.Children(func() {
		if d.kind == catalogHostDrag {
			a.catalogHostContents(c, d.host.id)
			return
		}
		// The title remains the front card; the two sheets are painted behind it.
		ui.Icon(c, chevronIcon).Size(11, 11).Rotate(-90)
		ui.Text(c, d.group.name).FontSize(11).FontWeight(600)
		ui.Spacer(c)
		ui.Text(c, strconv.Itoa(len(d.group.hosts))+" 台").FontSize(11).TextColor(a.theme.TextMuted)
	})
	if d.kind == catalogGroupDrag {
		ghost.Draw(func(p *ui.Painter, r ui.Rect) {
			for _, offset := range []float32{6, 3} {
				card := ui.Rect{X: r.X + offset, Y: r.Y + offset, W: r.W - offset*2, H: r.H}
				p.Fill(card, ui.Hex("#f9f9fa"), 9)
				p.Stroke(card, ui.RGB(0, 0, 0).Alpha(.08), 9, .5)
			}
			p.Fill(r, ui.Hex("#ffffff"), 9)
		})
	}
}
