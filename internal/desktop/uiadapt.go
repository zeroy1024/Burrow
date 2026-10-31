package desktop

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func tokenColor(c color.NRGBA) ui.Color {
	return ui.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

func windowColor(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func applyShadow(e ui.Element, layers theme.Shadow) ui.Element {
	return applyShadowOpacity(e, layers, 1)
}

func applyAnimatedShadow(e ui.Element, key any, layers theme.Shadow, animation ui.Animation) ui.Element {
	animated := make(theme.Shadow, len(layers))
	for i, layer := range layers {
		shown := e.AnimateColor(struct {
			Key   any
			Layer int
		}{Key: key, Layer: i}, tokenColor(layer.Color), animation)
		layer.Color.R, layer.Color.G, layer.Color.B, layer.Color.A = shown.R, shown.G, shown.B, shown.A
		animated[i] = layer
	}
	return applyShadow(e, animated)
}

func applyShadowOpacity(e ui.Element, layers theme.Shadow, opacity float32) ui.Element {
	// Both APIs append layers in painting order. Token shadows put the first
	// layer on top, so append them back to front, including zero-blur rings.
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]
		c := tokenColor(layer.Color)
		c.A = uint8(math.Round(float64(float32(c.A) * opacity)))
		if layer.Inset {
			e = e.InsetShadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
			continue
		}
		e = e.Shadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
	}
	return e
}

func tokenSpring(s theme.Spring) ui.SpringOptions {
	if s.Stiffness != 0 || s.Damping != 0 {
		return ui.SpringOptions{Stiffness: s.Stiffness, Damping: s.Damping}
	}
	return ui.SpringOptions{Duration: s.Duration, Bounce: s.Bounce}
}

func tokenEasing(b theme.Bezier) ui.Easing {
	return ui.CubicBezier(b[0], b[1], b[2], b[3])
}

// MyGo suppresses scalar animations under ReduceMotion, but AnimateColor keeps
// color and alpha transitions. Encode visual-only values in its alpha channel.
func animateVisual(e ui.Element, key any, target float32, duration time.Duration) float32 {
	c := ui.Color{A: uint8(math.Round(float64(target * 255)))}
	value := e.AnimateColor(key, c, ui.Animation{Duration: duration})
	if value.A == c.A {
		return target
	}
	return float32(value.A) / 255
}

func nativeTheme(t *theme.Theme) *ui.Theme {
	return &ui.Theme{
		Dark:           t.IsDark,
		Background:     tokenColor(t.C.Window.Bg),
		Surface:        tokenColor(t.C.IconButton.Bg),
		SurfaceHover:   tokenColor(t.C.Button.BgHover),
		SurfacePressed: tokenColor(t.C.Button.BgPressed),
		Border:         tokenColor(t.C.Button.Border),
		Text:           tokenColor(t.C.Pane.Name),
		TextMuted:      tokenColor(t.C.Pane.Host),
		Accent:         tokenColor(t.C.Button.PrimaryBg),
		AccentHover:    tokenColor(t.C.Button.PrimaryBg),
		AccentPressed:  tokenColor(t.C.Button.PrimaryBg),
		AccentText:     tokenColor(t.C.Button.PrimaryText),
		Danger:         tokenColor(t.C.Desktop.Danger),
		Warning:        tokenColor(t.C.Tab.Spinner),
		Success:        tokenColor(t.C.Tab.DotConnected),
		Selection:      tokenColor(t.C.FileList.BgSelected),
		Focus:          tokenColor(t.Color.Accent.Tint50),
		Inverse:        tokenColor(t.C.Tooltip.Bg),
		InverseText:    tokenColor(t.C.Tooltip.Text),
		Scrollbar:      tokenColor(t.C.Scrollbar.Color),
		ScrollbarWidth: t.C.Scrollbar.ThumbWidth,
		Radius:         t.C.Button.Radius,
		Spacing:        t.Space.N4,
		FontSize:       t.C.Button.FontSize,
		Font:           strings.Join(t.Font.Family.Ui, ", "),
	}
}
