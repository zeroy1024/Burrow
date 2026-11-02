package fileclient

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/textproto"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ftpOperationTimeout = 20 * time.Second

type FTPConfig struct {
	Host           string
	Port           int
	User, Password string
	TLSMode        string // "none" (also the empty default), "explicit", or "implicit"
	Mode           string // "passive" (also the empty default), or "active"
	Encoding       string
	// TLSConfig permits a caller to supply a test CA pool. Hostname and
	// certificate verification remain enabled; other fields are not used.
	TLSConfig *tls.Config
}

// The control stream has exactly one owner, including while a data stream is
// open. Callers must Close every OpenRead/OpenWrite result before another command.
// Canceling an active command closes this client; queued cancellation does not.
// Transfer tasks use independent clients so this cannot affect other consumers.
type ftpClient struct {
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	gate       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
	mu         sync.Mutex
	closed     bool
	sockets    map[net.Conn]struct{}
	listeners  map[net.Listener]struct{}
	control    *textproto.Conn
	connection net.Conn
	peer       net.IP
	local      net.IP
	tlsConfig  *tls.Config
	mode       string
	codec      filenameCodec
	mlsd       bool
	home       string
}

type ftpRedactedError struct {
	cause   error
	message string
}

func (e *ftpRedactedError) Error() string { return e.message }

func (e *ftpRedactedError) Unwrap() error { return e.cause }

