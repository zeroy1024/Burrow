package desktop

import (
	"bytes"
	"context"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"sync"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

	nativeterm "github.com/egoist/mygo/burrow-terminal"
	"github.com/egoist/mygo/ui"
)

type terminalPreview struct {
	view *nativeterm.Terminal
	err  error
}

type terminalCaptureFrame struct {
	body       ui.Rect
	rootSize   ui.Point
	pixelScale float32
}

// Only the latest press owns pixels. Geometry is updated by painted native
// views, so a pointer event can crop the same frame without retaining Elements.
type terminalCaptureState struct {
	generation uint64
	pressed    bool
	fromTab    bool
	capturing  bool
	bound      bool
	handle     ui.Handle
	session    *remoteSession
	terminal   *sshTerminal
	bitmap     *ui.Bitmap
	cancel     context.CancelFunc
	reason     string
	strip      ui.Rect
	tabHandle  ui.Rect
	tabSource  *remoteSession
}

func (a *app) terminalCapturePressed(c *ui.Context) bool {
	s := &a.terminalCapture
	if !s.pressed || !a.terminalCaptureVisible(s.session) {
		return false
	}
	return s.capturing || !s.bound || c.Resolve(s.handle).Pressed()
}

func (a *app) terminalCaptureVisible(session *remoteSession) bool {
	tab := a.currentTab()
	return session != nil && !session.closing && session.terminal != nil && a.activeSection() == sectionSSH &&
		tab != nil && tab.root.find(session) != nil && (!tab.maximized || tab.focused == session)
}

func (a *app) recordTerminalCapture(c *ui.Context, session *remoteSession, output, card ui.Element, top, bottom float32) {
	rootWidth, rootHeight := c.Size()
	live := session.terminal
	output.DrawOver(func(painter *ui.Painter, _ ui.Rect) {
		if !output.Valid() || !card.Valid() {
			return
		}
		body, bounds, layout := output.Bounds(), card.Bounds(), card.LayoutBounds()
		if layout.W <= 0 || layout.H <= 0 {
			return
		}
		xScale, yScale := bounds.W/layout.W, bounds.H/layout.H
		side := theme.Current().C.Pane.ContentSide * xScale
		x0, y0 := max(body.X+side, bounds.X+side), max(body.Y, bounds.Y+top*yScale)
		x1, y1 := min(body.X+body.W-side, bounds.X+bounds.W-side), min(body.Y+body.H, bounds.Y+bounds.H-bottom*yScale)
		live.capture = terminalCaptureFrame{
			body:       ui.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)},
			rootSize:   ui.Point{X: rootWidth, Y: rootHeight},
			pixelScale: painter.Scale(),
		}
	})
}

// captureTerminalPress runs only from the native pointer-down callback, before
// the drag changes the scene. CapturePage renders that displayed scene; PNG
// decoding and cropping are kept off the window thread.
func (a *app) captureTerminalPress(session *remoteSession, fromTab bool) {
	a.clearTerminalPreviews()
	s := &a.terminalCapture
	if !a.terminalCaptureVisible(session) {
		s.reason = locales.MessageTheSourcePaneIsHiddenUsingAnIndependentTerminalStateSnapshot()
		return
	}
	frame := session.terminal.capture
	if frame.body.W <= 0 || frame.body.H <= 0 || frame.rootSize.X <= 0 || frame.rootSize.Y <= 0 {
		s.reason = locales.MessageTheSourceTerminalHasNoRenderedContentYetUsingAnIndependentTerminalState()
		return
	}
	s.session, s.terminal, s.pressed, s.capturing = session, session.terminal, true, true
	s.fromTab = fromTab
	data, err := a.window.CapturePage()
	s.capturing = false
	if err != nil {
		s.reason = locales.MessageUnableToCaptureSourceTerminal() + err.Error()
		return
	}
	// CapturePage may refresh a held scene. The DrawOver metadata then belongs
	// to that same refreshed scene rather than to its preceding layout.
	frame = s.terminal.capture
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	generation, terminal := s.generation, s.terminal
	a.sshWorkers.Go(func() {
		if ctx.Err() != nil {
			return
		}
		cropped, err := cropTerminalCapture(data, frame)
		if ctx.Err() != nil {
			return
		}
		a.window.Update(func() {
			defer cancel()
			if ctx.Err() != nil || s.generation != generation || s.session != session || session.terminal != terminal {
				return
			}
			s.cancel = nil
			if err != nil {
				s.reason = locales.MessageUnableToCropSourceTerminal() + err.Error()
				return
			}
			s.bitmap = ui.NewBitmap(cropped)
			if preview := a.terminalPreviews[session]; preview != nil {
				if preview.view != nil {
					preview.view.Close()
				}
				delete(a.terminalPreviews, session)
			}
		})
	})
}

func cropTerminalCapture(data []byte, frame terminalCaptureFrame) (*image.RGBA, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	xScale, yScale := float64(frame.pixelScale), float64(frame.pixelScale)
	r := frame.body
	// Round inward: no pixel outside this native body may bring in a neighbour,
	// an identity header or a footer.
	crop := image.Rect(bounds.Min.X+int(math.Ceil(float64(r.X)*xScale)), bounds.Min.Y+int(math.Ceil(float64(r.Y)*yScale)),
		bounds.Min.X+int(math.Floor(float64(r.X+r.W)*xScale)), bounds.Min.Y+int(math.Floor(float64(r.Y+r.H)*yScale)))
	if crop.Empty() || !crop.In(bounds) {
		return nil, locales.Errorf(locales.MessageTerminalContentExtendsBeyondTheCapturedWindow)
	}
	result := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(result, result.Bounds(), source, crop.Min, draw.Src)
	return result, nil
}

