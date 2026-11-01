package uikit

import (
	"image/color"
	"math"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func Color(c color.NRGBA) ui.Color {
	return ui.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// Install refreshes the native theme from the current tokens on each root build.
func Install(c *ui.Context) *ui.Theme {
	t := UITheme(theme.Current())
	c.SetTheme(t)
	return t
}

func Shadow(e ui.Element, layers theme.Shadow) ui.Element {
	return ShadowOpacity(e, layers, 1)
}

func AnimatedShadow(e ui.Element, key any, layers theme.Shadow, animation ui.Animation) ui.Element {
	animated := make(theme.Shadow, len(layers))
	for i, layer := range layers {
		shown := e.AnimateColor(struct {
			Key   any
			Layer int
		}{Key: key, Layer: i}, Color(layer.Color), animation)
		layer.Color.R, layer.Color.G, layer.Color.B, layer.Color.A = shown.R, shown.G, shown.B, shown.A
		animated[i] = layer
	}
	return Shadow(e, animated)
}

func ShadowOpacity(e ui.Element, layers theme.Shadow, opacity float32) ui.Element {
	// Both APIs append layers in painting order. Token shadows put the first
	// layer on top, so append them back to front, including zero-blur rings.
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]
		c := Color(layer.Color)
		c.A = uint8(math.Round(float64(float32(c.A) * opacity)))
		if layer.Inset {
			e = e.InsetShadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
			continue
		}
		e = e.Shadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
	}
	return e
}

func UITheme(t *theme.Theme) *ui.Theme {
	return &ui.Theme{
		Dark:           t.IsDark,
		Background:     Color(t.C.Window.Bg),
		Surface:        Color(t.C.Button.Bg),
		SurfaceHover:   Color(t.C.Button.SecondaryBgHover),
		SurfacePressed: Color(t.C.Button.BgPressed),
		Border:         Color(t.C.Button.Border),
		Text:           Color(t.Color.Text.Primary),
		TextMuted:      Color(t.Color.Text.Secondary),
		Accent:         Color(t.C.Button.PrimaryBg),
		AccentHover:    Color(t.C.Button.PrimaryBgHover),
		AccentPressed:  Color(t.C.Button.PrimaryBgPressed),
		AccentText:     Color(t.C.Button.PrimaryText),
		Danger:         Color(t.Color.Danger.Default),
		Warning:        Color(t.Color.Warning.Default),
		Success:        Color(t.Color.Success.Default),
		Selection:      Color(t.C.FileList.BgSelected),
		Focus:          Color(t.Color.Accent.Tint50),
		Inverse:        Color(t.C.Tooltip.Bg),
		InverseText:    Color(t.C.Tooltip.Text),
		Scrollbar:      Color(t.C.Scrollbar.Color),
		ScrollbarWidth: t.C.Scrollbar.ThumbWidth,
		Radius:         t.C.Button.Radius,
		Spacing:        t.Space.N4,
		FontSize:       t.C.Button.FontSize,
		Font:           strings.Join(t.Font.Family.Ui, ", "),
	}
}
