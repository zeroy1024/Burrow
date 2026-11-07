package desktop

import (
	"context"
	"encoding/json"
	"sync"

	"sshdesk/internal/locales"
	"sshdesk/internal/store"
	"sshdesk/internal/wggateway"
)

type wgGatewayTransport struct {
	ready, done chan struct{}
	cancel      context.CancelFunc
	tunnel      *wggateway.Tunnel
	configKey   string
	err         error
	refs        int
	closing     bool
}

func wgCredentialKey(id string) string { return "gateway/" + id + "/wireguard" }

// WireGuard sharing is independent of SSH/MCP scopes. Two UDP devices with
// the same client/peer identity would compete for the peer's roaming endpoint.
func (a *app) acquireWGGateway(ctx context.Context, hop gatewayHop) (*wggateway.Tunnel, func(), error) {
	p := &a.gatewayPool
	config := *hop.WG
	data, _ := json.Marshal(config)
	configKey, identity := store.EndpointID(data), config.Identity()
	for {
		p.mu.Lock()
		if p.wireguard == nil {
			p.wireguard = map[string]*wgGatewayTransport{}
		}
		entry := p.wireguard[identity]
		if entry != nil && entry.closing {
			done := entry.done
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-done:
				continue
			}
		}
		if entry != nil && entry.configKey != configKey {
			p.mu.Unlock()
			return nil, nil, locales.NewError(locales.WgConflictingIdentity)
		}
		if entry == nil {
			lifetime, cancel := context.WithCancel(a.ctx)
			entry = &wgGatewayTransport{ready: make(chan struct{}), done: make(chan struct{}), cancel: cancel, configKey: configKey}
			p.wireguard[identity] = entry
			current := entry
			a.sshWorkers.Go(func() {
				credential, err := a.wgGatewayCredential(lifetime, hop)
				var tunnel *wggateway.Tunnel
				if err == nil {
					tunnel, err = wggateway.Open(lifetime, config, credential)
				}
				credential.PrivateKey, credential.PresharedKey = "", ""
				p.mu.Lock()
				current.tunnel, current.err = tunnel, err
				unused := current.refs == 0
				close(current.ready)
				p.mu.Unlock()
				if err != nil || unused {
					cancel()
					if tunnel != nil {
						tunnel.Close()
					}
				}
				if tunnel != nil {
					<-tunnel.Done()
				}
				cancel()
				p.mu.Lock()
				delete(p.wireguard, identity)
				close(current.done)
				p.mu.Unlock()
			})
		}
		entry.refs++
		p.mu.Unlock()
		release := sync.OnceFunc(func() {
			p.mu.Lock()
			entry.refs--
			last, tunnel := entry.refs == 0, entry.tunnel
			if last {
				entry.closing = true
			}
			p.mu.Unlock()
			if last {
				entry.cancel()
				if tunnel != nil {
					tunnel.Close()
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
			return entry.tunnel, release, nil
		}
	}
}

func (a *app) wgGatewayCredential(ctx context.Context, hop gatewayHop) (wggateway.Credential, error) {
	data, err := a.vault.Get(ctx, wgCredentialKey(hop.ID))
	if err != nil {
		return wggateway.Credential{}, locales.Errorf(locales.WgReadCredential, err)
	}
	defer clear(data)
	var credential wggateway.Credential
	if err := json.Unmarshal(data, &credential); err != nil {
		return credential, err
	}
	if credential.Revision != hop.WG.CredentialRevision {
		return wggateway.Credential{}, locales.NewError(locales.WgCredentialChanged)
	}
	return credential, nil
}
