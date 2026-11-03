// Package fileclient provides local and remote file operations.
package fileclient

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

var ErrNotDirectory = errors.New("not a directory")

type Entry struct {
	Name        string
	Size        int64
	Directory   bool
	Symlink     bool
	Modified    time.Time
	Permissions string
	Owner       string
	Mode        fs.FileMode
	ModeKnown   bool
}

// These optional capabilities keep transport clients and existing adapters
// usable without making mutation or symlink traversal part of ordinary Stat.
type PermissionSetter interface {
	Chmod(context.Context, string, fs.FileMode) error
}

// PermissionPreserver copies and verifies native access rules when Unix mode
// bits cannot describe them. The source is the existing target, the destination
// is the completed staging file, and both must be ordinary files.
type PermissionPreserver interface {
	PreservePermissions(ctx context.Context, source, destination string) error
}

// PrivateStageWriter restricts a staging file to 0600 before returning a writer.
// create requests exclusive creation; otherwise the file must already exist.
// created reports confirmed creation even when a later permission check fails,
// so callers can track their own file without claiming an EXCL conflict's file.
type PrivateStageWriter interface {
	OpenPrivateWrite(ctx context.Context, name string, offset int64, truncate, create bool) (stream io.WriteCloser, created bool, err error)
}

type DirectoryResolver interface {
	ResolveDirectory(context.Context, string) (string, error)
}

type TreeRemover interface {
	RemoveTree(context.Context, string) error
}

type FilesystemStats struct {
	Total, Free uint64
}

// FilesystemStatter is optional; it does not run remote utility programs.
type FilesystemStatter interface {
	StatFilesystem(context.Context, string) (FilesystemStats, error)
}

type RenameOptions struct {
	NoReplace bool
}

type Client interface {
	InitialPath(context.Context) (string, error)
	ReadDir(context.Context, string) ([]Entry, error)
	// Stat describes the entry itself, without following a symbolic link.
	Stat(context.Context, string) (Entry, error)
	OpenRead(context.Context, string, int64) (io.ReadCloser, error)
	OpenWrite(context.Context, string, int64, bool) (io.WriteCloser, error)
	Mkdir(context.Context, string) error
	// NoReplace is atomic for Local and standard SFTP. FTP has no conditional
	// rename command; its adapter checks the target before RNFR/RNTO instead.
	Rename(context.Context, string, string, ...RenameOptions) error
	Remove(context.Context, string, bool) error
	Done() <-chan struct{}
	Close() error
}

// A server's directory entry is one path component, never a path supplied by
// that server. This also keeps recursive downloads inside their chosen target.
func validEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

func validRemotePath(name string) error {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return fs.ErrInvalid
	}
	return nil
}

func sameRenameSpelling(first, second string) bool {
	return strings.EqualFold(norm.NFC.String(first), norm.NFC.String(second))
}
