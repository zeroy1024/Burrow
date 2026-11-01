//go:build integration

package fileclient

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshdesk/internal/sshclient"
)

func sftpFixtureConnection(t *testing.T) *sshclient.Connection {
	t.Helper()
	address, directory := os.Getenv("SSHDESK_TEST_ADDRESS"), os.Getenv("SSHDESK_TEST_DIR")
	if address == "" || directory == "" {
		t.Skip("SSHDESK_TEST_ADDRESS and SSHDESK_TEST_DIR are required")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			t.Fatal("the Docker SFTP fixture must use a loopback address")
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	connection, err := sshclient.Dial(ctx, sshclient.Config{Host: host, Port: port, User: "sshdesk", Auth: "key",
		KeyPath: filepath.Join(directory, "key_plain"),
		HostKey: sshclient.KnownHosts(filepath.Join(t.TempDir(), "known_hosts"), func(string, string, string) bool { return true })})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		connection.Close()
		cancel()
		done := make(chan struct{})
		go func() { connection.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("SFTP connection workers did not stop")
		}
	})
	return connection
}

type sftpShellOutput struct {
	mu      sync.Mutex
	text    strings.Builder
	changed chan struct{}
}

func (o *sftpShellOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	n, err := o.text.Write(data)
	o.mu.Unlock()
	select {
	case o.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (o *sftpShellOutput) contains(text string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.Contains(o.text.String(), text)
}

func sftpSiblingShell(t *testing.T, owner *sshclient.Connection) (*sshclient.Conn, *sftpShellOutput) {
	t.Helper()
	shell, err := owner.OpenShell(t.Context(), sshclient.ShellOptions{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	output := &sftpShellOutput{changed: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(output, shell); close(done) }()
	t.Cleanup(func() {
		shell.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("sibling Shell did not stop")
		}
	})
	if _, err := io.WriteString(shell, "stty -echo; printf '\\nSFTP-SHELL-%s\\n' READY\r"); err != nil {
		t.Fatal(err)
	}
	awaitSFTPShell(t, output, "\nSFTP-SHELL-READY\r\n")
	return shell, output
}

func awaitSFTPShell(t *testing.T, output *sftpShellOutput, expected string) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !output.contains(expected) {
		select {
		case <-output.changed:
		case <-deadline.C:
			t.Fatal("sibling Shell did not produce the expected marker")
		}
	}
}

func checkSFTPShell(t *testing.T, shell *sshclient.Conn, output *sftpShellOutput, marker string) {
	t.Helper()
	if _, err := io.WriteString(shell, "printf '\\n%s\\n' "+marker+"\r"); err != nil {
		t.Fatal(err)
	}
	awaitSFTPShell(t, output, "\r\n"+marker+"\r\n")
}

func TestSFTPIntegrationDirectory(t *testing.T) {
	owner := sftpFixtureConnection(t)
	client, err := NewSFTP(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	home, err := client.InitialPath(t.Context())
	if err != nil || home != "/home/sshdesk" {
		t.Fatalf("initial path = %q, error = %v", home, err)
	}
	entries, err := client.ReadDir(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	for _, name := range []string{"hello.txt", "你好.txt", "nested", "denied"} {
		if _, found := byName[name]; !found {
			t.Errorf("remote directory is missing fixture entry %q", name)
		}
	}
	if !byName["nested"].Directory || byName["hello.txt"].Directory || byName["hello.txt"].Permissions == "" || byName["hello.txt"].Owner == "" {
		t.Fatal("remote entry metadata was not preserved")
	}
	if _, err := client.ReadDir(t.Context(), path.Join(home, "denied")); err == nil {
		t.Fatal("restricted directory was readable")
	}
	select {
	case <-client.Done():
		t.Fatal("permission denial disconnected SFTP")
	default:
	}
	entries, err = client.ReadDir(t.Context(), path.Join(home, "nested"))
	if err != nil || len(entries) != 1 || entries[0].Name != "inside.txt" {
		t.Fatalf("nested directory did not remain usable: %v", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.ReadDir(canceled, home); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled ReadDir error = %v", err)
	}
	select {
	case <-client.Done():
		t.Fatal("a request canceled before I/O disconnected SFTP")
	default:
	}
}

func TestSFTPIntegrationSharedLifetime(t *testing.T) {
	for _, action := range []string{"close_files", "close_shell", "cancel_files"} {
		t.Run(action, func(t *testing.T) {
			owner := sftpFixtureConnection(t)
			shell, output := sftpSiblingShell(t, owner)
			fileCtx, cancelFiles := context.WithCancel(t.Context())
			defer cancelFiles()
			client, err := NewSFTP(fileCtx, owner)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { client.Close() })
			if action == "close_shell" {
				shell.Close()
				if _, err := client.ReadDir(t.Context(), "/home/sshdesk/nested"); err != nil {
					t.Fatalf("closing Shell disconnected SFTP: %v", err)
				}
			} else {
				if action == "cancel_files" {
					cancelFiles()
				} else {
					client.Close()
				}
				select {
				case <-client.Done():
				case <-time.After(3 * time.Second):
					t.Fatal("SFTP did not finish after close/cancel")
				}
				checkSFTPShell(t, shell, output, "SFTP_SIBLING_ALIVE")
			}
		})
	}
}

// This stream models an SSH channel whose remote window blocks Write. Closing
// the child records cancellation, but only closing its owner unblocks the write.
type sftpBlockedWrite struct {
	owner   *sshclient.Connection
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (s *sftpBlockedWrite) Read([]byte) (int, error) { return 0, io.EOF }
func (s *sftpBlockedWrite) Write([]byte) (int, error) {
	close(s.started)
	<-s.owner.Done()
	return 0, io.ErrClosedPipe
}
func (s *sftpBlockedWrite) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func TestSFTPBlockedInitializationCancellation(t *testing.T) {
	owner := sftpFixtureConnection(t)
	shell, output := sftpSiblingShell(t, owner)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := &sftpBlockedWrite{owner: owner, started: make(chan struct{}), closed: make(chan struct{})}
	var opens atomic.Int32
	opener := func(channelCtx context.Context) (io.ReadWriteCloser, error) {
		opens.Add(1)
		context.AfterFunc(channelCtx, func() { stream.Close() })
		return stream, nil
	}
	result := make(chan error, 1)
	go func() {
		_, err := newSFTP(ctx, owner, opener)
		result <- err
	}()
	select {
	case <-stream.started:
	case <-time.After(3 * time.Second):
		t.Fatal("initializer did not enter the blocked write")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled initializer error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("caller remained blocked in SFTP initialization")
	}
	select {
	case <-stream.closed:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled initializer did not close its child stream")
	}
	if _, err := newSFTP(t.Context(), owner, opener); !errors.Is(err, ErrSFTPInitPending) || opens.Load() != 1 {
		t.Fatalf("a blocked initializer was duplicated: error=%v, opens=%d", err, opens.Load())
	}
	checkSFTPShell(t, shell, output, "SFTP_INIT_CANCELED_SHELL_ALIVE")
	owner.Close()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		_, err := NewSFTP(t.Context(), owner)
		if !errors.Is(err, ErrSFTPInitPending) {
			if !errors.Is(err, sshclient.ErrClosed) {
				t.Fatalf("closed owner error = %v", err)
			}
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("owner shutdown did not release the protocol initializer")
		}
	}
}
