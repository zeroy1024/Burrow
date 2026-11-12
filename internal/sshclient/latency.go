package sshclient

import (
	"errors"
	"time"
)

// TCPRTT reads the kernel's RTT estimate without sending SSH requests. Custom
// dialers, proxies and tunnels have no direct endpoint socket here and report
// errors.ErrUnsupported rather than presenting the carrier's RTT as the target's.
// An idle connection may retain an estimate from its last traffic.
func (c *Connection) TCPRTT() (time.Duration, error) {
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return 0, ErrClosed
	}
	c.mu.Unlock()
	if c.tcp == nil {
		return 0, errors.ErrUnsupported
	}
	return tcpRTT(c.tcp)
}
