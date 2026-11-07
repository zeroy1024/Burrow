package sshclient_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"

	"golang.org/x/crypto/ssh"
)

func channelServer(t *testing.T, handle func(context.Context, ssh.NewChannel, int)) sshclient.Config {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		raw, err := listener.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		server, channels, requests, err := ssh.NewServerConn(raw, config)
		if err != nil {
			return
		}
		defer server.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var workers sync.WaitGroup
		workers.Go(func() { server.Wait(); cancel() })
		workers.Go(func() { ssh.DiscardRequests(requests) })
		count := 0
		for channel := range channels {
			count++
			index := count
			workers.Go(func() { handle(ctx, channel, index) })
		}
		workers.Wait()
	}()
	t.Cleanup(func() {
		listener.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("controlled SSH server did not stop")
		}
	})
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return sshclient.Config{Host: host, Port: port, User: "fixture", Auth: "password", HostKey: ssh.FixedHostKey(signer.PublicKey())}
}

func echoChannel(_ context.Context, incoming ssh.NewChannel, _ int) {
	channel, requests, err := incoming.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	var workers sync.WaitGroup
	workers.Go(func() { io.Copy(channel, channel) })
	for request := range requests {
		request.Reply(request.Type == "pty-req" || request.Type == "shell", nil)
	}
	workers.Wait()
}

func waitConnection(t *testing.T, connection *sshclient.Connection) {
	t.Helper()
	done := make(chan struct{})
	go func() { connection.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("connection did not join its channel workers")
	}
}

func TestConnectionCanceledChannelOpenDoesNotRetainOrMultiply(t *testing.T) {
	entered := make(chan struct{})
	var channelCount atomic.Int32
	cfg := channelServer(t, func(ctx context.Context, incoming ssh.NewChannel, index int) {
		channelCount.Add(1)
		if index == 1 {
			echoChannel(ctx, incoming, index)
			return
		}
		close(entered)
		// Deliberately never accept/reject this request. SSH has no per-request cancel.
		<-ctx.Done()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := sshclient.Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(); waitConnection(t, connection) })
	shell, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	childCtx, cancelChild := context.WithCancel(ctx)
	opened := make(chan error, 1)
	go func() {
		_, err := connection.OpenShell(childCtx, sshclient.ShellOptions{Columns: 80, Rows: 24})
		opened <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("second channel was not requested")
	}
	cancelChild()
	select {
	case err := <-opened:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not release the caller")
	}
	if _, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24}); !errors.Is(err, sshclient.ErrChannelPending) {
		t.Fatalf("pending request did not block another worker: %v", err)
	}
	if channelCount.Load() != 2 {
		t.Fatalf("server saw %d channel requests, want 2", channelCount.Load())
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelWait()
	if err := connection.WaitChannelReady(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled internal open became ready before server replied: %v", err)
	}
	if _, err := io.WriteString(shell, "still-alive"); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len("still-alive"))
	if _, err := io.ReadFull(shell, response); err != nil || string(response) != "still-alive" {
		t.Fatalf("cancel affected healthy sibling: %q, %v", response, err)
	}
	shell.Close()
	// The canceled internal request must not keep the transport alive.
	waitConnection(t, connection)
	if _, err := connection.Retain(); !errors.Is(err, sshclient.ErrClosed) {
		t.Fatalf("closed transport accepted another intent: %v", err)
	}
	if err := connection.WaitChannelReady(ctx); !errors.Is(err, sshclient.ErrClosed) {
		t.Fatalf("closed transport reported a ready channel slot: %v", err)
	}
}

func TestConnectionChannelReadySignalsCreationCompletion(t *testing.T) {
	entered, allow := make(chan struct{}), make(chan struct{})
	cfg := channelServer(t, func(ctx context.Context, incoming ssh.NewChannel, index int) {
		if index == 2 {
			close(entered)
			select {
			case <-allow:
			case <-ctx.Done():
				return
			}
		}
		echoChannel(ctx, incoming, index)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := sshclient.Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(); waitConnection(t, connection) })
	if err := connection.WaitChannelReady(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	type opened struct {
		conn *sshclient.Conn
		err  error
	}
	second := make(chan opened, 1)
	go func() {
		conn, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24})
		second <- opened{conn: conn, err: err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("delayed channel creation did not reach the server")
	}
	waiting := make(chan error, 1)
	go func() { waiting <- connection.WaitChannelReady(ctx) }()
	close(allow)
	select {
	case err := <-waiting:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("channel creation completion did not wake a queued intent")
	}
	select {
	case ready := <-second:
		if ready.err != nil {
			t.Fatal(ready.err)
		}
		ready.conn.Close()
	case <-ctx.Done():
		t.Fatal("second Shell did not open")
	}
	first.Close()
	waitConnection(t, connection)
}

func TestConnectionSubsystemKeepsProtocolAndSiblingIndependent(t *testing.T) {
	payload := []byte{0, 0, 0, 5, 2, 0, 0, 0, 3}
	var ptyRequests atomic.Int32
	cfg := channelServer(t, func(ctx context.Context, incoming ssh.NewChannel, index int) {
		if index == 1 {
			echoChannel(ctx, incoming, index)
			return
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			return
		}
		defer channel.Close()
		for request := range requests {
			if request.Type == "pty-req" {
				ptyRequests.Add(1)
			}
			if request.Type != "subsystem" {
				request.Reply(false, nil)
				continue
			}
			request.Reply(true, nil)
			channel.Stderr().Write([]byte("diagnostic, not protocol"))
			channel.Write(payload)
			channel.CloseWrite()
			return
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	connection, err := sshclient.Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close(); waitConnection(t, connection) })
	shell, err := connection.OpenShell(ctx, sshclient.ShellOptions{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := connection.OpenSubsystem(ctx, "sftp")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(stream)
	if err != nil || !bytes.Equal(got, payload) || ptyRequests.Load() != 0 {
		t.Fatalf("binary subsystem: %x, PTY requests=%d, err=%v", got, ptyRequests.Load(), err)
	}
	stream.Close()
	if _, err := io.WriteString(shell, "sibling"); err != nil {
		t.Fatal(err)
	}
	got = make([]byte, len("sibling"))
	if _, err := io.ReadFull(shell, got); err != nil || string(got) != "sibling" {
		t.Fatalf("subsystem exit affected shell: %q, %v", got, err)
	}
	shell.Close()
	waitConnection(t, connection)
}
