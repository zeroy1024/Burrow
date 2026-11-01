package desktop

import (
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (a *app) sessionAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Collapse
	ui.Column(c).Key("advanced").FillWidth().BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(tokenColor(t.C.Modal.Divider)).Children(func() {
		parts := ui.CollapsibleBase(c, &d.advanced)
		trigger := parts.Trigger.FillWidth().Height(p.HeaderHeight).Padding(0, t.C.Modal.BodyPaddingX).Gap(t.Space.N8).AlignItems(ui.Center).Label("高级")
		progress := trigger.AnimateTo("advanced-progress", boolFloat(d.advanced), tokenTween(p.Duration))
		if a.reduceMotion(c) {
			progress = boolFloat(d.advanced)
		}
		if trigger.Changed() {
			a.setModalAdvancedOpen(strings.ToLower(d.protocol), d.advanced)
		}
		trigger.Children(func() {
			ui.Icon(c, chevronIcon).Size(p.Chevron, p.Chevron).Rotate(-p.ChevronRotate + p.ChevronRotate*progress).TextColor(tokenColor(p.ChevronColor)).PassThrough()
			ui.Text(c, "高级").FontSize(p.TitleSize).FontWeight(p.TitleWeight).TextColor(tokenColor(p.Title)).PassThrough()
			target := float32(1)
			if d.advanced {
				target = 0
			}
			summary := ui.Text(c, d.advancedSummary(a)).Grow(1).MinWidth(0).SingleLine().FontSize(p.SummarySize).TextColor(tokenColor(p.Summary)).PassThrough()
			summary.Opacity(animateVisual(summary, "summary", target, t.Motion.Duration.Exit))
		})
		if !d.advanced && progress == 0 {
			return
		}
		clip := ui.Column(c).FillWidth().Shrink(0).ClipY().Disabled(!d.advanced)
		if progress < 1 {
			clip.Height(progress * d.advancedHeight)
		}
		clip.Children(func() {
			inner := ui.Column(c).FillWidth().Shrink(0).Padding(0, t.C.Modal.BodyPaddingX, t.Space.N20, t.C.Modal.BodyPaddingX).Gap(t.C.FormRow.RowGap)
			inner.Opacity(min(1, progress*p.ContentFadeFactor))
			inner.Children(func() {
				if d.files {
					a.sessionFileAdvanced(c, d)
				} else {
					a.sessionSSHAdvanced(c, d)
				}
			})
			if height := inner.Bounds().H; height > 0 {
				d.advancedHeight = height
			}
		})
		if d.advancedHeight == 0 {
			c.AnimationFrame()
		}
	})
}

func boolFloat(value bool) float32 {
	if value {
		return 1
	}
	return 0
}

func (a *app) sessionSSHAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	a.sessionField(c, d, "jump", "跳板机", "", func() {
		options := []string{"无"}
		ids := []string{""}
		for _, id := range a.jumpCandidates(d) {
			h := a.hosts[id]
			options = append(options, h.name+"  "+h.user+"@"+h.address)
			ids = append(ids, h.session.ID)
		}
		index := slices.Index(ids, d.jumpID)
		selected := "无"
		if index >= 0 {
			selected = options[index]
		}
		if sessionSelect(c, "跳板机", &selected, options).Changed() {
			d.jumpID = ids[slices.Index(options, selected)]
			d.changed("jump")
		}
	})
	a.sessionField(c, d, "keepalive", "Keepalive", "", func() {
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			a.sessionInput(c, d, "keepalive", "Keepalive 秒", &d.keepalive, true).Width(t.C.Input.NumberWidth).Shrink(0)
			ui.Text(c, "秒 · 0 为关闭").FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.Color.Text.Tertiary))
		})
	})
	a.sessionField(c, d, "command", "初始命令", "", func() { a.sessionInput(c, d, "command", "初始命令", &d.initCommand, true) })
	a.sessionField(c, d, "scheme", "终端配色", "", func() { a.sessionSchemeSelect(c, d) })
	a.sessionField(c, d, "env", "环境变量", "", func() { a.sessionEnvironment(c, d) })
}

