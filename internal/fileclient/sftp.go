package fileclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"sshdesk/internal/sshclient"

	"github.com/pkg/sftp"
)

var ErrSFTPInitPending = errors.New("an SFTP channel is still being initialized")

// A canceled protocol initializer can still be waiting for an SSH channel write.
// Keep one initializer per owner until its worker actually exits; this map does
// not retain a transport lease and does not cache clients or connections.
var sftpInitializers = struct {
	sync.Mutex
	owners map[*sshclient.Connection]struct{}
}{owners: make(map[*sshclient.Connection]struct{})}

type sftpClient struct {
	client   *sftp.Client
	stream   io.ReadWriteCloser
	cancel   context.CancelFunc
	home     string
	done     chan struct{}
	once     sync.Once
	closeErr error
	readGate chan struct{}
}

// NewSFTP opens a file subsystem on an existing authenticated SSH connection.
// It owns only its subsystem; closing it cannot close a sibling Shell.
func NewSFTP(ctx context.Context, owner *sshclient.Connection) (Client, error) {
	return newSFTP(ctx, owner, func(channelCtx context.Context) (io.ReadWriteCloser, error) {
		stream, err := owner.OpenSubsystem(channelCtx, "sftp")
		if err != nil {
			return nil, err
		}
		return stream, nil
	})
}

func newSFTP(ctx context.Context, owner *sshclient.Connection, open func(context.Context) (io.ReadWriteCloser, error)) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sftpInitializers.Lock()
	if _, pending := sftpInitializers.owners[owner]; pending {
		sftpInitializers.Unlock()
		return nil, ErrSFTPInitPending
	}
	sftpInitializers.owners[owner] = struct{}{}
	sftpInitializers.Unlock()
	releaseInitializer := sync.OnceFunc(func() {
		sftpInitializers.Lock()
		delete(sftpInitializers.owners, owner)
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
			if err == nil && home == "" {
				err = errors.New("SFTP server returned an empty initial directory")
			}
		}
		releaseInitializer()
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
			cancel: cancelChannel, home: opened.home, done: make(chan struct{}), readGate: make(chan struct{}, 1)}
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

func (c *sftpClient) ReadDir(ctx context.Context, directory string) ([]Entry, error) {
	select {
	case c.readGate <- struct{}{}:
		defer func() { <-c.readGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, io.ErrClosedPipe
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-c.done:
		return nil, io.ErrClosedPipe
	default:
	}
	operationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	type result struct {
		entries []Entry
		err     error
	}
	ready := make(chan result)
	go func() {
		infos, err := c.client.ReadDirContext(operationCtx, directory)
		entries := make([]Entry, 0, len(infos))
		for _, info := range infos {
			entry := Entry{Name: info.Name(), Size: info.Size(), Directory: info.IsDir(),
				Modified: info.ModTime(), Permissions: info.Mode().String()}
			if stat, ok := info.Sys().(*sftp.FileStat); ok {
				entry.Owner = strconv.FormatUint(uint64(stat.UID), 10) + ":" + strconv.FormatUint(uint64(stat.GID), 10)
			}
			entries = append(entries, entry)
		}
		select {
		case ready <- result{entries: entries, err: err}:
		case <-operationCtx.Done():
		case <-c.done:
		}
	}()
	select {
	case read := <-ready:
		if err := operationCtx.Err(); err != nil {
			c.Close()
			return nil, err
		}
		if read.err != nil {
			return nil, fmt.Errorf("read SFTP directory: %w", read.err)
		}
		return read.entries, nil
	case <-operationCtx.Done():
		// pkg/sftp's deferred directory-handle close has no context. Closing this
		// subsystem also releases that cleanup wait without touching other channels.
		c.Close()
		return nil, operationCtx.Err()
	case <-c.done:
		return nil, io.ErrClosedPipe
	}
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