func (a *app) captureTerminalTabPress(ev ui.InputEvent) {
	s := &a.terminalCapture
	x, y := s.strip.X+ev.X, s.strip.Y+ev.Y
	r := s.tabHandle
	session := s.tabSource
	tab := a.currentTab()
	if tab == nil || tab.focused != session || x < r.X || y < r.Y || x >= r.X+r.W || y >= r.Y+r.H {
		session = nil
	}
	a.captureTerminalPress(session, true)
}

// The preview never reads or writes a shell. Read waits until disposal so the
// emulator stays on its captured screen; generated replies are discarded.
type terminalPreviewConn struct {
	done chan struct{}
	once sync.Once
}

func (p *terminalPreviewConn) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	<-p.done
	return 0, io.EOF
}

func (p *terminalPreviewConn) Write(data []byte) (int, error) { return len(data), nil }

func (p *terminalPreviewConn) Resize(int, int) error { return nil }

func (p *terminalPreviewConn) Close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

func (a *app) nativeTerminalDragContents(c *ui.Context, session *remoteSession, viewport terminalDragViewport, width, height float32) bool {
	if session.terminal == nil {
		return false
	}
	if capture := &a.terminalCapture; capture.session == session && capture.terminal == session.terminal && capture.bitmap != nil {
		a.capturedTerminalDragContents(c, capture.bitmap, viewport, width, height)
		return true
	}
	if a.terminalPreviews == nil {
		a.terminalPreviews = make(map[*remoteSession]*terminalPreview)
	}
	preview := a.terminalPreviews[session]
	if preview == nil {
		live := session.terminal
		columns, rows := live.view.Size()
		snapshot := live.view.Snapshot()
		colors := live.colors
		conn := &terminalPreviewConn{done: make(chan struct{})}
		view, err := nativeterm.New(nativeterm.Options{
			Columns: columns, Rows: rows,
			Conn: conn, ResizeByConn: true, Font: live.font, Theme: &colors, ScrollbackLines: live.scrollbackLines,
			Padding: nativeterm.Padding{X: -1, Y: -1}, Transparent: true,
			NoBlink: true, NoDefaultKeybinds: true, NoClipboardWrite: true,
			ClipboardRead: nativeterm.ClipboardDeny,
		})
		if err != nil {
			conn.Close()
		} else {
			view.Resize(columns, rows)
			view.Feed(snapshot)
		}
		preview = &terminalPreview{view: view, err: err}
		a.terminalPreviews[session] = preview
	}
	t := theme.Current()
	sourceWidth := viewport.Width
	sourceHeight := viewport.Height - t.C.Pane.HeaderHeight - t.C.StatusBar.HeightPane
	scale := min(width/sourceWidth, height/sourceHeight)
	ui.Box(c).Size(width, height).Clip().PassThrough().Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Size(sourceWidth, sourceHeight).
			Scale(scale, scale).TransformOrigin(0, 0).Clip().PassThrough().Children(func() {
			ui.Box(c).Absolute().Left(t.C.Pane.ContentSide).Top(t.C.Pane.ContentSide).
				Width(sourceWidth - 2*t.C.Pane.ContentSide).Height(max(1, sourceHeight-2*t.C.Pane.ContentSide)).Clip().PassThrough().Children(func() {
				if preview.err != nil {
					uikit.Text(
						c,
						locales.MessageUnableToDisplayTerminalSnapshot()+preview.err.Error(),
						uikit.TextProps{Style: uikit.TextSmall},
					).PassThrough()
					return
				}
				nativeterm.View(c, preview.view).Fill().PassThrough().Unselectable().InputMethod(false).
					HandleInput(nil).ContextMenu(func(*ui.Menu) {})
			})
		})
	})
	return true
}

func (a *app) capturedTerminalDragContents(c *ui.Context, bitmap *ui.Bitmap, viewport terminalDragViewport, width, height float32) {
	t := theme.Current()
	sourceWidth := viewport.Width
	sourceHeight := viewport.Height - t.C.Pane.HeaderHeight - t.C.StatusBar.HeightPane
	scale := min(width/sourceWidth, height/sourceHeight)
	ui.Box(c).Size(width, height).Clip().PassThrough().Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Size(sourceWidth, sourceHeight).
			Scale(scale, scale).TransformOrigin(0, 0).Clip().PassThrough().Children(func() {
			contentWidth := sourceWidth - 2*t.C.Pane.ContentSide
			ui.Box(c).Absolute().Left(t.C.Pane.ContentSide).Top(t.C.Pane.ContentSide).
				Size(contentWidth, max(1, sourceHeight-2*t.C.Pane.ContentSide)).Clip().PassThrough().Children(func() {
				pixelsWide, pixelsHigh := bitmap.Size()
				ui.Image(c, bitmap).Size(contentWidth, contentWidth*float32(pixelsHigh)/float32(pixelsWide)).PassThrough()
			})
		})
	})
}

func (a *app) clearTerminalPreviews() {
	s := &a.terminalCapture
	if s.cancel != nil {
		s.cancel()
	}
	// NewBitmap owns its Go pixels; dropping the sole gesture reference lets
	// the SDK retire the texture with the last scene that used it.
	s.generation++
	s.pressed, s.capturing, s.bound, s.fromTab = false, false, false, false
	s.session, s.terminal, s.bitmap, s.cancel, s.reason = nil, nil, nil, nil, ""
	for session, preview := range a.terminalPreviews {
		if preview.view != nil {
			preview.view.Close()
		}
		delete(a.terminalPreviews, session)
	}
}
