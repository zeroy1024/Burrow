package fileclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"

	"github.com/pkg/sftp"
)

var ErrSFTPInitPending = errors.New("an SFTP channel is still being initialized")

// A canceled protocol initializer can still be waiting for an SSH channel write.
// Keep one initializer per owner until its worker actually exits; this map does
// not retain a transport lease and does not cache clients or connections.
var sftpInitializers = struct {
	sync.Mutex
	owners map[*sshclient.Connection]chan struct{}
}{owners: make(map[*sshclient.Connection]chan struct{})}

// WaitSFTPReady waits for the current protocol initializer to actually leave
// its worker, including a canceled initializer still blocked in an SSH write.
func WaitSFTPReady(ctx context.Context, owner *sshclient.Connection) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-owner.Done():
			return sshclient.ErrClosed
		default:
		}
		sftpInitializers.Lock()
		done := sftpInitializers.owners[owner]
		sftpInitializers.Unlock()
		if done == nil {
			return nil
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		case <-owner.Done():
			return sshclient.ErrClosed
		}
	}
}

type sftpClient struct {
	client   *sftp.Client
	stream   io.ReadWriteCloser
	cancel   context.CancelFunc
	home     string
	done     chan struct{}
	once     sync.Once
	closeErr error
	readGate chan struct{}
	codec    filenameCodec
}

type SFTPOptions struct {
	Encoding string
}

// NewSFTP opens a file subsystem on an existing authenticated SSH connection.
// It owns only its subsystem; closing it cannot close a sibling Shell.
func NewSFTP(ctx context.Context, owner *sshclient.Connection, options ...SFTPOptions) (Client, error) {
	return newSFTP(ctx, owner, func(channelCtx context.Context) (io.ReadWriteCloser, error) {
		stream, err := owner.OpenSubsystem(channelCtx, "sftp")
		if err != nil {
			return nil, err
		}
		return stream, nil
	}, options...)
}

func newSFTP(ctx context.Context, owner *sshclient.Connection, open func(context.Context) (io.ReadWriteCloser, error), options ...SFTPOptions) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var encoding string
	if len(options) > 0 {
		encoding = options[0].Encoding
	}
	codec, err := newFilenameCodec(encoding)
	if err != nil {
		return nil, err
	}
	sftpInitializers.Lock()
	if _, pending := sftpInitializers.owners[owner]; pending {
		sftpInitializers.Unlock()
		return nil, ErrSFTPInitPending
	}
	initializerDone := make(chan struct{})
	sftpInitializers.owners[owner] = initializerDone
	sftpInitializers.Unlock()
	releaseInitializer := sync.OnceFunc(func() {
		sftpInitializers.Lock()
		delete(sftpInitializers.owners, owner)
		close(initializerDone)
		sftpInitializers.Unlock()
	})

	channelCtx, cancelChannel := context.WithCancel(ctx)
	setupCtx, cancelSetup := context.WithTimeout(ctx, 20*time.Second)
	defer cancelSetup()
	stopSetup := context.AfterFunc(setupCtx, cancelChannel)
	defer stopSetup()
	type result struct {
		client *sftp.Client
		stream io.ReadWriteCloser
		home   string
		err    error
	}
	ready := make(chan result)
	go func() {
		defer releaseInitializer()
		stream, err := open(channelCtx)
		var client *sftp.Client
		var home string
		if err == nil {
			client, err = sftp.NewClientPipe(stream, stream)
		}
		if err == nil {
			home, err = client.Getwd()
			if err == nil {
				home, err = codec.decode(home)
			}
			if err == nil && home == "" {
				err = errors.New("SFTP server returned an empty initial directory")
			}
		}
		opened := result{client: client, stream: stream, home: home, err: err}
		select {
		case ready <- opened:
		case <-setupCtx.Done():
			if stream != nil {
				stream.Close()
			}
			cancelChannel()
		}
	}()

	select {
	case opened := <-ready:
		if err := setupCtx.Err(); err != nil {
			cancelChannel()
			if opened.stream != nil {
				opened.stream.Close()
			}
			return nil, err
		}
		if opened.err != nil {
			cancelChannel()
			if opened.stream != nil {
				opened.stream.Close()
			}
			return nil, fmt.Errorf("initialize SFTP: %w", opened.err)
		}
		// Stop the setup deadline before returning a long-lived subsystem.
		if !stopSetup() {
			cancelChannel()
			opened.stream.Close()
			return nil, setupCtx.Err()
		}
		client := &sftpClient{client: opened.client, stream: opened.stream,
			cancel: cancelChannel, home: opened.home, done: make(chan struct{}), readGate: make(chan struct{}, 1), codec: codec}
		go func() {
			select {
			case <-ctx.Done():
				client.Close()
			case <-client.done:
			}
		}()
		go func() { opened.client.Wait(); client.Close() }()
		return client, nil
	case <-setupCtx.Done():
		cancelChannel()
		return nil, setupCtx.Err()
	}
}

