package desktop

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"
)

var errAppUnavailable = locales.NewError(locales.MessageTheAppIsLockedOrItsUnlockSettingsAreBeingUpdatedTryAgain)

type applicationSecurityState struct {
	ready, initialized, busy, changing bool
	migration, requirePassword         bool
	status, password                   string
	errorText                          locales.Text
	dialog                             *protectionDraft
	migrationKey                       *migrationPassphraseError
	migrationPassword                  *migrationCredentialError
	migrationPhrases                   map[string]string
}

const (
	protectionCurrent = iota
	protectionNew
	protectionConfirm
	protectionFieldCount
)

type protectionFieldState struct {
	handle    ui.Handle
	focused   bool
	errorText locales.Text
}

type protectionDraft struct {
	modal                            uikit.ModalController
	kind, current, password, confirm string
	errorText                        locales.Text
	verificationError                locales.Text
	fields                           [protectionFieldCount]protectionFieldState
	submitted                        bool
}

func (a *app) businessReady() bool {
	return a.security.ready && !a.security.changing && !a.quitPending && a.ctx.Err() == nil
}

func (a *app) initializeBusiness() error {
	if a.security.initialized {
		return errors.New("application already initialized")
	}
	if a.prefsLoadErr != nil {
		return a.prefsLoadErr
	}
	a.catalogError = a.loadCatalog()
	if a.catalogError != nil {
		return a.catalogError
	}
	a.files.ratio = theme.Current().C.FTPSplit.DefaultRatio
	a.files.activeRemote = true
	a.files.localPaths = map[string]string{}
	if err := a.loadFilePreferences(); err != nil {
		return err
	}
	if err := a.loadHostFacts(); err != nil {
		a.persistenceNotice(locales.MessageUnableToReadHostInformation, err)
	}
	a.restoreWorkspace()
	if err := a.restoreTransfers(); err != nil {
		return err
	}
	a.initMCPUI()
	if err := a.initMCPBackend(); err != nil {
		a.persistenceNotice(locales.MessageUnableToInitializeMcp, err)
	}
	a.security.initialized = true
	a.security.ready = true
	a.security.status = "ready"
	a.security.errorText = locales.Text{}
	a.security.password = ""
	for id := range a.security.migrationPhrases {
		delete(a.security.migrationPhrases, id)
	}
	a.security.migrationKey = nil
	a.security.migrationPassword = nil
	a.security.migration = false
	if len(a.tabs) > 0 && a.section == sectionSSH {
		a.activateTab(a.activeTab)
	}
	a.startMCPBackend()
	return nil
}

func (a *app) completeStartup(err error) {
	a.security.busy = false
	if a.ctx.Err() != nil {
		return
	}
	if err == nil {
		err = a.initializeBusiness()
	}
	if err != nil {
		a.security.errorText = locales.ErrorText(err)
		a.security.status = "blocked"
	}
	a.window.Invalidate()
}

func (a *app) beginStartup() {
	if a.vault == nil || a.security.busy || a.security.ready {
		return
	}
	state := a.vault.State()
	if state == "pending_migration" {
		a.security.status = "migration"
		return
	}
	if state == "ready" && a.vault.RequiresPassword() {
		a.security.status = "locked"
		return
	}
	a.security.busy = true
	a.sshWorkers.Go(func() {
		var err error
		if state == "pending_init" {
			err = a.vault.Initialize(a.ctx, nil, nil)
		}
		a.window.Update(func() { a.completeStartup(err) })
	})
}

func (a *app) submitStartup() {
	if a.security.busy || a.vault == nil || a.security.status == "blocked" {
		return
	}
	a.security.busy = true
	a.security.errorText = locales.Text{}
	state := a.vault.State()
	input := []byte(a.security.password)
	a.security.password = ""
	phrases := map[string]string{}
	for id, phrase := range a.security.migrationPhrases {
		phrases[id] = phrase
	}
	if a.security.migrationKey != nil {
		phrases[a.security.migrationKey.ID] = string(input)
	}
	if a.security.migrationPassword != nil {
		phrases[a.security.migrationPassword.ID] = string(input)
	}
	a.sshWorkers.Go(func() {
		defer clear(input)
		var err error
		if state == "pending_migration" {
			err = a.migrateVault(a.ctx, phrases)
		} else {
			err = a.vault.Unlock(a.ctx, input)
		}
		a.window.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			var missing *migrationPassphraseError
			var missingPassword *migrationCredentialError
			if errors.As(err, &missing) || errors.As(err, &missingPassword) {
				a.security.migrationKey = missing
				a.security.migrationPassword = missingPassword
				a.security.migrationPhrases = phrases
			}
			if err != nil {
				a.security.busy = false
				a.security.errorText = locales.ErrorText(err)
				if a.vault.State() == "blocked" {
					a.security.status = "blocked"
				}
				a.window.Invalidate()
				return
			}
			a.completeStartup(nil)
		})
	})
}

