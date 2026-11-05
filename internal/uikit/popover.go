package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type Placement uint8

const (
	PlacementBottom Placement = iota
	PlacementTop
	PlacementLeft
	PlacementRight
)

type PopoverProps struct {
	Width     float32
	Placement Placement
	Label     string
	Motion    MotionMode
}

// Popover leaves opening with its caller and uses the SDK's outside-press,
// Escape, edge avoidance and focus-return lifecycle.
func Popover(c *ui.Context, anchor ui.Element, open *bool, props PopoverProps, body func()) ui.Element {
	t := theme.Current()
	width, height := c.Size()
	return ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		StylePopover(panel).Column().Padding(t.C.Popover.Padding).Label(props.Label).
			MaxWidth(width - 2*t.Space.N12).MaxHeight(height - 2*t.Space.N12)
		if props.Width > 0 {
			panel.Width(props.Width)
		}
		switch props.Placement {
		case PlacementTop:
			panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, t.Space.N4, 0)
		case PlacementLeft:
			panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorTopEnd).MarginEnd(t.Space.N4)
		case PlacementRight:
			panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorTopStart).MarginStart(t.Space.N4)
		default:
			panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(t.Space.N4, 0, 0, 0)
		}
		enter := ui.Motion{}
		if !MotionReduced(c, props.Motion) {
			enter.Scale = t.Opacity.PopoverEnterScale
		}
		panel.Transition(ui.ElementTransition{Duration: t.Motion.Duration.Exit, Enter: &enter, Exit: &ui.Motion{}})
		panel.Children(body)
	})
}

func StylePopover(panel ui.Element) ui.Element {
	t := theme.Current()
	return Shadow(panel.Radius(t.C.Popover.Radius).Background(Color(t.C.Popover.Bg)), t.C.Popover.Shadow)
}
