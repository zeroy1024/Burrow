package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/net/idna"
)

const forwardSetupTimeout = 5 * time.Second
const maxForwardClients = 64

var ErrInvalidForwardTarget = errors.New("invalid forwarding target")

// ForwardTarget validates a destination without resolving it on the client.
func ForwardTarget(host string, port int) (string, error) {
	host = strings.TrimSpace(host)
	if port < 1 || port > 65535 || host == "" || strings.ContainsAny(host, "/\\@?#") ||
		strings.IndexFunc(host, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return "", ErrInvalidForwardTarget
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if _, err := netip.ParseAddr(host); err != nil {
		domain, err := idna.Lookup.ToASCII(host)
		if err != nil || domain == "" || len(domain) > 253 || strings.ContainsAny(domain, ":[]") {
			return "", ErrInvalidForwardTarget
		}
		host = domain
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// ForwardListener owns a local listener and its active forwarding streams.
type ForwardListener struct {
	listener *net.TCPListener
	cancel   context.CancelFunc
	done     chan struct{}
	err      error
}

// ListenForward listens on the supplied local IP and opens destinations through this SSH transport.
// An empty target serves SOCKS5 CONNECT. Otherwise target must come from ForwardTarget.
func (c *Connection) ListenForward(ctx context.Context, bind netip.AddrPort, target string, destinationError func(error)) (*ForwardListener, error) {
	if !bind.IsValid() || bind.Port() < 1024 {
		return nil, fmt.Errorf("invalid local forwarding address %s", bind)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := c.Retain()
	if err != nil {
		return nil, err
	}
	ip := bind.Addr().Unmap()
	network := "tcp6"
	if ip.Is4() {
		network = "tcp4"
	}
	listener, err := net.ListenTCP(network, net.TCPAddrFromAddrPort(netip.AddrPortFrom(ip, bind.Port())))
	if err != nil {
		release()
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	forward := &ForwardListener{listener: listener, cancel: cancel, done: make(chan struct{})}
	stopOnDisconnect := context.AfterFunc(c.ctx, cancel)
	go func() {
		defer close(forward.done)
		defer release()
		defer stopOnDisconnect()
		forward.err = forward.serve(lifetime, c, target, destinationError)
	}()
	return forward, nil
}

func (f *ForwardListener) Close() {
	f.cancel()
	f.listener.Close()
}

func (f *ForwardListener) Wait() error {
	<-f.done
	return f.err
}

func (f *ForwardListener) serve(ctx context.Context, c *Connection, target string, destinationError func(error)) error {
	stop := context.AfterFunc(ctx, func() { f.listener.Close() })
	defer stop()
	var clients sync.WaitGroup
	defer func() {
		f.Close()
		clients.Wait()
	}()
	slots := make(chan struct{}, maxForwardClients)
	for {
		client, err := f.listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			clients.Go(func() {
				defer func() { <-slots }()
				c.serveForwardClient(ctx, client, target, destinationError)
			})
		default:
			client.Close()
		}
	}
}

func (c *Connection) serveForwardClient(ctx context.Context, client *net.TCPConn, target string, destinationError func(error)) {
	defer client.Close()
	stopLocal := context.AfterFunc(ctx, func() { client.Close() })
	defer stopLocal()
	socks := target == ""
	if socks {
		if err := client.SetDeadline(time.Now().Add(forwardSetupTimeout)); err != nil {
			return
		}
		var err error
		target, err = readSOCKSTarget(client)
		if err != nil {
			return
		}
	}
	setup, cancel := context.WithTimeout(ctx, forwardSetupTimeout)
	remote, err := c.DialContext(setup, "tcp", target)
	cancel()
	if err != nil {
		if socks {
			if client.SetWriteDeadline(time.Now().Add(forwardSetupTimeout)) == nil {
				writeSOCKSReply(client, socksFailure)
			}
		}
		if destinationError != nil && ctx.Err() == nil {
			destinationError(err)
		}
		return
	}
	defer remote.Close()
	stopRemote := context.AfterFunc(ctx, func() { remote.Close() })
	defer stopRemote()
	if socks {
		if err := client.SetDeadline(time.Now().Add(forwardSetupTimeout)); err != nil {
			return
		}
		if err := writeSOCKSReply(client, socksGranted); err != nil {
			return
		}
		if err := client.SetDeadline(time.Time{}); err != nil {
			return
		}
	}
	relayForward(client, remote.(*tunnelConn))
}

type halfCloseConn interface {
	net.Conn
	CloseWrite() error
}

func relayForward(left, right halfCloseConn) {
	copyTo := func(dst, src halfCloseConn) {
		if _, err := io.Copy(dst, src); err == nil {
			if err := dst.CloseWrite(); err == nil {
				return
			}
		}
		left.Close()
		right.Close()
	}
	var sender sync.WaitGroup
	sender.Go(func() { copyTo(right, left) })
	copyTo(left, right)
	sender.Wait()
}
