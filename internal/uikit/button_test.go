package uikit

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

func TestActiveWindowToggleUsesAccentFeedback(t *testing.T) {
	dark := theme.Dark()
	for _, surface := range []*theme.Theme{theme.Light(), theme.WithAccent(dark, dark.Color.AccentChoices.Purple)} {
		t.Run(surface.Name, func(t *testing.T) {
			const inset = 16
			size := surface.C.IconButton.Size
			center := inset + size/2
			var on bool
			tt := ui.NewTester(func(c *ui.Context) {
				c.Root().Background(Color(surface.C.Window.Bg))
				IconButton(c, IconActivity, IconButtonProps{Variant: IconOnWindow, On: &on, Surface: surface}).
					Absolute().Left(inset).Top(inset)
			}, 80, 64)
			check := func(state string, tint color.NRGBA) {
				t.Helper()
				tt.Advance(surface.C.IconButton.Transition)
				reference := ui.Render(func(c *ui.Context) {
					c.Root().Background(Color(surface.C.Window.Bg))
					ui.Box(c).Absolute().Left(inset).Top(inset).Size(size, size).
						Radius(surface.C.IconButton.Radius).Background(Color(tint))
				}, 80, 64, 1)
				x, y := int(center), inset+3
				if got, want := tt.Image().RGBAAt(x, y), reference.RGBAAt(x, y); got != want {
					t.Errorf("%s background = %v, want %v", state, got, want)
				}
			}

			tt.ClickAt(center, center)
			if !on {
				t.Fatal("button did not toggle on")
			}
			tt.Move(0, 0)
			check("active", surface.Color.Accent.Tint10)
			tt.Move(center, center)
			check("active and hovered", surface.Color.Accent.Tint28)
			tt.Press(center, center)
			check("active and pressed", surface.Color.Accent.Tint35)
		})
	}
}
