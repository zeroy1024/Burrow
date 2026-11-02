//go:build !darwin && !linux

package fileclient

import (
	"context"
	"errors"
	"io/fs"
	"os"
)

func openLocalLeaf(parent *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
	return parent.OpenFile(name, flags, mode)
}

func renameLocalAt(fromRoot *os.Root, from string, toRoot *os.Root, to string, noReplace bool) error {
	if noReplace {
		return fs.ErrUnsupported
	}
	if fromRoot.Name() != toRoot.Name() {
		return errors.New("cross-directory rename is unavailable on this platform")
	}
	return fromRoot.Rename(from, to)
}

func removeLocalTree(ctx context.Context, client *localClient, parent *os.Root, name string) error {
	if err := client.check(ctx); err != nil {
		return err
	}
	return parent.RemoveAll(name)
}
