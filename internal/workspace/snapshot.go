package workspace

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zeroy1024/Burrow/internal/profile"
)

type Section int

const (
	SSH Section = iota
	Files
	Settings
	MCP
	Gateways
)

func (s Section) HasCatalog() bool {
	return s == SSH || s == Files
}

type Snapshot struct {
	Version     int          `json:"version,omitempty"`
	Tabs        []SavedTab   `json:"tabs,omitempty"`
	FileTabs    []FileTab    `json:"fileTabs,omitempty"`
	Connections []Connection `json:"connections,omitempty"`
	Section     Section      `json:"section"`
	ActiveSSH   int          `json:"activeSSH"`
	ActiveFile  int          `json:"activeFile"`
}

type Connection struct {
	EndpointRef string           `json:"endpointRef,omitempty"`
	ID          int              `json:"id"`
	Endpoint    profile.Endpoint `json:"endpoint,omitzero"`
}

type SavedTab struct {
	Root      *Layout[Session] `json:"root"`
	Focused   int              `json:"focused"`
	Maximized bool             `json:"maximized,omitempty"`
}

type Session struct {
	EndpointRef string           `json:"endpointRef,omitempty"`
	ProfileID   string           `json:"profileId,omitempty"`
	Endpoint    profile.Endpoint `json:"endpoint,omitzero"`
	Temporary   bool             `json:"temporary,omitempty"`
	Connection  int              `json:"connection,omitempty"`
}

type FileTab struct {
	ID         string  `json:"id,omitempty"`
	Session    Session `json:"session"`
	LocalPath  string  `json:"localPath"`
	RemotePath string  `json:"remotePath"`
}

func mapLayout[A, B any](n *Layout[A], pane func(*A) *B) *Layout[B] {
	if n.Session != nil {
		return &Layout[B]{Session: pane(n.Session)}
	}
	return &Layout[B]{First: mapLayout(n.First, pane), Second: mapLayout(n.Second, pane), Stacked: n.Stacked, Ratio: n.Ratio}
}

func SaveTab[T any](tab Tab[T], save func(*T) *Session) SavedTab {
	return SavedTab{Root: mapLayout(tab.Root, save), Focused: slices.Index(tab.Root.Sessions(), tab.Focused), Maximized: tab.Maximized}
}

// RestoreTab expects a snapshot that passed Validate.
func RestoreTab[T any](saved SavedTab, open func(*Session) *T) Tab[T] {
	root := mapLayout(saved.Root, open)
	return Tab[T]{Root: root, Focused: root.Sessions()[saved.Focused], Maximized: saved.Maximized}
}

func (saved *Snapshot) Validate(keyIDs map[string]bool) error {
	if saved == nil {
		return nil
	}
	if saved.Version != 2 {
		return errors.New("unsupported workspace version")
	}
	switch saved.Section {
	case SSH, Files, MCP, Gateways:
	default:
		return errors.New("invalid workspace section")
	}
	sshCount, fileCount := len(saved.Tabs), len(saved.FileTabs)
	if saved.ActiveSSH < 0 || saved.ActiveSSH >= max(1, sshCount) || saved.ActiveFile < 0 || saved.ActiveFile >= max(1, fileCount) {
		return errors.New("invalid active workspace tab")
	}
	validate := func(stored Session, files bool) error {
		h := stored.Endpoint
		if stored.Connection < 0 || strings.TrimSpace(h.Name) == "" || strings.TrimSpace(h.Host) == "" ||
			h.Port < 1 || h.Port > 65535 || h.OS < 0 || h.OS >= profile.SystemCount {
			return errors.New("invalid saved workspace endpoint")
		}
		protocol := h.Protocol
		if protocol == "ssh" {
			protocol = ""
		}
		if files && protocol != "sftp" && protocol != "ftp" || !files && protocol != "" {
			return errors.New("workspace endpoint protocol does not match its view")
		}
		if stored.ProfileID != "" && stored.ProfileID != h.ID {
			return errors.New("workspace profile id differs from its endpoint")
		}
		if err := h.Config.Validate(protocol, keyIDs); err != nil {
			return err
		}
		return nil
	}
	connections := map[int]bool{}
	connectionRoutes := map[int]*[]profile.GatewayHop{}
	for _, connection := range saved.Connections {
		if connection.ID <= 0 || connections[connection.ID] {
			return errors.New("invalid or duplicate workspace connection id")
		}
		connections[connection.ID] = true
		connectionRoutes[connection.ID] = connection.Endpoint.GatewayRoute
		files := connection.Endpoint.Protocol == "sftp"
		if connection.Endpoint.Protocol == "ftp" {
			return errors.New("FTP cannot use a shared SSH connection")
		}
		if err := validate(Session{Endpoint: connection.Endpoint, Temporary: true}, files); err != nil {
			return err
		}
	}
	validateConnection := func(stored Session) error {
		if stored.Connection > 0 && !connections[stored.Connection] {
			return errors.New("workspace session references an unknown connection")
		}
		if stored.Connection > 0 {
			if stored.Endpoint.GatewayRoute != nil {
				if err := profile.ValidateRoute(*stored.Endpoint.GatewayRoute); err != nil {
					return err
				}
			}
			if !profile.SameRoute(stored.Endpoint.GatewayRoute, connectionRoutes[stored.Connection]) {
				return errors.New("workspace session gateway route differs from its connection")
			}
		}
		if stored.Endpoint.Protocol == "ftp" && stored.Connection != 0 {
			return errors.New("FTP workspace has an SSH connection reference")
		}
		return nil
	}
	for _, tab := range saved.Tabs {
		leaves, nodes := 0, 0
		var walk func(*Layout[Session]) error
		walk = func(node *Layout[Session]) error {
			nodes++
			if node == nil || nodes > 2*MaxPanes-1 {
				return errors.New("invalid workspace layout tree")
			}
			if node.Session != nil {
				if node.First != nil || node.Second != nil {
					return errors.New("workspace leaf contains child layouts")
				}
				leaves++
				if err := validateConnection(*node.Session); err != nil {
					return err
				}
				return validate(*node.Session, false)
			}
			if !(node.Ratio > 0 && node.Ratio < 1) {
				return errors.New("invalid workspace split ratio")
			}
			if err := walk(node.First); err != nil {
				return err
			}
			return walk(node.Second)
		}
		if err := walk(tab.Root); err != nil {
			return err
		}
		if leaves > MaxPanes || tab.Focused < 0 || tab.Focused >= leaves {
			return errors.New("invalid workspace focus or pane count")
		}
	}
	fileIDs := map[string]bool{}
	for _, file := range saved.FileTabs {
		if saved.Version >= 2 && file.ID == "" {
			return errors.New("missing file workspace identity")
		}
		if file.ID != "" {
			if strings.TrimSpace(file.ID) != file.ID || fileIDs[file.ID] {
				return errors.New("invalid or duplicate file workspace identity")
			}
			fileIDs[file.ID] = true
		}
		if err := validateConnection(file.Session); err != nil {
			return err
		}
		if err := validate(file.Session, true); err != nil {
			return fmt.Errorf("file workspace: %w", err)
		}
		if strings.ContainsAny(file.LocalPath+file.RemotePath, "\x00") {
			return errors.New("workspace path contains NUL")
		}
	}
	return nil
}
