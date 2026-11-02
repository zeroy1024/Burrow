package terminal

import (
	"math"

	"github.com/egoist/mygo/sshdesk-terminal/internal/vt"
	"github.com/egoist/mygo/ui"
)

// ViewportBackdropOptions describes chrome outside the terminal View. Top and
// Bottom are its bar heights; Gap is the blank space between each bar and View.
// Radius matches the surrounding card's outer corners.
type ViewportBackdropOptions struct{ Top, Bottom, Gap, Radius float32 }

// ViewportBackdrop paints actual adjacent scrollback rows beneath overlay bars.
// Build after View, in a wrapper that includes those bars. It shares the frame's
// render cache without another terminal, viewport change, resize or input target.
// Zero bar heights disable overscan; the View's geometry remains unchanged.
func ViewportBackdrop(c *ui.Context, t *Terminal, options ViewportBackdropOptions) ui.Element {
	v := t.viewOf(c)
	v.backdrop = options
	var origin ui.Point
	root := ui.Box(c).PassThrough().Role(ui.RoleNone).Draw(func(_ *ui.Painter, r ui.Rect) {
		// Compute the grid origin in this subtree's paint space. A color
		// effect paints it in a separate canvas from the terminal View.
		origin = ui.Point{X: r.X, Y: r.Y + options.Top + options.Gap}
	})
	root.Children(func() {
		for _, bottom := range []bool{false, true} {
			key, height := "top", options.Top
			if bottom {
				key, height = "bottom", options.Bottom
			}
			if height <= 0 {
				continue
			}
			// The innermost clip must carry the card's outer corners: MyGo's
			// stationary GPU path intersects ancestor clips as rectangles.
			band := ui.Box(c.Key(key)).Absolute().Left(0).Right(0).Height(height).
				PassThrough().Role(ui.RoleNone).Clip()
			if bottom {
				band.Bottom(0).Radius(
					0,
					0,
					options.Radius,
					options.Radius,
				)
			} else {
				band.Top(0).Radius(
					options.Radius,
					options.Radius,
					0,
					0,
				)
			}
			band.Children(func() {
				ui.Box(c).Fill().PassThrough().Role(ui.RoleNone).Draw(func(p *ui.Painter, r ui.Rect) {
					v.paintBackdrop(p, r, origin)
				})
			})
		}
	})
	return root
}

type viewportBackdropRow struct {
	y    int
	line rowCache
}

func (v *view) backdropRequest() vt.Overscan {
	rows := func(height float32) uint16 {
		if height <= 0 {
			return 0
		}
		// One additional row covers the grid's fractional bottom slack.
		count := math.Ceil(float64((height+v.backdrop.Gap)*v.scale)/float64(v.cellH)) + 1
		return uint16(min(count, 65535))
	}
	return vt.Overscan{Above: rows(v.backdrop.Top), Below: rows(v.backdrop.Bottom)}
}

func (v *view) paintBackdrop(p *ui.Painter, band ui.Rect, origin ui.Point) {
	if len(v.edgeRows) == 0 {
		return
	}
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
