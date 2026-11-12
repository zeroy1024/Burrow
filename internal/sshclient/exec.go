package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

const (
	maxExecOutput = 1024 * 1024
	maxExecStderr = 64 * 1024
)

var errExecOutputLimit = errors.New("SSH command output exceeds its limit")

type ExecResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  *int
	Truncated bool
}

type commandOutput struct {
	buffer   bytes.Buffer
	limit    int
	abort    func()
	exceeded bool
	// observe sees the bytes kept in buffer as they arrive.
	observe func([]byte)
}

func (b *commandOutput) Write(p []byte) (int, error) {
	left := b.limit - b.buffer.Len()
	if len(p) <= left {
		if b.observe != nil {
			b.observe(p)
		}
		return b.buffer.Write(p)
	}
	n, _ := b.buffer.Write(p[:left])
	if b.observe != nil && n > 0 {
		b.observe(p[:n])
	}
	b.exceeded = true
	b.abort()
	return n, errExecOutputLimit
}

// Exec runs a command in an independent channel with no PTY. Callers supply
// fixed commands and a deadline. Stdout is limited to 1 MiB, stderr to 64 KiB;
// stderr diagnostics accompany failures and are never mixed into stdout.
// The channel slot stays occupied until command I/O ends, including after a
// cancellation, so a server ignoring channel close cannot accumulate workers.
func (c *Connection) Exec(ctx context.Context, command string) ([]byte, error) {
	result, err := c.ExecCommand(ctx, command)
	// Remote stderr is arbitrary bytes, such as a Windows console code page.
	// Error text reaches UI and accessibility APIs that require valid UTF-8.
	diagnostics := strings.ToValidUTF8(strings.TrimSpace(string(result.Stderr)), "\uFFFD")
	if err != nil && len(result.Stderr) > 0 {
		err = fmt.Errorf("%w; stderr: %s", err, diagnostics)
	} else if err == nil && len(result.Stderr) > 0 {
		err = fmt.Errorf("SSH command diagnostics: %s", diagnostics)
	}
	return result.Stdout, err
}

// ExecCommand keeps diagnostics and the remote exit status separate. A nil
// ExitCode means the remote result is unknown, including interrupted channels.
func (c *Connection) ExecCommand(ctx context.Context, command string) (ExecResult, error) {
	return c.ExecObserved(ctx, command, nil)
}

// ExecObserved is ExecCommand with a live view of the output: observe sees
// each chunk within the limits as it arrives, before the result returns. The
// two streams call it from separate goroutines, and it must not keep the
// slice. Nil observes nothing.
func (c *Connection) ExecObserved(ctx context.Context, command string, observe func(stderr bool, data []byte)) (ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return ExecResult{}, errors.New("SSH command must not be empty")
	}
	l, err := c.acquireOpen(ctx)
	if err != nil {
		return ExecResult{}, err
	}
	type result struct {
		output ExecResult
		err    error
	}
	ready := make(chan result, 1)
	go func() {
		defer c.workers.Done()
		defer c.endOpen()
		session, err := c.client.NewSession()
		if err != nil {
			l.release(false)
			ready <- result{err: fmt.Errorf("open SSH command channel: %w", err)}
			return
		}
		var closeOnce sync.Once
		abort := func() {
			closeOnce.Do(func() { c.workers.Go(func() { session.Close() }) })
		}
		defer abort()
		defer l.release(false)
		if !l.bind(abort) {
			ready <- result{err: ErrClosed}
			return
		}
		stdout := &commandOutput{limit: maxExecOutput, abort: abort}
		stderr := &commandOutput{limit: maxExecStderr, abort: abort}
		if observe != nil {
			stdout.observe = func(data []byte) { observe(false, data) }
			stderr.observe = func(data []byte) { observe(true, data) }
		}
		session.Stdout, session.Stderr = stdout, stderr
		err = session.Run(command)
		output := ExecResult{Stdout: stdout.buffer.Bytes(), Stderr: stderr.buffer.Bytes()}
		var exit *ssh.ExitError
		if err == nil {
			code := 0
			output.ExitCode = &code
		} else if errors.As(err, &exit) {
			code := exit.ExitStatus()
			if code >= 0 {
				output.ExitCode = &code
			}
		}
		if stdout.exceeded || stderr.exceeded {
			output.Truncated = true
			err = errors.Join(errExecOutputLimit, err)
		}
		if err != nil {
			err = fmt.Errorf("run SSH command: %w", err)
		}
		ready <- result{output: output, err: err}
	}()
	select {
	case completed := <-ready:
		if err := ctx.Err(); err != nil {
			l.release(true)
			return ExecResult{}, err
		}
		return completed.output, completed.err
	case <-ctx.Done():
		l.release(true)
		return ExecResult{}, ctx.Err()
	case <-c.done:
		l.release(true)
		return ExecResult{}, ErrClosed
	}
}
