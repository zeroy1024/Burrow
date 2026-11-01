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

var credentialKeyIcon = lineIcon(`<circle cx="8" cy="8" r="5"/><path d="m12 12 9 9m-3-3 3-3m-6 0 3-3"/>`)

func (a *app) keySettings(c *ui.Context) {
	t := theme.Current()
	p := t.C.Settings
	a.credentialGroup(c, "私钥", func() {
		uikit.Button(c, localize(c, "导入…"), uikit.ButtonProps{}).Disabled(a.credentials.busy).OnClick(a.choosePrivateKey)
		uikit.Button(c, localize(c, "生成…"), uikit.ButtonProps{}).Disabled(a.credentials.busy).OnClick(a.beginKeyGeneration)
	}, func() {
		if len(a.keys) == 0 {
			ui.Text(c, localize(c, "尚未导入私钥。导入后可用于 SSH 或 SFTP 会话。")).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc)).Padding(p.RowPaddingY, p.RowPaddingX)
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
					ui.Icon(c, credentialKeyIcon).Size(t.Size.Icon, t.Size.Icon).TextColor(uikit.Color(t.Color.Text.Muted))
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N3).Children(func() {
					ui.Text(c, key.Name).FontSize(p.RowLabelSize).FontWeight(t.Font.Weight.Medium)
					ui.Row(c).Gap(t.Space.N8).Children(func() {
						ui.Text(c, localize(c, fmt.Sprintf(localize(c, "%s · %d 位"), key.Type, key.Bits))).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc))
						fingerprint := ui.ButtonBase(c).Label(localize(c, "复制私钥指纹")).OnClick(func() { mygo.Clipboard.WriteText(key.Fingerprint) })
						uikit.Tip(c, fingerprint, localize(c, "复制指纹")).Children(func() {
							ui.Text(c, key.Fingerprint).Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc)).SingleLine()
						})
					})
					ui.Text(c, key.Path).Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(p.GroupFootnoteSize).TextColor(uikit.Color(p.GroupFootnote)).SingleLine()
					ui.Text(c, localize(c, fmt.Sprintf(localize(c, "被 %d 个会话使用"), a.keyUseCount(key.ID)))).FontSize(p.GroupFootnoteSize).TextColor(uikit.Color(p.GroupFootnote))
				})
				uikit.IconButton(c, moreIcon, uikit.IconButtonProps{Label: localize(c, "密钥操作")}).Disabled(a.credentials.busy).Menu(func(m *ui.Menu) { a.managedKeyMenu(c, m, key) })
			})
		}
	})
	a.credentialGroup(c, "ssh-agent", nil, func() {
		uikit.SettingsRow(c, uikit.SettingsRowProps{Gap: t.Space.N12}).Children(func() {
			ui.Column(c).Grow(1).Gap(t.Space.N4).Children(func() {
				label, col := "尚未检测 ssh-agent", t.Color.Text.Secondary
				if a.credentials.agentLoading {
					label = "正在检测 ssh-agent…"
				} else if a.credentials.agentChecked {
					if a.credentials.agentError != nil {
						label = "未检测到 ssh-agent"
					} else {
						label = fmt.Sprintf(localize(c, "已连接 · %d 把密钥"), a.credentials.agentCount)
						col = t.Color.Success.Text
					}
				}
				ui.Text(c, localize(c, label)).FontSize(p.RowLabelSize).TextColor(uikit.Color(col))
				if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
					ui.Text(c, localize(c, "SSH_AUTH_SOCK="+socket)).Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc)).SingleLine()
				}
				if a.credentials.agentError != nil {
					ui.Text(c, localize(c, a.credentials.agentError.Error())).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc))
				}
			})
			uikit.Button(c, localize(c, "刷新"), uikit.ButtonProps{}).Disabled(a.credentials.agentLoading).OnClick(a.refreshCredentialAgent)
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
			uikit.Button(c, localize(c, "全部清除…"), uikit.ButtonProps{}).Disabled(count == 0 || a.credentials.busy).OnClick(func() { a.credentials.draft = &credentialDraft{kind: "clear-passwords"} })
		})
	})
	if a.credentials.busy {
		ui.Text(c, localize(c, "正在处理密钥…")).FontSize(p.GroupFootnoteSize).TextColor(uikit.Color(p.GroupFootnote))
	}
	if a.credentials.notice != "" {
		ui.Text(c, localize(c, a.credentials.notice)).FontSize(p.RowDescSize).TextColor(uikit.Color(t.Color.Text.Secondary))
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
	t := theme.Current()
	p := t.C.Settings
	uikit.SettingsRow(c, uikit.SettingsRowProps{Gap: t.Space.N12}).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N4).Children(func() {
			ui.Text(c, localize(c, label)).FontSize(p.RowLabelSize).TextColor(uikit.Color(p.RowLabel))
			ui.Text(c, localize(c, description)).FontSize(p.RowDescSize).TextColor(uikit.Color(p.RowDesc))
		})
		control()
	})
}

func (a *app) managedKeyMenu(c *ui.Context, m *ui.Menu, key managedKey) {
	if m.Item(localize(c, "重命名…")).Chosen() {
		a.credentials.draft = &credentialDraft{kind: "rename", keyID: key.ID, name: key.Name}
	}
	if m.Item(localize(c, "复制公钥")).Chosen() {
		a.copyManagedPublicKey(key)
	}
	if m.Item(localize(c, "在文件管理器中显示")).Chosen() {
		mygo.Shell.ShowItemInFolder(key.Path)
	}
	m.Separator()
	used := a.keyUseCount(key.ID)
	label := "删除…"
	if used > 0 {
		label = fmt.Sprintf(localize(c, "删除（被 %d 个会话使用）"), used)
	}
	if m.Item(localize(c, label)).Disabled(used > 0).Chosen() {
		a.credentials.draft = &credentialDraft{kind: "delete", keyID: key.ID, name: key.Name}
	}
}

