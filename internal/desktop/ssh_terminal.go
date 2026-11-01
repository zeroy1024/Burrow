package desktop

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"

	"sshdesk/internal/sshclient"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	nativeterm "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type sshTerminal struct {
	view   *nativeterm.Terminal
	colors nativeterm.Theme
	font   nativeterm.Font
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
	view, err := nativeterm.New(nativeterm.Options{
		Conn: transport, Font: a.sshFont(), Theme: a.sshColors(),
		Cursor: nativeterm.CursorStyle(prefs.CursorShape), NoBlink: !prefs.CursorBlink || a.reduceMotion,
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
		view: view, colors: *a.sshColors(), font: a.sshFont(),
	}
	session.focusPending = a.sshSessionFocused(session)
	session.output, session.state = nil, connected
	if a.sshSessionFocused(session) {
		session.focus.Focus()
	}
}

func (a *app) sshSessionFocused(session *remoteSession) bool {
	tab := a.currentTab()
	return a.section == sectionSSH && tab != nil && tab.focused == session
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

func (a *app) sshColors() *nativeterm.Theme {
	s, p := termtheme.Current(), a.terminal.prefs
	colors := &nativeterm.Theme{
		Foreground: tokenColor(s.Foreground),
		Background: tokenColor(termtheme.TerminalBackground(s, theme.Current().C.Pane.Bg, p.Background)),
		Cursor:     tokenColor(s.Cursor), CursorText: tokenColor(s.CursorText),
		Selection: tokenColor(s.SelectionBg), SelectionText: tokenColor(s.SelectionFg),
	}
	for i, value := range s.ANSI {
		colors.Palette[i] = tokenColor(value)
	}
	return colors
}

func (a *app) syncSSHTerminal(live *sshTerminal) {
	colors, font := a.sshColors(), a.sshFont()
	if *colors != live.colors {
		live.view.SetTheme(colors, nil)
		live.colors = *colors
	}
	if font.Family != live.font.Family || font.Size != live.font.Size || font.LineHeight != live.font.LineHeight {
		live.view.SetFont(font)
		live.font = font
	}
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
