package desktop

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
)

type settingsCategory struct {
	id, label, subtitle string
	icon                *ui.SVG
}

var settingsCategories = [...]settingsCategory{
	{"general", "通用", "语言、启动与选项卡", settingsIcon},
	{"appearance", "外观", "主题、强调色与动态效果", paletteIcon},
	{"terminal", "终端", "配色、背景、字体与光标 · 修改立即生效", terminalIcon},
	{"files", "文件传输", "目录、传输与同名文件", filesIcon},
	{"shortcuts", "快捷键", "点击快捷键进行录制", textCursorIcon},
	{"keys", "密钥与凭据", "私钥、ssh-agent 与已保存的密码", lockIcon},
	{"about", "关于", "SSH Desk 与开源许可", infoIcon},
}

type settingField struct {
	label, description string
	control            func()
}

func (a *app) settingsPage(c *ui.Context) {
	if !a.settingsUI.open {
		return
	}
	t := theme.Current()
	p := t.C.Settings
	a.settingsUI.locale = c.Locale()
	ui.Row(c).Key("settings-page").Fill().Gap(t.Space.N8).Children(func() {
		workspaceCard(c).Width(p.NavWidth).Shrink(0).FillHeight().Children(func() { a.settingsSidebar(c, 1) })
		a.settings(c)
	})
}

func (a *app) settingsSidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	p := t.C.Settings
	ui.Column(c).Fill().Children(func() {
		ui.Text(c, localize(c, "设置")).FontSize(t.C.Sider.TitleSize).FontWeight(t.C.Sider.TitleWeight).
			Padding(t.C.Sider.HeaderPaddingTop, t.C.Sider.HeaderPaddingRight, t.C.Sider.HeaderPaddingBottom, t.C.Sider.HeaderPaddingLeft)
		a.sidebarSearch(c.Key("settings-search"), &a.settingsUI.query, &a.settingsUI.search,
			localize(c, "搜索设置"), localize(c, "搜索设置"))
		ui.Column(c).FillWidth().Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Gap(t.C.Catalog.RowGap).Children(func() {
			for index, category := range settingsCategories {
				item := ui.ButtonBase(c).Key(category.id).FillWidth().Height(p.NavItemHeight).
					Padding(0, t.C.GroupHeader.PaddingX).Radius(p.NavItemRadius).Justify(ui.Start).AlignItems(ui.Center).
					Gap(t.C.SiderItem.Gap).Label(localize(c, category.label)).Selected(a.settingsCategory == index)
				bg, fg, icon := ui.Transparent, tokenColor(t.Color.Text.Primary), tokenColor(t.Color.Text.Secondary)
				if a.settingsCategory == index {
					bg, fg, icon = tokenColor(p.NavItemOn), tokenColor(p.NavTextOn), tokenColor(p.NavIconOn)
				} else if item.Hovered() {
					bg = tokenColor(t.Color.State.Hover)
				}
				item.Background(bg).TextColor(fg).
					OnClick(func() { a.selectSettingsCategory(index) }).Children(func() {
					ui.Box(c).Size(p.NavIconTile, p.NavIconTile).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, category.icon).Size(t.Size.Icon, t.Size.Icon).TextColor(icon).PassThrough()
					})
					ui.Text(c, localize(c, category.label)).FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Medium).PassThrough()
				})
			}
		})
		ui.Box(c).Grow(1)
		ui.Row(c).FillWidth().BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(tokenColor(t.Color.Border.Subtle)).
			Padding(t.Space.N10, t.Space.N16).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, infoIcon).Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(t.Color.Text.Tertiary))
			ui.Text(c, localize(c, "SSH Desk 0.1.0 · 原型")).FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Tertiary))
		})
	})
}

