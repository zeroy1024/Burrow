package uikit

import (
	"sshdesk/internal/theme"

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
	// A FormRow label can explicitly focus the compound host. On the rebuild
	// following that request, hand focus to the selected native radio.
	forwardFocus := parts.Track.Focused() && !parts.Track.IsDisabled()
	parts.Track.Children(func() {
		for index, item := range items {
			c.Key(item.Label)
			segment := parts.Segment(index).Radius(p.ItemRadius).Label(item.Label).
				Padding(0, p.ItemPaddingX).Gap(t.Space.N6)
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
			StyleSegment(segment, p, index == *selected)
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
					Count(c, *item.Count, CountProps{Style: CountQuiet, Max: props.CountMax}).
						FontSize(p.FontSize).TextColor(Color(ink))
				}
			})
		}
	})
	return parts.Track
}
