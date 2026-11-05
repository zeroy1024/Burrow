package desktop

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
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
	{"about", "关于", "Burrow 与开源许可", infoIcon},
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
		uikit.Card(c).Width(p.NavWidth).Shrink(0).FillHeight().Children(func() { a.settingsSidebar(c, 1) })
		a.settings(c)
	})
}

func (a *app) settingsSidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	ui.Column(c).Fill().Children(func() {
		uikit.SiderHeader(c, uikit.SiderHeaderProps{Title: localize(c, "设置"), HideAdd: true})
		a.sidebarSearch(c.Key("settings-search"), &a.settingsUI.query, &a.settingsUI.search,
			localize(c, "搜索设置"), localize(c, "搜索设置"))
		ui.Column(c).FillWidth().Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Gap(t.C.Catalog.RowGap).Children(func() {
			for index, category := range settingsCategories {
				uikit.SettingsNavItem(c, uikit.SettingsNavItemProps{
					Key: category.id, Label: localize(c, category.label), Icon: category.icon, Selected: a.settingsCategory == index,
				}).OnClick(func() { a.selectSettingsCategory(index) })
			}
		})
		ui.Box(c).Grow(1)
		ui.Row(c).FillWidth().BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(uikit.Color(t.Color.Border.Subtle)).
			Padding(t.Space.N10, t.Space.N16).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			uikit.Icon(c, infoIcon, uikit.IconProps{Tone: uikit.ToneTertiary})
			uikit.Text(c, "Burrow 0.1.1", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		})
	})
}

func (a *app) settings(c *ui.Context) {
	if a.settingsUI.recording != "" {
		searching := strings.TrimSpace(a.settingsUI.query) != ""
		visible := !searching && a.settingsCategory == 4 || searching && a.settingsCategoryHasMatch(4)
		if visible {
			visible = slices.ContainsFunc(defaultShortcutActions, func(action shortcutAction) bool {
				return action.id == a.settingsUI.recording && a.settingMatches(action.label, action.group+"快捷键")
			})
		}
		if !visible {
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
		}
	}
	t := theme.Current()
	p := t.C.Settings
	uikit.Card(c).Key("settings-content").Grow(1).MinWidth(0).FillHeight().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
			BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).AlignItems(ui.Center).Children(func() {
			category := settingsCategories[a.settingsCategory]
			ui.Column(c).Grow(1).Gap(t.Space.N4).Children(func() {
				title, subtitle := category.label, category.subtitle
				if strings.TrimSpace(a.settingsUI.query) != "" {
					title, subtitle = "搜索结果", "匹配设置名称与说明 · 修改立即生效"
				}
				uikit.Text(c, localize(c, title), uikit.TextProps{Style: uikit.TextTitle})
				uikit.Text(c, localize(c, subtitle), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
			})
		})
		viewport := ui.Box(c).Grow(1).MinHeight(0).FillWidth()
		viewport.Children(func() {
			scroll := &a.settingsUI.scroll[a.settingsCategory]
			contentScroll := ui.Scroll(c).Fill().TrackScroll(scroll).Children(func() {
				ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
					content := ui.Column(c).Key(a.settingsCategory).FillWidth().MaxWidth(p.ContentMax+2*p.ContentPaddingX).
						Padding(p.ContentPaddingY, p.ContentPaddingX).Gap(p.GroupGap)
					content.Children(func() {
						if a.settingsUI.notice != "" {
							uikit.Banner(c, localize(c, a.settingsUI.notice), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
						}
						if a.terminalError != nil {
							uikit.Banner(c, localize(c, a.terminalError.Error()), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
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
								uikit.EmptyMessage(c, localize(c, "没有匹配的设置"), uikit.EmptySettingsSearch)
							}
						} else {
							a.settingsCategoryContent(c, a.settingsCategory)
						}
					})
				})
			})
			uikit.ScrollFade(c, uikit.EdgeBottom, uikit.ScrollFadeProps{
				Height: p.FadeHeight, Gradient: true, Background: uikit.Color(t.Color.Surface.Card),
			})
			uikit.Scrollbars(c, viewport, contentScroll, scroll, uikit.ScrollbarProps{Label: "设置内容滚动条"})
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
		"语言 跟随系统 简体中文 English 启动时恢复会话 关闭最后一个选项卡时 关闭活动连接前确认",
		"应用主题 浅色 深色 跟随系统 强调色 预览 减少动态效果 窗口半透明 全局背景 毛玻璃 标题栏 玻璃栏 macOS",
		"配色方案 跟随应用明暗 固定一种 浅色外观时 深色外观时 配色 终端背景 字体 字号 行高 光标 样式 闪烁 回滚行数",
		"默认本地目录 同时传输数 同名冲突",
		"快捷键 操作 新建会话 关闭选项卡 重新打开关闭的选项卡 切换到 上一个 下一个 显示 隐藏侧栏 终端模式 文件传输模式 拆分窗格 搜索当前页面 设置",
		"私钥 导入 生成 ssh-agent 已保存的密码 添加到 agent 密钥与凭据",
		"关于 版本 开源许可 MyGo Catppuccin JetBrains Mono",
	}
	english := []string{"language system startup restore sessions close last tab active connection confirmation", "app theme light dark system accent preview reduce motion window translucency global background blur macOS", "color scheme follow app fixed light dark terminal background font size line height cursor style blink scrollback", "default local directory concurrent transfers name conflict", "keyboard shortcuts new close reopen select tabs previous next sidebar terminal file mode split search current page settings", "private key import generate ssh-agent saved passwords add to agent credentials", "about version open source license MyGo Catppuccin JetBrains Mono"}
	return (index == 5 && a.securitySettingsMatch()) || a.settingMatches(settingsCategories[index].label, texts[index]+english[index])
}

