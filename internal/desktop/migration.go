package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/migration"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"
)

type migrationStep int

const (
	migrationExport migrationStep = iota + 1
	migrationChoose
	migrationDecrypt
	migrationUnlock
	migrationReplace
)

type migrationDraft struct {
	step   migrationStep
	modal  uikit.ModalController
	busy   bool
	path   string
	stage  string
	secret string
	repeat string
	err    locales.Text
}

func (a *app) migrationSettings(c *ui.Context) {
	a.settingsGroup(c, "migration", locales.MigrationTitle(), "", []settingField{
		{
			id: "migration-export", label: locales.MigrationExport(), description: locales.MigrationExportDescription(),
			control: func() {
				uikit.Button(c, locales.MigrationExport(), uikit.ButtonProps{}).OnClick(func() {
					a.migrationDraft = &migrationDraft{step: migrationExport}
				})
			},
		},
		{
			id: "migration-import", label: locales.MigrationImport(), description: locales.MigrationImportDescription(),
			control: func() {
				uikit.Button(c, locales.MigrationImport(), uikit.ButtonProps{}).OnClick(a.chooseMigration)
			},
		},
	})
}

func migrationFilters() []mygo.FileFilter {
	return []mygo.FileFilter{{Name: locales.MigrationFileType(), Extensions: []string{migration.Extension}}}
}

func (a *app) chooseMigration() {
	d := &migrationDraft{step: migrationChoose, busy: true}
	a.migrationDraft = d
	parent := a.window
	a.sshWorkers.Go(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: locales.MigrationImport(), Filters: migrationFilters(),
		})
		if err == nil && len(paths) != 0 {
			err = migration.CheckVersion(paths[0], appVersion())
		}
		parent.Update(func() {
			d.busy = false
			if err != nil {
				d.err = migrationErrorText(err)
				return
			}
			if len(paths) == 0 {
				d.modal.Close()
				return
			}
			d.path = paths[0]
			d.step = migrationDecrypt
		})
	})
}

func (a *app) migrationDialog(c *ui.Context) {
	d := a.migrationDraft
	if d == nil {
		return
	}

	title, action := locales.MigrationImport(), locales.MigrationContinue()
	description := ""
	variant := uikit.ButtonPrimary
	switch d.step {
	case migrationExport:
		title, action = locales.MigrationExport(), locales.MigrationExport()
		description = locales.MigrationPasswordAdvice()
	case migrationDecrypt:
		description = filepath.Base(d.path)
	case migrationUnlock:
		description = locales.MigrationSourceUnlockDescription()
	case migrationReplace:
		title, action = locales.MigrationReplaceTitle(), locales.MigrationReplaceAction()
		description = locales.MigrationReplaceDescription()
		variant = uikit.ButtonDanger
	}

	open, buttonSubmitted, inputSubmitted := true, false, false
	result := uikit.Modal(
		c.Key("device-migration").Key(d),
		&open,
		uikit.ModalProps{
			Controller: &d.modal, Title: title, Width: theme.Current().C.Modal.GenerateWidth,
			Disabled: d.busy, Motion: uikit.MotionOverride(a.reduceMotion(c)),
			Footer: func(parts uikit.ModalParts) {
				uikit.ModalFooter(c).Justify(ui.End).Children(func() {
					uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{Disabled: d.busy}).
						OnClick(parts.RequestDismiss)
					if d.step != migrationChoose {
						buttonSubmitted = uikit.Button(c, action, uikit.ButtonProps{Variant: variant, Disabled: d.busy}).Clicked()
					}
				})
			},
		},
		func(uikit.ModalParts) {
			uikit.ModalBody(c).Children(func() {
				if description != "" {
					uikit.Text(c, description, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).FillWidth()
				}
				switch d.step {
				case migrationExport, migrationDecrypt:
					inputSubmitted = d.passwordField(c.Key(d.step), locales.MigrationPackagePassword(), &d.secret, true)
				case migrationUnlock:
					inputSubmitted = d.passwordField(c.Key(d.step), locales.MigrationSourceUnlockPassword(), &d.secret, true)
				}
				if d.step == migrationExport {
					inputSubmitted = d.passwordField(c.Key("repeat"), locales.MessageConfirmPassword(), &d.repeat, false) ||
						inputSubmitted
				}
				if !d.err.Empty() {
					uikit.Banner(c, d.err.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
				}
				if d.busy {
					uikit.Text(c, locales.MessageProcessing(), uikit.TextProps{
						Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
					})
				}
			})
		},
	)
	submitted := buttonSubmitted || d.step != migrationReplace && (inputSubmitted || result.Submit)
	if submitted && !d.busy && !result.Closing && !result.Dismissed {
		a.submitMigration(d)
	}
	if result.Dismissed {
		d.secret, d.repeat = "", ""
		a.migrationDraft = nil
		if d.stage != "" {
			if err := os.RemoveAll(d.stage); err != nil {
				a.persistenceNotice(locales.MigrationCleanupFailed, err)
			}
		}
	}
}

func (d *migrationDraft) passwordField(c *ui.Context, label string, value *string, focus bool) bool {
	var submitted bool
	uikit.FormRow(c, label, uikit.FormRowProps{}, func() ui.Element {
		input := uikit.PasswordInput(c, value, uikit.InputProps{Label: label, Disabled: d.busy}).Input.FillWidth()
		if focus && !d.busy {
			input.AutoFocus()
		}
		submitted = input.Submitted()
		return input
	})
	return submitted
}

