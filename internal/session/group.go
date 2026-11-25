// Package session coordinates Burrow's running SSH connections and channels.
package session

import (
	"context"
	"errors"
	"iter"
	"slices"

	"github.com/zeroy1024/Burrow/internal/sshclient"
)

// Group owns an authenticated transport and serializes channel-opening requests.
// Its zero value is ready for use. The caller serializes group access, request
// completion, and cancellation on one owner (for example, an event loop).
type Group struct {
	transport  *transport
	lifetime   context.Context
	generation int
	dial       *Dial
	queue      []*Request
	opening    *Request
}

type transport struct {
	conn     *sshclient.Connection
	ctx      context.Context
	services *ShellServices
}

func (g *Group) Connection() *sshclient.Connection {
	if g.transport == nil {
		return nil
	}
	return g.transport.conn
}
func (g *Group) Lifetime() context.Context { return g.lifetime }
func (g *Group) Generation() int           { return g.generation }
func (g *Group) Connecting() bool          { return g.dial != nil }
func (g *Group) Opening() *Request         { return g.opening }
func (g *Group) QueueLength() int          { return len(g.queue) }

// Waiting yields the contexts currently waiting for shared authentication.
func (g *Group) Waiting() iter.Seq[context.Context] {
	return func(yield func(context.Context) bool) {
		if g.dial != nil {
			for _, r := range g.dial.waiters {
				if !yield(r.Context) {
					return
				}
			}
		}
	}
}

// Request reserves one turn to start a channel. Ready and Failed run on the owner.
type Request struct {
	Context context.Context
	Ready   func(*sshclient.Connection)
	Failed  func(error)

	group     *Group
	transport *transport
	release   func()
}

// Acquire reuses the current connection or joins a dial. The first waiter
// receives the Dial and must run it on a worker. A request is submitted once.
func (g *Group) Acquire(parent context.Context, r *Request) *Dial {
	if r.Context.Err() != nil {
		return nil
	}
	r.group = g
	if g.transport != nil {
		err := g.reserve(r, g.transport)
		if err == nil {
			g.next()
			return nil
		}
		if !errors.Is(err, sshclient.ErrClosed) {
			r.Failed(err)
			return nil
		}
		g.transport = nil
	}
	if g.dial != nil && g.dial.ctx.Err() == nil {
		g.dial.waiters = append(g.dial.waiters, r)
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	g.generation++
	g.lifetime = ctx
	g.dial = &Dial{group: g, ctx: ctx, cancel: cancel, waiters: []*Request{r}}
	return g.dial
}

func (g *Group) reserve(r *Request, t *transport) error {
	release, err := t.conn.Retain()
	if err != nil {
		return err
	}
	r.release, r.transport = release, t
	g.queue = append(g.queue, r)
	return nil
}

func (g *Group) next() {
	if g.opening != nil {
		return
	}
	for len(g.queue) > 0 {
		r := g.queue[0]
		g.queue[0] = nil
		g.queue = g.queue[1:]
		if r.Context.Err() != nil {
			r.releaseLease()
			continue
		}
		g.opening = r
		r.Ready(r.transport.conn)
		return
	}
}

func (r *Request) releaseLease() {
	if r.release != nil {
		r.release()
		r.release = nil
	}
}

// Complete releases the setup reservation and lets the next consumer open a
// channel. An established channel has its own independent sshclient lease.
func (r *Request) Complete() {
	if r == nil || r.group == nil {
		return
	}
	r.releaseLease()
	if g := r.group; g.opening == r {
		g.opening = nil
		g.next()
	}
}

// Cancel withdraws the request after its context is canceled. An in-flight
// opener keeps its turn until Complete, even if its consumer has disappeared.
func (r *Request) Cancel() {
	if r == nil || r.group == nil {
		return
	}
	r.releaseLease()
	g := r.group
	if d := g.dial; d != nil {
		if i := slices.Index(d.waiters, r); i >= 0 {
			d.waiters = slices.Delete(d.waiters, i, i+1)
			if len(d.waiters) == 0 {
				d.cancel()
			}
		}
	}
	if i := slices.Index(g.queue, r); i >= 0 {
		g.queue = slices.Delete(g.queue, i, i+1)
	}
}

// Dial is one generation of authentication, shared by its waiters.
type Dial struct {
	group   *Group
	ctx     context.Context
	cancel  context.CancelFunc
	waiters []*Request
}

// Run dials and waits on a worker. post schedules callbacks in order on the
// group's owner. A late completion can never replace a newer transport.
func (d *Dial) Run(connect func(context.Context) (*sshclient.Connection, error), post func(func())) {
	conn, err := connect(d.ctx)
	post(func() { d.publish(conn, err) })
	if conn != nil {
		conn.Wait()
		d.cancel()
		post(func() {
			if d.group.Connection() == conn {
				d.group.transport = nil
			}
		})
	}
}

func (d *Dial) publish(conn *sshclient.Connection, err error) {
	g := d.group
	waiters := d.waiters
	d.waiters = nil
	if g.dial == d {
		g.dial = nil
	}
	if canceled := d.ctx.Err(); canceled != nil {
		err = canceled
	}
	var guard func()
	if err == nil {
		// Reserve the transport until every live waiter has its own lease.
		guard, err = conn.Retain()
	}
	if err != nil {
		d.cancel()
		if conn != nil {
			conn.Close()
		}
		for _, r := range waiters {
			if r.Context.Err() == nil {
				r.Failed(err)
			}
		}
		return
	}
	defer guard()
	g.transport = &transport{conn: conn, ctx: d.ctx}
	for _, r := range waiters {
		if r.Context.Err() == nil {
			if err := g.reserve(r, g.transport); err != nil {
				r.Failed(err)
			}
		}
	}
	g.next()
}
