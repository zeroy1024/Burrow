package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	maxExecOutput = 1024 * 1024
	maxExecStderr = 64 * 1024
)

var errExecOutputLimit = errors.New("SSH command output exceeds its limit")

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
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("SSH command must not be empty")
	}
	l, err := c.acquireOpen(ctx)
	if err != nil {
		return nil, err
	}
	type result struct {
		output []byte
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
		if stdout.exceeded || stderr.exceeded {
			err = errors.Join(errExecOutputLimit, err)
		}
		if err != nil {
			if stderr.buffer.Len() > 0 {
				err = fmt.Errorf("run SSH command: %w; stderr: %s", err, strings.TrimSpace(stderr.buffer.String()))
			} else {
				err = fmt.Errorf("run SSH command: %w", err)
			}
		} else if stderr.buffer.Len() > 0 {
			err = fmt.Errorf("SSH command diagnostics: %s", strings.TrimSpace(stderr.buffer.String()))
		}
		ready <- result{output: stdout.buffer.Bytes(), err: err}
	}()
	select {
	case completed := <-ready:
		if err := ctx.Err(); err != nil {
			l.release(true)
			return nil, err
		}
		return completed.output, completed.err
	case <-ctx.Done():
		l.release(true)
		return nil, ctx.Err()
	case <-c.done:
		l.release(true)
		return nil, ErrClosed
	}
}
