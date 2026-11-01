package desktop

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) keySettings(c *ui.Context) {
	t := theme.Current()
	p := t.C.Settings
	a.credentialGroup(c, "私钥", func() {
		uikit.Button(c, localize(c, "导入…"), uikit.ButtonProps{Disabled: a.credentials.busy}).OnClick(a.choosePrivateKey)
		uikit.Button(c, localize(c, "生成…"), uikit.ButtonProps{Disabled: a.credentials.busy}).OnClick(a.beginKeyGeneration)
	}, func() {
		if len(a.keys) == 0 {
			uikit.Text(c, localize(c, "尚未导入私钥。导入后可用于 SSH 或 SFTP 会话。"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Padding(p.RowPaddingY, p.RowPaddingX)
		}
		for _, key := range a.keys {
			key := key
			query := strings.ToLower(strings.TrimSpace(a.settingsUI.query))
			if query != "" && !strings.Contains(strings.ToLower(localize(c, "私钥")+" "+localize(c, "密钥与凭据")+" "+key.Name+" "+key.Type+" "+key.Fingerprint+" "+key.Path), query) {
				continue
			}
			row := uikit.SettingsRow(c, uikit.SettingsRowProps{Key: key.ID, Gap: t.Space.N12})
			if a.credentials.flashID == key.ID {
				if a.credentials.flashAt.IsZero() {
					a.credentials.flashAt = c.Now()
				}
				elapsed := c.Now().Sub(a.credentials.flashAt)
				if elapsed < t.Motion.Duration.Flash {
					alpha := 1 - float32(elapsed)/float32(t.Motion.Duration.Flash)
					row.Background(uikit.Color(theme.WithAlpha(t.Color.Accent.Default, float64(alpha)*float64(t.Color.Accent.Tint10.A)/255)))
					c.AnimationFrame()
				}
			}
			row.Children(func() {
				ui.Box(c).Size(t.C.Modal.IconTile, t.C.Modal.IconTile).Radius(t.C.Modal.IconTileRadius).Background(uikit.Color(t.Color.Surface.Control)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					uikit.Icon(c, uikit.IconKey, uikit.IconProps{Tone: uikit.ToneMuted})
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N3).Children(func() {
					uikit.Text(c, key.Name, uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Row(c).Gap(t.Space.N8).Children(func() {
						uikit.Text(c, localize(c, fmt.Sprintf(localize(c, "%s · %d 位"), key.Type, key.Bits)), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
						fingerprint := uikit.Button(c, key.Fingerprint, uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeXs}).
							Label(localize(c, "复制私钥指纹")).Padding(0).Height(t.Font.LineHeight.Sub).
							Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc)).
							OnClick(func() { mygo.Clipboard.WriteText(key.Fingerprint) })
						uikit.Tip(c, fingerprint, localize(c, "复制指纹"))
					})
					uikit.MiddleLabel(c, key.Path, uikit.MiddleLabelProps{
						Font:  ui.Font{Family: strings.Join(t.Font.Family.Mono, ", "), Size: t.Font.Size.Footnote, Weight: t.Font.Weight.Regular},
						Color: uikit.Color(t.Color.Text.Secondary), Path: true,
					}).FillWidth()
					uikit.Text(c, localize(c, fmt.Sprintf(localize(c, "被 %d 个会话使用"), a.keyUseCount(key.ID))), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				})
				uikit.MenuOn(c, uikit.IconButton(c, moreIcon, uikit.IconButtonProps{Label: localize(c, "密钥操作"), Disabled: a.credentials.busy}), a.managedKeyMenu(c, key))
			})
		}
	})
	a.credentialGroup(c, "ssh-agent", nil, func() {
		uikit.SettingsRow(c, uikit.SettingsRowProps{Gap: t.Space.N12}).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N4).Children(func() {
				label, tone := "尚未检测 ssh-agent", uikit.ToneSecondary
				if a.credentials.agentLoading {
					label = "正在检测 ssh-agent…"
				} else if a.credentials.agentChecked {
					if a.credentials.agentError != nil {
						label = "未检测到 ssh-agent"
					} else {
						label = fmt.Sprintf(localize(c, "已连接 · %d 把密钥"), a.credentials.agentCount)
						tone = uikit.ToneSuccess
					}
				}
				uikit.Text(c, localize(c, label), uikit.TextProps{Style: uikit.TextBody, Tone: tone})
				if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
					uikit.MiddleLabel(c, socket, uikit.MiddleLabelProps{
						Font:  ui.Font{Family: strings.Join(t.Font.Family.Mono, ", "), Size: t.Font.Size.Footnote, Weight: t.Font.Weight.Regular},
						Color: uikit.Color(t.Color.Text.Secondary), Path: true, Prefix: "SSH_AUTH_SOCK=",
					}).FillWidth()
				}
				if a.credentials.agentError != nil {
					uikit.Text(c, localize(c, a.credentials.agentError.Error()), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				}
			})
			uikit.Button(c, localize(c, "刷新"), uikit.ButtonProps{Disabled: a.credentials.agentLoading}).OnClick(a.refreshCredentialAgent)
		})
		uikit.Button(c, localize(c, "ssh-agent 使用说明"), uikit.ButtonProps{}).OnClick(func() {
			go func() {
				err := mygo.Shell.OpenExternal("https://www.openssh.com/manual.html")
				if err != nil {
					mygo.RunOnMain(func() {
						a.credentials.notice = err.Error()
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
	a.credentialGroup(c, "已保存的密码", nil, func() {
		a.credentialRow(c, fmt.Sprintf(localize(c, "%d 个会话保存了密码"), count), "密码和私钥口令保存在系统钥匙串中", func() {
			uikit.Button(c, localize(c, "全部清除…"), uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Disabled: count == 0 || a.credentials.busy}).OnClick(func() { a.credentials.draft = &credentialDraft{kind: "clear-passwords"} })
		})
	})
	if a.credentials.busy {
		uikit.Text(c, localize(c, "正在处理密钥…"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
	}
	if a.credentials.notice != "" {
		uikit.Text(c, localize(c, a.credentials.notice), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
	}
}

func (a *app) credentialGroup(c *ui.Context, title string, actions, content func()) {
	if strings.TrimSpace(a.settingsUI.query) != "" {
		description := ""
		switch title {
		case "私钥":
			description = "private keys import generate 导入 生成"
			for _, key := range a.keys {
				description += key.Name + key.Type + key.Fingerprint + key.Path
			}
		case "ssh-agent":
			description = "ssh-agent add refresh 添加到 agent 使用私钥连接成功后添加到 ssh-agent 已连接 刷新"
		case "已保存的密码":
			description = "saved passwords clear all 系统钥匙串 保存的密码 全部清除"
		}
		if !a.settingMatches(title, description) {
			return
		}
	}
	uikit.SettingsGroup(c, uikit.SettingsGroupProps{Title: localize(c, title), HeaderRow: true, Actions: actions}, content)
}

func (a *app) credentialRow(c *ui.Context, label, description string, control func()) {
	if !a.settingMatches(label, description) {
		return
	}
	uikit.SettingsRow(c, uikit.SettingsRowProps{
		Gap: theme.Current().Space.N12, Title: localize(c, label), Detail: localize(c, description), Control: control,
	})
}

func (a *app) managedKeyMenu(c *ui.Context, key managedKey) []uikit.MenuEntry {
	used := a.keyUseCount(key.ID)
	label := "删除…"
	if used > 0 {
		label = fmt.Sprintf(localize(c, "删除（被 %d 个会话使用）"), used)
	}
	return []uikit.MenuEntry{
		{ID: "rename", Label: localize(c, "重命名…"), Icon: uikit.IconEdit, OnChoose: func() {
			a.credentials.draft = &credentialDraft{kind: "rename", keyID: key.ID, name: key.Name}
		}},
		{ID: "copy-public", Label: localize(c, "复制公钥"), Icon: uikit.IconCopy, OnChoose: func() { a.copyManagedPublicKey(key) }},
		{ID: "reveal", Label: localize(c, "在文件管理器中显示"), Icon: uikit.IconFolder, OnChoose: func() { mygo.Shell.ShowItemInFolder(key.Path) }},
		{ID: "before-delete", Separator: true},
		{ID: "delete", Label: localize(c, label), Icon: uikit.IconTrash, Disabled: used > 0, Danger: true, OnChoose: func() {
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
	title, action := "导入私钥", "导入"
	switch d.kind {
	case "generate":
		title, action = "生成 SSH 密钥", "生成"
	case "rename":
		title, action = "重命名密钥", "保存"
	case "delete":
		title, action = "删除密钥", "移到废纸篓"
	case "clear-passwords":
		title, action = "清除已保存的密码", "全部清除"
	}
	open := true
	props := uikit.ModalProps{Width: p.GenerateWidth, Title: localize(c, title), Disabled: a.credentials.busy,
		CloseLabel: localize(c, "关闭密钥窗口"), Motion: uikit.MotionOverride(a.reduceMotion(c))}
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
			uikit.Text(c, localize(c, fmt.Sprintf(localize(c, "确认将「%s」的私钥移到系统废纸篓？"), d.name)), uikit.TextProps{})
		} else if d.kind == "clear-passwords" {
			uikit.Text(c, localize(c, "清除系统钥匙串中的全部会话密码。下次连接时需要重新输入。"), uikit.TextProps{})
		} else {
			credentialField(c, "名称", func() ui.Element {
				return uikit.Input(c, &d.name, uikit.InputProps{}).Label(localize(c, "密钥名称")).AutoFocus().FillWidth()
			})
			if d.kind == "generate" {
				credentialField(c, "类型", func() ui.Element {
					// Its popup lives outside the disabled panel; reset that transient
					// state when work starts so an already open menu cannot keep editing.
					return uikit.Select(c.Key(a.credentials.busy), &d.keyType, uikit.StringOptions([]string{"Ed25519", "RSA 4096"}), uikit.SelectProps{Label: localize(c, "密钥类型"), Disabled: a.credentials.busy}).Label(localize(c, "密钥类型"))
				})
				credentialField(c, "注释", func() ui.Element {
					return uikit.Input(c, &d.comment, uikit.InputProps{}).Label(localize(c, "密钥注释")).FillWidth()
				})
			}
			if d.kind == "generate" || d.kind == "import" {
				credentialField(c, "口令", func() ui.Element {
					return uikit.PasswordInput(c, &d.passphrase, uikit.InputProps{Label: localize(c, "私钥口令"), Mono: true}).Input.FillWidth()
				})
				uikit.Checkbox(c, &d.remember, localize(c, "将口令保存在系统钥匙串"))
			}
		}
		if d.errorText != "" {
			uikit.Banner(c, localize(c, d.errorText), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		if a.credentials.busy {
			uikit.Text(c, localize(c, "正在处理…"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(p.FooterGap).Children(func() {
			uikit.Button(c, localize(c, "取消"), uikit.ButtonProps{Disabled: a.credentials.busy}).OnClick(parts.RequestDismiss)
			variant := uikit.ButtonPrimary
			if d.kind == "delete" || d.kind == "clear-passwords" {
				variant = uikit.ButtonDanger
			}
			actionButton := uikit.Button(c, localize(c, action), uikit.ButtonProps{Variant: variant, Disabled: a.credentials.busy})
			actionButton.OnClick(submit)
		})
	})
	if result.Submit && !a.credentials.busy {
		submit()
	}
	if result.Dismissed && !a.credentials.busy {
		a.dismissCredentialDraft()
	}
}

func credentialField(c *ui.Context, label string, control func() ui.Element) {
	uikit.FormRow(c, localize(c, label), uikit.FormRowProps{Inline: true}, control)
}

func (a *app) renameManagedKey(d *credentialDraft) {
	name := strings.TrimSpace(d.name)
	if name == "" {
		d.errorText = "请填写密钥名称"
		return
	}
	index := slices.IndexFunc(a.keys, func(key managedKey) bool { return key.ID == d.keyID })
	if index < 0 {
		d.errorText = "密钥已不存在"
		return
	}
	old := a.keys[index].Name
	a.keys[index].Name = name
	if err := a.saveCatalog(); err != nil {
		a.keys[index].Name = old
		d.errorText = err.Error()
		return
	}
	a.dismissCredentialDraft()
}

func (a *app) deleteManagedKey(d *credentialDraft) {
	if a.keyUseCount(d.keyID) > 0 {
		d.errorText = "密钥仍被会话使用，不能删除"
		return
	}
	key, err := a.keyByID(d.keyID)
	if err != nil {
		d.errorText = err.Error()
		return
	}
	index := slices.IndexFunc(a.keys, func(candidate managedKey) bool { return candidate.ID == key.ID })
	previous := slices.Clone(a.keys)
	previousWorkspace := a.savedWorkspace
	a.savedWorkspace = a.workspaceSnapshot()
	a.keys = slices.Delete(a.keys, index, index+1)
	if err := a.saveCatalog(); err != nil {
		a.keys = previous
		a.savedWorkspace = previousWorkspace
		d.errorText = "保存凭据目录：" + err.Error()
		return
	}
	a.credentialWork(func(ctx context.Context) credentialResult { return trashManagedKey(ctx, key, index) })
}

func (a *app) clearCredentialPasswords() {
	var ids []string
	for _, h := range a.hosts {
		if h.session.PasswordSaved {
			ids = append(ids, h.session.ID)
		}
	}
	a.credentialWork(func(ctx context.Context) credentialResult { return clearStoredPasswords(ctx, ids) })
}
