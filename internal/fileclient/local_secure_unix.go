//go:build darwin || linux

package fileclient

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

// The directory descriptor comes from the already opened Root. Resolving the
// absolute parent path again after Lstat would reopen the symlink race.
func openLocalLeaf(parent *os.Root, name string, flags int, mode fs.FileMode) (*os.File, error) {
	directory, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	fd, err := unix.Openat(int(directory.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, uint32(mode.Perm()))
	if err != nil {
		return nil, &fs.PathError{Op: "openat", Path: name, Err: err}
	}
	return os.NewFile(uintptr(fd), name), nil
}

func renameLocalAt(fromRoot *os.Root, from string, toRoot *os.Root, to string, noReplace bool) error {
	fromDirectory, err := fromRoot.Open(".")
	if err != nil {
		return err
	}
	defer fromDirectory.Close()
	toDirectory, err := toRoot.Open(".")
	if err != nil {
		return err
	}
	defer toDirectory.Close()
	if noReplace {
		return renameLocalNoReplaceAt(int(fromDirectory.Fd()), from, int(toDirectory.Fd()), to)
	}
	return unix.Renameat(int(fromDirectory.Fd()), from, int(toDirectory.Fd()), to)
}

func removeLocalTree(ctx context.Context, client *localClient, parent *os.Root, name string) error {
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return client.removeTreeAt(ctx, directory, name, 0)
}

func (c *localClient) removeTreeAt(ctx context.Context, parent *os.File, name string, depth int) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	if depth > 256 {
		return errors.New("directory nesting exceeds the deletion limit")
	}
	fd := int(parent.Fd())
	err := unix.Unlinkat(fd, name, 0)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if !errors.Is(err, unix.EISDIR) && !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EACCES) {
		return &fs.PathError{Op: "unlinkat", Path: name, Err: err}
	}
	childFD, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
			// A directory replaced by a link is unlinked, never traversed.
			return unix.Unlinkat(fd, name, 0)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return &fs.PathError{Op: "openat", Path: name, Err: err}
	}
	directory := os.NewFile(uintptr(childFD), name)
	defer directory.Close()
	for {
		if err := c.check(ctx); err != nil {
			return err
		}
		// Restart enumeration at the same opened directory after each batch.
		// Removing entries can reorder directory offsets on some filesystems.
		readFD, err := unix.Openat(childFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		reader := os.NewFile(uintptr(readFD), name)
		entries, readErr := reader.ReadDir(128)
		closeErr := reader.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return errors.Join(readErr, closeErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if len(entries) == 0 {
			break
		}
		for _, entry := range entries {
			if err := c.removeTreeAt(ctx, directory, entry.Name(), depth+1); err != nil {
				return err
			}
		}
	}
	if err := c.check(ctx); err != nil {
		return err
	}
	return unix.Unlinkat(fd, name, unix.AT_REMOVEDIR)
}
