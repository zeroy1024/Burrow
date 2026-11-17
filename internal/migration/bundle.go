// Package migration manages encrypted, same-version Burrow data transfers.
package migration

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"filippo.io/age"
	"github.com/zeroy1024/Burrow/internal/localfs"
	"github.com/zeroy1024/Burrow/internal/vault"
	"golang.org/x/mod/semver"
)

const (
	Extension     = "burrow-migration"
	headerPrefix  = "BURROW-MIGRATION-1 "
	headerLimit   = 512
	workFactor    = 18
	databaseFile  = "burrow.db"
	keysDirectory = "vault-keys"
	updateFile    = "updater.json"
	maxUpdateSize = 64 << 10
	maxKeySize    = 4096
	versionFile   = "version"
	stageName     = ".migration-staging"
	incomingName  = "incoming"
)

var (
	ErrInvalid = errors.New("invalid Burrow migration package")
	ErrDecrypt = errors.New("cannot decrypt Burrow migration package")
	ErrVersion = errors.New("unknown Burrow application version")
	keyName    = regexp.MustCompile(`^[a-f0-9]{32}\.key$`)
)

type VersionMismatch struct {
	Exported, Current string
	Upgrade           bool
}

func (e *VersionMismatch) Error() string {
	return fmt.Sprintf("backup version %s differs from current version %s", e.Exported, e.Current)
}

func ValidVersion(version string) bool {
	canonical := "v" + version
	return len(headerPrefix)+len(version)+1 <= headerLimit &&
		semver.Canonical(canonical)+semver.Build(canonical) == canonical
}

func compareVersion(exported, current string) error {
	if !ValidVersion(current) {
		return ErrVersion
	}
	if exported == current {
		return nil
	}
	return &VersionMismatch{
		Exported: exported,
		Current:  current,
		Upgrade:  semver.Compare("v"+current, "v"+exported) < 0,
	}
}

func readVersion(r *bufio.Reader) (string, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return "", ErrInvalid
	}
	version, found := strings.CutPrefix(string(line[:len(line)-1]), headerPrefix)
	if !found || !ValidVersion(version) {
		return "", ErrInvalid
	}
	return version, nil
}

func CheckVersion(path, current string) error {
	f, err := openRegular(path)
	if err != nil {
		return err
	}
	defer f.Close()
	exported, err := readVersion(bufio.NewReaderSize(f, headerLimit))
	if err != nil {
		return err
	}
	return compareVersion(exported, current)
}

type ExportOptions struct {
	Directory, Destination, Version, Password string
}

func Export(ctx context.Context, manager *vault.Manager, opts ExportOptions) (result error) {
	if !ValidVersion(opts.Version) {
		return ErrVersion
	}
	stage, err := privateTempDir(opts.Directory)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.RemoveAll(stage)) }()
	snapshot := filepath.Join(stage, incomingName)
	if err := manager.Backup(ctx, snapshot); err != nil {
		return err
	}
	files, err := snapshotFiles(snapshot, opts.Directory)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(opts.Destination); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("migration destination must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	f, err := os.CreateTemp(filepath.Dir(opts.Destination), ".burrow-migration-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer func() {
		if temporary != "" {
			result = errors.Join(result, os.Remove(temporary))
		}
	}()
	writeErr := localfs.Protect(f)
	if writeErr == nil {
		writeErr = writeBundle(ctx, f, opts, files)
	}
	if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := os.Rename(temporary, opts.Destination); err != nil {
		return err
	}
	temporary = ""
	return syncDirectory(filepath.Dir(opts.Destination))
}

