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
	Key        any
	Label      string
	Icon       *ui.SVG
	Selected   bool
	StaticPill bool
	Handle     *ui.Handle
	Motion     MotionMode
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
		if props.Selected {
			foreground = Color(r.IconOn)
		}
	case props.Selected:
		foreground = Color(r.IconOn)
		if props.StaticPill {
			background = Color(r.BgOn)
		}
	case button.Hovered():
		background, foreground = Color(t.C.Desktop.RailHover), Color(t.Color.Text.Primary)
	}
	if runtime.GOOS == "windows" && props.Selected {
		foreground = Color(t.Color.Accent.Default)
		if props.StaticPill {
			background = Color(t.Color.Accent.Tint10)
		}
		if button.Pressed() {
			background = Color(t.Color.Accent.Tint15)
		}
	}
	button.Background(button.AnimateColor("background", background, Tween(r.Transition))).
		TextColor(button.AnimateColor("icon", foreground, Tween(r.Transition)))
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

type navPillState struct {
	lastSelected           int
	lastTrackW, lastTrackH float32
	seeded                 bool
}

func AnimateRailPill(c *ui.Context, rail ui.Element, items []ui.Element, selected int, reduceMotion bool) {
	t := theme.Current()
	r := t.C.Rail
	bg := Color(r.BgOn)
	if runtime.GOOS == "windows" {
		bg = Color(t.Color.Accent.Tint10)
	}
	animateNavPill(c, rail, items, selected, bg, r.ButtonRadius, reduceMotion)
}

func animateNavPill(c *ui.Context, track ui.Element, items []ui.Element, selected int, bg ui.Color, radius float32, reduceMotion bool) {
	t := theme.Current()
	reduced := reduceMotion || c.Preferences().ReduceMotion
	pillState := ui.Local(track, "nav-pill-state", func() navPillState {
		return navPillState{lastSelected: -1}
	})
	trackLayout := track.LayoutBounds()
	staticPill := trackLayout.W <= 0 || trackLayout.H <= 0
	if !staticPill && selected >= 0 && selected < len(items) {
		targetRect := items[selected].LayoutBounds()
		if targetRect.W > 0 && targetRect.H > 0 {
			if reduced || !pillState.seeded {
				fromRect := targetRect
				if prev := pillState.lastSelected; !reduced && prev >= 0 && prev < len(items) {
					if candidate := items[prev].LayoutBounds(); candidate.W > 0 && candidate.H > 0 {
						fromRect = candidate
					}
				}
				track.Jump("pill-x", fromRect.X-trackLayout.X)
				track.Jump("pill-y", fromRect.Y-trackLayout.Y)
				track.Jump("pill-w", fromRect.W)
				track.Jump("pill-h", fromRect.H)
				pillState.seeded = true
			} else if pillState.lastTrackW != trackLayout.W || pillState.lastTrackH != trackLayout.H {
				track.Jump("pill-x", targetRect.X-trackLayout.X)
				track.Jump("pill-y", targetRect.Y-trackLayout.Y)
				track.Jump("pill-w", targetRect.W)
				track.Jump("pill-h", targetRect.H)
			}
			pillAnim := ui.Animation{Spring: Spring(t.Motion.Spring.Smooth)}
			pillX := track.AnimateTo("pill-x", targetRect.X-trackLayout.X, pillAnim)
			pillY := track.AnimateTo("pill-y", targetRect.Y-trackLayout.Y, pillAnim)
			pillW := track.AnimateTo("pill-w", targetRect.W, pillAnim)
			pillH := track.AnimateTo("pill-h", targetRect.H, pillAnim)
			track.Draw(func(painter *ui.Painter, r ui.Rect) {
				if pillW <= 0 || pillH <= 0 {
					return
				}
				pill := ui.Rect{X: r.X + pillX, Y: r.Y + pillY, W: pillW, H: pillH}
				painter.Fill(pill, bg, radius)
			})
		}
	}
	pillState.lastSelected = selected
	pillState.lastTrackW = trackLayout.W
	pillState.lastTrackH = trackLayout.H
}
