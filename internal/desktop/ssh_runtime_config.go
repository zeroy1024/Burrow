package desktop

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"sshdesk/internal/sshclient"

	"github.com/egoist/mygo"
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
	steps      []sshDialStep
	knownHosts string
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
	directory, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return sshDialPlan{}, fmt.Errorf("定位 SSH 配置：%w", err)
	}
	plan := sshDialPlan{knownHosts: filepath.Join(directory, "known_hosts")}
	seen := map[string]bool{}
	for {
		endpoint = sshHostSnapshot(endpoint)
		if endpoint.session.ID != "" {
			if seen[endpoint.session.ID] {
				return sshDialPlan{}, errors.New("SSH 跳板机形成环路")
			}
			seen[endpoint.session.ID] = true
		}
		step := sshDialStep{endpoint: endpoint}
		if endpoint.session.Auth == "key" {
			step.key, err = a.keyByID(endpoint.session.KeyID)
			if err != nil {
				return sshDialPlan{}, err
			}
		}
		plan.steps = append(plan.steps, step)
		options := endpoint.session.SSH
		if options == nil || options.JumpHostID == "" {
			break
		}
		index := slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == options.JumpHostID })
		if index == -1 || a.hosts[index].protocol != "" {
			return sshDialPlan{}, errors.New("SSH 跳板机会话不存在")
		}
		endpoint = a.hosts[index]
	}
	slices.Reverse(plan.steps)
	return plan, nil
}

func (a *app) dialSSHPlan(ctx context.Context, plan sshDialPlan, override sshSecretsOverride, result *connectionTestResult) (*sshclient.Connection, error) {
	var connection *sshclient.Connection
	var chain []*sshclient.Connection
	success := false
	defer func() {
		if !success {
			for _, opened := range chain {
				opened.Close()
			}
		}
	}()
	for i, step := range plan.steps {
		setupCtx, cancelSetup := context.WithTimeout(ctx, 2*time.Minute)
		secrets := sshSecretsOverride{}
		if i == len(plan.steps)-1 {
			secrets = override
		}
		cfg, err := a.sshRuntimeConfig(setupCtx, step, secrets)
		if err != nil {
			cancelSetup()
			return nil, err
		}
		cfg.HostKey = a.sshHostKeyCallback(setupCtx, plan.knownHosts, result)
		if connection != nil {
			cfg.DialContext = connection.DialContext
		}
		if err := setupCtx.Err(); err != nil {
			cancelSetup()
			return nil, err
		}
		deadline, _ := setupCtx.Deadline()
		cfg.SetupTimeout = time.Until(deadline)
		if cfg.SetupTimeout <= 0 {
			cancelSetup()
			return nil, context.DeadlineExceeded
		}
		connection, err = sshclient.Dial(ctx, cfg)
		cfg.Password, cfg.Passphrase = "", ""
		setupErr := setupCtx.Err()
		cancelSetup()
		if err != nil {
			if setupErr != nil {
				return nil, setupErr
			}
			return nil, err
		}
		chain = append(chain, connection)
		opened := connection
		a.sshWorkers.Go(func() { opened.Wait() })
	}
	success = true
	return connection, nil
}

func (a *app) sshRuntimeConfig(ctx context.Context, step sshDialStep, override sshSecretsOverride) (sshclient.Config, error) {
	h := step.endpoint
	cfg := sshclient.Config{Host: strings.Trim(h.address, "[]"), Port: h.port, User: h.user, Auth: h.session.Auth}
	if cfg.User == "" {
		current, err := user.Current()
		if err != nil {
			return cfg, err
		}
		cfg.User = current.Username
	}
	if options := h.session.SSH; options != nil {
		cfg.KeepAlive = time.Duration(options.KeepAlive) * time.Second
	}
	address := cfg.User + "@" + net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	cfg.KeyboardInteractive = func(user, instruction string, questions []string, echoes []bool) ([]string, error) {
		answers := make([]string, 0, len(questions))
		for index, question := range questions {
			answer := a.askSSH(ctx, &sshPrompt{
				title: "SSH 交互认证", address: address, detail: instruction + "\n" + question,
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
			if !errors.Is(err, mygo.ErrSecretNotFound) {
				return cfg, fmt.Errorf("读取会话密码：%w", err)
			}
		}
		answer := a.askSSH(ctx, &sshPrompt{title: "输入 SSH 密码", address: address})
		if !answer.accepted {
			if ctx.Err() != nil {
				return cfg, ctx.Err()
			}
			return cfg, errSSHConnectCanceled
		}
		cfg.Password = answer.secret
	case "key":
		cfg.KeyPath = step.key.Path
		data, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return cfg, fmt.Errorf("读取私钥：%w", err)
		}
		_, err = ssh.ParsePrivateKey(data)
		clear(data)
		var encrypted *ssh.PassphraseMissingError
		if err == nil {
			return cfg, nil
		}
		if !errors.As(err, &encrypted) {
			return cfg, fmt.Errorf("解析私钥：%w", err)
		}
		if override.passphrase != nil {
			cfg.Passphrase = *override.passphrase
		} else {
			secret, err := mygo.Secrets.Get("key/" + step.key.ID + "/passphrase")
			if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
				return cfg, fmt.Errorf("读取私钥口令：%w", err)
			}
			cfg.Passphrase = string(secret)
			clear(secret)
		}
		if cfg.Passphrase == "" {
			answer := a.askSSH(ctx, &sshPrompt{title: "输入私钥口令", address: address})
			if !answer.accepted {
				if ctx.Err() != nil {
					return cfg, ctx.Err()
				}
				return cfg, errSSHConnectCanceled
			}
			cfg.Passphrase = answer.secret
		}
	case "agent":
	default:
		return cfg, fmt.Errorf("不支持的 SSH 认证方式 %q", cfg.Auth)
	}
	return cfg, nil
}

func (a *app) sshHostKeyCallback(ctx context.Context, path string, result *connectionTestResult) ssh.HostKeyCallback {
	verify := sshclient.KnownHosts(path, func(hostname, fingerprint, keyType string) bool {
		accepted := a.askSSH(ctx, &sshPrompt{title: "信任服务器主机密钥？", address: hostname,
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