func DialFTP(ctx context.Context, cfg FTPConfig) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Host) == "" || strings.TrimSpace(cfg.User) == "" {
		return nil, errors.New("FTP host and user must not be empty")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("FTP port must be between 1 and 65535")
	}
	if invalidFTPArgument(cfg.User) || invalidFTPArgument(cfg.Password) {
		return nil, errors.New("FTP credentials must not contain CR, LF, or NUL")
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "none"
	}
	if cfg.Mode == "" {
		cfg.Mode = "passive"
	}
	if cfg.TLSMode != "none" && cfg.TLSMode != "explicit" && cfg.TLSMode != "implicit" {
		return nil, errors.New("unsupported FTP TLS mode")
	}
	if cfg.Mode != "passive" && cfg.Mode != "active" {
		return nil, errors.New("unsupported FTP data mode")
	}
	codec, err := newFilenameCodec(cfg.Encoding)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	c := &ftpClient{ctx: lifetime, cancel: cancel, gate: make(chan struct{}, 1), done: make(chan struct{}),
		sockets: make(map[net.Conn]struct{}), listeners: make(map[net.Listener]struct{}), mode: cfg.Mode, codec: codec}
	if cfg.TLSMode != "none" {
		c.tlsConfig = &tls.Config{ServerName: strings.Trim(cfg.Host, "[]"), MinVersion: tls.VersionTLS12,
			ClientSessionCache: tls.NewLRUClientSessionCache(16)}
		if cfg.TLSConfig != nil {
			if cfg.TLSConfig.InsecureSkipVerify {
				cancel()
				return nil, errors.New("FTP TLS certificate verification cannot be disabled")
			}
			c.tlsConfig.RootCAs = cfg.TLSConfig.RootCAs
		}
	}
	c.mu.Lock()
	c.stopParent = context.AfterFunc(ctx, func() { c.Close() })
	c.mu.Unlock()
	err = c.run(ctx, func(opCtx context.Context) error {
		raw, err := c.dial(opCtx, net.JoinHostPort(strings.Trim(cfg.Host, "[]"), strconv.Itoa(cfg.Port)))
		if err != nil {
			return fmt.Errorf("connect FTP: %w", err)
		}
		c.peer = raw.RemoteAddr().(*net.TCPAddr).IP
		c.local = raw.LocalAddr().(*net.TCPAddr).IP
		c.connection = raw
		if cfg.TLSMode == "implicit" {
			c.connection, err = c.secure(opCtx, raw)
			if err != nil {
				return fmt.Errorf("initialize FTP TLS: %w", err)
			}
		}
		c.control = textproto.NewConn(c.connection)
		if _, _, err := c.control.ReadResponse(220); err != nil {
			return fmt.Errorf("read FTP greeting: %w", err)
		}
		if cfg.TLSMode == "explicit" {
			if _, _, err := c.command(234, "AUTH TLS"); err != nil {
				return fmt.Errorf("request FTP TLS: %w", err)
			}
			c.connection, err = c.secure(opCtx, raw)
			if err != nil {
				return fmt.Errorf("initialize FTP TLS: %w", err)
			}
			c.control = textproto.NewConn(c.connection)
		}
		code, _, err := c.command(0, "USER %s", cfg.User)
		if err == nil && code == 331 {
			_, _, err = c.command(230, "PASS %s", cfg.Password)
		} else if err == nil && code != 230 {
			err = &textproto.Error{Code: code, Msg: "FTP authentication rejected"}
		}
		if err != nil {
			return fmt.Errorf("authenticate FTP: %w", err)
		}
		if c.tlsConfig != nil {
			if _, _, err := c.command(200, "PBSZ 0"); err != nil {
				return err
			}
			if _, _, err := c.command(200, "PROT P"); err != nil {
				return err
			}
		}
		if _, _, err := c.command(200, "TYPE I"); err != nil {
			return err
		}
		code, features, err := c.command(0, "FEAT")
		if err != nil {
			return err
		}
		if code == 211 {
			utf8 := false
			requestedFacts := []string{}
			for _, line := range strings.Split(features, "\n") {
				feature := strings.Fields(strings.ToUpper(line))
				if len(feature) == 0 {
					continue
				}
				switch feature[0] {
				case "MLST", "MLSD":
					c.mlsd = true
					if feature[0] == "MLST" && len(feature) > 1 {
						for _, fact := range strings.Split(feature[1], ";") {
							name := strings.TrimSuffix(fact, "*")
							switch name {
							case "TYPE", "SIZE", "MODIFY", "UNIX.MODE", "UNIX.UID", "UNIX.GID", "UNIX.OWNERNAME", "UNIX.GROUPNAME":
								requestedFacts = append(requestedFacts, strings.ToLower(name))
							}
						}
					}
				case "UTF8", "UTF-8":
					utf8 = true
				}
			}
			if len(requestedFacts) > 0 {
				code, message, err := c.command(0, "OPTS MLST %s;", strings.Join(requestedFacts, ";"))
				if err != nil {
					return err
				}
				if code != 200 && code != 202 && code != 500 && code != 501 && code != 502 && code != 504 {
					return &textproto.Error{Code: code, Msg: message}
				}
			}
			if utf8 {
				if cfg.Encoding == "" || strings.EqualFold(cfg.Encoding, "UTF-8") {
					code, message, err := c.command(0, "OPTS UTF8 ON")
					if err != nil {
						return err
					}
					// pyftpdlib advertises UTF8 but rejects this OPTS argument;
					// other servers report that their UTF8 mode is always on.
					// The advertised UTF8 capability still defines name encoding.
					if code != 200 && code != 202 && code != 501 && code != 504 {
						return &textproto.Error{Code: code, Msg: message}
					}
				} else if _, _, err := c.command(200, "OPTS UTF8 OFF"); err != nil {
					return fmt.Errorf("set FTP filename encoding: %w", err)
				}
			}
		} else if code != 500 && code != 502 {
			return &textproto.Error{Code: code, Msg: features}
		}
		return nil
	})
	if err != nil && cfg.Password != "" {
		message := err.Error()
		redacted := strings.ReplaceAll(message, cfg.Password, "[redacted]")
		if redacted != message {
			// Preserve protocol classification without publishing an echoed
			// password to callers that display or persist the error message.
			err = &ftpRedactedError{cause: err, message: redacted}
		}
	}
	cfg.Password = ""
	if err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func invalidFTPArgument(value string) bool { return strings.ContainsAny(value, "\r\n\x00") }

func (c *ftpClient) wirePath(name string) (string, error) {
	if err := validRemotePath(name); err != nil {
		return "", err
	}
	if invalidFTPArgument(name) {
		return "", errors.New("FTP path must not contain CR, LF, or NUL")
	}
	wire, err := c.codec.encode(name)
	if err == nil && invalidFTPArgument(wire) {
		err = errors.New("encoded FTP path contains a command delimiter")
	}
	return wire, err
}

func (c *ftpClient) addSocket(connection net.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		connection.Close()
		return fs.ErrClosed
	}
	c.sockets[connection] = struct{}{}
	return nil
}

func (c *ftpClient) closeSocket(connection net.Conn) error {
	err := connection.Close()
	c.mu.Lock()
	delete(c.sockets, connection)
	c.mu.Unlock()
	return err
}

