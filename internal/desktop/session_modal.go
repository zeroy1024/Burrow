package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	sessionKeyIcon   = lineIcon(`<circle cx="8" cy="8" r="5"/><path d="m12 12 9 9m-6-6 3-3m0 6 3-3"/>`)
	sessionEyeIcon   = lineIcon(`<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>`)
	sessionAlertIcon = lineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/>`)
	sessionCheckIcon = lineIcon(`<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>`)
)

func (a *app) hostDialog(c *ui.Context) {
	page := a.page()
	d := &page.draft
	if d.deleting {
		uikit.Dialog(c, &page.addHostOpen, func() {
			ui.Column(c).Width(theme.Current().C.Dialog.HostWidth).Gap(theme.Current().Space.N14).Children(func() { a.deleteSiteDialogContents(c) })
		})
		return
	}
	if d.errors == nil {
		d.initState()
	}
	if d.agentResults != nil {
		select {
		case result := <-d.agentResults:
			d.agentCount, d.agentError, d.agentResults = result.count, result.err, nil
		default:
			c.After(theme.Current().C.TestResult.Crossfade)
		}
	}
	if d.testResults != nil {
		select {
		case result := <-d.testResults:
			d.cancelTest()
			d.testResult = &result
			d.testVisible = true
			c.Announce(result.Summary)
		default:
			c.After(theme.Current().C.TestResult.Crossfade)
		}
	}
	t := theme.Current()
	p := t.C.Modal
	d.submitted = false
	dialog := uikit.Modal(c, &page.addHostOpen, uikit.ModalProps{Label: a.sessionDraftTitle(d), Disabled: d.closing || d.saving}, func(backdrop, panel ui.Element, bounds uikit.ModalBounds) {
		alphaTarget, scaleTarget := float32(1), float32(1)
		duration := p.Fade
		animation := ui.Animation{Spring: uikit.Spring(p.Enter)}
		if !d.entered {
			alphaTarget, scaleTarget = 0, p.EnterScale
			d.entered = true
			c.AnimationFrame()
		}
		if d.closing {
			alphaTarget, scaleTarget, duration = 0, p.ExitScale, p.Exit
			animation = ui.Animation{Duration: p.Exit, Ease: uikit.Easing(t.Motion.Easing.In)}
			if d.closeStarted.IsZero() {
				d.closeStarted = c.Now()
			}
			if c.Now().Sub(d.closeStarted) >= p.Exit {
				page.addHostOpen = false
				if d.afterClose != nil {
					open := d.afterClose
					d.afterClose = nil
					open()
				}
				c.AnimationFrame()
				return
			}
			c.AnimationFrame()
		}
		fadeAnimation := uikit.Tween(duration)
		if d.closing {
			fadeAnimation.Ease = uikit.Easing(t.Motion.Easing.In)
		}
		alphaColor := panel.AnimateColor("modal-alpha", ui.Color{A: uint8(alphaTarget * 255)}, fadeAnimation)
		alpha := float32(alphaColor.A) / 255
		scale := panel.AnimateTo("modal-scale", scaleTarget, animation)
		if a.reduceMotion(c) {
			scale = 1
		}
		panel.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Duration: t.Motion.Duration.InstantNonzero}).Opacity(alpha)
		uikit.StyleModalScrim(backdrop, alpha)
		a.sessionDraftHeader(c, d)
		noticeHeight := float32(0)
		if count := a.draftOpenTabs(d); count > 0 {
			notice := ui.Row(c).Shrink(0).Margin(0, p.BodyPaddingX, t.Space.N12, p.BodyPaddingX).Padding(t.Space.N8, t.Space.N10).
				Radius(t.Radius.Control).Background(uikit.Color(t.Color.Surface.Sunken)).Gap(t.Space.N6).Children(func() {
				ui.Icon(c, infoIcon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(uikit.Color(t.Color.Text.Secondary))
				ui.Text(c, fmt.Sprintf("此会话已在 %d 个选项卡中打开，修改将在下次连接时生效。", count)).
					FontSize(t.Font.Size.Small).TextColor(uikit.Color(t.Color.Text.Muted)).Grow(1).MinWidth(0)
			})
			noticeHeight = notice.LayoutBounds().H + t.Space.N12
		}
		headerHeight := p.HeaderPaddingTop + p.IconTile + p.HeaderPaddingBottom
		footerHeight := p.FooterPaddingTop + t.C.Button.Height + p.FooterPaddingBottom
		ui.Scroll(c).Key("session-form-scroll").FillWidth().MinHeight(0).Shrink(1).
			MaxHeight(bounds.MaxHeight - headerHeight - footerHeight - noticeHeight).TrackScroll(&d.scroll).Children(func() {
			if d.files {
				a.sessionProtocolPicker(c, d)
			}
			ui.Column(c).FillWidth().Padding(t.Space.N8, p.BodyPaddingX, t.Space.N16, p.BodyPaddingX).
				Gap(t.C.FormRow.RowGap).Children(func() { a.sessionDraftFields(c, d) })
			a.sessionAdvanced(c, d)
		})
		a.sessionDraftFooter(c, d)
		// Built last so that Tab reaches the close button after the footer.
		uikit.IconButton(c, closeIcon, uikit.IconButtonProps{Key: "session-close", Label: "关闭会话设置"}).Absolute().Top(p.HeaderPaddingTop).Right(p.HeaderPaddingRight).
			OnClick(func() { a.closeSessionDraft(d) })
		if !d.closing && !d.saving {
			if panel.OverlayShortcut(ui.Super, ui.KeyS) {
				a.saveSessionDraft(c, d, false)
			}
			if panel.OverlayShortcut(ui.Super, ui.KeyT) {
				a.startDraftTest(c, d)
			}
			if d.submitted || panel.OverlayShortcut(0, ui.KeyEnter) {
				a.saveSessionDraft(c, d, true)
			}
		}
	})
	if dialog.Dismissed() {
		page.addHostOpen = true
		if !d.saving {
			a.closeSessionDraft(d)
		}
	}
}

func (a *app) sessionDraftTitle(d *hostDraft) string {
	if d.editing {
		return "编辑会话"
	}
	if d.files {
		return "新建 FTP/SFTP 会话"
	}
	return "新建 SSH 会话"
}

func (a *app) sessionDraftHeader(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Modal
	ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
		Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(p.IconTile, p.IconTile).Shrink(0).Radius(p.IconTileRadius).Background(uikit.Color(t.Color.Surface.Control)).Center().Children(func() {
			if d.files {
				protocol := theme.Current().C.ProtocolIcon
				icon, fg, bg := folderLockIcon, protocol.SftpGlyph, protocol.SftpBg
				if d.protocol == "FTP" {
					icon, fg, bg = folderIcon, protocol.FtpGlyph, protocol.FtpBg
				}
				ui.Box(c).Key("protocol-header-" + d.protocol).Fill().Radius(p.IconTileRadius).Background(uikit.Color(bg)).Center().
					Transition(ui.ElementTransition{Duration: p.Fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).
					Children(func() { ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(uikit.Color(fg)) })
			} else if d.editing && a.hosts[d.id].system != "" {
				h := a.hosts[d.id]
				style := systems[h.os]
				ui.Box(c).Fill().Radius(p.IconTileRadius).Background(uikit.Color(t.OSLogoBg(style.color))).Center().Children(func() {
					ui.Image(c, style.icon).Size(t.C.OsLogo.GlyphSize, t.C.OsLogo.GlyphSize).TextColor(uikit.Color(style.color))
				})
			} else {
				ui.Icon(c, terminalIcon).Size(t.Size.Icon, t.Size.Icon).TextColor(uikit.Color(t.Color.Text.Strong))
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
			ui.Text(c, a.sessionDraftTitle(d)).FontSize(p.TitleSize).FontWeight(p.TitleWeight).SingleLine()
			subtitle := "终端会话 · 保存后出现在左侧「会话」"
			if d.files {
				subtitle = "文件传输会话 · 保存后出现在左侧「站点」"
			}
			if d.editing {
				subtitle = a.hosts[d.id].name + " · " + d.protocol
			}
			ui.Text(c, subtitle).FontSize(p.SubtitleSize).TextColor(uikit.Color(p.Subtitle)).SingleLine()
		})
	})
}

func (a *app) sessionProtocolPicker(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Segmented
	selected := 0
	if d.protocol == "FTP" {
		selected = 1
	}
	before := selected
	ui.Column(c).FillWidth().Shrink(0).Padding(0, t.C.Modal.BodyPaddingX, t.Space.N8, t.C.Modal.BodyPaddingX).Children(func() {
		segments := ui.SegmentedBase(c.Key("file-protocol"), &selected, 2)
		segments.Track.FillWidth().Padding(t.C.Modal.ProtocolPadding).Radius(t.C.Modal.ProtocolRadius).Background(uikit.Color(p.Bg)).Label("协议").Children(func() {
			for index, protocol := range []string{"SFTP", "FTP"} {
				segment := segments.Segment(index).Grow(1).MinWidth(0).Padding(t.Space.N8, p.ItemPaddingX).
					Gap(t.Space.N8).Radius(p.ItemRadius).Label(protocol)
				uikit.StyleSegment(segment, p, index == selected)
				segment.Children(func() {
					icon, fg := folderLockIcon, t.C.ProtocolIcon.SftpGlyph
					if index == 1 {
						icon, fg = folderIcon, t.C.ProtocolIcon.FtpGlyph
					}
					ui.Icon(c, icon).Size(t.C.Modal.ProtocolIcon, t.C.Modal.ProtocolIcon).TextColor(uikit.Color(fg)).PassThrough()
					ui.Column(c).Gap(t.Space.N2).PassThrough().Children(func() {
						ui.Text(c, protocol).FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Semibold).PassThrough()
						description := "基于 SSH，加密传输"
						if index == 1 {
							description = "经典 FTP，可选 TLS"
						}
						ui.Text(c, description).FontSize(t.Font.Size.Footnote).TextColor(uikit.Color(t.Color.Text.Secondary)).PassThrough()
					})
				})
			}
		})
	})
	if before != selected {
		a.setModalAdvancedOpen(strings.ToLower(d.protocol), d.advanced)
		d.changeProtocol([]string{"SFTP", "FTP"}[selected])
		d.advanced = a.modalAdvancedOpen(strings.ToLower(d.protocol))
	}
}

func (a *app) sessionField(c *ui.Context, d *hostDraft, key, label, description string, content func()) {
	parts := uikit.FormRow(c, label, uikit.FormRowProps{Key: key})
	parts.Label.OnClick(func() {
		if h := d.handles[key]; h != nil {
			h.Focus()
		}
	})
	parts.Body.Children(func() {
		content()
		if description != "" {
			uikit.FieldMessage(c, description, uikit.FieldMessageProps{})
		}
		message := d.errors[key]
		if key == "host" && message == "" {
			message = d.errors["port"]
		}
		if message != "" {
			uikit.FieldMessage(c, message, uikit.FieldMessageProps{Error: true, Icon: sessionAlertIcon})
		}
	})
}

func (a *app) sessionInput(c *ui.Context, d *hostDraft, key, label string, value *string, mono bool) ui.Element {
	if d.handles[key] == nil {
		d.handles[key] = &ui.Handle{}
	}
	e := uikit.Input(c.Key(key), value, uikit.InputProps{Label: label, Handle: d.handles[key], Mono: mono, DeferStyle: true}).FillWidth().MinWidth(0)
	return a.sessionInputState(d, key, value, e)
}

func (a *app) sessionInputState(d *hostDraft, key string, value *string, e ui.Element) ui.Element {
	if key == "name" {
		e.AutoFocus()
		if d.editing && !d.selectedName {
			e.SetTextSelection(0, utf8.RuneCountInString(*value))
			d.selectedName = true
		}
	}
	if e.Changed() {
		d.changed(key)
		if key == "password" {
			d.passwordChanged = true
		}
		if key == "passphrase" {
			d.passphraseChanged = true
		}
		if key == "port" {
			d.portEdited = true
			if strings.IndexFunc(*value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				d.errors[key] = "端口需在 1–65535 之间"
			}
		}
	}
	if d.focused[key] && !e.Focused() {
		if key == "port" && *value == "" {
			d.portEdited = false
			*value = strconv.Itoa(d.defaultPort())
		}
		d.errors[key] = a.draftError(d, key)
	}
	d.focused[key] = e.Focused()
	uikit.StyleFormControl(e, d.errors[key])
	if e.Submitted() {
		d.submitted = true
	}
	return e
}

func (a *app) sessionDraftFields(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	a.sessionField(c, d, "name", "名称", "", func() {
		a.sessionInput(c, d, "name", "名称", &d.name, false).Placeholder("可选，默认用主机名")
	})
	a.sessionField(c, d, "host", "主机", "", func() {
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			a.sessionInput(c, d, "host", "主机", &d.address, true).Grow(1).Placeholder("主机名或 IP")
			ui.Text(c, "端口").Shrink(0).FontSize(t.C.FormRow.LabelSize).TextColor(uikit.Color(t.C.FormRow.Label))
			a.sessionPortInput(c, d)
		})
	})
	a.sessionField(c, d, "user", "用户名", "", func() {
		if d.auth == "anonymous" {
			anonymous := "anonymous"
			a.sessionInput(c, d, "anonymous", "用户名", &anonymous, true).Disabled(true).Background(uikit.Color(t.C.Input.DisabledBg)).TextColor(uikit.Color(t.C.Input.DisabledText)).Border(t.C.Input.BorderWidth, uikit.Color(t.C.Input.DisabledBorder))
		} else {
			a.sessionInput(c, d, "user", "用户名", &d.user, true).Placeholder("root")
		}
	})
	a.sessionField(c, d, "auth", "认证方式", "", func() {
		values, labels := []string{"password", "key", "agent"}, []string{"密码", "私钥", "Agent"}
		if d.protocol == "FTP" {
			values, labels = []string{"password", "anonymous"}, []string{"密码", "匿名"}
		}
		index := slices.Index(values, d.auth)
		if uikit.Segmented(c.Key("auth"), &index, labels, uikit.SegmentedProps{Form: true}).Changed() {
			d.auth = values[index]
			d.changed("auth")
		}
	})
	transition := a.sessionCollapseTransition(c)
	if d.auth != "anonymous" {
		ui.Column(c).Key("auth-" + d.auth).FillWidth().Gap(t.C.FormRow.RowGap).Transition(transition).Children(func() {
			switch d.auth {
			case "password":
				a.sessionPasswordField(c, d, "password", "密码", "保存在系统钥匙串", &d.password, &d.showPassword)
			case "key":
				a.sessionField(c, d, "key", "私钥", "", func() {
					ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
						a.sessionKeySelect(c, d)
						uikit.Button(c, "管理…", uikit.ButtonProps{}).Height(t.Size.ControlSm).Shrink(0).OnClick(func() { d.cancelTest(); a.page().addHostOpen = false; a.openCredentialSettings() })
					})
				})
				a.sessionPasswordField(c, d, "passphrase", "口令", "", &d.passphrase, &d.showPassphrase)
			case "agent":
				if !d.listedAgent {
					results := make(chan sessionAgentResult, 1)
					d.agentResults = results
					go func() { count, err := agentState(); results <- sessionAgentResult{count: count, err: err} }()
					d.listedAgent = true
				}
				a.sessionField(c, d, "agent", "Agent", "", func() {
					if d.agentResults != nil {
						ui.Text(c, "正在检测 ssh-agent…").FontSize(t.Font.Size.Small).TextColor(uikit.Color(t.Color.Text.Secondary))
						return
					}
					color, icon, text := t.Color.Success.Text, sessionCheckIcon, fmt.Sprintf("已检测到 ssh-agent · %d 把密钥", d.agentCount)
					if d.agentError != nil {
						color, icon, text = t.Color.Warning.Text, sessionAlertIcon, "未检测到 ssh-agent（SSH_AUTH_SOCK）"
					}
					ui.Row(c).Height(t.C.Input.Height).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(uikit.Color(color))
						ui.Text(c, text).FontSize(t.Font.Size.Small).TextColor(uikit.Color(color))
					})
				})
			}
		})
	}

	a.sessionField(c, d, "group", "分组", "", func() {
		if d.creatingGroup {
			a.sessionInput(c, d, "group", "新建分组", &d.newGroup, false).Placeholder("新建分组名称")
			return
		}
		options := append(a.groupNames(), "新建分组…")
		previous := d.group
		if uikit.Select(c, "分组", &d.group, options).Changed() {
			if d.group == "新建分组…" {
				d.group = previous
				d.creatingGroup = true
			}
			d.changed("group")
		}
	})
}

func (a *app) sessionPortInput(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Input
	ui.Box(c).Key("port-field").Width(p.PortWidth).Height(p.Height).Shrink(0).Children(func() {
		input := a.sessionInput(c, d, "port", "端口", &d.port, true)
		if input.Focused() || d.portEdited {
			return
		}
		// TextInput keeps keyboard editing. Its unfocused text is painted by
		// Text so automatic protocol/TLS updates can crossfade independently.
		input.TextColor(ui.Transparent)
		animation := uikit.Tween(t.C.Modal.Fade)
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Padding(0, p.PaddingX).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Text(c, d.port).Role(ui.RoleNone).Font(strings.Join(p.MonoFont, ", ")).FontSize(p.MonoSize).LetterSpacing(p.MonoTracking).
				FixedLineHeight(p.LineHeight).TextColor(uikit.Color(p.Text)).SingleLine().PassThrough().ContentTransition(ui.ContentTransition{Animation: &animation})
		})
	})
}

func (a *app) sessionPasswordField(c *ui.Context, d *hostDraft, key, label, description string, value *string, shown *bool) {
	a.sessionField(c, d, key, label, description, func() {
		if d.handles[key] == nil {
			d.handles[key] = &ui.Handle{}
		}
		parts := uikit.PasswordInput(c, value, uikit.PasswordProps{
			Label: label, Handle: d.handles[key], Shown: *shown, EyeIcon: sessionEyeIcon, ToggleKey: "show-" + key, InputKey: key,
		})
		input := a.sessionInputState(d, key, value, parts.Input)
		if key == "password" && d.passwordSaved && !d.passwordChanged {
			input.Placeholder("●●●●●●●●●●")
		}
		if key == "passphrase" {
			input.Placeholder("私钥口令（无则留空）")
		}
		parts.Toggle.OnClick(func() { *shown = !*shown })
	})
}

func (a *app) sessionKeySelect(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Input
	previous := d.keyID
	parts := ui.SelectBase(c.Key("private-key"), &d.keyID)
	if d.handles["key"] == nil {
		d.handles["key"] = &ui.Handle{}
	}
	parts.Trigger.Bind(d.handles["key"])
	parts.Trigger.Height(p.Height).Grow(1).MinWidth(0).Padding(0, p.PaddingX).Radius(p.Radius).Gap(p.Gap).
		Background(uikit.Color(p.Bg)).Label("私钥").Children(func() {
		ui.Icon(c, sessionKeyIcon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(uikit.Color(p.Icon))
		keyIndex := slices.IndexFunc(a.keys, func(k managedKey) bool { return k.ID == d.keyID })
		if keyIndex < 0 {
			ui.Text(c, "请选择私钥").Grow(1).FontSize(p.FontSize).TextColor(uikit.Color(p.Placeholder))
		} else {
			key := a.keys[keyIndex]
			ui.Text(c, key.Name).FontSize(p.FontSize).FontWeight(t.Font.Weight.Medium).SingleLine()
			ui.Text(c, key.Type+" · "+abbreviateFingerprint(key.Fingerprint)).Grow(1).MinWidth(0).FontSize(t.Font.Size.Footnote).TextColor(uikit.Color(t.Color.Text.Secondary)).SingleLine()
		}
		ui.Icon(c, chevronIcon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(uikit.Color(p.Icon))
	})
	uikit.StyleFormControl(parts.Trigger, d.errors["key"])
	parts.Popup(func(panel ui.Element) {
		uikit.StyleSelectPopup(panel)
		for _, key := range a.keys {
			item := parts.Item(key.ID).Padding(t.Space.N6, t.C.Menu.ItemPaddingX).Radius(t.C.Menu.ItemRadius)
			if item.Highlighted() {
				item.Background(uikit.Color(t.Color.State.Hover))
			}
			item.Children(func() {
				ui.Text(c, key.Name).FontSize(t.Font.Size.Body)
				ui.Text(c, key.Type+" · "+abbreviateFingerprint(key.Fingerprint)).FontSize(t.Font.Size.Footnote).TextColor(uikit.Color(t.Color.Text.Secondary))
			})
		}
		parts.Item("__import__").Padding(t.Space.N6, t.C.Menu.ItemPaddingX).Children(func() { ui.Text(c, "导入私钥…").FontSize(t.Font.Size.Body) })
	})
	if previous != d.keyID {
		if d.keyID == "__import__" {
			d.keyID = previous
			a.choosePrivateKey()
		}
		d.changed("key")
	}
}

func abbreviateFingerprint(value string) string {
	if len(value) <= 24 {
		return value
	}
	return value[:16] + "…" + value[len(value)-4:]
}

func (a *app) sessionCollapseTransition(c *ui.Context) ui.ElementTransition {
	if a.reduceMotion(c) {
		return ui.ElementTransition{Duration: theme.Current().C.Collapse.Duration, Enter: &ui.Motion{}, Exit: &ui.Motion{}}
	}
	return ui.ElementTransition{Duration: theme.Current().C.Collapse.Duration, Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}}
}

func (a *app) sessionDraftFooter(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Modal
	ui.Row(c).FillWidth().Shrink(0).Padding(p.FooterPaddingTop, p.FooterPaddingRight, p.FooterPaddingBottom, p.FooterPaddingLeft).
		BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(uikit.Color(p.Divider)).Gap(p.FooterGap).AlignItems(ui.Center).Children(func() {
		disabled := strings.TrimSpace(d.address) == ""
		test := uikit.Button(c, "测试连接", uikit.ButtonProps{}).Padding(0, p.ButtonPaddingX).Shrink(0).Disabled(disabled || d.testResults != nil).OnClick(func() { a.startDraftTest(c, d) })
		if disabled || d.testResults != nil {
			test.Opacity(t.Opacity.Disabled)
		}
		a.sessionTestResult(c, d)
		ui.Spacer(c)
		uikit.Button(c, "取消", uikit.ButtonProps{Variant: uikit.ButtonPlain}).Padding(0, p.ButtonPaddingX).OnClick(func() { a.closeSessionDraft(d) })
		label := "仅保存"
		if d.editing {
			label = "保存"
		}
		save := uikit.Button(c, label, uikit.ButtonProps{}).Padding(0, p.ButtonPaddingX).Shrink(0).Disabled(disabled).OnClick(func() { a.saveSessionDraft(c, d, false) })
		connect := uikit.Button(c, "保存并连接  ↩", uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Padding(0, p.ButtonPaddingX).Shrink(0).Disabled(disabled).OnClick(func() { a.saveSessionDraft(c, d, true) })
		if disabled {
			save.Opacity(t.Opacity.Disabled)
			connect.Opacity(t.Opacity.Disabled)
		}
	})
}

func (a *app) sessionTestResult(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.TestResult
	if d.testResult == nil && d.testResults == nil {
		return
	}
	row := ui.Row(c).Key("test-result").MinWidth(0).Shrink(1).Gap(p.Gap).AlignItems(ui.Center).
		Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}})
	shown := float32(0)
	if d.testVisible {
		shown = 1
	}
	alpha := uikit.Visual(row, "result-alpha", shown, p.Crossfade)
	row.Opacity(alpha).Hidden(alpha == 0 && !d.testVisible)
	row.Children(func() {
		stage := "testing"
		if d.testResults == nil {
			stage = "success"
			if d.testResult.Err != nil {
				stage = "failure"
			}
		}
		ui.Row(c).Key(stage).MinWidth(0).Gap(p.Gap).AlignItems(ui.Center).
			Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
			if d.testResults != nil {
				ui.Spinner(c).Size(p.SpinnerSize, p.SpinnerSize).Shrink(0).Label("正在连接").TextColor(uikit.Color(p.Testing))
				ui.Text(c, "正在连接…").FontSize(p.FontSize).TextColor(uikit.Color(p.Testing)).MinWidth(0).SingleLine()
				return
			}
			result := d.testResult
			icon, color, glyph := sessionCheckIcon, p.Success, p.SuccessIcon
			if result.Err != nil {
				icon, color, glyph = sessionAlertIcon, p.Fail, p.FailIcon
			}
			ui.Icon(c, icon).Size(p.Icon, p.Icon).Shrink(0).TextColor(uikit.Color(glyph))
			label := ui.Text(c, result.Summary).FontSize(p.FontSize).TextColor(uikit.Color(color)).MinWidth(0).SingleLine()
			if result.Detail != "" {
				sessionResultTooltip(c, label, result.Detail)
			}
			if result.Err != nil {
				sessionResultTooltip(c, ui.Icon(c, infoIcon).Size(p.InfoSize, p.InfoSize).Shrink(0).TextColor(uikit.Color(p.Info)), result.Detail)
			}
		})
	})
}

func sessionResultTooltip(c *ui.Context, anchor ui.Element, detail string) {
	t := theme.Current()
	lines := strings.Split(detail, "\n")
	if len(lines) > 3 {
		lines = append(lines[:2], lines[len(lines)-1])
	}
	text := strings.Join(lines, "\n")
	anchor.Description(text)
	ui.TooltipBase(c, anchor, func(panel ui.Element) {
		uikit.StyleTooltip(panel).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, t.C.Tooltip.Offset, 0).
			Children(func() { ui.Text(c, text).Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(t.Font.Size.Footnote) })
	})
}

func (a *app) chooseSessionDirectory(d *hostDraft) {
	if d.choosingDirectory {
		return
	}
	d.choosingDirectory = true
	generation := d.generation
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "选择本地目录", Directory: true})
		mygo.RunOnMain(func() {
			if d.generation != generation {
				return
			}
			d.choosingDirectory = false
			if err != nil {
				a.terminalError = fmt.Errorf("选择本地目录：%w", err)
			} else if len(paths) > 0 {
				d.localDir = paths[0]
				d.changed("localDir")
			}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	}()
}
