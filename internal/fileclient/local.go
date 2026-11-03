package fileclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

type localClient struct {
	mu       sync.Mutex
	closed   bool
	files    map[*localFile]struct{}
	root     *os.Root
	done     chan struct{}
	closeErr error
}

type localFile struct {
	file  *os.File
	owner *localClient
	ctx   context.Context
	mu    sync.Mutex
	stop  func() bool
	once  sync.Once
	err   error
}

// NewLocal opens the user's filesystem without confining it to a fixed root.
func NewLocal() Client {
	return newLocalClient()
}

func newLocalClient() *localClient {
	return &localClient{files: map[*localFile]struct{}{}, done: make(chan struct{})}
}

// OpenLocalRoot keeps the selected directory open for an entire transfer.
// Absolute client paths are resolved relative to this descriptor, so replacing
// a descendant directory cannot redirect a later operation outside that tree.
func OpenLocalRoot(ctx context.Context, directory string) (Client, error) {
	c := newLocalClient()
	name, err := c.path(ctx, directory)
	if err != nil {
		return nil, err
	}
	c.root, err = os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// OpenLocalSource anchors a directory upload at that directory, or a single
// file at its parent. A source link is never promoted into a transfer root.
func OpenLocalSource(ctx context.Context, source string) (Client, error) {
	c := newLocalClient()
	name, err := c.path(ctx, source)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open transfer source", Path: name, Err: fs.ErrInvalid}
	}
	directory, leaf := name, "."
	if !info.IsDir() {
		directory, leaf = filepath.Dir(name), filepath.Base(name)
	}
	c.root, err = os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	opened, err := c.root.Lstat(leaf)
	if err == nil && !os.SameFile(info, opened) {
		err = &fs.PathError{Op: "open transfer source", Path: name, Err: fs.ErrInvalid}
	}
	if err != nil {
		return nil, errors.Join(err, c.root.Close())
	}
	return c, nil
}

func (c *localClient) InitialPath(ctx context.Context) (string, error) {
	if err := c.check(ctx); err != nil {
		return "", err
	}
	if c.root != nil {
		return c.root.Name(), nil
	}
	return os.UserHomeDir()
}

func (c *localClient) ReadDir(ctx context.Context, name string) ([]Entry, error) {
	name, err := c.path(ctx, name)
	if err != nil {
		return nil, err
	}
	var entries []os.DirEntry
	if c.root == nil {
		entries, err = os.ReadDir(name)
	} else {
		directory, openErr := c.root.Open(name)
		if openErr != nil {
			return nil, openErr
		}
		entries, err = directory.ReadDir(-1)
		err = errors.Join(err, directory.Close())
	}
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if err := c.check(ctx); err != nil {
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, localEntry(info))
	}
	return result, nil
}

func (c *localClient) Stat(ctx context.Context, name string) (Entry, error) {
	name, err := c.path(ctx, name)
	if err != nil {
		return Entry{}, err
	}
	var info fs.FileInfo
	if c.root == nil {
		info, err = os.Lstat(name)
	} else {
		info, err = c.root.Lstat(name)
	}
	if err != nil {
		return Entry{}, err
	}
	entry := localEntry(info)
	if c.root != nil && name == "." {
		entry.Name = filepath.Base(c.root.Name())
	}
	return entry, nil
}

