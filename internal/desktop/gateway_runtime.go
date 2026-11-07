package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"sshdesk/internal/locales"
	"sshdesk/internal/sshclient"
	"sshdesk/internal/store"
)

type gatewayDialFunc func(context.Context, string, string) (net.Conn, error)

func (f gatewayDialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

func (f gatewayDialFunc) Dial(network, address string) (net.Conn, error) {
	return f(context.Background(), network, address)
}

type gatewayTransport struct {
	ready  chan struct{}
	cancel context.CancelFunc
	conn   *sshclient.Connection
	guard  func()
	err    error
	refs   int
}

// Unlike view-owned target groups, these transports are shared by independent
// targets. The mutex owns the registry and leases; I/O never holds it.
type gatewayPool struct {
	mu      sync.Mutex
	entries map[string]*gatewayTransport
	uses    map[string]int
	keyUses map[string]int
}

func (p *gatewayPool) retainRoute(route []gatewayHop) func() {
	p.mu.Lock()
	if p.uses == nil {
		p.uses = map[string]int{}
		p.keyUses = map[string]int{}
	}
	for _, hop := range route {
		p.uses[hop.ID]++
		if hop.SSH != nil && hop.SSH.KeyID != "" {
			p.keyUses[hop.SSH.KeyID]++
		}
	}
	p.mu.Unlock()
	return sync.OnceFunc(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, hop := range route {
			p.uses[hop.ID]--
			if p.uses[hop.ID] == 0 {
				delete(p.uses, hop.ID)
			}
			if hop.SSH != nil && hop.SSH.KeyID != "" {
				p.keyUses[hop.SSH.KeyID]--
				if p.keyUses[hop.SSH.KeyID] == 0 {
					delete(p.keyUses, hop.SSH.KeyID)
				}
			}
		}
	})
}

func (p *gatewayPool) keyInUse(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.keyUses[id] > 0
}

func (p *gatewayPool) inUse(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.uses[id] > 0
}

func (p *gatewayPool) acquire(
	a *app,
	ctx context.Context,
	key string,
	build func(context.Context) (*sshclient.Connection, error),
) (*sshclient.Connection, func(), error) {
	p.mu.Lock()
	if p.entries == nil {
		p.entries = map[string]*gatewayTransport{}
	}
	entry := p.entries[key]
	if entry != nil && entry.conn != nil {
		select {
		case <-entry.conn.Done():
			entry = nil
		default:
		}
	}
	if entry == nil {
		lifetime, cancel := context.WithCancel(a.ctx)
		entry = &gatewayTransport{ready: make(chan struct{}), cancel: cancel}
		p.entries[key] = entry
		current := entry
		a.sshWorkers.Go(func() {
			connection, err := build(lifetime)
			var guard func()
			if err == nil {
				guard, err = connection.Retain()
			}
			p.mu.Lock()
			current.conn, current.err, current.guard = connection, err, guard
			unused := current.refs == 0
			close(current.ready)
			p.mu.Unlock()
			if err != nil || unused {
				cancel()
				if guard != nil {
					guard()
				}
				if connection != nil {
					connection.Close()
				}
			}
			if connection != nil {
				connection.Wait()
			}
		})
	}
	entry.refs++
	p.mu.Unlock()
	release := sync.OnceFunc(func() {
		p.mu.Lock()
		entry.refs--
		last, guard := entry.refs == 0, entry.guard
		if last && p.entries[key] == entry {
			delete(p.entries, key)
		}
		p.mu.Unlock()
		if last {
			entry.cancel()
			if guard != nil {
				guard()
			}
		}
	})
	select {
	case <-ctx.Done():
		release()
		return nil, nil, ctx.Err()
	case <-entry.ready:
		if entry.err != nil {
			release()
			return nil, nil, entry.err
		}
		return entry.conn, release, nil
	}
}

func gatewayRuntimeKey(route []gatewayHop, steps []sshDialStep, scope string) string {
	var identity strings.Builder
	fmt.Fprintf(&identity, "%q\n", scope)
	sshIndex := 0
	for _, hop := range route {
		fmt.Fprintf(&identity, "%q %q\n", hop.ID, hop.Kind)
		if hop.Kind == "socks5" {
			s := hop.SOCKS
			fmt.Fprintf(&identity, "%q %d %q %t %q\n", s.Address, s.Port, s.Username, s.PasswordSaved, s.PasswordRevision)
			continue
		}
		step := steps[sshIndex]
		sshIndex++
		h := step.endpoint
		keepalive := 0
		if h.session.SSH != nil {
			keepalive = h.session.SSH.KeepAlive
		}
		fmt.Fprintf(&identity, "%q %q %d %q %q %q %t %q %d %q %q\n",
			h.session.ID, h.address, h.port, h.user, h.session.Auth, h.session.KeyID,
			h.session.PasswordSaved, h.session.PasswordRevision, keepalive, step.key.SecretID, step.key.Fingerprint)
	}
	return store.EndpointID([]byte(identity.String()))
}

