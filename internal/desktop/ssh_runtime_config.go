package desktop

import (
	"context"
	"errors"
	"net"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/vault"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshDialStep struct {
	endpoint host
	key      managedKey
}

// Plans contain value snapshots only. Workers never inspect the live catalogs
// while the window thread edits or reorders them.
type sshDialPlan struct {
	route         []gatewayHop
	gatewayScope  string
	promptChanged func(bool, string)
	// hopProgress observes actual connection/authentication stages; a ready SSH
	// gateway also proves the upstream hops that its transport passed through.
	hopProgress func(index int, ready bool)
	steps       []sshDialStep
	trust       sshclient.HostKeyStore
}

type sshSecretsOverride struct {
	password, passphrase *string
}

func sshHostSnapshot(h host) host {
	if h.session.SSH != nil {
		options := *h.session.SSH
		options.Env = slices.Clone(options.Env)
		h.session.SSH = &options
	}
	if h.session.Files != nil {
		options := *h.session.Files
		h.session.Files = &options
	}
	return h
}

// captureSSHPlan is called on the window thread before starting a worker.
func (a *app) captureSSHPlan(endpoint host) (sshDialPlan, error) {
	route, err := a.captureGatewayRoute(endpoint)
	if err != nil {
		return sshDialPlan{}, err
	}
	plan, err := a.captureGatewayPlan(route)
	if err != nil {
		return plan, err
	}
	step := sshDialStep{endpoint: sshHostSnapshot(endpoint)}
	if endpoint.session.Auth == "key" {
		step.key, err = a.keyByID(endpoint.session.KeyID)
		if err != nil {
			return plan, err
		}
	}
	plan.steps = append(plan.steps, step)
	return plan, nil
}

func (a *app) captureGatewayPlan(route []gatewayHop) (sshDialPlan, error) {
	plan := sshDialPlan{trust: a.store, route: route}
	for _, hop := range route {
		if hop.Kind != "ssh" {
			continue
		}
		h := transferStoredHost(*hop.SSH)
		step := sshDialStep{endpoint: h}
		if h.session.Auth == "key" {
			var err error
			step.key, err = a.keyByID(h.session.KeyID)
			if err != nil {
				return plan, err
			}
		}
		plan.steps = append(plan.steps, step)
	}
	return plan, nil
}

func (a *app) dialSSHPlan(ctx context.Context, plan sshDialPlan, override sshSecretsOverride, result *connectionTestResult) (*sshclient.Connection, error) {
	if plan.promptChanged != nil {
		ctx = context.WithValue(ctx, mcpPromptObserverKey{}, plan.promptChanged)
	}
	dial, release, err := a.gatewayDialer(ctx, plan)
	if err != nil {
		return nil, err
	}
	connection, err := a.openSSHStep(ctx, plan.steps[len(plan.steps)-1], plan.trust, dial, override, result)
	if err != nil {
		release()
		return nil, err
	}
	a.sshWorkers.Go(func() { connection.Wait(); release() })
	return connection, nil
}

