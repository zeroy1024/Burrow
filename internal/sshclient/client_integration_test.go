//go:build integration

package sshclient_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/store"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type integrationFixture struct {
	address, directory string
	config             sshclient.Config
}

func sshFixture(t *testing.T) integrationFixture {
	t.Helper()
	address, directory := os.Getenv("BURROW_TEST_ADDRESS"), os.Getenv("BURROW_TEST_DIR")
	if address == "" || directory == "" {
		t.Skip("BURROW_TEST_ADDRESS and BURROW_TEST_DIR are required for the Docker SSH fixture")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			t.Fatal("the Docker SSH fixture must use a loopback address")
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return integrationFixture{
		address: address, directory: directory,
		config: sshclient.Config{
			Host: host, Port: port, User: "burrow", Auth: "key",
			KeyPath: filepath.Join(directory, "key_plain"),
			HostKey: sshclient.KnownHosts(t.Context(), integrationTrustStore(t, filepath.Join(t.TempDir(), "burrow.db")), func(string, string, string) bool { return true }),
		},
	}
}

func fixtureSecret(t *testing.T, directory, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	defer clear(data)
	return strings.TrimSpace(string(data))
}

type integrationOutput struct {
	mu      sync.Mutex
	data    []byte
	changed chan struct{}
}

func (o *integrationOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	o.data = append(o.data, p...)
	o.mu.Unlock()
	select {
	case o.changed <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (o *integrationOutput) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.data)
}

type integrationSession struct {
	conn     *sshclient.Conn
	cancel   context.CancelFunc
	output   *integrationOutput
	readDone chan struct{}
	waitDone chan struct{}
	waitErr  error
}

func dialIntegrationConnection(t *testing.T, cfg sshclient.Config) *sshclient.Connection {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	connection, err := sshclient.Dial(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("connect fixture: %v", err)
	}
	t.Cleanup(func() {
		connection.Close()
		cancel()
		done := make(chan struct{})
		go func() { connection.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SSH connection workers did not stop")
		}
	})
	return connection
}

func openIntegrationSession(t *testing.T, cfg sshclient.Config) *integrationSession {
	t.Helper()
	return openSharedIntegrationSession(t, dialIntegrationConnection(t, cfg))
}

func openSharedIntegrationSession(t *testing.T, connection *sshclient.Connection) *integrationSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	conn, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24})
	if err != nil {
		cancel()
		t.Fatalf("open fixture shell: %v", err)
	}
	s := &integrationSession{conn: conn, cancel: cancel, output: &integrationOutput{changed: make(chan struct{}, 1)}, readDone: make(chan struct{}), waitDone: make(chan struct{})}
	go func() { _, _ = io.Copy(s.output, conn); close(s.readDone) }()
	go func() { s.waitErr = conn.Wait(); close(s.waitDone) }()
	t.Cleanup(func() {
		conn.Close()
		cancel()
		for _, done := range []<-chan struct{}{s.readDone, s.waitDone} {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("SSH fixture I/O did not stop after Close")
			}
		}
	})
	// Stop command echo so typed command text cannot satisfy output assertions.
	s.command(t, "stty -echo; printf '\\nSSH-%s\\n' READY")
	s.awaitText(t, "SSH-READY")
	return s
}

func (s *integrationSession) command(t *testing.T, command string) {
	t.Helper()
	if _, err := io.WriteString(s.conn, command+"\r"); err != nil {
		t.Fatalf("write fixture command: %v", err)
	}
}

func (s *integrationSession) awaitText(t *testing.T, expected string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		if strings.Contains(s.output.text(), expected) {
			return
		}
		select {
		case <-s.output.changed:
		case <-s.readDone:
			if !strings.Contains(s.output.text(), expected) {
				t.Fatalf("SSH output ended before expected marker %q (%d bytes received)", expected, len(s.output.text()))
			}
			return
		case <-timer.C:
			t.Fatalf("SSH output did not contain marker %q (%d bytes received)", expected, len(s.output.text()))
		}
	}
}

func (s *integrationSession) awaitExit(t *testing.T) error {
	t.Helper()
	for _, done := range []<-chan struct{}{s.waitDone, s.readDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("SSH exit did not finish")
		}
	}
	return s.waitErr
}

