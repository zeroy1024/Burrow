//go:build integration

package fileclient_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"sshdesk/internal/fileclient"
)

func ftpDockerConfig(t *testing.T) fileclient.FTPConfig {
	t.Helper()
	address, directory := os.Getenv("SSHDESK_FTP_ADDRESS"), os.Getenv("SSHDESK_TEST_DIR")
	if address == "" || directory == "" {
		t.Skip("SSHDESK_FTP_ADDRESS and SSHDESK_TEST_DIR are required for the Docker FTP fixture")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatal("the Docker FTP fixture must use a loopback address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	password, err := os.ReadFile(filepath.Join(directory, "password"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(password)
	return fileclient.FTPConfig{Host: host, Port: port, User: "sshdesk", Password: strings.TrimSpace(string(password))}
}

func openFTPFixture(t *testing.T, cfg fileclient.FTPConfig) fileclient.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	client, err := fileclient.DialFTP(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("connect FTP fixture: %v", err)
	}
	t.Cleanup(func() { client.Close(); cancel() })
	return client
}

func awaitFTPDone(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(description)
	}
}

func TestFTPIntegrationDocker(t *testing.T) {
	cfg := ftpDockerConfig(t)
	t.Run("login_pwd_utf8_and_navigation", func(t *testing.T) {
		client := openFTPFixture(t, cfg)
		root, err := client.InitialPath(t.Context())
		if err != nil || root == "" {
			t.Fatalf("FTP working directory: %v", err)
		}
		entries, err := client.ReadDir(t.Context(), root)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]fileclient.Entry{}
		for _, entry := range entries {
			found[entry.Name] = entry
		}
		for _, name := range []string{"hello.txt", "你好.txt", "nested"} {
			if _, exists := found[name]; !exists {
				t.Fatalf("FTP fixture omitted expected entry %q", name)
			}
		}
		if !found["nested"].Directory || found["hello.txt"].Directory || found["hello.txt"].Size == 0 {
			t.Fatal("FTP file/directory metadata was incorrect")
		}
		hello := found["hello.txt"]
		if hello.Owner != "1001:1001" || hello.Permissions != "-rw-r--r--" || !hello.ModeKnown || hello.Mode.Perm() != 0644 {
			t.Fatalf("FTP fixture owner or permissions mismatch: %+v", hello)
		}
		children, err := client.ReadDir(t.Context(), path.Join(root, "nested"))
		if err != nil || len(children) == 0 {
			t.Fatalf("FTP nested directory: %v", err)
		}
		for range 3 {
			if _, err := client.ReadDir(t.Context(), root); err != nil {
				t.Fatalf("FTP repeated passive listing: %v", err)
			}
		}
	})
	t.Run("wrong_password", func(t *testing.T) {
		invalid := cfg
		invalid.Password += "-invalid"
		client, err := fileclient.DialFTP(t.Context(), invalid)
		if err == nil {
			client.Close()
			t.Fatal("wrong FTP password authenticated")
		}
		if !strings.Contains(err.Error(), "authenticate FTP") {
			t.Fatalf("wrong password failed for an unexpected reason: %v", err)
		}
	})
	t.Run("permission_error_preserves_connection", func(t *testing.T) {
		client := openFTPFixture(t, cfg)
		root, err := client.InitialPath(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.ReadDir(t.Context(), path.Join(root, "denied")); err == nil {
			t.Fatal("FTP denied directory was unexpectedly readable")
		}
		select {
		case <-client.Done():
			t.Fatal("FTP permission error closed a healthy control connection")
		default:
		}
		if _, err := client.InitialPath(t.Context()); err != nil {
			t.Fatalf("FTP control stream lost alignment after a permission error: %v", err)
		}
	})
	t.Run("lifetime_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		client, err := fileclient.DialFTP(ctx, cfg)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		defer client.Close()
		cancel()
		awaitFTPDone(t, client.Done(), "FTP cancellation did not close the session")
		if _, err := client.InitialPath(t.Context()); err == nil {
			t.Fatal("canceled FTP session still accepted commands")
		}
	})
}

