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
	animation := ui.Snappy
	if MotionReduced(c, props.Motion) {
		animation = Anim(c, t.Motion.Duration.Focus, props.Motion)
	}
	position := e.AnimateTo("knob", target, animation)
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
		knob := ui.Rect{X: rect.X + p.Inset + at*(rect.W-rect.H), Y: rect.Y + p.Inset, W: diameter, H: diameter}
		for _, shadow := range p.KnobShadow {
			painter.Shadow(knob, diameter/2, shadow.X, shadow.Y, shadow.Blur, shadow.Spread, Color(shadow.Color))
		}
		painter.Fill(knob, Color(p.Knob), diameter/2)
	})
	return StyleDisabled(e, props.Disabled)
}

type CheckProps struct {
	Mixed, Disabled bool
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
		box := ui.Box(c).Size(p.Size, p.Size).Radius(p.Radius).Shrink(0).Border(t.Size.Divider, Color(p.Border)).PassThrough()
		if *value || props.Mixed {
			box.Background(Color(p.On)).BorderColor(Color(p.On))
		} else if row.Hovered() {
			box.BorderColor(Color(p.On))
		}
		if row.FocusVisible() && !row.IsDisabled() {
			Shadow(box, t.Shadow.ControlFocus)
		}
		box.DrawOver(func(painter *ui.Painter, rect ui.Rect) {
			if props.Mixed {
				mark := ui.Rect{X: rect.X + rect.W/4, Y: rect.Y + rect.H/2 - t.Size.Divider/2,
					W: rect.W / 2, H: t.Size.Divider}
				painter.Fill(mark, Color(p.Mark), t.Size.Divider/2)
			} else if *value {
				var mark ui.Path
				mark.MoveTo(rect.X+rect.W/4, rect.Y+rect.H/2).
					LineTo(rect.X+rect.W*0.45, rect.Y+rect.H*0.7).
					LineTo(rect.X+rect.W*0.8, rect.Y+rect.H*0.3)
				painter.StrokePath(&mark, t.Size.Divider*2, Color(p.Mark))
			}
		})
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
		circle := ui.Box(c).Size(p.Size, p.Size).Radius(p.Size/2).Shrink(0).
			Border(t.Size.Divider, Color(p.Border)).Center().PassThrough()
		if row.FocusVisible() && !row.IsDisabled() {
			Shadow(circle, t.Shadow.ControlFocus)
		}
		if *selected == value {
			circle.BorderColor(Color(p.On)).Children(func() {
				ui.Box(c).Size(p.RadioDot, p.RadioDot).Radius(p.RadioDot / 2).Background(Color(p.On)).PassThrough()
			})
		}
		ui.Text(c, label).FontSize(t.Font.Size.Body).PassThrough()
	})
	return StyleDisabled(row, props.Disabled)
}
