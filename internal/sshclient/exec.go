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
}

func (b *commandOutput) Write(p []byte) (int, error) {
	left := b.limit - b.buffer.Len()
	if len(p) <= left {
		return b.buffer.Write(p)
	}
	n, _ := b.buffer.Write(p[:left])
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
	if err != nil && len(result.Stderr) > 0 {
		err = fmt.Errorf("%w; stderr: %s", err, strings.TrimSpace(string(result.Stderr)))
	} else if err == nil && len(result.Stderr) > 0 {
		err = fmt.Errorf("SSH command diagnostics: %s", strings.TrimSpace(string(result.Stderr)))
	}
	return result.Stdout, err
}

// ExecCommand keeps diagnostics and the remote exit status separate. A nil
// ExitCode means the remote result is unknown, including interrupted channels.
func (c *Connection) ExecCommand(ctx context.Context, command string) (ExecResult, error) {
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
