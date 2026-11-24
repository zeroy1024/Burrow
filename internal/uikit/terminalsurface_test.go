package uikit

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
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
					card := TerminalPaneCard(c, radius, colors, true)
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

func TestTerminalPaneFocusKeepsSplitGapClear(t *testing.T) {
	const scale = 2
	const left, top, width, height, gap = 4, 4, 32, 40, 8
	surface := theme.Dark()
	colors := TerminalColors{Background: color.NRGBA{R: 30, G: 30, B: 46, A: 255}}
	window := color.NRGBA{R: 232, G: 230, B: 230, A: 255}
	ring := color.RGBA(surface.C.PaneFocus.RingFocused[0].Color)
	gapStart, gapEnd := (left+width)*scale, (left+width+gap)*scale
	y := (top + height/2) * scale
	for active := range 2 {
		got := ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Background(Color(window)).Children(func() {
				for index := range 2 {
					props := PaneProps{Surface: surface, Multiple: true, Focused: index == active}
					pane := Pane(c, props).Absolute().Top(top).Left(float32(left+index*(width+gap))).Size(width, height)
					pane.Children(func() {
						card := TerminalPaneCard(c, surface.C.Pane.Radius, colors, true)
						StylePaneFocus(pane, card, props)
					})
				}
			})
		}, 80, 48, scale)
		for x := gapStart; x < gapEnd; x++ {
			if pixel := got.RGBAAt(x, y); pixel != color.RGBA(window) {
				t.Fatalf("focused pane %d paints into the divider gap at x=%d: %v", active, x, pixel)
			}
		}
		edge := [2]int{gapStart - 1, gapEnd}[active]
		if pixel := got.RGBAAt(edge, y); pixel != ring {
			t.Errorf("focused pane %d inner edge = %v, want %v", active, pixel, ring)
		}
	}
}
