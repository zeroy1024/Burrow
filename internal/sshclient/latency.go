package sshclient

import (
	"context"
	"time"
)

type latencyProbe struct {
	done    chan struct{}
	elapsed time.Duration
	err     error
}

// ProbeLatency measures an SSH global request on the existing transport. Both
// success and unsupported-request replies count as responses. Concurrent probes
// and automatic keepalives share one request; caller cancellation only stops
// waiting, so a missing reply cannot accumulate workers or close sibling channels.
func (c *Connection) ProbeLatency(ctx context.Context) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return 0, ErrClosed
	}
	probe := c.probe
	if probe == nil {
		probe = &latencyProbe{done: make(chan struct{})}
		c.probe = probe
		// Register under the same lock that closes the transport before Wait.
		c.workers.Go(func() {
			started := time.Now()
			_, _, probe.err = c.client.SendRequest("keepalive@"+"openssh.com", true, nil)
			probe.elapsed = time.Since(started)
			c.mu.Lock()
			c.probe = nil
			close(probe.done)
			c.mu.Unlock()
		})
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-c.done:
		return 0, ErrClosed
	case <-probe.done:
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return probe.elapsed, probe.err
	}
}
