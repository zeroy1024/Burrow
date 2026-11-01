package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type SparklineProps struct {
	Height float32
	Tone   Tone
	Label  string
}

func Sparkline(c *ui.Context, values []float64, props SparklineProps) ui.Element {
	p := theme.Current().C.Sparkline
	height := props.Height
	if height == 0 {
		height = p.Height
	}
	stroke := Color(p.Line)
	if props.Tone != ToneNeutral {
		stroke = ToneColor(props.Tone)
	}
	return ui.Box(c).Height(height).FillWidth().Role(ui.RoleImage).Label(props.Label).Draw(func(painter *ui.Painter, r ui.Rect) {
		if len(values) == 0 {
			return
		}
		var line, fill ui.Path
		for i, value := range values {
			x := r.X + float32(i)*r.W/float32(max(len(values)-1, 1))
			y := r.Y + r.H*(1-float32(min(max(value, 0), 1)))
			if i == 0 {
				line.MoveTo(x, y)
				fill.MoveTo(x, r.Y+r.H).LineTo(x, y)
				continue
			}
			line.LineTo(x, y)
			fill.LineTo(x, y)
		}
		fill.LineTo(r.X+r.W, r.Y+r.H).Close()
		painter.FillPath(&fill, stroke.Alpha(p.FillAlpha))
		painter.StrokePath(&line, p.Stroke, stroke)
	})
}