func (a *app) settingsText(c *ui.Context, text string, props uikit.TextProps, size float32) {
	text = localize(c, text)
	query := []rune(strings.ToLower(strings.TrimSpace(a.settingsUI.query)))
	if len(query) == 0 {
		uikit.Text(c, text, props).FontSize(size)
		return
	}
	font, col := uikit.TextFont(props.Style), uikit.ToneColor(props.Tone)
	font.Size = size
	var spans []ui.Span
	at := 0
	for _, r := range text {
		ink := col
		if at < len(query) && unicode.ToLower(r) == query[at] {
			ink = uikit.Color(theme.Current().Color.Text.Accent)
			at++
		}
		spans = append(spans, ui.Span{Text: string(r), Color: ink})
	}
	ui.RichText(c, spans...).Font(font.Family).FontSize(font.Size).FontWeight(font.Weight)
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
	uikit.SettingsGroup(c, uikit.SettingsGroupProps{Key: title, Title: localize(c, title), Footnote: localize(c, footnote)}, func() {
		for index, field := range visible {
			uikit.SettingsRow(c, uikit.SettingsRowProps{
				Key: field.label, Gap: t.Space.N16, Title: localize(c, field.label), Detail: localize(c, field.description),
				Divider: index > 0,
				Text: func() {
					a.settingsText(c, field.label, uikit.TextProps{}, p.RowLabelSize)
					if field.description != "" {
						a.settingsText(c, field.description, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}, p.RowDescSize)
					}
				},
				Control: func() {
					ui.Row(c).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center).Children(field.control)
				},
			})
		}
	})
}

func settingsRow(c *ui.Context, label string, content func()) {
	uikit.SettingsRow(c, uikit.SettingsRowProps{
		Gap: theme.Current().Space.N16, Inline: true, Title: localize(c, label), Control: content,
	})
}

func settingsSegment(c *ui.Context, id, label string, options []string, current int, change func(int)) {
	items := make([]uikit.SegItem, len(options))
	for index, label := range options {
		items[index] = uikit.SegItem{Label: localize(c, label)}
	}
	if uikit.Segmented(c.Key(id), &current, items, uikit.SegmentedProps{Label: localize(c, label)}).Changed() {
		change(current)
	}
}

