package desktop

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const (
	terminalPaneRadius = 10
	paneTransition     = 160 * time.Millisecond
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
	focusTarget, hoverTarget := float32(0), float32(0)
	if multiple && focused {
		focusTarget = 1
	}
	if multiple && !focused && pane.Hovered() {
		hoverTarget = 1
	}
	focus := pane.Animate("focus-ring", focusTarget, paneTransition)
	hover := pane.Animate("hover-ring", hoverTarget, paneTransition)
	if !multiple {
		return
	}
	pane.Shadow(
		0,
		0,
		0,
		1,
		ui.RGB(0, 0, 0).Alpha(0.14*hover),
	).Shadow(
		0,
		0,
		0,
		1.5,
		ui.Hex("#0a84ff").Alpha(focus),
	)
}

func terminalActivity(c *ui.Context, session *remoteSession) {
	if session.hasNewOutput {
		ui.Row(c).Shrink(0).Padding(2, 7).Gap(4).Radius(9).
			Background(ui.Hex("#0a84ff").Alpha(0.10)).PassThrough().Children(func() {
			ui.Box(c).Size(6, 6).Radius(99).Background(ui.Hex("#0a84ff"))
			ui.Text(c, "新输出").FontSize(10).TextColor(ui.Hex("#0a6fd6")).SingleLine()
		})
	}
	if session.hasBell {
		ui.Row(c).Shrink(0).Padding(2, 7).Radius(9).Label("会话响铃").Tooltip("会话响铃").
			Background(ui.RGB(245, 166, 35).Alpha(0.14)).PassThrough().Children(func() {
			ui.Icon(c, bellIcon).Size(13, 13).TextColor(ui.Hex("#c7861a"))
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
			ui.Hex(secondaryText),
		)
		return
	}
	cursor.Background(ui.Hex(primaryText))
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