func (a *app) startupView(c *ui.Context) {
	t := theme.Current()
	a.theme = uikit.Install(c)
	c.Root().Background(uikit.Color(t.C.Window.Bg))
	ui.Column(c).Fill().Children(func() {
		uikit.TitleBar(c).FillWidth()
		ui.Column(c).Grow(1).FillWidth().Center().Padding(t.Space.N24).Children(func() {
			uikit.Card(c).Width(t.C.Modal.GenerateWidth).Padding(t.Space.N24).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space.N16).Children(func() {
					uikit.Text(c, "Burrow", uikit.TextProps{Style: uikit.TextTitle})
					migration := a.security.migration
					title := locales.MessageEnterYourUnlockPasswordToOpenTheApp()
					if !a.security.requirePassword {
						title = locales.MessageOpeningTheApp()
					}
					if a.security.status == "blocked" {
						title = locales.MessageUnableToOpenTheApp()
					}
					if migration {
						title = locales.MessageMigrateSavedCredentials()
					}
					if a.security.status == "blocked" {
						title = locales.MessageUnableToOpenTheApp()
					}
					uikit.Text(c, title, uikit.TextProps{Style: uikit.TextBodyStrong})
					if migration {
						uikit.Text(
							c,
							locales.MessageReadExistingSystemCredentialsAndManagedKeysOnceToMigrateThemNormalUse(),
							uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
						)
					}
					showPassword := !migration && a.security.requirePassword || a.security.migrationKey != nil || a.security.migrationPassword != nil
					submitted := false
					if showPassword && a.security.status != "blocked" {
						label := locales.MessageUnlockPassword()
						if a.security.migrationKey != nil {
							label = locales.MessageOriginalKeyPassphrase()
						}
						if a.security.migrationPassword != nil {
							label = locales.MessageSessionPassword()
						}
						field := uikit.PasswordInput(c, &a.security.password, uikit.InputProps{Label: label, Disabled: a.security.busy})
						submitted = field.Input.AutoFocus().Submitted()
					}
					if !a.security.errorText.Empty() {
						uikit.Banner(c, a.security.errorText.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
					}
					if a.security.busy {
						uikit.Text(c, locales.MessageProcessing(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
					}
					ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
						uikit.Button(c, locales.MessageQuitApp(), uikit.ButtonProps{}).OnClick(func() { mygo.App.Quit() })
						if a.vault != nil && a.security.status != "blocked" && (migration || a.security.requirePassword) {
							label := locales.MessageUnlock()
							if migration {
								label = locales.MessageReadAndMigrateCredentials()
							}
							if uikit.Button(c, label, uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: a.security.busy}).Clicked() || submitted {
								a.submitStartup()
							}
						}
					})
				})
			})
		})
	})
}

func (a *app) applicationSecuritySettings(c *ui.Context) {
	on := a.security.requirePassword
	fields := []settingField{
		{"MessageRequireUnlockingWhenOpeningTheApp", locales.MessageRequireUnlockingWhenOpeningTheApp(), "", locales.MessageRequireTheUnlockPasswordEachTimeBurrowStarts(), func() {
			selected := on
			if uikit.Switch(c, &selected, uikit.ToggleProps{
				Label: locales.MessageRequireUnlockingWhenOpeningTheApp(), Disabled: a.credentials.busy || a.catalogSaving || a.security.changing,
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}).Changed() {
				kind := "enable"
				if on {
					kind = "disable"
				}
				a.security.dialog = &protectionDraft{kind: kind}
			}
		}},
	}
	if on {
		fields = append(fields, settingField{"change-password", locales.MessageChangePassword(), "", "", func() {
			uikit.Button(
				c,
				locales.MessageChangePassword2(),
				uikit.ButtonProps{Disabled: a.credentials.busy || a.catalogSaving},
			).OnClick(func() { a.security.dialog = &protectionDraft{kind: "change"} })
		}})
	}
	a.settingsGroup(c, "MessageAppUnlock", locales.MessageAppUnlock(), "", fields)
}

