package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) catalogTransition(kind catalogDragKind) ui.ElementTransition {
	t := theme.Current()
	animation := catalogDisplaceAnimation()
	if d := a.catalogDrag; d != nil && d.kind == catalogGroupDrag && kind == catalogHostDrag {
		duration := t.C.ReorderMotion.GroupCollapse
		if d.phase != catalogDragging {
			duration = t.C.ReorderMotion.GroupRestore
		}
		animation = ui.Animation{Duration: duration, Ease: tokenEasing(t.Motion.Easing.Out)}
	}
	return ui.ElementTransition{
		Position: true, Size: true, Animation: &animation,
		Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	}
}

func (a *app) catalogList(c *ui.Context) {
	t := theme.Current()
	viewport := ui.Box(c).Key("catalog-viewport").Grow(1).MinHeight(0).Clip()
	viewport.Children(func() {
		ui.Scroll(c.Key("session-list")).TrackScroll(&a.catalogScroll).Fill().MinHeight(0).
			ScrollbarInsets(0, -t.C.Scrollbar.TrackWidth, 0, 0).
			HandleInput(func(ev ui.InputEvent) bool {
				return a.catalogScrollbar.input(ev, &a.catalogScroll)
			}).Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Children(func() {
			ui.Column(c).Key("catalog-items").FillWidth().Children(func() {
				a.catalogSlots = a.catalogPlan(c)
				for i, slot := range a.catalogSlots {
					gap := t.C.Catalog.RowGap
					if slot.key.kind == catalogGroupDrag {
						gap = t.C.Catalog.GroupGap
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
						// Prime every row's collapse Exit before removing it, including
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
					ui.Text(c, "没有匹配的主机").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary)).Padding(t.Space.N12)
				}
			})
		})
		fadingScrollbar(c, scrollbarView{
			element: viewport, offset: &a.catalogScroll, label: "会话栏滚动条", color: tokenColor(t.C.Scrollbar.Color),
		}, &a.catalogScrollbar)
	})
	if a.section == sectionSSH {
		a.syncCatalogWorkspacePreview(c)
	}
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
	t := theme.Current()
	animation := catalogDisplaceAnimation()
	row := ui.Box(c).Key("catalog-placeholder").FillWidth().Height(slot.rect.H).Shrink(0).
		Radius(t.C.DropIndicator.ZoneRadius).Transition(ui.ElementTransition{
		Position: true, Size: true, Animation: &animation,
		Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	})
	background := tokenColor(t.C.DropIndicator.PlaceholderBg)
	shadow := t.C.DropIndicator.PlaceholderShadow
	if a.catalogDrag.placeholderReady && containsCatalogPoint(a.catalogScroll.Bounds(c), a.pointer) {
		background = tokenColor(t.C.DropIndicator.PlaceholderAccentBg)
		shadow = t.C.DropIndicator.PlaceholderAccentShadow
	}
	row.Background(row.AnimateColor("placeholder-bg", background, catalogColorAnimation()))
	applyAnimatedShadow(row, "placeholder-stroke", shadow, catalogColorAnimation())
	if !a.catalogDrag.placeholderReady {
		a.catalogDrag.placeholderReady = true
		c.AnimationFrame()
	}
	return row
}

func (a *app) catalogHeader(c *ui.Context, slot catalogSlot) ui.Element {
	t := theme.Current()
	group := slot.group
	expanded := a.catalogExpanded(group)
	header := ui.ButtonBase(c).Key(slot.key).Height(t.C.GroupHeader.Height).Shrink(0).
		FillWidth().Padding(0, t.C.GroupHeader.PaddingX).Gap(t.C.GroupHeader.Gap).Radius(t.C.GroupHeader.Radius).
		Label(group.name).Expanded(expanded).TextColor(tokenColor(t.C.GroupHeader.Text))
	a.trackCatalogDrag(c, header, slot)
	highlight := a.catalogDrag != nil && a.catalogDrag.phase == catalogDragging &&
		a.catalogDrag.kind == catalogHostDrag && a.catalogDrag.hoverGroup == group
	background, text, chevron := ui.Transparent, tokenColor(t.C.GroupHeader.Text), tokenColor(t.C.GroupHeader.Text)
	if highlight {
		background, text, chevron = tokenColor(t.C.GroupHeader.DropBg), tokenColor(t.C.GroupHeader.DropText), tokenColor(t.C.GroupHeader.DropCount)
	}
	header.Background(header.AnimateColor("header-bg", background, catalogColorAnimation()))
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
		arrow := ui.Icon(c, chevronIcon).Key("chevron").Size(t.Font.Size.Caption, t.Font.Size.Caption)
		arrow.TextColor(arrow.AnimateColor("color", chevron, catalogColorAnimation())).
			AnimatePose(ui.Pose{Rotate: rotation}, catalogDisplaceAnimation())
		label := ui.Text(c, group.name).FontSize(t.C.GroupHeader.FontSize).FontWeight(t.C.GroupHeader.Weight)
		label.TextColor(label.AnimateColor("color", text, catalogColorAnimation()))
		ui.Spacer(c)
		motion := a.catalogMotion(slot.key)
		count := len(group.hosts)
		if count != motion.count {
			motion.down, motion.count = count < motion.count, count
		}
		ui.Text(c, strconv.Itoa(count)).Key("count").FontSize(t.C.GroupHeader.CountSize).TextColor(tokenColor(t.C.GroupHeader.Count)).
			ContentTransition(ui.ContentTransition{Numeric: true, Down: motion.down})
		if highlight && group.collapsed && !a.catalogDrag.autoExpanded[group] {
			progress := min(1, float32(c.Now().Sub(a.catalogDrag.hoverStarted))/float32(t.C.GroupHeader.ExpandDelay))
			ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(t.C.GroupHeader.ExpandHeight).
				Background(tokenColor(t.C.GroupHeader.ExpandTrack)).PassThrough().Children(func() {
				ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).WidthPercent(progress * 100).
					Background(tokenColor(t.C.GroupHeader.ExpandFill)).PassThrough()
			})
		}
	})
	return header
}