func (c *ftpClient) dial(ctx context.Context, address string) (net.Conn, error) {
	raw, err := (&net.Dialer{Timeout: ftpOperationTimeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if err := c.addSocket(raw); err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := raw.SetDeadline(deadline); err != nil {
			c.closeSocket(raw)
			return nil, err
		}
	}
	return raw, nil
}

func (c *ftpClient) secure(ctx context.Context, connection net.Conn) (net.Conn, error) {
	secure := tls.Client(connection, c.tlsConfig)
	if err := secure.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	return secure, nil
}

// begin holds the control gate until finish, including a returned data stream's
// lifetime. Only setup/metadata receive the fixed timeout, never file bytes.
func (c *ftpClient) begin(ctx context.Context, timed bool) (context.Context, func(error) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-c.done:
		return nil, nil, fs.ErrClosed
	}
	if err := ctx.Err(); err != nil {
		<-c.gate
		return nil, nil, err
	}
	select {
	case <-c.done:
		<-c.gate
		return nil, nil, fs.ErrClosed
	default:
	}
	opCtx, cancel := context.WithCancel(ctx)
	if timed {
		cancel()
		opCtx, cancel = context.WithTimeout(ctx, ftpOperationTimeout)
	}
	stop := context.AfterFunc(opCtx, func() { c.Close() })
	finish := func(err error) error {
		stop()
		var networkError net.Error
		if opCtx.Err() != nil {
			err = opCtx.Err()
		} else if errors.As(err, &networkError) && networkError.Timeout() {
			if deadline, ok := opCtx.Deadline(); ok && !time.Now().Before(deadline) {
				err = context.DeadlineExceeded
			}
		}
		cancel()
		var protocolError textproto.ProtocolError
		var reply *textproto.Error
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
			errors.As(err, &networkError) || errors.As(err, &protocolError) ||
			errors.As(err, &reply) && reply.Code == 421 {
			c.Close()
		}
		<-c.gate
		return normalizeFTPError(err)
	}
	if c.connection != nil {
		deadline, _ := opCtx.Deadline()
		if err := c.connection.SetDeadline(deadline); err != nil {
			return nil, nil, finish(err)
		}
	}
	return opCtx, finish, nil
}

func (c *ftpClient) run(ctx context.Context, operation func(context.Context) error) error {
	opCtx, finish, err := c.begin(ctx, true)
	if err != nil {
		return err
	}
	return finish(operation(opCtx))
}

func (c *ftpClient) command(expected int, format string, args ...any) (int, string, error) {
	if err := c.control.PrintfLine(format, args...); err != nil {
		return 0, "", err
	}
	return c.control.ReadResponse(expected)
}

func normalizeFTPError(err error) error {
	var reply *textproto.Error
	if !errors.As(err, &reply) {
		return err
	}
	message := strings.ToLower(reply.Msg)
	if reply.Code == 550 {
		switch {
		case strings.Contains(message, "no such"), strings.Contains(message, "not found"), strings.Contains(message, "does not exist"), strings.Contains(message, "cannot find"):
			return errors.Join(fs.ErrNotExist, err)
		case strings.Contains(message, "permission"), strings.Contains(message, "denied"), strings.Contains(message, "not allowed"):
			return errors.Join(fs.ErrPermission, err)
		}
	}
	return err
}

