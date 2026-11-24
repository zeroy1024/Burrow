package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// A local pipe gives the SSH channel independent deadlines and lets Close
// unblock local I/O even when the remote server does not acknowledge channel
// close. The parent joins the bounded bridge workers on transport shutdown.
type tunnelConn struct {
	reader, writer          net.Conn
	bridgeRead, bridgeWrite net.Conn
	remote                  net.Conn
	lease                   *lease
	cancel                  context.CancelFunc
	once                    sync.Once
}

func (t *tunnelConn) abort() {
	t.once.Do(func() {
		t.cancel()
		t.reader.Close()
		t.writer.Close()
		t.bridgeRead.Close()
		t.bridgeWrite.Close()
		t.lease.owner.workers.Go(func() { t.remote.Close() })
	})
}

func (t *tunnelConn) Close() error {
	t.abort()
	t.lease.release(false)
	return nil
}

func (t *tunnelConn) LocalAddr() net.Addr { return t.remote.LocalAddr() }

func (t *tunnelConn) RemoteAddr() net.Addr { return t.remote.RemoteAddr() }

func (t *tunnelConn) Read(b []byte) (int, error)  { return t.reader.Read(b) }
func (t *tunnelConn) Write(b []byte) (int, error) { return t.writer.Write(b) }
func (t *tunnelConn) CloseWrite() error           { return t.writer.Close() }

func (t *tunnelConn) SetDeadline(at time.Time) error {
	return errors.Join(t.SetReadDeadline(at), t.SetWriteDeadline(at))
}

func (t *tunnelConn) SetReadDeadline(at time.Time) error  { return t.reader.SetReadDeadline(at) }
func (t *tunnelConn) SetWriteDeadline(at time.Time) error { return t.writer.SetWriteDeadline(at) }

func (t *tunnelConn) relay() {
	defer t.Close()
	var sender sync.WaitGroup
	sender.Go(func() {
		if _, err := io.Copy(t.remote, t.bridgeRead); err != nil {
			t.Close()
			return
		}
		// An SSH TCP connection embeds its channel and supports EOF independently of Close.
		if err := t.remote.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
			t.Close()
		}
	})
	if _, err := io.Copy(t.bridgeWrite, t.remote); err != nil {
		t.Close()
	} else {
		t.bridgeWrite.Close()
	}
	sender.Wait()
}

// DialContext opens a TCP connection from the remote host for a jump-host
// handshake. Its context bounds establishment, as net.Dialer does; Close owns
// an established tunnel without closing siblings. A server that never answers
// channel creation keeps the single open
// slot busy until it replies or the parent transport ends.
func (c *Connection) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported SSH tunnel network %q", network)
	}
	lifetime, cancel := context.WithCancel(c.ctx)
	stopSetup := context.AfterFunc(ctx, cancel)
	keep := false
	defer func() {
		stopSetup()
		if !keep {
			cancel()
		}
	}()
	l, err := c.acquireOpen(lifetime)
	if err != nil {
		return nil, err
	}
	type result struct {
		conn net.Conn
		err  error
	}
	ready := make(chan result, 1)
	go func() {
		defer c.workers.Done()
		conn, err := c.client.Dial(network, address)
		var tunnel *tunnelConn
		if err == nil {
			// A separate pipe per direction preserves TCP half-closes while keeping local deadlines.
			reader, bridgeWrite := net.Pipe()
			bridgeRead, writer := net.Pipe()
			tunnel = &tunnelConn{
				reader: reader, writer: writer, bridgeRead: bridgeRead, bridgeWrite: bridgeWrite,
				remote: conn, lease: l, cancel: cancel,
			}
			if !l.bind(tunnel.abort) {
				tunnel.abort()
				tunnel, err = nil, ErrClosed
			} else {
				c.workers.Go(tunnel.relay)
			}
		} else {
			err = fmt.Errorf("open SSH tunnel: %w", err)
		}
		if err != nil {
			l.release(true)
		}
		c.endOpen()
		if tunnel == nil {
			ready <- result{err: err}
			return
		}
		ready <- result{conn: tunnel}
	}()
	select {
	case opened := <-ready:
		stopSetup()
		if err := ctx.Err(); err != nil {
			l.release(true)
			return nil, err
		}
		if lifetime.Err() != nil {
			l.release(true)
			return nil, ErrClosed
		}
		keep = opened.err == nil
		return opened.conn, opened.err
	case <-ctx.Done():
		l.release(true)
		return nil, ctx.Err()
	case <-c.done:
		l.release(true)
		return nil, ErrClosed
	}
}
