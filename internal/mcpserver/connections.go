package mcpserver

import "github.com/zeroy1024/Burrow/internal/store"

func (c Connection) DisplayID() string {
	if c.CanonicalID != "" {
		return c.CanonicalID
	}
	return c.ID
}

func (c Connection) Supports(capability string) bool {
	switch capability {
	case "exec":
		return c.Protocol == "ssh"
	case "read", "write":
		return c.Protocol == "ssh" || c.Protocol == "sftp" || c.Protocol == "ftp" || c.Protocol == "ftps"
	}
	return false
}

func grantedConnection(key store.MCPKey, c Connection, capability string) bool {
	if !c.Supports(capability) {
		return false
	}
	for _, g := range key.Grants {
		if g.ConnectionID == c.ID && g.EndpointDigest == c.Identity &&
			(capability == "read" && g.Read || capability == "write" && g.Write || capability == "exec" && g.Exec) {
			return true
		}
	}
	return false
}

func visibleConnections(key store.MCPKey, connections []Connection, protocol string) []map[string]any {
	byID := make(map[string]Connection, len(connections))
	for _, c := range connections {
		byID[c.ID] = c
	}
	visible := []map[string]any{}
	indices := map[string]int{}
	for _, c := range connections {
		if protocol != "" && c.Protocol != protocol {
			continue
		}
		read, write, exec := grantedConnection(key, c, "read"), grantedConnection(key, c, "write"), grantedConnection(key, c, "exec")
		if !read && !write && !exec {
			continue
		}
		id := c.DisplayID()
		if protocol != "" {
			// Preserve explicit protocol filters and legacy source IDs.
			id = c.ID
		}
		index, found := indices[id]
		if !found {
			index = len(visible)
			indices[id] = index
			visible = append(visible, map[string]any{"connection": byID[id], "files_read": false, "files_write": false, "ssh_exec": false})
		}
		item := visible[index]
		item["files_read"] = item["files_read"].(bool) || read
		item["files_write"] = item["files_write"].(bool) || write
		item["ssh_exec"] = item["ssh_exec"].(bool) || exec
	}
	return visible
}
