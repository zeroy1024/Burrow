package terminal

import (
	"math"

	"github.com/egoist/mygo/sshdesk-terminal/internal/vt"
	"github.com/egoist/mygo/ui"
)

// ViewportBackdropOptions describes chrome outside the terminal View. Top and
// Bottom are its bar heights; Gap is the layout spacing between each bar and View.
// Radius matches the surrounding card's outer corners.
type ViewportBackdropOptions struct{ Top, Bottom, Gap, Radius float32 }

// ViewportBackdrop paints actual adjacent scrollback rows beneath overlay bars
// and through the spacing between those bars and the visible grid.
// Build after View, in a wrapper that includes those bars. It shares the frame's
// render cache without another terminal, viewport change, resize or input target.
// Zero bar heights disable overscan; the View's geometry remains unchanged.
func ViewportBackdrop(c *ui.Context, t *Terminal, options ViewportBackdropOptions) ui.Element {
	v := t.viewOf(c)
	v.backdrop = options
	return ui.Box(c).PassThrough().Role(ui.RoleNone).Draw(func(p *ui.Painter, frame ui.Rect) {
		if len(v.edgeRows) == 0 {
			return
		}
		// Compute the grid origin in this subtree's paint space. A color
		// effect paints it in a separate canvas from the terminal View.
		origin := ui.Point{X: frame.X, Y: frame.Y + options.Top + options.Gap}
		gridTop := (float32(math.Round(float64(origin.Y*v.scale))) + float32(v.oy)) / v.scale
		gridBottom := gridTop + float32(v.rows*v.cellH)/v.scale
		for _, bottom := range []bool{false, true} {
			height := options.Top
			band := ui.Rect{X: frame.X, Y: frame.Y, W: frame.W, H: gridTop - frame.Y}
			if bottom {
				height = options.Bottom
				// Start at the last complete cell, including the View's bottom
				// slack as well as the gap before the status bar.
				band.Y, band.H = gridBottom, frame.Y+frame.H-gridBottom
			}
			if height <= 0 {
				continue
			}
			p.Clip(band, 0, func() {
				// MyGo's stationary GPU path keeps only the innermost clip's
				// rounded shape; the outer rectangle limits it to this band.
				p.Clip(frame, options.Radius, func() {
					v.paintBackdrop(p, band, origin)
				})
			})
		}
	})
}

type viewportBackdropRow struct {
	y    int
	line rowCache
}

func (v *view) backdropRequest(atBottom bool) vt.Overscan {
	rows := func(height float32) uint16 {
		if height <= 0 {
			return 0
		}
		// One additional row covers the grid's fractional bottom slack.
		count := math.Ceil(float64((height+v.backdrop.Gap)*v.scale)/float64(v.cellH)) + 1
		return uint16(min(count, 65535))
	}
	request := vt.Overscan{Above: rows(v.backdrop.Top), Below: rows(v.backdrop.Bottom)}
	// Clear-screen sequences can preserve history; show it above the grid
	// only while the user is scrolled back, not above the live screen.
	if atBottom {
		request.Above = 0
	}
	return request
}

func (v *view) paintBackdrop(p *ui.Painter, band ui.Rect, origin ui.Point) {
	s := v.scale
	cw, ch := float32(v.cellW)/s, float32(v.cellH)/s
	ox := (float32(math.Round(float64(origin.X*s))) + float32(v.ox)) / s
	oy := (float32(math.Round(float64(origin.Y*s))) + float32(v.oy)) / s
	base := float32(v.baseline) / s
	p.Fill(band, color(v.colors.Background), 0)
	v.drawImages(
		p,
		belowBackground,
		ox,
		oy,
	)
	for i := range v.edgeRows {
		row := &v.edgeRows[i]
		line, y := &row.line, oy+float32(row.y)*ch
		for _, bg := range line.bgs {
			rect := ui.Rect{X: ox + float32(bg.x0)*cw, Y: y, W: float32(bg.x1-bg.x0) * cw, H: ch}
			p.Fill(rect, bg.c, 0)
		}
	}
	v.drawImages(
		p,
		belowText,
		ox,
		oy,
	)
	for i := range v.edgeRows {
		row := &v.edgeRows[i]
		v.drawText(
			p,
			&row.line,
			ox,
			oy+float32(row.y)*ch,
			cw,
			ch,
			base,
			ui.Color{},
		)
	}
	v.drawImages(
		p,
		aboveText,
		ox,
		oy,
	)
}
