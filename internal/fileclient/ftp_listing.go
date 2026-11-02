package fileclient

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

func parseFTPEntry(line string, machine bool) (Entry, bool, error) {
	if strings.TrimSpace(line) == "" {
		return Entry{}, true, nil
	}
	var entry Entry
	var skip bool
	var err error
	if machine {
		entry, skip, err = parseMLSDEntry(line)
	} else {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "total" {
			_, err := parseFTPSize(fields[1])
			return Entry{}, err == nil, err
		}
		if len(fields) > 0 && strings.ContainsAny(fields[0][:1], "-dl") {
			entry, err = parseUnixFTPEntry(line)
		} else {
			entry, err = parseDOSFTPEntry(line)
		}
	}
	if err != nil {
		return Entry{}, false, err
	}
	if skip || entry.Name == "." || entry.Name == ".." {
		return Entry{}, true, nil
	}
	if entry.Name == "" {
		return Entry{}, false, errors.New("FTP listing contains an empty filename")
	}
	return entry, false, nil
}

func parseMLSDEntry(line string) (Entry, bool, error) {
	facts, name, found := strings.Cut(line, " ")
	if !found || !strings.HasSuffix(facts, ";") {
		return Entry{}, false, errors.New("malformed FTP MLSD facts")
	}
	values := map[string]string{}
	for _, fact := range strings.Split(strings.TrimSuffix(facts, ";"), ";") {
		key, value, ok := strings.Cut(fact, "=")
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return Entry{}, false, errors.New("malformed FTP MLSD fact")
		}
		key = strings.ToLower(key)
		if _, exists := values[key]; exists {
			return Entry{}, false, fmt.Errorf("duplicate FTP MLSD fact %q", key)
		}
		values[key] = value
	}
	entry := Entry{Name: name}
	kind := strings.ToLower(values["type"])
	switch {
	case kind == "cdir" || kind == "pdir":
		return Entry{}, true, nil
	case kind == "file":
	case kind == "dir":
		entry.Directory = true
	case kind == "slink" || kind == "symlink" || kind == "os.unix=slink" || kind == "os.unix=symlink":
		entry.Symlink = true
	case strings.HasPrefix(kind, "os.unix=slink:") || strings.HasPrefix(kind, "os.unix=symlink:"):
		entry.Symlink = true
	default:
		return Entry{}, false, fmt.Errorf("unsupported FTP MLSD entry type %q", values["type"])
	}
	if size, ok := values["size"]; ok {
		var err error
		entry.Size, err = parseFTPSize(size)
		if err != nil {
			return Entry{}, false, err
		}
	}
	if modified, ok := values["modify"]; ok {
		var err error
		entry.Modified, err = parseMLSDTime(modified)
		if err != nil {
			return Entry{}, false, err
		}
	}
	if value, ok := values["unix.mode"]; ok {
		value = strings.TrimPrefix(strings.ToLower(value), "0o")
		bits, err := strconv.ParseUint(value, 8, 32)
		if err != nil {
			return Entry{}, false, fmt.Errorf("invalid FTP Unix mode: %w", err)
		}
		entry.Mode, entry.ModeKnown = unixFTPMode(uint32(bits)), true
		if entry.Directory {
			entry.Mode |= fs.ModeDir
		}
		if entry.Symlink {
			entry.Mode |= fs.ModeSymlink
		}
		entry.Permissions = entry.Mode.String()
	}
	owner, group := values["unix.ownername"], values["unix.groupname"]
	if owner == "" {
		owner = values["unix.uid"]
	}
	if owner == "" {
		owner = values["unix.owner"]
	}
	if group == "" {
		group = values["unix.gid"]
	}
	if group == "" {
		group = values["unix.group"]
	}
	entry.Owner = owner
	if group != "" {
		entry.Owner += ":" + group
	}
	return entry, false, nil
}

func parseMLSDTime(value string) (time.Time, error) {
	if len(value) < 14 || !ftpDigits(value[:14]) {
		return time.Time{}, errors.New("invalid FTP MLSD modification time")
	}
	if len(value) > 14 && (value[14] != '.' || !ftpDigits(value[15:])) {
		return time.Time{}, errors.New("invalid FTP MLSD fractional modification time")
	}
	modified, err := time.Parse("20060102150405", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse FTP MLSD modification time: %w", err)
	}
	return modified, nil
}

