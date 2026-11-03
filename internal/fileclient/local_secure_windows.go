package fileclient

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"sshdesk/internal/localfs"
)

func openLocalLeaf(parent *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
	return localfs.OpenAt(parent, name, flags, mode)
}

func renameLocalAt(fromRoot *os.Root, from string, toRoot *os.Root, to string, noReplace bool) error {
	return localfs.RenameAt(fromRoot, from, toRoot, to, noReplace)
}

func removeLocalTree(ctx context.Context, client *localClient, parent *os.Root, name string) error {
	if err := client.check(ctx); err != nil {
		return err
	}
	// Root.RemoveAll provides Windows handle-relative traversal and never follows
	// a reparse point out of the selected directory.
	return parent.RemoveAll(name)
}

func (c *localClient) PreservePermissions(ctx context.Context, source, destination string) error {
	open := func(name string) (*os.File, error) {
		name, err := c.path(ctx, name)
		if err != nil {
			return nil, err
		}
		parent, err := c.openParent(name)
		if err != nil {
			return nil, err
		}
		defer parent.Close()
		return openLocalLeaf(parent, filepath.Base(name), os.O_RDONLY, 0)
	}
	from, err := open(source)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := open(destination)
	if err != nil {
		return err
	}
	defer to.Close()
	if err := c.check(ctx); err != nil {
		return err
	}
	return localfs.CopyPermissions(from, to)
}

func renameLocalNoReplace(from, to string) error {
	fromRoot, err := os.OpenRoot(filepath.Dir(from))
	if err != nil {
		return err
	}
	defer fromRoot.Close()
	toRoot, err := os.OpenRoot(filepath.Dir(to))
	if err != nil {
		return err
	}
	return errors.Join(
		renameLocalAt(fromRoot, filepath.Base(from), toRoot, filepath.Base(to), true),
		toRoot.Close(),
	)
}
