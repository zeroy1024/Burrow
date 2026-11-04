package desktop

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"sshdesk/internal/fileclient"
	"sshdesk/internal/hostinfo"
	"sshdesk/internal/sshclient"
	"sshdesk/internal/theme"
)

type connectionTestResult struct {
	Summary, Detail, Fingerprint string
	Elapsed                      time.Duration
	startedAt                    time.Time
	Err                          error
	NewHost                      bool
	HostFacts                    *hostinfo.Facts
	HostFactsKey                 string
	HostFactsErr                 error
}

type hostKeyChangedError struct{ old, current string }

func (e *hostKeyChangedError) Error() string {
	return "主机密钥已变更\n原指纹 " + e.old + "\n新指纹 " + e.current
}

// prepareConnectionTest captures catalog and key metadata on the window thread.
// The returned worker shares the runtime's trust and authentication rules.
func (a *app) prepareConnectionTest(h host, password, passphrase string, passphraseEdited bool) func(context.Context) connectionTestResult {
	h = sshHostSnapshot(h)
	var plan sshDialPlan
	var planErr error
	if h.protocol != "ftp" {
		plan, planErr = a.captureSSHPlan(h)
	}
	return func(parent context.Context) connectionTestResult {
		timeout := theme.Current().Motion.Duration.Test
		if h.protocol != "ftp" {
			// Interactive authentication must not inherit the prototype's ten
			// second UI budget while the user is answering server challenges.
			timeout = 2 * time.Minute
		}
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		result := connectionTestResult{startedAt: time.Now()}
		err := planErr
		if err == nil {
			if h.protocol == "ftp" {
				err = testFTP(ctx, h, password, &result)
			} else {
				override := sshSecretsOverride{password: &password}
				if passphraseEdited || passphrase != "" {
					override.passphrase = &passphrase
				}
				var connection *sshclient.Connection
				connection, err = a.dialSSHPlan(ctx, plan, override, &result)
				if err == nil {
					if h.protocol == "sftp" {
						encoding := "UTF-8"
						if h.session.Files != nil {
							encoding = h.session.Files.Encoding
						}
						var client fileclient.Client
						client, err = fileclient.NewSFTP(ctx, connection, fileclient.SFTPOptions{Encoding: encoding})
						if err == nil {
							err = testFileDirectory(ctx, client, h)
							closeErr := client.Close()
							if err == nil {
								err = closeErr
							}
						}
					}
					if h.protocol == "" || h.protocol == "ssh" {
						// Keep the test transport alive until Exec has delivered its
						// result; releasing its final command lease closes the transport.
						var facts hostinfo.Facts
						release, factsErr := connection.Retain()
						if factsErr == nil {
							facts, factsErr = hostinfo.Collect(ctx, sshHostRunner{connection}, connection.ServerVersion(), sshPlatform(h))
							release()
						}
						result.HostFactsErr = factsErr
						if factsErr == nil {
							result.HostFacts, result.HostFactsKey = &facts, hostFactsKey(plan)
						}
					}
					connection.Close()
					connection.Wait()
				}
			}
		}
		if err != nil || result.Elapsed == 0 {
			result.Elapsed = time.Since(result.startedAt)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		result.Err = err
		if err == nil {
			result.Summary = fmt.Sprintf("连接成功 · %dms", result.Elapsed.Milliseconds())
			if result.NewHost {
				result.Summary += " · 已确认主机指纹"
				result.Detail = result.Fingerprint
			}
			if facts := result.HostFacts; facts != nil {
				for _, line := range [][2]string{{"主机名", facts.Hostname}, {"系统", facts.OSName}, {"内核", facts.Kernel},
					{"架构", facts.Arch}, {"默认 Shell", facts.Shell}} {
					if line[1] != "" {
						result.Detail = strings.TrimSpace(result.Detail + "\n" + line[0] + "：" + line[1])
					}
				}
			}
			if result.HostFactsErr != nil {
				result.Summary += " · 基础信息获取失败"
				detail := sanitizeConnectionError(result.HostFactsErr.Error(), password, passphrase)
				result.Detail = strings.TrimSpace(result.Detail + "\n" + detail)
			}
		} else {
			result.Summary = connectionErrorSummary(err)
			result.Detail = fmt.Sprintf("%s\n%dms", sanitizeConnectionError(err.Error(), password, passphrase), result.Elapsed.Milliseconds())
		}
		return result
	}
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

func testFTP(ctx context.Context, h host, password string, result *connectionTestResult) error {
	options := h.session.Files
	if options == nil {
		return errors.New("缺少 FTP 设置")
	}
	user := h.user
	if h.session.Auth == "anonymous" {
		user, password = "anonymous", "anonymous@"
	}
	client, err := fileclient.DialFTP(ctx, fileclient.FTPConfig{
		Host: h.address, Port: h.port, User: user, Password: password,
		TLSMode: options.FTPTLS, Mode: options.FTPMode, Encoding: options.Encoding,
	})
	if err != nil {
		return err
	}
	defer client.Close()
	err = testFileDirectory(ctx, client, h)
	if err == nil {
		result.Elapsed = time.Since(result.startedAt)
	}
	return err
}

func testFileDirectory(ctx context.Context, client fileclient.Client, h host) error {
	initial, err := client.InitialPath(ctx)
	if err != nil {
		return err
	}
	location := initial
	if h.session.Files != nil && h.session.Files.RemoteDir != "" {
		location = remoteFileLocation(initial, h.session.Files.RemoteDir)
	}
	_, err = client.ReadDir(ctx, location)
	return err
}
