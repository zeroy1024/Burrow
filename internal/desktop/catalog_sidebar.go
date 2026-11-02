package desktop

import (
	"fmt"
	"slices"
	"strings"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) catalogTransition(kind catalogDragKind) ui.ElementTransition {
	t := theme.Current()
	animation := a.catalogDisplaceAnimation()
	if d := a.page().catalogDrag; d != nil && d.kind == catalogGroupDrag && kind == catalogHostDrag {
		duration := t.C.ReorderMotion.GroupCollapse
		if d.phase != catalogDragging {
			duration = t.C.ReorderMotion.GroupRestore
		}
		animation = ui.Animation{Duration: duration, Ease: uikit.Easing(t.Motion.Easing.Out)}
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
		content := ui.Scroll(c.Key("session-list")).TrackScroll(&a.page().catalogScroll).Fill().MinHeight(0).Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Children(func() {
			ui.Column(c).Key("catalog-items").FillWidth().Children(func() {
				a.page().catalogSlots = a.catalogPlan(c)
				for i, slot := range a.page().catalogSlots {
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
							a.page().catalogSlots[i].headerRect = bounds
						}
					}
				}
				if d := a.page().catalogDrag; d != nil && d.phase == catalogDragging {
					// Keep the actual pressed node alive, under the same parent/key,
					// while the placeholder alone takes part in the preview layout.
					built := slices.ContainsFunc(a.page().catalogSlots, func(slot catalogSlot) bool {
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
				if len(a.page().catalogSlots) == 0 && strings.TrimSpace(a.page().query) != "" {
					uikit.EmptyMessage(c, "没有匹配的主机", uikit.EmptyCatalogSearch)
				}
			})
		})
		uikit.Scrollbars(c, viewport, content, &a.page().catalogScroll, uikit.ScrollbarProps{
			Label: "会话栏滚动条", State: &a.page().catalogScrollbar,
		})
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
	d := a.page().catalogDrag
	if d != nil && d.key == slot.key && !d.finished {
		row.Opacity(0)
	} else {
		row.Transition(a.catalogTransition(slot.key.kind))
	}
	return row
}

func (a *app) catalogPlaceholder(c *ui.Context, slot catalogSlot) ui.Element {
	t := theme.Current()
	animation := a.catalogDisplaceAnimation()
	row := uikit.CatalogPlaceholder(c, "catalog-placeholder", slot.rect.H, ui.ElementTransition{
		Position: true, Size: true, Animation: &animation, Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	})
	background := uikit.Color(t.C.DropIndicator.PlaceholderBg)
	shadow := t.C.DropIndicator.PlaceholderShadow
	if a.page().catalogDrag.placeholderReady && containsCatalogPoint(a.page().catalogScroll.Bounds(c), a.pointer) {
		background = uikit.Color(t.C.DropIndicator.PlaceholderAccentBg)
		shadow = t.C.DropIndicator.PlaceholderAccentShadow
	}
	uikit.StyleCatalogPlaceholder(row, background, shadow, catalogColorAnimation())
	if !a.page().catalogDrag.placeholderReady {
		a.page().catalogDrag.placeholderReady = true
		c.AnimationFrame()
	}
	return row
}

func (a *app) catalogHeader(c *ui.Context, slot catalogSlot) ui.Element {
	t := theme.Current()
	group := slot.group
	expanded := a.catalogExpanded(group)
	count := a.catalogModeCount(group.hosts)
	arrowAnimation := a.catalogDisplaceAnimation()
	result := uikit.SiderGroup(c, uikit.SiderGroupProps{
		Key: slot.key, Title: group.name, Open: &expanded, Count: count,
		Chevron: chevronIcon, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		ArrowAnimation: &arrowAnimation, DeferPresentation: true,
	})
	header := result.Root
	// The existing source is tracked before any hover, count, or landing styling.
	a.trackCatalogDrag(c, header, slot)
	highlight := a.page().catalogDrag != nil && a.page().catalogDrag.phase == catalogDragging &&
		a.page().catalogDrag.kind == catalogHostDrag && a.page().catalogDrag.hoverGroup == group
	motion := a.catalogMotion(slot.key)
	if count != motion.count {
		motion.down, motion.count = count < motion.count, count
	}
	result.Present(c, uikit.SiderGroupPresentation{
		Highlighted: highlight, DisableToggle: a.page().catalogDrag != nil, CountDown: motion.down,
	})
	if result.Toggled {
		a.page().lastGroup = group
		if err := a.commitCatalogMove(func() { a.setGroupCollapsed(group, !a.groupCollapsed(group)) }); err != nil {
			uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "保存分组状态失败", Detail: err.Error()})
		}
	}
	var menu []uikit.MenuEntry
	if a.page().catalogDrag == nil {
		menu = a.catalogGroupMenu(c, group)
	}
	uikit.ContextMenuOn(c, header, menu)
	if highlight && a.groupCollapsed(group) && !a.page().catalogDrag.autoExpanded[group] {
		header.Children(func() {
			progress := min(1, float32(c.Now().Sub(a.page().catalogDrag.hoverStarted))/float32(t.C.GroupHeader.ExpandDelay))
			uikit.SiderGroupExpand(c, progress)
		})
	}
	return header
}

