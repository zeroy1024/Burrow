package uikit

import (
	"image/color"

	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type IconButtonProps struct {
	Label          string
	Key            any
	Size           Size
	Variant        IconVariant
	On             *bool
	IconOn         *ui.SVG
	Shortcut       string
	Disabled       bool
	Surface        *theme.Theme
	IconTransition *ui.ContentTransition
}

type IconVariant uint8

const (
	IconPlain IconVariant = iota
	IconFilled
	IconOnWindow
)

type IconProps struct {
	Size Size
	Tone Tone
}

type IconSource interface {
	IconName | *ui.SVG
}

func resolveIcon[T IconSource](source T) *ui.SVG {
	switch icon := any(source).(type) {
	case IconName:
		return IconSVG(icon)
	case *ui.SVG:
		return icon
	}
	panic("uikit: unsupported icon source")
}

func Icon[T IconSource](c *ui.Context, source T, props IconProps) ui.Element {
	t := theme.Current()
	size := t.C.IconButton.IconSize
	switch props.Size {
	case SizeXs:
		size = t.C.IconButton.IconSizeXs
	case SizeSm:
		size = t.C.IconButton.IconSizeSm
	}
	ink := Color(t.C.IconButton.Icon)
	if props.Tone != ToneNeutral {
		ink = ToneColor(props.Tone)
	}
	return ui.Icon(c, resolveIcon(source)).Size(size, size).TextColor(ink)
}

func IconButton[T IconSource](c *ui.Context, icon T, props IconButtonProps) ui.Element {
	t := theme.Current()
	if props.Surface != nil {
		t = props.Surface
	}
	if props.Key != nil {
		c = c.Key(props.Key)
	}
	var button ui.Element
	if props.On == nil {
		button = ui.ButtonBase(c)
	} else {
		button = ui.ToggleBase(c, props.On)
	}
	button.Label(props.Label).Disabled(props.Disabled)
	on := props.On != nil && *props.On
	styleIconButton(button, t, props.Variant, on)
	size, glyph := t.C.IconButton.Size, t.C.IconButton.IconSize
	switch props.Size {
	case SizeSm:
		size, glyph = t.C.IconButton.SizeSm, t.C.IconButton.IconSizeSm
	case SizeXs:
		size, glyph = t.C.IconButton.SizeXs, t.C.IconButton.IconSizeXs
	}
	button.Size(size, size)
	Tip(c, button, props.Label, props.Shortcut)
	button.Children(func() {
		// ToggleBase has processed this frame's value input before resolving the on glyph.
		image := resolveIcon(icon)
		if props.On != nil && *props.On && props.IconOn != nil {
			image = props.IconOn
		}
		imageElement := ui.Icon(c, image).Size(glyph, glyph).PassThrough()
		if props.IconTransition != nil {
			imageElement.ContentTransition(*props.IconTransition)
		}
	})
	return StyleDisabled(button, props.Disabled)
}

type IconTileProps struct {
	// Size picks the tile: SizeSm beside a line of text, SizeLg beside a
	// title; other sizes suit a list row.
	Size Size
	// Background replaces the neutral fill, for a status or brand color.
	Background color.NRGBA
}

// IconTile sets a glyph, or a logo the caller builds, on a rounded fill.
func IconTile(c *ui.Context, props IconTileProps, glyph func()) ui.Element {
	p := theme.Current().C.IconTile
	size, radius := p.Size, p.Radius
	switch props.Size {
	case SizeXs, SizeSm:
		size, radius = p.SizeSm, p.RadiusSm
	case SizeLg:
		size, radius = p.SizeLg, p.RadiusLg
	}
	background := p.Bg
	if props.Background.A != 0 {
		background = props.Background
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Background(Color(background)).Center().Children(glyph)
}
