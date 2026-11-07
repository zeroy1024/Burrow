package desktop

import (
	"crypto/subtle"
	"net"
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/mcpserver"
	"sshdesk/internal/store"
)

// Only display and discovery are grouped. Every member retains its runtime,
// endpoint digest and grants, so existing policies and jobs keep their source.
func (b *mcpDesktop) groupConnections(connections []mcpserver.Connection) {
	var ssh []int
	for i, c := range connections {
		if c.Protocol == "ssh" {
			ssh = append(ssh, i)
		}
	}
	slices.SortFunc(ssh, func(left, right int) int { return strings.Compare(connections[left].ID, connections[right].ID) })
	for i, c := range connections {
		if c.Protocol != "sftp" {
			continue
		}
		for _, index := range ssh {
			primary := connections[index]
			if b.a.sameMCPSSH(primary.Runtime.(*fileSession).endpoint, c.Runtime.(*fileSession).endpoint) {
				connections[i].CanonicalID = primary.ID
				break
			}
		}
	}
}

func (a *app) sameMCPSSH(left, right host) bool {
	l, r := left.session, right.session
	if left.address != right.address || left.port != right.port || left.user != right.user ||
		l.Auth != r.Auth || l.KeyID != r.KeyID || l.PasswordSaved != r.PasswordSaved ||
		!sameGatewayRoute(l.GatewayRoute, r.GatewayRoute) {
		return false
	}
	// SSH file tools use UTF-8. A site with another filename encoding has
	// different path semantics and must remain an independent file connection.
	if r.Files != nil && r.Files.Encoding != "" && !strings.EqualFold(r.Files.Encoding, "UTF-8") {
		return false
	}
	if !l.PasswordSaved {
		return true
	}
	// Independently saved copies can have different credential revisions. Prove
	// equivalence in memory without storing or exposing a password fingerprint.
	// Before the vault is available, or for unavailable credentials, keep rows separate.
	if a.vault == nil {
		return false
	}
	first, firstErr := a.sessionPassword(left)
	second, secondErr := a.sessionPassword(right)
	if firstErr != nil || secondErr != nil {
		return false
	}
	firstBytes, secondBytes := []byte(first), []byte(second)
	defer clear(firstBytes)
	defer clear(secondBytes)
	return subtle.ConstantTimeCompare(firstBytes, secondBytes) == 1
}

func cloneMCPGrants(grants []mcpKeyGrant) []mcpKeyGrant {
	cloned := slices.Clone(grants)
	for i := range cloned {
		cloned[i].Sources = slices.Clone(cloned[i].Sources)
	}
	return cloned
}

func (g *mcpKeyGrant) updatePermissions() {
	g.AllowRead, g.AllowWrite, g.AllowExec = false, false, false
	for _, source := range g.Sources {
		g.AllowRead = g.AllowRead || source.Read
		g.AllowWrite = g.AllowWrite || source.Write
		g.AllowExec = g.AllowExec || source.Exec
	}
}

func (g *mcpKeyGrant) setPermission(capability string, enabled bool) {
	for i := range g.Sources {
		// Existing grants stay on their source until explicitly disabled. A new
		// grant uses the displayed SSH profile; it never copies to every member.
		if enabled && i != 0 {
			continue
		}
		switch capability {
		case "read":
			g.Sources[i].Read = enabled
		case "write":
			g.Sources[i].Write = enabled
		case "exec":
			g.Sources[i].Exec = enabled
		}
	}
	g.updatePermissions()
}

func (g *mcpKeyGrant) loadPermissions(grants []store.MCPGrant) {
	legacyFiles := false
	for i := range g.Sources {
		source := &g.Sources[i]
		for _, grant := range grants {
			if grant.ConnectionID != source.ConnectionID {
				continue
			}
			if grant.EndpointDigest != source.EndpointDigest {
				g.Warning = "连接信息已变更，原授权已失效；请重新选择权限并保存确认"
				continue
			}
			source.Read, source.Write, source.Exec = grant.Read, grant.Write, grant.Exec
			legacyFiles = legacyFiles || i > 0 && (grant.Read || grant.Write)
		}
	}
	if legacyFiles && g.Warning == "" {
		g.Warning = "已合并显示；已有文件授权仍使用原 SFTP 配置，新增权限使用 SSH 配置。"
	}
	g.updatePermissions()
}

func (a *app) mcpGrants() []mcpKeyGrant {
	var grants []mcpKeyGrant
	if a.mcpBackend == nil {
		return grants
	}
	connections := a.mcpBackend.connections
	byID := make(map[string]mcpserver.Connection, len(connections))
	for _, c := range connections {
		byID[c.ID] = c
	}
	indices := map[string]int{}
	for _, c := range connections {
		id := c.DisplayID()
		index, found := indices[id]
		if !found {
			primary := byID[id]
			index = len(grants)
			indices[id] = index
			grants = append(grants, mcpKeyGrant{ConnectionID: id, EndpointDigest: primary.Identity, Name: primary.Name,
				Protocol:      primary.Protocol,
				Endpoint:      primary.User + "@" + net.JoinHostPort(primary.Host, strconv.Itoa(primary.Port)),
				SupportsFiles: primary.Supports("read"), SupportsSSH: primary.Supports("exec"),
				Sources: []store.MCPGrant{{ConnectionID: id, EndpointDigest: primary.Identity}}})
		}
		if c.ID != id {
			grants[index].Sources = append(grants[index].Sources, store.MCPGrant{ConnectionID: c.ID, EndpointDigest: c.Identity})
		}
	}
	return grants
}
