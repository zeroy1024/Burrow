package uikit

import (
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

// AgentRing outlines a pane that an agent operates and no person can type
// into: a gradient around its border, which turns while motion is true.
func AgentRing(c *ui.Context, radius float32, motion bool) ui.Element {
	t := theme.Current().C.AgentPane
	return ui.Box(c).Key("agent-ring").Absolute().Top(0).Right(0).Bottom(0).Left(0).PassThrough().
		Draw(func(p *ui.Painter, r ui.Rect) {
			inset := t.RingWidth / 2
			path := roundedRectPath(ui.Rect{X: r.X + inset, Y: r.Y + inset, W: r.W - 2*inset, H: r.H - 2*inset}, max(0, radius-inset))
			angle := float32(0)
			if motion {
				angle = 360 * float32(p.Now().UnixNano()%int64(t.RingPeriod)) / float32(t.RingPeriod)
				// The turn is slow; half the display's rate keeps it smooth for less power.
				p.AnimationFrameRate(30)
			}
			start, middle, end := Color(t.RingStart), Color(t.RingMiddle), Color(t.RingEnd)
			p.StrokePathConicGradient(path, t.RingWidth, ui.ConicGradient{Angle: angle, Oklab: true, Stops: []ui.GradientStop{
				{Color: start, At: 0}, {Color: middle, At: 1. / 3}, {Color: end, At: 2. / 3}, {Color: start, At: 1},
			}})
		})
}

// AgentChip labels an agent's pane at the left of its header band, which is
// band high, and names the login it works on after the label.
func AgentChip(c *ui.Context, text, login string, band float32, surface *theme.Theme) ui.Element {
	if surface == nil {
		surface = theme.Current()
	}
	p, a := surface.C.Badge, surface.C.AgentPane
	ink := Color(a.RingStart)
	return ui.Row(c).Key("agent-chip").Absolute().Top(0).Left(a.ChipInset).Height(band).Gap(a.ChipGap).AlignItems(ui.Center).
		Role(ui.RoleStatus).Label(text + " " + login).PassThrough().Children(func() {
		ui.Row(c).Height(p.HeightMd).Padding(0, p.PaddingX).Gap(a.ChipGap).AlignItems(ui.Center).Radius(p.Radius).
			Background(ink.Alpha(.16)).PassThrough().Children(func() {
			size := surface.C.IconButton.IconSizeXs
			ui.Icon(c, IconSVG(IconBot)).Size(size, size).TextColor(ink).PassThrough()
			ui.Text(c, text).FontSize(p.FontSize).SingleLine().TextColor(Color(surface.Color.Text.Primary)).PassThrough()
		})
		ui.Text(c, login).FontSize(p.FontSize).SingleLine().TextColor(Color(surface.C.StatusBar.Text)).PassThrough()
	})
}

// roundedRectPath approximates each quarter-circle corner with one cubic.
func roundedRectPath(r ui.Rect, radius float32) *ui.Path {
	radius = min(radius, r.W/2, r.H/2)
	const kappa = 0.5522848
	k := radius * (1 - kappa)
	left, top, right, bottom := r.X, r.Y, r.X+r.W, r.Y+r.H
	var path ui.Path
	path.MoveTo(left+radius, top).LineTo(right-radius, top).
		CubeTo(right-k, top, right, top+k, right, top+radius).LineTo(right, bottom-radius).
		CubeTo(right, bottom-k, right-k, bottom, right-radius, bottom).LineTo(left+radius, bottom).
		CubeTo(left+k, bottom, left, bottom-k, left, bottom-radius).LineTo(left, top+radius).
		CubeTo(left, top+k, left+k, top, left+radius, top).Close()
	return &path
}
