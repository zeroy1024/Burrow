package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

type ShellOptions struct {
	Columns, Rows int
	OnError       func(error)
	Env           []string
	InitCommand   string
}

// Stream is one leased SSH channel. It never closes a sibling channel.
type Stream struct {
	owner           *Connection
	lease           *lease
	session         *ssh.Session
	stdin           io.Writer
	input           chan streamWrite
	initialized     chan struct{}
	initializedOnce sync.Once
	output          *io.PipeReader
	sink            *io.PipeWriter
	onError         func(error)
	shell           bool
	done            chan struct{}
	waitErr         error
	abortOnce       sync.Once
	remoteCloseOnce sync.Once
	errorOnce       sync.Once
	discardOutput   atomic.Bool
}

type streamOutput struct{ stream *Stream }

func (output streamOutput) Write(p []byte) (int, error) {
	s := output.stream
	if s.discardOutput.Load() {
		return len(p), nil
	}
	n, err := s.sink.Write(p)
	if err != nil && s.discardOutput.Load() {
		// A canceled view stops displaying output, but SSH must keep draining
		// this channel's receive window for the peer to complete channel close.
		return len(p), nil
	}
	return n, err
}

type streamWrite struct {
	data []byte
	done chan streamWriteResult
}

type streamWriteResult struct {
	n   int
	err error
}

// Conn is an independent interactive Shell on a shared Connection.
type Conn struct {
	*Stream
	resizeMu      sync.Mutex
	columns, rows int
}

func (c *Connection) OpenShell(ctx context.Context, opts ShellOptions) (*Conn, error) {
	if opts.Columns < 1 || opts.Rows < 1 {
		return nil, errors.New("SSH terminal size must be positive")
	}
	stream, err := c.open(ctx, "", opts)
	if err != nil {
		return nil, err
	}
	return &Conn{Stream: stream, columns: opts.Columns, rows: opts.Rows}, nil
}

// InitializationDone closes after the initial-command write finishes or the
// channel closes. Write failures are reported through ShellOptions.OnError.
func (c *Conn) InitializationDone() <-chan struct{} { return c.initialized }

// OpenSubsystem opens a binary subsystem without a PTY. Its stdout is the
// protocol stream; stderr is drained separately and never mixed into it.
func (c *Connection) OpenSubsystem(ctx context.Context, name string) (*Stream, error) {
	if name == "" {
		return nil, errors.New("SSH subsystem name must not be empty")
	}
	return c.open(ctx, name, ShellOptions{})
}

func (c *Connection) open(ctx context.Context, subsystem string, opts ShellOptions) (*Stream, error) {
	l, err := c.beginOpen(ctx)
	if err != nil {
		return nil, err
	}
	type result struct {
		stream *Stream
		err    error
	}
	ready := make(chan result, 1)
	go func() {
		defer c.workers.Done()
		publish := func(opened result) {
			c.endOpen()
			ready <- opened
		}
		session, err := c.client.NewSession()
		if err != nil {
			l.release(false)
			publish(result{err: fmt.Errorf("open SSH channel: %w", err)})
			return
		}
		output, sink := io.Pipe()
		s := &Stream{
			owner: c, lease: l, session: session, output: output, sink: sink,
			onError: opts.OnError, done: make(chan struct{}),
			input: make(chan streamWrite), initialized: make(chan struct{}),
			shell: subsystem == "",
		}
		if !l.bind(s.abort) {
			s.abort()
			publish(result{err: ctx.Err()})
			return
		}
		stdin, err := session.StdinPipe()
		s.stdin = stdin
		if err == nil {
			if subsystem == "" {
				err = s.startShell(opts)
			} else {
				err = s.startSubsystem(subsystem)
			}
		}
		if err != nil {
			l.release(true)
			publish(result{err: err})
			return
		}
		select {
		case <-l.done:
			s.abort()
			publish(result{err: ctx.Err()})
		default:
			publish(result{stream: s})
			// Publish before starting the first write: its echoed output may
			// need the caller's reader before stdin can make progress.
			c.workers.Go(func() { s.writeInput(opts.InitCommand) })
		}
	}()
	select {
	case opened := <-ready:
		if err := ctx.Err(); err != nil {
			l.release(true)
			return nil, err
		}
		if opened.stream == nil && opened.err == nil {
			opened.err = ErrClosed
		}
		return opened.stream, opened.err
	case <-ctx.Done():
		l.release(true)
		return nil, ctx.Err()
	case <-c.done:
		l.release(true)
		return nil, ErrClosed
	}
}