func TestFTPIntegrationUploadResume(t *testing.T) {
	cfg := ftpDockerConfig(t)
	payload := bytes.Repeat([]byte("disposable-resume-data-"), (8<<20)/22+1)[:8<<20]
	name := fmt.Sprintf("/.ftp-resume-%d.part", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(t.Context())
	client, err := fileclient.DialFTP(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	writer, err := client.OpenWrite(ctx, name, 0, true)
	if err != nil {
		cancel()
		client.Close()
		t.Fatal(err)
	}
	if _, err := io.CopyN(writer, bytes.NewReader(payload), 7<<20); err != nil {
		cancel()
		writer.Close()
		client.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		cancel()
		client.Close()
		t.Fatalf("acknowledge partial upload: %v", err)
	}
	cancel()
	client.Close()
	awaitFTPDone(t, client.Done(), "partial upload control did not stop")
	resumed := openFTPFixture(t, cfg)
	partial, err := resumed.Stat(t.Context(), name)
	if err != nil {
		t.Fatalf("fresh Stat after acknowledged partial upload: %v", err)
	}
	if partial.Size <= 0 || partial.Size > 7<<20 {
		t.Fatalf("unexpected retained part size %d", partial.Size)
	}
	t.Logf("acknowledged partial upload retained %d bytes", partial.Size)
	reader, err := resumed.OpenRead(t.Context(), name, 0)
	if err != nil {
		t.Fatalf("open prefix: %v", err)
	}
	h := sha256.New()
	if _, err := io.CopyN(h, reader, partial.Size); err != nil {
		reader.Close()
		t.Fatalf("read retained prefix: %v", err)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(reader, 1)); err != nil {
		reader.Close()
		t.Fatalf("drain prefix EOF: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close prefix: %v", err)
	}
	wantPrefix := sha256.Sum256(payload[:partial.Size])
	if !bytes.Equal(h.Sum(nil), wantPrefix[:]) {
		t.Fatal("retained upload prefix changed")
	}
	if _, err := resumed.Stat(t.Context(), name); err != nil {
		t.Fatalf("Stat after prefix EOF: %v", err)
	}
	writer, err = resumed.OpenWrite(t.Context(), name, partial.Size, false)
	if err != nil {
		t.Fatalf("REST/STOR resume: %v", err)
	}
	_, writeErr := io.Copy(writer, bytes.NewReader(payload[partial.Size:]))
	if err := errors.Join(writeErr, writer.Close()); err != nil {
		t.Fatalf("write resumed suffix: %v", err)
	}
	reader, err = resumed.OpenRead(t.Context(), name, 0)
	if err != nil {
		t.Fatalf("read completed file: %v", err)
	}
	actual, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		t.Fatalf("close completed file: %v", err)
	}
	if !bytes.Equal(actual, payload) {
		t.Fatal("resumed upload contents changed")
	}
	if err := resumed.Remove(t.Context(), name, false); err != nil {
		t.Fatal(err)
	}
}

func TestFTPIntegrationPassiveCapacity(t *testing.T) {
	cfg := ftpDockerConfig(t)
	for i := range 16 {
		connection, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
		if err != nil {
			t.Fatal(err)
		}
		control := textproto.NewConn(connection)
		t.Cleanup(func() { control.Close() })
		connection.SetDeadline(time.Now().Add(10 * time.Second))
		if _, _, err := control.ReadResponse(220); err != nil {
			t.Fatal(err)
		}
		if err := control.PrintfLine("USER %s", cfg.User); err != nil {
			t.Fatal(err)
		}
		if _, _, err := control.ReadResponse(331); err != nil {
			t.Fatal(err)
		}
		if err := control.PrintfLine("PASS %s", cfg.Password); err != nil {
			t.Fatal(err)
		}
		if _, _, err := control.ReadResponse(230); err != nil {
			t.Fatal("fixture authentication failed")
		}
		if err := control.PrintfLine("EPSV"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := control.ReadResponse(229); err != nil {
			t.Fatalf("passive slot %d of 16: %v", i+1, err)
		}
	}
}

func TestFTPIntegrationStatBurst(t *testing.T) {
	client := openFTPFixture(t, ftpDockerConfig(t))
	for i := range 150 {
		if _, err := client.Stat(t.Context(), "/hello.txt"); err != nil {
			t.Fatalf("Stat iteration %d: %v", i, err)
		}
	}
}

func TestFTPIntegrationReportedPrefix(t *testing.T) {
	name := os.Getenv("SSHDESK_FTP_RESUME_PART")
	if name == "" {
		t.Skip("SSHDESK_FTP_RESUME_PART selects a read-only reported fixture part")
	}
	client := openFTPFixture(t, ftpDockerConfig(t))
	entry, err := client.Stat(t.Context(), name)
	if err != nil {
		t.Fatalf("initial Stat: %v", err)
	}
	reader, err := client.OpenRead(t.Context(), name, 0)
	if err != nil {
		t.Fatalf("open RETR: %v", err)
	}
	n, readErr := io.CopyN(io.Discard, reader, entry.Size)
	if readErr == nil {
		_, readErr = io.Copy(io.Discard, io.LimitReader(reader, 1))
	}
	if err := errors.Join(readErr, reader.Close()); err != nil {
		t.Fatalf("RETR through EOF (%d bytes): %v", n, err)
	}
	if _, err := client.Stat(t.Context(), name); err != nil {
		t.Fatalf("Stat after RETR EOF: %v", err)
	}
}

func ftpsDockerConfig(t *testing.T, metadata string) fileclient.FTPConfig {
	t.Helper()
	directory := os.Getenv("SSHDESK_TEST_DIR")
	if directory == "" {
		t.Skip("SSHDESK_TEST_DIR is required for the Docker FTPS fixture")
	}
	data, err := os.ReadFile(filepath.Join(directory, metadata))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("start the disposable FTPS fixture: " + metadata)
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Address string `json:"address"`
		TLSMode string `json:"tls_mode"`
		CA      string `json:"ca"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(fixture.Address)
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatal("FTPS fixture must use loopback")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(fixture.CA)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("fixture CA is invalid")
	}
	password, err := os.ReadFile(filepath.Join(directory, "password"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(password)
	return fileclient.FTPConfig{Host: host, Port: port, User: "sshdesk", Password: strings.TrimSpace(string(password)),
		TLSMode: fixture.TLSMode, TLSConfig: &tls.Config{RootCAs: roots}}
}

func TestFTPSIntegrationDocker(t *testing.T) {
	for _, fixture := range []struct{ name, metadata string }{
		{"explicit", "ftps-server.json"},
		{"implicit", "ftps-implicit-server.json"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			cfg := ftpsDockerConfig(t, fixture.metadata)
			t.Run("verified_control_and_data_channels", func(t *testing.T) {
				client := openFTPFixture(t, cfg)
				root, err := client.InitialPath(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				entries, err := client.ReadDir(t.Context(), root)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.ContainsFunc(entries, func(e fileclient.Entry) bool { return e.Name == "你好.txt" }) {
					t.Fatal("FTPS listing omitted UTF-8 fixture entry")
				}
				name := path.Join(root, fmt.Sprintf("ftps-roundtrip-%d.txt", time.Now().UnixNano()))
				contents := "FTPS verified transfer: 中文\n"
				writer, err := client.OpenWrite(t.Context(), name, 0, true)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := io.WriteString(writer, contents)
				if err := errors.Join(writeErr, writer.Close()); err != nil {
					t.Fatal(err)
				}
				reader, err := client.OpenRead(t.Context(), name, 0)
				if err != nil {
					t.Fatal(err)
				}
				actual, readErr := io.ReadAll(reader)
				if err := errors.Join(readErr, reader.Close()); err != nil {
					t.Fatal(err)
				}
				if string(actual) != contents {
					t.Fatal("FTPS roundtrip changed file contents")
				}
				if err := client.Remove(t.Context(), name, false); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("untrusted_CA_rejected", func(t *testing.T) {
				untrusted := cfg
				untrusted.TLSConfig = &tls.Config{RootCAs: x509.NewCertPool()}
				client, err := fileclient.DialFTP(t.Context(), untrusted)
				if err == nil {
					client.Close()
					t.Fatal("FTPS accepted an untrusted certificate")
				}
				var unknown x509.UnknownAuthorityError
				if !errors.As(err, &unknown) {
					t.Fatalf("unexpected rejection: %v", err)
				}
			})
			t.Run("wrong_hostname_rejected", func(t *testing.T) {
				wrong := cfg
				listener, err := net.Listen("tcp", "[::1]:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				ready := make(chan error, 1)
				go func() {
					connection, err := listener.Accept()
					if err != nil {
						ready <- err
						return
					}
					defer connection.Close()
					upstream, err := net.DialTimeout("tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), 5*time.Second)
					if err != nil {
						ready <- err
						return
					}
					defer upstream.Close()
					connection.SetDeadline(time.Now().Add(10 * time.Second))
					upstream.SetDeadline(time.Now().Add(10 * time.Second))
					copied := make(chan struct{})
					go func() { io.Copy(upstream, connection); close(copied) }()
					io.Copy(connection, upstream)
					connection.Close()
					upstream.Close()
					<-copied
					ready <- nil
				}()
				wrong.Host, wrong.Port = "::1", listener.Addr().(*net.TCPAddr).Port
				client, err := fileclient.DialFTP(t.Context(), wrong)
				if err == nil {
					client.Close()
					t.Fatal("FTPS accepted a certificate for the wrong hostname")
				}
				var hostname x509.HostnameError
				if !errors.As(err, &hostname) {
					t.Fatalf("unexpected hostname rejection: %v", err)
				}
				select {
				case err := <-ready:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(12 * time.Second):
					t.Fatal("FTPS hostname proxy did not stop")
				}
			})
			t.Run("data_stream_cancellation", func(t *testing.T) {
				client := openFTPFixture(t, cfg)
				root, err := client.InitialPath(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				name := path.Join(root, fmt.Sprintf("ftps-cancel-%d.bin", time.Now().UnixNano()))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				writer, err := client.OpenWrite(ctx, name, 0, true)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write(make([]byte, 32<<10)); err != nil {
					writer.Close()
					t.Fatal(err)
				}
				cancel()
				_, writeErr := writer.Write(make([]byte, 32<<10))
				if err := errors.Join(writeErr, writer.Close()); err == nil {
					t.Fatal("FTPS cancellation was reported as success")
				}
				awaitFTPDone(t, client.Done(), "FTPS cancellation did not close the stream owner")
				cleanup := openFTPFixture(t, cfg)
				if err := cleanup.Remove(t.Context(), name, false); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestFTPPrivateDirectoryIntegration(t *testing.T) {
	for _, fixture := range []struct{ name, metadata string }{
		{"ftp", ""}, {"explicit", "ftps-server.json"}, {"implicit", "ftps-implicit-server.json"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var cfg fileclient.FTPConfig
			if fixture.metadata == "" {
				cfg = ftpDockerConfig(t)
			} else {
				cfg = ftpsDockerConfig(t, fixture.metadata)
			}
			client := openFTPFixture(t, cfg)
			maker, ok := client.(fileclient.PrivateDirectoryMaker)
			if !ok {
				t.Fatal("FTP client is missing PrivateDirectoryMaker")
			}
			root, err := client.InitialPath(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			directory := path.Join(root, fmt.Sprintf("private-stage-%d", time.Now().UnixNano()))
			created, err := maker.MkdirPrivate(t.Context(), directory)
			if err != nil || !created {
				t.Fatalf("create private directory: created=%t err=%v", created, err)
			}
			entry, err := client.Stat(t.Context(), directory)
			if err != nil || !entry.Directory || entry.Symlink || !entry.ModeKnown || entry.Mode.Perm() != 0700 {
				t.Fatalf("private directory metadata: %+v err=%v", entry, err)
			}
			entries, err := client.ReadDir(t.Context(), directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("private directory must initially be empty: %v", err)
			}
			marker := path.Join(directory, "content")
			writer, err := client.OpenWrite(t.Context(), marker, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := io.WriteString(writer, "owned stage content\n")
			if err := errors.Join(writeErr, writer.Close()); err != nil {
				t.Fatal(err)
			}
			setter := client.(fileclient.PermissionSetter)
			if err := setter.Chmod(t.Context(), directory, 0755); err != nil {
				t.Fatal(err)
			}
			created, err = maker.MkdirPrivate(t.Context(), directory)
			if err == nil || created {
				t.Fatalf("existing directory was taken over: created=%t err=%v", created, err)
			}
			entry, err = client.Stat(t.Context(), directory)
			if err != nil || entry.Mode.Perm() != 0755 {
				t.Fatalf("existing directory permissions were changed: %+v err=%v", entry, err)
			}
			reader, err := client.OpenRead(t.Context(), marker, 0)
			if err != nil {
				t.Fatal(err)
			}
			contents, readErr := io.ReadAll(reader)
			if err := errors.Join(readErr, reader.Close()); err != nil {
				t.Fatal(err)
			}
			if string(contents) != "owned stage content\n" {
				t.Fatal("existing directory content was changed")
			}
			if err := client.Remove(t.Context(), marker, false); err != nil {
				t.Fatal(err)
			}
			if err := client.Remove(t.Context(), directory, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFTPPrivateDirectoryPermissionFailure(t *testing.T) {
	fixture := startFTPProtocolFixture(t, "private_chmod_rejected")
	client := openFTPFixture(t, fixture.config)
	maker := client.(fileclient.PrivateDirectoryMaker)
	created, err := maker.MkdirPrivate(t.Context(), "/new-owned-directory")
	if !created || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("confirmed MKD ownership was lost after chmod refusal: created=%t err=%v", created, err)
	}
}

// A local protocol fixture exposes timing and malformed replies that a healthy
// Docker server cannot generate. It never listens outside loopback.
type ftpProtocolFixture struct {
	config       fileclient.FTPConfig
	pwdEntered   chan struct{}
	listEntered  chan struct{}
	closed       chan struct{}
	dataClosed   chan struct{}
	control      net.Conn
	data         net.Conn
	mu           sync.Mutex
	dataListener net.Listener
	listener     net.Listener
}

func startFTPProtocolFixture(t *testing.T, pwdMode string) *ftpProtocolFixture {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &ftpProtocolFixture{listener: listener, pwdEntered: make(chan struct{}),
		listEntered: make(chan struct{}), closed: make(chan struct{}), dataClosed: make(chan struct{})}
	address := listener.Addr().(*net.TCPAddr)
	f.config = fileclient.FTPConfig{Host: "127.0.0.1", Port: address.Port, User: "fixture", Password: "local-protocol-only"}
	go f.serve(pwdMode)
	t.Cleanup(func() {
		f.listener.Close()
		f.mu.Lock()
		if f.control != nil {
			f.control.Close()
		}
		if f.data != nil {
			f.data.Close()
		}
		if f.dataListener != nil {
			f.dataListener.Close()
		}
		f.mu.Unlock()
		awaitFTPDone(t, f.closed, "controlled FTP server did not stop")
	})
	return f
}

func (f *ftpProtocolFixture) serve(pwdMode string) {
	defer close(f.closed)
	connection, err := f.listener.Accept()
	if err != nil {
		return
	}
	f.mu.Lock()
	f.control = connection
	f.mu.Unlock()
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(15 * time.Second))
	if pwdMode == "silent_greeting" {
		io.Copy(io.Discard, connection)
		return
	}
	fmt.Fprint(connection, "220 controlled fixture\r\n")
	reader := bufio.NewReader(connection)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		verb, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch verb {
		case "MKD":
			if pwdMode == "private_chmod_rejected" {
				fmt.Fprint(connection, "257 created\r\n")
			} else {
				fmt.Fprint(connection, "500 unsupported command\r\n")
			}
		case "SITE":
			if pwdMode == "private_chmod_rejected" {
				fmt.Fprint(connection, "550 permission denied\r\n")
			} else {
				fmt.Fprint(connection, "500 unsupported command\r\n")
			}
		case "USER":
			fmt.Fprint(connection, "331 password required\r\n")
		case "PASS":
			fmt.Fprint(connection, "230 authenticated\r\n")
		case "FEAT":
			fmt.Fprint(connection, "211 no features\r\n")
		case "TYPE", "OPTS":
			fmt.Fprint(connection, "200 accepted\r\n")
		case "CWD":
			fmt.Fprint(connection, "250 directory changed\r\n")
		case "PWD":
			select {
			case <-f.pwdEntered:
			default:
				close(f.pwdEntered)
			}
			if pwdMode == "blocked_pwd" {
				io.Copy(io.Discard, reader)
				return
			}
			if pwdMode == "malformed_pwd" {
				fmt.Fprint(connection, "257 one quote \"\r\n")
			} else {
				fmt.Fprint(connection, "257 \"/\"\r\n")
			}
		case "EPSV":
			fmt.Fprint(connection, "500 use passive mode\r\n")
		case "PASV":
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return
			}
			f.mu.Lock()
			f.dataListener = listener
			f.mu.Unlock()
			port := listener.Addr().(*net.TCPAddr).Port
			// This different loopback IP must be ignored in favor of the control peer.
			fmt.Fprintf(connection, "227 Entering Passive Mode (127,0,0,2,%d,%d)\r\n", port/256, port%256)
			data, err := listener.Accept()
			if err != nil {
				return
			}
			listener.Close()
			f.mu.Lock()
			f.data = data
			f.mu.Unlock()
		case "LIST", "NLST":
			fmt.Fprint(connection, "150 opening data connection\r\n")
			f.mu.Lock()
			data := f.data.(*net.TCPConn)
			f.mu.Unlock()
			if verb == "LIST" {
				close(f.listEntered)
			}
			if pwdMode == "blocked_list" {
				if _, err := data.Read(make([]byte, 1)); errors.Is(err, io.EOF) {
					close(f.dataClosed)
				}
				data.Close()
				return
			}
			if verb == "LIST" {
				fmt.Fprint(data, "-rw-r--r-- 1 user group 6 Jan 01 2026 hello.txt\r\n")
			} else {
				fmt.Fprint(data, "hello.txt\r\n")
			}
			data.CloseWrite()
			fmt.Fprint(connection, "226 listing complete\r\n")
			// Both listings must release their data sockets after consuming EOF.
			// NLST is last, so its close confirms that the full listing finished.
			if _, err := data.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				data.Close()
				return
			}
			if verb == "NLST" {
				close(f.dataClosed)
			}
			data.Close()
		default:
			fmt.Fprint(connection, "500 unsupported command\r\n")
		}
	}
}

func TestFTPIntegrationProtocolBoundaries(t *testing.T) {
	t.Run("cancel_active_data_connection", func(t *testing.T) {
		fixture := startFTPProtocolFixture(t, "blocked_list")
		client := openFTPFixture(t, fixture.config)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := client.ReadDir(ctx, "/"); result <- err }()
		awaitFTPDone(t, fixture.listEntered, "FTP listing did not open its data connection")
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("FTP listing cancellation: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("FTP listing cancellation left a blocked reader")
		}
		awaitFTPDone(t, client.Done(), "canceled FTP listing left its control session alive")
		awaitFTPDone(t, fixture.dataClosed, "canceled FTP listing left its data socket alive")
	})
	t.Run("passive_peer_and_data_cleanup", func(t *testing.T) {
		fixture := startFTPProtocolFixture(t, "normal")
		client := openFTPFixture(t, fixture.config)
		entries, err := client.ReadDir(t.Context(), "/")
		if err != nil || len(entries) != 1 || entries[0].Name != "hello.txt" {
			t.Fatalf("FTP passive listing failed: %v", err)
		}
		awaitFTPDone(t, fixture.dataClosed, "FTP data connection was not released after listing")
	})
	t.Run("malformed_pwd_is_error", func(t *testing.T) {
		fixture := startFTPProtocolFixture(t, "malformed_pwd")
		client := openFTPFixture(t, fixture.config)
		if _, err := client.InitialPath(t.Context()); err == nil || !strings.Contains(err.Error(), "malformed PWD") {
			t.Fatalf("malformed FTP PWD did not produce a parsing error: %v", err)
		}
		awaitFTPDone(t, client.Done(), "malformed FTP PWD left the session alive")
	})
	t.Run("cancel_active_and_waiting_operations", func(t *testing.T) {
		fixture := startFTPProtocolFixture(t, "blocked_pwd")
		client := openFTPFixture(t, fixture.config)
		activeCtx, cancelActive := context.WithCancel(t.Context())
		defer cancelActive()
		activeDone := make(chan error, 1)
		go func() { _, err := client.InitialPath(activeCtx); activeDone <- err }()
		awaitFTPDone(t, fixture.pwdEntered, "FTP PWD never reached the server")
		waitingCtx, cancelWaiting := context.WithCancel(t.Context())
		cancelWaiting()
		if _, err := client.InitialPath(waitingCtx); !errors.Is(err, context.Canceled) {
			t.Fatalf("queued cancellation did not return context.Canceled: %v", err)
		}
		select {
		case <-client.Done():
			t.Fatal("canceling a queued operation killed the active FTP session")
		default:
		}
		cancelActive()
		select {
		case err := <-activeDone:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("active cancellation did not return context.Canceled: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("active FTP operation did not stop")
		}
		awaitFTPDone(t, client.Done(), "active cancellation did not close FTP sockets")
	})
	t.Run("cancel_during_greeting", func(t *testing.T) {
		fixture := startFTPProtocolFixture(t, "silent_greeting")
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		client, err := fileclient.DialFTP(ctx, fixture.config)
		if err == nil {
			client.Close()
			t.Fatal("silent FTP server unexpectedly connected")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("FTP greeting deadline was not propagated: %v", err)
		}
	})
	t.Run("already_canceled_dial", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if client, err := fileclient.DialFTP(ctx, fileclient.FTPConfig{Host: "127.0.0.1", Port: 1, User: "fixture"}); err == nil {
			client.Close()
			t.Fatal("already canceled FTP dial succeeded")
		} else if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled dial: %v", err)
		}
	})
	t.Run("reject_control_characters", func(t *testing.T) {
		cfg := fileclient.FTPConfig{Host: "127.0.0.1", Port: 1, User: "fixture\r\nDELE anything"}
		if client, err := fileclient.DialFTP(t.Context(), cfg); err == nil {
			client.Close()
			t.Fatal("FTP credentials accepted command delimiters")
		}
		fixture := startFTPProtocolFixture(t, "normal")
		client := openFTPFixture(t, fixture.config)
		if _, err := client.ReadDir(t.Context(), "/\r\nDELE anything"); err == nil {
			t.Fatal("FTP directory accepted command delimiters")
		}
		if _, err := client.InitialPath(t.Context()); err != nil {
			t.Fatalf("argument validation damaged FTP session: %v", err)
		}
	})
}
