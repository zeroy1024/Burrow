package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"sshdesk/internal/locales"
	"sshdesk/internal/localfs"
)

func (a *app) exportManagedKey(key managedKey) {
	parent := a.window
	a.credentialWork(func(ctx context.Context) (result credentialResult) {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: locales.MessageExportPrivateKey(), DefaultPath: key.Name, Message: locales.MessageTheExportedFileContainsAPrivateKeyKeepItSafe()})
		if err != nil {
			return credentialResult{err: err}
		}
		if path == "" || ctx.Err() != nil {
			return credentialResult{}
		}
		data, err := a.vault.Get(ctx, key.SecretID)
		if err != nil {
			return credentialResult{err: err}
		}
		defer clear(data)
		// The native save dialog confirms overwrites; never follow an existing
		// symlink or special file to a different destination.
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() {
				return credentialResult{err: locales.NewError(locales.MessageTheExportDestinationMustBeARegularFile)}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return credentialResult{err: err}
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".burrow-key-export-*")
		if err != nil {
			return credentialResult{err: err}
		}
		temporary := file.Name()
		defer func() {
			if temporary != "" {
				if err := mygo.Shell.TrashItem(temporary); err != nil {
					result.err = errors.Join(result.err, locales.Errorf(locales.MessageCleanUpExportStagingFile, err))
				}
			}
		}()
		if err := localfs.Protect(file); err != nil {
			return credentialResult{err: errors.Join(err, file.Close())}
		}
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return credentialResult{err: locales.Errorf(locales.MessageExportPrivateKey3, err)}
		}
		if err := os.Rename(temporary, path); err != nil {
			return credentialResult{err: err}
		}
		temporary = ""
		return credentialResult{notice: locales.Deferred(locales.MessagePrivateKeyExported)}
	})
}
