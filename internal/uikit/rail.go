package uikit

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
)

func Rail(c *ui.Context) ui.Element {
	t := theme.Current()
	r := t.C.Rail
	return ui.Column(c).Width(r.Width).Shrink(0).
		Padding(r.PaddingTop, t.Space.N6, r.PaddingBottom, t.Space.N6).
		Gap(r.Gap).AlignItems(ui.Center).Label(locales.MessageMainNavigation()).Role(ui.RoleTabList).FocusGroup(ui.Vertical)
}

type RailButtonProps struct {
	Key      any
	Label    string
	Icon     *ui.SVG
	Selected bool
	Handle   *ui.Handle
	Motion   MotionMode
}

// RailButton returns an activation target; the application owns page switching.
func RailButton(c *ui.Context, props RailButtonProps) ui.Element {
	t := theme.Current()
	r := t.C.Rail
	button := ui.ButtonBase(c).Key(props.Key).Size(r.ButtonSize, r.ButtonSize).Radius(r.ButtonRadius).
		Label(props.Label).Description(props.Label).Role(ui.RoleTab).Selected(props.Selected).TextColor(Color(r.Icon)).FocusRing(false)
	if props.Handle != nil {
		button.Bind(props.Handle)
	}
	foreground := Color(r.Icon)
	switch {
	case props.Selected:
		foreground = Color(r.IconOn)
	case button.Hovered() && !button.Pressed():
		foreground = Color(t.Color.Text.Primary)
	}
	if runtime.GOOS == "windows" && props.Selected {
		foreground = Color(t.Color.Accent.Default)
	}
	button.TextColor(button.AnimateColor("icon", foreground, Tween(r.Transition)))
	if button.FocusVisible() {
		Shadow(button, t.Shadow.ControlFocus)
	}
	button.Children(func() {
		icon := ui.Icon(c, props.Icon).Size(t.Size.IconMd, t.Size.IconMd).PassThrough()
		if !MotionReduced(c, props.Motion) {
			scale := float32(1)
			if button.Pressed() {
				scale = 0.9
			}
			icon.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: Spring(t.Motion.Spring.Displace)})
		}
	})
	ui.TooltipBase(c, button, func(tip ui.Element) {
		StyleTooltip(tip).AttachTo(button, ui.AnchorRight, ui.AnchorLeft).
			MarginStart(t.C.Tooltip.RailOffset).Children(func() { ui.Text(c, props.Label).PassThrough() })
	})
	return button
}

func AnimateRailPill(c *ui.Context, rail ui.Element, items []ui.Element, selected int, reduceMotion bool) {
	t := theme.Current()
	r := t.C.Rail
	bg := Color(r.BgOn)
	if runtime.GOOS == "windows" {
		bg = Color(t.Color.Accent.Tint10)
	}
	paintSlidingSelection(
		rail,
		items,
		selected,
		slidingSelectionStyle{
			background: bg,
			hover:      Color(t.C.Desktop.RailHover),
			pressed:    Color(t.C.Desktop.RailPressed),
			radius:     r.ButtonRadius,
			feedback:   Tween(r.Transition),
			reduced:    reduceMotion || c.Preferences().ReduceMotion,
		},
	)
}