func (c *ftpClient) InitialPath(ctx context.Context) (string, error) {
	var directory string
	err := c.run(ctx, func(context.Context) error {
		wire, err := c.initialDirectory()
		if err != nil {
			return err
		}
		directory, err = c.codec.decode(wire)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("read FTP working directory: %w", err)
	}
	return directory, nil
}

func (c *ftpClient) initialDirectory() (string, error) {
	if c.home != "" {
		return c.home, nil
	}
	_, message, err := c.command(257, "PWD")
	if err != nil {
		return "", err
	}
	first := strings.IndexByte(message, '"')
	if first < 0 {
		c.Close()
		return "", errors.New("FTP server returned a malformed PWD response")
	}
	var value strings.Builder
	closed := false
	for index := first + 1; index < len(message); index++ {
		if message[index] == '"' {
			if index+1 < len(message) && message[index+1] == '"' {
				value.WriteByte('"')
				index++
				continue
			}
			closed = true
			break
		}
		value.WriteByte(message[index])
	}
	if !closed {
		c.Close()
		return "", errors.New("FTP server returned a malformed PWD response")
	}
	wire := value.String()
	if wire == "" || invalidFTPArgument(wire) || !path.IsAbs(wire) {
		return "", errors.New("FTP server returned an invalid working directory")
	}
	if _, err := c.codec.decode(wire); err != nil {
		return "", err
	}
	c.home = wire
	return wire, nil
}

// FTP's CWD is mutable, while SFTP and the file pane interpret relative paths
// against the login directory. Anchor those paths before any CWD can change it.
func (c *ftpClient) absolutePath(wire string) (string, error) {
	home, err := c.initialDirectory()
	if err != nil {
		return "", err
	}
	if path.IsAbs(wire) {
		return path.Clean(wire), nil
	}
	return path.Join(home, wire), nil
}

func (c *ftpClient) ReadDir(ctx context.Context, directory string) ([]Entry, error) {
	wire, err := c.wirePath(directory)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	err = c.run(ctx, func(opCtx context.Context) error {
		var err error
		entries, err = c.readDirectory(opCtx, wire)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read FTP directory: %w", err)
	}
	return entries, nil
}

func (c *ftpClient) readDirectory(ctx context.Context, wire string) ([]Entry, error) {
	wire, err := c.absolutePath(wire)
	if err != nil {
		return nil, err
	}
	if _, _, err := c.command(250, "CWD %s", wire); err != nil {
		return nil, err
	}
	if c.mlsd {
		entries, err := c.readListing(ctx, "MLSD", "", true)
		if err == nil {
			for _, entry := range entries {
				if !validEntryName(entry.Name) || invalidFTPArgument(entry.Name) {
					return nil, errors.New("FTP server returned an invalid entry name")
				}
			}
			return entries, nil
		}
		if !unsupportedFTPCommand(err) {
			return nil, err
		}
		c.mlsd = false
	}
	entries, err := c.readListing(ctx, "LIST", "-a", false)
	if err != nil {
		return nil, err
	}
	names, err := c.listNames(ctx, wire)
	if err != nil {
		return nil, err
	}
	return reconcileFTPNames(entries, names)
}

func (c *ftpClient) readListing(ctx context.Context, verb, argument string, machine bool) ([]Entry, error) {
	data, raw, err := c.openData(ctx, verb, argument, 0, false)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	entries := []Entry{}
	var parseErr error
	for scanner.Scan() {
		line, err := c.codec.decode(scanner.Text())
		if err != nil {
			parseErr = err
			break
		}
		entry, skip, err := parseFTPEntry(line, machine)
		if err != nil {
			parseErr = err
			break
		}
		if skip {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		parseErr = err
	}
	if parseErr != nil {
		c.closeSocket(raw)
		c.Close()
		return nil, parseErr
	}
	if err := c.finishData(data, raw); err != nil {
		return nil, err
	}
	return entries, nil
}

func unsupportedFTPCommand(err error) bool {
	var reply *textproto.Error
	return errors.As(err, &reply) && (reply.Code == 500 || reply.Code == 502 || reply.Code == 504 || reply.Code == 522)
}

func (c *ftpClient) Stat(ctx context.Context, name string) (Entry, error) {
	wire, err := c.wirePath(name)
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	err = c.run(ctx, func(opCtx context.Context) error {
		var err error
		wire, err = c.absolutePath(wire)
		if err != nil {
			return err
		}
		if wire == "/" {
			_, _, err := c.command(250, "CWD /")
			entry = Entry{Name: "/", Directory: true}
			return err
		}
		parent, base := path.Split(wire)
		if parent == "" {
			parent = "."
		}
		entries, err := c.readDirectory(opCtx, parent)
		if err != nil {
			return err
		}
		decoded, err := c.codec.decode(base)
		if err != nil {
			return err
		}
		entry, err = c.directoryEntry(wire, decoded, entries)
		if err != nil {
			return err
		}
		if !entry.ModeKnown && !entry.Directory && !entry.Symlink {
			listed, listErr := c.readListing(opCtx, "LIST", "-a "+wire, false)
			if listErr != nil {
				if unsupportedFTPCommand(listErr) {
					return nil
				}
				return listErr
			}
			if len(listed) == 1 && !listed[0].Directory && !listed[0].Symlink && listed[0].Size == entry.Size {
				entry.Mode, entry.ModeKnown = listed[0].Mode, listed[0].ModeKnown
				entry.Permissions, entry.Owner = listed[0].Permissions, listed[0].Owner
			}
		}
		return nil
	})
	if err != nil {
		return Entry{}, fmt.Errorf("stat FTP entry: %w", err)
	}
	return entry, nil
}

func (c *ftpClient) Mkdir(ctx context.Context, name string) error {
	return c.pathCommand(ctx, name, 257, "MKD")
}

func (c *ftpClient) Remove(ctx context.Context, name string, directory bool) error {
	verb := "DELE"
	if directory {
		verb = "RMD"
	}
	return c.pathCommand(ctx, name, 250, verb)
}

func (c *ftpClient) pathCommand(ctx context.Context, name string, expected int, verb string) error {
	wire, err := c.wirePath(name)
	if err != nil {
		return err
	}
	return c.run(ctx, func(context.Context) error {
		wire, err := c.absolutePath(wire)
		if err != nil {
			return err
		}
		_, _, err = c.command(expected, verb+" %s", wire)
		return err
	})
}

func (c *ftpClient) Rename(ctx context.Context, oldName, newName string, options ...RenameOptions) error {
	oldWire, err := c.wirePath(oldName)
	if err != nil {
		return err
	}
	newWire, err := c.wirePath(newName)
	if err != nil {
		return err
	}
	// FTP provides no atomic no-replace RNTO variant. This preflight prevents
	// replacing an already visible target, but cannot exclude another client
	// creating it between LIST and RNTO.
	if len(options) > 0 && options[0].NoReplace {
		caseOnly := oldWire != newWire && path.Dir(oldWire) == path.Dir(newWire) && sameRenameSpelling(path.Base(oldName), path.Base(newName))
		if existing, err := c.Stat(ctx, newName); err == nil {
			if !caseOnly || existing.Name != path.Base(oldName) {
				return fs.ErrExist
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if caseOnly {
			temporary := path.Join(path.Dir(oldWire), ".sshdesk-rename-"+rand.Text())
			decoded, err := c.codec.decode(temporary)
			if err != nil {
				return err
			}
			if err := c.Rename(ctx, oldName, decoded, RenameOptions{NoReplace: true}); err != nil {
				return err
			}
			if err := c.Rename(ctx, decoded, newName, RenameOptions{NoReplace: true}); err != nil {
				if rollback := c.Rename(ctx, decoded, oldName, RenameOptions{NoReplace: true}); rollback != nil {
					return fmt.Errorf("FTP rename failed; original remains at %s: %w", decoded, errors.Join(err, rollback))
				}
				return err
			}
			return nil
		}
	}
	return c.run(ctx, func(context.Context) error {
		oldWire, err := c.absolutePath(oldWire)
		if err != nil {
			return err
		}
		newWire, err := c.absolutePath(newWire)
		if err != nil {
			return err
		}
		if _, _, err := c.command(350, "RNFR %s", oldWire); err != nil {
			return err
		}
		_, _, err = c.command(250, "RNTO %s", newWire)
		return err
	})
}

func (c *ftpClient) OpenRead(ctx context.Context, name string, offset int64) (io.ReadCloser, error) {
	stream, err := c.open(ctx, name, offset, false, false)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *ftpClient) OpenWrite(ctx context.Context, name string, offset int64, truncate bool) (io.WriteCloser, error) {
	if truncate && offset != 0 {
		return nil, fmt.Errorf("FTP truncation requires a zero offset: %w", fs.ErrInvalid)
	}
	stream, err := c.open(ctx, name, offset, true, truncate)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (c *ftpClient) open(ctx context.Context, name string, offset int64, write, truncate bool) (*ftpStream, error) {
	if offset < 0 {
		return nil, fs.ErrInvalid
	}
	wire, err := c.wirePath(name)
	if err != nil {
		return nil, err
	}
	opCtx, finish, err := c.begin(ctx, false)
	if err != nil {
		return nil, err
	}
	setupCtx, cancel := context.WithTimeout(opCtx, ftpOperationTimeout)
	stopSetup := context.AfterFunc(setupCtx, func() { c.Close() })
	if deadline, ok := setupCtx.Deadline(); ok {
		err = c.connection.SetDeadline(deadline)
	}
	verb := "RETR"
	if write {
		verb = "STOR"
	}
	var data, raw net.Conn
	if err == nil {
		wire, err = c.absolutePath(wire)
	}
	if err == nil {
		data, raw, err = c.openData(setupCtx, verb, wire, offset, write && !truncate)
	}
	stopped := stopSetup()
	setupErr := setupCtx.Err()
	cancel()
	if err == nil && (!stopped || setupErr != nil) {
		err = setupErr
		if err == nil {
			err = context.DeadlineExceeded
		}
	}
	if err != nil {
		if raw != nil {
			c.closeSocket(raw)
		}
		return nil, finish(err)
	}
	deadline, _ := opCtx.Deadline()
	if err = c.connection.SetDeadline(deadline); err == nil {
		err = data.SetDeadline(deadline)
	}
	if err != nil {
		c.closeSocket(raw)
		c.Close()
		return nil, finish(err)
	}
	return &ftpStream{owner: c, data: data, raw: raw, ctx: opCtx, write: write, finish: finish}, nil
}

func (c *ftpClient) openData(ctx context.Context, verb, argument string, offset int64, restart bool) (net.Conn, net.Conn, error) {
	var data, raw net.Conn
	var listener *net.TCPListener
	var err error
	if c.mode == "active" {
		listener, err = c.activeListener(ctx)
		if err != nil {
			return nil, nil, err
		}
		defer c.closeListener(listener)
	} else {
		port, err := c.passivePort()
		if err != nil {
			return nil, nil, err
		}
		raw, err = c.dial(ctx, net.JoinHostPort(c.peer.String(), strconv.Itoa(port)))
		if err != nil {
			return nil, nil, err
		}
		data = raw
	}
	failed := func(err error) (net.Conn, net.Conn, error) {
		if raw != nil {
			c.closeSocket(raw)
		}
		return nil, nil, err
	}
	if offset > 0 || restart {
		if _, _, err := c.command(350, "REST %d", offset); err != nil {
			return failed(err)
		}
	}
	format := verb
	args := []any{}
	if argument != "" {
		format += " %s"
		args = append(args, argument)
	}
	code, message, err := c.command(0, format, args...)
	if err != nil {
		return failed(err)
	}
	if code != 125 && code != 150 {
		return failed(&textproto.Error{Code: code, Msg: message})
	}
	if listener != nil {
		for {
			raw, err = listener.AcceptTCP()
			if err != nil {
				return failed(err)
			}
			if raw.RemoteAddr().(*net.TCPAddr).IP.Equal(c.peer) {
				break
			}
			raw.Close()
		}
		if err := c.addSocket(raw); err != nil {
			return failed(err)
		}
		if deadline, ok := ctx.Deadline(); ok {
			if err := raw.SetDeadline(deadline); err != nil {
				return failed(err)
			}
		}
		data = raw
	}
	if c.tlsConfig != nil {
		data, err = c.secure(ctx, raw)
		if err != nil {
			c.Close()
			return failed(err)
		}
	}
	return data, raw, nil
}

func (c *ftpClient) passivePort() (int, error) {
	code, message, err := c.command(0, "EPSV")
	if err != nil {
		return 0, err
	}
	if code == 229 {
		start, end := strings.IndexByte(message, '('), strings.LastIndexByte(message, ')')
		if start < 0 || end <= start+1 {
			return 0, textproto.ProtocolError("malformed FTP EPSV response")
		}
		part := message[start+1 : end]
		pieces := strings.Split(part, part[:1])
		if len(pieces) != 5 || pieces[0] != "" || pieces[1] != "" || pieces[2] != "" || pieces[4] != "" {
			return 0, textproto.ProtocolError("malformed FTP EPSV response")
		}
		return ftpPort(pieces[3])
	}
	if c.peer.To4() == nil || (code != 500 && code != 502 && code != 522) {
		return 0, &textproto.Error{Code: code, Msg: message}
	}
	_, message, err = c.command(227, "PASV")
	if err != nil {
		return 0, err
	}
	start, end := strings.IndexByte(message, '('), strings.LastIndexByte(message, ')')
	if start < 0 || end <= start {
		return 0, textproto.ProtocolError("malformed FTP PASV response")
	}
	parts := strings.Split(message[start+1:end], ",")
	if len(parts) != 6 {
		return 0, textproto.ProtocolError("malformed FTP PASV response")
	}
	values := make([]int, 6)
	for index, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 255 {
			return 0, textproto.ProtocolError("malformed FTP PASV response")
		}
		values[index] = value
	}
	// The advertised host is deliberately ignored, including loopback/private
	// addresses: a data connection always goes to the control connection's peer.
	return ftpPort(strconv.Itoa(values[4]*256 + values[5]))
}

func ftpPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, textproto.ProtocolError("invalid FTP data port")
	}
	return port, nil
}

func (c *ftpClient) activeListener(ctx context.Context) (*net.TCPListener, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: c.local})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		listener.Close()
		return nil, fs.ErrClosed
	}
	c.listeners[listener] = struct{}{}
	c.mu.Unlock()
	if deadline, ok := ctx.Deadline(); ok {
		if err := listener.SetDeadline(deadline); err != nil {
			c.closeListener(listener)
			return nil, err
		}
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if ip := c.local.To4(); ip != nil {
		_, _, err = c.command(200, "PORT %d,%d,%d,%d,%d,%d", ip[0], ip[1], ip[2], ip[3], port/256, port%256)
	} else {
		_, _, err = c.command(200, "EPRT |2|%s|%d|", c.local.String(), port)
	}
	if err != nil {
		c.closeListener(listener)
		return nil, err
	}
	return listener, nil
}

func (c *ftpClient) closeListener(listener net.Listener) {
	listener.Close()
	c.mu.Lock()
	delete(c.listeners, listener)
	c.mu.Unlock()
}

func (c *ftpClient) finishData(data, raw net.Conn) error {
	// TLS Close writes close_notify. Bound that final write as well as the
	// control completion reply, while keeping file-copy time itself unbounded.
	deadline := time.Now().Add(ftpOperationTimeout)
	data.SetWriteDeadline(deadline)
	closeErr := data.Close()
	c.mu.Lock()
	delete(c.sockets, raw)
	c.mu.Unlock()
	if err := c.connection.SetDeadline(deadline); err != nil {
		c.Close()
		return errors.Join(closeErr, err)
	}
	code, message, replyErr := c.control.ReadResponse(0)
	if replyErr == nil && code != 226 && code != 250 {
		replyErr = &textproto.Error{Code: code, Msg: message}
	}
	if replyErr != nil {
		c.Close()
	}
	return errors.Join(closeErr, replyErr)
}

type ftpStream struct {
	owner               *ftpClient
	data                net.Conn
	raw                 net.Conn
	ctx                 context.Context
	write               bool
	finish              func(error) error
	mu                  sync.Mutex
	eof, active, closed bool
	once                sync.Once
	err                 error
}

func (s *ftpStream) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return fs.ErrClosed
	}
	s.active = true
	return nil
}

