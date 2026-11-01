package uikit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type scrollbarAxis struct {
	last         float32
	visibleUntil time.Time
}

// ScrollbarState contains presentation timing only. The caller keeps the
// native ScrollState, including offsets, handles, glides and list anchors.
type ScrollbarState struct {
	x, y scrollbarAxis
}

type ScrollbarProps struct {
	Label                string
	Horizontal, Vertical bool
	Top, Bottom          float32
	Left, Right          float32
	Color                ui.Color
	State                *ScrollbarState
	Input                func(ui.InputEvent) bool
}

// Scrollbars decorates an existing native scroll/list without adding a parent
// around its content. host is its non-scrolling frame, built in the same pass.
// The caller binds scroller to offset with TrackScroll before building content.
func Scrollbars(c *ui.Context, host, scroller ui.Element, offset *ui.ScrollState, props ScrollbarProps) {
	t := theme.Current()
	p := t.C.Scrollbar
	if !props.Horizontal && !props.Vertical {
		props.Vertical = true
	}
	if props.Color.A == 0 {
		props.Color = Color(p.Color)
	}
	state := props.State
	if state == nil {
		state = ui.Local(host, "uikit.scrollbars", func() ScrollbarState { return ScrollbarState{} })
	}
	// SDK thumbs paint after the scroll node's clip is restored. Move them
	// outside the window, including padded popovers, while retaining native
	// wheel, keyboard, virtual-list and programmatic scrolling.
	windowWidth, windowHeight := c.Size()
	scroller.ScrollbarInsets(0, -windowWidth, -windowHeight, -windowWidth)
	scroller.HandleInput(func(event ui.InputEvent) bool {
		if event.Kind == ui.InputScroll {
			until := event.Time.Add(p.HoldDuration)
			if props.Horizontal && offset.MaxX > 0 && (event.DX != 0 || !props.Vertical && event.DY != 0) {
				state.x.visibleUntil = until
				c.Invalidate()
			}
			if props.Vertical && offset.MaxY > 0 && event.DY != 0 {
				state.y.visibleUntil = until
				c.Invalidate()
			}
		}
		if props.Input != nil {
			return props.Input(event)
		}
		return false
	})
	frame, bounds := host.LayoutBounds(), scroller.LayoutBounds()
	maxX, maxY := offset.MaxX, offset.MaxY
	positionX, positionY := offset.X, offset.Y
	host.Children(func() {
		// This observer does not replace the host's own DrawOver (table sizing,
		// drag hit boxes, fades). Refresh once after layout measures a new range.
		ui.Box(c).Key("scrollbar-layout").Absolute().Fill().PassThrough().Role(ui.RoleNone).
			DrawOver(func(_ *ui.Painter, _ ui.Rect) {
				if scroller.LayoutBounds() != bounds || offset.MaxX != maxX || offset.MaxY != maxY ||
					offset.X != positionX || offset.Y != positionY {
					c.Invalidate()
				}
			})
		area := ui.Box(c).Key("scrollbar-frame").Absolute().Left(bounds.X-frame.X).Top(bounds.Y-frame.Y).
			Size(bounds.W, bounds.H).PassThrough().Role(ui.RoleNone)
		area.Children(func() {
			if props.Vertical {
				state.drawAxis(c, area, offset, &state.y, props, false, bounds.W, bounds.H)
			}
			if props.Horizontal {
				state.drawAxis(c, area, offset, &state.x, props, true, bounds.W, bounds.H)
			}
		})
	})
}

func (state *ScrollbarState) drawAxis(
	c *ui.Context, area ui.Element, offset *ui.ScrollState, axis *scrollbarAxis,
	props ScrollbarProps, horizontal bool, width, height float32,
) {
	t := theme.Current()
	p, now := t.C.Scrollbar, c.Now()
	position, limit, viewport := offset.Y, offset.MaxY, height
	start, end, cross := props.Top, props.Bottom, props.Right
	key := "vertical"
	if horizontal {
		position, limit, viewport = offset.X, offset.MaxX, width
		start, end, cross, key = props.Left, props.Right, props.Bottom, "horizontal"
		if props.Vertical && offset.MaxY > 0 {
			end += p.TrackWidth
		}
	} else if props.Horizontal && offset.MaxX > 0 {
		end += p.TrackWidth
	}
	if axis.last != position {
		axis.last, axis.visibleUntil = position, now.Add(p.HoldDuration)
	}
	length := viewport - start - end - 2*p.EndInset
	if limit <= 0 || length <= p.MinThumb {
		Visual(area, key, 0, p.FadeDuration)
		return
	}
	thumbSize := min(length, max(p.MinThumb, length*viewport/(viewport+limit)))
	travel := length - thumbSize
	if travel <= 0 {
		return
	}
	set := func(value float32) {
		value = min(max(value, 0), limit)
		if horizontal {
			offset.X = value
		} else {
			offset.Y = value
		}
		axis.visibleUntil = now.Add(p.HoldDuration)
		c.Invalidate()
	}
	track := ui.Box(c.Key(key)).Absolute().KeepFocus().Label(props.Label).HandleInput(func(event ui.InputEvent) bool {
		if event.Kind != ui.InputScroll {
			return false
		}
		delta := event.DY
		if horizontal && event.DX != 0 {
			delta = event.DX
		}
		if delta == 0 {
			return false
		}
		set(position + delta)
		return true
	})
	if horizontal {
		track.Left(start + p.EndInset).Right(end + p.EndInset).Bottom(cross).Height(p.TrackWidth)
	} else {
		track.Top(start + p.EndInset).Bottom(end + p.EndInset).Right(cross).Width(p.TrackWidth)
	}
	dragging := false
	track.Children(func() {
		thumb := ui.Box(c).Key("thumb").Absolute().KeepFocus().Radius(t.Radius.Pill).Background(props.Color)
		if horizontal {
			thumb.Bottom(p.ThumbInset).Height(p.ThumbWidth).Left(travel * position / limit).Width(thumbSize)
		} else {
			thumb.Right(p.ThumbInset).Width(p.ThumbWidth).Top(travel * position / limit).Height(thumbSize)
		}
		if dx, dy, held := thumb.Dragged(); held {
			dragging = true
			delta := dy
			if horizontal {
				delta = dx
			}
			if delta != 0 {
				set(position + delta*limit/travel)
			}
			axis.visibleUntil = now.Add(p.HoldDuration)
		}
	})
	if track.Clicked() {
		x, y, _ := track.PointerPosition()
		at := y
		if horizontal {
			at = x
		}
		set((at - thumbSize/2) * limit / travel)
	}
	x, y, over := area.PointerPosition()
	near := over && x >= width-cross-p.RevealZone && y >= start && y <= height-end
	if horizontal {
		near = over && y >= height-cross-min(p.RevealZone, p.TrackWidth) && x >= start && x <= width-end
	}
	target := float32(0)
	if near || dragging || now.Before(axis.visibleUntil) {
		target = 1
	}
	track.Opacity(Visual(area, key, target, p.FadeDuration))
	if now.Before(axis.visibleUntil) {
		c.After(axis.visibleUntil.Sub(now))
	}
}
