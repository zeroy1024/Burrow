package desktop

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/migration"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"
)

type desktopData struct {
	db       *store.Store
	vault    *vault.Manager
	vaultErr error
	notice   *uikit.ToastProps
}

func openDesktopData(ctx context.Context, directory string) (desktopData, error) {
	name := filepath.Join(directory, "burrow.db")
	var data desktopData
	var restore *migration.Restore
	var err error
	data.db, err = store.OpenPrepared(ctx, name, func() error {
		recovered, err := migration.Recover(directory)
		if err != nil {
			return err
		}
		if recovered {
			data.notice = migrationRollbackNotice(nil)
		}
		if err := migration.CleanStaging(directory); err != nil {
			data.notice = &uikit.ToastProps{
				ID: "migration-cleanup", Title: locales.Deferred(locales.MigrationCleanupFailed),
				Detail: locales.ErrorText(err), Tone: uikit.ToneWarning,
			}
		}
		restore, err = migration.Apply(directory, appVersion())
		if err == nil {
			return nil
		}
		// An apply failure can leave some files switched. Restore them before
		// SQLite opens either database.
		if restore != nil {
			if _, rollbackErr := migration.Recover(directory); rollbackErr != nil {
				restore = nil
				return errors.Join(err, rollbackErr)
			}
		}
		restore = nil
		data.notice = migrationRollbackNotice(err)
		return nil
	})
	if err == nil {
		data.vault, data.vaultErr = vault.OpenWithKeyStore(ctx, data.db, directory, legacyUnlockKeys())
	}
	if restore == nil {
		return data, err
	}

	switch {
	case err == nil && data.vaultErr != nil:
		err = data.vaultErr
	case err == nil && data.vault.State() != "ready":
		err = migration.ErrInvalid
	}
	if err == nil {
		backup, commitErr := restore.Commit()
		if backup != "" {
			data.notice = &uikit.ToastProps{
				ID:     "migration-imported",
				Title:  locales.Deferred(locales.MigrationImported),
				Detail: locales.Format(locales.MigrationBackupLocation, backup),
				Tone:   uikit.ToneSuccess,
			}
			if commitErr != nil {
				data.notice.Tone = uikit.ToneWarning
				data.notice.Detail = locales.Format(locales.MigrationBackupSyncFailed, backup, commitErr)
			}
			return data, nil
		}
		err = commitErr
	}

	if data.vault != nil {
		data.vault.Close()
	}
	if data.db != nil {
		err = errors.Join(err, data.db.Close())
	}
	data.notice = migrationRollbackNotice(err)
	data.db, err = store.OpenPrepared(ctx, name, func() error {
		_, rollbackErr := migration.Recover(directory)
		return rollbackErr
	})
	if err == nil {
		data.vault, data.vaultErr = vault.OpenWithKeyStore(ctx, data.db, directory, legacyUnlockKeys())
	}
	return data, err
}

func migrationRollbackNotice(err error) *uikit.ToastProps {
	var detail locales.Text
	if err != nil {
		detail = migrationErrorText(err)
	}
	return &uikit.ToastProps{
		ID: "migration-rolled-back", Title: locales.Deferred(locales.MigrationRolledBack),
		Detail: detail, Tone: uikit.ToneDanger,
	}
}
