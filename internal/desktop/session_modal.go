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
	sessionKeyIcon   = uikit.LineIcon(`<circle cx="8" cy="8" r="5"/><path d="m12 12 9 9m-6-6 3-3m0 6 3-3"/>`)
	sessionEyeIcon   = uikit.LineIcon(`<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>`)
	sessionAlertIcon = uikit.LineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/>`)
	sessionCheckIcon = uikit.LineIcon(`<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>`)
)

func (a *app) hostDialog(c *ui.Context) {
	page := a.page()
	d := &page.draft
	if d.deleting {
		requested := false
		result := uikit.Modal(c.Key("delete-site-dialog"), &page.addHostOpen, uikit.ModalProps{
			Title: "删除会话「" + a.hosts[d.id].name + "」？", Compact: true,
			Width: theme.Current().C.Dialog.HostWidth, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		}, func(parts uikit.ModalParts) {
			requested = a.deleteSiteDialogContents(c, parts)
		})
		if requested || result.Submit {
			a.confirmDeleteSite(c, d.id)
		}
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
		default:
			c.After(theme.Current().C.TestResult.Crossfade)
		}
	}
	t := theme.Current()
	p := t.C.Modal
	d.submitted = false
	dialog := uikit.Modal(c.Key(d.generation), &page.addHostOpen, uikit.ModalProps{
		Title: a.sessionDraftTitle(d), Disabled: d.closing || d.saving, Dirty: d.dirty(),
		ExternalLifecycle: true, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		CloseKey: "session-close", CloseLabel: "关闭会话设置",
		Header: func(uikit.ModalParts) { a.sessionDraftHeader(c, d) },
	}, func(parts uikit.ModalParts) {
		backdrop, panel, bounds := parts.Backdrop, parts.Panel, parts.Bounds
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
				panel.Hidden(true)
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
		opacity := alpha
		if d.closing || d.saving {
			opacity *= t.Opacity.Disabled / 0.5
		}
		panel.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Duration: t.Motion.Duration.InstantNonzero}).Opacity(opacity)
		uikit.StyleModalScrim(backdrop, alpha)
		noticeHeight := float32(0)
		if count := a.draftOpenTabs(d); count > 0 {
			// Banner fills its parent's content width. Inset that content with
			// padding rather than adding margins outside an already full width.
			notice := ui.Box(c).FillWidth().MinWidth(0).Shrink(0).
				Padding(0, p.BodyPaddingX, t.Space.N12, p.BodyPaddingX).Children(func() {
				uikit.Banner(c, fmt.Sprintf("此会话已在 %d 个选项卡中打开，修改将在下次连接时生效。", count), uikit.BannerProps{Icon: infoIcon})
			})
			noticeHeight = notice.LayoutBounds().H
		}
		headerHeight := p.HeaderPaddingTop + p.IconTile + p.HeaderPaddingBottom
		footerHeight := p.FooterPaddingTop + t.C.Button.Height + p.FooterPaddingBottom
		formScroll := ui.Scroll(c).Key("session-form-scroll").FillWidth().MinHeight(0).Shrink(1).
			MaxHeight(bounds.MaxHeight - headerHeight - footerHeight - noticeHeight).TrackScroll(&d.scroll).Children(func() {
			if d.files {
				a.sessionProtocolPicker(c, d)
			}
			uikit.FormSection(c, "", func() { a.sessionDraftFields(c, d, parts.RequestDismiss) }).
				Padding(t.Space.N8, p.BodyPaddingX, t.Space.N16, p.BodyPaddingX)
			a.sessionAdvanced(c, d)
		})
		a.sessionDraftFooter(c, d, parts.RequestDismiss)
		uikit.Scrollbars(c, panel, formScroll, &d.scroll, uikit.ScrollbarProps{Label: "会话表单滚动条"})
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
	if dialog.DismissCanceled {
		d.afterClose = nil
	}
	if dialog.Submit && !d.saving && !d.closing {
		a.saveSessionDraft(c, d, true)
	}
	if dialog.Dismissed {
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
					Children(func() { uikit.Icon(c, icon, uikit.IconProps{}).TextColor(uikit.Color(fg)) })
			} else if d.editing && a.hosts[d.id].system != "" {
				h := a.hosts[d.id]
				style := systems[h.os]
				ui.Box(c).Fill().Radius(p.IconTileRadius).Background(uikit.Color(t.OSLogoBg(style.color))).Center().Children(func() {
					ui.Image(c, style.icon).Size(t.C.OsLogo.GlyphSize, t.C.OsLogo.GlyphSize).TextColor(uikit.Color(style.color))
				})
			} else {
				uikit.Icon(c, terminalIcon, uikit.IconProps{}).TextColor(uikit.Color(t.Color.Text.Strong))
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
			uikit.Text(c, a.sessionDraftTitle(d), uikit.TextProps{Style: uikit.TextTitle}).SingleLine()
			subtitle := "终端会话 · 保存后出现在左侧「会话」"
			if d.files {
				subtitle = "文件传输会话 · 保存后出现在左侧「站点」"
			}
			if d.editing {
				subtitle = a.hosts[d.id].name + " · " + d.protocol
			}
			uikit.Text(c, subtitle, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine()
		})
	})
}

func (a *app) sessionProtocolPicker(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	selected := 0
	if d.protocol == "FTP" {
		selected = 1
	}
	before := selected
	ui.Column(c).FillWidth().Shrink(0).Padding(0, t.C.Modal.BodyPaddingX, t.Space.N8, t.C.Modal.BodyPaddingX).Children(func() {
		uikit.Segmented(c.Key("file-protocol"), &selected, []uikit.SegItem{
			{Label: "SFTP", IconSVG: folderLockIcon, IconTone: uikit.ToneAccent, Detail: "基于 SSH，加密传输"},
			{Label: "FTP", IconSVG: folderIcon, IconTone: uikit.ToneTertiary, Detail: "经典 FTP，可选 TLS"},
		}, uikit.SegmentedProps{Label: "协议", Fill: true, Cards: true})
	})
	if before != selected {
		a.setModalAdvancedOpen(strings.ToLower(d.protocol), d.advanced)
		d.changeProtocol([]string{"SFTP", "FTP"}[selected])
		d.advanced = a.modalAdvancedOpen(strings.ToLower(d.protocol))
	}
}

func (a *app) sessionField(c *ui.Context, d *hostDraft, key, label, description string, content func() ui.Element) {
	parts := uikit.FormRow(c, label, uikit.FormRowProps{Key: key}, content)
	parts.Body.Children(func() {
		if description != "" {
			uikit.FieldMessage(c, description, uikit.FieldMessageProps{})
		}
		message := d.errors[key]
		if key == "host" && message == "" {
			message = d.errors["port"]
		}
		if message != "" {
			uikit.FieldMessage(c, message, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
		}
	})
}

func (a *app) sessionInput(c *ui.Context, d *hostDraft, key, label string, value *string, mono bool) ui.Element {
	if d.handles[key] == nil {
		d.handles[key] = &ui.Handle{}
	}
	props := uikit.InputProps{Label: label, Handle: d.handles[key], Mono: mono, DeferStyle: true, Disabled: key == "anonymous"}
	var e ui.Element
	if key == "port" {
		e = uikit.PortInput(c.Key(key), value, props)
	} else {
		e = uikit.Input(c.Key(key), value, props)
	}
	e.FillWidth().MinWidth(0)
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
			if *value != "" {
				d.errors[key] = a.draftError(d, key)
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

func (a *app) sessionDraftFields(c *ui.Context, d *hostDraft, dismiss func()) {
	t := theme.Current()
	a.sessionField(c, d, "name", "名称", "", func() ui.Element {
		return a.sessionInput(c, d, "name", "名称", &d.name, false).Placeholder("可选，默认用主机名")
	})
	a.sessionField(c, d, "host", "主机", "", func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input = a.sessionInput(c, d, "host", "主机", &d.address, true).Grow(1).Placeholder("主机名或 IP")
			ui.Text(c, "端口").Shrink(0).FontSize(t.C.FormRow.LabelSize).TextColor(uikit.Color(t.C.FormRow.Label))
			a.sessionPortInput(c, d)
		})
		return input
	})
	a.sessionField(c, d, "user", "用户名", "", func() ui.Element {
		if d.auth == "anonymous" {
			anonymous := "anonymous"
			return a.sessionInput(c, d, "anonymous", "用户名", &anonymous, true)
		} else {
			return a.sessionInput(c, d, "user", "用户名", &d.user, true).Placeholder("root")
		}
	})
	a.sessionField(c, d, "auth", "认证方式", "", func() ui.Element {
		values, labels := []string{"password", "key", "agent"}, []string{"密码", "私钥", "Agent"}
		if d.protocol == "FTP" {
			values, labels = []string{"password", "anonymous"}, []string{"密码", "匿名"}
		}
		index := slices.Index(values, d.auth)
		input := uikit.Segmented(c.Key("auth"), &index, uikit.SegmentItems(labels), uikit.SegmentedProps{Label: "认证方式"})
		if input.Changed() {
			d.auth = values[index]
			d.changed("auth")
		}
		return input
	})
	transition := a.sessionCollapseTransition(c)
	if d.auth != "anonymous" {
		ui.Column(c).Key("auth-" + d.auth).FillWidth().Gap(t.C.FormRow.RowGap).Transition(transition).Children(func() {
			switch d.auth {
			case "password":
				a.sessionPasswordField(c, d, "password", "密码", "保存在系统钥匙串", &d.password)
			case "key":
				a.sessionField(c, d, "key", "私钥", "", func() ui.Element {
					var input ui.Element
					ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
						input = a.sessionKeySelect(c, d)
						uikit.Button(c, "管理…", uikit.ButtonProps{Size: uikit.SizeSm}).Shrink(0).OnClick(func() { d.afterClose = a.openCredentialSettings; dismiss() })
					})
					return input
				})
				a.sessionPasswordField(c, d, "passphrase", "口令", "", &d.passphrase)
			case "agent":
				if !d.listedAgent {
					results := make(chan sessionAgentResult, 1)
					d.agentResults = results
					go func() { count, err := agentState(); results <- sessionAgentResult{count: count, err: err} }()
					d.listedAgent = true
				}
				a.sessionField(c, d, "agent", "Agent", "", func() ui.Element {
					if d.agentResults != nil {
						return uikit.Text(c, "正在检测 ssh-agent…", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
					}
					color, icon, text := t.Color.Success.Text, sessionCheckIcon, fmt.Sprintf("已检测到 ssh-agent · %d 把密钥", d.agentCount)
					if d.agentError != nil {
						color, icon, text = t.Color.Warning.Text, sessionAlertIcon, "未检测到 ssh-agent（SSH_AUTH_SOCK）"
					}
					return ui.Row(c).Height(t.C.Input.Height).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
						uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeSm}).TextColor(uikit.Color(color))
						uikit.Text(c, text, uikit.TextProps{Style: uikit.TextSmall}).TextColor(uikit.Color(color))
					})
				})
			}
		})
	}

	a.sessionField(c, d, "group", "分组", "", func() ui.Element {
		if d.creatingGroup {
			return a.sessionInput(c, d, "group", "新建分组", &d.newGroup, false).Placeholder("新建分组名称")
		}
		options := append(a.groupNames(), "新建分组…")
		previous := d.group
		input := uikit.Select(c, &d.group, uikit.StringOptions(options), uikit.SelectProps{Label: "分组"})
		if input.Changed() {
			if d.group == "新建分组…" {
				d.group = previous
				d.creatingGroup = true
			}
			d.changed("group")
		}
		return input
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
		// The editor's content starts after both its border and padding.
		// This overlay is a sibling, so include that border in its own inset.
		inset := p.PaddingX + p.BorderWidth
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Padding(0, inset).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Text(c, d.port).Role(ui.RoleNone).Font(strings.Join(p.MonoFont, ", ")).FontSize(p.MonoSize).LetterSpacing(p.MonoTracking).
				FixedLineHeight(p.LineHeight).TextColor(uikit.Color(p.Text)).SingleLine().PassThrough().ContentTransition(ui.ContentTransition{Animation: &animation})
		})
	})
}

func (a *app) sessionPasswordField(c *ui.Context, d *hostDraft, key, label, description string, value *string) {
	a.sessionField(c, d, key, label, description, func() ui.Element {
		if d.handles[key] == nil {
			d.handles[key] = &ui.Handle{}
		}
		parts := uikit.PasswordInput(c, value, uikit.InputProps{
			Label: label, Handle: d.handles[key], Mono: true, DeferStyle: true,
		})
		input := a.sessionInputState(d, key, value, parts.Input)
		savedPassword := key == "password" && d.passwordSaved && !d.passwordChanged
		if savedPassword && !input.Composing() {
			p := theme.Current().C.Input
			// Native placeholders use muted text; paint the saved marker in the input's style.
			parts.Root.Children(func() {
				ui.Row(c).Key("saved-password-mask").Absolute().Top(0).Right(p.Height).Bottom(0).
					Left(p.PaddingX + p.BorderWidth).AlignItems(ui.Center).PassThrough().Children(func() {
					ui.Text(c, "••••••••••").Role(ui.RoleNone).Font(strings.Join(p.MonoFont, ", ")).
						FontSize(p.MonoSize).LetterSpacing(p.MonoTracking).FixedLineHeight(p.LineHeight).
						TextColor(uikit.Color(p.Text)).SingleLine().PassThrough()
				})
			})
		}
		if key == "passphrase" {
			input.Placeholder("私钥口令（无则留空）")
		}
		return input
	})
}

func (a *app) sessionKeySelect(c *ui.Context, d *hostDraft) ui.Element {
	previous := d.keyID
	if d.handles["key"] == nil {
		d.handles["key"] = &ui.Handle{}
	}
	options := make([]uikit.Option[string], 0, len(a.keys)+2)
	options = append(options, uikit.Option[string]{Value: "", Label: "请选择私钥", Disabled: true})
	for _, key := range a.keys {
		options = append(options, uikit.Option[string]{Value: key.ID, Label: key.Name,
			Detail: key.Type + " · " + abbreviateFingerprint(key.Fingerprint)})
	}
	options = append(options, uikit.Option[string]{Value: "__import__", Label: "导入私钥…"})
	input := uikit.Select(c.Key("private-key"), &d.keyID, options, uikit.SelectProps{
		Label: "私钥", Placeholder: "请选择私钥", Handle: d.handles["key"], Error: d.errors["key"],
		Leading: func() {
			uikit.Icon(c, sessionKeyIcon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneTertiary}).PassThrough()
		},
	})
	if previous != d.keyID {
		if d.keyID == "__import__" {
			d.keyID = previous
			a.choosePrivateKey()
		}
		d.changed("key")
	}
	return input
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

func (a *app) sessionDraftFooter(c *ui.Context, d *hostDraft, dismiss func()) {
	t := theme.Current()
	p := t.C.Modal
	uikit.ModalFooter(c).Children(func() {
		disabled := strings.TrimSpace(d.address) == ""
		uikit.Button(c, "测试连接", uikit.ButtonProps{Disabled: disabled || d.testResults != nil}).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.startDraftTest(c, d) })
		a.sessionTestResult(c, d)
		ui.Spacer(c)
		uikit.Button(c, "取消", uikit.ButtonProps{Variant: uikit.ButtonPlain}).Padding(0, p.ButtonPaddingX).OnClick(dismiss)
		label := "仅保存"
		if d.editing {
			label = "保存"
		}
		uikit.Button(c, label, uikit.ButtonProps{Disabled: disabled}).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.saveSessionDraft(c, d, false) })
		uikit.Button(c, "保存并连接", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Kbd: "↩", Disabled: disabled}).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.saveSessionDraft(c, d, true) })
	})
}

func (a *app) sessionTestResult(c *ui.Context, d *hostDraft) {
	if d.testResult == nil && d.testResults == nil {
		return
	}
	state := uikit.TestState{Visible: d.testVisible}
	if d.testResults == nil {
		state.State = uikit.TestOK
		if d.testResult.Err != nil {
			state.State = uikit.TestFailed
		}
		state.Summary, state.Detail = d.testResult.Summary, d.testResult.Detail
		state.ID, state.Elapsed = fmt.Sprintf("%p", d.testResult), d.testResult.Elapsed
	}
	uikit.TestResult(c, &state, uikit.TestResultProps{Motion: uikit.MotionOverride(a.reduceMotion(c)),
		SuccessIcon: sessionCheckIcon, FailureIcon: sessionAlertIcon, InfoIcon: infoIcon})
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
