package migration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zeroy1024/Burrow/internal/localfs"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/vault"
)

const (
	pendingName    = ".migration-pending"
	applyingName   = ".migration-applying"
	installingName = "installing"
	previousName   = "previous"
	rejectedName   = "rejected"
)

var liveFiles = [...]string{
	databaseFile, databaseFile + "-wal", databaseFile + "-shm", databaseFile + "-journal",
	keysDirectory, updateFile,
}

func Preflight(
	ctx context.Context,
	live *store.Store,
	stage string,
	validateRecords func(*store.Store) error,
) (requiresPassword bool, result error) {
	incoming := filepath.Join(stage, incomingName)
	db, err := store.OpenSnapshot(ctx, filepath.Join(incoming, databaseFile))
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	if err := live.ValidateSnapshot(ctx, db); err != nil {
		return false, err
	}
	if err := validateRecords(db); err != nil {
		return false, err
	}
	manager, err := vault.OpenWithKeyStore(ctx, db, incoming, nil)
	if err != nil {
		return false, err
	}
	defer manager.Close()
	if manager.State() != "ready" {
		return false, ErrInvalid
	}
	if manager.RequiresPassword() {
		exists, err := pathExists(filepath.Join(incoming, keysDirectory))
		if exists {
			return false, ErrInvalid
		}
		return true, err
	}
	return false, manager.VerifyItems(ctx)
}

func VerifyPassword(ctx context.Context, stage string, password []byte) (result error) {
	incoming := filepath.Join(stage, incomingName)
	db, err := store.OpenSnapshot(ctx, filepath.Join(incoming, databaseFile))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, db.Close()) }()
	manager, err := vault.OpenWithKeyStore(ctx, db, incoming, nil)
	if err != nil {
		return err
	}
	defer manager.Close()
	if err := manager.Unlock(ctx, password); err != nil {
		return err
	}
	return manager.VerifyItems(ctx)
}

// Schedule publishes the already verified stage after the quit-time writes.
func Schedule(directory, stage string) error {
	pending := filepath.Join(directory, pendingName)
	exists, err := pathExists(pending)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("a Burrow migration is already pending")
	}
	if err := os.Rename(stage, pending); err != nil {
		return err
	}
	if err := syncDirectory(directory); err != nil {
		return errors.Join(err, os.Rename(pending, stage))
	}
	return nil
}

// Restore owns the journal while starting the candidate database. The live
// database lock must cover Apply, Recover and Commit.
type Restore struct{ directory string }

func Apply(directory, version string) (*Restore, error) {
	pending := filepath.Join(directory, pendingName)
	exists, err := pathExists(pending)
	if !exists || err != nil {
		return nil, err
	}
	// Consume the pending stage even if the version changed before restart.
	applying := filepath.Join(directory, applyingName)
	if err := os.Rename(pending, applying); err != nil {
		return nil, err
	}
	restore := &Restore{directory: directory}
	if err := syncDirectory(directory); err != nil {
		return restore, err
	}
	exported, err := os.ReadFile(filepath.Join(applying, incomingName, versionFile))
	if err != nil {
		return restore, err
	}
	if !ValidVersion(string(exported)) {
		return restore, ErrInvalid
	}
	if err := compareVersion(string(exported), version); err != nil {
		return restore, err
	}
	return restore, restore.install(applying)
}

func (r *Restore) install(applying string) error {
	incoming := filepath.Join(applying, incomingName)
	source, err := openRegular(filepath.Join(incoming, databaseFile))
	if err != nil {
		return err
	}
	if err := source.Close(); err != nil {
		return err
	}
	previous := filepath.Join(applying, previousName)
	if err := localfs.MkdirAll(previous, 0700); err != nil {
		return err
	}
	if err := moveFiles(r.directory, previous, liveFiles[:]); err != nil {
		return err
	}
	// This rename marks that every existing live file is safely in previous.
	installing := filepath.Join(applying, installingName)
	if err := os.Rename(incoming, installing); err != nil {
		return err
	}
	if err := syncDirectory(applying); err != nil {
		return err
	}
	return moveFiles(installing, r.directory, []string{databaseFile, keysDirectory, updateFile})
}

// Recover undoes a failed or interrupted switch. Each rename is repeatable
// after a crash because the installing phase ends before restoring old files.
func Recover(directory string) (bool, error) {
	applying := filepath.Join(directory, applyingName)
	exists, err := pathExists(applying)
	if !exists || err != nil {
		return exists, err
	}
	installing := filepath.Join(applying, installingName)
	active, err := pathExists(installing)
	if err != nil {
		return true, err
	}
	if active {
		rejected := filepath.Join(applying, rejectedName)
		if err := localfs.MkdirAll(rejected, 0700); err != nil {
			return true, err
		}
		if err := moveFiles(directory, rejected, liveFiles[:]); err != nil {
			return true, err
		}
		if err := os.Rename(installing, filepath.Join(applying, incomingName)); err != nil {
			return true, err
		}
		if err := syncDirectory(applying); err != nil {
			return true, err
		}
	}
	previous := filepath.Join(applying, previousName)
	if exists, err := pathExists(previous); err != nil {
		return true, err
	} else if exists {
		if err := moveFiles(previous, directory, liveFiles[:]); err != nil {
			return true, err
		}
	}
	return true, errors.Join(os.RemoveAll(applying), syncDirectory(directory))
}

func (r *Restore) Commit() (string, error) {
	backup, err := os.MkdirTemp(r.directory, "migration-backup-")
	if err != nil {
		return "", err
	}
	// Rename to an unused path on all platforms, including Windows.
	if err := os.Remove(backup); err != nil {
		return "", err
	}
	if err := os.Rename(filepath.Join(r.directory, applyingName), backup); err != nil {
		return "", err
	}
	return filepath.Join(backup, previousName), syncDirectory(r.directory)
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func moveFiles(from, to string, names []string) error {
	for _, name := range names {
		source := filepath.Join(from, name)
		exists, err := pathExists(source)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		target := filepath.Join(to, name)
		exists, err = pathExists(target)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("migration file already exists: " + target)
		}
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return errors.Join(syncDirectory(from), syncDirectory(to))
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		// Windows does not support FlushFileBuffers on directory handles.
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