func (a *app) catalogGroupMenu(c *ui.Context, group *hostGroup) []uikit.MenuEntry {
	addLabel := "添加主机"
	if group.scope == sectionFiles {
		addLabel = "添加站点"
	}
	return []uikit.MenuEntry{
		{ID: "add", Label: localize(c, addLabel), Icon: uikit.IconPlus, OnChoose: func() {
			a.page().lastGroup = group
			a.startHostDraft()
		}},
		{ID: "new-group", Label: localize(c, "新建分组"), Icon: uikit.IconFolder, OnChoose: a.startGroupDraft},
		{ID: "rename-group", Label: localize(c, "修改组名"), Icon: uikit.IconEdit, OnChoose: func() {
			a.startGroupRename(group)
		}},
		{ID: "before-delete", Separator: true},
		{ID: "delete-group", Label: localize(c, "删除分组"), Icon: uikit.IconTrash, Danger: true,
			Disabled: a.groupDeleteDestination(group) == nil, OnChoose: func() {
				a.page().groupDelete = group
				a.page().groupRename = nil
				a.page().groupDialogOpen = true
			}},
	}
}

func (a *app) catalogHostMenu(c *ui.Context, id hostID) []uikit.MenuEntry {
	h := a.hosts[id]
	connectLabel, editLabel, deleteLabel := "连接主机", "编辑主机", "删除主机"
	if a.section == sectionFiles {
		connectLabel, editLabel, deleteLabel = "连接站点", "编辑站点", "删除站点"
	}
	return []uikit.MenuEntry{
		{ID: "connect", Label: localize(c, connectLabel), Icon: uikit.IconTerminal, OnChoose: func() {
			a.openCatalogHost(id)
			if a.section == sectionFiles {
				session := a.currentFileSession()
				if session.state == disconnected || session.state == connectionFailed {
					a.reconnectFileSession(session)
				}
			}
		}},
		{ID: "after-connect", Separator: true},
		{ID: "edit", Label: localize(c, editLabel), Icon: uikit.IconEdit, OnChoose: func() {
			for _, group := range a.catalogGroups() {
				if slices.Contains(group.hosts, id) {
					a.page().lastGroup = group
					break
				}
			}
			a.startEditHostDraft(id)
		}},
		{ID: "copy", Label: localize(c, "复制地址"), Icon: uikit.IconCopy, OnChoose: func() {
			address := fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)
			if a.section == sectionFiles {
				protocol := "sftp"
				if h.protocol == "ftp" {
					protocol = "ftp"
				}
				address = protocol + "://" + address
			}
			mygo.Clipboard.WriteText(address)
		}},
		{ID: "before-delete", Separator: true},
		{ID: "delete", Label: localize(c, deleteLabel), Icon: uikit.IconTrash, Danger: true, OnChoose: func() {
			if a.fileSiteBusy(id) {
				uikit.Notify(c, "请先取消或清除该会话的传输任务")
				return
			}
			a.page().draft = hostDraft{id: id, deleting: true}
			a.page().addHostOpen = true
		}},
	}
}

