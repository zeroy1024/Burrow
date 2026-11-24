package uikit

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalPaneCardCompositesChromeBeforeClipping(t *testing.T) {
	const radius = 10
	colors := TerminalColors{Background: color.NRGBA{R: 30, G: 30, B: 46, A: 255}}
	background := Color(colors.Background)
	render := func(withChrome, translucent bool) *image.RGBA {
		return ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(232, 230, 230)).Children(func() {
				ui.Box(c).Absolute().Top(8).Left(8).Size(64, 48).Children(func() {
					if !withChrome {
						ui.Box(c).Fill().Radius(radius).Background(background)
						return
					}
					card := TerminalPaneCard(c, radius, colors)
					card.Children(func() {
						ScrollFade(c, EdgeBottom, ScrollFadeProps{
							Height: 24, Background: background, Translucent: translucent,
						})
					})
				})
			})
		}, 80, 64, 2)
	}
	corner := image.Rect(12, 80, 50, 116)
	reference := render(false, false)
	for _, translucent := range []bool{false, true} {
		got := render(true, translucent)
		for y := corner.Min.Y; y < corner.Max.Y; y++ {
			for x := corner.Min.X; x < corner.Max.X; x++ {
				if got.RGBAAt(x, y) != reference.RGBAAt(x, y) {
					t.Fatalf("translucent=%t, corner pixel (%d, %d) = %v, single rounded fill = %v",
						translucent, x, y, got.RGBAAt(x, y), reference.RGBAAt(x, y))
				}
			}
		}
	}
}
