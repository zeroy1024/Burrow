package desktop

import (
	"slices"
	"strconv"
	"strings"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) sessionAdvanced(c *ui.Context, d *hostDraft) {
	trigger := uikit.Section(c, &d.advanced, uikit.SectionProps{
		Key: "advanced", Title: locales.MessageAdvanced(), Summary: d.advancedSummary(a), Chevron: chevronIcon,
		Reduced: a.reduceMotion(c),
	}, func() {
		if d.files {
			a.sessionFileAdvanced(c, d)
		} else {
			a.sessionSSHAdvanced(c, d)
		}
	})
	if trigger.Changed() {
		a.setModalAdvancedOpen(strings.ToLower(d.protocol), d.advanced)
	}
}

func (a *app) sessionSSHAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()

	a.sessionField(c, d, "platform", locales.MessageRemoteSystem(), "", func() ui.Element {
		options := []uikit.Option[string]{{Value: "", Label: locales.MessageDetectAutomatically()}}
		for _, platform := range hostinfo.Overrides {
			options = append(options, uikit.Option[string]{Value: string(platform), Label: platform.DisplayName()})
		}
		input := uikit.Select(
			c,
			&d.platform,
			options,
			uikit.SelectProps{Label: locales.MessageRemoteSystem()},
		).Width(t.C.Input.SelectWidth)
		if input.Changed() {
			d.changed("platform")
		}
		return input
	})
	a.sessionField(c, d, "keepalive", locales.TcpKeepalive(), locales.TcpKeepaliveHelp(), func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input = a.sessionInput(
				c,
				d,
				"keepalive",
				locales.MessageKeepaliveSeconds(),
				&d.keepalive,
				true,
			).Width(t.C.Input.NumberWidth).Shrink(0)
			uikit.Text(
				c,
				locales.MessageSecondsDisablesKeepalive(),
				uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary},
			)
		})
		return input
	})
	a.sessionField(c, d, "healthCheck", locales.SshHealthCheck(), locales.SshHealthCheckHelp(), func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input = a.sessionInput(c, d, "healthCheck", locales.SshHealthCheck(), &d.healthCheck, true).
				Width(t.C.Input.NumberWidth).Shrink(0)
			uikit.Text(c, locales.SshHealthCheckDisabled(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
		})
		return input
	})
	a.sessionField(c, d, "command", locales.MessageInitialCommand(), "", func() ui.Element {
		return a.sessionInput(c, d, "command", locales.MessageInitialCommand(), &d.initCommand, true)
	})
	a.sessionField(
		c,
		d,
		"scheme",
		locales.MessageTerminalColorScheme(),
		"",
		func() ui.Element { return a.sessionSchemeSelect(c, d) },
	)
	a.sessionField(
		c,
		d,
		"env",
		locales.MessageEnvironmentVariables(),
		"",
		func() ui.Element { return a.sessionEnvironment(c, d) },
	)
}

func (a *app) sessionSchemeSelect(c *ui.Context, d *hostDraft) ui.Element {
	t := theme.Current()
	selected := d.schemeName()
	options := make([]uikit.Option[string], 0, len(termtheme.Names())+1)
	options = append(options, uikit.Option[string]{Value: "", Label: locales.MessageUseGlobalSetting()})
	for _, name := range termtheme.Names() {
		scheme, _ := termtheme.Lookup(name)
		options = append(options, uikit.Option[string]{Value: name, Label: name, Leading: func() {
			ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).
				Background(uikit.Color(scheme.Background)).PassThrough()
		}})
	}
	props := uikit.SelectProps{Label: locales.MessageTerminalColorScheme()}
	if selected == "" {
		props.Detail = a.terminal.followLight + " / " + a.terminal.followDark
		props.Leading = func() {
			ui.Row(c).Shrink(0).PassThrough().Children(func() {
				for _, name := range []string{a.terminal.followLight, a.terminal.followDark} {
					if scheme, ok := termtheme.Lookup(name); ok {
						ui.Box(c).Size(t.Space.N6, t.Size.IconSm).Background(uikit.Color(scheme.Background)).PassThrough()
					}
				}
			})
		}
	}
	input := uikit.Select(c.Key("session-scheme"), &selected, options, props)
	if selected != d.schemeName() {
		d.termScheme = ""
		if selected != "" {
			d.termScheme = termtheme.Fixed(selected).String()
		}
		d.changed("scheme")
	}
	return input
}

