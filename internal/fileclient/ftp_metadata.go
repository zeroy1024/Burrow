package fileclient

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/textproto"
	"path"
	"strings"
)

// LIST's filename column is not standardized. NLST supplies the authoritative
// names so padding can never silently change the path used by an operation.
func (c *ftpClient) listNames(ctx context.Context, directory string) ([]string, error) {
	data, raw, err := c.openData(ctx, "NLST", "-a", 0, false)
	if err != nil {
		var reply *textproto.Error
		if errors.As(err, &reply) && (unsupportedFTPCommand(err) || reply.Code == 501 || reply.Code == 550) {
			// NLST's flags are not standardized. A plain response is accepted
			// only when it accounts for every LIST -a entry below.
			data, raw, err = c.openData(ctx, "NLST", "", 0, false)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read exact FTP filenames: %w", err)
	}
	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	names := []string{}
	var parseErr error
	for scanner.Scan() {
		name := scanner.Text()
		prefix := strings.TrimSuffix(directory, "/") + "/"
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
		} else if strings.HasPrefix(name, "./") {
			name = name[2:]
		}
		name, err = c.codec.decode(name)
		if err != nil {
			parseErr = err
			break
		}
		if name == "." || name == ".." {
			continue
		}
		if !validEntryName(name) || invalidFTPArgument(name) {
			parseErr = errors.New("FTP NLST returned an invalid filename")
			break
		}
		names = append(names, name)
	}
	if parseErr == nil {
		parseErr = scanner.Err()
	}
	if parseErr != nil {
		c.closeSocket(raw)
		c.Close()
		return nil, parseErr
	}
	if err := c.finishData(data, raw); err != nil {
		return nil, err
	}
	return names, nil
}

func reconcileFTPNames(entries []Entry, names []string) ([]Entry, error) {
	remaining := map[string]bool{}
	for _, name := range names {
		if remaining[name] {
			return nil, errors.New("FTP returned duplicate filenames")
		}
		remaining[name] = true
	}
	matched := make([]bool, len(entries))
	// Resolve exact matches before considering padding from DOS-style LIST.
	for i := range entries {
		if entries[i].ModeKnown && remaining[entries[i].Name] {
			matched[i] = true
			delete(remaining, entries[i].Name)
		}
	}
	for i := range entries {
		if matched[i] {
			continue
		}
		candidate := ""
		for name := range remaining {
			if strings.TrimLeft(entries[i].Name, " \t") != strings.TrimLeft(name, " \t") {
				continue
			}
			if candidate != "" {
				return nil, errors.New("FTP LIST filename padding is ambiguous; refusing to guess an operation path")
			}
			candidate = name
		}
		if candidate == "" {
			return nil, errors.New("FTP LIST and NLST filenames differ; refresh the directory")
		}
		entries[i].Name = candidate
		delete(remaining, candidate)
	}
	if len(remaining) != 0 {
		return nil, errors.New("FTP LIST omitted entries present in NLST")
	}
	return entries, nil
}

func (c *ftpClient) directoryEntry(wire, name string, entries []Entry) (Entry, error) {
	var aliases []Entry
	for _, entry := range entries {
		if entry.Name == name {
			return entry, nil
		}
		if sameRenameSpelling(entry.Name, name) {
			aliases = append(aliases, entry)
		}
	}
	if len(aliases) == 0 {
		return Entry{}, fs.ErrNotExist
	}
	// A listing's spelling does not reveal the server filesystem's comparison
	// rules. Query this exact path before treating an equivalent name as absent.
	code, message, err := c.command(0, "MLST %s", wire)
	if err != nil {
		return Entry{}, err
	}
	if unsupportedFTPCommand(&textproto.Error{Code: code, Msg: message}) {
		if len(aliases) != 1 || aliases[0].Symlink {
			return Entry{}, errors.New("FTP server cannot safely resolve this equivalent filename")
		}
		verb, expected := "SIZE", 213
		if aliases[0].Directory {
			verb, expected = "CWD", 250
		}
		code, message, err = c.command(0, verb+" %s", wire)
		if err != nil {
			return Entry{}, err
		}
		if code != expected {
			return Entry{}, normalizeFTPError(&textproto.Error{Code: code, Msg: message})
		}
	} else if code != 250 {
		return Entry{}, normalizeFTPError(&textproto.Error{Code: code, Msg: message})
	}
	if len(aliases) != 1 {
		return Entry{}, errors.New("FTP server returned ambiguous equivalent filenames")
	}
	// Keep the listing's Lstat metadata; SIZE/CWD only confirmed path existence.
	return aliases[0], nil
}

func (c *ftpClient) Chmod(ctx context.Context, name string, mode fs.FileMode) error {
	wire, err := c.wirePath(name)
	if err != nil {
		return err
	}
	bits := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		bits |= 04000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 02000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 01000
	}
	return c.run(ctx, func(context.Context) error {
		wire, err := c.absolutePath(wire)
		if err != nil {
			return err
		}
		_, _, err = c.command(200, "SITE CHMOD %04o %s", bits, wire)
		return err
	})
}

func (c *ftpClient) ResolveDirectory(ctx context.Context, name string) (string, error) {
	wire, err := c.wirePath(name)
	if err != nil {
		return "", err
	}
	var resolved string
	err = c.run(ctx, func(context.Context) error {
		wire, err := c.absolutePath(wire)
		if err != nil {
			return err
		}
		if _, _, err := c.command(250, "CWD %s", wire); err != nil {
			var reply *textproto.Error
			if errors.As(err, &reply) && reply.Code == 550 {
				if strings.Contains(strings.ToLower(reply.Msg), "not a directory") {
					return ErrNotDirectory
				}
				// Some servers use the same 550 text for files and denied folders.
				// Following SIZE is only for this explicit browse action, never Stat.
				code, message, sizeErr := c.command(0, "SIZE %s", wire)
				if sizeErr != nil {
					return sizeErr
				}
				if code == 213 {
					return ErrNotDirectory
				}
				if code == 421 {
					return &textproto.Error{Code: code, Msg: message}
				}
			}
			return err
		}
		_, message, err := c.command(257, "PWD")
		if err != nil {
			return err
		}
		if !strings.HasPrefix(message, "\"") {
			return errors.New("FTP PWD did not return a quoted directory")
		}
		var value strings.Builder
		closed := false
		for i := 1; i < len(message); i++ {
			if message[i] == '"' {
				if i+1 < len(message) && message[i+1] == '"' {
					value.WriteByte('"')
					i++
					continue
				}
				closed = true
				break
			}
			value.WriteByte(message[i])
		}
		if !closed || !path.IsAbs(value.String()) {
			return errors.New("FTP PWD returned an invalid directory")
		}
		resolved, err = c.codec.decode(value.String())
		return err
	})
	return resolved, err
}
