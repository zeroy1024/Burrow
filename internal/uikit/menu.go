package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// MenuEntry is shared by the system context menu and the in-window menu.
// Callbacks are menu commands; they are never retained in component-local state.
type MenuEntry struct {
	ID          string
	Label       string
	Icon        IconName
	Glyph       *ui.SVG
	Shortcut    string
	Modifiers   ui.Modifiers
	ShortcutKey ui.Key
	Disabled    bool
	Checked     *bool
	Radio       bool
	Danger      bool
	Separator   bool
	Children    []MenuEntry
	OnChoose    func()
}

func NativeMenu(menu *ui.Menu, entries []MenuEntry) {
	nativeMenu(menu, entries, false)
}

func nativeMenu(menu *ui.Menu, entries []MenuEntry, disabled bool) {
	for _, entry := range entries {
		if entry.Separator {
			menu.Separator()
			continue
		}
		var item *ui.MenuItem
		if len(entry.Children) != 0 {
			item = menu.Submenu(entry.Label, func(sub *ui.Menu) { nativeMenu(sub, entry.Children, disabled || entry.Disabled) })
		} else {
			item = menu.Item(entry.Label)
		}
		item.Disabled(disabled || entry.Disabled)
		if entry.Checked != nil {
			if entry.Radio {
				item.Radio(*entry.Checked)
			} else {
				item.Checked(*entry.Checked)
			}
		}
		if entry.ShortcutKey != ui.KeyUnknown {
			item.Shortcut(entry.Modifiers, entry.ShortcutKey)
		}
		if item.Chosen() && entry.OnChoose != nil {
			entry.OnChoose()
		}
	}
}

func RowMenu(row ui.Element, entries []MenuEntry) ui.Element {
	return row.ContextMenu(func(menu *ui.Menu) { NativeMenu(menu, entries) })
}

// RowMenuOn shares one popup and renderer between a row's context gesture and
// its more button. Context gestures use a point anchor, never a native menu.
func RowMenuOn(c *ui.Context, row, trigger ui.Element, entries []MenuEntry) bool {
	return menuOn(c, trigger, row, entries, false)
}

// ContextMenuOn uses the shared popup at the context gesture without taking
// over the row's primary click, keyboard navigation, role or expanded state.
// Targets share the popup with children that own pointer input, such as a
// tab's drag handle and close button.
func ContextMenuOn(c *ui.Context, row ui.Element, entries []MenuEntry, targets ...ui.Element) bool {
	return menuOn(
		c,
		row,
		row,
		entries,
		true,
		targets...,
	)
}

func MenuButton[T IconSource](c *ui.Context, icon T, label string, entries []MenuEntry) ui.Element {
	return MenuOn(c, IconButton(c, icon, IconButtonProps{Label: label}), entries)
}

// MenuOn adds a token-styled menu to an existing, independently styled trigger.
func MenuOn(c *ui.Context, trigger ui.Element, entries []MenuEntry) ui.Element {
	menuOn(c, trigger, ui.Element{}, entries, false)
	return trigger
}

type menuState struct {
	Open, Context bool
	Point         ui.Point
}