func (a *app) sessionEnvironment(c *ui.Context, d *hostDraft) ui.Element {
	if d.handles["env"] == nil {
		d.handles["env"] = &ui.Handle{}
	}
	result := uikit.ChipInput(c, &d.env, uikit.ChipInputProps{
		Label: locales.MessageEnvironmentVariableKeyValue(), Placeholder: "KEY=VALUE", Mono: true,
		PreserveCommas: true, CommitOnBlur: true, Handle: d.handles["env"], Draft: &d.envInput,
		Validate: func(value string) error {
			if !environmentPattern.MatchString(value) {
				return locales.NewError(locales.MessageUseTheFormatKeyValue)
			}
			return nil
		},
	})
	if result.Changed {
		d.changed("env")
	}
	if result.Error != "" {
		d.errors["env"] = locales.Deferred(locales.MessageUseTheFormatKeyValue)
	}
	return result.Input
}

func (a *app) sessionFileAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	a.sessionField(c, d, "remoteDir", locales.MessageRemoteDirectory(), locales.MessageDirectoryOpenedAfterConnectingLeaveBlankForTheLoginDirectory(), func() ui.Element {
		return a.sessionInput(c, d, "remoteDir", locales.MessageRemoteDirectory(), &d.remoteDir, true)
	})
	a.sessionField(c, d, "localDir", locales.MessageLocalDirectories(), locales.MessageOpenedWhenShowingTheLocalPaneLeaveBlankForTheDefaultInSettings(), func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			input = a.sessionInput(c, d, "localDir", locales.MessageLocalDirectories(), &d.localDir, true).Grow(1)
			surface := *t
			surface.C.IconButton.Size = t.C.Input.Height
			surface.C.IconButton.BgFilled = t.C.Button.Bg
			uikit.IconButton(c, folderIcon, uikit.IconButtonProps{Key: "choose-directory", Label: locales.MessageChooseLocalDirectory(), Variant: uikit.IconFilled,
				Disabled: d.choosingDirectory, Surface: &surface}).
				Border(surface.C.Button.BorderWidth, uikit.Color(surface.C.Button.Border)).OnClick(func() { a.chooseSessionDirectory(d) })
		})
		return input
	})
	a.sessionField(c, d, "encoding", locales.MessageFilenameEncoding(), "", func() ui.Element {
		input := uikit.Select(
			c,
			&d.encoding,
			uikit.StringOptions([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}),
			uikit.SelectProps{Label: locales.MessageFilenameEncoding()},
		).Width(t.C.Input.SelectWidth)
		if input.Changed() {
			d.changed("encoding")
		}
		return input
	})
	if d.protocol == "FTP" {
		ui.Column(c).Key("ftp-options").FillWidth().Gap(t.C.FormRow.RowGap).Transition(a.sessionCollapseTransition(c)).Children(func() {
			a.sessionField(c, d, "ftpMode", locales.MessageTransferMode(), "", func() ui.Element {
				selected := 0
				if d.ftpMode == "active" {
					selected = 1
				}
				input := uikit.Segmented(
					c,
					&selected,
					uikit.SegmentItems([]string{locales.MessagePassivePasv(), locales.MessageActivePort()}),
					uikit.SegmentedProps{Label: locales.MessageFtpMode()},
				)
				if input.Changed() {
					d.ftpMode = []string{"passive", "active"}[selected]
					d.changed("ftpMode")
				}
				return input
			})
			description := map[string]string{"none": locales.MessageUnencryptedPasswordsCanBeIntercepted(), "explicit": locales.MessageAuthTlsUsesTheSamePort(), "implicit": locales.MessageImplicitTlsDefaultPort()}[d.ftpTLS]
			a.sessionField(c, d, "ftpTLS", locales.MessageEncryption(), description, func() ui.Element {
				values := []string{"none", "explicit", "implicit"}
				selected := slices.Index(values, d.ftpTLS)
				input := uikit.Segmented(
					c,
					&selected,
					uikit.SegmentItems([]string{locales.MessageNone(), locales.MessageExplicitTls(), locales.MessageImplicitTls()}),
					uikit.SegmentedProps{Label: "TLS"},
				)
				if input.Changed() {
					d.ftpTLS = values[selected]
					if !d.portEdited {
						d.port = strconv.Itoa(d.defaultPort())
					}
					d.changed("ftpTLS")
				}
				return input
			})
		})
	}
}
