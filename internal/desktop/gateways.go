package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"sshdesk/internal/store"
)

const directGateway = "direct"

type socksGateway struct {
	Address          string `json:"address"`
	Port             int    `json:"port"`
	Username         string `json:"username,omitempty"`
	PasswordSaved    bool   `json:"passwordSaved,omitempty"`
	PasswordRevision string `json:"passwordRevision,omitempty"`
}

type gatewayConfig struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Kind         string        `json:"kind"`
	UpstreamID   string        `json:"upstreamId,omitempty"` // SOCKS5 only; SSH follows its source host
	SSHProfileID string        `json:"sshProfileId,omitempty"`
	SOCKS        *socksGateway `json:"socks,omitempty"`
}

// Captured routes contain endpoint and credential references, never passwords.
// A non-nil empty route means explicitly captured direct access.
type gatewayHop struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Kind  string         `json:"kind"`
	SSH   *storedSession `json:"ssh,omitempty"`
	SOCKS *socksGateway  `json:"socks,omitempty"`
}

func gatewayCredentialKey(id string) string { return "gateway/" + id + "/credential" }

func gatewayReference(selection string) string {
	if selection == directGateway {
		return ""
	}
	return selection
}

func sameGatewayRoute(left, right *[]gatewayHop) bool {
	key := func(value *[]gatewayHop) string {
		route := []gatewayHop{}
		if value != nil {
			route = *value
		}
		steps := []sshDialStep{}
		for _, hop := range route {
			if hop.SSH != nil {
				steps = append(steps, sshDialStep{endpoint: transferStoredHost(*hop.SSH)})
			}
		}
		return gatewayRuntimeKey(route, steps, "")
	}
	return key(left) == key(right)
}

// gatewayCatalog resolves routes over one catalog state. A host is reached
// through its own gateway selection, and an SSH gateway follows the route to
// its source host, as OpenSSH resolves a ProxyJump host with that host's own
// configuration; only a SOCKS5 gateway selects an upstream of its own.
type gatewayCatalog struct {
	gateways []gatewayConfig
	hosts    []host
	fallback string // the application default gateway
}

func (a *app) gatewayCatalog() gatewayCatalog {
	return gatewayCatalog{gateways: a.gateways, hosts: a.hosts, fallback: a.defaultGateway}
}

// withGateway resolves an unsaved gateway in place of its saved version.
func (r gatewayCatalog) withGateway(g gatewayConfig) gatewayCatalog {
	r.gateways = slices.Clone(r.gateways)
	if index := slices.IndexFunc(r.gateways, func(value gatewayConfig) bool { return value.ID == g.ID }); index < 0 {
		r.gateways = append(r.gateways, g)
	} else {
		r.gateways[index] = g
	}
	return r
}

func (r gatewayCatalog) gateway(id string) (gatewayConfig, bool) {
	index := slices.IndexFunc(r.gateways, func(g gatewayConfig) bool { return g.ID == id })
	if index < 0 {
		return gatewayConfig{}, false
	}
	return r.gateways[index], true
}

func (r gatewayCatalog) sshHost(id string) (host, bool) {
	index := slices.IndexFunc(r.hosts, func(h host) bool { return h.session.ID == id && h.protocol == "" })
	if index < 0 {
		return host{}, false
	}
	return r.hosts[index], true
}

// gatewayLoop names the host or gateway at which a route came back to itself.
type gatewayLoop struct{ key, message string }

func (e *gatewayLoop) Error() string { return e.message }

// hostRoute returns the gateway IDs, first hop first, that reach the host
// with this session ID and gateway selection. A host that follows the default
// is reached directly when the default route itself passes through it, as the
// exit's own source host does; an explicit selection that leads back to the
// host is a loop.
func (r gatewayCatalog) hostRoute(id, selection string, visiting []string) ([]string, error) {
	if id != "" {
		key := "host:" + id
		if slices.Contains(visiting, key) {
			name := id
			if index := slices.IndexFunc(r.hosts, func(h host) bool { return h.session.ID == id }); index >= 0 {
				name = r.hosts[index].name
			}
			return nil, &gatewayLoop{key: key, message: fmt.Sprintf("主机「%s」的连接路径会经过它自己", name)}
		}
		visiting = append(visiting[:len(visiting):len(visiting)], key)
	}
	switch selection {
	case directGateway:
		return []string{}, nil
	case "":
		if r.fallback == "" {
			return []string{}, nil
		}
		route, err := r.gatewayRoute(r.fallback, visiting)
		var loop *gatewayLoop
		if id != "" && errors.As(err, &loop) && slices.Contains(visiting, loop.key) {
			return []string{}, nil
		}
		return route, err
	}
	return r.gatewayRoute(selection, visiting)
}

// gatewayRoute returns the gateway IDs, first hop first, ending with id.
func (r gatewayCatalog) gatewayRoute(id string, visiting []string) ([]string, error) {
	g, ok := r.gateway(id)
	if !ok {
		return nil, fmt.Errorf("网关不存在：%s", id)
	}
	key := "gateway:" + id
	if slices.Contains(visiting, key) {
		return nil, &gatewayLoop{key: key, message: fmt.Sprintf("网关「%s」形成环路", g.Name)}
	}
	visiting = append(visiting[:len(visiting):len(visiting)], key)
	upstream := []string{}
	var err error
	switch g.Kind {
	case "ssh":
		source, ok := r.sshHost(g.SSHProfileID)
		if !ok {
			return nil, fmt.Errorf("网关「%s」引用的 SSH 主机不存在", g.Name)
		}
		upstream, err = r.hostRoute(source.session.ID, source.session.Gateway, visiting)
	case "socks5":
		if g.SOCKS == nil {
			return nil, errors.New("SOCKS5 网关缺少配置")
		}
		if g.UpstreamID != "" {
			upstream, err = r.gatewayRoute(g.UpstreamID, visiting)
		}
	default:
		return nil, errors.New("未知网关类型")
	}
	if err != nil {
		return nil, err
	}
	return append(upstream, g.ID), nil
}

