// Package fileclient provides read-only remote directory access.
package fileclient

import (
	"context"
	"time"
)

type Entry struct {
	Name        string
	Size        int64
	Directory   bool
	Modified    time.Time
	Permissions string
	Owner       string
}

type Client interface {
	InitialPath(context.Context) (string, error)
	ReadDir(context.Context, string) ([]Entry, error)
	Done() <-chan struct{}
	Close() error
}
