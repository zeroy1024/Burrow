// Package fileclient provides local and remote file operations.
package fileclient

import (
	"context"
	"io"
	"io/fs"
	"strings"
	"time"
)

type Entry struct {
	Name        string
	Size        int64
	Directory   bool
	Symlink     bool
	Modified    time.Time
	Permissions string
	Owner       string
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