func (c *sftpClient) InitialPath(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case <-c.done:
		return "", io.ErrClosedPipe
	default:
		return c.home, nil
	}
}

// Metadata operations are serialized and bounded. Cancellation closes only this
// file subsystem, leaving the owner's other Shell and SFTP channels intact.
func sftpOperation[T any](c *sftpClient, ctx context.Context, operation func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case c.readGate <- struct{}{}:
		defer func() { <-c.readGate }()
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-c.done:
		return zero, fs.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	select {
	case <-c.done:
		return zero, fs.ErrClosed
	default:
	}
	operationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	type result struct {
		value T
		err   error
	}
	ready := make(chan result)
	go func() {
		value, err := operation()
		select {
		case ready <- result{value, err}:
		case <-operationCtx.Done():
		case <-c.done:
		}
	}()
	select {
	case completed := <-ready:
		if err := operationCtx.Err(); err != nil {
			c.Close()
			return zero, err
		}
		return completed.value, completed.err
	case <-operationCtx.Done():
		c.Close()
		return zero, operationCtx.Err()
	case <-c.done:
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		return zero, fs.ErrClosed
	}
}

func (c *sftpClient) wirePath(name string) (string, error) {
	if err := validRemotePath(name); err != nil {
		return "", err
	}
	return c.codec.encode(name)
}

func (c *sftpClient) entry(info os.FileInfo) (Entry, error) {
	if info.Size() < 0 {
		return Entry{}, fmt.Errorf("SFTP entry size exceeds the supported range: %w", fs.ErrInvalid)
	}
	name, err := c.codec.decode(info.Name())
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Name: name, Size: info.Size(), Directory: info.IsDir(),
		Symlink: info.Mode()&os.ModeSymlink != 0, Modified: info.ModTime(), Permissions: info.Mode().String()}
	if stat, ok := info.Sys().(*sftp.FileStat); ok {
		entry.Owner = strconv.FormatUint(uint64(stat.UID), 10) + ":" + strconv.FormatUint(uint64(stat.GID), 10)
		entry.Mode, entry.ModeKnown = info.Mode(), stat.Mode != 0
	}
	return entry, nil
}