func (a *app) credentialDialogs(c *ui.Context) {
	d := a.credentials.draft
	if d == nil {
		return
	}
	t := theme.Current()
	p := t.C.Modal
	open := true
	uikit.Modal(c.Key("credential-dialog"), &open, uikit.ModalProps{Width: p.GenerateWidth}, func(backdrop ui.Element, panel ui.Element, _ uikit.ModalBounds) {
		uikit.StyleModalScrim(backdrop, 1)
		panel.Padding(p.HeaderPaddingTop, p.BodyPaddingX, p.FooterPaddingBottom).Gap(t.Space.N14)
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
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, localize(c, title)).Grow(1).FontSize(p.TitleSize).FontWeight(p.TitleWeight)
			uikit.IconButton(c, closeIcon, uikit.IconButtonProps{Label: localize(c, "关闭密钥窗口")}).Disabled(a.credentials.busy).OnClick(func() { a.dismissCredentialDraft() })
		})
		if d.kind == "delete" {
			ui.Text(c, localize(c, fmt.Sprintf(localize(c, "确认将「%s」的私钥移到系统废纸篓？"), d.name))).FontSize(t.Font.Size.Body)
		} else if d.kind == "clear-passwords" {
			ui.Text(c, localize(c, "清除系统钥匙串中的全部会话密码。下次连接时需要重新输入。")).FontSize(t.Font.Size.Body)
		} else {
			credentialField(c, "名称", func() {
				uikit.Input(c, &d.name, uikit.InputProps{}).Label(localize(c, "密钥名称")).AutoFocus().FillWidth()
			})
			if d.kind == "generate" {
				credentialField(c, "类型", func() {
					uikit.SelectNative(c, "", &d.keyType, []string{"Ed25519", "RSA 4096"}).Label(localize(c, "密钥类型"))
				})
				credentialField(c, "注释", func() { uikit.Input(c, &d.comment, uikit.InputProps{}).Label(localize(c, "密钥注释")).FillWidth() })
			}
			if d.kind == "generate" || d.kind == "import" {
				credentialField(c, "口令", func() {
					uikit.Input(c, &d.passphrase, uikit.InputProps{}).Password().Label(localize(c, "私钥口令")).FillWidth()
				})
				ui.Checkbox(c, &d.remember, localize(c, "将口令保存在系统钥匙串"))
			}
		}
		if d.errorText != "" {
			ui.Text(c, localize(c, d.errorText)).FontSize(t.Font.Size.Footnote).TextColor(uikit.Color(t.Color.Danger.Text))
		}
		if a.credentials.busy {
			ui.Text(c, localize(c, "正在处理…")).FontSize(t.Font.Size.Footnote).TextColor(uikit.Color(t.Color.Text.Secondary))
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(p.FooterGap).Children(func() {
			uikit.Button(c, localize(c, "取消"), uikit.ButtonProps{}).Disabled(a.credentials.busy).OnClick(a.dismissCredentialDraft)
			actionButton := uikit.Button(c, localize(c, action), uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Disabled(a.credentials.busy)
			if d.kind == "delete" || d.kind == "clear-passwords" {
				actionButton.Background(uikit.Color(t.Color.Danger.Default))
			}
			actionButton.OnClick(func() {
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
			})
		})
	})
	if !open && !a.credentials.busy {
		a.dismissCredentialDraft()
	}
}

func credentialField(c *ui.Context, label string, control func()) {
	parts := uikit.FormRow(c, localize(c, label), uikit.FormRowProps{Inline: true})
	parts.Body.Children(control)
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
	a.keys = slices.Delete(a.keys, index, index+1)
	if err := a.saveCatalog(); err != nil {
		a.keys = previous
		d.errorText = "保存凭据目录：" + err.Error()
		return
	}
	a.credentialWork(func(_ context.Context) credentialResult {
		if err := mygo.Secrets.Delete("key/" + key.ID + "/passphrase"); err != nil {
			return credentialResult{restoreKey: &key, restoreIndex: index, err: fmt.Errorf("删除保存的口令：%w", err)}
		}
		if err := mygo.Shell.TrashItem(key.Path); err != nil {
			return credentialResult{restoreKey: &key, restoreIndex: index, err: fmt.Errorf("移动私钥到废纸篓：%w", err)}
		}
		if _, err := os.Stat(key.Path + ".pub"); err == nil {
			if err := mygo.Shell.TrashItem(key.Path + ".pub"); err != nil {
				return credentialResult{deletedID: key.ID, err: fmt.Errorf("私钥已移到废纸篓，公钥移动失败：%w", err)}
			}
		} else if !os.IsNotExist(err) {
			return credentialResult{deletedID: key.ID, err: err}
		}
		return credentialResult{deletedID: key.ID, notice: "密钥已移到废纸篓"}
	})
}

func (a *app) clearCredentialPasswords() {
	var ids []string
	for _, h := range a.hosts {
		if h.session.PasswordSaved {
			ids = append(ids, h.session.ID)
		}
	}
	a.credentialWork(func(ctx context.Context) credentialResult {
		result := credentialResult{notice: "已清除所有保存的会话密码"}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				result.err = err
				return result
			}
			if err := mygo.Secrets.Delete("session/" + id + "/password"); err != nil {
				result.err = fmt.Errorf("清除保存的密码：%w", err)
				return result
			}
			result.clearedIDs = append(result.clearedIDs, id)
		}
		return result
	})
}