func TestSSHIntegrationAuthentication(t *testing.T) {
	fixture := sshFixture(t)
	for _, name := range []string{"password", "plain_key", "encrypted_key", "agent"} {
		t.Run(name, func(t *testing.T) {
			cfg := fixture.config
			switch name {
			case "password":
				cfg.Auth, cfg.Password = "password", fixtureSecret(t, fixture.directory, "password")
			case "encrypted_key":
				cfg.KeyPath = filepath.Join(fixture.directory, "key_encrypted")
				cfg.Passphrase = fixtureSecret(t, fixture.directory, "key_passphrase")
			case "agent":
				startIntegrationAgent(t, fixture.directory)
				cfg.Auth = "agent"
			}
			s := openIntegrationSession(t, cfg)
			s.command(t, "printf '\\nAUTH-%s\\n' OK; exit 0")
			s.awaitText(t, "AUTH-OK")
			if err := s.awaitExit(t); err != nil {
				t.Fatalf("authenticated shell exit: %v", err)
			}
		})
	}
	for _, name := range []string{"wrong_password", "wrong_key_passphrase"} {
		t.Run(name, func(t *testing.T) {
			cfg := fixture.config
			failure := "unable to authenticate"
			if name == "wrong_password" {
				cfg.Auth, cfg.Password = "password", "intentionally-invalid-fixture-credential"
			} else {
				cfg.KeyPath, cfg.Passphrase = filepath.Join(fixture.directory, "key_encrypted"), "intentionally-invalid-fixture-credential"
				failure = "parse SSH private key"
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			conn, err := sshclient.Dial(ctx, cfg)
			if err == nil {
				conn.Close()
				t.Fatal("invalid credential authenticated")
			}
			if !strings.Contains(err.Error(), failure) {
				t.Fatalf("invalid credential failed for an unexpected reason: %v", err)
			}
		})
	}
}

func TestSSHIntegrationHostKeyTrust(t *testing.T) {
	fixture := sshFixture(t)
	t.Run("reject_unknown", func(t *testing.T) {
		cfg, path := fixture.config, filepath.Join(t.TempDir(), "burrow.db")
		db := integrationTrustStore(t, path)
		called := false
		cfg.HostKey = sshclient.KnownHosts(t.Context(), db, func(string, string, string) bool { called = true; return false })
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		conn, err := sshclient.Dial(ctx, cfg)
		if err == nil {
			conn.Close()
			t.Fatal("untrusted host connected")
		}
		if !called || !strings.Contains(err.Error(), "not trusted") {
			t.Fatalf("unknown host was not rejected by trust decision: %v", err)
		}
		other, _, keyErr := ed25519.GenerateKey(rand.Reader)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		key, keyErr := ssh.NewPublicKey(other)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		remote, keyErr := net.ResolveTCPAddr("tcp", fixture.address)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		checkErr := db.CheckHostKey(t.Context(), fixture.address, remote, key)
		var unknown *knownhosts.KeyError
		if !errors.As(checkErr, &unknown) || len(unknown.Want) != 0 {
			t.Fatalf("rejecting the host persisted trust: %v", checkErr)
		}

	})
	t.Run("accept_persist_and_reuse", func(t *testing.T) {
		cfg, path := fixture.config, filepath.Join(t.TempDir(), "burrow.db")
		db := integrationTrustStore(t, path)
		calls := 0
		cfg.HostKey = sshclient.KnownHosts(t.Context(), db, func(_ string, fingerprint, keyType string) bool {
			calls++
			if !strings.HasPrefix(fingerprint, "SHA256:") || keyType == "" {
				t.Error("host confirmation omitted fingerprint or key type")
			}
			return true
		})
		for range 2 {
			s := openIntegrationSession(t, cfg)
			s.command(t, "exit 0")
			if err := s.awaitExit(t); err != nil {
				t.Fatal(err)
			}
		}
		if calls != 1 {
			t.Fatalf("host confirmation called %d times, want once", calls)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("trusted host key file was not saved with mode 0600")
		}
	})
	t.Run("reject_changed_without_confirmation", func(t *testing.T) {
		cfg, path := fixture.config, filepath.Join(t.TempDir(), "burrow.db")
		db := integrationTrustStore(t, path)
		other, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		key, err := ssh.NewPublicKey(other)
		if err != nil {
			t.Fatal(err)
		}
		remote, err := net.ResolveTCPAddr("tcp", fixture.address)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.TrustHostKey(t.Context(), fixture.address, remote, key); err != nil {
			t.Fatal(err)
		}
		called := false
		cfg.HostKey = sshclient.KnownHosts(t.Context(), db, func(string, string, string) bool { called = true; return true })
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		conn, err := sshclient.Dial(ctx, cfg)
		if err == nil {
			conn.Close()
			t.Fatal("changed host key connected")
		}
		var mismatch *knownhosts.KeyError
		if called || !errors.As(err, &mismatch) || len(mismatch.Want) == 0 {
			t.Fatalf("changed key was not rejected without a confirmation override: %v", err)
		}
	})
}

func TestSSHIntegrationTerminalStreamAndResize(t *testing.T) {
	fixture := sshFixture(t)
	s := openIntegrationSession(t, fixture.config)
	s.command(t, "printf '\\nUTF8:你好，终端\\n\\033[31mCOLOR:RED\\033[0m\\n'; printf 'SIZE1:'; stty size; printf 'SIZE1-%s\\n' DONE")
	s.awaitText(t, "SIZE1-DONE")
	output := s.output.text()
	for _, expected := range []string{"UTF8:你好，终端", "\x1b[31mCOLOR:RED\x1b[0m", "SIZE1:24 80\r\n"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("terminal output omitted expected UTF-8/color/initial-size value %q", expected)
		}
	}
	if err := s.conn.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	s.command(t, "printf 'SIZE2:'; stty size; printf 'SIZE2-%s\\n' DONE")
	s.awaitText(t, "SIZE2-DONE")
	if !strings.Contains(s.output.text(), "SIZE2:40 120\r\n") {
		t.Fatal("remote PTY did not receive resized columns and rows")
	}
	s.command(t, "exit 0")
	if err := s.awaitExit(t); err != nil {
		t.Fatal(err)
	}
}