func (a *app) catalogHostRow(c *ui.Context, slot catalogSlot) ui.Element {
	t := theme.Current()
	id, h := hostID(slot.key.id), a.hosts[slot.key.id]
	result := uikit.SiderRow(c, uikit.SiderRowProps{
		Key: slot.key, Title: h.name, Subtitle: h.user + "@" + h.address, Description: h.system,
		Logo: func() { a.catalogHostIcon(c, id) }, DragValue: hostDrag{id: id, group: slot.group},
		DeferPresentation: true, Motion: uikit.MotionOverride(a.reduceMotion(c)),
	})
	row := result.Root
	if row.Clicked() {
		a.page().lastGroup = slot.group
	}
	// Presentation reuses this same source after the custom drag controller runs.
	a.trackCatalogDrag(c, row, slot)
	animation := catalogColorAnimation()
	flashing := a.page().catalogFlash != nil && *a.page().catalogFlash == slot.key
	seeding := flashing && a.page().catalogFlashSeed > 0
	if flashing {
		animation = ui.Animation{Duration: t.C.DropIndicator.FlashDuration, Ease: uikit.Easing(t.C.DropIndicator.FlashEasing)}
		if seeding {
			animation = ui.Animation{Duration: t.C.Desktop.InstantDuration}
		}
	}
	if a.page().catalogDrag == nil {
		result.SetMenu(a.catalogHostMenu(c, id))
	}
	result.Present(c, uikit.SiderRowPresentation{
		InteractionDisabled: a.page().catalogDrag != nil, Flashing: flashing, Seeding: seeding, Animation: &animation,
	})
	if seeding {
		if result.Seeded {
			a.page().catalogFlashSeed = 0
			a.page().catalogFlashUntil = c.Now().Add(t.C.DropIndicator.FlashDuration)
		}
		c.AnimationFrame()
	}
	if result.Open {
		a.openCatalogHost(id)
	}
	return row
}

func (a *app) catalogHostContents(c *ui.Context, id hostID) {
	h := a.hosts[id]
	a.catalogHostIcon(c, id)
	uikit.SiderRowLabel(c, h.name, h.user+"@"+h.address)
}

func (a *app) catalogDragPreview(c *ui.Context) {
	t := theme.Current()
	d := a.page().catalogDrag
	if d == nil || d.finished {
		return
	}
	if d.workspacePreview != nil && a.drag == d.workspacePreview {
		return
	}
	// This root sibling sits above the clipped sidebar and never takes layout
	// space or pointer input. The native source node only drives the gesture.
	ghost := uikit.DragLift(c, d, ui.Point{X: d.origin.W, Y: d.origin.H})
	scale, shadow := float32(1), float32(0)
	if d.phase == catalogDragging {
		ghost.Left(d.origin.X).Top(d.visualY)
		if !c.Now().Equal(d.started) {
			scale, shadow = t.C.DragLift.Scale, 1
		}
	} else {
		animation := a.catalogLandingAnimation()
		if d.phase == catalogCanceling {
			animation = ui.Animation{Spring: uikit.Spring(t.C.ReorderMotion.Cancel)}
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
				a.page().catalogFlash, a.page().catalogFlashSeed = &key, 2
				a.page().catalogFlashUntil = c.Now().Add(t.C.DropIndicator.FlashDuration)
			}
			c.AnimationFrame()
		}
		ghost.DragOffset()
		if !d.finished {
			c.AnimationFrame()
		}
	}
	if a.reduceMotion(c) {
		scale = 1
	}
	returningGroup := d.kind == catalogGroupDrag && d.phase != catalogDragging
	if returningGroup {
		// Keep the return spring's position, but stop presenting a lifted stack
		// once the real list has restored this group's expanded state.
		ghost.AnimatePose(ui.Pose{Scale: 1}, ui.Animation{Duration: t.C.Desktop.InstantDuration})
	} else {
		lift := a.positionAnimation(uikit.Tween(t.C.DragLift.LiftDuration))
		uikit.StyleDragLift(c, ghost, uikit.DragLiftProps{Scale: scale, Shadow: shadow,
			Animation: lift, Motion: uikit.MotionOverride(a.reduceMotion(c))})
	}
	ghost.Children(func() {
		if d.kind == catalogGroupDrag && !returningGroup {
			// Paint the sheets before the front card so their shadows remain behind it.
			for _, offset := range []float32{t.C.DragLift.StackInset, t.C.DragLift.StackInset / 2} {
				uikit.DragLiftSheet(c, offset, d.origin.H)
			}
		}
		card := uikit.DragLiftCard(c)
		if returningGroup {
			p := t.C.GroupHeader
			card.Background(ui.Transparent).Padding(0, p.PaddingX).Gap(p.Gap)
		}
		card.Children(func() {
			if d.kind == catalogHostDrag {
				a.catalogHostContents(c, d.host.id)
				return
			}
			uikit.DragGroupLabel(c, d.group.name, a.catalogModeCount(d.group.hosts), chevronIcon,
				uikit.DragGroupLabelProps{Dragging: !returningGroup, Expanded: a.catalogExpanded(d.group)})
		})
	})
}
