package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

type ShellOptions struct {
	Columns, Rows int
	OnError       func(error)
}

// Stream is one leased SSH channel. It never closes a sibling channel.
type Stream struct {
	owner           *Connection
	lease           *lease
	session         *ssh.Session
	stdin           io.Writer
	output          *io.PipeReader
	sink            *io.PipeWriter
	onError         func(error)
	done            chan struct{}
	waitErr         error
	abortOnce       sync.Once
	remoteCloseOnce sync.Once
	errorOnce       sync.Once
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
			c.mu.Lock()
			c.opening = false
			close(c.openingDone)
			c.openingDone = nil
			c.mu.Unlock()
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
	s.session.Stdout, s.session.Stderr = s.sink, s.sink
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
		_, err := io.Copy(s.sink, stdout)
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
	close(s.done)
}

func (s *Stream) Read(p []byte) (int, error) { return s.output.Read(p) }

func (s *Stream) Write(p []byte) (int, error) {
	select {
	case <-s.lease.done:
		return 0, io.ErrClosedPipe
	default:
	}
	n, err := s.stdin.Write(p)
	if err != nil {
		s.fail(fmt.Errorf("write SSH input: %w", err))
	}
	return n, err
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
		s.owner.workers.Go(func() { s.session.Close() })
	})
}

func (s *Stream) abort() {
	s.abortOnce.Do(func() {
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
