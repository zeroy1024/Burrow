// Package sshclient manages authenticated SSH connections and their independent channels.
package sshclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	ErrClosed         = errors.New("SSH connection is closed")
	ErrChannelPending = errors.New("an SSH channel is still being created")
)

// Config describes one authenticated connection. Credentials are not retained after Dial.
type Config struct {
	Host       string
	Port       int
	User       string
	Auth       string // "agent", "password", or "key"
	Password   string
	KeyPath    string
	Passphrase string
	HostKey    ssh.HostKeyCallback
}

// Connection owns a transport. Each channel or temporary Retain holds a lease;
// once ownership has begun, releasing the final lease closes the transport.
type Connection struct {
	client      *ssh.Client
	raw         net.Conn
	ctx         context.Context
	stopCancel  func() bool
	mu          sync.Mutex
	leases      map[*lease]struct{}
	started     bool
	opening     bool
	openingDone chan struct{}
	closed      bool
	localClose  bool
	workers     sync.WaitGroup
	done        chan struct{}
	waitErr     error
	closeOnce   sync.Once
	closeErr    error
}

type lease struct {
	owner *Connection
	done  chan struct{}
	once  sync.Once
	abort func() // read and set under owner.mu
}

// Dial only authenticates. The caller must Close an unused connection; an
// initial zero-lease connection remains open until its first consumer starts.
func Dial(ctx context.Context, cfg Config) (*Connection, error) {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.User) == "" {
		return nil, errors.New("SSH host and user must not be empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("SSH port must be between 1 and 65535")
	}
	if cfg.HostKey == nil {
		return nil, errors.New("SSH host key verification is required")
	}
	auth, agentConn, err := authentication(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if agentConn != nil {
		defer agentConn.Close()
		stopAgent := context.AfterFunc(ctx, func() { agentConn.Close() })
		defer stopAgent()
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	raw, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect SSH: %w", err)
	}
	stopCancel := context.AfterFunc(ctx, func() { raw.Close() })
	success := false
	defer func() {
		if !success {
			stopCancel()
			raw.Close()
		}
	}()
	// Host-key confirmation and hardware-backed signing may require interaction.
	if err := raw.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		return nil, fmt.Errorf("set SSH setup deadline: %w", err)
	}
	transport, channels, requests, err := ssh.NewClientConn(raw, address, &ssh.ClientConfig{
		User: cfg.User, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: cfg.HostKey,
	})
	if err != nil {
		return nil, fmt.Errorf("authenticate SSH: %w", err)
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("clear SSH setup deadline: %w", err)
	}
	c := &Connection{
		client: ssh.NewClient(transport, channels, requests), raw: raw, ctx: ctx,
		stopCancel: stopCancel, leases: make(map[*lease]struct{}), done: make(chan struct{}),
	}
	go c.watch()
	success = true
	return c, nil
}

// Retain protects an intent while its owner schedules a channel-opening worker.
// Its release is idempotent and must be called on success, failure, or cancellation.
func (c *Connection) Retain() (func(), error) {
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	l := c.addLeaseLocked()
	c.mu.Unlock()
	return func() { l.release(false) }, nil
}

func (c *Connection) addLeaseLocked() *lease {
	l := &lease{owner: c, done: make(chan struct{})}
	c.leases[l], c.started = struct{}{}, true
	return l
}

func (c *Connection) beginOpen(ctx context.Context) (*lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.ctx.Err() != nil {
		return nil, ErrClosed
	}
	if c.opening {
		return nil, ErrChannelPending
	}
	l := c.addLeaseLocked()
	c.opening = true
	c.openingDone = make(chan struct{})
	// Register workers before transport completion can make Wait return.
	c.workers.Add(2)
	go func() {
		defer c.workers.Done()
		select {
		case <-ctx.Done():
			l.release(true)
		case <-l.done:
		}
	}()
	return l, nil
}

// WaitChannelReady waits for the current channel creation to finish. It does
// not reserve the next slot; callers that schedule several intents serialize them.
func (c *Connection) WaitChannelReady(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed || c.ctx.Err() != nil {
		c.mu.Unlock()
		return ErrClosed
	}
	done := c.openingDone
	c.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	case <-done:
		c.mu.Lock()
		closed := c.closed || c.ctx.Err() != nil
		c.mu.Unlock()
		if closed {
			return ErrClosed
		}
		return nil
	}
}

func (l *lease) bind(abort func()) bool {
	c := l.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.leases[l]; !ok || c.closed {
		return false
	}
	l.abort = abort
	return true
}

func (l *lease) release(abort bool) {
	l.once.Do(func() {
		c := l.owner
		c.mu.Lock()
		delete(c.leases, l)
		cleanup := l.abort
		last := c.started && len(c.leases) == 0 && !c.closed
		if last {
			c.closed = true
		}
		c.mu.Unlock()
		close(l.done)
		if abort && cleanup != nil {
			cleanup()
		}
		if last {
			c.Close()
		}
	})
}

func (c *Connection) watch() {
	err := c.client.Wait()
	c.mu.Lock()
	c.closed = true
	if c.ctx.Err() != nil {
		err = c.ctx.Err()
	} else if c.localClose {
		err = nil
	}
	c.waitErr = err
	leases := make([]*lease, 0, len(c.leases))
	for l := range c.leases {
		leases = append(leases, l)
	}
	c.mu.Unlock()
	c.stopCancel()
	for _, l := range leases {
		l.release(true)
	}
	close(c.done)
}

// Done closes when the transport has ended. Wait also joins channel workers.
func (c *Connection) Done() <-chan struct{} { return c.done }

func (c *Connection) Wait() error {
	<-c.done
	c.workers.Wait()
	return c.waitErr
}

// Close stops the entire transport. Child handles use their own Close instead.
func (c *Connection) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed, c.localClose = true, true
		leases := make([]*lease, 0, len(c.leases))
		for l := range c.leases {
			leases = append(leases, l)
		}
		c.mu.Unlock()
		c.stopCancel()
		c.closeErr = c.raw.Close()
		if errors.Is(c.closeErr, net.ErrClosed) {
			c.closeErr = nil
		}
		for _, l := range leases {
			l.release(true)
		}
	})
	return c.closeErr
}
