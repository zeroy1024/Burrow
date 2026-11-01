package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SegmentedProps struct {
	Form bool
}

func Segmented(c *ui.Context, selected *int, labels []string, props SegmentedProps) ui.Element {
	t := theme.Current()
	p := t.C.Segmented
	parts := ui.SegmentedBase(c, selected, len(labels))
	parts.Track.Padding(p.Padding).Radius(p.Radius).Background(Color(p.Bg))
	if props.Form {
		parts.Track.Height(t.C.Input.Height).AlignSelf(ui.Start)
	}
	parts.Track.Children(func() {
		for i, label := range labels {
			segment := parts.Segment(i).Radius(p.ItemRadius).Label(label)
			fontSize := p.FontSize
			if props.Form {
				segment.Height(t.C.Input.Height-2*p.Padding).Padding(0, t.C.Modal.ButtonPaddingX)
				fontSize = t.Font.Size.Small
			} else {
				segment.Padding(p.ItemPaddingY, p.ItemPaddingX)
			}
			StyleSegment(segment, p, i == *selected)
			segment.Children(func() { ui.Text(c, label).FontSize(fontSize).SingleLine().PassThrough() })
		}
	})
	return parts.Track
}