func (a *app) settings(c *ui.Context) {
	t := theme.Current()
	p := t.C.Settings
	workspaceCard(c).Key("settings-content").Grow(1).MinWidth(0).FillHeight().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
			BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(tokenColor(t.Color.Border.Divider)).AlignItems(ui.Center).Children(func() {
			category := settingsCategories[a.settingsCategory]
			ui.Column(c).Grow(1).Gap(t.Space.N4).Children(func() {
				title, subtitle := category.label, category.subtitle
				if strings.TrimSpace(a.settingsUI.query) != "" {
					title, subtitle = "搜索结果", "匹配设置名称与说明 · 修改立即生效"
				}
				ui.Text(c, localize(c, title)).FontSize(p.TitleSize).FontWeight(p.TitleWeight)
				ui.Text(c, localize(c, subtitle)).FontSize(p.SubtitleSize).TextColor(tokenColor(p.Subtitle))
			})
		})
		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			scroll := &a.settingsUI.scroll[a.settingsCategory]
			ui.Scroll(c).Fill().TrackScroll(scroll).Children(func() {
				ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
					content := ui.Column(c).Key(a.settingsCategory).FillWidth().MaxWidth(p.ContentMax+2*p.ContentPaddingX).
						Padding(p.ContentPaddingY, p.ContentPaddingX).Gap(p.GroupGap)
					content.Children(func() {
						if a.settingsUI.notice != "" {
							ui.Text(c, localize(c, a.settingsUI.notice)).FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.Color.Danger.Text))
						}
						if a.terminalError != nil {
							ui.Text(c, localize(c, a.terminalError.Error())).FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.Color.Danger.Text))
						}
						if strings.TrimSpace(a.settingsUI.query) != "" {
							count := 0
							for index := range settingsCategories {
								if a.settingsCategoryHasMatch(index) {
									a.settingsCategoryContent(c, index)
									count++
								}
							}
							if count == 0 {
								ui.Text(c, localize(c, "没有匹配的设置")).FontSize(t.Font.Size.Body).TextColor(tokenColor(t.Color.Text.Tertiary))
							}
						} else {
							a.settingsCategoryContent(c, a.settingsCategory)
						}
					})
				})
			})
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(p.FadeHeight).PassThrough().
				LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{{Color: tokenColor(theme.WithAlpha(t.Color.Surface.Card, 0))}, {At: 1, Color: tokenColor(t.Color.Surface.Card)}}})
		})
	})
}

func (a *app) settingsCategoryContent(c *ui.Context, index int) {
	switch index {
	case 0:
		a.generalSettings(c)
	case 1:
		a.appearanceSettings(c)
	case 2:
		a.terminalSettings(c)
	case 3:
		a.fileSettings(c)
	case 4:
		a.shortcutSettings(c)
	case 5:
		a.keySettings(c)
	case 6:
		a.aboutSettings(c)
	}
}

func (a *app) settingMatches(label, description string) bool {
	query := strings.ToLower(strings.TrimSpace(a.settingsUI.query))
	if query == "" {
		return true
	}
	haystack := []rune(strings.ToLower(label + description + localizeTag(a.settingsUI.locale, label) + localizeTag(a.settingsUI.locale, description)))
	needle := []rune(query)
	at := 0
	for _, r := range haystack {
		if r == needle[at] {
			at++
			if at == len(needle) {
				return true
			}
		}
	}
	return false
}

func (a *app) settingsCategoryHasMatch(index int) bool {
	texts := []string{
		"语言 跟随系统 简体中文 English 启动时恢复会话 关闭最后一个选项卡时 关闭有活动连接的选项卡前确认",
		"应用主题 浅色 深色 跟随系统 强调色 预览 减少动态效果 窗口半透明 标题栏 玻璃栏",
		"配色方案 跟随应用明暗 固定一种 浅色外观时 深色外观时 配色 终端背景 字体 字号 行高 粗体使用亮色 光标 样式 闪烁 回滚行数",
		"默认本地目录 同时传输数 同名冲突",
		"快捷键 操作 新建会话 关闭选项卡 重新打开关闭的选项卡 切换到 上一个 下一个 显示 隐藏侧栏 终端模式 文件传输模式 拆分窗格 搜索当前页面 设置",
		"私钥 导入 生成 ssh-agent 已保存的密码 添加到 agent 密钥与凭据",
		"关于 版本 开源许可 MyGo Catppuccin JetBrains Mono",
	}
	english := []string{"language system startup restore sessions close last tab active connection confirmation", "app theme light dark system accent preview reduce motion window translucency", "color scheme follow app fixed light dark terminal background font size line height bold bright cursor style blink scrollback", "default local directory concurrent transfers name conflict", "keyboard shortcuts new close reopen select tabs previous next sidebar terminal file mode split search current page settings", "private key import generate ssh-agent saved passwords add to agent credentials", "about version open source license MyGo Catppuccin JetBrains Mono"}
	return a.settingMatches(settingsCategories[index].label, texts[index]+english[index])
}

func (a *app) settingsText(c *ui.Context, text string, size float32, col color.NRGBA) {
	text = localize(c, text)
	query := []rune(strings.ToLower(strings.TrimSpace(a.settingsUI.query)))
	if len(query) == 0 {
		ui.Text(c, localize(c, text)).FontSize(size).TextColor(tokenColor(col))
		return
	}
	var spans []ui.Span
	at := 0
	for _, r := range text {
		ink := tokenColor(col)
		if at < len(query) && unicode.ToLower(r) == query[at] {
			ink = tokenColor(theme.Current().Color.Text.Accent)
			at++
		}
		spans = append(spans, ui.Span{Text: string(r), Color: ink})
	}
	ui.RichText(c, spans...).FontSize(size)
}

