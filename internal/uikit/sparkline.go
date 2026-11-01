package uikit

import (
	"image/color"

	"github.com/egoist/mygo/ui"
)

type SparklineProps struct {
	Height, StrokeWidth float32
	Fill, Line          color.NRGBA
}

func Sparkline(c *ui.Context, values []float32, props SparklineProps) ui.Element {
	return ui.Box(c).Height(props.Height).Draw(func(p *ui.Painter, r ui.Rect) {
		var line, fill ui.Path
		for i, value := range values {
			x := r.X + float32(i)*r.W/float32(len(values)-1)
			y := r.Y + r.H*(1-value)
			if i == 0 {
				line.MoveTo(x, y)
				fill.MoveTo(x, r.Y+r.H).LineTo(x, y)
				continue
			}
			line.LineTo(x, y)
			fill.LineTo(x, y)
		}
		fill.LineTo(r.X+r.W, r.Y+r.H).Close()
		p.FillPath(&fill, Color(props.Fill))
		p.StrokePath(&line, props.StrokeWidth, Color(props.Line))
	})
}
