package desktop

import (
	"time"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (a *app) terminalLines(c *ui.Context, session *remoteSession, focused bool) {
	scheme, prefs := termtheme.Current(), a.terminal.prefs
	column := ui.Column(c).FillWidth().MinWidth(0).Selectable().SelectionColor(tokenColor(scheme.SelectionBg))
	if session.contentWidth > 0 {
		column.Width(max(1, session.contentWidth-2*theme.Current().C.Pane.ContentSide)).Shrink(0)
	}
	// Native text selection owns keyboard focus. Sync the pane without moving
	// that focus to the scroll view, which would hide the selection.
	if column.FocusWithin() && !a.currentTab().focused.focusPending && a.currentTab().focused != session {
		a.currentTab().focused = session
		session.hasNewOutput, session.hasBell = false, false
		session.cursorStarted = time.Time{}
		c.AnimationFrame()
	}
	selected := make(map[uint64]ui.SelectedText)
	if column.FocusWithin() {
		for _, part := range column.SelectedTexts() {
			selected[part.ID] = part
		}
	}
	lineView := func(line terminalLine) {
		paragraph := ui.RichText(c).FillWidth().MinWidth(0).FixedLineHeight(prefs.LineHeight).Shrink(0)
		selection := selected[paragraph.ID()]
		spans := terminalSpans(line, scheme, prefs, selection.Start, selection.End)
		paragraph.Children(func() { ui.RichText(c, spans...) })
	}
	column.Children(func() {
		for _, line := range session.output {
			lineView(line)
		}
	})
}