func (a *app) catalogHostMenu(c *ui.Context, id hostID) func(*ui.Menu) {
	h := a.hosts[id]
	return func(m *ui.Menu) {
		if a.section == sectionFiles {
			a.fileSiteMenu(c, m, id)
			return
		}
		if m.Item("新建会话").Chosen() {
			a.openHost(id)
		}
		if m.Item("复制连接地址").Chosen() {
			mygo.Clipboard.WriteText(fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port))
		}
	}
}

func (a *app) catalogHostRow(c *ui.Context, slot catalogSlot) ui.Element {
	t := theme.Current()
	id, h := hostID(slot.key.id), a.hosts[slot.key.id]
	row := ui.ButtonBase(c).Key(slot.key).Height(t.C.Catalog.RowHeight).Shrink(0).FillWidth().
		Padding(0, t.C.SiderItem.PaddingRight, 0, t.C.SiderItem.PaddingLeft).Gap(t.C.SiderItem.Gap).Radius(t.C.SiderItem.Radius).
		Role(ui.RoleListItem).Label(h.name).Description(h.user + "@" + h.address).TextColor(tokenColor(t.C.SiderItem.NameColor))
	a.trackCatalogDrag(c, row, slot)
	background := ui.Transparent
	if a.catalogDrag == nil && row.Hovered() {
		background = tokenColor(t.C.SiderItem.BgHover)
	}
	animation := catalogColorAnimation()
	colorKey := "row-bg"
	flashing := a.catalogFlash != nil && *a.catalogFlash == slot.key
	seeding := flashing && a.catalogFlashSeed > 0
	if flashing {
		colorKey = "landing-bg"
		background = ui.Transparent
		animation = ui.Animation{Duration: t.C.DropIndicator.FlashDuration, Ease: tokenEasing(t.C.DropIndicator.FlashEasing)}
		if seeding {
			background = tokenColor(t.C.DropIndicator.FlashBg)
			animation = ui.Animation{Duration: t.C.Desktop.InstantDuration}
		}
	}
	shownBackground := row.AnimateColor(colorKey, background, animation)
	row.Background(shownBackground)
	if flashing {
		opacity := float32(shownBackground.A) / float32(t.C.DropIndicator.FlashBg.A)
		applyShadowOpacity(row, t.C.DropIndicator.FlashShadow, opacity)
	}
	if seeding {
		if shownBackground == background {
			a.catalogFlashSeed = 0
			a.catalogFlashUntil = c.Now().Add(t.C.DropIndicator.FlashDuration)
		}
		c.AnimationFrame()
	}
	if a.catalogDrag == nil {
		row.OnClick(func() { a.openCatalogHost(id) })
		row.ContextMenu(a.catalogHostMenu(c, id))
		if row.Focused() && row.Shortcut(0, ui.KeyEnter) {
			a.openCatalogHost(id)
		}
	}
	row.Children(func() {
		a.catalogHostContents(c, id)
		action := ui.ButtonBase(c).Key("host-actions").Size(t.C.SiderItem.MoreSize, t.C.SiderItem.MoreSize).Shrink(0).Radius(t.C.SiderItem.MoreRadius).
			TextColor(tokenColor(t.C.SiderItem.MoreIcon)).
			Label("更多操作 " + h.name).Description(h.system).Tooltip("更多操作")
		actionBackground := ui.Transparent
		if a.catalogDrag == nil && action.Hovered() {
			actionBackground = tokenColor(t.C.SiderItem.MoreBg)
		}
		action.Background(action.AnimateColor("background", actionBackground, catalogColorAnimation()))
		target := float32(0)
		if a.catalogDrag == nil && (row.Hovered() || row.Focused() || action.Focused()) {
			target = 1
		}
		action.Opacity(animateVisual(action, "more", target, t.C.DragLift.LiftDuration)).
			Disabled(a.catalogDrag != nil).Children(func() { ui.Icon(c, moreIcon).Size(t.Size.Icon, t.Size.Icon) })
		if a.catalogDrag == nil {
			action.Menu(a.catalogHostMenu(c, id))
		}
	})
	return row
}

