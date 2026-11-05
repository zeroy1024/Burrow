package desktop

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/hostinfo"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) sessionAdvanced(c *ui.Context, d *hostDraft) {
	trigger := uikit.Section(c, &d.advanced, uikit.SectionProps{
		Key: "advanced", Title: "高级", Summary: d.advancedSummary(a), Chevron: chevronIcon,
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
	a.sessionField(c, d, "jump", "跳板机", "", func() ui.Element {
		options := []uikit.Option[string]{{Value: "", Label: "无"}}
		for _, id := range a.jumpCandidates(d) {
			h := a.hosts[id]
			options = append(options, uikit.Option[string]{Value: h.session.ID, Label: h.name + "  " + h.user + "@" + h.address})
		}
		input := uikit.Select(c, &d.jumpID, options, uikit.SelectProps{Label: "跳板机"})
		if input.Changed() {
			d.changed("jump")
		}
		return input
	})
	a.sessionField(c, d, "platform", "远端系统", "", func() ui.Element {
		options := []uikit.Option[string]{{Value: "", Label: "自动识别"}}
		for _, platform := range hostinfo.Overrides {
			options = append(options, uikit.Option[string]{Value: string(platform), Label: platform.DisplayName()})
		}
		input := uikit.Select(c, &d.platform, options, uikit.SelectProps{Label: "远端系统"}).Width(t.C.Input.SelectWidth)
		if input.Changed() {
			d.changed("platform")
		}
		return input
	})
	a.sessionField(c, d, "keepalive", "Keepalive", "", func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input = a.sessionInput(c, d, "keepalive", "Keepalive 秒", &d.keepalive, true).Width(t.C.Input.NumberWidth).Shrink(0)
			uikit.Text(c, "秒 · 0 为关闭", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
		})
		return input
	})
	a.sessionField(c, d, "command", "初始命令", "", func() ui.Element { return a.sessionInput(c, d, "command", "初始命令", &d.initCommand, true) })
	a.sessionField(c, d, "scheme", "终端配色", "", func() ui.Element { return a.sessionSchemeSelect(c, d) })
	a.sessionField(c, d, "env", "环境变量", "", func() ui.Element { return a.sessionEnvironment(c, d) })
}

func (a *app) sessionSchemeSelect(c *ui.Context, d *hostDraft) ui.Element {
	t := theme.Current()
	selected := d.schemeName()
	options := make([]uikit.Option[string], 0, len(termtheme.Names())+1)
	options = append(options, uikit.Option[string]{Value: "跟随全局", Label: "跟随全局"})
	for _, name := range termtheme.Names() {
		scheme, _ := termtheme.Lookup(name)
		options = append(options, uikit.Option[string]{Value: name, Label: name, Leading: func() {
			ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).
				Background(uikit.Color(scheme.Background)).PassThrough()
		}})
	}
	props := uikit.SelectProps{Label: "终端配色"}
	if selected == "跟随全局" {
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
		if selected != "跟随全局" {
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
		Label: "环境变量 KEY=VALUE", Placeholder: "KEY=VALUE", Mono: true,
		PreserveCommas: true, CommitOnBlur: true, Handle: d.handles["env"], Draft: &d.envInput,
		Validate: func(value string) error {
			if !environmentPattern.MatchString(value) {
				return errors.New("格式应为 KEY=VALUE")
			}
			return nil
		},
	})
	if result.Changed {
		d.changed("env")
	}
	if result.Error != "" {
		d.errors["env"] = result.Error
	}
	return result.Input
}

func (a *app) sessionFileAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	a.sessionField(c, d, "remoteDir", "远程目录", "连接后打开的目录，留空 = 登录目录", func() ui.Element { return a.sessionInput(c, d, "remoteDir", "远程目录", &d.remoteDir, true) })
	a.sessionField(c, d, "localDir", "本地目录", "展开本地面板时打开，留空 = 设置里的默认目录", func() ui.Element {
		var input ui.Element
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			input = a.sessionInput(c, d, "localDir", "本地目录", &d.localDir, true).Grow(1)
			surface := *t
			surface.C.IconButton.Size = t.C.Input.Height
			surface.C.IconButton.BgFilled = t.C.Button.Bg
			uikit.IconButton(c, folderIcon, uikit.IconButtonProps{Key: "choose-directory", Label: "选择本地目录", Variant: uikit.IconFilled,
				Disabled: d.choosingDirectory, Surface: &surface}).
				Border(surface.C.Button.BorderWidth, uikit.Color(surface.C.Button.Border)).OnClick(func() { a.chooseSessionDirectory(d) })
		})
		return input
	})
	a.sessionField(c, d, "encoding", "文件名编码", "", func() ui.Element {
		input := uikit.Select(c, &d.encoding, uikit.StringOptions([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}), uikit.SelectProps{Label: "文件名编码"}).Width(t.C.Input.SelectWidth)
		if input.Changed() {
			d.changed("encoding")
		}
		return input
	})
	if d.protocol == "FTP" {
		ui.Column(c).Key("ftp-options").FillWidth().Gap(t.C.FormRow.RowGap).Transition(a.sessionCollapseTransition(c)).Children(func() {
			a.sessionField(c, d, "ftpMode", "传输模式", "", func() ui.Element {
				selected := 0
				if d.ftpMode == "active" {
					selected = 1
				}
				input := uikit.Segmented(c, &selected, uikit.SegmentItems([]string{"被动 (PASV)", "主动 (PORT)"}), uikit.SegmentedProps{Label: "FTP 模式"})
				if input.Changed() {
					d.ftpMode = []string{"passive", "active"}[selected]
					d.changed("ftpMode")
				}
				return input
			})
			description := map[string]string{"none": "明文传输，密码可被截获", "explicit": "AUTH TLS，端口不变", "implicit": "隐式 TLS 默认端口 990"}[d.ftpTLS]
			a.sessionField(c, d, "ftpTLS", "加密", description, func() ui.Element {
				values := []string{"none", "explicit", "implicit"}
				selected := slices.Index(values, d.ftpTLS)
				input := uikit.Segmented(c, &selected, uikit.SegmentItems([]string{"无", "显式 TLS", "隐式 TLS"}), uikit.SegmentedProps{Label: "TLS"})
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