func (a *app) settingsGroup(c *ui.Context, title, footnote string, fields []settingField) {
	visible := make([]settingField, 0, len(fields))
	for _, field := range fields {
		if a.settingMatches(field.label, field.description) {
			visible = append(visible, field)
		}
	}
	if len(visible) == 0 {
		return
	}
	t := theme.Current()
	p := t.C.Settings
	ui.Column(c).Key(title).FillWidth().Gap(p.GroupTitleGap).Children(func() {
		ui.Text(c, localize(c, title)).FontSize(p.GroupTitleSize).FontWeight(p.GroupTitleWeight).TextColor(tokenColor(p.GroupTitle))
		ui.Column(c).FillWidth().Radius(p.GroupRadius).Background(tokenColor(p.GroupBg)).Border(p.GroupBorderWidth, tokenColor(p.GroupBorder)).Children(func() {
			for index, field := range visible {
				if index > 0 {
					ui.Row(c).FillWidth().Padding(0, 0, 0, p.RowDividerInset).Children(func() { ui.Box(c).FillWidth().Height(t.Size.Divider).Background(tokenColor(p.RowDivider)) })
				}
				ui.Row(c).Key(field.label).FillWidth().MinHeight(p.RowMinHeight).Padding(p.RowPaddingY, p.RowPaddingX).Gap(t.Space.N16).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N4).Children(func() {
						a.settingsText(c, field.label, p.RowLabelSize, p.RowLabel)
						if field.description != "" {
							a.settingsText(c, field.description, p.RowDescSize, p.RowDesc)
						}
					})
					ui.Row(c).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center).Children(field.control)
				})
			}
		})
		if footnote != "" {
			ui.Text(c, localize(c, footnote)).FontSize(p.GroupFootnoteSize).TextColor(tokenColor(p.GroupFootnote))
		}
	})
}

func settingsRow(c *ui.Context, label string, content func()) {
	t := theme.Current()
	ui.Row(c).FillWidth().Gap(t.Space.N16).AlignItems(ui.Center).Children(func() { ui.Text(c, localize(c, label)).FontSize(t.C.Settings.RowLabelSize).Grow(1); content() })
}

func settingsSegment(c *ui.Context, id string, options []string, current int, change func(int)) {
	p := theme.Current().C.Segmented
	ui.Row(c).Key(id).Padding(p.Padding).Radius(p.Radius).Background(tokenColor(p.Bg)).FocusGroup(ui.Horizontal).Children(func() {
		for index, label := range options {
			item := ui.ButtonBase(c).Key(index).Label(localize(c, label)).Selected(index == current).Height(theme.Current().C.Input.Height-2*p.Padding).
				Padding(0, p.ItemPaddingX).FontSize(p.FontSize).Radius(p.ItemRadius).OnClick(func() { change(index) })
			styleSegment(item, p, index == current).Children(func() { ui.Text(c, localize(c, label)).SingleLine().PassThrough() })
		}
	})
}

func (a *app) preferenceSwitch(c *ui.Context, label string, value *bool) {
	p := theme.Current().C.Switch
	if ui.Switch(c, value).Size(p.W, p.H).Label(localize(c, label)).Changed() {
		a.preferencesChanged()
	}
}