func (a *app) applicationSecurityDialog(c *ui.Context) {
	d := a.security.dialog
	if d == nil {
		return
	}
	t := theme.Current()
	title := locales.MessageSetUnlockPassword()
	if d.kind == "change" {
		title = locales.MessageChangeUnlockPassword()
	}
	if d.kind == "disable" {
		title = locales.MessageDisableAppUnlock()
	}
	d.submitted = false
	open := true
	submit := false
	props := uikit.ModalProps{
		Controller: &d.modal,
		Title:      title,
		Width:      t.C.Modal.GenerateWidth,
		Disabled:   a.security.changing,
		Motion:     uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Justify(ui.End).Children(func() {
				uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{Disabled: a.security.changing}).
					OnClick(parts.RequestDismiss)
				submit = uikit.Button(c, locales.CommonSave(), uikit.ButtonProps{
					Variant: uikit.ButtonPrimary, Disabled: a.security.changing,
				}).Clicked()
			})
		},
	}
	result := uikit.Modal(
		c.Key("application-security").Key(d),
		&open,
		props,
		func(parts uikit.ModalParts) {
			uikit.ModalBody(c).Children(func() {
				if d.kind != "enable" {
					d.passwordField(
						c,
						protectionCurrent,
						locales.MessageCurrentPassword(),
						&d.current,
					)
				}
				if d.kind != "disable" {
					d.passwordField(
						c,
						protectionNew,
						locales.MessageNewPassword(),
						&d.password,
					)
					d.passwordField(
						c,
						protectionConfirm,
						locales.MessageConfirmPassword(),
						&d.confirm,
					)
					uikit.Text(c, locales.MessageKeepYourUnlockPasswordSafeSavedPasswordsAndManagedPrivateKeysCannotBe(),
						uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).FillWidth()
				} else {
					uikit.Text(c, locales.MessageBurrowWillOpenWithoutAnUnlockPasswordYourSavedCredentialsWillBeKept(),
						uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).FillWidth()
				}
				if !d.errorText.Empty() {
					uikit.Banner(c, d.errorText.String(), uikit.BannerProps{
						Tone: uikit.ToneDanger, Icon: sessionAlertIcon,
					})
				}
				if a.security.changing {
					uikit.Text(c, locales.MessageProcessing(), uikit.TextProps{
						Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
					})
				}
			})
		},
	)
	if (submit || result.Submit || d.submitted) && !a.security.changing {
		a.changeApplicationProtection(d)
	}
	if result.Dismissed && !a.security.changing {
		d.current, d.password, d.confirm = "", "", ""
		a.security.dialog = nil
	}
}

func (d *protectionDraft) fieldError(field int) locales.Text {
	switch field {
	case protectionCurrent:
		if d.current == "" {
			return locales.Deferred(locales.MessageEnterTheCurrentPassword)
		}
		return d.verificationError
	case protectionNew:
		if d.password == "" {
			return locales.Deferred(locales.MessageEnterANewPassword)
		}
		if err := vault.ValidatePassword(d.password); err != nil {
			return locales.ErrorText(err)
		}
	case protectionConfirm:
		if d.confirm == "" {
			return locales.Deferred(locales.MessageEnterTheNewPasswordAgain)
		}
		if d.confirm != d.password {
			return locales.Deferred(locales.MessageThePasswordsDoNotMatch)
		}
	}
	return locales.Text{}
}

func (d *protectionDraft) validate() bool {
	fields := []int{protectionCurrent, protectionNew, protectionConfirm}
	if d.kind == "enable" {
		fields = fields[1:]
	}
	if d.kind == "disable" {
		fields = fields[:1]
	}
	first := -1
	for _, field := range fields {
		message := d.fieldError(field)
		d.fields[field].errorText = message
		if !message.Empty() && first == -1 {
			first = field
		}
	}
	if first != -1 {
		d.fields[first].handle.Focus()
		return false
	}
	return true
}

