package fileclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/textproto"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
)

const ftpOperationTimeout = 20 * time.Second

type FTPConfig struct {
	Host           string
	Port           int
	User, Password string
}

type ftpClient struct {
	server     *ftp.ServerConn
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	gate       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
	mu         sync.Mutex
	closed     bool
	operation  context.Context
	sockets    map[*ftpSocket]struct{}
}

type ftpSocket struct {
	net.Conn
	owner     *ftpClient
	closeOnce sync.Once
	closeErr  error
}

// DialFTP authenticates a plain FTP control connection. Passive data sockets
// use its resolved peer rather than trusting an address supplied in PASV.
func DialFTP(ctx context.Context, cfg FTPConfig) (Client, error) {
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.User) == "" {
		return nil, errors.New("FTP host and user must not be empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("FTP port must be between 1 and 65535")
	}
	if invalidFTPArgument(cfg.User) || invalidFTPArgument(cfg.Password) {
		return nil, errors.New("FTP credentials must not contain CR, LF, or NUL")
	}
	lifetime, cancel := context.WithCancel(ctx)
	client := &ftpClient{ctx: lifetime, cancel: cancel, gate: make(chan struct{}, 1),
		done: make(chan struct{}), sockets: make(map[*ftpSocket]struct{})}
	// AfterFunc may start immediately for an already canceled parent.
	client.mu.Lock()
	client.stopParent = context.AfterFunc(ctx, func() { client.Close() })
	client.mu.Unlock()
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	err := client.run(ctx, func() error {
		server, err := ftp.Dial(address,
			ftp.DialWithDialFunc(client.dial), ftp.DialWithTrustPasvIP(false))
		if err != nil {
			return fmt.Errorf("connect FTP: %w", err)
		}
		client.server = server
		if err := server.Login(cfg.User, cfg.Password); err != nil {
			return fmt.Errorf("authenticate FTP: %w", err)
		}
		return nil
	})
	cfg.Password = ""
	if err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func invalidFTPArgument(value string) bool { return strings.ContainsAny(value, "\r\n\x00") }

// The library calls dial synchronously from the operation holding gate. Every
// socket is tracked so canceling a login/listing can interrupt both connections.
func (c *ftpClient) dial(network, address string) (net.Conn, error) {
	c.mu.Lock()
	ctx := c.operation
	c.mu.Unlock()
	connection, err := (&net.Dialer{Timeout: ftpOperationTimeout}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			connection.Close()
			return nil, err
		}
	}
	socket := &ftpSocket{Conn: connection, owner: c}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		connection.Close()
		return nil, fs.ErrClosed
	}
	c.sockets[socket] = struct{}{}
	c.mu.Unlock()
	return socket, nil
}

func (s *ftpSocket) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.Conn.Close()
		s.owner.mu.Lock()
		delete(s.owner.sockets, s)
		s.owner.mu.Unlock()
	})
	return s.closeErr
}

// ServerConn has one control reply stream. Waiting callers can cancel without
// disturbing its owner; canceling an active operation closes this FTP session.
func (c *ftpClient) run(ctx context.Context, operation func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fs.ErrClosed
	case c.gate <- struct{}{}:
	}
	defer func() { <-c.gate }()
	deadline := time.Now().Add(ftpOperationTimeout)
	if requested, ok := ctx.Deadline(); ok && requested.Before(deadline) {
		deadline = requested
	}
	opCtx, cancel := context.WithDeadline(c.ctx, deadline)
	defer cancel()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fs.ErrClosed
	}
	c.operation = opCtx
	sockets := make([]*ftpSocket, 0, len(c.sockets))
	for socket := range c.sockets {
		sockets = append(sockets, socket)
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.operation = nil
		c.mu.Unlock()
	}()
	stopRequest := context.AfterFunc(ctx, func() { c.Close() })
	defer stopRequest()
	var err error
	for _, socket := range sockets {
		if err = socket.SetDeadline(deadline); err != nil {
			break
		}
	}
	if err == nil {
		err = operation()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if errors.Is(opCtx.Err(), context.DeadlineExceeded) {
		err = opCtx.Err()
	} else if err == nil && c.ctx.Err() != nil {
		err = c.ctx.Err()
	}
	var networkError net.Error
	var protocolError textproto.ProtocolError
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.As(err, &networkError) || errors.As(err, &protocolError) {
		c.Close()
	}
	return err
}

func (c *ftpClient) InitialPath(ctx context.Context) (string, error) {
	var directory string
	err := c.run(ctx, func() error {
		var err error
		directory, err = c.currentDirectory()
		return err
	})
	if err != nil {
		return "", fmt.Errorf("read FTP working directory: %w", err)
	}
	if directory == "" || invalidFTPArgument(directory) {
		return "", errors.New("FTP server returned an invalid working directory")
	}
	return directory, nil
}

// ftp v0.2.4 slices between PWD's first and last quote without checking that
// there are two quotes. Treat that malformed server response as a failed session.
func (c *ftpClient) currentDirectory() (directory string, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			bounds, ok := failure.(runtime.Error)
			if !ok || !strings.Contains(bounds.Error(), "slice bounds out of range") {
				panic(failure)
			}
			c.Close()
			err = errors.New("FTP server returned a malformed PWD response")
		}
	}()
	return c.server.CurrentDir()
}

func (c *ftpClient) ReadDir(ctx context.Context, directory string) ([]Entry, error) {
	if invalidFTPArgument(directory) {
		return nil, errors.New("FTP path must not contain CR, LF, or NUL")
	}
	var entries []*ftp.Entry
	err := c.run(ctx, func() error {
		// LIST can report an unreadable directory as an empty successful result.
		// CWD establishes that the target is a directory the account may enter.
		if err := c.server.ChangeDir(directory); err != nil {
			return err
		}
		var err error
		entries, err = c.server.List("")
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read FTP directory: %w", err)
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "." || entry.Name == ".." {
			continue
		}
		if entry.Name == "" || invalidFTPArgument(entry.Name) {
			return nil, errors.New("FTP server returned an invalid entry name")
		}
		if entry.Size > math.MaxInt64 {
			return nil, errors.New("FTP entry size exceeds the supported range")
		}
		result = append(result, Entry{Name: entry.Name, Size: int64(entry.Size),
			Directory: entry.Type == ftp.EntryTypeFolder, Modified: entry.Time})
	}
	return result, nil
}

func (c *ftpClient) Done() <-chan struct{} { return c.done }

// Close interrupts sockets directly; QUIT would wait for a reply from a possibly
// unresponsive server. No FTP operation runs on the caller's UI thread here.
func (c *ftpClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		sockets := make([]*ftpSocket, 0, len(c.sockets))
		for socket := range c.sockets {
			sockets = append(sockets, socket)
		}
		c.mu.Unlock()
		close(c.done)
		c.cancel()
		c.stopParent()
		var failures []error
		for _, socket := range sockets {
			if err := socket.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				failures = append(failures, err)
			}
		}
		c.closeErr = errors.Join(failures...)
	})
	return c.closeErr
}