func (a *app) generalSettings(c *ui.Context) {
	a.settingsGroup(c, "语言", "", []settingField{{"语言", "更改后立即生效", func() {
		options := []string{localize(c, "跟随系统"), i18n.Autonym("zh-Hans"), i18n.Autonym("en")}
		tags := []string{"", "zh-Hans", "en"}
		index := slices.Index(tags, a.prefs.Locale)
		if index < 0 {
			index = 0
		}
		selected := options[index]
		if ui.Select(c, &selected, options).Width(theme.Current().C.Settings.ContentMax / 3).Label(localize(c, "语言")).Changed() {
			a.prefs.Locale = tags[slices.Index(options, selected)]
			mygo.App.SetLocale(a.prefs.Locale)
			a.preferencesChanged()
		}
	}}})
	a.settingsGroup(c, "启动与选项卡", "", []settingField{
		{"启动时恢复会话", "重新打开上次退出时的选项卡（不会自动输入密码）", func() { a.preferenceSwitch(c, "启动时恢复会话", &a.prefs.RestoreSessions) }},
		{"关闭最后一个选项卡时", "", func() {
			values := []string{"keep", "closeWindow", "quit"}
			settingsSegment(c, "last-tab-close", []string{"保留窗口", "关闭窗口", "退出应用"}, slices.Index(values, a.prefs.LastTabClose), func(index int) { a.prefs.LastTabClose = values[index]; a.preferencesChanged() })
		}},
		{"关闭有活动连接的选项卡前确认", "", func() {
			a.preferenceSwitch(c, "关闭有活动连接的选项卡前确认", &a.prefs.ConfirmCloseActive)
		}},
	})
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "应用主题", "", []settingField{{"应用主题", "", func() {
		ui.Row(c).Gap(t.Space.N12).Width(t.C.Settings.ContentMax * 2 / 3).Children(func() {
			for index, label := range []string{"浅色", "深色", "跟随系统"} {
				ui.Column(c).Grow(1).Gap(t.Space.N8).Children(func() {
					p := t.C.ThemeCard
					selected := a.appearance == index
					item := ui.ButtonBase(c).Key(index).FillWidth().AspectRatio(p.Aspect).Radius(p.Radius).Clip().Label(localize(c, label)).Selected(selected).
						Border(p.BorderWidth, tokenColor(p.Border)).OnClick(func() { a.setAppearance(index) })
					if selected {
						item.Border(p.SelectedWidth, tokenColor(p.Selected))
						applyShadow(item, p.SelectedShadow)
					}
					item.Draw(func(painter *ui.Painter, r ui.Rect) { drawThemeThumbnail(painter, r, index, t) })
					ui.Row(c).Justify(ui.Center).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
						radio := ui.Box(c).Size(p.Radio, p.Radio).Radius(p.Radio).Border(t.Size.Divider, tokenColor(t.Color.Border.Control))
						if selected {
							radio.Border(p.RadioRing, tokenColor(p.Selected))
						}
						ui.Text(c, localize(c, label)).FontSize(t.Font.Size.Small)
					})
				})
			}
		})
	}}})
	a.settingsGroup(c, "强调色", "", []settingField{{"强调色", a.accentName(), func() {
		p := t.C.AccentSwatch
		ui.Row(c).Gap(p.Gap).AlignItems(ui.Center).Children(func() {
			colors := []color.NRGBA{t.Color.AccentChoices.Blue, t.Color.AccentChoices.Purple, t.Color.AccentChoices.Pink, t.Color.AccentChoices.Red, t.Color.AccentChoices.Orange, t.Color.AccentChoices.Yellow, t.Color.AccentChoices.Green, t.Color.AccentChoices.Graphite}
			values := []string{"system"}
			for _, ink := range colors {
				values = append(values, termtheme.Hex(ink))
			}
			for index, value := range values {
				swatch := ui.ButtonBase(c).Key(value).Size(p.Size, p.Size).Radius(p.Size).Label(localize(c, a.accentLabel(index))).Selected(a.prefs.Accent == value).
					OnClick(func() { a.prefs.Accent = value; a.syncAppearance(); a.preferencesChanged() })
				if index == 0 {
					swatch.Draw(func(painter *ui.Painter, r ui.Rect) {
						for i, ink := range colors {
							slice := r
							slice.X += float32(i) * r.W / float32(len(colors))
							slice.W = r.W / float32(len(colors))
							painter.Fill(slice, tokenColor(ink), 0)
						}
					})
				} else {
					swatch.Background(tokenColor(colors[index-1]))
				}
				if strings.EqualFold(a.prefs.Accent, value) {
					applyShadow(swatch, theme.Shadow{{Spread: p.RingGap, Color: p.RingInset}, {Spread: p.RingGap + p.RingWidth, Color: t.Color.Accent.Default}})
				}
				tooltip(c, swatch, localize(c, a.accentLabel(index)))
			}
		})
	}}, {"预览", "", func() {
		ui.Row(c).Gap(t.Space.N12).AlignItems(ui.Center).Children(func() {
			primaryButton(c, localize(c, "主按钮"))
			settingsSegment(c, "accent-preview", []string{"项目一", "项目二"}, 0, func(int) {})
			on := true
			ui.Box(c).Width(t.Space.N24 * 2).Height(t.C.Progress.Height).Radius(t.C.Progress.Radius).Background(tokenColor(t.C.Progress.Track)).Children(func() {
				ui.Box(c).Width(t.Space.N24).Height(t.C.Progress.Height).Radius(t.C.Progress.Radius).Background(tokenColor(t.C.Progress.Fill))
			})
			ui.Switch(c, &on).Disabled(true).Label(localize(c, "开关预览"))
		})
	}}})
	a.settingsGroup(c, "辅助功能", "关闭应用内动态效果时，系统减少动态效果仍对 MyGo 内建控件生效。", []settingField{
		{"减少动态效果", "减少位移与缩放，保留透明度和颜色过渡", func() {
			values := []string{"system", "on", "off"}
			settingsSegment(c, "reduce-motion", []string{"跟随系统", "始终开启", "关闭"}, slices.Index(values, a.prefs.ReduceMotion), func(index int) { a.prefs.ReduceMotion = values[index]; a.preferencesChanged() })
		}},
		{"窗口半透明", "标题栏、玻璃栏的模糊；系统「降低透明度」开启时自动关闭", func() {
			value := a.prefs.Translucency
			locked := a.reduceTransparency
			if locked {
				value = false
			}
			if ui.Switch(c, &value).Label(localize(c, "窗口半透明")).Disabled(locked).Changed() {
				a.prefs.Translucency = value
				a.preferencesChanged()
			}
		}},
	})
}

