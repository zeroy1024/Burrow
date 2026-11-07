package desktop

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) keySettings(c *ui.Context) {
	t := theme.Current()
	p := t.C.Settings
	a.applicationSecuritySettings(c)
	a.credentialGroup(c, locales.MessagePrivateKeys(), func() {
		uikit.Button(c, locales.MessageImport2(), uikit.ButtonProps{Disabled: a.credentials.busy}).OnClick(a.choosePrivateKey)
		uikit.Button(
			c,
			locales.MessageGenerate2(),
			uikit.ButtonProps{Disabled: a.credentials.busy},
		).OnClick(a.beginKeyGeneration)
	}, func() {
		if len(a.keys) == 0 {
			uikit.Text(
				c,
				locales.MessageNoPrivateKeysYetImportAKeyToUseItWithSshOr(),
				uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
			).Padding(p.RowPaddingY, p.RowPaddingX)
		}
		for _, key := range a.keys {
			key := key
			query := strings.ToLower(strings.TrimSpace(a.settingsUI.query))
			if query != "" && !strings.Contains(
				strings.ToLower(
					locales.MessagePrivateKeys()+" "+locales.MessageKeysCredentials()+" "+key.Name+" "+key.Type+" "+key.Fingerprint+" ",
				),
				query,
			) {
				continue
			}
			row := uikit.SettingsRow(c, uikit.SettingsRowProps{Key: key.ID, Gap: t.Space.N12})
			if a.credentials.flashID == key.ID {
				if a.credentials.flashAt.IsZero() {
					a.credentials.flashAt = c.Now()
				}
				started := a.credentials.flashAt
				row.Draw(func(p *ui.Painter, rect ui.Rect) {
					elapsed := p.Now().Sub(started)
					if elapsed >= t.Motion.Duration.Flash {
						return
					}
					alpha := 1 - float32(elapsed)/float32(t.Motion.Duration.Flash)
					ink := theme.WithAlpha(t.Color.Accent.Default, float64(alpha)*float64(t.Color.Accent.Tint10.A)/255)
					p.Fill(rect, uikit.Color(ink), 0)
					p.AnimationFrame()
				})
			}
			row.Children(func() {
				uikit.IconTile(c, uikit.IconTileProps{Size: uikit.SizeLg}, func() {
					uikit.Icon(c, uikit.IconKey, uikit.IconProps{Tone: uikit.ToneMuted})
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N3).Children(func() {
					uikit.Text(c, key.Name, uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Row(c).Gap(t.Space.N8).Children(func() {
						uikit.Text(
							c,
							fmt.Sprintf(locales.MessageBits(), key.Type, key.Bits),
							uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
						)
						fingerprint := uikit.Button(c, key.Fingerprint, uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeXs}).
							Label(locales.MessageCopyPrivateKeyFingerprint()).Padding(0).Height(t.Font.LineHeight.Sub).
							Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc)).
							OnClick(func() { mygo.Clipboard.WriteText(key.Fingerprint) })
						uikit.Tip(c, fingerprint, locales.MessageCopyFingerprint())
					})
					uikit.Text(
						c,
						locales.MessageEncryptedInBurrow(),
						uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
					)
					uikit.Text(
						c,
						locales.KeysUsageCount(a.keyUseCount(key.ID)),
						uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
					)
				})
				uikit.MenuOn(
					c,
					uikit.IconButton(
						c,
						moreIcon,
						uikit.IconButtonProps{Label: locales.MessageKeyActions(), Disabled: a.credentials.busy},
					),
					a.managedKeyMenu(c, key),
				)
			})
		}
	})
	a.credentialGroup(c, "ssh-agent", nil, func() {
		uikit.SettingsRow(c, uikit.SettingsRowProps{Gap: t.Space.N12}).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N4).Children(func() {
				label, tone := locales.MessageSshAgentNotChecked(), uikit.ToneSecondary
				if a.credentials.agentLoading {
					label = locales.MessageCheckingSshAgent()
				} else if a.credentials.agentChecked {
					if a.credentials.agentError != nil {
						label = locales.MessageSshAgentNotDetected()
					} else {
						label = locales.KeysAgentConnectedCount(a.credentials.agentCount)
						tone = uikit.ToneSuccess
					}
				}
				uikit.Text(c, label, uikit.TextProps{Style: uikit.TextBody, Tone: tone})
				if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
					uikit.MiddleLabel(c, socket, uikit.MiddleLabelProps{
						Font:  ui.Font{Family: strings.Join(t.Font.Family.Mono, ", "), Size: t.Font.Size.Footnote, Weight: t.Font.Weight.Regular},
						Color: uikit.Color(t.Color.Text.Secondary), Path: true, Prefix: "SSH_AUTH_SOCK=",
					}).FillWidth()
				}
				if a.credentials.agentError != nil {
					uikit.Text(c, a.credentials.agentError.Error(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				}
			})
			uikit.Button(
				c,
				locales.MessageRefresh(),
				uikit.ButtonProps{Disabled: a.credentials.agentLoading},
			).OnClick(a.refreshCredentialAgent)
		})
		uikit.Button(c, locales.MessageAboutSshAgent(), uikit.ButtonProps{}).OnClick(func() {
			go func() {
				err := mygo.Shell.OpenExternal("https://www.openssh.com/manual.html")
				if err != nil {
					mygo.RunOnMain(func() {
						a.credentials.notice = locales.ErrorText(err)
						if a.window != nil {
							a.window.Invalidate()
						}
					})
				}
			}()
		})
	})
	count := 0
	for _, h := range a.hosts {
		if h.session.PasswordSaved {
			count++
		}
	}
	a.credentialGroup(c, locales.MessageSavedPasswords(), nil, func() {
		a.credentialRow(c, locales.KeysSavedPasswordCount(
			count,
		), locales.MessagePasswordsAndKeyPassphrasesAreEncryptedOnThisDevice(), func() {
			uikit.Button(
				c,
				locales.MessageClearAll2(),
				uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Disabled: count == 0 || a.credentials.busy},
			).OnClick(func() { a.credentials.draft = &credentialDraft{kind: "clear-passwords"} })
		})
	})
	if a.credentials.busy {
		uikit.Text(c, locales.MessageProcessingKey(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
	}
	if !a.credentials.notice.Empty() {
		uikit.Text(c, a.credentials.notice.String(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
	}
}

func (a *app) credentialGroup(c *ui.Context, title string, actions, content func()) {
	if strings.TrimSpace(a.settingsUI.query) != "" {
		description := ""
		switch title {
		case locales.MessagePrivateKeys():
			description = "private keys import generate 导入 生成"
			for _, key := range a.keys {
				description += key.Name + key.Type + key.Fingerprint
			}
		case "ssh-agent":
			description = "ssh-agent add refresh 添加到 agent 使用私钥连接成功后添加到 ssh-agent 已连接 刷新"
		case locales.MessageSavedPasswords():
			description = "saved passwords clear all 系统钥匙串 保存的密码 全部清除"
		}
		if !a.settingMatches(title, description) {
			return
		}
	}
	uikit.SettingsGroup(c, uikit.SettingsGroupProps{Title: title, Actions: actions}, content)
}

func (a *app) credentialRow(c *ui.Context, label, help string, control func()) {
	if !a.settingMatches(label, help) {
		return
	}
	uikit.SettingsRow(c, uikit.SettingsRowProps{
		Gap: theme.Current().Space.N12, Title: label, Help: help, Control: control,
	})
}

func (a *app) managedKeyMenu(c *ui.Context, key managedKey) []uikit.MenuEntry {
	used := a.keyUseCount(key.ID)
	label := locales.MessageDelete()
	if used > 0 {
		label = locales.KeysDeleteUsedCount(used)
	}
	return []uikit.MenuEntry{
		{ID: "rename", Label: locales.MessageRename2(), Icon: uikit.IconEdit, OnChoose: func() {
			a.credentials.draft = &credentialDraft{kind: "rename", keyID: key.ID, name: key.Name}
		}},
		{ID: "copy-public", Label: locales.MessageCopyPublicKey(), Icon: uikit.IconCopy, OnChoose: func() { a.copyManagedPublicKey(key) }},
		{ID: "export", Label: locales.MessageExportPrivateKey2(), Icon: uikit.IconFolder, OnChoose: func() { a.exportManagedKey(key) }},
		{ID: "before-delete", Separator: true},
		{ID: "delete", Label: label, Icon: uikit.IconTrash, Disabled: used > 0, Danger: true, OnChoose: func() {
			a.credentials.draft = &credentialDraft{kind: "delete", keyID: key.ID, name: key.Name}
		}},
	}
}

type credentialFormValues struct {
	name, comment, keyType string
	remember               bool
}

func (a *app) credentialDialogs(c *ui.Context) {
	d := a.credentials.draft
	if d == nil {
		return
	}
	t := theme.Current()
	p := t.C.Modal
	title, action := locales.MessageImportPrivateKey(), locales.MessageImport()
	switch d.kind {
	case "generate":
		title, action = locales.MessageGenerateSshKey(), locales.MessageGenerate()
	case "rename":
		title, action = locales.MessageRenameKey(), locales.CommonSave()
	case "delete":
		title, action = locales.MessageDeleteKey(), locales.CommonDelete()
	case "clear-passwords":
		title, action = locales.MessageClearSavedPasswords(), locales.MessageClearAll()
	}
	open := true
	props := uikit.ModalProps{Width: p.GenerateWidth, Title: title, Disabled: a.credentials.busy,
		Controller: &d.modal,
		CloseLabel: locales.MessageCloseKeyWindow(), Motion: uikit.MotionOverride(a.reduceMotion(c))}
	props.Header = func(parts uikit.ModalParts) {
		parts.Panel.Padding(p.HeaderPaddingTop, p.BodyPaddingX, p.FooterPaddingBottom).Gap(t.Space.N14)
		uikit.ModalHeader(c, props)
	}
	submit := func() {
		switch d.kind {
		case "rename":
			a.renameManagedKey(d)
		case "delete":
			a.deleteManagedKey(d)
		case "clear-passwords":
			a.clearCredentialPasswords()
		default:
			a.submitCredentialDraft()
		}
	}
	result := uikit.Modal(c.Key("credential-dialog").Key(d), &open, props, func(parts uikit.ModalParts) {
		current := credentialFormValues{name: d.name, comment: d.comment, keyType: d.keyType, remember: d.remember}
		initial := ui.Local(parts.Panel, "uikit.credentials.initial", func() credentialFormValues { return current })
		parts.SetDirty(current != *initial || d.passphrase != "")
		if d.kind == "delete" {
			uikit.Text(c, fmt.Sprintf(locales.MessageDeleteTheManagedPrivateKeyThisCannotBeUndone(), d.name), uikit.TextProps{})
		} else if d.kind == "clear-passwords" {
			uikit.Text(c, locales.MessageClearAllSavedSessionPasswordsYouWillNeedToEnterThemAgainWhen(), uikit.TextProps{})
		} else {
			credentialField(c, locales.CommonName(), func() ui.Element {
				return uikit.Input(c, &d.name, uikit.InputProps{}).Label(locales.MessageKeyName()).AutoFocus().FillWidth()
			})
			if d.kind == "generate" {
				credentialField(c, locales.MessageType(), func() ui.Element {
					// Its popup lives outside the disabled panel; reset that transient
					// state when work starts so an already open menu cannot keep editing.
					return uikit.Select(
						c.Key(a.credentials.busy),
						&d.keyType,
						uikit.StringOptions([]string{"Ed25519", "RSA 4096"}),
						uikit.SelectProps{Label: locales.MessageKeyType(), Disabled: a.credentials.busy},
					).Label(locales.MessageKeyType())
				})
				credentialField(c, locales.MessageComment(), func() ui.Element {
					return uikit.Input(c, &d.comment, uikit.InputProps{}).Label(locales.MessageKeyComment()).FillWidth()
				})
			}
			if d.kind == "generate" || d.kind == "import" {
				credentialField(c, locales.MessagePassphrase(), func() ui.Element {
					return uikit.PasswordInput(
						c,
						&d.passphrase,
						uikit.InputProps{Label: locales.MessagePrivateKeyPassphrase(), Mono: true},
					).Input.FillWidth()
				})
				uikit.Checkbox(c, &d.remember, locales.MessageSaveTheKeyPassphraseWithEncryption())
			}
		}
		if !d.errorText.Empty() {
			uikit.Banner(c, d.errorText.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		if a.credentials.busy {
			uikit.Text(c, locales.MessageProcessing(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(p.FooterGap).Children(func() {
			uikit.Button(
				c,
				locales.CommonCancel(),
				uikit.ButtonProps{Disabled: a.credentials.busy},
			).OnClick(parts.RequestDismiss)
			variant := uikit.ButtonPrimary
			if d.kind == "delete" || d.kind == "clear-passwords" {
				variant = uikit.ButtonDanger
			}
			actionButton := uikit.Button(c, action, uikit.ButtonProps{Variant: variant, Disabled: a.credentials.busy})
			actionButton.OnClick(submit)
		})
	})
	if result.Submit && !a.credentials.busy {
		submit()
	}
	if result.Dismissed && !a.credentials.busy {
		clear(d.data)
		d.passphrase = ""
		a.credentials.draft = nil
	}
}

func credentialField(c *ui.Context, label string, control func() ui.Element) {
	uikit.FormRow(c, label, uikit.FormRowProps{Inline: true}, control)
}

func (a *app) renameManagedKey(d *credentialDraft) {
	name := strings.TrimSpace(d.name)
	if name == "" {
		d.errorText = locales.Deferred(locales.MessageEnterAKeyName)
		return
	}
	index := slices.IndexFunc(a.keys, func(key managedKey) bool { return key.ID == d.keyID })
	if index < 0 {
		d.errorText = locales.Deferred(locales.MessageTheKeyNoLongerExists)
		return
	}
	old := a.keys[index].Name
	a.keys[index].Name = name
	a.saveCatalogAsync(func(err error) {
		if err != nil {
			a.keys[index].Name = old
			d.errorText = locales.ErrorText(err)
			return
		}
		a.dismissCredentialDraft()
	})
}

func (a *app) deleteManagedKey(d *credentialDraft) {
	if a.keyUseCount(d.keyID) > 0 {
		d.errorText = locales.Deferred(locales.MessageThisKeyIsUsedByASessionAndCannotBeDeleted)
		return
	}
	key, err := a.keyByID(d.keyID)
	if err != nil {
		d.errorText = locales.ErrorText(err)
		return
	}
	a.credentialWork(func(ctx context.Context) credentialResult {
		if err := ctx.Err(); err != nil {
			return credentialResult{err: err}
		}
		changes, err := a.vault.Prepare(nil, []string{key.SecretID, "key/" + key.ID + "/passphrase"})
		return credentialResult{deletedID: key.ID, changes: changes, notice: locales.Deferred(
			locales.MessageManagedPrivateKeyDeleted,
		), err: err}
	})
}

func (a *app) clearCredentialPasswords() {
	var ids []string
	for _, h := range a.hosts {
		if h.session.PasswordSaved {
			ids = append(ids, h.session.ID)
		}
	}
	a.credentialWork(func(ctx context.Context) credentialResult { return a.clearStoredPasswords(ctx, ids) })
}