func TestSSHIntegrationConcurrentSessions(t *testing.T) {
	fixture := sshFixture(t)
	sessions := make([]*integrationSession, 4)
	for i := range sessions {
		sessions[i] = openIntegrationSession(t, fixture.config)
	}
	for i, session := range sessions {
		session.command(t, fmt.Sprintf("SESSION_ID=%d; printf '\\nSESSION:%%s\\n' \"$SESSION_ID\"; exit 0", i))
	}
	for i, session := range sessions {
		session.awaitText(t, fmt.Sprintf("SESSION:%d", i))
		if err := session.awaitExit(t); err != nil {
			t.Fatalf("session %d: %v", i, err)
		}
		for j := range sessions {
			if i != j && strings.Contains(session.output.text(), fmt.Sprintf("SESSION:%d", j)) {
				t.Fatalf("session %d received session %d output", i, j)
			}
		}
	}
}

func TestSSHIntegrationSharedConnection(t *testing.T) {
	fixture := sshFixture(t)
	var authentications atomic.Int32
	check := fixture.config.HostKey
	fixture.config.HostKey = func(host string, address net.Addr, key ssh.PublicKey) error {
		authentications.Add(1)
		return check(host, address, key)
	}
	connection := dialIntegrationConnection(t, fixture.config)
	first := openSharedIntegrationSession(t, connection)
	second := openSharedIntegrationSession(t, connection)
	for index, session := range []*integrationSession{first, second} {
		session.command(t, fmt.Sprintf("VALUE=handle%d; printf '\\nTRANSPORT:%%s\\nVALUE:%%s\\nMETA-DONE\\n' \"$SSH_CONNECTION\" \"$VALUE\"", index))
		session.awaitText(t, "META-DONE")
	}
	transport := func(session *integrationSession) string {
		_, after, ok := strings.Cut(session.output.text(), "\nTRANSPORT:")
		if !ok {
			t.Fatal("remote shell omitted SSH connection identity")
		}
		value, _, _ := strings.Cut(after, "\r\n")
		return value
	}
	if transport(first) != transport(second) || authentications.Load() != 1 {
		t.Fatal("split shells did not share one authenticated TCP connection")
	}
	if err := first.conn.Resize(92, 31); err != nil {
		t.Fatal(err)
	}
	first.command(t, "printf 'FIRST-SIZE:'; stty size; printf 'FIRST-%s\\n' DONE")
	second.command(t, "printf 'SECOND-SIZE:'; stty size; printf '\\nSECOND-VALUE:%s\\nSECOND-%s\\n' \"$VALUE\" DONE")
	first.awaitText(t, "FIRST-DONE")
	second.awaitText(t, "SECOND-DONE")
	if !strings.Contains(first.output.text(), "FIRST-SIZE:31 92\r\n") ||
		!strings.Contains(second.output.text(), "SECOND-SIZE:24 80\r\n") ||
		!strings.Contains(second.output.text(), "SECOND-VALUE:handle1") {
		t.Fatal("shared transport mixed Shell state or PTY dimensions")
	}
	if _, err := connection.OpenSubsystem(t.Context(), "missing-integration-subsystem"); err == nil {
		t.Fatal("missing subsystem unexpectedly succeeded")
	}
	first.cancel()
	if err := first.awaitExit(t); err == nil {
		t.Fatal("canceled Shell reported success")
	}
	select {
	case <-connection.Done():
		t.Fatal("canceling one Shell closed the shared transport")
	default:
	}
	second.command(t, "printf '\\nSIBLING-%s\\n' ALIVE; exit 0")
	second.awaitText(t, "SIBLING-ALIVE")
	if err := second.awaitExit(t); err != nil {
		t.Fatal(err)
	}
	waitConnection(t, connection)
}

