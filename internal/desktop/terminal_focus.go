package desktop

import (
	"slices"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const (
	terminalPrimaryText   = "#1d1d1f"
	terminalSecondaryText = "#8e8e93"
)

var terminalSaturation = shader.MustCompile(`package main

var Saturation float

func ColorEffect(pos vec2, color vec4) vec4 {
	luminance := dot(color.rgb, vec3(0.213, 0.715, 0.072))
	return vec4(mix(vec3(luminance), color.rgb, Saturation), color.a)
}
`)

func (a *app) paneShortcuts(c *ui.Context) {
	tab := a.currentTab()
	if tab == nil || tab.root.count() == 1 {
		return
	}
	sessions := tab.root.sessions()
	keys := [...]ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4}
	for index, session := range sessions {
		if c.Shortcut(ui.Super, keys[index]) {
			a.focusSession(tab, session)
			return
		}
	}
	var direction int
	if c.Shortcut(ui.Super, ui.KeyBracketLeft) {
		direction = -1
	}
	if c.Shortcut(ui.Super, ui.KeyBracketRight) {
		direction = 1
	}
	if direction != 0 {
		index := (slices.Index(sessions, tab.focused) + direction + len(sessions)) % len(sessions)
		a.focusSession(tab, sessions[index])
	}
}

func paneFocusShadows(pane ui.Element, focused, multiple bool) {
	t := theme.Current().C.PaneFocus
	focusTarget, hoverTarget := float32(0), float32(0)
	if multiple && focused {
		focusTarget = 1
	}
	if multiple && !focused && pane.Hovered() {
		hoverTarget = 1
	}
	focus := animateVisual(pane, "focus-ring", focusTarget, t.Transition)
	hover := animateVisual(pane, "hover-ring", hoverTarget, t.Transition)
	if !multiple {
		return
	}
	applyShadowOpacity(pane, t.RingHover, hover)
	applyShadowOpacity(pane, t.RingFocused, focus)
}

func terminalActivity(c *ui.Context, session *remoteSession) {
	t := theme.Current()
	b := t.C.ActivityBadge
	if session.hasNewOutput {
		ui.Row(c).Shrink(0).Padding(b.PaddingY, b.PaddingRight, b.PaddingY, b.PaddingLeft).Gap(b.Gap).Radius(b.Radius).
			Background(tokenColor(b.OutputBg)).PassThrough().Children(func() {
			ui.Box(c).Size(b.DotSize, b.DotSize).Radius(t.Radius.Pill).Background(tokenColor(b.OutputDot))
			ui.Text(c, "新输出").FontSize(b.FontSize).TextColor(tokenColor(b.OutputText)).SingleLine()
		})
	}
	if session.hasBell {
		ui.Row(c).Shrink(0).Padding(b.PaddingY, b.PaddingRight, b.PaddingY, b.PaddingLeft).Radius(b.Radius).Label("会话响铃").Tooltip("会话响铃").
			Background(tokenColor(b.BellBg)).PassThrough().Children(func() {
			ui.Icon(c, bellIcon).Size(b.BellIconSize, b.BellIconSize).TextColor(tokenColor(b.BellText))
		})
	}
}

func terminalCursor(c *ui.Context, session *remoteSession, focused bool) {
	cursor := ui.Box(c).Key("terminal-cursor").Size(8, 16).Shrink(0).Radius(1).Margin(
		0,
		0,
		1.5,
		0,
	)
	if !focused {
		cursor.InsetShadow(
			0,
			0,
			0,
			1.2,
			ui.Hex(terminalSecondaryText),
		)
		return
	}
	cursor.Background(ui.Hex(terminalPrimaryText))
	if c.Preferences().ReduceMotion {
		return
	}
	if session.cursorStarted.IsZero() {
		session.cursorStarted = c.Now()
	}
	const interval = 500 * time.Millisecond
	phase := c.Now().Sub(session.cursorStarted) % (2 * interval)
	if phase >= interval {
		cursor.Opacity(0)
	}
	c.After(interval - phase%interval)
}