func (a *app) submitMigration(d *migrationDraft) {
	d.err = locales.Text{}
	switch d.step {
	case migrationExport:
		length := utf8.RuneCountInString(d.secret)
		switch {
		case !migration.ValidVersion(appVersion()):
			d.err = migrationErrorText(migration.ErrVersion)
		case length < 12 || length > 128:
			d.err = locales.Deferred(locales.MigrationPasswordLength)
		case d.secret != d.repeat:
			d.err = locales.Deferred(locales.MigrationPasswordsDiffer)
		default:
			a.exportMigration(d)
		}
	case migrationDecrypt, migrationUnlock:
		if d.secret == "" {
			d.err = locales.Deferred(locales.MigrationPasswordRequired)
			return
		}
		a.validateMigration(d)
	case migrationReplace:
		a.migrationStage, d.stage = d.stage, ""
		d.modal.Close()
		mygo.App.Quit()
	}
}

func (a *app) exportMigration(d *migrationDraft) {
	d.busy = true
	parent := a.window
	password := d.secret
	d.secret, d.repeat = "", ""
	a.sshWorkers.Go(func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: locales.MigrationExport(), Filters: migrationFilters(),
			DefaultPath: "Burrow-" + appVersion() + "." + migration.Extension,
		})
		parent.Update(func() {
			if err != nil || path == "" {
				d.busy = false
				if err != nil {
					d.err = migrationErrorText(err)
				}
				return
			}
			suffix := "." + migration.Extension
			if !strings.HasSuffix(strings.ToLower(path), suffix) {
				path += suffix
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					d.busy = false
					d.err = locales.Deferred(locales.MigrationChooseNewName)
					return
				}
			}
			a.persistBeforeClose(func(err error) {
				if err != nil {
					d.busy = false
					d.err = migrationErrorText(err)
					return
				}
				a.persistTransfersDone(func(err error) {
					if err != nil {
						d.busy = false
						d.err = migrationErrorText(err)
						return
					}
					a.writeMigration(d, path, password)
				})
			})
		})
	})
}

func (a *app) writeMigration(d *migrationDraft, path, password string) {
	a.persist(func() error {
		return migration.Export(a.ctx, a.vault, migration.ExportOptions{
			Directory: a.dataDirectory, Destination: path, Version: appVersion(), Password: password,
		})
	}, func(err error) {
		d.busy = false
		if err != nil {
			d.err = migrationErrorText(err)
			return
		}
		a.files.notice = &uikit.ToastProps{
			ID: "migration-exported", Title: locales.Deferred(locales.MigrationExported),
			Tone: uikit.ToneSuccess,
		}
		d.modal.Close()
	})
}

func (a *app) validateMigration(d *migrationDraft) {
	d.busy = true
	password := d.secret
	d.secret = ""
	stage, step, path := d.stage, d.step, d.path
	a.sshWorkers.Go(func() {
		var err error
		var requiresPassword bool
		if step == migrationUnlock {
			unlock := []byte(password)
			err = migration.VerifyPassword(a.ctx, stage, unlock)
			clear(unlock)
		} else {
			stage, err = migration.Extract(a.ctx, migration.ImportOptions{
				Directory: a.dataDirectory, Source: path, Version: appVersion(), Password: password,
			})
			if err == nil {
				requiresPassword, err = migration.Preflight(a.ctx, a.store, stage, a.validateMigrationRecords)
				if err != nil {
					err = errors.Join(err, os.RemoveAll(stage))
					stage = ""
				}
			}
		}
		a.window.Update(func() {
			d.busy = false
			d.stage = stage
			if err != nil {
				d.err = migrationErrorText(err)
				return
			}
			d.step = migrationReplace
			if requiresPassword {
				d.step = migrationUnlock
			}
		})
	})
}

func (a *app) validateMigrationRecords(db *store.Store) error {
	snapshot := &app{ctx: a.ctx, store: db}
	if err := snapshot.loadCatalog(); err != nil {
		return err
	}
	if err := snapshot.loadFilePreferences(); err != nil {
		return err
	}
	// Transfer manifests use native absolute paths. Validate them on the
	// receiving platform before replacing its data.
	_, err := snapshot.readSavedTransfers()
	if err != nil {
		return locales.Errorf(locales.MigrationTransferHistoryInvalid, err)
	}
	return nil
}

func (a *app) discardRequestedMigration() {
	if a.migrationStage == "" {
		return
	}
	if err := os.RemoveAll(a.migrationStage); err != nil {
		a.persistenceNotice(locales.MigrationCleanupFailed, err)
	}
	a.migrationStage = ""
}

func migrationErrorText(err error) locales.Text {
	var mismatch *migration.VersionMismatch
	switch {
	case errors.As(err, &mismatch):
		if mismatch.Upgrade {
			return locales.Format(locales.MigrationUpgradeRequired, mismatch.Current, mismatch.Exported)
		}
		return locales.Format(locales.MigrationVersionUnsupported, mismatch.Current, mismatch.Exported)
	case errors.Is(err, migration.ErrVersion):
		return locales.Deferred(locales.MigrationVersionUnknown)
	case errors.Is(err, migration.ErrInvalid):
		return locales.Deferred(locales.MigrationPackageInvalid)
	case errors.Is(err, migration.ErrDecrypt):
		return locales.Deferred(locales.MigrationDecryptFailed)
	case errors.Is(err, vault.ErrUnlock):
		return locales.Deferred(locales.MigrationSourceUnlockFailed)
	default:
		return locales.ErrorText(err)
	}
}
