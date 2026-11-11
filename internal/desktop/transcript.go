package desktop

import (
	"fmt"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

// reconnectHint ends the output of a pane that has no shell, as a shell ends
// its output with a prompt: Enter connects. A live terminal gets the same
// hint fed into it (showReconnectHint).
func reconnectHint(c *ui.Context, session *remoteSession) []terminalLine {
	if !session.awaitingConnect() {
		return nil
	}
	if len(session.transcript) == 0 && len(session.output) == 0 {
		h := session.endpoint
		return []terminalLine{
			{{Text: locales.MessageSessionRestoredNotConnectedYet()}},
			{{Text: fmt.Sprintf(
				locales.MessagePressEnterToConnectTo(),
				fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port),
			), Dim: true}},
		}
	}
	return []terminalLine{{{Text: locales.MessagePressEnterToReconnect(), Dim: true}}}
}

func (a *app) terminalLines(c *ui.Context, session *remoteSession, focused bool) {
	h := session.endpoint
	scheme, prefs := a.sessionScheme(session), a.terminal.prefs
	column := ui.Column(c).FillWidth().MinWidth(0).Selectable().SelectionColor(uikit.Color(scheme.SelectionBg))
	if session.contentWidth > 0 {
		column.Width(max(1, session.contentWidth-2*theme.Current().C.Pane.ContentSide)).Shrink(0)
	}
	// Native text selection owns keyboard focus. Sync the pane without moving
	// that focus to the scroll view, which would hide the selection.
	if column.FocusWithin() && a.currentTab().focused != session {
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
		for _, line := range session.transcript {
			lineView(line)
		}
		for _, line := range session.output {
			lineView(line)
		}
		if hint := reconnectHint(c, session); len(hint) > 0 {
			for _, line := range hint[:len(hint)-1] {
				lineView(line)
			}
			// The last hint line takes the cursor, as a shell prompt does.
			ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).Children(func() {
				paragraph := ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight)
				selection := selected[paragraph.ID()]
				paragraph.Children(func() {
					ui.RichText(c, terminalSpans(hint[len(hint)-1], scheme, prefs, selection.Start, selection.End)...)
				})
				a.terminalCursor(c, session, focused, " ")
			})
			return
		}
		if session.state != connected {
			return
		}
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).Children(func() {
			prompt := terminalLine{
				{Text: h.user + "@" + h.name, Foreground: terminalIndexed(2), Bold: true},
				{Text: ":/srv/app", Foreground: terminalIndexed(4), Bold: true},
				{Text: "# "},
			}
			paragraph := ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight)
			selection := selected[paragraph.ID()]
			paragraph.Children(func() {
				ui.RichText(c, terminalSpans(prompt, scheme, prefs, selection.Start, selection.End)...)
			})
			a.terminalCursor(c, session, focused, " ")
		})
	})
}

type terminalDragViewport struct {
	Width, Height, ScrollY float32
}

func terminalDragViewportFor(session *remoteSession) terminalDragViewport {
	return terminalDragViewport{
		Width: session.contentWidth, Height: session.contentHeight,
		ScrollY: min(session.scroll.offset.Y, session.scroll.offset.MaxY),
	}
}

func (a *app) terminalDragContents(
	c *ui.Context,
	session *remoteSession,
	viewport terminalDragViewport,
	width, height float32,
) {
	if a.nativeTerminalDragContents(c, session, viewport, width, height) {
		return
	}
	t := theme.Current()
	prefs, scheme := a.terminal.prefs, a.sessionScheme(session)
	colors := uikit.ResolveTerminalColors(scheme, prefs.Background)
	sourceWidth := viewport.Width
	sourceHeight := viewport.Height - t.C.Pane.HeaderHeight - t.C.StatusBar.HeightPane
	scale := min(width/sourceWidth, height/sourceHeight)
	ui.Box(c).Size(width, height).Clip().PassThrough().Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Size(sourceWidth, sourceHeight).
			Scale(scale, scale).TransformOrigin(0, 0).Clip().PassThrough().
			Font(strings.Join(prefs.FontFamily, ", ")).FontSize(prefs.FontSize).
			TextColor(uikit.Color(colors.Foreground)).Children(func() {
			ui.Column(c).Absolute().Left(t.C.Pane.ContentSide).Top(t.C.Pane.ContentSide - viewport.ScrollY).
				Width(sourceWidth - 2*t.C.Pane.ContentSide).MinWidth(0).Shrink(0).PassThrough().Children(func() {
				lineView := func(line terminalLine) {
					paragraph := ui.RichText(c).FillWidth().MinWidth(0).
						FixedLineHeight(prefs.LineHeight).Shrink(0).PassThrough()
					paragraph.Children(func() {
						ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...).PassThrough()
					})
				}
				for _, line := range session.transcript {
					lineView(line)
				}
				for _, line := range session.output {
					lineView(line)
				}
				for _, line := range reconnectHint(c, session) {
					lineView(line)
				}
				if session.state != connected {
					return
				}
				ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).PassThrough().Children(func() {
					h := session.endpoint
					prompt := terminalLine{
						{Text: h.user + "@" + h.name, Foreground: terminalIndexed(2), Bold: true},
						{Text: ":/srv/app", Foreground: terminalIndexed(4), Bold: true},
						{Text: "# "},
					}
					ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight).PassThrough().
						Children(func() {
							ui.RichText(c, terminalSpans(prompt, scheme, prefs, 0, 0)...).PassThrough()
						})
					cursorHeight := min(prefs.LineHeight, prefs.FontSize+termtheme.CursorHeightPadding)
					ui.Box(c).Size(a.cellWidth, cursorHeight).Shrink(0).Radius(termtheme.CursorRadius).
						Margin(0, 0, termtheme.CursorBottomInset, 0).PassThrough().Children(func() {
						mark := ui.Box(c).Absolute().Left(0).Bottom(0).
							Background(uikit.Color(scheme.Cursor)).PassThrough()
						switch prefs.CursorShape {
						case termtheme.CursorBar:
							mark.Size(termtheme.CursorBarWidth, cursorHeight)
						case termtheme.CursorUnderline:
							mark.Size(a.cellWidth, termtheme.CursorUnderlineSize)
						default:
							mark.Size(a.cellWidth, cursorHeight)
						}
					})
				})
			})
		})
	})
}
