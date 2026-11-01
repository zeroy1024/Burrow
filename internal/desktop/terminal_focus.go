package desktop

import (
	"slices"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
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

func terminalActivity(c *ui.Context, session *remoteSession, surface *theme.Theme) {
	if session.hasNewOutput {
		uikit.ActivityBadge(c.Key(uikit.ActivityNewOutput), uikit.ActivityNewOutput, surface)
	}
	if session.hasBell {
		uikit.ActivityBadge(c.Key(uikit.ActivityBell), uikit.ActivityBell, surface)
	}
}

func (a *app) terminalCursor(c *ui.Context, session *remoteSession, focused bool, text string) {
	prefs, scheme := a.terminal.prefs, a.sessionScheme(session)
	background := termtheme.TerminalBackground(scheme, theme.Current().C.Pane.Bg, prefs.Background)
	surface := terminalSurface(background)
	width := a.terminalCellWidth()
	height := min(prefs.LineHeight, prefs.FontSize+termtheme.CursorHeightPadding)
	cursor := ui.Box(c).Key("terminal-cursor").Size(width, height).Shrink(0).
		Radius(termtheme.CursorRadius).Margin(0, 0, termtheme.CursorBottomInset, 0).Unselectable()
	if !focused {
		cursor.InsetShadow(
			0,
			0,
			0,
			termtheme.CursorOutlineWidth, uikit.Color(surface.C.PaneFocus.CursorRingUnfocused),
		)
		return
	}
	cursor.Children(func() {
		mark := ui.Box(c).Absolute().Left(0).Bottom(0).Background(uikit.Color(scheme.Cursor))
		switch prefs.CursorShape {
		case termtheme.CursorBar:
			mark.Size(termtheme.CursorBarWidth, height)
		case termtheme.CursorUnderline:
			mark.Size(width, termtheme.CursorUnderlineSize)
		default:
			mark.Size(width, height).Children(func() {
				ui.Text(c, text).TextColor(uikit.Color(scheme.CursorText)).SingleLine()
			})
		}
	})
	if !prefs.CursorBlink || a.reduceMotion(c) {
		return
	}
	if session.cursorStarted.IsZero() {
		session.cursorStarted = c.Now()
	}
	const interval = termtheme.CursorBlinkInterval
	phase := c.Now().Sub(session.cursorStarted) % (2 * interval)
	if phase >= interval {
		cursor.Opacity(0)
	}
	c.After(interval - phase%interval)
}