func (a *app) catalogHostContents(c *ui.Context, id hostID) {
	t := theme.Current()
	h := a.hosts[id]
	a.catalogHostIcon(c, id)
	ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N1).Children(func() {
		ui.Text(c, h.name).FontSize(t.C.SiderItem.NameSize).FontWeight(t.C.SiderItem.NameWeight).FixedLineHeight(t.C.SiderItem.NameLineHeight).SingleLine().Ellipsis("…")
		ui.Text(c, h.user+"@"+h.address).Font(strings.Join(t.C.SiderItem.SubFamily, ", ")).FontSize(t.C.SiderItem.SubSize).FixedLineHeight(t.C.SiderItem.SubLineHeight).
			TextColor(tokenColor(t.C.SiderItem.SubColor)).SingleLine().Ellipsis("…")
	})
}

func (a *app) catalogDragPreview(c *ui.Context) {
	t := theme.Current()
	d := a.catalogDrag
	if d == nil || d.finished {
		return
	}
	if d.workspacePreview != nil && a.drag == d.workspacePreview {
		return
	}
	// This root sibling sits above the clipped sidebar and never takes layout
	// space or pointer input. The native source node only drives the gesture.
	ghost := ui.Box(c).Key(d).Absolute().Radius(t.C.DragLift.Radius).
		Height(d.origin.H).Width(d.origin.W).TextColor(tokenColor(t.C.SiderItem.NameColor)).PassThrough()
	scale, shadow := float32(1), float32(0)
	if d.phase == catalogDragging {
		ghost.Left(d.origin.X).Top(d.visualY)
		if !c.Now().Equal(d.started) {
			scale, shadow = t.C.DragLift.Scale, 1
		}
	} else {
		animation := catalogLandingAnimation()
		if d.phase == catalogCanceling {
			animation = ui.Animation{Spring: tokenSpring(t.C.ReorderMotion.Cancel)}
		}
		animation.Spring.Velocity = d.velocityY
		ghost.Left(d.settleBase.X).Top(d.settleBase.Y).Width(d.settleBase.W).
			Draggable(&d.offset, ui.DraggableOptions{Axis: ui.Vertical, Animation: animation})
		if d.settleSeed {
			d.settleSeed = false
			d.offset = ui.Point{}
			c.AnimationFrame()
		} else if !ghost.Transitioning() && c.Now().Sub(d.settleStarted) >= t.C.DragLift.LiftDuration {
			d.finished = true
			if d.phase == catalogSettling && d.kind == catalogHostDrag {
				key := d.key
				a.catalogFlash, a.catalogFlashSeed = &key, 2
				a.catalogFlashUntil = c.Now().Add(t.C.DropIndicator.FlashDuration)
			}
			c.AnimationFrame()
		}
		ghost.DragOffset()
		if !d.finished {
			c.AnimationFrame()
		}
	}
	lift := ui.Animation{Duration: t.MotionDuration(t.C.DragLift.LiftDuration, c.Preferences().ReduceMotion)}
	ghost.AnimatePose(ui.Pose{Scale: scale}, lift)
	alpha := animateVisual(ghost, "shadow", shadow, t.C.DragLift.LiftDuration)
	applyShadowOpacity(ghost, t.C.DragLift.Shadow, alpha)
	ghost.Children(func() {
		if d.kind == catalogGroupDrag {
			// Paint the sheets before the front card so their shadows remain behind it.
			for _, offset := range []float32{t.C.DragLift.StackInset, t.C.DragLift.StackInset / 2} {
				sheet := ui.Box(c).Absolute().Top(offset).Left(offset).Right(offset).Height(d.origin.H).
					Radius(t.C.DragLift.Radius).Background(tokenColor(t.C.DragLift.StackBg)).
					Opacity(t.C.DragLift.StackBackOpacity).PassThrough()
				applyShadow(sheet, t.C.DragLift.StackShadow)
			}
		}
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Padding(0, t.C.SiderItem.PaddingRight, 0, t.C.SiderItem.PaddingLeft).Gap(t.C.SiderItem.Gap).
			Radius(t.C.DragLift.Radius).Background(tokenColor(t.C.DragLift.Bg)).PassThrough().Children(func() {
			if d.kind == catalogHostDrag {
				a.catalogHostContents(c, d.host.id)
				return
			}
			ui.Icon(c, chevronIcon).Size(t.Font.Size.Caption, t.Font.Size.Caption).Rotate(-90)
			ui.Text(c, d.group.name).FontSize(t.C.GroupHeader.FontSize).FontWeight(t.Font.Weight.Semibold)
			ui.Spacer(c)
			ui.Text(c, strconv.Itoa(len(d.group.hosts))+" 台").FontSize(t.C.GroupHeader.CountSize).
				TextColor(tokenColor(t.C.GroupHeader.Count))
		})
	})
}