func (a *app) gatewayDialer(ctx context.Context, plan sshDialPlan) (gatewayDialFunc, func(), error) {
	var dial gatewayDialFunc
	releases := []func(){a.gatewayPool.retainRoute(plan.route)}
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	sshIndex := 0
	for index, hop := range plan.route {
		upstream := dial
		if hop.Kind == "ssh" {
			step := plan.steps[sshIndex]
			sshIndex++
			key := gatewayRuntimeKey(plan.route[:index+1], plan.steps[:sshIndex], plan.gatewayScope)
			if plan.hopProgress != nil {
				plan.hopProgress(index, false)
			}
			connection, stop, err := a.gatewayPool.acquire(a, ctx, key, func(lifetime context.Context) (*sshclient.Connection, error) {
				if plan.promptChanged != nil {
					lifetime = context.WithValue(lifetime, mcpPromptObserverKey{}, plan.promptChanged)
				}
				return a.openSSHStep(lifetime, step, plan.trust, upstream, sshSecretsOverride{}, nil)
			})
			if err != nil {
				release()
				return nil, nil, locales.Errorf(locales.MessageGateway, hop.Name, err)
			}
			releases = append(releases, stop)
			if plan.hopProgress != nil {
				plan.hopProgress(index, true)
			}
			dial = connection.DialContext
			continue
		}
		if plan.hopProgress != nil {
			dial = a.gatewayTestSOCKSDialer(hop, upstream, index, plan.hopProgress)
		} else {
			dial = a.socksGatewayDialer(hop, upstream)
		}
	}
	return dial, release, nil
}

func (a *app) socksGatewayAuth(ctx context.Context, hop gatewayHop) (*proxy.Auth, error) {
	options := *hop.SOCKS
	var auth *proxy.Auth
	if options.Username != "" {
		password := ""
		if options.PasswordSaved {
			data, err := a.vault.Get(ctx, gatewayCredentialKey(hop.ID))
			if err != nil {
				return nil, locales.Errorf(locales.MessageReadPassword, err)
			}
			var credential sessionCredential
			err = json.Unmarshal(data, &credential)
			clear(data)
			if err != nil {
				return nil, err
			}
			if credential.Revision != options.PasswordRevision {
				return nil, locales.NewError(locales.MessageTheSavedSocks5PasswordHasChangedReconnectToUseIt)
			}
			password = credential.Secret
		} else {
			answer := a.askSSH(ctx, &sshPrompt{
				title: locales.Deferred(
					locales.MessageEnterSocks5GatewayPassword,
				), address: hop.Name + " · " + options.Username + "@" + options.Address,
			})
			if !answer.accepted {
				return nil, errSSHConnectCanceled
			}
			password = answer.secret
		}
		if len(password) == 0 || len(password) > 255 {
			return nil, locales.NewError(locales.MessageTheSocks5PasswordMustBeBytes)
		}
		auth = &proxy.Auth{User: options.Username, Password: password}
	}
	return auth, nil
}

func (a *app) socksGatewayDialer(hop gatewayHop, upstream gatewayDialFunc) gatewayDialFunc {
	options := *hop.SOCKS
	initialize := func(ctx context.Context) (proxy.ContextDialer, error) {
		auth, err := a.socksGatewayAuth(ctx, hop)
		if err != nil {
			return nil, err
		}
		if upstream == nil {
			upstream = (&net.Dialer{Timeout: 20 * time.Second}).DialContext
		}
		dialer, err := proxy.SOCKS5(
			"tcp",
			net.JoinHostPort(strings.Trim(options.Address, "[]"), strconv.Itoa(options.Port)),
			auth,
			upstream,
		)
		if err != nil {
			return nil, err
		}
		return dialer.(proxy.ContextDialer), nil
	}
	// Cached downstream SSH transports need no upstream authentication. Prepare
	// once on an actual dial, then reuse the credentials for this FTP client's data connections.
	var once sync.Once
	var dialer proxy.ContextDialer
	var initializeErr error
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		once.Do(func() { dialer, initializeErr = initialize(ctx) })
		if initializeErr != nil {
			return nil, locales.Errorf(locales.MessageGateway, hop.Name, initializeErr)
		}
		connection, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, locales.Errorf(locales.MessageGateway, hop.Name, err)
		}
		return connection, nil
	}
}

func (a *app) openSSHStep(
	ctx context.Context,
	step sshDialStep,
	trust sshclient.HostKeyStore,
	dial gatewayDialFunc,
	override sshSecretsOverride,
	result *connectionTestResult,
) (*sshclient.Connection, error) {
	setupCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cfg, err := a.sshRuntimeConfig(setupCtx, step, override)
	if err != nil {
		return nil, err
	}
	cfg.HostKey = a.sshHostKeyCallback(setupCtx, trust, result)
	if dial != nil {
		cfg.DialContext = dial
	}
	if err := setupCtx.Err(); err != nil {
		return nil, err
	}
	deadline, _ := setupCtx.Deadline()
	cfg.SetupTimeout = time.Until(deadline)
	connection, err := sshclient.Dial(ctx, cfg)
	cfg.Password, cfg.Passphrase, cfg.Signer = "", "", nil
	if err != nil && setupCtx.Err() != nil {
		return nil, setupCtx.Err()
	}
	return connection, err
}