func (a *app) sessionSchemeSelect(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Input
	selected := d.schemeName()
	parts := ui.SelectBase(c.Key("session-scheme"), &selected)
	parts.Trigger.Height(p.Height).FillWidth().MinWidth(0).Padding(0, p.PaddingX).Gap(p.Gap).Radius(p.Radius).
		Background(tokenColor(p.Bg)).Border(p.BorderWidth, tokenColor(p.Border)).Label("终端配色").Children(func() {
		if selected == "跟随全局" {
			ui.Row(c).Shrink(0).Children(func() {
				for _, name := range []string{a.terminal.followLight, a.terminal.followDark} {
					if scheme, ok := termtheme.Lookup(name); ok {
						ui.Box(c).Size(t.Space.N6, t.Size.IconSm).Background(tokenColor(scheme.Background)).PassThrough()
					}
				}
			})
		} else if scheme, ok := termtheme.Lookup(selected); ok {
			ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).Background(tokenColor(scheme.Background)).PassThrough()
		}
		ui.Text(c, selected).FontSize(p.FontSize).SingleLine()
		if selected == "跟随全局" {
			ui.Text(c, a.terminal.followLight+" / "+a.terminal.followDark).Grow(1).MinWidth(0).SingleLine().FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.Color.Text.Secondary))
		} else {
			ui.Spacer(c)
		}
		ui.Icon(c, chevronIcon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(tokenColor(p.Icon))
	})
	parts.Popup(func(panel ui.Element) {
		panel.Padding(t.Space.N4).Radius(t.C.Menu.Radius).Background(tokenColor(t.C.Menu.Bg))
		applyShadow(panel, t.C.Menu.Shadow)
		for _, name := range append([]string{"跟随全局"}, termtheme.Names()...) {
			item := parts.Item(name).Padding(t.Space.N6, t.C.Menu.ItemPaddingX).Gap(t.Space.N8).Radius(t.C.Menu.ItemRadius)
			if item.Highlighted() {
				item.Background(tokenColor(t.Color.State.Hover))
			}
			item.Children(func() {
				if scheme, ok := termtheme.Lookup(name); ok {
					ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).Background(tokenColor(scheme.Background))
				}
				ui.Text(c, name).FontSize(t.Font.Size.Body)
			})
		}
	})
	if selected != d.schemeName() {
		d.termScheme = ""
		if selected != "跟随全局" {
			d.termScheme = termtheme.Fixed(selected).String()
		}
		d.changed("scheme")
	}
}

