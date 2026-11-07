package uikit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type SpinnerProps struct {
	Size   Size
	Tone   Tone
	Label  string
	Motion MotionMode
}

func Spinner(c *ui.Context, props SpinnerProps) ui.Element {
	t := theme.Current()
	size := t.C.Spinner.SizeMd
	switch props.Size {
	case SizeXs:
		size = t.C.Spinner.SizeXs
	case SizeSm:
		size = t.C.Spinner.SizeSm
	}
	ink := Color(t.C.Spinner.Color)
	if props.Tone != ToneNeutral {
		ink = ToneColor(props.Tone)
	}
	label := props.Label
	if label == "" {
		label = locales.MessageProcessing2()
	}
	reduced := MotionReduced(c, props.Motion)
	period := t.C.Spinner.Period
	// Keep the SDK's progress role/range, replacing only its fixed 900ms paint cycle.
	e := ui.Spinner(c).Size(size, size).Label(label).TextColor(ink)
	return e.Draw(func(p *ui.Painter, r ui.Rect) {
		const spokes = 12
		lead := 0
		if !reduced {
			elapsed := p.Now().UnixNano() % int64(period)
			step := int64(period) / spokes
			lead = int(elapsed / step)
			p.After(time.Duration(step - elapsed%step))
		}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		inner, outer, width := r.W*0.22, r.W*0.46, r.W*0.09
		for index := range spokes {
			angle := float64(index) * 2 * math.Pi / spokes
			x, y := float32(math.Sin(angle)), float32(-math.Cos(angle))
			age := (lead - index + spokes) % spokes
			alpha := 1 - float32(age)/spokes*0.85
			var line ui.Path
			line.MoveTo(cx+x*inner, cy+y*inner).LineTo(cx+x*outer, cy+y*outer)
			p.StrokePath(&line, width, ink.Alpha(alpha))
		}
	})
}

func TabSpinner(c *ui.Context, props SpinnerProps) ui.Element {
	t := theme.Current().C.Tab
	reduced := MotionReduced(c, props.Motion)
	e := ui.Box(c).Size(t.SpinnerSize, t.SpinnerSize).Shrink(0).Role(ui.RoleProgress).
		Range(0, 1, -1).Label(locales.MessageConnecting2())
	return e.Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		radius := (r.W - t.SpinnerStroke) / 2
		var track ui.Path
		track.Circle(cx, cy, radius)
		p.StrokePath(&track, t.SpinnerStroke, Color(t.SpinnerTrack))
		phase := float32(0)
		if !reduced {
			phase = float32(p.Now().UnixNano()%int64(t.SpinnerPeriod)) / float32(t.SpinnerPeriod)
			p.AnimationFrame()
		}
		var arc ui.Path
		const steps = 32
		for index := range steps + 1 {
			angle := float64(phase+float32(index)/steps*0.75) * 2 * math.Pi
			x, y := cx+radius*float32(math.Cos(angle)), cy+radius*float32(math.Sin(angle))
			if index == 0 {
				arc.MoveTo(x, y)
			} else {
				arc.LineTo(x, y)
			}
		}
		p.StrokePath(&arc, t.SpinnerStroke, Color(t.Spinner))
	})
}

type StatusDotProps struct {
	Motion MotionMode
}

// StatusDot is for tabs and their tooltips; catalog rows carry no connection state.
func StatusDot(c *ui.Context, state ConnState, options ...StatusDotProps) ui.Element {
	props := StatusDotProps{}
	if len(options) != 0 {
		props = options[0]
	}
	if state == ConnConnecting {
		return TabSpinner(c, SpinnerProps{Motion: props.Motion})
	}
	t := theme.Current()
	p := t.C.Tab
	label, ink := state.Label(), Color(p.RingDisconnected)
	if state == ConnConnected {
		ink = Color(p.DotConnected)
	} else if state == ConnError {
		ink = Color(p.DotError)
	}
	e := ui.Box(c).Size(p.SpinnerSize, p.SpinnerSize).Role(ui.RoleImage).Label(label).PassThrough()
	return e.Draw(func(painter *ui.Painter, r ui.Rect) {
		dot := ui.Rect{X: r.X + (r.W-p.DotSize)/2, Y: r.Y + (r.H-p.DotSize)/2, W: p.DotSize, H: p.DotSize}
		if state == ConnDisconnected {
			painter.Stroke(dot, ink, p.DotSize/2, p.RingWidth)
			return
		}
		if state == ConnConnected {
			halo := t.Size.DotHalo
			painter.Fill(ui.Rect{X: dot.X - halo, Y: dot.Y - halo, W: dot.W + 2*halo, H: dot.H + 2*halo}, Color(t.Color.Success.Halo), (p.DotSize+2*halo)/2)
		}
		painter.Fill(dot, ink, p.DotSize/2)
	})
}