func (c *sftpClient) ReadDir(ctx context.Context, directory string) ([]Entry, error) {
	wire, err := c.wirePath(directory)
	if err != nil {
		return nil, err
	}
	infos, err := sftpOperation(c, ctx, func() ([]os.FileInfo, error) { return c.client.ReadDir(wire) })
	if err != nil {
		return nil, fmt.Errorf("read SFTP directory: %w", err)
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		entry, err := c.entry(info)
		if err != nil {
			return nil, err
		}
		if !validEntryName(entry.Name) {
			return nil, errors.New("SFTP server returned an invalid entry name")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (c *sftpClient) Stat(ctx context.Context, name string) (Entry, error) {
	wire, err := c.wirePath(name)
	if err != nil {
		return Entry{}, err
	}
	info, err := sftpOperation(c, ctx, func() (os.FileInfo, error) { return c.client.Lstat(wire) })
	if err != nil {
		return Entry{}, fmt.Errorf("stat SFTP entry: %w", err)
	}
	return c.entry(info)
}

func (c *sftpClient) StatFilesystem(ctx context.Context, name string) (FilesystemStats, error) {
	wire, err := c.wirePath(name)
	if err != nil {
		return FilesystemStats{}, err
	}
	return sftpOperation(c, ctx, func() (FilesystemStats, error) {
		if _, supported := c.client.HasExtension("statvfs" + "@" + "openssh.com"); !supported {
			return FilesystemStats{}, errors.ErrUnsupported
		}
		stat, err := c.client.StatVFS(wire)
		if err != nil {
			return FilesystemStats{}, err
		}
		if stat.Frsize == 0 || stat.Blocks == 0 || stat.Bfree > stat.Blocks ||
			stat.Blocks > ^uint64(0)/stat.Frsize {
			return FilesystemStats{}, errors.New("SFTP server returned invalid filesystem statistics")
		}
		return FilesystemStats{
			Total: stat.Blocks * stat.Frsize, Free: stat.Bfree * stat.Frsize,
		}, nil
	})
}

func (c *sftpClient) OpenRead(ctx context.Context, name string, offset int64) (io.ReadCloser, error) {
	stream, err := c.open(ctx, name, offset, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *sftpClient) OpenWrite(ctx context.Context, name string, offset int64, truncate bool) (io.WriteCloser, error) {
	flags := os.O_WRONLY | os.O_CREATE
	if truncate {
		flags |= os.O_TRUNC
	}
	stream, err := c.open(ctx, name, offset, flags)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *sftpClient) OpenPrivateWrite(
	ctx context.Context, name string, offset int64, truncate, create bool,
) (io.WriteCloser, bool, error) {
	if offset < 0 || truncate && offset != 0 {
		return nil, false, fs.ErrInvalid
	}
	wire, err := c.wirePath(name)
	if err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	select {
	case c.readGate <- struct{}{}:
		defer func() { <-c.readGate }()
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-c.done:
		return nil, false, fs.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	select {
	case <-c.done:
		return nil, false, fs.ErrClosed
	default:
	}
	type openedStage struct {
		file    *sftp.File
		created bool
		err     error
	}
	operation := func() (openedStage, error) {
		flags := os.O_WRONLY
		if create {
			flags |= os.O_CREATE | os.O_EXCL
		} else {
			info, err := c.client.Lstat(wire)
			if err != nil {
				return openedStage{}, err
			}
			if !info.Mode().IsRegular() {
				return openedStage{}, fs.ErrInvalid
			}
			if info.Mode().Perm()&0200 == 0 {
				// A full part may already carry the read-only final target mode.
				// Restore owner access without truncating it before verification.
				if err := c.client.Chmod(wire, 0600); err != nil {
					return openedStage{}, err
				}
			}
		}
		var file *sftp.File
		var err error
		if create {
			file, err = c.client.OpenFileMode(wire, flags, 0600)
		} else {
			file, err = c.client.OpenFile(wire, flags)
		}
		if err != nil {
			return openedStage{}, err
		}
		result := openedStage{file: file, created: create}
		fail := func(err error) (openedStage, error) {
			result.file = nil
			return result, errors.Join(err, file.Close())
		}
		info, err := file.Stat()
		if err != nil {
			return fail(err)
		}
		if !info.Mode().IsRegular() {
			return fail(fs.ErrInvalid)
		}
		if !create {
			if err := file.Chmod(0600); err != nil {
				return fail(err)
			}
			info, err = file.Stat()
			if err != nil {
				return fail(err)
			}
		}
		mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
		if !info.Mode().IsRegular() || info.Mode()&mask != 0600 {
			return fail(errors.New("SFTP staging file permissions could not be restricted to 0600"))
		}
		if truncate {
			if err := file.Truncate(0); err != nil {
				return fail(err)
			}
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return fail(err)
		}
		return result, nil
	}
	operationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ready := make(chan openedStage, 1)
	go func() {
		opened, err := operation()
		opened.err = err
		ready <- opened
	}()
	var opened openedStage
	received := false
	select {
	case opened = <-ready:
		received = true
		err = operationCtx.Err()
	case <-operationCtx.Done():
		err = operationCtx.Err()
	case <-c.done:
		err = ctx.Err()
		if err == nil {
			err = fs.ErrClosed
		}
	}
	if err != nil {
		// Creation owns a remote file even if a later Fchmod/Fstat is canceled.
		// Settle that acknowledgement after interrupting this subsystem's I/O.
		c.Close()
		if !received {
			opened = <-ready
		}
		if opened.file != nil {
			opened.err = errors.Join(opened.err, opened.file.Close())
			opened.file = nil
		}
	}
	err = errors.Join(err, opened.err)
	if err != nil {
		return nil, opened.created, fmt.Errorf("open private SFTP staging file: %w", err)
	}
	stream := &sftpFileStream{owner: c, file: opened.file, ctx: ctx}
	stream.stop = context.AfterFunc(ctx, func() { c.Close() })
	return stream, opened.created, nil
}

func (c *sftpClient) open(ctx context.Context, name string, offset int64, flags int) (*sftpFileStream, error) {
	if offset < 0 {
		return nil, fs.ErrInvalid
	}
	wire, err := c.wirePath(name)
	if err != nil {
		return nil, err
	}
	file, err := sftpOperation(c, ctx, func() (*sftp.File, error) {
		file, err := c.client.OpenFile(wire, flags)
		if err != nil {
			return nil, err
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		return file, nil
	})
	if err != nil {
		return nil, fmt.Errorf("open SFTP file: %w", err)
	}
	stream := &sftpFileStream{owner: c, file: file, ctx: ctx}
	// File byte streams have no whole-transfer deadline. Their caller owns the
	// deadline and may interrupt the subsystem without closing a shared transport.
	stream.stop = context.AfterFunc(ctx, func() { c.Close() })
	return stream, nil
}

func (c *sftpClient) Mkdir(ctx context.Context, name string) error {
	return c.change(ctx, name, func(wire string) error { return c.client.Mkdir(wire) })
}

func (c *sftpClient) Rename(ctx context.Context, oldName, newName string, options ...RenameOptions) error {
	oldWire, err := c.wirePath(oldName)
	if err != nil {
		return err
	}
	newWire, err := c.wirePath(newName)
	if err != nil {
		return err
	}
	_, err = sftpOperation(c, ctx, func() (struct{}, error) {
		if len(options) > 0 && options[0].NoReplace {
			if oldWire == newWire {
				_, err := c.client.Lstat(oldWire)
				return struct{}{}, err
			}
			if _, statErr := c.client.Lstat(newWire); statErr == nil {
				if path.Dir(oldName) != path.Dir(newName) || !sameRenameSpelling(path.Base(oldName), path.Base(newName)) {
					return struct{}{}, fs.ErrExist
				}
				name, err := localRenameTemporary()
				if err != nil {
					return struct{}{}, err
				}
				temporary := path.Join(path.Dir(oldWire), name)
				if err := c.renameNoReplace(oldWire, temporary); err != nil {
					return struct{}{}, err
				}
				err = c.renameNoReplace(temporary, newWire)
				if err != nil {
					rollback := c.renameNoReplace(temporary, oldWire)
					if rollback != nil {
						return struct{}{}, fmt.Errorf("rename failed; original remains at %s: %w", path.Join(path.Dir(oldName), name), errors.Join(err, rollback))
					}
				}
				return struct{}{}, err
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				return struct{}{}, statErr
			}
			return struct{}{}, c.renameNoReplace(oldWire, newWire)
		}
		// OpenSSH's extension supplies replacement semantics needed by the
		// explicit overwrite policy. Standard SFTP rename handles other servers.
		if _, supported := c.client.HasExtension("posix-rename" + "@" + "openssh.com"); supported {
			return struct{}{}, c.client.PosixRename(oldWire, newWire)
		}
		return struct{}{}, c.client.Rename(oldWire, newWire)
	})
	return err
}

func (c *sftpClient) renameNoReplace(from, to string) error {
	if _, err := c.client.Lstat(to); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	err := c.client.Rename(from, to)
	var status *sftp.StatusError
	if errors.As(err, &status) && (status.Code == 4 || status.Code == 11) {
		if _, statErr := c.client.Lstat(to); statErr == nil {
			return errors.Join(fs.ErrExist, err)
		}
	}
	return err
}

func (c *sftpClient) ResolveDirectory(ctx context.Context, name string) (string, error) {
	wire, err := c.wirePath(name)
	if err != nil {
		return "", err
	}
	return sftpOperation(c, ctx, func() (string, error) {
		info, err := c.client.Stat(wire)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", ErrNotDirectory
		}
		resolved, err := c.client.RealPath(wire)
		if err != nil {
			return "", err
		}
		return c.codec.decode(resolved)
	})
}

func (c *sftpClient) ResolveCanonicalDirectory(ctx context.Context, name string) (string, error) {
	return c.ResolveDirectory(ctx, name)
}

func (c *sftpClient) Chmod(ctx context.Context, name string, mode fs.FileMode) error {
	wire, err := c.wirePath(name)
	if err != nil {
		return err
	}
	_, err = sftpOperation(c, ctx, func() (struct{}, error) {
		info, err := c.client.Lstat(wire)
		if err != nil {
			return struct{}{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return struct{}{}, fs.ErrInvalid
		}
		return struct{}{}, c.client.Chmod(wire, mode)
	})
	return err
}

func (c *sftpClient) Remove(ctx context.Context, name string, directory bool) error {
	return c.change(ctx, name, func(wire string) error {
		if directory {
			return c.client.RemoveDirectory(wire)
		}
		return c.client.Remove(wire)
	})
}

func (c *sftpClient) change(ctx context.Context, name string, operation func(string) error) error {
	wire, err := c.wirePath(name)
	if err != nil {
		return err
	}
	_, err = sftpOperation(c, ctx, func() (struct{}, error) { return struct{}{}, operation(wire) })
	return err
}

type sftpFileStream struct {
	owner          *sftpClient
	file           *sftp.File
	ctx            context.Context
	stop           func() bool
	mu             sync.Mutex
	active, closed bool
	once           sync.Once
	err            error
}

func (s *sftpFileStream) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return fs.ErrClosed
	}
	select {
	case <-s.owner.done:
		return fs.ErrClosed
	default:
	}
	s.active = true
	return nil
}

func (s *sftpFileStream) end(err error) error {
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
	if canceled := s.ctx.Err(); canceled != nil {
		return canceled
	}
	return err
}

func (s *sftpFileStream) Read(buffer []byte) (int, error) {
	if err := s.begin(); err != nil {
		return 0, err
	}
	// The SSH SDK's channel Write has no context and can wait on a remote
	// window. Own the worker's buffer so cancellation can return immediately
	// without a late protocol response modifying the caller's reused buffer.
	owned := make([]byte, len(buffer))
	n, err := s.transfer(func() (int, error) { return s.file.Read(owned) })
	copy(buffer, owned[:n])
	return n, err
}

func (s *sftpFileStream) Write(buffer []byte) (int, error) {
	if err := s.begin(); err != nil {
		return 0, err
	}
	owned := append([]byte(nil), buffer...)
	return s.transfer(func() (int, error) { return s.file.Write(owned) })
}

func (s *sftpFileStream) transfer(operation func() (int, error)) (int, error) {
	type result struct {
		n   int
		err error
	}
	ready := make(chan result, 1)
	go func() { n, err := operation(); ready <- result{n, err} }()
	select {
	case completed := <-ready:
		return completed.n, s.end(completed.err)
	case <-s.ctx.Done():
		s.owner.Close()
		return 0, s.end(s.ctx.Err())
	case <-s.owner.done:
		return 0, s.end(fs.ErrClosed)
	}
}

func (s *sftpFileStream) Close() error {
	s.once.Do(func() {
		s.stop()
		s.mu.Lock()
		s.closed = true
		active := s.active
		s.mu.Unlock()
		if active {
			s.owner.Close()
		}
		_, s.err = sftpOperation(s.owner, s.ctx, func() (struct{}, error) { return struct{}{}, s.file.Close() })
	})
	return s.err
}

func (c *sftpClient) Done() <-chan struct{} { return c.done }

func (c *sftpClient) Close() error {
	c.once.Do(func() {
		close(c.done)
		c.cancel()
		// Closing the pipe lets pkg/sftp's receiver finish itself; its public
		// Close waits for that receiver and can block on a channel write mutex.
		c.closeErr = c.stream.Close()
	})
	return c.closeErr
}
