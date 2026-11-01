package desktop

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type connectionTestResult struct {
	Summary, Detail, Fingerprint string
	Elapsed                      time.Duration
	startedAt                    time.Time
	Err                          error
	NewHost                      bool
}

type hostKeyChangedError struct{ old, current string }

func (e *hostKeyChangedError) Error() string {
	return "主机密钥已变更\n原指纹 " + e.old + "\n新指纹 " + e.current
}

// Each test owns its sockets. Cancellation closes them even while an SSH
// handshake or an FTP reply is blocked; no result is written back to UI state.
func (a *app) testConnection(parent context.Context, h host, password, passphrase string, passphraseEdited bool) connectionTestResult {
	ctx, cancel := context.WithTimeout(parent, theme.Current().Motion.Duration.Test)
	defer cancel()
	started := time.Now()
	result := connectionTestResult{startedAt: started}
	var err error
	if h.protocol == "ftp" {
		err = testFTP(ctx, h, password, &result)
	} else {
		var client *ssh.Client
		client, err = a.testSSH(ctx, h, password, passphrase, &result, map[string]bool{}, !passphraseEdited)
		if err == nil {
			if h.protocol == "sftp" {
				var session *ssh.Session
				session, err = client.NewSession()
				if err == nil {
					err = session.RequestSubsystem("sftp")
					session.Close()
				}
			}
			client.Close()
		}
	}
	if err != nil || result.Elapsed == 0 {
		result.Elapsed = time.Since(started)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	result.Err = err
	if err == nil {
		result.Summary = fmt.Sprintf("连接成功 · %dms", result.Elapsed.Milliseconds())
		if result.NewHost {
			result.Summary += " · 新主机指纹"
			result.Detail = result.Fingerprint
		}
	} else {
		result.Summary = connectionErrorSummary(err)
		result.Detail = fmt.Sprintf("%s\n%dms", sanitizeConnectionError(err.Error(), password, passphrase), result.Elapsed.Milliseconds())
	}
	return result
}

func sanitizeConnectionError(detail string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[已隐藏]")
		}
	}
	return detail
}

func connectionErrorSummary(err error) string {
	var changed *hostKeyChangedError
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var op *net.OpError
	switch {
	case errors.As(err, &changed):
		return "主机密钥已变更"
	case errors.Is(err, context.DeadlineExceeded):
		return "连接超时"
	case errors.Is(err, context.Canceled):
		return "测试已取消"
	case errors.As(err, &cert), errors.As(err, &unknown):
		return "证书不受信任"
	case strings.Contains(err.Error(), "unable to authenticate"), strings.Contains(err.Error(), "530"), strings.Contains(err.Error(), "no supported methods"):
		return "认证失败"
	case strings.Contains(err.Error(), "connection refused"):
		return "连接被拒绝"
	case errors.As(err, &op):
		if op.Timeout() {
			return "连接超时"
		}
		return "主机不可达"
	default:
		return "协议错误"
	}
}

func closeOnCancel(ctx context.Context, conn net.Conn) func() bool {
	return context.AfterFunc(ctx, func() { conn.Close() })
}

