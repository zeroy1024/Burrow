package sshclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

// A local pipe gives the SSH channel independent deadlines and lets Close
// unblock local I/O even when the remote server does not acknowledge channel
// close. The parent joins the bounded bridge workers on transport shutdown.
type tunnelConn struct {
	net.Conn
	bridge net.Conn
	remote net.Conn
	lease  *lease
	cancel context.CancelFunc
	once   sync.Once
}

func (t *tunnelConn) abort() {
	t.once.Do(func() {
		t.cancel()
		t.Conn.Close()
		t.bridge.Close()
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
			local, bridge := net.Pipe()
			tunnel = &tunnelConn{Conn: local, bridge: bridge, remote: conn, lease: l, cancel: cancel}
			if !l.bind(tunnel.abort) {
				tunnel.abort()
				tunnel, err = nil, ErrClosed
			} else {
				opened := tunnel
				c.workers.Go(func() {
					defer opened.Close()
					_, _ = io.Copy(opened.bridge, opened.remote)
				})
				c.workers.Go(func() {
					defer opened.Close()
					_, _ = io.Copy(opened.remote, opened.bridge)
				})
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
