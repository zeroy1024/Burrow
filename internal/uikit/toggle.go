package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type ToggleProps struct {
	Label    string
	Disabled bool
	Motion   MotionMode
}

func Switch(c *ui.Context, value *bool, props ToggleProps) ui.Element {
	t := theme.Current()
	p := t.C.Switch
	e := ui.SwitchBase(c, value).Size(p.W, p.H).Radius(p.H / 2).
		Label(props.Label).Disabled(props.Disabled).FocusRing(false)
	target := float32(0)
	if *value {
		target = 1
	}
	animation := ui.Animation{Spring: Spring(p.Animation)}
	if MotionReduced(c, props.Motion) {
		animation = Anim(c, t.Motion.Duration.Focus, props.Motion)
	}
	position := e.AnimateTo("knob", target, animation)
	pressTarget := float32(0)
	if e.Pressed() && !e.IsDisabled() {
		pressTarget = 1
	}
	press := e.AnimateTo("press", pressTarget, Anim(c, t.Motion.Duration.Fast, props.Motion))
	stretch := press * t.Space.N2
	e.Background(Color(p.Off).Mix(Color(p.On), position))
	rtl := c.Direction() == ui.RightToLeft
	if e.FocusVisible() && !e.IsDisabled() {
		Shadow(e, t.Shadow.ControlFocus)
	}
	e.Draw(func(painter *ui.Painter, rect ui.Rect) {
		at := position
		if rtl {
			at = 1 - at
		}
		diameter := rect.H - 2*p.Inset
		knob := ui.Rect{X: rect.X + p.Inset + at*(rect.W-rect.H-stretch), Y: rect.Y + p.Inset, W: diameter + stretch, H: diameter}
		for _, shadow := range p.KnobShadow {
			painter.Shadow(knob, diameter/2, shadow.X, shadow.Y, shadow.Blur, shadow.Spread, Color(shadow.Color))
		}
		painter.Fill(knob, Color(p.Knob), diameter/2)
	})
	return StyleDisabled(e, props.Disabled)
}

type CheckProps struct {
	Mixed, Disabled bool
	Motion          MotionMode
	// HideLabel keeps label for accessibility only, for a box in a table cell
	// whose column header already names it.
	HideLabel bool
}

func Checkbox(c *ui.Context, value *bool, label string, options ...CheckProps) ui.Element {
	props := CheckProps{}
	if len(options) > 0 {
		props = options[0]
	}
	t := theme.Current()
	p := t.C.Check
	row := ui.CheckboxBase(c, value).Gap(t.Space.N8).AlignItems(ui.Center).
		Label(label).Disabled(props.Disabled).FocusRing(false)
	if props.Mixed {
		row.Mixed()
	}
	row.Children(func() {
		active := *value || props.Mixed
		bgTarget, borderTarget, markTarget := ui.Transparent, Color(p.Border), float32(0)
		if active {
			bgTarget, borderTarget, markTarget = Color(p.On), Color(p.On), 1
		} else if row.Hovered() && !row.IsDisabled() {
			borderTarget = Color(p.On)
		}
		anim := Tween(t.Motion.Duration.Focus)
		box := ui.Box(c).Size(p.Size, p.Size).Radius(p.Radius).Shrink(0).
			Background(row.AnimateColor("bg", bgTarget, anim)).
			Border(t.Size.Divider, row.AnimateColor("border", borderTarget, anim)).PassThrough()
		if row.FocusVisible() && !row.IsDisabled() {
			Shadow(box, t.Shadow.ControlFocus)
		}
		alpha := Visual(row, "mark", markTarget, t.Motion.Duration.Focus)
		if alpha > 0 {
			markProgress := alpha
			if MotionReduced(c, props.Motion) {
				markProgress = 1
			}
			mixed := props.Mixed
			box.DrawOver(func(painter *ui.Painter, rect ui.Rect) {
				ink := Color(p.Mark).Alpha(alpha)
				if mixed {
					w := (rect.W / 2) * markProgress
					mark := ui.Rect{X: rect.X + (rect.W-w)/2, Y: rect.Y + rect.H/2 - t.Size.Divider/2,
						W: w, H: t.Size.Divider}
					painter.Fill(mark, ink, t.Size.Divider/2)
					return
				}
				var mark ui.Path
				x0, y0 := rect.X+rect.W/4, rect.Y+rect.H/2
				x1, y1 := rect.X+rect.W*0.45, rect.Y+rect.H*0.7
				x2, y2 := rect.X+rect.W*0.8, rect.Y+rect.H*0.3
				mark.MoveTo(x0, y0)
				if markProgress <= 0.45 {
					k := markProgress / 0.45
					mark.LineTo(x0+(x1-x0)*k, y0+(y1-y0)*k)
				} else {
					k := (markProgress - 0.45) / 0.55
					mark.LineTo(x1, y1).LineTo(x1+(x2-x1)*k, y1+(y2-y1)*k)
				}
				painter.StrokePath(&mark, t.Size.Divider*2, ink)
			})
		}
		if !props.HideLabel {
			ui.Text(c, label).FontSize(t.Font.Size.Body).PassThrough()
		}
	})
	return StyleDisabled(row, props.Disabled)
}

func Radio[T comparable](c *ui.Context, selected *T, value T, label string, options ...CheckProps) ui.Element {
	props := CheckProps{}
	if len(options) > 0 {
		props = options[0]
	}
	t := theme.Current()
	p := t.C.Check
	row := ui.RadioBase(c, selected, value).Gap(t.Space.N8).AlignItems(ui.Center).
		Label(label).Disabled(props.Disabled).FocusRing(false)
	row.Children(func() {
		on := *selected == value
		borderTarget, dotTarget := Color(p.Border), float32(0)
		if on {
			borderTarget, dotTarget = Color(p.On), 1
		} else if row.Hovered() && !row.IsDisabled() {
			borderTarget = Color(p.On)
		}
		circle := ui.Box(c).Size(p.Size, p.Size).Radius(p.Size/2).Shrink(0).
			Border(t.Size.Divider, row.AnimateColor("border", borderTarget, Tween(t.Motion.Duration.Focus))).Center().PassThrough()
		if row.FocusVisible() && !row.IsDisabled() {
			Shadow(circle, t.Shadow.ControlFocus)
		}
		dotAlpha := Visual(row, "dot-alpha", dotTarget, t.Motion.Duration.Focus)
		dotScale := float32(1)
		if MotionReduced(c, props.Motion) {
			row.Jump("dot-scale", dotTarget)
		} else {
			dotScale = row.AnimateTo("dot-scale", dotTarget, ui.Animation{Spring: Spring(t.C.Switch.Animation)})
		}
		if dotAlpha > 0 && dotScale > 0 {
			circle.Children(func() {
				size := p.RadioDot * dotScale
				ui.Box(c).Size(size, size).Radius(size / 2).Background(Color(p.On).Alpha(dotAlpha)).PassThrough()
			})
		}
		ui.Text(c, label).FontSize(t.Font.Size.Body).PassThrough()
	})
	return StyleDisabled(row, props.Disabled)
}
