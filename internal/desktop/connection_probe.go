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

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type connectionTestResult struct {
	Summary, Detail locales.Text
	Fingerprint     string
	Elapsed         time.Duration
	startedAt       time.Time
	Err             error
	NewHost         bool
	HostFacts       *hostinfo.Facts
	HostFactsKey    string
	HostFactsErr    error
}

type hostKeyChangedError struct{ old, current string }

func (e *hostKeyChangedError) Error() string {
	return locales.MessageHostKeyChangedPreviousFingerprint() + e.old + locales.MessageNewFingerprint() + e.current
}

// prepareConnectionTest captures catalog and key metadata on the window thread.
// The returned worker shares the runtime's trust and authentication rules.
func (a *app) prepareConnectionTest(h host, password, passphrase string, passphraseEdited bool) func(context.Context) connectionTestResult {
	h = sshHostSnapshot(h)
	var plan sshDialPlan
	var planErr error
	plan, planErr = a.captureSSHPlan(h)
	return func(parent context.Context) connectionTestResult {
		timeout := theme.Current().Motion.Duration.Test
		interactive := h.protocol != "ftp" || h.session.Auth == "password" && password == ""
		if interactive || len(plan.route) > 0 {
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
				var dial gatewayDialFunc
				var release func()
				dial, release, err = a.gatewayDialer(ctx, plan)
				if err == nil {
					defer release()
					if password == "" && h.session.Auth != "anonymous" {
						answer := a.askSSH(
							ctx,
							&sshPrompt{title: locales.Deferred(locales.MessageEnterFtpPassword), address: h.user + "@" + h.address},
						)
						if answer.accepted {
							password = answer.secret
						} else {
							err = errSSHConnectCanceled
						}
					}
					if err == nil {
						err = testFTP(ctx, h, password, &result, dial)
					}
				}
			} else {
				override := sshSecretsOverride{}
				if password != "" || h.session.PasswordSaved {
					override.password = &password
				}
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
			result.Summary = locales.Format(locales.MessageConnectedMs, result.Elapsed.Milliseconds())
			if result.NewHost {
				result.Summary = locales.Concat(result.Summary, locales.Deferred(locales.MessageHostFingerprintVerified))
				result.Detail = locales.Raw(result.Fingerprint)
			}
			if facts := result.HostFacts; facts != nil {
				for _, line := range []struct {
					label func() string
					value string
				}{
					{label: locales.MessageHostname, value: facts.Hostname},
					{label: locales.MessageOperatingSystem, value: facts.OSName},
					{label: locales.MessageKernel, value: facts.Kernel},
					{label: locales.MessageArchitecture, value: facts.Arch},
					{label: locales.MessageDefaultShell, value: facts.Shell},
				} {
					if line.value != "" {
						result.Detail = locales.Concat(
							result.Detail,
							locales.Raw("\n"),
							locales.Deferred(line.label),
							locales.Raw(": "+line.value),
						).TrimSpace()
					}
				}
			}
			if result.HostFactsErr != nil {
				result.Summary = locales.Concat(result.Summary, locales.Deferred(locales.MessageUnableToRetrieveHostInformation))
				detail := sanitizeConnectionError(result.HostFactsErr.Error(), password, passphrase)
				result.Detail = locales.Concat(result.Detail, locales.Raw("\n"+detail)).TrimSpace()
			}
		} else {
			result.Summary = locales.Deferred(func() string { return connectionErrorSummary(err) })
			result.Detail = locales.Raw(
				fmt.Sprintf("%s\n%dms", sanitizeConnectionError(err.Error(), password, passphrase), result.Elapsed.Milliseconds()),
			)
		}
		return result
	}
}

func sanitizeConnectionError(detail string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, locales.MessageHidden())
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
		return locales.MessageHostKeyChanged()
	case errors.Is(err, context.DeadlineExceeded):
		return locales.MessageConnectionTimedOut()
	case errors.Is(err, context.Canceled), errors.Is(err, errSSHConnectCanceled):
		return locales.MessageTestCancelled()
	case errors.As(err, &cert), errors.As(err, &unknown):
		return locales.MessageUntrustedCertificate()
	case strings.Contains(err.Error(), "unable to authenticate"), strings.Contains(err.Error(), "530"), strings.Contains(err.Error(), "no supported methods"):
		return locales.MessageAuthenticationFailed()
	case strings.Contains(err.Error(), "connection refused"):
		return locales.MessageConnectionRefused()
	case errors.As(err, &op):
		if op.Timeout() {
			return locales.MessageConnectionTimedOut()
		}
		return locales.MessageHostUnreachable()
	default:
		return locales.MessageProtocolError()
	}
}

func closeOnCancel(ctx context.Context, conn net.Conn) func() bool {
	return context.AfterFunc(ctx, func() { conn.Close() })
}

func testFTP(ctx context.Context, h host, password string, result *connectionTestResult, dial gatewayDialFunc) error {
	options := h.session.Files
	if options == nil {
		return locales.NewError(locales.MessageMissingFtpSettings)
	}
	user := h.user
	if h.session.Auth == "anonymous" {
		user, password = "anonymous", "anonymous@"
	}
	client, err := fileclient.DialFTP(ctx, fileclient.FTPConfig{
		DialContext: dial,
		Host:        h.address, Port: h.port, User: user, Password: password,
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
