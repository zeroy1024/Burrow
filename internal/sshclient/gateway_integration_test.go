//go:build integration

package sshclient_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"

	"golang.org/x/net/proxy"
)

type gatewayEndpoint struct{ Address, Internal, User string }

type gatewayFixture struct {
	integrationFixture
	Target      struct{ Address string }
	Jump, SOCKS gatewayEndpoint
}

// gatewayFixtures loads the gateways started by
// scripts/ssh-integration-server.py --gateways. Routes end at the SSH
// fixture's Docker bridge address, which the host cannot reach directly.
func gatewayFixtures(t *testing.T) gatewayFixture {
	t.Helper()
	fixture := gatewayFixture{integrationFixture: sshFixture(t)}
	data, err := os.ReadFile(filepath.Join(fixture.directory, "gateway-server.json"))
	if os.IsNotExist(err) {
		t.Skip("start the disposable gateway fixtures: gateway-server.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if direct, err := net.DialTimeout("tcp", net.JoinHostPort(fixture.Target.Address, "22"), time.Second); err == nil {
		direct.Close()
		t.Fatalf("target %s is reachable without a gateway; the fixture proves nothing", fixture.Target.Address)
	}
	return fixture
}

func (f gatewayFixture) jumpConfig(t *testing.T) sshclient.Config {
	t.Helper()
	host, portText, err := net.SplitHostPort(f.Jump.Address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	config := f.config
	config.Host, config.Port = host, port
	return config
}

func (f gatewayFixture) targetConfig(dial func(context.Context, string, string) (net.Conn, error)) sshclient.Config {
	config := f.config
	config.Host, config.Port, config.DialContext = f.Target.Address, 22, dial
	return config
}

// expectPeer asserts the address the target server saw as the client.
func expectPeer(t *testing.T, session *integrationSession, address string) {
	t.Helper()
	session.command(t, "printf '\\nPEER=%s=END\\n' \"${SSH_CONNECTION%% *}\"")
	session.awaitText(t, "PEER="+address+"=END")
}

func socksDialer(t *testing.T, fixture gatewayFixture, address, password string, forward proxy.Dialer) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	dialer, err := proxy.SOCKS5("tcp", address, &proxy.Auth{User: fixture.SOCKS.User, Password: password}, forward)
	if err != nil {
		t.Fatal(err)
	}
	return dialer.(proxy.ContextDialer).DialContext
}

type contextForward func(context.Context, string, string) (net.Conn, error)

func (f contextForward) Dial(network, address string) (net.Conn, error) {
	return f(context.Background(), network, address)
}

func (f contextForward) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

func TestGatewayIntegrationSSHJump(t *testing.T) {
	fixture := gatewayFixtures(t)
	jump := dialIntegrationConnection(t, fixture.jumpConfig(t))
	target := dialIntegrationConnection(t, fixture.targetConfig(jump.DialContext))
	expectPeer(t, openSharedIntegrationSession(t, target), fixture.Jump.Internal)

	// A second target transport shares the jump connection, and closing one
	// tunnel leaves both the jump host and its sibling usable.
	sibling := dialIntegrationConnection(t, fixture.targetConfig(jump.DialContext))
	target.Close()
	expectPeer(t, openSharedIntegrationSession(t, sibling), fixture.Jump.Internal)
	direct := openSharedIntegrationSession(t, jump)
	direct.command(t, "printf '\\n%s\\n' JUMP_STILL_USABLE")
	direct.awaitText(t, "JUMP_STILL_USABLE")
}

func TestGatewayIntegrationJumpRefusedTargetReportsFailure(t *testing.T) {
	fixture := gatewayFixtures(t)
	jump := dialIntegrationConnection(t, fixture.jumpConfig(t))
	// The shell holds the jump transport's lease across the failed tunnel.
	session := openSharedIntegrationSession(t, jump)
	config := fixture.targetConfig(jump.DialContext)
	config.Port = 1 // nothing listens there in the fixture container
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if connection, err := sshclient.Dial(ctx, config); err == nil {
		connection.Close()
		t.Fatal("connected through the jump host to a closed port")
	}
	session.command(t, "printf '\\n%s\\n' JUMP_SURVIVES_REFUSAL")
	session.awaitText(t, "JUMP_SURVIVES_REFUSAL")
}

func TestGatewayIntegrationSOCKS5(t *testing.T) {
	fixture := gatewayFixtures(t)
	password := fixtureSecret(t, fixture.directory, "password")
	dial := socksDialer(t, fixture, fixture.SOCKS.Address, password, proxy.Direct)
	expectPeer(t, openIntegrationSession(t, fixture.targetConfig(dial)), fixture.SOCKS.Internal)

	rejected := socksDialer(t, fixture, fixture.SOCKS.Address, password+"-wrong", proxy.Direct)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if connection, err := sshclient.Dial(ctx, fixture.targetConfig(rejected)); err == nil {
		connection.Close()
		t.Fatal("SOCKS5 proxy accepted a wrong password")
	}
}

func TestGatewayIntegrationSOCKS5OverSSHJump(t *testing.T) {
	fixture := gatewayFixtures(t)
	password := fixtureSecret(t, fixture.directory, "password")
	jump := dialIntegrationConnection(t, fixture.jumpConfig(t))
	// The proxy is addressed by its bridge address, reachable only from the jump host.
	dial := socksDialer(t, fixture, net.JoinHostPort(fixture.SOCKS.Internal, "1080"), password, contextForward(jump.DialContext))
	expectPeer(t, openIntegrationSession(t, fixture.targetConfig(dial)), fixture.SOCKS.Internal)
}
