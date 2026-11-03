package uikit

import (
	"fmt"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type Option[T comparable] struct {
	Value    T
	Label    string
	Disabled bool
	Leading  func()
	Detail   string
}

type SelectProps struct {
	Label, Placeholder string
	Handle             *ui.Handle
	Disabled           bool
	Error              string
	Detail             string
	Leading            func()
}

func Select[T comparable](c *ui.Context, selected *T, options []Option[T], props SelectProps) ui.Element {
	t := theme.Current()
	p := t.C.Input
	parts := ui.SelectBase(c, selected)
	trigger := StyleSelect(parts.Trigger, props.Label).Gap(p.Gap).Disabled(props.Disabled).FocusRing(false)
	if props.Handle != nil {
		trigger.Bind(props.Handle)
	}
	label := fmt.Sprint(*selected)
	matched := false
	var leading func()
	detail := props.Detail
	for _, option := range options {
		if option.Value == *selected {
			matched = true
			label, leading = option.Label, option.Leading
			if detail == "" {
				detail = option.Detail
			}
			break
		}
	}
	placeholder := props.Placeholder != "" && (!matched || label == "")
	if placeholder {
		label = props.Placeholder
	}
	if props.Leading != nil {
		leading = props.Leading
	}
	trigger.Children(func() {
		if leading != nil {
			leading()
		}
		text := ui.Text(c, label).FontSize(p.FontSize).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).PassThrough()
		if placeholder {
			text.TextColor(Color(p.Placeholder))
		}
		if detail != "" {
			ui.Text(c, detail).Grow(1).MinWidth(0).Shrink(1).SingleLine().Ellipsis("…").FontSize(t.Font.Size.Footnote).
				TextColor(Color(t.Color.Text.Secondary)).PassThrough()
		} else {
			text.Grow(1)
		}
		Icon(c, IconChevronDown, IconProps{Size: SizeSm, Tone: ToneTertiary}).Shrink(0).PassThrough()
	})
	StyleDisabled(trigger, props.Disabled)
	StyleFormControl(trigger, props.Error)
	if !trigger.IsDisabled() {
		parts.Popup(func(panel ui.Element) {
			width, _ := c.Size()
			available := max(0, width-2*t.Space.N4)
			StyleSelectPopup(panel).MaxWidth(available).MinWidth(min(trigger.Bounds().W, available))
			SelectPopupViewport(c, trigger, panel).Children(func() {
				for _, option := range options {
					// Item reads hover during construction, so its identity must already be assigned.
					c.Key(option.Value)
					var item ui.Element
					if option.Disabled {
						// SelectBase does not skip disabled Items for keyboard selection.
						// A displayed disabled row is not registered in its choices.
						item = ui.Row(c).Role(ui.RoleListItem).Label(option.Label).Disabled(true)
					} else {
						item = parts.Item(option.Value).Role(ui.RoleListItem).Label(option.Label)
					}
					StyleDisabled(item, option.Disabled)
					item.FillWidth().MinWidth(0).MinHeight(p.Height).Padding(t.Space.N6, t.C.Menu.ItemPaddingX).Gap(t.Space.N8).
						Radius(t.C.Menu.ItemRadius).AlignItems(ui.Center)
					if item.Highlighted() {
						item.Background(Color(t.C.Menu.ItemHover)).TextColor(Color(t.C.Menu.ItemHoverText))
					}
					item.Children(func() {
						if option.Leading != nil {
							option.Leading()
						}
						ui.Text(c, option.Label).FontSize(p.FontSize).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).PassThrough()
						if option.Detail != "" {
							ui.Text(c, option.Detail).FontSize(t.Font.Size.Footnote).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).PassThrough()
						}
						ui.Spacer(c).PassThrough()
						mark := ui.Icon(c, IconSVG(IconCheck)).Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).PassThrough()
						if option.Value != *selected {
							mark.Opacity(0)
						}
					})
				}
			})
		})
	}

	return trigger
}

func StringOptions(labels []string) []Option[string] {
	options := make([]Option[string], len(labels))
	for index, label := range labels {
		options[index] = Option[string]{Value: label, Label: label}
	}
	return options
}

func StyleSelect(e ui.Element, label string) ui.Element {
	p := theme.Current().C.Input
	return e.Height(p.Height).FillWidth().MinWidth(0).Padding(0, p.PaddingX).Radius(p.Radius).
		Background(Color(p.Bg)).Border(p.BorderWidth, Color(p.Border)).FontSize(p.FontSize).Label(label)
}

func StyleSelectPopup(panel ui.Element) ui.Element {
	t := theme.Current()
	return Shadow(panel.Padding(t.Space.N4).Radius(t.C.Menu.Radius).Background(Color(t.C.Menu.Bg)), t.C.Menu.Shadow)
}

// SelectPopupViewport leaves placement with SelectBase while keeping its options
// scrollable within the larger space above or below the trigger.
func SelectPopupViewport(c *ui.Context, trigger, panel ui.Element) ui.Element {
	_, height := c.Size()
	bounds := trigger.Bounds()
	layout := trigger.LayoutBounds()
	// Bounds comes from the previous frame. Recompute after resizing or moving
	// the trigger, once the current layout has reached its final position.
	trigger.DrawOver(func(_ *ui.Painter, rect ui.Rect) {
		if rect.Y != layout.Y || rect.H != layout.H {
			c.Invalidate()
		}
	})
	padding := theme.Current().Space.N4
	room := max(bounds.Y, height-bounds.Y-bounds.H) - padding
	viewport := ui.Scroll(c).FillWidth().MinHeight(0).MaxHeight(max(0, room-2*padding))
	scroll := ui.Local(viewport, "uikit.popup.scroll", func() ui.ScrollState { return ui.ScrollState{} })
	viewport.TrackScroll(scroll)
	Scrollbars(c, panel, viewport, scroll, ScrollbarProps{Label: "选项滚动条"})
	return viewport
}
