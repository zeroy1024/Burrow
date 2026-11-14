//go:build integration

package wggateway_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/wggateway"

	"golang.org/x/crypto/ssh"
)

type gatewayFixture struct {
	Target    struct{ Address string }
	WireGuard struct{ Config, Tunnel string }
}

// wireGuardFixture loads the disposable peer started by
// scripts/ssh-integration-server.py --gateways. The SSH target is a Docker
// bridge address that the host cannot reach without the tunnel.
func wireGuardFixture(t *testing.T) (gatewayFixture, wggateway.Config, wggateway.Credential) {
	t.Helper()
	directory := os.Getenv("BURROW_TEST_DIR")
	if directory == "" {
		t.Skip("BURROW_TEST_DIR is required for the Docker WireGuard fixture")
	}
	data, err := os.ReadFile(filepath.Join(directory, "gateway-server.json"))
	if os.IsNotExist(err) {
		t.Skip("start the disposable gateway fixtures: gateway-server.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixture gatewayFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(fixture.WireGuard.Config)
	if err != nil {
		t.Fatal(err)
	}
	config, credential, err := wggateway.Parse(source)
	if err != nil {
		t.Fatalf("Parse fixture configuration: %v", err)
	}
	host, _, err := net.SplitHostPort(config.Endpoint)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("WireGuard fixture endpoint must be a loopback mapping: %q", config.Endpoint)
	}
	config.CredentialRevision, credential.Revision = "fixture", "fixture"
	return fixture, config, credential
}

func sshBanner(t *testing.T, tunnel *wggateway.Tunnel, address string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := tunnel.DialContext(ctx, "tcp", net.JoinHostPort(address, "22"))
	if err != nil {
		t.Fatalf("DialContext %s: %v", address, err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	banner, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		t.Fatalf("read SSH banner through tunnel: %v", err)
	}
	return banner
}

func TestWireGuardIntegrationReachesBridgeTarget(t *testing.T) {
	fixture, config, credential := wireGuardFixture(t)
	if direct, err := net.DialTimeout("tcp", net.JoinHostPort(fixture.Target.Address, "22"), time.Second); err == nil {
		direct.Close()
		t.Fatalf("target %s is reachable without the tunnel; the fixture proves nothing", fixture.Target.Address)
	}
	tunnel, err := wggateway.Open(t.Context(), config, credential)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tunnel.Close()
	if err := tunnel.Probe(t.Context()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if banner := sshBanner(t, tunnel, fixture.Target.Address); !strings.HasPrefix(banner, "SSH-2.0-") {
		t.Fatalf("unexpected banner through tunnel: %q", banner)
	}
	// Two streams share one tunnel without a second handshake requirement.
	if banner := sshBanner(t, tunnel, fixture.Target.Address); !strings.HasPrefix(banner, "SSH-2.0-") {
		t.Fatalf("unexpected banner on second stream: %q", banner)
	}
}

func TestWireGuardIntegrationEnforcesAllowedIPsAndNetworks(t *testing.T) {
	fixture, config, credential := wireGuardFixture(t)
	// Narrow the client to the tunnel network only.
	config.AllowedIPs = []string{"10.99.0.0/24"}
	tunnel, err := wggateway.Open(t.Context(), config, credential)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer tunnel.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if connection, err := tunnel.DialContext(ctx, "tcp", net.JoinHostPort(fixture.Target.Address, "22")); err == nil {
		connection.Close()
		t.Fatal("dialed a target outside AllowedIPs")
	}
	if connection, err := tunnel.DialContext(ctx, "udp", net.JoinHostPort(fixture.WireGuard.Tunnel, "53")); err == nil {
		connection.Close()
		t.Fatal("dialed a non-TCP network")
	}
	if connection, err := tunnel.DialContext(ctx, "tcp", "fixture.invalid:22"); err == nil {
		connection.Close()
		t.Fatal("resolved a host name without configured DNS")
	}
}

func TestWireGuardIntegrationRejectsUnknownPeerAndStaleCredential(t *testing.T) {
	_, config, credential := wireGuardFixture(t)

	stale := credential
	stale.Revision = "previous"
	if tunnel, err := wggateway.Open(t.Context(), config, stale); err == nil {
		tunnel.Close()
		t.Fatal("opened a tunnel with a stale credential revision")
	}

	other, err := wggateway.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	mismatched := credential
	mismatched.PrivateKey = other
	if tunnel, err := wggateway.Open(t.Context(), config, mismatched); err == nil {
		tunnel.Close()
		t.Fatal("opened a tunnel whose private key does not match the stored public key")
	}

	// A self-consistent key the peer has never seen must not complete a handshake.
	unknown := config
	unknown.ClientPublicKey, err = wggateway.PublicKey(other)
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := wggateway.Open(t.Context(), unknown, mismatched)
	if err != nil {
		t.Fatalf("Open with unknown key: %v", err)
	}
	defer tunnel.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := tunnel.Probe(ctx); err == nil {
		t.Fatal("handshake completed for a key the peer does not list")
	}
}

func TestWireGuardIntegrationClosedTunnelStopsDialing(t *testing.T) {
	fixture, config, credential := wireGuardFixture(t)
	tunnel, err := wggateway.Open(t.Context(), config, credential)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := tunnel.Probe(t.Context()); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	tunnel.Close()
	select {
	case <-tunnel.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done did not report the closed tunnel")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if connection, err := tunnel.DialContext(ctx, "tcp", net.JoinHostPort(fixture.Target.Address, strconv.Itoa(22))); err == nil {
		connection.Close()
		t.Fatal("dialed through a closed tunnel")
	}
}

// peerConnections counts the SSH connections the target server still holds
// from the WireGuard peer, observed over an independent direct connection.
func peerConnections(t *testing.T) int {
	t.Helper()
	address, directory := os.Getenv("BURROW_TEST_ADDRESS"), os.Getenv("BURROW_TEST_DIR")
	if address == "" {
		t.Skip("BURROW_TEST_ADDRESS is required for the Docker SSH fixture")
	}
	data, err := os.ReadFile(filepath.Join(directory, "gateway-server.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ WireGuard struct{ Internal string } }
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	peer := net.ParseIP(fixture.WireGuard.Internal).To4()
	if peer == nil {
		t.Fatalf("WireGuard peer address %q is not IPv4", fixture.WireGuard.Internal)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	// The disposable loopback fixture has no prior identity to compare with.
	connection, err := sshclient.Dial(ctx, sshclient.Config{
		Host: host, Port: port, User: "burrow", Auth: "key",
		KeyPath: filepath.Join(directory, "key_plain"), HostKey: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("connect fixture: %v", err)
	}
	defer connection.Close()
	// /proc/net/tcp prints addresses as little-endian hexadecimal words.
	remote := fmt.Sprintf("%02X%02X%02X%02X", peer[3], peer[2], peer[1], peer[0])
	output, err := connection.Exec(ctx, `awk '$2 ~ /:0016$/ && $4 == "01" && $3 ~ /^`+remote+`:/ { n++ } END { print n+0 }' /proc/net/tcp`)
	if err != nil {
		t.Fatalf("count server connections: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("unexpected connection count %q", output)
	}
	return count
}

func TestWireGuardIntegrationCloseReachesPeerBeforeTunnelCloses(t *testing.T) {
	fixture, config, credential := wireGuardFixture(t)
	before := peerConnections(t)
	// Close the stream and the tunnel back to back, as releasing the last
	// connection of a gateway does.
	for range 5 {
		tunnel, err := wggateway.Open(t.Context(), config, credential)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		connection, err := tunnel.DialContext(ctx, "tcp", net.JoinHostPort(fixture.Target.Address, "22"))
		cancel()
		if err != nil {
			tunnel.Close()
			t.Fatalf("DialContext: %v", err)
		}
		if _, err := bufio.NewReader(connection).ReadString('\n'); err != nil {
			t.Fatalf("read SSH banner: %v", err)
		}
		if err := connection.Close(); err != nil {
			t.Fatalf("Close stream: %v", err)
		}
		tunnel.Close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		left := peerConnections(t) - before
		if left <= 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server still holds %d connection(s) whose tunnel was closed", left)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