func drawThemeThumbnail(p *ui.Painter, r ui.Rect, index int, t *theme.Theme) {
	light, dark := theme.ForAppearance(false), theme.ForAppearance(true)
	sample := light
	if index == 1 {
		sample = dark
	}
	p.Fill(r, tokenColor(sample.C.Window.Bg), t.C.ThemeCard.Radius)
	body := ui.Rect{X: r.X + t.Space.N6, Y: r.Y + t.Space.N12, W: r.W - 2*t.Space.N6, H: r.H - t.Space.N18}
	p.Fill(body, tokenColor(sample.Color.Surface.Card), t.C.Input.Radius)
	sidebar := body
	sidebar.W = body.W / 4
	p.Fill(sidebar, tokenColor(sample.Color.Surface.Control), t.C.Input.Radius)
	if index == 2 {
		right := r
		right.X += r.W / 2
		right.W = r.W / 2
		p.Fill(right, tokenColor(dark.C.Window.Bg), 0)
		right.X += t.Space.N4
		right.Y += t.Space.N12
		right.W -= t.Space.N10
		right.H -= t.Space.N18
		p.Fill(right, tokenColor(dark.Color.Surface.Card), t.C.Input.Radius)
	}
	for i := 0; i < 3; i++ {
		line := ui.Rect{X: body.X + body.W/3, Y: body.Y + t.Space.N8 + float32(i)*t.Space.N8, W: body.W / 2, H: t.Space.N2}
		p.Fill(line, tokenColor(sample.Color.Border.Control), t.Space.N2)
	}
}

func (a *app) accentLabel(index int) string {
	return []string{"跟随系统", "蓝色", "紫色", "粉色", "红色", "橙色", "黄色", "绿色", "石墨"}[index]
}
func (a *app) accentName() string {
	values := []string{"system", "#0a84ff", "#8e5cf7", "#ec4899", "#ff3b30", "#ff9500", "#f5b400", "#30c45a", "#8e8e93"}
	index := slices.Index(values, strings.ToLower(a.prefs.Accent))
	if index < 0 {
		return a.prefs.Accent
	}
	return a.accentLabel(index)
}

func (a *app) terminalSettings(c *ui.Context) {
	if strings.TrimSpace(a.settingsUI.query) == "" {
		a.terminalPreview(c)
	}
	a.terminalColorSettings(c)
	a.terminalTextSettings(c)
}

func (a *app) terminalPreview(c *ui.Context) {
	t := theme.Current()
	scheme := termtheme.Current()
	prefs := a.terminal.prefs
	ui.Column(c).FillWidth().Radius(t.C.Settings.GroupRadius).Border(t.C.Settings.GroupBorderWidth, tokenColor(t.C.Settings.GroupBorder)).Clip().Children(func() {
		ui.Row(c).FillWidth().Padding(t.Space.N10, t.Space.N14).Background(tokenColor(t.Color.Surface.Card)).Children(func() {
			ui.Text(c, localize(c, "预览")).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Semibold).TextColor(tokenColor(t.Color.Text.Muted)).Grow(1)
			ui.Text(c, localize(c, fmt.Sprintf("%s · %s %.1f / %.0f · %s", scheme.Name, prefs.FontFamily[0], prefs.FontSize, prefs.LineHeight, localize(c, []string{"块状光标", "竖线光标", "下划线光标"}[prefs.CursorShape])))).FontSize(t.Font.Size.Footnote).TextColor(tokenColor(t.Color.Text.Tertiary))
		})
		ui.Column(c).FillWidth().Padding(t.Space.N14).Background(tokenColor(termtheme.TerminalBackground(scheme, t.C.Pane.Bg, prefs.Background))).
			Font(strings.Join(prefs.FontFamily, ", ")).FontSize(prefs.FontSize).Children(func() {
			lines := terminalPreviewLines()
			for _, line := range lines {
				ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...).FixedLineHeight(prefs.LineHeight).FillWidth()
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.RichText(c, terminalSpans(terminalLine{{Text: "deploy@prod-web-01", Foreground: terminalIndexed(2)}, {Text: ":/srv/app", Foreground: terminalIndexed(4)}, {Text: "$ "}}, scheme, prefs, 0, 0)...).FixedLineHeight(prefs.LineHeight)
				width := prefs.FontSize / 2
				height := prefs.FontSize
				if prefs.CursorShape == termtheme.CursorBar {
					width = t.Size.Divider
				} else if prefs.CursorShape == termtheme.CursorUnderline {
					height = t.Size.Divider
				}
				shown := float32(1)
				if prefs.CursorBlink && int(c.Now().UnixMilli()/600)%2 != 0 {
					shown = 0
				}
				if prefs.CursorBlink {
					c.After(600 * time.Millisecond)
				}
				ui.Box(c).Size(width, height).Background(tokenColor(scheme.Cursor)).Opacity(shown)
			})
		})
	})
}

