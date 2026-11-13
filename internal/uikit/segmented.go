package uikit

import (
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SegItem struct {
	Label    string
	Icon     IconName
	IconSVG  *ui.SVG
	IconTone Tone
	Count    *int
	Detail   string
}

type SegmentedProps struct {
	Size     Size
	Fill     bool
	Label    string
	Cards    bool
	Motion   MotionMode
	CountMax int
}

func SegmentItems(labels []string) []SegItem {
	items := make([]SegItem, len(labels))
	for index, label := range labels {
		items[index] = SegItem{Label: label}
	}
	return items
}

type segmentedPillState struct {
	seeded       bool
	lastSelected int
	lastTrackW   float32
}

func Segmented(c *ui.Context, selected *int, items []SegItem, props SegmentedProps) ui.Element {
	t := theme.Current()
	p := t.C.Segmented
	parts := ui.SegmentedBase(c, selected, len(items))
	height := p.ItemHeightMd + 2*p.Padding
	if props.Size == SizeSm || props.Size == SizeXs {
		height = p.HeightSm
	}
	parts.Track.Padding(p.Padding).Radius(p.Radius).Background(Color(p.Bg)).Label(props.Label)
	if props.Fill {
		parts.Track.FillWidth()
	}
	if props.Cards {
		parts.Track.Padding(t.C.Modal.ProtocolPadding).Radius(t.C.Modal.ProtocolRadius)
	} else {
		parts.Track.Height(height).AlignSelf(ui.Start)
	}
	trackLayout := parts.Track.LayoutBounds()
	staticPill := trackLayout.W <= 0
	pillState := ui.Local(parts.Track, "uikit.segmented.pill", func() segmentedPillState {
		return segmentedPillState{lastSelected: *selected}
	})
	segments := make([]ui.Element, len(items))
	// A FormRow label can explicitly focus the compound host. On the rebuild
	// following that request, hand focus to the selected native radio.
	forwardFocus := parts.Track.Focused() && !parts.Track.IsDisabled()
	parts.Track.Children(func() {
		for index, item := range items {
			c.Key(item.Label)
			segment := parts.Segment(index).Radius(p.ItemRadius).Label(item.Label).
				Padding(0, p.ItemPaddingX).Gap(t.Space.N6)
			segments[index] = segment
			if forwardFocus && index == *selected {
				segment.Focus()
			}
			if props.Fill {
				segment.Grow(1).Basis(0).MinWidth(0)
			}
			if props.Cards {
				segment.Padding(t.Space.N8, p.ItemPaddingX).Gap(t.Space.N8)
			} else {
				segment.Height(height - 2*p.Padding)
			}
			StyleSegment(segment, p, index == *selected, staticPill)
			segment.Children(func() {
				iconSize := t.Size.IconSm
				if props.Cards {
					iconSize = t.C.Modal.ProtocolIcon
				}
				if item.IconSVG != nil {
					Icon(c, item.IconSVG, IconProps{Size: SizeSm, Tone: item.IconTone}).Size(iconSize, iconSize).PassThrough()
				} else if item.Icon != "" {
					Icon(c, item.Icon, IconProps{Size: SizeSm, Tone: item.IconTone}).Size(iconSize, iconSize).PassThrough()
				}
				if item.Detail != "" {
					ui.Column(c).Gap(t.Space.N2).PassThrough().Children(func() {
						ui.Text(c, item.Label).FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Semibold).PassThrough()
						ui.Text(c, item.Detail).FontSize(t.Font.Size.Footnote).TextColor(Color(t.Color.Text.Secondary)).PassThrough()
					})
				} else {
					ui.Text(c, item.Label).FontSize(p.FontSize).SingleLine().PassThrough()
				}
				if item.Count != nil {
					ink := p.CountText
					if index == *selected {
						ink = p.CountTextOn
					}
					count := Count(c, *item.Count, CountProps{Style: CountQuiet, Max: props.CountMax}).FontSize(p.FontSize)
					count.TextColor(count.AnimateColor("fg", Color(ink), Tween(p.Transition)))
				}
			})
		}
	})
	if !staticPill && *selected >= 0 && *selected < len(segments) {
		targetRect := segments[*selected].LayoutBounds()
		if targetRect.W > 0 && targetRect.H > 0 {
			if !pillState.seeded {
				fromRect := targetRect
				if prev := pillState.lastSelected; prev >= 0 && prev < len(segments) {
					if candidate := segments[prev].LayoutBounds(); candidate.W > 0 && candidate.H > 0 {
						fromRect = candidate
					}
				}
				parts.Track.Jump("pill-x", fromRect.X-trackLayout.X)
				parts.Track.Jump("pill-y", fromRect.Y-trackLayout.Y)
				parts.Track.Jump("pill-w", fromRect.W)
				parts.Track.Jump("pill-h", fromRect.H)
				pillState.seeded = true
			} else if pillState.lastTrackW != trackLayout.W {
				parts.Track.Jump("pill-x", targetRect.X-trackLayout.X)
				parts.Track.Jump("pill-y", targetRect.Y-trackLayout.Y)
				parts.Track.Jump("pill-w", targetRect.W)
				parts.Track.Jump("pill-h", targetRect.H)
			}
			pillAnim := ui.Animation{Spring: Spring(t.Motion.Spring.Smooth)}
			if MotionReduced(c, props.Motion) {
				pillAnim = Anim(c, p.Transition, props.Motion)
			}
			pillX := parts.Track.AnimateTo("pill-x", targetRect.X-trackLayout.X, pillAnim)
			pillY := parts.Track.AnimateTo("pill-y", targetRect.Y-trackLayout.Y, pillAnim)
			pillW := parts.Track.AnimateTo("pill-w", targetRect.W, pillAnim)
			pillH := parts.Track.AnimateTo("pill-h", targetRect.H, pillAnim)
			parts.Track.Draw(func(painter *ui.Painter, r ui.Rect) {
				if pillW <= 0 || pillH <= 0 {
					return
				}
				pill := ui.Rect{X: r.X + pillX, Y: r.Y + pillY, W: pillW, H: pillH}
				for _, layer := range p.ShadowOn {
					if !layer.Inset {
						painter.Shadow(pill, p.ItemRadius, layer.X, layer.Y, layer.Blur, layer.Spread, Color(layer.Color))
					}
				}
				painter.Fill(pill, Color(p.BgOn), p.ItemRadius)
			})
		}
	}
	pillState.lastSelected = *selected
	pillState.lastTrackW = trackLayout.W
	return parts.Track
}
