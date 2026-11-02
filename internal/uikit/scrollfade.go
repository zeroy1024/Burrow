package uikit

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type ScrollFadeProps struct {
	Height      float32
	Translucent bool
	Gradient    bool
	// Uniform keeps an entire control bar frosted instead of fading to clear.
	Uniform              bool
	Background, MaskTint ui.Color
}

func ScrollFade(c *ui.Context, edge Edge, options ...ScrollFadeProps) ui.Element {
	t := theme.Current()
	fade := t.C.FadeBar
	props := ScrollFadeProps{Gradient: true}
	if len(options) != 0 {
		props = options[0]
	}
	if props.Height == 0 {
		props.Height = t.Size.FadeExtent
	}
	if props.Background.A == 0 {
		props.Background = Color(t.Color.Surface.Card)
	}
	if props.MaskTint.A == 0 {
		props.MaskTint = Color(t.Color.Text.Primary)
	}
	angle := float32(180)
	if edge == EdgeBottom {
		angle = 0
	}
	root := ui.Box(c).Absolute().Left(0).Right(0).Height(props.Height).PassThrough()
	if edge == EdgeBottom {
		root.Bottom(0)
	} else {
		root.Top(0)
	}
	root.Children(func() {
		band := func() ui.Element { return ui.Box(c).Absolute().Fill().PassThrough() }
		if props.Gradient {
			band().LinearGradient(ui.LinearGradient{From: props.Background, To: ui.Transparent, Angle: angle})
			return
		}
		if !props.Translucent {
			band().Background(props.Background)
			return
		}
		if props.Uniform {
			band().Material(glass.Blur{Radius: t.Glass.Bar.Blur})
			band().Background(props.Background.Alpha(fade.TintMid))
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
	})
	return root
}