func snapshotFiles(snapshot, directory string) (map[string]string, error) {
	files := map[string]string{databaseFile: filepath.Join(snapshot, databaseFile)}
	keys, err := os.ReadDir(filepath.Join(snapshot, keysDirectory))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(keys) > 1 || len(keys) == 1 && !keyName.MatchString(keys[0].Name()) {
		return nil, ErrInvalid
	}
	if len(keys) == 1 {
		name := keysDirectory + "/" + keys[0].Name()
		files[name] = filepath.Join(snapshot, filepath.FromSlash(name))
	}
	updatePath := filepath.Join(directory, updateFile)
	if _, err := os.Lstat(updatePath); err == nil {
		files[updateFile] = updatePath
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return files, nil
}

func writeBundle(ctx context.Context, dst io.Writer, opts ExportOptions, files map[string]string) error {
	recipient, err := age.NewScryptRecipient(opts.Password)
	if err != nil {
		return err
	}
	recipient.SetWorkFactor(workFactor)
	if _, err := io.WriteString(dst, headerPrefix+opts.Version+"\n"); err != nil {
		return err
	}
	encrypted, err := age.Encrypt(dst, recipient)
	if err != nil {
		return err
	}
	w := tar.NewWriter(encrypted)
	if err := writeEntry(ctx, w, versionFile, int64(len(opts.Version)), strings.NewReader(opts.Version)); err != nil {
		return err
	}
	for name, path := range files {
		if err := writeFileEntry(ctx, w, name, path); err != nil {
			return err
		}
	}
	return errors.Join(w.Close(), encrypted.Close())
}

func writeFileEntry(ctx context.Context, w *tar.Writer, name, path string) error {
	f, err := openRegular(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if name == updateFile {
		data, err := io.ReadAll(io.LimitReader(f, maxUpdateSize+1))
		if err != nil {
			return err
		}
		if err := validateUpdate(data); err != nil {
			return err
		}
		return writeEntry(ctx, w, name, int64(len(data)), strings.NewReader(string(data)))
	}
	return writeEntry(ctx, w, name, info.Size(), f)
}

func writeEntry(ctx context.Context, w *tar.Writer, name string, size int64, src io.Reader) error {
	if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	_, err := io.CopyN(w, contextReader{Context: ctx, Reader: src}, size)
	return err
}

type ImportOptions struct {
	Directory, Source, Version, Password string
}

// Extract decrypts into a private staging directory without changing live data.
func Extract(ctx context.Context, opts ImportOptions) (stage string, result error) {
	f, err := openRegular(opts.Source)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReaderSize(contextReader{Context: ctx, Reader: f}, headerLimit)
	exported, err := readVersion(r)
	if err != nil {
		return "", err
	}
	if err := compareVersion(exported, opts.Version); err != nil {
		return "", err
	}
	identity, err := age.NewScryptIdentity(opts.Password)
	if err != nil {
		return "", err
	}
	identity.SetMaxWorkFactor(workFactor)
	decrypted, err := age.Decrypt(r, identity)
	if err != nil {
		return "", ErrDecrypt
	}
	dir, err := privateTempDir(opts.Directory)
	if err != nil {
		return "", err
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, os.RemoveAll(dir))
		}
	}()
	incoming := filepath.Join(dir, incomingName)
	if err := localfs.MkdirAll(incoming, 0700); err != nil {
		return "", err
	}
	if err := extractEntries(ctx, tar.NewReader(decrypted), incoming, exported); err != nil {
		return "", err
	}
	// tar stops at its end marker. Read through the age end marker to verify
	// the final authenticated chunk and reject extra plaintext.
	n, err := io.CopyN(io.Discard, decrypted, 1)
	if n != 0 || err != io.EOF {
		return "", ErrInvalid
	}
	return dir, nil
}

func extractEntries(ctx context.Context, r *tar.Reader, incoming, version string) error {
	first, err := r.Next()
	if err != nil || first.Name != versionFile || first.Typeflag != tar.TypeReg ||
		first.Size != int64(len(version)) {
		return ErrInvalid
	}
	data, err := io.ReadAll(r)
	if err != nil || string(data) != version {
		return ErrInvalid
	}
	seen := map[string]bool{}
	sawKey := false
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil || h.Typeflag != tar.TypeReg || seen[h.Name] {
			return ErrInvalid
		}
		seen[h.Name] = true
		switch h.Name {
		case databaseFile:
			if h.Size < 100 {
				return ErrInvalid
			}
		case updateFile:
			if h.Size > maxUpdateSize {
				return ErrInvalid
			}
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			if err := validateUpdate(data); err != nil {
				return err
			}
			if err := writeStageFile(ctx, incoming, h.Name, strings.NewReader(string(data))); err != nil {
				return err
			}
			continue
		default:
			key, found := strings.CutPrefix(h.Name, keysDirectory+"/")
			if !found || sawKey || !keyName.MatchString(key) || h.Size > maxKeySize {
				return ErrInvalid
			}
			sawKey = true
			if err := localfs.MkdirAll(filepath.Join(incoming, keysDirectory), 0700); err != nil {
				return err
			}
		}
		if err := writeStageFile(ctx, incoming, h.Name, r); err != nil {
			return err
		}
	}
	if !seen[databaseFile] {
		return ErrInvalid
	}
	return writeStageFile(ctx, incoming, versionFile, strings.NewReader(version))
}

func validateUpdate(data []byte) error {
	var object map[string]json.RawMessage
	if len(data) > maxUpdateSize || json.Unmarshal(data, &object) != nil || object == nil {
		return ErrInvalid
	}
	return nil
}

func writeStageFile(ctx context.Context, directory, name string, r io.Reader) error {
	f, err := localfs.OpenFile(
		filepath.Join(directory, filepath.FromSlash(name)),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0600,
	)
	if err != nil {
		return err
	}
	_, writeErr := io.Copy(f, contextReader{Context: ctx, Reader: r})
	return errors.Join(writeErr, f.Sync(), f.Close(), syncDirectory(filepath.Dir(f.Name())))
}

func privateTempDir(parent string) (string, error) {
	path := filepath.Join(parent, stageName)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", err
	}
	if err := localfs.MkdirAll(path, 0700); err != nil {
		return "", errors.Join(err, os.Remove(path))
	}
	return path, nil
}

// CleanStaging discards an unfinished export or unconfirmed import after a
// process interruption. Startup calls it while holding the live database lock.
func CleanStaging(directory string) error {
	return os.RemoveAll(filepath.Join(directory, stageName))
}

func openRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		return nil, errors.Join(ErrInvalid, err, f.Close())
	}
	return f, nil
}

type contextReader struct {
	context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