func (a *app) sessionEnvironment(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	p := t.C.Input
	ui.Row(c).FillWidth().MinHeight(p.Height).Padding(t.Space.N3).Gap(t.Space.N4).Wrap().Radius(p.Radius).
		Background(tokenColor(p.Bg)).Border(p.BorderWidth, tokenColor(p.Border)).Children(func() {
		for index, value := range d.env {
			chip := ui.Row(c).Key(index).Height(p.ChipHeight).Padding(0, t.Space.N6).Gap(t.Space.N4).Radius(p.ChipRadius).Background(tokenColor(p.ChipBg)).AlignItems(ui.Center)
			if !environmentPattern.MatchString(value) {
				chip.Background(tokenColor(t.Color.Danger.Subtle))
			}
			chip.Children(func() {
				ui.Text(c, value).Font(strings.Join(p.MonoFont, ", ")).FontSize(p.ChipFontSize).SingleLine()
				label := "删除环境变量 " + strings.SplitN(value, "=", 2)[0]
				remove := styleIconButton(ui.ButtonBase(c.Key("remove-env")).Label(label), t).
					Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).Padding(0).Center()
				tooltip(c, remove, label)
				remove.Children(func() {
					ui.Icon(c, closeIcon).Size(t.Size.IconSm, t.Size.IconSm).Shrink(0).PassThrough()
				})
				remove.OnClick(func() {
					d.env = slices.Delete(d.env, index, index+1)
					d.changed("env")
				})
			})
		}
		if d.handles["env"] == nil {
			d.handles["env"] = &ui.Handle{}
		}
		input := ui.TextInputBase(c.Key("env-entry"), &d.envInput).Bind(d.handles["env"]).Height(p.ChipHeight).MinWidth(t.C.Input.NumberWidth).Grow(1).
			Font(strings.Join(p.MonoFont, ", ")).FontSize(p.MonoSize).FixedLineHeight(p.LineHeight).Placeholder("KEY=VALUE").Label("环境变量 KEY=VALUE")
		input.OnTextKey(func(k ui.TextKey) bool {
			if k.Mods == 0 && k.Key == ui.KeyEnter {
				value := k.Text()
				if value != "" {
					d.env = append(d.env, value)
					k.SetText("")
					d.changed("env")
					if !environmentPattern.MatchString(value) {
						d.errors["env"] = "格式应为 KEY=VALUE"
					}
				}
				return true
			}
			if k.Mods == 0 && k.Key == ui.KeyBackspace && k.Text() == "" && len(d.env) > 0 {
				d.env = d.env[:len(d.env)-1]
				d.changed("env")
				return true
			}
			return false
		})
		if input.Changed() {
			d.changed("env")
		}
		if d.focused["env"] && !input.Focused() && d.envInput != "" {
			d.env = append(d.env, d.envInput)
			d.envInput = ""
			d.changed("env")
			d.errors["env"] = a.draftError(d, "env")
		}
		d.focused["env"] = input.Focused()
	})
}

func (a *app) sessionFileAdvanced(c *ui.Context, d *hostDraft) {
	t := theme.Current()
	a.sessionField(c, d, "remoteDir", "远程目录", "连接后打开的目录，留空 = 登录目录", func() { a.sessionInput(c, d, "remoteDir", "远程目录", &d.remoteDir, true) })
	a.sessionField(c, d, "localDir", "本地目录", "展开本地面板时打开，留空 = 设置里的默认目录", func() {
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			a.sessionInput(c, d, "localDir", "本地目录", &d.localDir, true).Grow(1)
			keyedIconButton(c, "choose-directory", "选择本地目录", folderIcon).Size(t.C.Input.Height, t.C.Input.Height).
				Background(tokenColor(t.C.Button.Bg)).Border(t.C.Button.BorderWidth, tokenColor(t.C.Button.Border)).Disabled(d.choosingDirectory).OnClick(func() { a.chooseSessionDirectory(d) })
		})
	})
	a.sessionField(c, d, "encoding", "文件名编码", "", func() {
		if sessionSelect(c, "文件名编码", &d.encoding, []string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}).Width(t.C.Input.SelectWidth).Changed() {
			d.changed("encoding")
		}
	})
	if d.protocol == "FTP" {
		ui.Column(c).Key("ftp-options").FillWidth().Gap(t.C.FormRow.RowGap).Transition(a.sessionCollapseTransition(c)).Children(func() {
			a.sessionField(c, d, "ftpMode", "传输模式", "", func() {
				selected := 0
				if d.ftpMode == "active" {
					selected = 1
				}
				if sessionSegments(c, &selected, "被动 (PASV)", "主动 (PORT)").Changed() {
					d.ftpMode = []string{"passive", "active"}[selected]
					d.changed("ftpMode")
				}
			})
			description := map[string]string{"none": "明文传输，密码可被截获", "explicit": "AUTH TLS，端口不变", "implicit": "隐式 TLS 默认端口 990"}[d.ftpTLS]
			a.sessionField(c, d, "ftpTLS", "加密", description, func() {
				values := []string{"none", "explicit", "implicit"}
				selected := slices.Index(values, d.ftpTLS)
				if sessionSegments(c, &selected, "无", "显式 TLS", "隐式 TLS").Changed() {
					d.ftpTLS = values[selected]
					if !d.portEdited {
						d.port = strconv.Itoa(d.defaultPort())
					}
					d.changed("ftpTLS")
				}
			})
		})
	}
}
