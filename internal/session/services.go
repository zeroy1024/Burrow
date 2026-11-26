package session

import (
	"context"
	"net/netip"

	"github.com/zeroy1024/Burrow/internal/sshclient"
)

// ShellServices is shared by live shells on one authenticated transport.
// The final shell closes its listeners even if SFTP still holds the transport.
type ShellServices struct {
	transport *transport
	ctx       context.Context
	cancel    context.CancelFunc
	users     int
	forwards  map[*Forward]struct{}
}

func (t *transport) holdShell() *ShellServices {
	if t.services == nil {
		ctx, cancel := context.WithCancel(t.ctx)
		t.services = &ShellServices{transport: t, ctx: ctx, cancel: cancel, forwards: make(map[*Forward]struct{})}
	}
	t.services.users++
	return t.services
}

func (s *ShellServices) Context() context.Context          { return s.ctx }
func (s *ShellServices) Connection() *sshclient.Connection { return s.transport.conn }

func (s *ShellServices) release() {
	s.users--
	if s.users == 0 {
		s.cancel()
		for forward := range s.forwards {
			forward.Close()
		}
		s.transport.services = nil
	}
}

type Forward struct {
	owner    *ShellServices
	listener *sshclient.ForwardListener
	ctx      context.Context
	cancel   context.CancelFunc
}

// Listen starts one TCP forward, or a SOCKS proxy when target is empty.
// Call Listen and Close on the owner; Wait may block on a worker.
func (s *ShellServices) Listen(bind netip.AddrPort, target string, onError func(error)) (*Forward, error) {
	ctx, cancel := context.WithCancel(s.ctx)
	listener, err := s.Connection().ListenForward(ctx, bind, target, onError)
	if err != nil {
		cancel()
		return nil, err
	}
	f := &Forward{owner: s, listener: listener, ctx: ctx, cancel: cancel}
	s.forwards[f] = struct{}{}
	return f, nil
}

func (f *Forward) Context() context.Context { return f.ctx }
func (f *Forward) Wait() error              { return f.listener.Wait() }
func (f *Forward) Close() {
	delete(f.owner.forwards, f)
	f.cancel()
	f.listener.Close()
}