func menuOn(
	c *ui.Context,
	trigger, row ui.Element,
	entries []MenuEntry,
	contextOnly bool,
	targets ...ui.Element,
) bool {
	state := ui.Local(trigger, "uikit.menu.state", func() menuState { return menuState{} })
	open := &state.Open
	wasOpen := *open
	opening := false
	keyOpen := !contextOnly && trigger.Shortcut(0, ui.KeyDown)
	if trigger.IsDisabled() || len(entries) == 0 {
		*open = false
	} else {
		context, keyboard := false, false
		if row.ID() != 0 {
			context = row.RightClicked() || trigger.RightClicked()
			for _, target := range targets {
				if target.RightClicked() {
					context = true
				}
			}
			keyboard = row.Shortcut(ui.Shift, ui.KeyF10)
		}
		if context || keyboard {
			bounds := row.Bounds()
			state.Point = ui.Point{X: bounds.X, Y: bounds.Y + bounds.H}
			if context {
				x, y, _ := row.PointerPosition()
				state.Point = ui.Point{X: bounds.X + x, Y: bounds.Y + y}
			}
			*open, state.Context, opening = true, true, true
		} else if !contextOnly && (trigger.Clicked() || keyOpen) {
			*open, state.Context = !*open, false
			opening = *open
		}
	}
	if !contextOnly {
		trigger.Role(ui.RoleMenuButton).Expanded(*open)
		if *open {
			trigger.Opacity(1).Background(Color(theme.Current().C.IconButton.BgOn))
		}
	}
	focusTarget := trigger
	if state.Context {
		focusTarget = row
	}
	closeMenu := func() {
		*open = false
		if !trigger.IsDisabled() {
			focusTarget.Focus()
		}
		c.Invalidate()
	}
	anchor := trigger
	var point *ui.Point
	if *open && state.Context {
		point = &state.Point
		ui.Overlay(c, func() {
			anchor = ui.Box(c).Key(struct {
				Kind    string
				Trigger uint64
			}{"uikit.menu.point", trigger.ID()}).Absolute().Left(point.X).Top(point.Y).
				Size(0, 0).PassThrough().Role(ui.RoleNone)
		})
	}
	outside := menuPopup(c, anchor, open, entries, false, opening, closeMenu, point)
	if wasOpen && !*open && !outside && !trigger.IsDisabled() {
		focusTarget.Focus()
	}
	return opening
}

