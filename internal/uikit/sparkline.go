package uikit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type SparklinePoint struct {
	At    time.Time
	Value *float64 // Nil breaks the line wherever a reading is missing.
}

type SparklineProps struct {
	Height float32
	Tone   Tone
	Label  string
	// Max is the value at the top edge; 0 means 1, for values given as
	// fractions.
	Max    float64
	Window time.Duration
	// Grid draws dashed lines at the top and middle over a solid baseline.
	Grid bool
}

func Sparkline(c *ui.Context, samples []SparklinePoint, props SparklineProps) ui.Element {
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
	now := c.Now().Round(0)
	start := now.Add(-props.Window)
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
		var line, fill ui.Path
		var x, y float32
		var hasPoint, inSegment bool
		for _, sample := range samples {
			if sample.Value == nil {
				if inSegment {
					fill.LineTo(x, r.Y+r.H).Close()
					inSegment = false
				}
				continue
			}
			position := float32(sample.At.Sub(start)) / float32(props.Window)
			x = left + (right-left)*min(max(position, 0), 1)
			y = base - (base-upper)*float32(min(max(*sample.Value/top, 0), 1))
			hasPoint = true
			if !inSegment {
				line.MoveTo(x, y)
				fill.MoveTo(x, r.Y+r.H).LineTo(x, y)
				inSegment = true
				continue
			}
			line.LineTo(x, y)
			fill.LineTo(x, y)
		}
		if !hasPoint {
			return
		}
		if inSegment {
			fill.LineTo(x, r.Y+r.H).Close()
		}
		painter.FillPathGradient(&fill, ui.LinearGradient{From: ink.Alpha(p.FillAlpha), To: ink.Alpha(0), Angle: 180})
		painter.StrokePath(&line, width, ink)
		var halo, dot ui.Path
		painter.FillPath(halo.Circle(x, y, p.HaloRadius), ink.Alpha(p.HaloAlpha))
		painter.FillPath(dot.Circle(x, y, p.DotRadius), ink)
	})
}