func parseUnixFTPEntry(line string) (Entry, error) {
	fields, name, ok := ftpListingFields(line, 8)
	if !ok {
		return Entry{}, errors.New("malformed Unix FTP LIST entry")
	}
	mode := fields[0]
	if len(mode) == 11 && strings.ContainsAny(mode[10:], "+.@") {
		mode = mode[:10]
	}
	if len(mode) != 10 {
		return Entry{}, errors.New("invalid Unix FTP LIST mode")
	}
	for _, permission := range mode[1:] {
		if !strings.ContainsRune("rwxstST-", permission) {
			return Entry{}, errors.New("invalid Unix FTP LIST permissions")
		}
	}
	if _, err := parseFTPSize(fields[1]); err != nil {
		return Entry{}, fmt.Errorf("parse Unix FTP LIST link count: %w", err)
	}
	size, err := parseFTPSize(fields[4])
	if err != nil {
		return Entry{}, err
	}
	modified, err := parseUnixFTPTime(fields[5], fields[6], fields[7])
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Name: name, Size: size, Modified: modified, Permissions: mode, Owner: fields[2] + ":" + fields[3]}
	entry.Mode, entry.ModeKnown = symbolicFTPMode(mode), true
	switch mode[0] {
	case '-':
	case 'd':
		entry.Directory = true
	case 'l':
		entry.Symlink = true
		if before, _, found := strings.Cut(entry.Name, " -> "); found {
			entry.Name = before
		}
	default:
		return Entry{}, errors.New("unsupported Unix FTP LIST entry type")
	}
	return entry, nil
}

func parseUnixFTPTime(month, day, clockOrYear string) (time.Time, error) {
	if !strings.Contains(clockOrYear, ":") {
		if len(clockOrYear) != 4 || !ftpDigits(clockOrYear) {
			return time.Time{}, errors.New("invalid Unix FTP LIST year")
		}
		value, err := time.Parse("Jan 2 2006", month+" "+day+" "+clockOrYear)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse Unix FTP LIST date: %w", err)
		}
		return value, nil
	}
	// LIST has no timezone or year for recent entries. Match the previous FTP
	// adapter's UTC interpretation and six-month year-inference convention.
	now := time.Now().UTC()
	layout := "Jan 2 2006 15:04"
	if len(clockOrYear) == 8 {
		layout += ":05"
	}
	value, err := time.Parse(layout, month+" "+day+" "+strconv.Itoa(now.Year())+" "+clockOrYear)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Unix FTP LIST time: %w", err)
	}
	if !value.Before(now.AddDate(0, 6, 0)) {
		value = value.AddDate(-1, 0, 0)
	}
	return value, nil
}

func parseDOSFTPEntry(line string) (Entry, error) {
	fields, name, ok := ftpListingFields(line, 3)
	if !ok {
		return Entry{}, errors.New("unsupported FTP LIST entry format")
	}
	var modified time.Time
	var err error
	for _, layout := range []string{
		"01-02-06 03:04PM", "2006-01-02 15:04",
		"01-02-2006 03:04PM", "01-02-2006 15:04",
	} {
		modified, err = time.Parse(layout, fields[0]+" "+strings.ToUpper(fields[1]))
		if err == nil {
			break
		}
	}
	if err != nil {
		return Entry{}, fmt.Errorf("parse DOS FTP LIST date: %w", err)
	}
	entry := Entry{Name: name, Modified: modified}
	if strings.EqualFold(fields[2], "<DIR>") {
		entry.Directory = true
		return entry, nil
	}
	entry.Size, err = parseFTPSize(fields[2])
	if err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func ftpListingFields(line string, count int) ([]string, string, bool) {
	fields := make([]string, 0, count)
	for len(fields) < count {
		line = strings.TrimLeft(line, " \t")
		end := strings.IndexAny(line, " \t")
		if end < 0 {
			return nil, "", false
		}
		fields = append(fields, line[:end])
		line = line[end:]
	}
	name := line[1:]
	return fields, name, name != ""
}

func parseFTPSize(value string) (int64, error) {
	if !ftpDigits(value) {
		return 0, errors.New("invalid FTP listing size")
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse FTP listing size: %w", err)
	}
	return size, nil
}

func ftpDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func unixFTPMode(bits uint32) fs.FileMode {
	mode := fs.FileMode(bits & 0777)
	if bits&04000 != 0 {
		mode |= fs.ModeSetuid
	}
	if bits&02000 != 0 {
		mode |= fs.ModeSetgid
	}
	if bits&01000 != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}

func symbolicFTPMode(text string) fs.FileMode {
	mode := fs.FileMode(0)
	for i, bit := range []fs.FileMode{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001} {
		value := text[i+1]
		if value != '-' && value != 'S' && value != 'T' {
			mode |= bit
		}
	}
	if strings.ContainsAny(text[3:4], "sS") {
		mode |= fs.ModeSetuid
	}
	if strings.ContainsAny(text[6:7], "sS") {
		mode |= fs.ModeSetgid
	}
	if strings.ContainsAny(text[9:10], "tT") {
		mode |= fs.ModeSticky
	}
	if text[0] == 'd' {
		mode |= fs.ModeDir
	}
	if text[0] == 'l' {
		mode |= fs.ModeSymlink
	}
	return mode
}
