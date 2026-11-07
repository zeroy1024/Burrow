package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	sessionKeyIcon   = uikit.IconSVG(uikit.IconKey)
	sessionEyeIcon   = uikit.IconSVG(uikit.IconEye)
	sessionAlertIcon = uikit.IconSVG(uikit.IconAlertCircle)
	sessionCheckIcon = uikit.IconSVG(uikit.IconCheckCircle)
)

func (a *app) hostDialog(c *ui.Context) {
	page := a.page()
	d := &page.draft
	if d.deleting {
		requested := false
		result := uikit.Modal(c.Key("delete-site-dialog"), &page.addHostOpen, uikit.ModalProps{
			Controller: &d.modal,
			Title:      locales.SessionDeleteTitle(d.name), Compact: true,
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
		Title: a.sessionDraftTitle(d), Disabled: d.saving, Dirty: d.dirty(),
		Controller: &d.modal, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		CloseKey: "session-close", CloseLabel: locales.MessageCloseSessionSettings(),
		Header: func(uikit.ModalParts) { a.sessionDraftHeader(c, d) },
	}, func(parts uikit.ModalParts) {
		panel, bounds := parts.Panel, parts.Bounds
		noticeHeight := float32(0)
		if count := a.draftOpenTabs(d); count > 0 {
			// Banner fills its parent's content width. Inset that content with
			// padding rather than adding margins outside an already full width.
			notice := ui.Box(c).FillWidth().MinWidth(0).Shrink(0).
				Padding(0, p.BodyPaddingX, t.Space.N12, p.BodyPaddingX).Children(func() {
				uikit.Banner(c, locales.SessionOpenTabsNotice(count), uikit.BannerProps{Icon: infoIcon})
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
		uikit.Scrollbars(c, panel, formScroll, &d.scroll, uikit.ScrollbarProps{Label: locales.MessageSessionFormScrollbar()})
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
	if dialog.Closing && !d.closing {
		a.closeSessionDraft(d)
	}
	if dialog.Dismissed && d.afterClose != nil {
		afterClose := d.afterClose
		d.afterClose = nil
		afterClose()
		c.Invalidate()
	}
}

func (a *app) sessionDraftTitle(d *hostDraft) string {
	if d.editing {
		return locales.MessageEditSession2()
	}
	if d.files {
		return locales.MessageNewFtpSftpSession()
	}
	return locales.MessageNewSshSession()
}

func (a *app) sessionDraftHeader(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Modal
	ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
		Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
		uikit.IconTile(c, uikit.IconTileProps{Size: uikit.SizeLg}, func() {
			if d.files {
				protocol := theme.Current().C.ProtocolIcon
				icon, fg, bg := folderLockIcon, protocol.SftpGlyph, protocol.SftpBg
				if d.protocol == "FTP" {
					icon, fg, bg = folderIcon, protocol.FtpGlyph, protocol.FtpBg
				}
				ui.Box(c).Key("protocol-header-" + d.protocol).Fill().Radius(t.C.IconTile.RadiusLg).Background(uikit.Color(bg)).Center().
					Transition(ui.ElementTransition{Duration: p.Fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).
					Children(func() { uikit.Icon(c, icon, uikit.IconProps{}).TextColor(uikit.Color(fg)) })
			} else if style, ok := a.draftSystemStyle(d); ok {
				ui.Box(c).Fill().Radius(t.C.IconTile.RadiusLg).Background(uikit.Color(t.OSLogoBg(style.color))).Center().Children(func() {
					ui.Image(c, style.icon).Size(t.C.OsLogo.GlyphSize, t.C.OsLogo.GlyphSize).TextColor(uikit.Color(uikit.OSLogoInk(style.color)))
				})
			} else {
				uikit.Icon(c, terminalIcon, uikit.IconProps{}).TextColor(uikit.Color(t.Color.Text.Strong))
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
			uikit.Text(c, a.sessionDraftTitle(d), uikit.TextProps{Style: uikit.TextTitle}).SingleLine()
			subtitle := locales.MessageTerminalSessionAppearsUnderSessionsAfterSaving()
			if d.files {
				subtitle = locales.MessageFileTransferSessionAppearsUnderSitesAfterSaving()
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
			{Label: "SFTP", IconSVG: folderLockIcon, IconTone: uikit.ToneAccent, Detail: locales.MessageEncryptedTransferOverSsh()},
			{Label: "FTP", IconSVG: folderIcon, IconTone: uikit.ToneTertiary, Detail: locales.MessageClassicFtpWithOptionalTls()},
		}, uikit.SegmentedProps{Label: locales.MessageProtocol(), Fill: true, Cards: true})
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
		if key == "host" && message.Empty() {
			message = d.errors["port"]
		}
		if !message.Empty() {
			uikit.FieldMessage(c, message.String(), uikit.FieldMessageProps{Tone: uikit.ToneDanger})
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
	uikit.StyleFormControl(e, d.errors[key].String())
	if e.Submitted() {
		d.submitted = true
	}
	return e
}

func (a *app) sessionDraftFields(c *ui.Context, d *hostDraft, dismiss func()) {
	t := theme.Current()
	a.sessionField(c, d, "name", locales.CommonName(), "", func() ui.Element {
		return a.sessionInput(
			c,
			d,
			"name",
			locales.CommonName(),
			&d.name,
			false,
		).Placeholder(locales.MessageOptionalDefaultsToHostname())
	})
	a.sessionField(c, d, "host", locales.MessageHost(), "", func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input = a.sessionInput(
				c,
				d,
				"host",
				locales.MessageHost(),
				&d.address,
				true,
			).Grow(1).Placeholder(locales.MessageHostnameOrIp())
			ui.Text(c, locales.MessagePort()).Shrink(0).FontSize(t.C.FormRow.LabelSize).TextColor(uikit.Color(t.C.FormRow.Label))
			a.sessionPortInput(c, d)
		})
		return input
	})
	a.sessionField(c, d, "user", locales.MessageUsername(), "", func() ui.Element {
		if d.auth == "anonymous" {
			anonymous := "anonymous"
			return a.sessionInput(c, d, "anonymous", locales.MessageUsername(), &anonymous, true)
		} else {
			return a.sessionInput(c, d, "user", locales.MessageUsername(), &d.user, true).Placeholder("root")
		}
	})
	a.sessionField(c, d, "auth", locales.MessageAuthenticationMethod(), "", func() ui.Element {
		values, labels := []string{"password", "key", "agent"}, []string{locales.CommonPassword(), locales.MessagePrivateKeys(), "Agent"}
		if d.protocol == "FTP" {
			values, labels = []string{"password", "anonymous"}, []string{locales.CommonPassword(), locales.MessageAnonymous()}
		}
		index := slices.Index(values, d.auth)
		input := uikit.Segmented(
			c.Key("auth"),
			&index,
			uikit.SegmentItems(labels),
			uikit.SegmentedProps{Label: locales.MessageAuthenticationMethod()},
		)
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
				a.sessionPasswordField(
					c,
					d,
					"password",
					locales.CommonPassword(),
					locales.MessageSaveInSystemCredentialStore(),
					&d.password,
				)
			case "key":
				a.sessionField(c, d, "key", locales.MessagePrivateKeys(), "", func() ui.Element {
					var input ui.Element
					ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
						input = a.sessionKeySelect(c, d)
						uikit.Button(
							c,
							locales.MessageManage(),
							uikit.ButtonProps{Size: uikit.SizeSm},
						).Shrink(0).OnClick(func() { d.afterClose = a.openCredentialSettings; dismiss() })
					})
					return input
				})
				a.sessionPasswordField(c, d, "passphrase", locales.MessagePassphrase(), "", &d.passphrase)
			case "agent":
				if !d.listedAgent {
					results := make(chan sessionAgentResult, 1)
					d.agentResults = results
					go func() { count, err := agentState(); results <- sessionAgentResult{count: count, err: err} }()
					d.listedAgent = true
				}
				a.sessionField(c, d, "agent", "Agent", "", func() ui.Element {
					if d.agentResults != nil {
						return uikit.Text(
							c,
							locales.MessageCheckingSshAgent(),
							uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
						)
					}
					color, icon, text := t.Color.Success.Text, sessionCheckIcon, locales.KeysAgentDetectedCount(d.agentCount)
					if d.agentError != nil {
						color, icon, text = t.Color.Warning.Text, sessionAlertIcon, locales.MessageSshAgentNotDetectedSshAuthSock()
					}
					return ui.Row(c).Height(t.C.Input.Height).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
						uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeSm}).TextColor(uikit.Color(color))
						uikit.Text(c, text, uikit.TextProps{Style: uikit.TextSmall}).TextColor(uikit.Color(color))
					})
				})
			}
		})
	}

	a.sessionField(c, d, "gateway", locales.MessageConnectionGateway(), "", func() ui.Element {
		input := uikit.Select(
			c,
			&d.gateway,
			a.hostGatewayOptions(d),
			uikit.SelectProps{Label: locales.MessageConnectionGateway()},
		)
		if input.Changed() {
			d.changed("gateway")
		}
		return input
	})
	a.sessionField(c, d, "group", locales.MessageGroup(), "", func() ui.Element {
		if d.creatingGroup {
			return a.sessionInput(
				c,
				d,
				"group",
				locales.MessageNewGroup(),
				&d.newGroup,
				false,
			).Placeholder(locales.MessageNewGroupName())
		}
		options := append(a.groupNames(), locales.MessageNewGroup2())
		previous := d.group
		input := uikit.Select(c, &d.group, uikit.StringOptions(options), uikit.SelectProps{Label: locales.MessageGroup()})
		if input.Changed() {
			if d.group == locales.MessageNewGroup2() {
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
		input := a.sessionInput(c, d, "port", locales.MessagePort(), &d.port, true)
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
			input.Placeholder(locales.MessagePrivateKeyPassphraseLeaveBlankIfNone())
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
	options = append(options, uikit.Option[string]{Value: "", Label: locales.MessageSelectAPrivateKey(), Disabled: true})
	for _, key := range a.keys {
		options = append(options, uikit.Option[string]{Value: key.ID, Label: key.Name,
			Detail: key.Type + " · " + abbreviateFingerprint(key.Fingerprint)})
	}
	options = append(options, uikit.Option[string]{Value: "__import__", Label: locales.MessageImportPrivateKey2()})
	input := uikit.Select(c.Key("private-key"), &d.keyID, options, uikit.SelectProps{
		Label: locales.MessagePrivateKeys(), Placeholder: locales.MessageSelectAPrivateKey(), Handle: d.handles["key"], Error: d.errors["key"].String(),
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
		uikit.Button(
			c,
			locales.MessageTestConnection(),
			uikit.ButtonProps{Disabled: disabled || d.testResults != nil},
		).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.startDraftTest(c, d) })
		a.sessionTestResult(c, d)
		ui.Spacer(c)
		uikit.Button(
			c,
			locales.CommonCancel(),
			uikit.ButtonProps{Variant: uikit.ButtonPlain},
		).Padding(0, p.ButtonPaddingX).OnClick(dismiss)
		label := locales.MessageSaveOnly()
		if d.editing {
			label = locales.CommonSave()
		}
		uikit.Button(c, label, uikit.ButtonProps{Disabled: disabled}).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.saveSessionDraft(c, d, false) })
		uikit.Button(
			c,
			locales.MessageSaveAndConnect(),
			uikit.ButtonProps{Variant: uikit.ButtonPrimary, Kbd: "↩", Disabled: disabled},
		).Padding(0, p.ButtonPaddingX).Shrink(0).OnClick(func() { a.saveSessionDraft(c, d, true) })
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
		state.Summary, state.Detail = d.testResult.Summary.String(), d.testResult.Detail.String()
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
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: locales.MessageChooseLocalDirectory(), Directory: true})
		mygo.RunOnMain(func() {
			if d.generation != generation {
				return
			}
			d.choosingDirectory = false
			if err != nil {
				a.terminalError = locales.Errorf(locales.MessageChooseLocalDirectory2, err)
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