func (c *localClient) OpenRead(ctx context.Context, name string, offset int64) (io.ReadCloser, error) {
	stream, err := c.open(ctx, name, offset, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *localClient) OpenWrite(
	ctx context.Context,
	name string,
	offset int64,
	truncate bool,
) (io.WriteCloser, error) {
	flags := os.O_CREATE | os.O_WRONLY
	if truncate {
		flags |= os.O_TRUNC
	}
	stream, err := c.open(ctx, name, offset, flags)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *localClient) open(ctx context.Context, name string, offset int64, flags int) (*localFile, error) {
	if offset < 0 {
		return nil, &fs.PathError{Op: "seek", Path: name, Err: fs.ErrInvalid}
	}
	name, err := c.path(ctx, name)
	if err != nil {
		return nil, err
	}
	parent, err := c.openParent(name)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	base := filepath.Base(name)
	info, err := parent.Lstat(base)
	if err == nil && !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if err != nil && !(flags&os.O_CREATE != 0 && errors.Is(err, fs.ErrNotExist)) {
		return nil, err
	}
	truncate := flags&os.O_TRUNC != 0
	flags &^= os.O_TRUNC
	if info == nil {
		flags |= os.O_EXCL
	} else {
		flags &^= os.O_CREATE
	}
	file, err := openLocalLeaf(parent, base, flags, 0600)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil && (!opened.Mode().IsRegular() || info != nil && !os.SameFile(info, opened)) {
		err = &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if err == nil {
		err = c.check(ctx)
	}
	if err == nil && info != nil && flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		err = secureLocalWrite(file)
	}
	if err == nil && truncate {
		err = file.Truncate(0)
	}
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	stream := &localFile{file: file, owner: c, ctx: ctx}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return nil, errors.Join(err, file.Close())
	}
	if c.closed {
		c.mu.Unlock()
		return nil, errors.Join(fs.ErrClosed, file.Close())
	}
	// Register cancellation before publishing the handle to Close. The callback
	// may run immediately for a canceled context, so protect its stop function.
	stream.mu.Lock()
	stream.stop = context.AfterFunc(ctx, func() { stream.Close() })
	c.files[stream] = struct{}{}
	stream.mu.Unlock()
	c.mu.Unlock()
	return stream, nil
}

func (c *localClient) Mkdir(ctx context.Context, name string) error {
	name, err := c.path(ctx, name)
	if err != nil {
		return err
	}
	if c.root != nil {
		return c.root.Mkdir(name, 0755)
	}
	return os.Mkdir(name, 0755)
}

func (c *localClient) Rename(ctx context.Context, oldName, newName string, options ...RenameOptions) error {
	oldName, err := c.path(ctx, oldName)
	if err != nil {
		return err
	}
	newName, err = c.path(ctx, newName)
	if err != nil {
		return err
	}
	fromRoot, err := c.openParent(oldName)
	if err != nil {
		return err
	}
	defer fromRoot.Close()
	toRoot, err := c.openParent(newName)
	if err != nil {
		return err
	}
	defer toRoot.Close()
	from, to := filepath.Base(oldName), filepath.Base(newName)
	noReplace := len(options) > 0 && options[0].NoReplace
	err = renameLocalAt(fromRoot, from, toRoot, to, noReplace)
	if !noReplace || !errors.Is(err, fs.ErrExist) || from == to || !sameRenameSpelling(from, to) {
		return err
	}
	oldInfo, oldErr := fromRoot.Lstat(from)
	newInfo, newErr := toRoot.Lstat(to)
	if oldErr != nil || newErr != nil || !os.SameFile(oldInfo, newInfo) {
		return err
	}
	name, err := localRenameTemporary()
	if err != nil {
		return err
	}
	if err := renameLocalAt(fromRoot, from, fromRoot, name, true); err != nil {
		return err
	}
	if err := renameLocalAt(fromRoot, name, toRoot, to, true); err != nil {
		rollback := renameLocalAt(fromRoot, name, fromRoot, from, true)
		if rollback != nil {
			return fmt.Errorf("rename failed; original remains at %s: %w", filepath.Join(filepath.Dir(oldName), name), errors.Join(err, rollback))
		}
		return err
	}
	return nil
}

func localRenameTemporary() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return ".sshdesk-rename-" + hex.EncodeToString(random[:]), nil
}

func (c *localClient) ResolveDirectory(ctx context.Context, name string) (string, error) {
	name, err := c.path(ctx, name)
	if err != nil {
		return "", err
	}
	if c.root != nil {
		directory, err := c.root.OpenRoot(name)
		if err != nil {
			return "", err
		}
		return directory.Name(), errors.Join(directory.Close(), c.check(ctx))
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.Join(ErrNotDirectory, &fs.PathError{Op: "resolve directory", Path: name, Err: syscall.ENOTDIR})
	}
	return resolved, c.check(ctx)
}

func (c *localClient) Chmod(ctx context.Context, name string, mode fs.FileMode) error {
	name, err := c.path(ctx, name)
	if err != nil {
		return err
	}
	parent, err := c.openParent(name)
	if err != nil {
		return err
	}
	defer parent.Close()
	base := filepath.Base(name)
	info, err := parent.Lstat(base)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrInvalid}
	}
	file, err := openLocalLeaf(parent, base, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err == nil && !os.SameFile(info, opened) {
		err = &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrInvalid}
	}
	if err == nil {
		err = c.check(ctx)
	}
	if err == nil {
		err = chmodLocalFile(file, mode)
	}
	return errors.Join(err, file.Close())
}