func terminalPreviewLines() []terminalLine {
	return []terminalLine{
		{{Text: "deploy@prod-web-01", Foreground: terminalIndexed(2)}, {Text: ":/srv/app", Foreground: terminalIndexed(4)}, {Text: "$ git status -sb"}},
		{{Text: "## main...origin/main", Foreground: terminalIndexed(2)}},
		{{Text: " M ", Foreground: terminalIndexed(1)}, {Text: "config/nginx.conf"}},
		{{Text: "?? ", Foreground: terminalIndexed(1)}, {Text: "scripts/backup.sh"}},
	}
}

func (a *app) terminalColorSettings(c *ui.Context) {
	t := theme.Current()
	if a.settingMatches("配色方案", "方案 跟随应用明暗 固定一种 配色 浅色外观时 深色外观时") {
		ui.Column(c).FillWidth().Gap(t.C.Settings.GroupTitleGap).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, localize(c, "配色方案")).FontSize(t.C.Settings.GroupTitleSize).FontWeight(t.C.Settings.GroupTitleWeight).TextColor(tokenColor(t.C.Settings.GroupTitle)).Grow(1)
				button(c, localize(c, "导入配色…")).Height(t.Size.ControlSm).Disabled(a.terminal.importing).OnClick(a.chooseTerminalTheme)
			})
			ui.Column(c).FillWidth().Radius(t.C.Settings.GroupRadius).Background(tokenColor(t.C.Settings.GroupBg)).Border(t.C.Settings.GroupBorderWidth, tokenColor(t.C.Settings.GroupBorder)).Children(func() {
				ui.Row(c).FillWidth().Padding(t.C.Settings.RowPaddingY, t.C.Settings.RowPaddingX).AlignItems(ui.Center).Gap(t.Space.N12).Children(func() {
					ui.Column(c).Grow(1).Gap(t.Space.N4).Children(func() {
						ui.Text(c, localize(c, "方案")).FontSize(t.C.Settings.RowLabelSize)
						ui.Text(c, localize(c, "跟随：按应用当前明暗，在下方两种配色间自动切换")).FontSize(t.C.Settings.RowDescSize).TextColor(tokenColor(t.C.Settings.RowDesc))
					})
					index := 0
					if a.terminal.mode.Kind == termtheme.FixedScheme {
						index = 1
					}
					settingsSegment(c, "scheme-mode", []string{"跟随应用明暗", "固定一种"}, index, func(index int) {
						a.changeTerminalSettings(func() {
							if index == 0 {
								a.terminal.mode = termtheme.FollowAppTheme()
							} else {
								a.terminal.mode = termtheme.Fixed(termtheme.Current().Name)
							}
						})
					})
				})
				a.schemeGrid(c)
				if a.terminal.mode.Kind == termtheme.FollowApp {
					a.schemeSelectRow(c, "浅色外观时", &a.terminal.followLight)
					a.schemeSelectRow(c, "深色外观时", &a.terminal.followDark)
				} else {
					selected := a.terminal.mode.Name
					a.schemeSelectRow(c, "配色", &selected)
					if selected != a.terminal.mode.Name {
						a.changeTerminalSettings(func() { a.terminal.mode = termtheme.Fixed(selected) })
					}
				}
			})
			ui.Text(c, localize(c, "点击卡片：按配色自身明暗放进浅色或深色槽位。导入支持 Ghostty 主题格式。")).FontSize(t.C.Settings.GroupFootnoteSize).TextColor(tokenColor(t.C.Settings.GroupFootnote))
		})
	}
	a.settingsGroup(c, "背景", "", []settingField{{"终端背景", "配色自带：任何外观都用配色的背景色。与卡片一致：用卡片底色", func() {
		settingsSegment(c, "terminal-bg", []string{"配色自带（默认）", "与卡片一致"}, int(a.terminal.prefs.Background), func(index int) {
			a.changeTerminalSettings(func() { a.terminal.prefs.Background = termtheme.BackgroundSource(index) })
		})
	}}})
}

func (a *app) schemeSelectRow(c *ui.Context, label string, value *string) {
	t := theme.Current()
	ui.Row(c).FillWidth().MinHeight(t.C.Settings.RowMinHeight).Padding(t.C.Settings.RowPaddingY, t.C.Settings.RowPaddingX).
		BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(tokenColor(t.C.Settings.RowDivider)).AlignItems(ui.Center).Children(func() {
		ui.Text(c, localize(c, label)).FontSize(t.C.Settings.RowLabelSize).Grow(1)
		selected := *value
		if a.settingsSchemeSelect(c, label, &selected) {
			a.changeTerminalSettings(func() { *value = selected })
		}
	})
}