func (d *protectionDraft) passwordField(c *ui.Context, field int, label string, value *string) {
	state := &d.fields[field]
	parts := uikit.FormRow(c, label, uikit.FormRowProps{Key: field}, func() ui.Element {
		input := uikit.PasswordInput(c, value, uikit.InputProps{
			Label: label, Handle: &state.handle, DeferStyle: true,
		}).Input.FillWidth()
		first := field == protectionCurrent || d.kind == "enable" && field == protectionNew
		if first {
			input.AutoFocus()
		}
		if input.Changed() {
			state.errorText = locales.Text{}
			d.errorText = locales.Text{}
			if field == protectionCurrent {
				d.verificationError = locales.Text{}
			}
			if field == protectionNew && d.confirm != "" {
				d.fields[protectionConfirm].errorText = d.fieldError(protectionConfirm)
			}
		}
		if state.focused && !input.Focused() {
			state.errorText = d.fieldError(field)
		}
		state.focused = input.Focused()
		uikit.StyleFormControl(input, state.errorText.String())
		if input.Submitted() {
			d.submitted = true
		}
		return input
	})
	parts.Body.Children(func() {
		if !state.errorText.Empty() {
			uikit.FieldMessage(c, state.errorText.String(), uikit.FieldMessageProps{
				Tone: uikit.ToneDanger,
			})
		}
	})
}

func (a *app) changeApplicationProtection(d *protectionDraft) {
	if !a.businessReady() || a.credentials.busy || a.catalogSaving || a.savingSession() {
		return
	}
	if !d.validate() {
		a.window.Invalidate()
		return
	}
	protected := d.kind != "disable"
	a.security.changing = true
	d.errorText = locales.Text{}
	current, next := d.current, d.password
	a.persist(func() error {
		obsolete, err := a.vault.ChangeProtection(a.ctx, current, next, protected)
		current, next = "", ""
		if err == nil {
			// Retire the previous wrapping key only after the new generation
			// committed. Cleanup failure cannot undo the durable transition.
			if cleanupErr := obsolete.Run(mygo.Shell.TrashItem); cleanupErr != nil {
				return &protectionCleanupError{cause: cleanupErr}
			}
		}
		return err
	}, func(err error) {
		a.security.changing = false
		var cleanup *protectionCleanupError
		if err != nil && !errors.As(err, &cleanup) {
			if errors.Is(err, vault.ErrUnlock) {
				d.verificationError = locales.ErrorText(err)
				d.fields[protectionCurrent].errorText = d.verificationError
				d.fields[protectionCurrent].handle.Focus()
			} else {
				d.errorText = locales.ErrorText(err)
			}
			if !a.vault.Unlocked() {
				a.security.ready = false
				a.security.status = "blocked"
				a.security.errorText = locales.ErrorText(err)
			}
			a.window.Invalidate()
			return
		}
		d.current, d.password, d.confirm = "", "", ""
		d.modal.Close()
		a.security.requirePassword = protected
		if cleanup != nil {
			a.credentials.notice = locales.Concat(
				locales.Deferred(locales.MessageUnlockSettingsSavedButPreviousUnlockMaterialCouldNotBeCleanedUp),
				locales.ErrorText(cleanup.cause),
			)
		}
		a.scheduleTransfers()
	})
}

type protectionCleanupError struct{ cause error }

func (e *protectionCleanupError) Error() string { return e.cause.Error() }

func (a *app) securitySettingsMatch() bool {
	return a.settingMatches(
		locales.MessageAppUnlock(),
		"打开应用时需要解锁 修改密码 解锁密码 application unlock password",
	)
}

func (a *app) clearSecurityInputs() {
	a.security.password = ""
	if d := a.gatewayUI.draft; d != nil {
		d.wg.private, d.wg.psk = "", ""
	}
	if d := a.security.dialog; d != nil {
		d.current, d.password, d.confirm = "", "", ""
	}
	for id := range a.security.migrationPhrases {
		delete(a.security.migrationPhrases, id)
	}
}
