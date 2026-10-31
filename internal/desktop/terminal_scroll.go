package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type terminalScrollState struct {
	offset       ui.ScrollState
	lastY        float32
	visibleUntil time.Time
}

func (s *terminalScrollState) input(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll || ev.DY == 0 {
		return false
	}
	s.visibleUntil = ev.Time.Add(700 * time.Millisecond)
	// At a boundary the native scroll does not request a frame, but the indicator still needs one.
	atTop := s.offset.Y <= 0 && ev.DY < 0
	atBottom := s.offset.Y >= s.offset.MaxY && ev.DY > 0
	return atTop || atBottom
}

func terminalScrollbar(c *ui.Context, pane ui.Element, state *terminalScrollState) {
	now := c.Now()
	if state.offset.Y != state.lastY {
		state.lastY = state.offset.Y
		state.visibleUntil = now.Add(700 * time.Millisecond)
	}
	bounds := pane.Bounds()
	viewHeight := bounds.H
	trackHeight := viewHeight - terminalHeaderHeight - terminalFooterHeight - 4
	if state.offset.MaxY <= 0 || trackHeight <= 24 {
		// Initialize the fade before the first layout has measured the scroll range.
		pane.Animate("scrollbar", 0, 180*time.Millisecond)
		return
	}
	thumbHeight := max(24, trackHeight*viewHeight/(viewHeight+state.offset.MaxY))
	travel := trackHeight - thumbHeight
	track := ui.Box(c).Key("scrollbar-track").Absolute().Top(terminalHeaderHeight + 2).
		Bottom(terminalFooterHeight + 2).Right(0).Width(12).Label("终端滚动条")
	var dragging bool
	track.Children(func() {
		thumb := ui.Box(c).Key("scrollbar-thumb").Absolute().Right(3).Width(6).
			Top(travel * state.offset.Y / state.offset.MaxY).Height(thumbHeight).
			Radius(99).Background(c.Theme().Scrollbar)
		if _, dy, held := thumb.Dragged(); held {
			dragging = true
			state.visibleUntil = now.Add(700 * time.Millisecond)
			if dy != 0 {
				state.offset.Y = min(max(state.offset.Y+dy*state.offset.MaxY/travel, 0), state.offset.MaxY)
			}
		}
	})
	if track.Clicked() {
		_, y, _ := track.PointerPosition()
		state.offset.Y = min(max((y-thumbHeight/2)*state.offset.MaxY/travel, 0), state.offset.MaxY)
		state.visibleUntil = now.Add(700 * time.Millisecond)
	}
	x, y, over := pane.PointerPosition()
	nearRight := over && x >= bounds.W-24
	inScrollRows := y >= terminalHeaderHeight && y <= bounds.H-terminalFooterHeight
	nearEdge := nearRight && inScrollRows
	target := float32(0)
	if nearEdge || dragging || now.Before(state.visibleUntil) {
		target = 1
	}
	track.Opacity(pane.Animate("scrollbar", target, 180*time.Millisecond))
	if now.Before(state.visibleUntil) {
		c.After(state.visibleUntil.Sub(now))
	}
}