func (a *app) schemeGrid(c *ui.Context) {
	t := theme.Current()
	names := []string{termtheme.DefaultLight, termtheme.DefaultDark, "Dracula", "Nord"}
	for _, name := range termtheme.Names() {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	count := len(names)
	if !a.settingsUI.allSchemes && count > 8 {
		names = names[:8]
	}
	ui.Column(c).FillWidth().Padding(t.Space.N6, t.Space.N14, t.Space.N16).Gap(t.Space.N12).Children(func() {
		for start := 0; start < len(names); start += 4 {
			ui.Row(c).FillWidth().Gap(t.Space.N12).Children(func() {
				for index := start; index < start+4; index++ {
					if index < len(names) {
						a.schemeCard(c, names[index])
					} else {
						ui.Box(c).Grow(1)
					}
				}
			})
		}
		if count > 8 {
			label := fmt.Sprintf(localize(c, "全部 %d 种 ▾"), count)
			if a.settingsUI.allSchemes {
				label = "收起 ▴"
			}
			button(c, localize(c, label)).OnClick(func() { a.settingsUI.allSchemes = !a.settingsUI.allSchemes })
		}
	})
}

func (a *app) schemeCard(c *ui.Context, name string) {
	t := theme.Current()
	p := t.C.SchemeCard
	scheme, _ := termtheme.Lookup(name)
	selected := a.terminal.mode.Kind == termtheme.FixedScheme && a.terminal.mode.Name == name
	badge := ""
	if a.terminal.mode.Kind == termtheme.FollowApp {
		if name == a.terminal.followLight {
			selected = true
			badge = "☀ 浅色"
		}
		if name == a.terminal.followDark {
			selected = true
			badge = "☾ 深色"
		}
	}
	item := ui.ButtonBase(c).Key(name).Grow(1).Basis(0).MinWidth(0).Padding(p.Padding).Radius(p.Radius).Label(name).Selected(selected).Background(tokenColor(t.Color.Surface.Card)).TextColor(tokenColor(t.Color.Text.Primary)).
		Border(p.BorderWidth, tokenColor(p.Border)).OnClick(func() {
		a.changeTerminalSettings(func() {
			if a.terminal.mode.Kind == termtheme.FixedScheme {
				a.terminal.mode = termtheme.Fixed(name)
			} else if scheme.IsDark() {
				a.terminal.followDark = name
			} else {
				a.terminal.followLight = name
			}
		})
	})
	if selected {
		item.Border(p.SelectedWidth, tokenColor(p.Selected))
	}
	if _, imported := a.terminal.imported[name]; imported && !termtheme.IsBundled(name) {
		item.ContextMenu(a.importedThemeMenu(c, name))
	}
	item.Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
			ui.Column(c).FillWidth().Height(p.PreviewHeight).Radius(p.PreviewRadius).Padding(t.Space.N6).Background(tokenColor(scheme.Background)).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(p.PreviewFontSize).Clip().Children(func() {
				prefs := a.terminal.prefs
				prefs.FontSize = p.PreviewFontSize
				prefs.LineHeight = p.PreviewLineHeight
				lines := []terminalLine{{{Text: "deploy@web", Foreground: terminalIndexed(2)}, {Text: ":~$ ls"}}, {{Text: "src  ", Foreground: terminalIndexed(4)}, {Text: "bin  ", Foreground: terminalIndexed(6)}, {Text: "a.js", Foreground: terminalIndexed(5)}}, {{Text: "WARN ", Foreground: terminalIndexed(3)}, {Text: "ERR ", Foreground: terminalIndexed(1)}, {Text: "ok"}}}
				for _, line := range lines {
					ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...).FixedLineHeight(p.PreviewLineHeight).SingleLine().PassThrough()
				}
			})
			ui.Row(c).FillWidth().Gap(p.StripeGap).Children(func() {
				for _, ink := range scheme.ANSI[:8] {
					ui.Box(c).Grow(1).Height(p.StripeHeight).Radius(p.StripeRadius).Background(tokenColor(ink)).PassThrough()
				}
			})
			ui.Row(c).FillWidth().Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).Grow(1).MinWidth(0).FontSize(p.NameSize).FontWeight(p.NameWeight).SingleLine().PassThrough()
				if badge != "" {
					ui.Text(c, localize(c, badge)).FontSize(p.BadgeSize).FontWeight(p.BadgeWeight).TextColor(tokenColor(p.BadgeText)).Background(tokenColor(p.BadgeBg)).Radius(t.Radius.Chip).PassThrough()
				}
			})
		})
	})
}

