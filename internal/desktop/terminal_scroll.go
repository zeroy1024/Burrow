package desktop

import (
	"image/color"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type terminalScrollState struct {
	offset ui.ScrollState
	scrollbarState
}

type scrollbarState struct {
	lastY        float32
	visibleUntil time.Time
}

func (s *terminalScrollState) input(ev ui.InputEvent) bool {
	return s.scrollbarState.input(ev, &s.offset)
}

func (s *scrollbarState) input(ev ui.InputEvent, offset *ui.ScrollState) bool {
	if ev.Kind != ui.InputScroll || ev.DY == 0 {
		return false
	}
	s.visibleUntil = ev.Time.Add(theme.Current().C.Scrollbar.HoldDuration)
	// At a boundary the native scroll does not request a frame, but the indicator still needs one.
	atTop := offset.Y <= 0 && ev.DY < 0
	atBottom := offset.Y >= offset.MaxY && ev.DY > 0
	return atTop || atBottom
}

func terminalScrollbar(c *ui.Context, pane ui.Element, state *terminalScrollState, background color.NRGBA) {
	t := theme.Current()
	fadingScrollbar(c, scrollbarView{
		element: pane, offset: &state.offset, label: "终端滚动条",
		top: t.C.Pane.HeaderHeight, bottom: t.C.StatusBar.HeightPane,
		color: terminalScrollbarColor(background),
	}, &state.scrollbarState)
}

type scrollbarView struct {
	element     ui.Element
	offset      *ui.ScrollState
	top, bottom float32
	label       string
	color       ui.Color
}

func fadingScrollbar(c *ui.Context, view scrollbarView, state *scrollbarState) {
	t := theme.Current()
	scrollbar := t.C.Scrollbar
	pane, offset := view.element, view.offset
	now := c.Now()
	if offset.Y != state.lastY {
		state.lastY = offset.Y
		state.visibleUntil = now.Add(scrollbar.HoldDuration)
	}
	bounds := pane.Bounds()
	viewHeight := bounds.H
	trackHeight := viewHeight - view.top - view.bottom - 2*scrollbar.EndInset
	if offset.MaxY <= 0 || trackHeight <= scrollbar.MinThumb {
		uikit.
			// Initialize the fade before the first layout has measured the scroll range.
			Visual(pane, "scrollbar", 0, scrollbar.FadeDuration)
		return
	}
	thumbHeight := max(scrollbar.MinThumb, trackHeight*viewHeight/(viewHeight+offset.MaxY))
	travel := trackHeight - thumbHeight
	track := ui.Box(c).Key("scrollbar-track").Absolute().Top(view.top + scrollbar.EndInset).
		Bottom(view.bottom + scrollbar.EndInset).Right(0).Width(scrollbar.TrackWidth).Label(view.label).
		HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputScroll || ev.DY == 0 {
				return false
			}
			// The overlay is a sibling of the native scroll, outside its input chain.
			offset.Y = min(max(offset.Y+ev.DY, 0), offset.MaxY)
			state.visibleUntil = ev.Time.Add(scrollbar.HoldDuration)
			return true
		})
	var dragging bool
	track.Children(func() {
		thumb := ui.Box(c).Key("scrollbar-thumb").Absolute().Right(scrollbar.ThumbInset).Width(scrollbar.ThumbWidth).
			Top(travel * offset.Y / offset.MaxY).Height(thumbHeight).
			Radius(t.Radius.Pill).Background(view.color)
		if _, dy, held := thumb.Dragged(); held {
			dragging = true
			state.visibleUntil = now.Add(scrollbar.HoldDuration)
			if dy != 0 {
				offset.Y = min(max(offset.Y+dy*offset.MaxY/travel, 0), offset.MaxY)
			}
		}
	})
	if track.Clicked() {
		_, y, _ := track.PointerPosition()
		offset.Y = min(max((y-thumbHeight/2)*offset.MaxY/travel, 0), offset.MaxY)
		state.visibleUntil = now.Add(scrollbar.HoldDuration)
	}
	x, y, over := pane.PointerPosition()
	nearRight := over && x >= bounds.W-scrollbar.RevealZone
	inScrollRows := y >= view.top && y <= bounds.H-view.bottom
	nearEdge := nearRight && inScrollRows
	target := float32(0)
	if nearEdge || dragging || now.Before(state.visibleUntil) {
		target = 1
	}
	track.Opacity(uikit.Visual(pane, "scrollbar", target, scrollbar.FadeDuration))
	if now.Before(state.visibleUntil) {
		c.After(state.visibleUntil.Sub(now))
	}
}
