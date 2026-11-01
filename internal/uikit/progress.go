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
	Tone          ProgressTone
	Thin          bool
	Motion        MotionMode
	Label         string
}

type ProgressTone uint8

const (
	ProgressRunning ProgressTone = iota
	ProgressPaused
	ProgressDone
	ProgressError
)

func Progress(c *ui.Context, props ProgressProps) ui.Element {
	t := theme.Current()
	p := t.C.Progress
	height := props.Height
	if height == 0 {
		height = p.Height
	}
	if props.Thin {
		height = t.Space.N2
	}
	track, fill := p.Track, p.Fill
	switch props.Tone {
	case ProgressPaused:
		fill = p.FillPaused
	case ProgressDone:
		fill = p.FillDone
	case ProgressError:
		fill = p.FillError
	}
	if props.Track.A != 0 {
		track = props.Track
	}
	if props.Fill.A != 0 {
		fill = props.Fill
	}
	bar := ui.Box(c).Height(height).Radius(p.Radius).Background(Color(track)).Clip().FillWidth().
		Role(ui.RoleProgress).Label(props.Label)
	if props.Value < 0 {
		bar.Value("进行中").Range(0, 1, -1)
		if MotionReduced(c, props.Motion) {
			bar.Children(func() { ui.Box(c).Fill().Stripes(Color(fill), height*2, height*2, 45) })
		} else {
			phase := bar.Loop("uikit.progress.indeterminate", t.Motion.Duration.Spin, ui.Linear)
			bar.Children(func() {
				ui.Box(c).Absolute().Top(0).Bottom(0).WidthPercent(40).Left(bar.Bounds().W * (phase*1.4 - .4)).Radius(p.Radius).Background(Color(fill))
			})
		}
		return bar
	}
	bar.Range(0, 1, float64(props.Value))
	shown := props.Value
	if MotionReduced(c, props.Motion) {
		bar.Jump("progress", props.Value)
	} else {
		shown = bar.AnimateTo("progress", props.Value, props.Animation)
	}
	return bar.Children(func() {
		ui.Box(c).Height(height).WidthPercent(min(max(shown, 0), 1) * 100).Radius(p.Radius).Background(Color(fill))
	})
}