func menuPopup(
	c *ui.Context,
	anchor ui.Element,
	open *bool,
	entries []MenuEntry,
	submenu, focusFirst bool,
	closeMenu func(),
	point *ui.Point,
) bool {
	outside := false
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		t := theme.Current()
		p := t.C.Menu
		panel.Key(struct {
			Kind   string
			Anchor uint64
		}{"uikit.menu", anchor.ID()})
		StyleSelectPopup(panel).Role(ui.RoleMenu).Label("操作菜单").Column().KeepFocus()
		outside = panel.PressedOutside()
		x, y, _ := panel.PointerPosition()
		pointer := ui.Point{X: x, Y: y}
		lastPointer := ui.Local(panel, "uikit.menu.pointer", func() ui.Point { return pointer })
		pointerMoved := pointer != *lastPointer
		*lastPointer = pointer
		if submenu {
			panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorTopStart).MarginStart(t.Space.N4)
		}
		_, height := c.Size()
		hasChecks, hasIcons := false, false
		for _, entry := range entries {
			hasChecks = hasChecks || entry.Checked != nil
			hasIcons = hasIcons || entry.Icon != "" || entry.Glyph != nil
		}
		child := ui.Local(panel, "uikit.menu.child", func() string { return "" })
		if submenu && panel.OverlayShortcut(0, ui.KeyLeft) {
			*open = false
			anchor.Focus()
			c.Invalidate()
		}
		var viewport ui.Element
		if submenu {
			viewport = ui.Scroll(c).MaxHeight(max(0, height-2*t.Space.N12)).MinHeight(0).FillWidth()
		} else if point != nil {
			padding := t.Space.N4
			room := max(point.Y, height-point.Y) - 3*padding
			viewport = ui.Scroll(c).MaxHeight(max(0, room)).MinHeight(0).FillWidth()
		} else {
			viewport = SelectPopupViewport(c, anchor, panel)
		}
		if submenu || point != nil {
			scroll := ui.Local(viewport, "uikit.menu.scroll", func() ui.ScrollState { return ui.ScrollState{} })
			viewport.TrackScroll(scroll)
			Scrollbars(c, panel, viewport, scroll, ScrollbarProps{Label: "菜单滚动条"})
		}
		viewport.FocusGroup(ui.Vertical).Children(func() {
			first := true
			for _, entry := range entries {
				if entry.Separator {
					Divider(c.Key(entry.ID), DividerProps{Inset: t.Space.N4}).Margin(t.Space.N4, t.Space.N4)
					continue
				}
				item := ui.ButtonBase(c.Key(entry.ID)).Role(ui.RoleMenuItem).Label(entry.Label).
					Height(p.ItemHeight).FillWidth().Shrink(0).Padding(0, p.ItemPaddingX).Gap(t.Space.N8).
					Radius(p.ItemRadius).FocusRing(false).Justify(ui.Start).Disabled(entry.Disabled)
				if entry.Checked != nil {
					role := ui.RoleMenuItemCheckBox
					if entry.Radio {
						role = ui.RoleMenuItemRadio
					}
					item.Role(role).Checked(*entry.Checked)
				}
				if first && !entry.Disabled {
					if focusFirst {
						item.Focus()
					}
					first = false
				}
				if !entry.Disabled && pointerMoved && item.Hovered() {
					item.Focus()
				}
				ink, background := Color(t.Color.Text.Primary), ui.Transparent
				if entry.Danger {
					ink = Color(t.Color.Danger.Text)
				}
				if !entry.Disabled && (item.Focused() || *child == entry.ID) {
					background, ink = Color(p.ItemHover), Color(p.ItemHoverText)
					if entry.Danger {
						background, ink = Color(t.Color.Danger.Subtle), Color(t.Color.Danger.Text)
					}
				}
				item.Background(background).TextColor(ink)
				StyleDisabled(item, entry.Disabled)
				item.Children(func() {
					if hasChecks {
						ui.Box(c).Size(t.Size.Icon, t.Size.Icon).Center().PassThrough().Children(func() {
							if entry.Checked != nil && *entry.Checked {
								if entry.Radio {
									ui.Box(c).Size(t.C.Check.RadioDot, t.C.Check.RadioDot).Radius(t.Radius.Pill).Background(ink).PassThrough()
								} else {
									ui.Icon(c, IconSVG(IconCheck)).Size(t.Size.IconSm, t.Size.IconSm).PassThrough()
								}
							}
						})
					}
					if hasIcons {
						ui.Box(c).Size(t.Size.Icon, t.Size.Icon).Center().PassThrough().Children(func() {
							if entry.Icon != "" {
								ui.Icon(c, IconSVG(entry.Icon)).Size(t.Size.Icon, t.Size.Icon).PassThrough()
							} else if entry.Glyph != nil {
								ui.Icon(c, entry.Glyph).Size(t.Size.Icon, t.Size.Icon).PassThrough()
							}
						})
					}
					ui.Text(c, entry.Label).FontSize(p.FontSize).SingleLine().PassThrough()
					ui.Spacer(c).PassThrough()
					if entry.Shortcut != "" {
						Kbd(c, entry.Shortcut, KbdProps{Tone: KbdFlat}).TextColor(ink)
					}
					if len(entry.Children) != 0 {
						ui.Icon(c, IconSVG(IconArrowRight)).Size(t.Size.IconXs, t.Size.IconXs).PassThrough()
					}
				})
				if entry.Disabled {
					continue
				}
				if len(entry.Children) == 0 {
					if *child != "" && (item.Focused() || pointerMoved && item.Hovered()) {
						*child = ""
						c.Invalidate()
					}
					if item.Clicked() {
						closeMenu()
						if entry.OnChoose != nil {
							entry.OnChoose()
						}
					}
					continue
				}
				keyOpen := item.Shortcut(0, ui.KeyRight) || item.Clicked()
				if (pointerMoved && item.Hovered() || keyOpen) && *child != entry.ID {
					*child = entry.ID
					c.Invalidate()
				}
				childOpen := *child == entry.ID
				wasChildOpen := childOpen
				item.Expanded(childOpen)
				childOutside := menuPopup(c, item, &childOpen, entry.Children, true, keyOpen, closeMenu, nil)
				if wasChildOpen && !childOpen {
					*child = ""
					c.Invalidate()
					if !childOutside {
						item.Focus()
					}
				}
			}
		})
	})
	return outside
}