func (a *app) captureGatewayRoute(h host) ([]gatewayHop, error) {
	if h.session.GatewayRoute != nil {
		return *h.session.GatewayRoute, validateGatewayRoute(*h.session.GatewayRoute)
	}
	catalog := a.gatewayCatalog()
	ids, err := catalog.hostRoute(h.session.ID, h.session.Gateway, nil)
	if err != nil {
		return nil, err
	}
	route := make([]gatewayHop, 0, len(ids))
	for _, id := range ids {
		// Every ID was resolved above, with its source host or SOCKS5 options.
		g, _ := catalog.gateway(id)
		hop := gatewayHop{ID: g.ID, Name: g.Name, Kind: g.Kind}
		if g.Kind == "ssh" {
			source, _ := catalog.sshHost(g.SSHProfileID)
			// The hops before this one already reach the source host.
			endpoint := cloneHostEndpoint(source)
			endpoint.session.Gateway, endpoint.session.GatewayRoute = directGateway, nil
			endpoint.session.Files = nil
			if endpoint.session.SSH != nil {
				endpoint.session.SSH = &sshOptions{KeepAlive: endpoint.session.SSH.KeepAlive}
			}
			stored := snapshotWorkspaceSession(endpoint, endpoint.session, true, 0).Endpoint
			hop.SSH = &stored
		} else {
			options := *g.SOCKS
			hop.SOCKS = &options
		}
		route = append(route, hop)
	}
	return route, validateGatewayRoute(route)
}

func validateGatewayRoute(route []gatewayHop) error {
	seen := map[string]bool{}
	for _, hop := range route {
		if hop.ID == "" || seen[hop.ID] {
			return errors.New("网关快照标识无效或形成环路")
		}
		seen[hop.ID] = true
		switch hop.Kind {
		case "ssh":
			if hop.SSH == nil || hop.SOCKS != nil || hop.SSH.Protocol != "" || hop.SSH.GatewayRoute != nil {
				return errors.New("SSH 网关快照无效")
			}
			if err := validateTransferEndpoint(*hop.SSH, false); err != nil {
				return err
			}
		case "socks5":
			if hop.SSH != nil || hop.SOCKS == nil {
				return errors.New("SOCKS5 网关快照无效")
			}
			if err := validateSOCKS(*hop.SOCKS); err != nil {
				return err
			}
		default:
			return errors.New("未知网关快照类型")
		}
	}
	return nil
}

func validateSOCKS(s socksGateway) error {
	if !validSessionHost(s.Address) || s.Port < 1 || s.Port > 65535 {
		return errors.New("SOCKS5 地址或端口无效")
	}
	if len(s.Username) > 255 || s.PasswordSaved && (s.Username == "" || s.PasswordRevision == "") {
		return errors.New("SOCKS5 认证信息无效")
	}
	return nil
}

func (a *app) validateGateways() error {
	ids := map[string]bool{}
	for _, g := range a.gateways {
		if g.ID == "" || g.ID == directGateway || ids[g.ID] || strings.TrimSpace(g.Name) == "" {
			return errors.New("网关名称或标识无效")
		}
		ids[g.ID] = true
		if g.Kind == "ssh" {
			if g.SOCKS != nil || !slices.ContainsFunc(a.hosts, func(h host) bool { return h.session.ID == g.SSHProfileID && h.protocol == "" }) {
				return fmt.Errorf("网关「%s」引用的 SSH 主机不存在", g.Name)
			}
			if g.UpstreamID != "" {
				return fmt.Errorf("SSH 网关「%s」的上游由源主机的连接网关决定", g.Name)
			}
		} else if g.Kind == "socks5" {
			if g.SSHProfileID != "" || g.SOCKS == nil {
				return errors.New("SOCKS5 网关配置无效")
			}
			if err := validateSOCKS(*g.SOCKS); err != nil {
				return err
			}
		} else {
			return errors.New("未知网关类型")
		}
	}
	if a.defaultGateway != "" && !ids[a.defaultGateway] {
		return errors.New("默认网关不存在")
	}
	for _, g := range a.gateways {
		if _, err := a.captureGatewayRoute(host{session: sessionConfig{Gateway: g.ID}}); err != nil {
			return err
		}
	}
	catalog := a.gatewayCatalog()
	for _, h := range a.hosts {
		if id := gatewayReference(h.session.Gateway); id != "" && !ids[id] {
			return errors.New("会话引用的网关不存在")
		}
		if _, err := catalog.hostRoute(h.session.ID, h.session.Gateway, nil); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) freezeGatewayRoute(h host) (host, error) {
	route, err := a.captureGatewayRoute(h)
	if err != nil {
		return h, err
	}
	h.session.GatewayRoute = &route
	return h, nil
}

func (a *app) gatewayCatalogValues(c *store.Catalog) error {
	if err := a.validateGateways(); err != nil {
		return err
	}
	c.DefaultGatewayID = a.defaultGateway
	for _, g := range a.gateways {
		data, err := json.Marshal(g)
		if err != nil {
			return err
		}
		c.Gateways = append(c.Gateways, store.Gateway{ID: g.ID, SSHProfileID: g.SSHProfileID, UpstreamID: g.UpstreamID, Value: data})
	}
	return nil
}
