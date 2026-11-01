package uikit

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type ScrollFadeProps struct {
	Height               float32
	Bottom, Translucent  bool
	Background, MaskTint ui.Color
}

func ScrollFade(c *ui.Context, props ScrollFadeProps) {
	fade := theme.Current().C.FadeBar
	angle := float32(180)
	if props.Bottom {
		angle = 0
	}
	band := func() ui.Element {
		element := ui.Box(c).Absolute().Left(0).Right(0).Height(props.Height).PassThrough()
		if props.Bottom {
			return element.Bottom(0)
		}
		return element.Top(0)
	}
	if !props.Translucent {
		band().Background(props.Background)
		return
	}
	// Mask progressively stronger blur layers before adding the surface tint.
	layers := [...]struct{ radius, solid, end float32 }{
		{radius: fade.Blur1, solid: fade.Mask1Solid, end: fade.Mask1End},
		{radius: fade.Blur2, solid: fade.Mask2Solid, end: fade.Mask2End},
		{radius: fade.Blur3, solid: fade.Mask3Solid, end: fade.Mask3End},
	}
	for _, layer := range layers {
		band().Material(glass.Blur{
			Radius: layer.radius,
			Mask: &ui.LinearGradient{
				From: props.MaskTint, To: ui.Transparent, Angle: angle, Start: layer.solid, End: layer.end,
			},
		})
	}
	band().LinearGradient(ui.LinearGradient{Angle: angle, Stops: []ui.GradientStop{
		{Color: props.Background.Alpha(fade.TintStart), At: 0},
		{Color: props.Background.Alpha(fade.TintMid), At: fade.TintMidStop},
		{Color: props.Background.Alpha(fade.TintEnd), At: 1},
	}})
}
