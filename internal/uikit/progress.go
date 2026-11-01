package uikit

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type ProgressProps struct {
	Value, Height float32
	Track, Fill   color.NRGBA
	Animation     ui.Animation
}

func Progress(c *ui.Context, props ProgressProps) ui.Element {
	p := theme.Current().C.Progress
	bar := ui.Box(c).Height(props.Height).Radius(p.Radius).Background(Color(props.Track)).Clip().
		Role(ui.RoleProgress).Range(0, 1, float64(props.Value))
	bar.FillWidth()
	shown := bar.AnimateTo("progress", props.Value, props.Animation)
	return bar.Children(func() {
		ui.Box(c).Height(props.Height).WidthPercent(min(max(shown, 0), 1) * 100).Radius(p.Radius).Background(Color(props.Fill))
	})
}
