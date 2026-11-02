package desktop

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"sshdesk/internal/sshclient"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	nativeterm "github.com/egoist/mygo/sshdesk-terminal"
	"github.com/egoist/mygo/ui"
)

type sshTerminal struct {
	view            *nativeterm.Terminal
	colors          nativeterm.Theme
	font            nativeterm.Font
	cursor          termtheme.CursorShape
	blink           bool
	scrollbackLines int
	capture         terminalCaptureFrame
}

// The emulator consumes saved history before reading any new remote output.
type sshTerminalConn struct {
	*sshclient.Conn
	reader   io.Reader
	activity func()
	resized  func(int, int)
}

func (c *sshTerminalConn) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 {
		c.activity()
	}
	return n, err
}

func (c *sshTerminalConn) Resize(columns, rows int) error {
	if err := c.Conn.Resize(columns, rows); err != nil {
		return err
	}
	c.resized(columns, rows)
	return nil
}

func (a *app) attachSSH(session *remoteSession, run *sshRuntime) {
	var history []byte
	if session.terminal != nil {
		history = session.terminal.view.History()
		session.terminal.view.Close()
		session.terminal = nil
	} else {
		for _, line := range session.output {
			for _, span := range line {
				history = append(history, span.Text...)
			}
			history = append(history, '\r', '\n')
		}
	}
	var outputPending atomic.Bool
	transport := &sshTerminalConn{
		Conn: run.conn, reader: io.MultiReader(bytes.NewReader(history), run.conn),
		activity: func() {
			// Collapse bursts of output into one UI notification per pending frame.
			if !outputPending.CompareAndSwap(false, true) {
				return
			}
			a.window.Update(func() {
				outputPending.Store(false)
				if session.ssh == run && run.ctx.Err() == nil && !a.sshSessionFocused(session) {
					session.hasNewOutput = true
				}
			})
		},
		resized: func(columns, rows int) {
			a.window.Update(func() {
				if session.ssh == run && run.ctx.Err() == nil {
					session.columns, session.rows = columns, rows
				}
			})
		},
	}
	prefs := a.terminal.prefs
	blink := prefs.CursorBlink && !a.sshReduceMotion()
	scrollbackLines := a.prefs.ScrollbackLines
	columns, rows := run.conn.Size()
	view, err := nativeterm.New(nativeterm.Options{
		Columns: columns, Rows: rows,
		Conn: transport, Font: a.sshFont(), Theme: a.sshColors(session),
		Cursor: nativeterm.CursorStyle(prefs.CursorShape), NoBlink: !blink, ScrollbackLines: scrollbackLines,
		Padding:          nativeterm.Padding{X: theme.Current().C.Pane.ContentSide, Y: -1},
		NoClipboardWrite: true, ClipboardRead: nativeterm.ClipboardDeny,
		OnBell: func() {
			a.window.Update(func() {
				if session.ssh == run && run.ctx.Err() == nil && !a.sshSessionFocused(session) {
					session.hasBell = true
				}
			})
		},
		OnExit: func(int) {
			err := run.conn.Wait()
			a.window.Update(func() {
				if session.ssh != run || run.ctx.Err() != nil || session.state == connectionFailed {
					return
				}
				session.state = disconnected
				if err != nil {
					session.state = connectionFailed
					showSSHMessage(session, "SSH 会话结束："+err.Error())
				}
			})
		},
	})
	if err != nil {
		run.cancel()
		run.conn.Close()
		session.state = connectionFailed
		showSSHMessage(session, "无法启动终端："+err.Error())
		return
	}
	session.terminal = &sshTerminal{
		view: view, colors: *a.sshColors(session), font: a.sshFont(),
		cursor: prefs.CursorShape, blink: blink, scrollbackLines: scrollbackLines,
	}
	session.focusPending = a.sshSessionFocused(session)
	session.output, session.state = nil, connected
	if a.sshSessionFocused(session) {
		session.focus.Focus()
	}
}

func (a *app) sshSessionFocused(session *remoteSession) bool {
	tab := a.currentTab()
	return a.activeSection() == sectionSSH && tab != nil && tab.focused == session
}

func (a *app) sshReduceMotion() bool {
	switch a.prefs.ReduceMotion {
	case "on":
		return true
	case "off":
		return false
	default:
		return mygo.Theme.Preferences().ReduceMotion
	}
}

func (a *app) sshFont() nativeterm.Font {
	p := a.terminal.prefs
	font := ui.Font{Family: strings.Join(p.FontFamily, ", "), Size: p.FontSize}
	m := font.Metrics()
	return nativeterm.Font{
		Family: font.Family, Size: p.FontSize,
		LineHeight: p.LineHeight / (m.Ascent + m.Descent + m.LineGap),
	}
}

func (a *app) sshColors(session *remoteSession) *nativeterm.Theme {
	s, p := a.sessionScheme(session), a.terminal.prefs
	resolved := uikit.ResolveTerminalColors(s, p.Background)
	colors := &nativeterm.Theme{
		Foreground: uikit.Color(s.Foreground),
		Background: uikit.Color(resolved.Background),
		Cursor:     uikit.Color(s.Cursor), CursorText: uikit.Color(s.CursorText),
		Selection: uikit.Color(s.SelectionBg), SelectionText: uikit.Color(s.SelectionFg),
	}
	for i, value := range s.ANSI {
		colors.Palette[i] = uikit.Color(value)
	}
	return colors
}

func (a *app) syncSSHTerminal(session *remoteSession) {
	live := session.terminal
	colors, font := a.sshColors(session), a.sshFont()
	if *colors != live.colors {
		live.view.SetTheme(colors, nil)
		live.colors = *colors
	}
	if font.Family != live.font.Family || font.Size != live.font.Size || font.LineHeight != live.font.LineHeight {
		live.view.SetFont(font)
		live.font = font
	}
	shape, blink := a.terminal.prefs.CursorShape, a.terminal.prefs.CursorBlink && !a.sshReduceMotion()
	if shape != live.cursor || blink != live.blink {
		code := map[termtheme.CursorShape]int{termtheme.CursorBlock: 2, termtheme.CursorUnderline: 4, termtheme.CursorBar: 6}[shape]
		if blink {
			code--
		}
		live.view.Feed([]byte(fmt.Sprintf("\x1b[%d q", code)))
		live.cursor, live.blink = shape, blink
	}
}

// sshTerminalView supplies the native text surface; its caller retains the pane
// geometry, clipping, opacity and chrome already used by the prototype.
func (a *app) sshTerminalView(c *ui.Context, session *remoteSession) ui.Element {
	a.syncSSHTerminal(session)
	output := nativeterm.View(c, session.terminal.view).Bind(&session.focus).Label(session.endpoint.name + " SSH 终端")
	if session.focusPending {
		if a.sshSessionFocused(session) {
			output.Focus()
		}
		session.focusPending = false
	}
	if output.Focused() {
		tab := a.currentTab()
		if tab.focused != session && !tab.focused.focusPending {
			tab.focused = session
			session.hasNewOutput, session.hasBell = false, false
			c.AnimationFrame()
		}
	}
	return output
}

func showSSHMessage(session *remoteSession, message string) {
	if session.terminal != nil {
		session.terminal.view.Feed([]byte("\r\n\x1b[0m" + message + "\r\n"))
		return
	}
	appendSessionLines(session, terminalLine{{Text: message}})
}

func releaseSSH(session *remoteSession) {
	stopSSH(session)
	if session.terminal != nil {
		session.terminal.view.Close()
		session.terminal = nil
	}
}
