package uikit

import (
	"math"
	"strconv"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ConnState uint8

const (
	ConnDisconnected ConnState = iota
	ConnConnecting
	ConnConnected
	ConnError
)

type SessionTabVariant uint8

const (
	SessionTabTerminal SessionTabVariant = iota
	SessionTabFiles
)

type SessionTabProps struct {
	Key                   any
	Title, Hint           string
	Selected              bool
	Variant               SessionTabVariant
	State                 ConnState
	Handle                *ui.Handle
	Extra                 int
	CloseLabel, DragLabel string
	CloseIcon             *ui.SVG
}

func SessionTab(c *ui.Context, props SessionTabProps) ui.Element {
	t := theme.Current()
	p := t.C.Tab
	button := ui.ButtonBase(c).Key(props.Key).Height(p.Height).
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius)
	if props.Handle != nil {
		button.Bind(props.Handle)
	}
	if props.Variant == SessionTabTerminal {
		button.Transition(ui.ElementTransition{Position: true, Animation: &ui.Animation{Spring: Spring(t.C.ReorderMotion.TabDisplace)}})
	}
	button.AlignItems(ui.Center).Role(ui.RoleTab).Label(props.Title).Selected(props.Selected)
	foreground := Color(p.Text)
	if props.Selected {
		foreground = Color(p.TextSelected)
	}
	if props.State == ConnDisconnected {
		foreground = Color(p.TextDisconnected)
	} else if props.Variant == SessionTabFiles && props.State == ConnError && props.Selected {
		foreground = Color(p.TextErrorSelected)
	}
	animation := Tween(p.StateTransition)
	if props.Variant == SessionTabFiles {
		Tip(c, button, props.Hint)
		button.TextColor(button.AnimateColor("text", foreground, animation))
	}
	background := ui.Transparent
	if props.Selected {
		background = Color(p.BgSelected)
		Shadow(button, p.ShadowSelected)
	} else if button.Hovered() {
		background = Color(p.BgHover)
	}
	button.Background(button.AnimateColor("background", background, animation))
	if props.Variant == SessionTabTerminal {
		button.TextColor(button.AnimateColor("text", foreground, animation))
		Tip(c, button, props.Hint)
	}
	return button
}

type SessionTabParts struct {
	Close, DragHandle ui.Element
}

// SessionTabContents leaves close, activation, menus and drag ownership with the caller.
func SessionTabContents(c *ui.Context, root ui.Element, props SessionTabProps) SessionTabParts {
	t := theme.Current()
	p := t.C.Tab
	ui.Box(c).Width(p.SpinnerSize).Height(p.ContentHeight).Shrink(0).Center().Children(func() { TabConnectionIndicator(c, props.State) })
	ui.Text(c, props.Title).FontSize(p.FontSize).FontWeight(p.WeightSelected).FixedLineHeight(p.ContentHeight).Top(p.TitleOffset).SingleLine()
	if props.Extra > 0 {
		ui.Text(c, "+"+strconv.Itoa(props.Extra)).FontSize(t.Font.Size.Caption).FixedLineHeight(p.ContentHeight).Top(p.CountOffset).TextColor(Color(p.Text))
	}
	parts := SessionTabParts{Close: IconButton(c, props.CloseIcon, IconButtonProps{Label: props.CloseLabel}).Size(t.Size.Icon, p.ContentHeight)}
	visible := float32(0)
	if props.Selected || root.Hovered() || parts.Close.Focused() {
		visible = 1
	}
	parts.Close.Opacity(Visual(parts.Close, "visible", visible, p.StateTransition))
	if props.Variant == SessionTabTerminal {
		parts.DragHandle = ui.Box(c).Key("tab-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(p.DragHandleInset).Label(props.DragLabel)
	}
	return parts
}

func TabConnectionIndicator(c *ui.Context, state ConnState) {
	t := theme.Current().C.Tab
	reduceMotion := c.Preferences().ReduceMotion
	color := Color(t.DotConnected)
	switch state {
	case ConnDisconnected:
		color = Color(t.RingDisconnected)
	case ConnError:
		color = Color(t.DotError)
	case ConnConnecting:
		color = Color(t.Spinner)
	}
	indicator := ui.Box(c).Key("connection-indicator").Size(t.SpinnerSize, t.SpinnerSize)
	color = indicator.AnimateColor("state", color, Tween(t.StateTransition))
	filledTarget, ringTarget, spinnerTarget := float32(0), float32(0), float32(0)
	switch {
	case state == ConnDisconnected:
		ringTarget = 1
	case state == ConnConnecting && !reduceMotion:
		spinnerTarget = 1
	default:
		filledTarget = 1
	}
	filled := Visual(indicator, "filled", filledTarget, t.StateTransition)
	ring := Visual(indicator, "ring", ringTarget, t.StateTransition)
	spinner := Visual(indicator, "spinner", spinnerTarget, t.StateTransition)
	indicator.Draw(func(p *ui.Painter, r ui.Rect) {
		alpha := func(color ui.Color, opacity float32) ui.Color {
			color.A = uint8(float32(color.A) * opacity)
			return color
		}
		small := ui.Rect{
			X: r.X + (r.W-t.DotSize)/2, Y: r.Y + (r.H-t.DotSize)/2,
			W: t.DotSize, H: t.DotSize,
		}
		if filled > 0 {
			p.Fill(small, alpha(color, filled), t.DotSize/2)
		}
		if ring > 0 {
			p.Stroke(small, alpha(Color(t.RingDisconnected), ring), t.DotSize/2, t.RingWidth)
		}
		if spinner == 0 {
			return
		}
		var track ui.Path
		track.Circle(r.X+r.W/2, r.Y+r.H/2, (r.W-t.SpinnerStroke)/2)
		p.StrokePath(&track, t.SpinnerStroke, alpha(Color(t.SpinnerTrack), spinner))
		phase := float32(p.Now().UnixNano()%int64(t.SpinnerPeriod)) / float32(t.SpinnerPeriod)
		var arc ui.Path
		cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, (r.W-t.SpinnerStroke)/2
		steps := 16
		for i := range steps + 1 {
			angle := float64((phase + float32(i)/float32(steps)/4) * 2 * math.Pi)
			x, y := cx+radius*float32(math.Cos(angle)), cy+radius*float32(math.Sin(angle))
			if i == 0 {
				arc.MoveTo(x, y)
			} else {
				arc.LineTo(x, y)
			}
		}
		p.StrokePath(&arc, t.SpinnerStroke, alpha(Color(t.Spinner), spinner))
		if state == ConnConnecting && !reduceMotion {
			p.AnimationFrame()
		}
	})
}
