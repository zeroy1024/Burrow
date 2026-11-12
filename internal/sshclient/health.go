package sshclient

import (
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// activityConn tracks SSH input, including replies and ordinary channel traffic.
// Elapsed times share a monotonic origin and do not depend on wall-clock changes.
type activityConn struct {
	net.Conn
	started  time.Time
	received atomic.Int64
}

func (c *activityConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.received.Store(time.Since(c.started).Nanoseconds())
	}
	return n, err
}

func (c *Connection) healthCheck(interval time.Duration) {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	var pending <-chan error
	lastReceived := c.activity.received.Load()
	missed := 0
	for {
		select {
		case <-c.done:
			return
		case <-c.ctx.Done():
			return
		case err := <-pending:
			pending = nil
			if err != nil {
				c.failTransport(fmt.Errorf("SSH health check: %w", err))
				return
			}
			missed = 0
			timer.Reset(interval)
		case <-timer.C:
			received := c.activity.received.Load()
			if received != lastReceived {
				lastReceived, missed = received, 0
			}
			idle := time.Since(c.activity.started) - time.Duration(received)
			if idle < interval {
				missed = 0
				timer.Reset(interval - idle)
				continue
			}
			// Count observed idle intervals, so waking from sleep does not
			// turn a single delayed timer into three failed checks.
			missed++
			if missed >= 3 {
				c.failTransport(errors.New("SSH health check timed out"))
				return
			}
			if pending == nil {
				// SendRequest cannot be cancelled independently of the transport.
				// Keep at most one pending request, even if the peer never replies.
				result := make(chan error, 1)
				pending = result
				c.workers.Go(func() {
					_, _, err := c.client.SendRequest("keepalive@"+"openssh.com", true, nil)
					result <- err
				})
			}
			timer.Reset(interval)
		}
	}
}
