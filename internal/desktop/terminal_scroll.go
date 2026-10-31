package desktop

import (
	"time"

	"sshdesk/internal/theme"

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
	s.visibleUntil = ev.Time.Add(theme.Current().C.Scrollbar.HoldDuration)
	// At a boundary the native scroll does not request a frame, but the indicator still needs one.
	atTop := s.offset.Y <= 0 && ev.DY < 0
	atBottom := s.offset.Y >= s.offset.MaxY && ev.DY > 0
	return atTop || atBottom
}

func terminalScrollbar(c *ui.Context, pane ui.Element, state *terminalScrollState) {
	t := theme.Current()
	scrollbar, header, footer := t.C.Scrollbar, t.C.Pane.HeaderHeight, t.C.StatusBar.HeightPane
	now := c.Now()
	if state.offset.Y != state.lastY {
		state.lastY = state.offset.Y
		state.visibleUntil = now.Add(scrollbar.HoldDuration)
	}
	bounds := pane.Bounds()
	viewHeight := bounds.H
	trackHeight := viewHeight - header - footer - 2*scrollbar.EndInset
	if state.offset.MaxY <= 0 || trackHeight <= scrollbar.MinThumb {
		// Initialize the fade before the first layout has measured the scroll range.
		animateVisual(pane, "scrollbar", 0, scrollbar.FadeDuration)
		return
	}
	thumbHeight := max(scrollbar.MinThumb, trackHeight*viewHeight/(viewHeight+state.offset.MaxY))
	travel := trackHeight - thumbHeight
	track := ui.Box(c).Key("scrollbar-track").Absolute().Top(header + scrollbar.EndInset).
		Bottom(footer + scrollbar.EndInset).Right(0).Width(scrollbar.TrackWidth).Label("终端滚动条")
	var dragging bool
	track.Children(func() {
		thumb := ui.Box(c).Key("scrollbar-thumb").Absolute().Right(scrollbar.ThumbInset).Width(scrollbar.ThumbWidth).
			Top(travel * state.offset.Y / state.offset.MaxY).Height(thumbHeight).
			Radius(t.Radius.Pill).Background(tokenColor(scrollbar.Color))
		if _, dy, held := thumb.Dragged(); held {
			dragging = true
			state.visibleUntil = now.Add(scrollbar.HoldDuration)
			if dy != 0 {
				state.offset.Y = min(max(state.offset.Y+dy*state.offset.MaxY/travel, 0), state.offset.MaxY)
			}
		}
	})
	if track.Clicked() {
		_, y, _ := track.PointerPosition()
		state.offset.Y = min(max((y-thumbHeight/2)*state.offset.MaxY/travel, 0), state.offset.MaxY)
		state.visibleUntil = now.Add(scrollbar.HoldDuration)
	}
	x, y, over := pane.PointerPosition()
	nearRight := over && x >= bounds.W-scrollbar.RevealZone
	inScrollRows := y >= header && y <= bounds.H-footer
	nearEdge := nearRight && inScrollRows
	target := float32(0)
	if nearEdge || dragging || now.Before(state.visibleUntil) {
		target = 1
	}
	track.Opacity(animateVisual(pane, "scrollbar", target, scrollbar.FadeDuration))
	if now.Before(state.visibleUntil) {
		c.After(state.visibleUntil.Sub(now))
	}
}