func TestSSHIntegrationRetainedIntent(t *testing.T) {
	fixture := sshFixture(t)
	connection := dialIntegrationConnection(t, fixture.config)
	hold, err := connection.Retain()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hold)
	first := openSharedIntegrationSession(t, connection)
	first.command(t, "exit 0")
	if err := first.awaitExit(t); err != nil {
		t.Fatal(err)
	}
	select {
	case <-connection.Done():
		t.Fatal("queued intent did not retain the transport")
	default:
	}
	second := openSharedIntegrationSession(t, connection)
	hold()
	hold()
	second.command(t, "printf '\\nRETAINED-%s\\n' OK; exit 0")
	second.awaitText(t, "RETAINED-OK")
	if err := second.awaitExit(t); err != nil {
		t.Fatal(err)
	}
	waitConnection(t, connection)
}

func TestSSHIntegrationExitPreservesTail(t *testing.T) {
	fixture := sshFixture(t)
	s := openIntegrationSession(t, fixture.config)
	before := strings.Count(s.output.text(), "X")
	s.command(t, "head -c 131072 /dev/zero | tr '\\000' X; printf '\\nTAIL-%s\\n' DONE; exit 7")
	err := s.awaitExit(t)
	var exit *ssh.ExitError
	if !errors.As(err, &exit) || exit.ExitStatus() != 7 {
		t.Fatalf("remote exit status was not retained: %v", err)
	}
	output := s.output.text()
	if strings.Count(output, "X")-before != 131072 || !strings.Contains(output, "TAIL-DONE") {
		t.Fatal("SSH exit lost output bytes or final marker")
	}
}

func TestSSHIntegrationCancellation(t *testing.T) {
	fixture := sshFixture(t)
	t.Run("connected", func(t *testing.T) {
		s := openIntegrationSession(t, fixture.config)
		s.cancel()
		if err := s.awaitExit(t); err == nil {
			t.Fatal("canceled shell reported a successful exit")
		}
		if _, err := s.conn.Write([]byte("ignored\r")); err == nil {
			t.Fatal("canceled SSH connection accepted input")
		}
	})
	t.Run("pending_host_confirmation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		cfg := fixture.config
		entered := make(chan struct{})
		cfg.HostKey = sshclient.KnownHosts(t.Context(), integrationTrustStore(t, filepath.Join(t.TempDir(), "burrow.db")), func(string, string, string) bool {
			close(entered)
			<-ctx.Done()
			return false
		})
		result := make(chan error, 1)
		go func() {
			conn, err := sshclient.Dial(ctx, cfg)
			if conn != nil {
				conn.Close()
			}
			result <- err
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("host confirmation was never reached")
		}
		cancel()
		select {
		case err := <-result:
			if err == nil {
				t.Fatal("canceled handshake connected")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("canceled host confirmation did not release Dial")
		}
	})
}

func startIntegrationAgent(t *testing.T, directory string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "key_plain"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.ParseRawPrivateKey(data)
	clear(data)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(directory, "agent-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	var clientsMu sync.Mutex
	var clients []net.Conn
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			clientsMu.Lock()
			clients = append(clients, client)
			clientsMu.Unlock()
			workers.Go(func() {
				defer client.Close()
				_ = agent.ServeAgent(keyring, client)
			})
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-accepted
		clientsMu.Lock()
		for _, client := range clients {
			client.Close()
		}
		clientsMu.Unlock()
		done := make(chan struct{})
		go func() { workers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SSH agent fixture did not stop")
		}
	})
	t.Setenv("SSH_AUTH_SOCK", socket)
}

func integrationTrustStore(t *testing.T, path string) *store.Store {
	t.Helper()
	db, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