func (a *app) testSSH(ctx context.Context, h host, password, passphrase string, result *connectionTestResult, seen map[string]bool, useSavedPassphrase bool) (*ssh.Client, error) {
	if seen[h.session.ID] {
		return nil, errors.New("SSH 跳板机形成环路")
	}
	seen[h.session.ID] = true
	username := h.user
	if username == "" {
		current, err := user.Current()
		if err != nil {
			return nil, err
		}
		username = current.Username
	}
	auth, cleanup, err := a.sshAuthentication(ctx, h, password, passphrase, useSavedPassphrase)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	address := net.JoinHostPort(strings.Trim(h.address, "[]"), strconv.Itoa(h.port))
	var conn net.Conn
	var jump *ssh.Client
	returned := false
	defer func() {
		if jump != nil && !returned {
			jump.Close()
		}
	}()
	if options := h.session.SSH; options != nil && options.JumpHostID != "" {
		found := false
		for _, candidate := range a.hosts {
			if candidate.session.ID == options.JumpHostID {
				found = true
				p, e := a.sessionPassword(candidate)
				if e != nil {
					return nil, e
				}
				jump, e = a.testSSH(ctx, candidate, p, "", result, seen, true)
				if e != nil {
					return nil, e
				}
				break
			}
		}
		if !found {
			return nil, errors.New("跳板机会话不存在")
		}
		stopJump := context.AfterFunc(ctx, func() { jump.Close() })
		defer stopJump()
		conn, err = jump.DialContext(ctx, "tcp", address)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}
	stop := closeOnCancel(ctx, conn)
	callback, err := testHostKeyCallback(result)
	if err != nil {
		stop()
		conn.Close()
		return nil, err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, &ssh.ClientConfig{User: username, Auth: auth, HostKeyCallback: callback, Timeout: time.Until(deadline)})
	if err != nil {
		stop()
		conn.Close()
		return nil, err
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	result.Elapsed = time.Since(result.startedAt)
	// Keep cancellation attached until the caller closes the authenticated client.
	go func() {
		client.Wait()
		stop()
		if jump != nil {
			jump.Close()
		}
	}()
	returned = true
	return client, nil
}

func (a *app) sshAuthentication(ctx context.Context, h host, password, passphrase string, useSavedPassphrase bool) ([]ssh.AuthMethod, func(), error) {
	noop := func() {}
	switch h.session.Auth {
	case "", "password":
		return []ssh.AuthMethod{ssh.Password(password)}, noop, nil
	case "key":
		key, err := a.keyByID(h.session.KeyID)
		if err != nil {
			return nil, noop, err
		}
		data, err := os.ReadFile(key.Path)
		if err != nil {
			return nil, noop, err
		}
		defer clear(data)
		var signer ssh.Signer
		if passphrase == "" && useSavedPassphrase {
			secret, e := mygo.Secrets.Get("key/" + key.ID + "/passphrase")
			if e != nil && !errors.Is(e, mygo.ErrSecretNotFound) {
				return nil, noop, e
			}
			passphrase = string(secret)
			clear(secret)
		}
		if passphrase == "" {
			signer, err = ssh.ParsePrivateKey(data)
		} else {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
		}
		if err != nil {
			return nil, noop, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, noop, nil
	case "agent":
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return nil, noop, errors.New("未检测到 ssh-agent（SSH_AUTH_SOCK）")
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, noop, err
		}
		deadline, _ := ctx.Deadline()
		if err := conn.SetDeadline(deadline); err != nil {
			conn.Close()
			return nil, noop, err
		}
		stop := closeOnCancel(ctx, conn)
		signers, err := agent.NewClient(conn).Signers()
		if err != nil {
			stop()
			conn.Close()
			return nil, noop, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, func() { stop(); conn.Close() }, nil
	default:
		return nil, noop, errors.New("不支持的认证方式")
	}
}

func testHostKeyCallback(result *connectionTestResult) (ssh.HostKeyCallback, error) {
	directory, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, ".ssh", "known_hosts")
	var verify ssh.HostKeyCallback
	if _, err := os.Stat(path); err == nil {
		verify, err = knownhosts.New(path)
		if err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if verify != nil {
			err := verify(hostname, remote, key)
			if err == nil {
				return nil
			}
			var mismatch *knownhosts.KeyError
			if !errors.As(err, &mismatch) {
				return err
			}
			if len(mismatch.Want) > 0 {
				return &hostKeyChangedError{old: ssh.FingerprintSHA256(mismatch.Want[0].Key), current: fingerprint}
			}
		}
		// A test reports unknown keys without trusting or persisting them.
		result.NewHost, result.Fingerprint = true, fingerprint
		return nil
	}, nil
}

func testFTP(ctx context.Context, h host, password string, result *connectionTestResult) error {
	options := h.session.Files
	if options == nil {
		return errors.New("缺少 FTP 设置")
	}
	username := h.user
	if h.session.Auth == "anonymous" {
		username = "anonymous"
		password = "anonymous" + "@"
	}
	for _, value := range []string{username, password} {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("FTP 凭据不能包含换行")
		}
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(strings.Trim(h.address, "[]"), strconv.Itoa(h.port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := closeOnCancel(ctx, conn)
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	config := &tls.Config{ServerName: strings.Trim(h.address, "[]"), MinVersion: tls.VersionTLS12}
	if options.FTPTLS == "implicit" {
		secure := tls.Client(conn, config)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	control := textproto.NewConn(conn)
	if _, _, err := control.ReadResponse(220); err != nil {
		return err
	}
	command := func(expect int, format string, args ...any) (int, error) {
		id, err := control.Cmd(format, args...)
		if err != nil {
			return 0, err
		}
		control.StartResponse(id)
		defer control.EndResponse(id)
		code, _, err := control.ReadResponse(expect)
		return code, err
	}
	if options.FTPTLS == "explicit" {
		if _, err := command(234, "AUTH TLS"); err != nil {
			return err
		}
		secure := tls.Client(conn, config)
		if err := secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
		control = textproto.NewConn(conn)
	}
	code, err := command(0, "USER %s", username)
	if err != nil {
		return err
	}
	if code == 331 {
		if _, err = command(230, "PASS %s", password); err != nil {
			return err
		}
	} else if code != 230 {
		return fmt.Errorf("FTP USER: %d", code)
	}
	result.Elapsed = time.Since(result.startedAt)
	if options.FTPMode == "active" {
		local := conn.LocalAddr().(*net.TCPAddr)
		listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: local.IP})
		if err != nil {
			return err
		}
		defer listener.Close()
		port := listener.Addr().(*net.TCPAddr).Port
		if ip := local.IP.To4(); ip != nil {
			_, err = command(200, "PORT %d,%d,%d,%d,%d,%d", ip[0], ip[1], ip[2], ip[3], port/256, port%256)
		} else {
			_, err = command(200, "EPRT |2|%s|%d|", local.IP.String(), port)
		}
		if err != nil {
			return err
		}
	} else {
		if conn.LocalAddr().(*net.TCPAddr).IP.To4() != nil {
			_, err = command(227, "PASV")
		} else {
			_, err = command(229, "EPSV")
		}
		if err != nil {
			return err
		}
	}
	if _, err = command(200, "NOOP"); err != nil {
		return err
	}
	_, err = command(221, "QUIT")
	return err
}