func (s *ftpStream) end(err error) error {
	s.mu.Lock()
	s.active = false
	if errors.Is(err, io.EOF) {
		s.eof = true
	}
	s.mu.Unlock()
	if canceled := s.ctx.Err(); canceled != nil {
		return canceled
	}
	return err
}

func (s *ftpStream) Read(buffer []byte) (int, error) {
	if s.write {
		return 0, fs.ErrInvalid
	}
	if err := s.begin(); err != nil {
		return 0, err
	}
	n, err := s.data.Read(buffer)
	return n, s.end(err)
}

func (s *ftpStream) Write(buffer []byte) (int, error) {
	if !s.write {
		return 0, fs.ErrInvalid
	}
	if err := s.begin(); err != nil {
		return 0, err
	}
	n, err := s.data.Write(buffer)
	return n, s.end(err)
}

func (s *ftpStream) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		abort := s.active || (!s.write && !s.eof)
		s.mu.Unlock()
		if abort || s.ctx.Err() != nil {
			s.owner.Close()
			s.owner.closeSocket(s.raw)
			s.err = s.finish(s.ctx.Err())
			return
		}
		s.err = s.finish(s.owner.finishData(s.data, s.raw))
	})
	return s.err
}

func (c *ftpClient) Done() <-chan struct{} { return c.done }

// Close the raw sockets rather than sending QUIT or a TLS close_notify that
// could wait on the peer. This interrupts active data/control I/O immediately.
func (c *ftpClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		sockets := make([]net.Conn, 0, len(c.sockets))
		for socket := range c.sockets {
			sockets = append(sockets, socket)
		}
		listeners := make([]net.Listener, 0, len(c.listeners))
		for listener := range c.listeners {
			listeners = append(listeners, listener)
		}
		stop := c.stopParent
		c.mu.Unlock()
		close(c.done)
		c.cancel()
		if stop != nil {
			stop()
		}
		var failures []error
		for _, socket := range sockets {
			if err := socket.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				failures = append(failures, err)
			}
		}
		for _, listener := range listeners {
			if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				failures = append(failures, err)
			}
		}
		c.closeErr = errors.Join(failures...)
	})
	return c.closeErr
}