func (a *app) sshRuntimeConfig(ctx context.Context, step sshDialStep, override sshSecretsOverride) (sshclient.Config, error) {
	h := step.endpoint
	cfg := sshclient.Config{Host: strings.Trim(h.address, "[]"), Port: h.port, User: h.user, Auth: h.session.Auth, KeepAlive: 30 * time.Second}
	if cfg.User == "" {
		current, err := user.Current()
		if err != nil {
			return cfg, err
		}
		cfg.User = current.Username
	}
	if options := h.session.SSH; options != nil {
		cfg.KeepAlive = time.Duration(options.KeepAlive) * time.Second
		cfg.HealthCheck = time.Duration(options.HealthCheck) * time.Second
	}
	address := cfg.User + "@" + net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	cfg.KeyboardInteractive = func(user, instruction string, questions []string, echoes []bool) ([]string, error) {
		answers := make([]string, 0, len(questions))
		for index, question := range questions {
			answer := a.askSSH(ctx, &sshPrompt{
				title: locales.Deferred(
					locales.MessageSshInteractiveAuthentication,
				), address: address, detail: locales.Raw(
					instruction + "\n" + question,
				),
				echo: index < len(echoes) && echoes[index],
			})
			if !answer.accepted {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, errSSHConnectCanceled
			}
			answers = append(answers, answer.secret)
		}
		return answers, nil
	}
	switch cfg.Auth {
	case "", "password":
		cfg.Auth = "password"
		if override.password != nil {
			cfg.Password = *override.password
			return cfg, nil
		}
		if h.session.PasswordSaved {
			secret, err := a.sessionPassword(h)
			if err == nil {
				cfg.Password = secret
				return cfg, nil
			}
			if !errors.Is(err, vault.ErrNotFound) {
				return cfg, locales.Errorf(locales.MessageReadSessionPassword, err)
			}
		}
		answer := a.askSSH(ctx, &sshPrompt{title: locales.Deferred(locales.MessageEnterSshPassword), address: address})
		if !answer.accepted {
			if ctx.Err() != nil {
				return cfg, ctx.Err()
			}
			return cfg, errSSHConnectCanceled
		}
		cfg.Password = answer.secret
	case "key":
		data, err := a.vault.Get(ctx, step.key.SecretID)
		if err != nil {
			return cfg, locales.Errorf(locales.MessageReadPrivateKey, err)
		}
		defer clear(data)
		cfg.Signer, err = ssh.ParsePrivateKey(data)
		var encrypted *ssh.PassphraseMissingError
		if err == nil {
			return cfg, nil
		}
		if !errors.As(err, &encrypted) {
			return cfg, locales.Errorf(locales.MessageParsePrivateKey, err)
		}
		if override.passphrase != nil {
			cfg.Passphrase = *override.passphrase
		} else {
			secret, err := a.vault.Get(ctx, "key/"+step.key.ID+"/passphrase")
			if err != nil && !errors.Is(err, vault.ErrNotFound) {
				return cfg, locales.Errorf(locales.MessageReadPrivateKeyPassphrase, err)
			}
			cfg.Passphrase = string(secret)
			clear(secret)
		}
		if cfg.Passphrase == "" {
			answer := a.askSSH(
				ctx,
				&sshPrompt{title: locales.Deferred(locales.MessageEnterPrivateKeyPassphrase), address: address},
			)
			if !answer.accepted {
				if ctx.Err() != nil {
					return cfg, ctx.Err()
				}
				return cfg, errSSHConnectCanceled
			}
			cfg.Passphrase = answer.secret
		}
		cfg.Signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(cfg.Passphrase))
		cfg.Passphrase = ""
		if err != nil {
			return cfg, locales.Errorf(locales.MessageParsePrivateKey, err)
		}
	case "agent":
	default:
		return cfg, locales.Errorf(locales.MessageUnsupportedSshAuthenticationMethod, cfg.Auth)
	}
	return cfg, nil
}

func (a *app) sshHostKeyCallback(ctx context.Context, trust sshclient.HostKeyStore, result *connectionTestResult) ssh.HostKeyCallback {
	verify := sshclient.KnownHosts(ctx, trust, func(hostname, fingerprint, keyType string) bool {
		accepted := a.askSSH(ctx, &sshPrompt{title: locales.Deferred(
			locales.MessageTrustTheServerSHostKey,
		), address: hostname,
			fingerprint: fingerprint, keyType: keyType}).accepted
		if accepted && result != nil {
			result.NewHost, result.Fingerprint = true, fingerprint
		}
		return accepted
	})
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := verify(hostname, remote, key)
		var mismatch *knownhosts.KeyError
		if errors.As(err, &mismatch) && len(mismatch.Want) > 0 {
			return &hostKeyChangedError{old: ssh.FingerprintSHA256(mismatch.Want[0].Key), current: ssh.FingerprintSHA256(key)}
		}
		return err
	}
}
