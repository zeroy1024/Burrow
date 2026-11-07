package uikit

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
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
	background, foreground := ui.Transparent, Color(r.Icon)
	switch {
	case button.Pressed():
		background = Color(t.C.Desktop.RailPressed)
	case props.Selected:
		background = Color(r.BgOn)
	case button.Hovered():
		background = Color(t.C.Desktop.RailHover)
	}
	if runtime.GOOS == "windows" && props.Selected {
		background, foreground = Color(t.Color.Accent.Tint10), Color(t.Color.Accent.Default)
		if button.Pressed() {
			background = Color(t.Color.Accent.Tint15)
		}
	}
	button.Background(button.AnimateColor("background", background, Tween(r.Transition)))
	if runtime.GOOS == "windows" {
		button.TextColor(button.AnimateColor("icon", foreground, Tween(r.Transition)))
	}
	if button.FocusVisible() {
		Shadow(button, t.Shadow.ControlFocus)
	}
	button.Children(func() { ui.Icon(c, props.Icon).Size(t.Size.IconMd, t.Size.IconMd).PassThrough() })
	ui.TooltipBase(c, button, func(tip ui.Element) {
		StyleTooltip(tip).AttachTo(button, ui.AnchorRight, ui.AnchorLeft).
			MarginStart(t.C.Tooltip.RailOffset).Children(func() { ui.Text(c, props.Label).PassThrough() })
	})
	return button
}
