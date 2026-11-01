package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type SearchVariant uint8

const (
	SearchSunken SearchVariant = iota
	SearchFilter
)

type SearchProps struct {
	Variant                 SearchVariant
	Label, Placeholder, Kbd string
	Icon                    *ui.SVG
	Handle                  *ui.Handle
	Height                  float32
	Disabled, ReadOnly      bool
}

type SearchParts struct {
	Root, Input, Clear ui.Element
	Changed            bool
}

// SearchField keeps the input's full hit area; only its clear action takes a
// separate hit target. Cmd+F remains scoped to the active page by its caller.
func SearchField(c *ui.Context, value *string, props SearchProps) SearchParts {
	previous := *value
	t := theme.Current()
	p := t.C.Search
	height := props.Height
	if height == 0 {
		height = p.Height
	}
	shortcut := props.Kbd
	rightPadding, leftPadding := p.PaddingX, p.PaddingX
	if props.Icon != nil {
		leftPadding += t.Size.Icon + p.Gap
	}
	if shortcut != "" {
		width, _ := ui.ShapeText(shortcut, ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: p.KbdSize}, 0).Size()
		rightPadding += width + p.Gap
	}
	clear := props.Variant == SearchFilter && *value != "" && !props.ReadOnly
	if clear {
		rightPadding += height
	}
	parts := SearchParts{Root: ui.Box(c).Height(height).Shrink(0).Radius(p.Radius).Disabled(props.Disabled)}
	if props.Variant == SearchSunken {
		parts.Root.Margin(0, p.MarginX, p.MarginBottom, p.MarginX).Background(Color(p.Bg))
	} else {
		parts.Root.Background(Color(t.C.Input.Bg)).Border(t.C.Input.BorderWidth, Color(t.C.Input.Border))
	}
	parts.Root.Children(func() {
		parts.Input = ui.TextInputBase(c.Key("input"), value).Fill().MinWidth(0).
			Padding(0, rightPadding, 0, leftPadding).Radius(p.Radius).
			FontSize(p.FontSize).FixedLineHeight(p.LineHeight).TextColor(Color(t.Color.Text.Primary)).
			Placeholder(props.Placeholder).Label(props.Label).ReadOnly(props.ReadOnly).FocusRing(false)
		if props.Handle != nil {
			parts.Input.Bind(props.Handle)
		}
		if shortcut != "" {
			parts.Input.Description(shortcut)
		}
		if !props.ReadOnly && parts.Input.Shortcut(0, ui.KeyEscape) {
			*value = ""
			c.Invalidate()
		}
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Padding(0, p.PaddingX).
			Gap(p.Gap).AlignItems(ui.Center).PassThrough().Children(func() {
			if props.Icon != nil {
				ui.Icon(c, props.Icon).Size(t.Size.Icon, t.Size.Icon).Shrink(0).TextColor(Color(p.Text)).PassThrough()
			}
			ui.Spacer(c).PassThrough()
			if shortcut != "" {
				Kbd(c, shortcut, KbdProps{Tone: KbdFlat, FontSize: p.KbdSize, Text: &p.KbdText}).Shrink(0)
			}
		})
		if clear {
			parts.Clear = IconButton(c, IconClose, IconButtonProps{Key: "clear", Label: "清除搜索"}).
				Absolute().Top(0).Bottom(0).Right(0).Size(height, height).KeepFocus()
			if parts.Clear.Clicked() {
				*value = ""
				parts.Input.Focus()
				c.Invalidate()
			}
		}
	})
	StyleDisabled(parts.Root, props.Disabled)
	focus := float32(0)
	if parts.Input.FocusVisible() && !parts.Input.IsDisabled() {
		focus = 1
	}
	ShadowOpacity(parts.Root, t.C.Input.FocusShadow, Visual(parts.Root, "input-focus", focus, t.C.Input.Transition))
	parts.Changed = parts.Input.Changed() || *value != previous
	return parts
}
