package uikit

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type MultiSelectProps struct {
	Label, Placeholder string
	Disabled           bool
	Error              string
}

func MultiSelect[T comparable](
	c *ui.Context,
	selected *[]T,
	options []Option[T],
	props MultiSelectProps,
) ui.Element {
	t := theme.Current()
	p := t.C.Input
	root := ui.Row(c).FillWidth().MinWidth(0).MinHeight(p.Height).
		Padding(t.Space.N3, p.PaddingX).Gap(t.Space.N6).AlignItems(ui.Center).
		Radius(p.Radius).Background(Color(p.Bg)).Disabled(props.Disabled).
		Focusable().FocusRing(false).Role(ui.RoleComboBox).Label(props.Label)
	open := ui.Local(root, "uikit.multiselect.open", func() bool { return false })
	if props.Disabled {
		*open = false
	}
	remove := -1
	var toggle ui.Element
	root.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Wrap().Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
			for index, value := range *selected {
				label := fmt.Sprint(value)
				for _, option := range options {
					if option.Value == value {
						label = option.Label
						break
					}
				}
				ui.Row(c.Key(value)).Height(p.ChipHeight).MaxWidthPercent(100).
					Padding(0, t.Space.N6).Gap(t.Space.N4).AlignItems(ui.Center).
					Radius(p.ChipRadius).Background(Color(p.ChipBg)).Children(func() {
					ui.Text(c, label).FontSize(p.ChipFontSize).MinWidth(0).Shrink(1).
						SingleLine().Ellipsis("…").PassThrough()
					if IconButton(c, IconClose, IconButtonProps{
						Key: "remove", Label: locales.MessageRemove() + label, Size: SizeXs, Disabled: props.Disabled,
					}).Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).Clicked() {
						remove = index
					}
				})
			}
			if len(*selected) == 0 {
				ui.Text(c, props.Placeholder).FontSize(p.FontSize).TextColor(Color(p.Placeholder)).
					SingleLine().PassThrough()
			}
		})
		toggle = IconButton(c, IconChevronDown, IconButtonProps{
			Key: "options", Label: props.Label, Size: SizeXs, Disabled: props.Disabled,
		}).Shrink(0)
	})
	if remove >= 0 {
		*selected = slices.Delete(*selected, remove, remove+1)
		c.Invalidate()
	}
	justOpened := false
	if !props.Disabled && remove < 0 {
		keyboard := root.Shortcut(0, ui.KeyDown) || root.Shortcut(0, ui.KeyEnter) || root.Shortcut(0, ui.KeySpace)
		if root.Clicked() || toggle.Clicked() || keyboard {
			*open = !*open
			justOpened = *open
			root.Focus()
			c.Invalidate()
		}
	}
	root.Expanded(*open)
	StyleDisabled(root, props.Disabled)
	StyleFormControl(root, props.Error)
	if props.Disabled {
		return root
	}
	ui.PopoverBase(c, root, open, func(panel ui.Element) {
		width, _ := c.Size()
		available := max(0, width-2*t.Space.N4)
		StyleSelectPopup(panel).Column().Role(ui.RoleMenu).Label(props.Label).KeepFocus().
			MaxWidth(available).MinWidth(min(root.Bounds().W, available))
		SelectPopupViewport(c, root, panel).FocusGroup(ui.Vertical).Children(func() {
			first := true
			for _, option := range options {
				checked := slices.Contains(*selected, option.Value)
				item := ui.ButtonBase(c.Key(option.Value)).FillWidth().MinWidth(0).MinHeight(p.Height).
					Padding(t.Space.N6, t.C.Menu.ItemPaddingX).Gap(t.Space.N8).
					Radius(t.C.Menu.ItemRadius).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).
					Role(ui.RoleMenuItemCheckBox).Checked(checked).Label(option.Label).Disabled(option.Disabled)
				if first && !option.Disabled {
					if justOpened {
						item.Focus()
					}
					first = false
				}
				highlighted := !option.Disabled && (item.Hovered() || item.Focused())
				if highlighted {
					item.Background(Color(t.C.Menu.ItemHover)).TextColor(Color(t.C.Menu.ItemHoverText))
				}
				StyleDisabled(item, option.Disabled)
				item.Children(func() {
					if option.Leading != nil {
						option.Leading()
					}
					ui.Text(c, option.Label).FontSize(p.FontSize).MinWidth(0).Shrink(1).
						SingleLine().Ellipsis("…").PassThrough()
					if option.Detail != "" {
						ui.Text(c, option.Detail).FontSize(t.Font.Size.Footnote).MinWidth(0).Shrink(1).
							SingleLine().Ellipsis("…").PassThrough()
					}
					ui.Spacer(c).PassThrough()
					mark := ui.Icon(c, IconSVG(IconCheck)).Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).PassThrough()
					if !checked {
						mark.Opacity(0)
					}
				})
				if !option.Disabled && item.Clicked() {
					if index := slices.Index(*selected, option.Value); index >= 0 {
						*selected = slices.Delete(*selected, index, index+1)
					} else {
						*selected = append(*selected, option.Value)
					}
					c.Invalidate()
				}
			}
		})
	})
	return root
}