func (a *app) terminalTextSettings(c *ui.Context) {
	t := theme.Current()
	if a.settingsUI.fonts == nil {
		for _, family := range ui.FontFamilies() {
			if family.Monospaced {
				a.settingsUI.fonts = append(a.settingsUI.fonts, family.Name)
			}
		}
		if index := slices.Index(a.settingsUI.fonts, "JetBrains Mono"); index > 0 {
			a.settingsUI.fonts = append([]string{"JetBrains Mono"}, append(a.settingsUI.fonts[:index], a.settingsUI.fonts[index+1:]...)...)
		}
	}
	a.settingsGroup(c, "字体", "", []settingField{
		{"字体", "", func() {
			selected := a.terminal.prefs.FontFamily[0]
			if ui.Select(c, &selected, a.settingsUI.fonts).Width(t.C.Settings.SelectWidth).Label(localize(c, "字体")).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.FontFamily = []string{selected, "monospace"} })
			}
		}},
		{"字号", "", func() {
			value := float64(a.terminal.prefs.FontSize)
			if settingsNumberInput(c, "字号", &value, 9, 24, 0.5, t.C.Settings.NumberWidth, false) {
				a.changeTerminalSettings(func() {
					a.terminal.prefs.FontSize = float32(value)
					if !a.prefs.LineHeightEdited {
						a.terminal.prefs.LineHeight = float32(math.Round(value * 19 / 12.5))
					}
					a.terminal.prefs.LineHeight = max(a.terminal.prefs.LineHeight, float32(math.Ceil(value*1.2)))
				})
			}
			ui.Text(c, localize(c, "px")).FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Tertiary))
		}},
		{"行高", "", func() {
			value := float64(a.terminal.prefs.LineHeight)
			if settingsNumberInput(c, "行高", &value, math.Ceil(float64(a.terminal.prefs.FontSize)*1.2), 40, 1, t.C.Settings.NumberWidth, false) {
				a.changeTerminalSettings(func() { a.terminal.prefs.LineHeight = float32(value); a.prefs.LineHeightEdited = true })
			}
			ui.Text(c, localize(c, "px")).FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Tertiary))
		}},
		{"粗体使用亮色", "ANSI 0–7 粗体改用 8–15 亮色（xterm 行为）", func() {
			value := a.terminal.prefs.BoldIsBright
			if ui.Switch(c, &value).Label(localize(c, "粗体使用亮色")).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.BoldIsBright = value })
			}
		}},
	})
	a.settingsGroup(c, "光标", "", []settingField{
		{"样式", "", func() {
			settingsSegment(c, "cursor-shape", []string{"▉ 块", "▏竖线", "▁ 下划线"}, int(a.terminal.prefs.CursorShape), func(index int) {
				a.changeTerminalSettings(func() { a.terminal.prefs.CursorShape = termtheme.CursorShape(index) })
			})
		}},
		{"闪烁", "", func() {
			value := a.terminal.prefs.CursorBlink
			if ui.Switch(c, &value).Label(localize(c, "光标闪烁")).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.CursorBlink = value })
			}
		}},
	})
	a.settingsGroup(c, "回滚", "超过后丢弃最早的输出。", []settingField{{"回滚行数", "行 · 每个会话", func() {
		value := float64(a.prefs.ScrollbackLines)
		if settingsNumberInput(c, "回滚行数", &value, 1000, 200000, 1000, t.C.Settings.ScrollbackWidth, true) {
			a.prefs.ScrollbackLines = int(value)
			a.preferencesChanged()
		}
	}}})
}

func (a *app) terminalNumberSetting(c *ui.Context, label string, destination *float32, lo, hi float64) {
	settingsRow(c, label, func() {
		value := float64(*destination)
		if settingsNumberInput(c, label, &value, lo, hi, 0.5, theme.Current().C.Settings.NumberWidth, false) {
			a.changeTerminalSettings(func() { *destination = float32(value) })
		}
	})
}

func (a *app) fileSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "目录", "", []settingField{{"默认本地目录", "展开本地面板时打开的目录", func() {
		if textInput(c, &a.prefs.DefaultLocalDir).Width(t.C.Settings.SelectWidth).Font(strings.Join(t.Font.Family.Mono, ", ")).Label(localize(c, "默认本地目录")).Changed() {
			a.preferencesChanged()
		}
		keyedIconButton(c, "default-local-folder", "选择本地目录", filesIcon).OnClick(a.chooseDefaultLocalDirectory)
	}}})
	a.settingsGroup(c, "传输", "", []settingField{
		{"同时传输数", "", func() {
			value := float64(a.prefs.TransferParallel)
			if settingsNumberInput(c, "同时传输数", &value, 1, 8, 1, t.C.Settings.NumberWidth, false) {
				a.prefs.TransferParallel = int(value)
				a.preferencesChanged()
			}
		}},
		{"同名冲突", "", func() {
			values := []string{"ask", "overwrite", "skip", "rename"}
			settingsSegment(c, "conflict-policy", []string{"询问", "覆盖", "跳过", "重命名"}, slices.Index(values, a.prefs.ConflictPolicy), func(index int) { a.prefs.ConflictPolicy = values[index]; a.preferencesChanged() })
		}},
	})
}

func (a *app) chooseDefaultLocalDirectory() {
	window := a.window
	if window == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "默认本地目录", Directory: true})
		window.Update(func() {
			if err != nil {
				if a.settingsUI.open {
					a.settingsUI.notice = err.Error()
				} else {
					a.files.notice = "选择本地目录失败：" + err.Error()
				}
				return
			}
			if len(paths) > 0 {
				a.prefs.DefaultLocalDir = paths[0]
				a.preferencesChanged()
			}
		})
	}()
}
