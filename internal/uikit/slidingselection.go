package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type slidingSelectionStyle struct {
	background, hover, pressed ui.Color
	radius                     float32
	shadow                     theme.Shadow
	feedback                   ui.Animation
	reduced                    bool
}

type slidingSelectionState struct {
	lastSelected            int
	trackWidth, trackHeight float32
	seeded                  bool
}

type selectionFeedback struct {
	bounds ui.Rect
	color  ui.Color
}

// paintSlidingSelection keeps all fills on the track, below the item content.
func paintSlidingSelection(track ui.Element, items []ui.Element, selected int, style slidingSelectionStyle) {
	layout := track.LayoutBounds()
	pill := animatedSelectionBounds(track, items, selected, layout, style.reduced)
	hasPill := pill.W > 0 && pill.H > 0
	feedback := []selectionFeedback{}
	for index, item := range items {
		target := ui.Transparent
		switch {
		case style.pressed.A != 0 && item.Pressed():
			target = style.pressed
		case index != selected && item.Hovered():
			target = style.hover
		}
		shown := item.AnimateColor("selection-feedback", target, style.feedback)
		// Selected also applies a native accent fill, so clear it while the
		// track owns the background. The accessibility state is retained.
		item.Background(ui.Transparent)
		if !hasPill {
			// The first frame can paint the selected item before layout is available.
			if index == selected {
				item.Background(style.background)
				Shadow(item, style.shadow)
			} else {
				item.Background(shown)
			}
			continue
		}
		if shown.A != 0 {
			bounds := item.LayoutBounds()
			bounds.X -= layout.X
			bounds.Y -= layout.Y
			feedback = append(feedback, selectionFeedback{bounds: bounds, color: shown})
		}
	}
	if !hasPill {
		return
	}
	track.Draw(func(p *ui.Painter, rect ui.Rect) {
		drawn := pill
		drawn.X += rect.X
		drawn.Y += rect.Y
		for _, fill := range feedback {
			bounds := fill.bounds
			bounds.X += rect.X
			bounds.Y += rect.Y
			if style.background.A < 255 && overlaps(bounds, drawn) {
				fillOutsideSelection(p, bounds, drawn, style.radius, fill.color)
			} else {
				p.Fill(bounds, fill.color, style.radius)
			}
		}
		for _, layer := range style.shadow {
			if !layer.Inset {
				p.Shadow(drawn, style.radius, layer.X, layer.Y, layer.Blur, layer.Spread, Color(layer.Color))
			}
		}
		p.Fill(drawn, style.background, style.radius)
	})
}

func animatedSelectionBounds(
	track ui.Element,
	items []ui.Element,
	selected int,
	layout ui.Rect,
	reduced bool,
) ui.Rect {
	state := ui.Local(track, "sliding-selection", func() slidingSelectionState {
		return slidingSelectionState{lastSelected: -1}
	})
	previous := state.lastSelected
	state.lastSelected = selected
	if layout.W <= 0 || layout.H <= 0 || selected < 0 || selected >= len(items) {
		state.seeded = false
		return ui.Rect{}
	}
	target := items[selected].LayoutBounds()
	if target.W <= 0 || target.H <= 0 {
		state.seeded = false
		return ui.Rect{}
	}
	resized := state.trackWidth != layout.W || state.trackHeight != layout.H
	if reduced || !state.seeded || resized {
		from := target
		if !reduced && !state.seeded && previous >= 0 && previous < len(items) {
			if bounds := items[previous].LayoutBounds(); bounds.W > 0 && bounds.H > 0 {
				from = bounds
			}
		}
		track.Jump("pill-x", from.X-layout.X)
		track.Jump("pill-y", from.Y-layout.Y)
		track.Jump("pill-w", from.W)
		track.Jump("pill-h", from.H)
		state.seeded = true
	}
	state.trackWidth, state.trackHeight = layout.W, layout.H
	animation := ui.Animation{Spring: Spring(theme.Current().Motion.Spring.Smooth)}
	return ui.Rect{
		X: track.AnimateTo("pill-x", target.X-layout.X, animation),
		Y: track.AnimateTo("pill-y", target.Y-layout.Y, animation),
		W: track.AnimateTo("pill-w", target.W, animation),
		H: track.AnimateTo("pill-h", target.H, animation),
	}
}

func overlaps(a, b ui.Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

func fillOutsideSelection(p *ui.Painter, bounds, pill ui.Rect, radius float32, ink ui.Color) {
	// Subtract the rounded selection with a reverse-wound path so a
	// translucent selection keeps its intended color over the window.
	var path ui.Path
	path.MoveTo(bounds.X, bounds.Y).
		LineTo(bounds.X+bounds.W, bounds.Y).
		LineTo(bounds.X+bounds.W, bounds.Y+bounds.H).
		LineTo(bounds.X, bounds.Y+bounds.H).
		Close()

	left, top := pill.X, pill.Y
	right, bottom := left+pill.W, top+pill.H
	r := min(radius, pill.W/2, pill.H/2)
	d := r * (1 - 0.55228475)
	path.MoveTo(left+r, top).
		CubeTo(left+d, top, left, top+d, left, top+r).
		LineTo(left, bottom-r).
		CubeTo(left, bottom-d, left+d, bottom, left+r, bottom).
		LineTo(right-r, bottom).
		CubeTo(right-d, bottom, right, bottom-d, right, bottom-r).
		LineTo(right, top+r).
		CubeTo(right, top+d, right-d, top, right-r, top).
		Close()
	p.Clip(bounds, radius, func() { p.FillPath(&path, ink) })
}
