// Package profile defines the endpoint snapshots shared by workspaces and file transfers.
package profile

import (
	"time"

	"github.com/zeroy1024/Burrow/internal/wggateway"
)

type Config struct {
	Gateway          string        `json:"gateway,omitempty"`
	GatewayRoute     *[]GatewayHop `json:"gatewayRoute,omitempty"`
	ID               string        `json:"id"`
	Auth             string        `json:"auth"`
	KeyID            string        `json:"keyId,omitempty"`
	PortEdited       bool          `json:"portEdited"`
	SSH              *SSHOptions   `json:"ssh,omitempty"`
	Files            *FileOptions  `json:"files,omitempty"`
	PasswordSaved    bool          `json:"passwordSaved,omitempty"`
	PasswordRevision string        `json:"passwordRevision,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
}

type SSHOptions struct {
	KeepAlive   int      `json:"keepAlive"`             // TCP keepalive seconds
	HealthCheck int      `json:"healthCheck,omitempty"` // SSH health-check seconds; zero is off
	InitCommand string   `json:"initCommand,omitempty"`
	TermScheme  string   `json:"termScheme,omitempty"`
	Env         []string `json:"env,omitempty"`
	// Platform pins the remote system; empty detects it on connect.
	Platform string        `json:"platform,omitempty"`
	Forwards []ForwardRule `json:"forwards,omitempty"`
}

type FileOptions struct {
	RemoteDir string `json:"remoteDir,omitempty"`
	LocalDir  string `json:"localDir,omitempty"`
	Encoding  string `json:"encoding"`
	FTPMode   string `json:"ftpMode,omitempty"`
	FTPTLS    string `json:"ftpTls,omitempty"`
}

type Endpoint struct {
	Config
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	GroupID  int    `json:"groupId"`
	Order    int    `json:"order"`
	OS       int    `json:"os"`
	System   string `json:"system,omitempty"`
}

type SOCKSGateway struct {
	Address          string `json:"address"`
	Port             int    `json:"port"`
	Username         string `json:"username,omitempty"`
	PasswordSaved    bool   `json:"passwordSaved,omitempty"`
	PasswordRevision string `json:"passwordRevision,omitempty"`
}

// Captured routes contain endpoint and credential references, never passwords.
// A non-nil empty route means explicitly captured direct access.
type GatewayHop struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Kind  string            `json:"kind"`
	SSH   *Endpoint         `json:"ssh,omitempty"`
	SOCKS *SOCKSGateway     `json:"socks,omitempty"`
	WG    *wggateway.Config `json:"wireguard,omitempty"`
}