func (a *app) preferenceSwitch(c *ui.Context, label string, value *bool) {
	p := theme.Current().C.Switch
	if uikit.Switch(c, value, uikit.ToggleProps{Label: localize(c, label), Motion: uikit.MotionOverride(a.reduceMotion(c))}).Size(p.W, p.H).Label(localize(c, label)).Changed() {
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
		if uikit.Select(c, &selected, uikit.StringOptions(options), uikit.SelectProps{Label: localize(c, "语言")}).Width(theme.Current().C.Settings.ContentMax / 3).Label(localize(c, "语言")).Changed() {
			a.prefs.Locale = tags[slices.Index(options, selected)]
			mygo.App.SetLocale(a.prefs.Locale)
			a.preferencesChanged()
		}
	}}})
	a.settingsGroup(c, "启动与选项卡", "", []settingField{
		{"启动时恢复会话", "重新打开上次退出时的选项卡（不会自动输入密码）", func() { a.preferenceSwitch(c, "启动时恢复会话", &a.prefs.RestoreSessions) }},
		{"关闭最后一个选项卡时", "", func() {
			values := []string{"keep", "closeWindow", "quit"}
			settingsSegment(c, "last-tab-close", "关闭最后一个选项卡时", []string{"保留窗口", "关闭窗口", "退出应用"}, slices.Index(values, a.prefs.LastTabClose), func(index int) { a.prefs.LastTabClose = values[index]; a.preferencesChanged() })
		}},
		{"关闭活动连接前确认", "", func() {
			a.preferenceSwitch(c, "关闭活动连接前确认", &a.prefs.ConfirmCloseActive)
		}},
	})
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "应用主题", "", []settingField{{"应用主题", "", func() {
		ui.Row(c).Gap(t.Space.N12).Width(t.C.Settings.ContentMax * 2 / 3).Children(func() {
			choice := a.appearance
			for index, label := range []string{"浅色", "深色", "跟随系统"} {
				uikit.ThemeCard(c, uikit.ThemeCardProps{Key: index, Label: localize(c, label), Appearance: index,
					Selected: a.appearance == index, Choice: &choice}).OnClick(func() { a.setAppearance(index) })
			}
			if choice != a.appearance {
				a.setAppearance(choice)
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
				swatches := colors
				if index > 0 {
					swatches = colors[index-1 : index]
				}
				uikit.AccentSwatch(c, uikit.AccentSwatchProps{
					Key: value, Label: localize(c, a.accentLabel(index)), Selected: strings.EqualFold(a.prefs.Accent, value),
					Colors: swatches, SystemGradient: index == 0,
				}).
					OnClick(func() { a.prefs.Accent = value; a.syncAppearance(); a.preferencesChanged() })
			}
		})
	}}, {"预览", "", func() {
		ui.Row(c).Gap(t.Space.N12).AlignItems(ui.Center).Children(func() {
			uikit.Button(c, localize(c, "主按钮"), uikit.ButtonProps{Variant: uikit.ButtonPrimary})
			settingsSegment(c, "accent-preview", "预览", []string{"项目一", "项目二"}, 0, func(int) {})
			on := true
			uikit.Progress(c.Key("accent-progress-preview"), uikit.ProgressProps{
				Value: 0.5, Height: t.C.Progress.Height, Track: t.C.Progress.Track, Fill: t.C.Progress.Fill,
			}).Width(t.Space.N24 * 2)
			uikit.Switch(c, &on, uikit.ToggleProps{Label: localize(c, "开关预览"), Disabled: true, Motion: uikit.MotionOverride(a.reduceMotion(c))})
		})
	}}})
	a.settingsGroup(c, "辅助功能", "关闭应用内动态效果时，系统减少动态效果仍对 MyGo 内建控件生效。", []settingField{
		{"减少动态效果", "减少位移与缩放，保留透明度和颜色过渡", func() {
			values := []string{"system", "on", "off"}
			settingsSegment(c, "reduce-motion", "减少动态效果", []string{"跟随系统", "始终开启", "关闭"}, slices.Index(values, a.prefs.ReduceMotion), func(index int) { a.prefs.ReduceMotion = values[index]; a.preferencesChanged() })
		}},
		{"窗口半透明", "macOS 全局毛玻璃及栏内模糊；系统「降低透明度」开启时自动关闭", func() {
			value := a.prefs.Translucency
			locked := a.reduceTransparency
			if locked {
				value = false
			}
			if uikit.Switch(c, &value, uikit.ToggleProps{Label: localize(c, "窗口半透明"), Disabled: locked, Motion: uikit.MotionOverride(a.reduceMotion(c))}).Changed() {
				a.prefs.Translucency = value
				a.preferencesChanged()
			}
		}},
	})
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
	colors := uikit.ResolveTerminalColors(scheme, prefs.Background)
	ui.Column(c).FillWidth().Radius(t.C.Settings.GroupRadius).Border(t.C.Settings.GroupBorderWidth, uikit.Color(t.C.Settings.GroupBorder)).Clip().Children(func() {
		ui.Row(c).FillWidth().Padding(t.Space.N10, t.Space.N14).Background(uikit.Color(t.Color.Surface.Card)).Children(func() {
			uikit.Text(c, localize(c, "预览"), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneMuted}).FontWeight(t.Font.Weight.Semibold).Grow(1)
			uikit.Text(c, localize(c, fmt.Sprintf("%s · %s %.1f / %.0f · %s", scheme.Name, prefs.FontFamily[0], prefs.FontSize, prefs.LineHeight, localize(c, []string{"块状光标", "竖线光标", "下划线光标"}[prefs.CursorShape]))), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
		})
		ui.Column(c).FillWidth().Padding(t.Space.N14).Background(uikit.Color(colors.Background)).
			TextColor(uikit.Color(colors.Foreground)).SelectionColor(uikit.Color(colors.Selection)).
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
				blink := prefs.CursorBlink && !a.reduceMotion(c)
				if blink && c.Now().UnixNano()/int64(termtheme.CursorBlinkInterval)%2 != 0 {
					shown = 0
				}
				if blink {
					c.After(termtheme.CursorBlinkInterval)
				}
				ui.Box(c).Size(width, height).Background(uikit.Color(scheme.Cursor)).Opacity(shown)
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
				uikit.Text(c, localize(c, "配色方案"), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).FontWeight(t.C.Settings.GroupTitleWeight).Grow(1)
				uikit.Button(c, localize(c, "导入配色…"), uikit.ButtonProps{Size: uikit.SizeSm, Disabled: a.terminal.importing}).OnClick(a.chooseTerminalTheme)
			})
			uikit.SettingsGroupBody(c).Children(func() {
				uikit.SettingsRow(c, uikit.SettingsRowProps{Gap: t.Space.N12}).MinHeight(0).Children(func() {
					ui.Column(c).Grow(1).Gap(t.Space.N4).Children(func() {
						uikit.Text(c, localize(c, "方案"), uikit.TextProps{}).FontSize(t.C.Settings.RowLabelSize)
						uikit.Text(c, localize(c, "跟随：按应用当前明暗，在下方两种配色间自动切换"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
					})
					index := 0
					if a.terminal.mode.Kind == termtheme.FixedScheme {
						index = 1
					}
					settingsSegment(c, "scheme-mode", "配色方案模式", []string{"跟随应用明暗", "固定一种"}, index, func(index int) {
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
			uikit.Text(c, localize(c, "点击卡片：按配色自身明暗放进浅色或深色槽位。导入支持 Ghostty 主题格式。"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
		})
	}
	a.settingsGroup(c, "背景", "", []settingField{{"终端背景", "配色自带：任何外观都用配色的背景色。与卡片一致：用卡片底色", func() {
		settingsSegment(c, "terminal-bg", "终端背景", []string{"配色自带（默认）", "与卡片一致"}, int(a.terminal.prefs.Background), func(index int) {
			a.changeTerminalSettings(func() { a.terminal.prefs.Background = termtheme.BackgroundSource(index) })
		})
	}}})
}

func (a *app) schemeSelectRow(c *ui.Context, label string, value *string) {
	t := theme.Current()
	uikit.SettingsRow(c, uikit.SettingsRowProps{}).
		BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(uikit.Color(t.C.Settings.RowDivider)).AlignItems(ui.Center).Children(func() {
		uikit.Text(c, localize(c, label), uikit.TextProps{}).FontSize(t.C.Settings.RowLabelSize).Grow(1)
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
			uikit.Button(c, localize(c, label), uikit.ButtonProps{}).OnClick(func() { a.settingsUI.allSchemes = !a.settingsUI.allSchemes })
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
	item := uikit.SchemeCard(c, uikit.SchemeCardProps{
		Name: name, Badge: localize(c, badge), Selected: selected, Background: scheme.Background, Colors: scheme.ANSI[:8],
	}, func() {
		prefs := a.terminal.prefs
		prefs.FontSize = p.PreviewFontSize
		prefs.LineHeight = p.PreviewLineHeight
		lines := []terminalLine{{{Text: "deploy@web", Foreground: terminalIndexed(2)}, {Text: ":~$ ls"}}, {{Text: "src  ", Foreground: terminalIndexed(4)}, {Text: "bin  ", Foreground: terminalIndexed(6)}, {Text: "a.js", Foreground: terminalIndexed(5)}}, {{Text: "WARN ", Foreground: terminalIndexed(3)}, {Text: "ERR ", Foreground: terminalIndexed(1)}, {Text: "ok"}}}
		for _, line := range lines {
			ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...).FixedLineHeight(p.PreviewLineHeight).SingleLine().PassThrough()
		}
	}).OnClick(func() {
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
	if _, imported := a.terminal.imported[name]; imported && !termtheme.IsBundled(name) {
		item.ContextMenu(a.importedThemeMenu(c, name))
	}
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
			if uikit.Select(c, &selected, uikit.StringOptions(a.settingsUI.fonts), uikit.SelectProps{Label: localize(c, "字体")}).Width(t.C.Settings.SelectWidth).Label(localize(c, "字体")).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.FontFamily = []string{selected, "monospace"} })
			}
		}},
		{"字号", "", func() {
			value := float64(a.terminal.prefs.FontSize)
			if settingsNumberInput(c, "字号", &value, 9, 24, 0.5, t.C.Settings.NumberWidth, false) {
				a.changeTerminalSettings(func() {
					a.terminal.prefs.FontSize = float32(value)
					if !a.prefs.LineHeightEdited {
						defaults := termtheme.DefaultPrefs()
						a.terminal.prefs.LineHeight = float32(math.Round(
							value * float64(defaults.LineHeight) / float64(defaults.FontSize),
						))
					}
					a.terminal.prefs.LineHeight = max(a.terminal.prefs.LineHeight, float32(math.Ceil(value*1.2)))
				})
			}
			uikit.Text(c, localize(c, "px"), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		}},
		{"行高", "", func() {
			value := float64(a.terminal.prefs.LineHeight)
			if settingsNumberInput(c, "行高", &value, math.Ceil(float64(a.terminal.prefs.FontSize)*1.2), 40, 1, t.C.Settings.NumberWidth, false) {
				a.changeTerminalSettings(func() { a.terminal.prefs.LineHeight = float32(value); a.prefs.LineHeightEdited = true })
			}
			uikit.Text(c, localize(c, "px"), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		}},
	})
	a.settingsGroup(c, "光标", "", []settingField{
		{"样式", "", func() {
			settingsSegment(c, "cursor-shape", "光标样式", []string{"▉ 块", "▏竖线", "▁ 下划线"}, int(a.terminal.prefs.CursorShape), func(index int) {
				a.changeTerminalSettings(func() { a.terminal.prefs.CursorShape = termtheme.CursorShape(index) })
			})
		}},
		{"闪烁", "", func() {
			value := a.terminal.prefs.CursorBlink
			if uikit.Switch(c, &value, uikit.ToggleProps{Label: localize(c, "光标闪烁"), Motion: uikit.MotionOverride(a.reduceMotion(c))}).Changed() {
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
		if uikit.Input(c, &a.prefs.DefaultLocalDir, uikit.InputProps{Mono: true, Label: localize(c, "默认本地目录")}).Width(t.C.Settings.SelectWidth).Changed() {
			a.preferencesChanged()
		}
		uikit.IconButton(c, filesIcon, uikit.IconButtonProps{Key: "default-local-folder", Label: "选择本地目录"}).OnClick(a.chooseDefaultLocalDirectory)
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
			settingsSegment(c, "conflict-policy", "同名冲突", []string{"询问", "覆盖", "跳过", "重命名"}, slices.Index(values, a.prefs.ConflictPolicy), func(index int) { a.prefs.ConflictPolicy = values[index]; a.preferencesChanged() })
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
					a.files.notice = &uikit.ToastProps{
						ID: "选择本地目录失败：" + err.Error(), Tone: uikit.ToneDanger,
						Title: "选择本地目录失败", Detail: err.Error(),
					}
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