func (s *Stream) startShell(opts ShellOptions) error {
	s.session.Stdout, s.session.Stderr = streamOutput{s}, streamOutput{s}
	for _, value := range opts.Env {
		name, value, ok := strings.Cut(value, "=")
		if !ok || name == "" || strings.ContainsRune(name, '\x00') || strings.ContainsRune(value, '\x00') {
			return errors.New("SSH environment entries must have the form KEY=VALUE without NUL")
		}
		if err := s.session.Setenv(name, value); err != nil {
			return fmt.Errorf("set SSH environment %s: %w", name, err)
		}
	}
	if err := s.session.RequestPty("xterm-256color", opts.Rows, opts.Columns, ssh.TerminalModes{
		ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return fmt.Errorf("request SSH terminal: %w", err)
	}
	if err := s.session.Shell(); err != nil {
		return fmt.Errorf("start SSH shell: %w", err)
	}
	s.owner.workers.Go(func() { s.finish(s.session.Wait()) })
	return nil
}

func (s *Stream) writeInput(initial string) {
	select {
	case <-s.lease.done:
		s.completeInitialization()
		return
	default:
	}
	if initial != "" {
		_, err := io.WriteString(s.stdin, initial+"\n")
		if err != nil {
			s.inputFailed(fmt.Errorf("write SSH initial command: %w", err))
			s.completeInitialization()
			return
		}
	}
	s.completeInitialization()
	for {
		select {
		case <-s.lease.done:
			return
		case request := <-s.input:
			n, err := s.stdin.Write(request.data)
			if err != nil {
				s.inputFailed(fmt.Errorf("write SSH input: %w", err))
			}
			request.done <- streamWriteResult{n: n, err: err}
			if err != nil {
				return
			}
		}
	}
}

func (s *Stream) completeInitialization() {
	s.initializedOnce.Do(func() { close(s.initialized) })
}

func (s *Stream) inputFailed(err error) {
	select {
	case <-s.lease.done:
		// An explicit close/cancel already owns the view's final state.
	default:
		s.fail(err)
	}
}

func (s *Stream) startSubsystem(name string) error {
	stdout, err := s.session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open SSH subsystem output: %w", err)
	}
	stderr, err := s.session.StderrPipe()
	if err != nil {
		return fmt.Errorf("open SSH subsystem errors: %w", err)
	}
	if err := s.session.RequestSubsystem(name); err != nil {
		return fmt.Errorf("start SSH subsystem: %w", err)
	}
	s.owner.workers.Go(func() {
		// Subsystem diagnostics are not protocol bytes.
		_, _ = io.Copy(io.Discard, stderr)
	})
	s.owner.workers.Go(func() {
		_, err := io.Copy(streamOutput{s}, stdout)
		s.finish(err)
	})
	return nil
}

func (s *Stream) finish(err error) {
	s.waitErr = err
	// The final successful pipe Write has already been read by the consumer.
	s.sink.Close()
	s.closeRemote()
	s.lease.release(false)
	s.completeInitialization()
	close(s.done)
}

func (s *Stream) Read(p []byte) (int, error) { return s.output.Read(p) }

func (s *Stream) Write(p []byte) (int, error) {
	select {
	case <-s.lease.done:
		return 0, io.ErrClosedPipe
	default:
	}
	request := streamWrite{data: append([]byte(nil), p...), done: make(chan streamWriteResult, 1)}
	select {
	case <-s.lease.done:
		return 0, io.ErrClosedPipe
	case s.input <- request:
	}
	select {
	case <-s.lease.done:
		return 0, io.ErrClosedPipe
	case result := <-request.done:
		return result.n, result.err
	}
}

func (s *Stream) fail(err error) {
	s.errorOnce.Do(func() {
		s.Close()
		if s.onError != nil {
			s.onError(err)
		}
	})
}

func (s *Stream) closeRemote() {
	s.remoteCloseOnce.Do(func() {
		// Session.Close can block on network I/O. Keep it off the caller/UI thread
		// and join it when the connection owner shuts down.
		s.owner.workers.Go(func() {
			if s.shell {
				// OpenSSH can keep a PTY alive while a canceled stdin write is
				// flow-controlled. Hang up this shell's process group first;
				// a peer that does not support signals still receives Close.
				s.session.Signal(ssh.SIGHUP)
			}
			s.session.Close()
		})
	})
}

func (s *Stream) abort() {
	s.abortOnce.Do(func() {
		s.completeInitialization()
		s.discardOutput.Store(true)
		s.output.Close()
		s.sink.Close()
		s.closeRemote()
	})
}

// Close ends local I/O immediately and initiates the channel-close request.

func (s *Stream) Close() error {
	s.abort()
	s.lease.release(false)
	return nil
}

func (s *Stream) Wait() error {
	<-s.done
	return s.waitErr
}

// Size returns the last PTY grid accepted by the server.
func (c *Conn) Size() (columns, rows int) {
	c.resizeMu.Lock()
	defer c.resizeMu.Unlock()
	return c.columns, c.rows
}

func (c *Conn) Resize(columns, rows int) error {
	if columns < 1 || rows < 1 {
		return errors.New("SSH terminal size must be positive")
	}
	select {
	case <-c.lease.done:
		return io.ErrClosedPipe
	default:
	}
	c.resizeMu.Lock()
	defer c.resizeMu.Unlock()
	if columns == c.columns && rows == c.rows {
		return nil
	}
	if err := c.session.WindowChange(rows, columns); err != nil {
		err = fmt.Errorf("resize SSH terminal: %w", err)
		c.fail(err)
		return err
	}
	c.columns, c.rows = columns, rows
	return nil
}
