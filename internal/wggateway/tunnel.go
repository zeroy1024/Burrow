package wggateway

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"sshdesk/internal/locales"
)

type Tunnel struct {
	device *device.Device
	net    *netstack.Net
	config Config
	peer   *device.Peer
	stop   func() bool
}

func Open(ctx context.Context, config Config, credential Credential) (*Tunnel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.CredentialRevision == "" || credential.Revision != config.CredentialRevision {
		return nil, locales.NewError(locales.WgCredentialChanged)
	}
	public, err := PublicKey(credential.PrivateKey)
	if err != nil {
		return nil, err
	}
	if public != config.ClientPublicKey {
		return nil, locales.NewError(locales.WgKeyMismatch)
	}
	private, err := DecodeKey(credential.PrivateKey)
	if err != nil {
		return nil, err
	}
	defer clear(private)
	peer, err := DecodeKey(config.PublicKey)
	if err != nil {
		return nil, err
	}
	defer clear(peer)
	var preshared []byte
	if credential.PresharedKey != "" {
		preshared, err = DecodeKey(credential.PresharedKey)
		if err != nil {
			return nil, err
		}
		defer clear(preshared)
	}
	host, port, _ := net.SplitHostPort(config.Endpoint)
	setup, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoints, err := net.DefaultResolver.LookupNetIP(setup, "ip", host)
	if err != nil {
		return nil, locales.Errorf(locales.WgSetup, err)
	}
	if len(endpoints) == 0 {
		return nil, locales.NewError(locales.WgInvalidEndpoint)
	}
	addresses := make([]netip.Addr, 0, len(config.Addresses))
	for _, value := range config.Addresses {
		prefix, _ := netip.ParsePrefix(value)
		addresses = append(addresses, prefix.Addr())
	}
	dns := make([]netip.Addr, 0, len(config.DNS))
	for _, value := range config.DNS {
		address, _ := netip.ParseAddr(value)
		dns = append(dns, address)
	}
	// CreateNetTUN allocates only a gVisor stack. Do not replace this with
	// tun.CreateTUN, wg-quick, or platform network configuration APIs.
	tun, network, err := netstack.CreateNetTUN(addresses, dns, config.MTU)
	if err != nil {
		return nil, locales.Errorf(locales.WgSetup, err)
	}
	wg := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	var ipc bytes.Buffer
	fmt.Fprintf(&ipc, "private_key=%s\nlisten_port=%d\nreplace_peers=true\npublic_key=%s\nendpoint=%s\npersistent_keepalive_interval=%d\n",
		hex.EncodeToString(private), config.ListenPort, hex.EncodeToString(peer),
		net.JoinHostPort(endpoints[0].Unmap().String(), port), config.Keepalive)
	if len(preshared) > 0 {
		fmt.Fprintf(&ipc, "preshared_key=%s\n", hex.EncodeToString(preshared))
	}
	for _, value := range config.AllowedIPs {
		prefix, _ := netip.ParsePrefix(value)
		fmt.Fprintf(&ipc, "allowed_ip=%s\n", prefix.Masked())
	}
	err = wg.IpcSetOperation(bytes.NewReader(ipc.Bytes()))
	clear(ipc.Bytes())
	if err == nil {
		err = wg.Up()
	}
	if err != nil {
		wg.Close()
		return nil, locales.Errorf(locales.WgSetup, err)
	}
	var publicKey device.NoisePublicKey
	copy(publicKey[:], peer)
	t := &Tunnel{device: wg, net: network, config: config, peer: wg.LookupPeer(publicKey)}
	t.stop = context.AfterFunc(ctx, wg.Close)
	return t, nil
}

func (t *Tunnel) Done() <-chan struct{} { return t.device.Wait() }

func (t *Tunnel) Close() {
	t.stop()
	t.device.Close()
}

func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, locales.Errorf(locales.WgUnsupportedNetwork, network)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if _, err := netip.ParseAddr(host); err != nil && len(t.config.DNS) == 0 {
		return nil, locales.NewError(locales.WgNoDNS)
	}
	// netstack appends the DNS root label itself.
	resolved, err := t.net.LookupContextHost(ctx, strings.TrimSuffix(host, "."))
	if err != nil {
		return nil, locales.Errorf(locales.WgDial, err)
	}
	var failures []error
	for index, value := range resolved {
		ip, err := netip.ParseAddr(value)
		if err != nil || !t.config.Allows(ip) || network == "tcp4" && !ip.Is4() || network == "tcp6" && !ip.Is6() {
			continue
		}
		attempt, cancel := context.WithCancel(ctx)
		if deadline, ok := ctx.Deadline(); ok {
			cancel()
			attempt, cancel = context.WithTimeout(ctx, time.Until(deadline)/time.Duration(len(resolved)-index))
		}
		connection, err := t.net.DialContext(attempt, network, net.JoinHostPort(ip.String(), port))
		cancel()
		if err == nil {
			return connection, nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return nil, locales.NewError(locales.WgTargetOutsideAllowedIPs)
	}
	return nil, locales.Errorf(locales.WgDial, errors.Join(failures...))
}

// Probe verifies an authenticated handshake, independently of any target
// service. Reuse a handshake completed within the protocol's five-second
// initiation limit; otherwise request one and wait without owning the tunnel.
func (t *Tunnel) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	previous, err := t.lastHandshake()
	if err != nil {
		return err
	}
	if !previous.IsZero() && time.Since(previous) < 5*time.Second {
		return nil
	}
	if err := t.peer.SendHandshakeInitiation(false); err != nil {
		return locales.Errorf(locales.WgHandshake, err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return locales.Errorf(locales.WgHandshake, ctx.Err())
		case <-t.Done():
			return locales.NewError(locales.WgRuntimeClosed)
		case <-ticker.C:
			current, err := t.lastHandshake()
			if err != nil {
				return err
			}
			if current.After(previous) {
				return nil
			}
		}
	}
}

// IpcGetOperation also emits private keys. Consume only handshake counters;
// never retain or log its raw output, or expose it through MCP snapshots.
type handshakeReader struct{ seconds, nanos int64 }

func (s *handshakeReader) Write(data []byte) (int, error) {
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		key, value, ok := bytes.Cut(line, []byte{'='})
		if !ok {
			continue
		}
		switch string(key) {
		case "last_handshake_time_sec":
			s.seconds, _ = strconv.ParseInt(string(value), 10, 64)
		case "last_handshake_time_nsec":
			s.nanos, _ = strconv.ParseInt(string(value), 10, 64)
		}
	}
	return len(data), nil
}

func (t *Tunnel) lastHandshake() (time.Time, error) {
	var state handshakeReader
	if err := t.device.IpcGetOperation(&state); err != nil {
		return time.Time{}, locales.Errorf(locales.WgHandshake, err)
	}
	if state.seconds == 0 {
		return time.Time{}, nil
	}
	return time.Unix(state.seconds, state.nanos), nil
}