func (c *localClient) RemoveTree(ctx context.Context, name string) error {
	name, err := c.path(ctx, name)
	if err != nil {
		return err
	}
	if filepath.Dir(name) == name {
		return &fs.PathError{Op: "remove tree", Path: name, Err: fs.ErrInvalid}
	}
	parent, err := c.openParent(name)
	if err != nil {
		return err
	}
	defer parent.Close()
	return removeLocalTree(ctx, c, parent, filepath.Base(name))
}

func (c *localClient) Remove(ctx context.Context, name string, directory bool) error {
	name, err := c.path(ctx, name)
	if err != nil {
		return err
	}
	parent, err := c.openParent(name)
	if err != nil {
		return err
	}
	defer parent.Close()
	base := filepath.Base(name)
	info, err := parent.Lstat(base)
	if err != nil {
		return err
	}
	if directory && !info.IsDir() {
		return &fs.PathError{Op: "remove", Path: name, Err: syscall.ENOTDIR}
	}
	if !directory && info.IsDir() {
		return &fs.PathError{Op: "remove", Path: name, Err: syscall.EISDIR}
	}
	return parent.Remove(base)
}

func (c *localClient) Done() <-chan struct{} { return c.done }

func (c *localClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return c.closeErr
	}
	c.closed = true
	files := make([]*localFile, 0, len(c.files))
	for file := range c.files {
		files = append(files, file)
	}
	c.mu.Unlock()
	var err error
	for _, file := range files {
		err = errors.Join(err, file.Close())
	}
	if c.root != nil {
		err = errors.Join(err, c.root.Close())
	}
	c.closeErr = err
	close(c.done)
	return err
}

func (c *localClient) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fs.ErrClosed
	}
	return nil
}

func (c *localClient) path(ctx context.Context, name string) (string, error) {
	if err := c.check(ctx); err != nil {
		return "", err
	}
	if strings.ContainsRune(name, '\x00') {
		return "", &fs.PathError{Op: "path", Path: name, Err: fs.ErrInvalid}
	}
	if name == "~" || strings.HasPrefix(name, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		name = filepath.Join(home, strings.TrimPrefix(name, "~"))
	}
	if !filepath.IsAbs(name) {
		return "", &fs.PathError{Op: "path", Path: name, Err: fs.ErrInvalid}
	}
	name = filepath.Clean(name)
	if c.root != nil {
		relative, err := filepath.Rel(c.root.Name(), name)
		if err != nil || !filepath.IsLocal(relative) {
			return "", &fs.PathError{Op: "path", Path: name, Err: fs.ErrInvalid}
		}
		return relative, nil
	}
	return name, nil
}

func (c *localClient) openParent(name string) (*os.Root, error) {
	if c.root != nil {
		return c.root.OpenRoot(filepath.Dir(name))
	}
	return os.OpenRoot(filepath.Dir(name))
}

func localEntry(info fs.FileInfo) Entry {
	entry := Entry{
		Name: info.Name(), Size: info.Size(), Directory: info.IsDir(),
		Symlink: info.Mode()&os.ModeSymlink != 0, Modified: info.ModTime(),
		Permissions: info.Mode().String(),
		Mode:        info.Mode(), ModeKnown: true,
	}
	setLocalMetadata(&entry, info)
	return entry
}

func (f *localFile) Read(data []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := f.file.Read(data)
	if canceled := f.ctx.Err(); canceled != nil {
		err = canceled
	}
	return n, err
}

func (f *localFile) Write(data []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := f.file.Write(data)
	if canceled := f.ctx.Err(); canceled != nil {
		err = canceled
	}
	return n, err
}

func (f *localFile) Close() error {
	f.once.Do(func() {
		f.mu.Lock()
		f.stop()
		f.err = f.file.Close()
		f.mu.Unlock()
		f.owner.mu.Lock()
		delete(f.owner.files, f)
		f.owner.mu.Unlock()
	})
	return f.err
}
