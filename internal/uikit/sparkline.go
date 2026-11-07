package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type SparklineProps struct {
	Height float32
	Tone   Tone
	Label  string
	// Max is the value at the top edge; 0 means 1, for values given as
	// fractions.
	Max float64
	// Slots fixes how many samples span the width, so a history that is still
	// filling grows in from the right edge. 0 spreads the values across it.
	Slots int
	// Grid draws dashed lines at the top and middle over a solid baseline.
	Grid bool
}

func Sparkline(c *ui.Context, values []float64, props SparklineProps) ui.Element {
	p := theme.Current().C.Sparkline
	height := props.Height
	if height == 0 {
		height = p.Height
	}
	width := p.Stroke
	top := props.Max
	if top == 0 {
		top = 1
	}
	ink := Color(p.Line)
	if props.Tone != ToneNeutral {
		ink = ToneColor(props.Tone)
	}
	if props.Slots > 0 && len(values) > props.Slots {
		values = values[len(values)-props.Slots:]
	}
	return ui.Box(c).Height(height).FillWidth().Role(ui.RoleImage).Label(props.Label).Draw(func(painter *ui.Painter, r ui.Rect) {
		// The line stops a dot's radius short of the right and top edges so
		// the end dot stays inside the box.
		left, right := r.X, r.X+r.W-p.DotRadius
		upper, base := r.Y+p.DotRadius, r.Y+r.H-width/2
		if props.Grid {
			grid := Color(p.Grid)
			var dashes ui.Path
			middle := (upper + r.Y + r.H) / 2
			dashes.MoveTo(left, upper).LineTo(r.X+r.W, upper).MoveTo(left, middle).LineTo(r.X+r.W, middle)
			painter.StrokePath(dashes.Cap(ui.ButtCap).Dash(0, 2, 3), 1, grid)
			painter.Line(left, r.Y+r.H-0.5, r.X+r.W, r.Y+r.H-0.5, 1, grid)
		}
		if len(values) == 0 {
			return
		}
		step := (right - left) / float32(max(len(values), props.Slots, 2)-1)
		var line, fill ui.Path
		var x, y float32
		for i, value := range values {
			x = right - float32(len(values)-1-i)*step
			y = base - (base-upper)*float32(min(max(value/top, 0), 1))
			if i == 0 {
				line.MoveTo(x, y)
				fill.MoveTo(x, r.Y+r.H).LineTo(x, y)
				continue
			}
			line.LineTo(x, y)
			fill.LineTo(x, y)
		}
		fill.LineTo(x, r.Y+r.H).Close()
		painter.FillPathGradient(&fill, ui.LinearGradient{From: ink.Alpha(p.FillAlpha), To: ink.Alpha(0), Angle: 180})
		painter.StrokePath(&line, width, ink)
		var halo, dot ui.Path
		painter.FillPath(halo.Circle(x, y, p.HaloRadius), ink.Alpha(p.HaloAlpha))
		painter.FillPath(dot.Circle(x, y, p.DotRadius), ink)
	})
}
