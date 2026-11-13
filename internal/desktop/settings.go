package desktop

import (
	"fmt"
	"image/color"
	"math"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type settingsCategory struct {
	id, label, subtitle string
	icon                *ui.SVG
}

func settingsCategories() []settingsCategory {
	return []settingsCategory{
		{"general", locales.MessageGeneral(), locales.MessageLanguageStartupAndTabs(), settingsIcon},
		{"appearance", locales.MessageAppearance(), locales.MessageThemeAccentAndMotion(), paletteIcon},
		{"terminal", locales.MessageTerminal(), locales.MessageColorsBackgroundFontAndCursorChangesApplyImmediately(), terminalIcon},
		{"files", locales.FilesTitle(), locales.MessageDirectoriesTransfersAndNameConflicts(), filesIcon},
		{"shortcuts", locales.MessageKeyboardShortcuts(), locales.MessageClickAShortcutToRecordEscCancelsDeleteClears(), textCursorIcon},
		{"keys", locales.MessageKeysCredentials(), locales.MessagePrivateKeysSshAgentAndSavedPasswords(), lockIcon},
		{"about", locales.AboutTitle(), locales.MessageBurrowAndOpenSourceLicenses(), infoIcon},
	}
}

type settingField struct {
	id                       any
	label, description, help string
	control                  func()
}

func (a *app) settingsPage(c *ui.Context) {
	if !a.settingsUI.open {
		return
	}
	t := theme.Current()
	p := t.C.Settings
	ui.Row(c).Key("settings-page").Fill().Gap(t.Space.N8).Children(func() {
		uikit.Card(c).Width(p.NavWidth).Shrink(0).FillHeight().Children(func() { a.settingsSidebar(c, 1) })
		a.settings(c)
	})
}

func (a *app) settingsSidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	ui.Column(c).Fill().Children(func() {
		uikit.SiderHeader(c, uikit.SiderHeaderProps{Title: locales.SettingsTitle(), HideAdd: true})
		a.sidebarSearch(c.Key("settings-search"), &a.settingsUI.query, &a.settingsUI.search,
			locales.MessageSearchSettings(), locales.MessageSearchSettings())
		categories := settingsCategories()
		navList := ui.Column(c).Key("settings-nav-list").FillWidth().Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Gap(t.C.Catalog.RowGap)
		navLayout := navList.LayoutBounds()
		staticPill := navLayout.W <= 0 || navLayout.H <= 0
		items := make([]ui.Element, len(categories))
		navList.Children(func() {
			for index, category := range categories {
				items[index] = uikit.SettingsNavItem(c, uikit.SettingsNavItemProps{
					Key: category.id, Label: category.label, Icon: category.icon,
					Selected: a.settingsCategory == index, StaticPill: staticPill,
				}).OnClick(func() { a.selectSettingsCategory(index) })
			}
		})
		uikit.AnimateSettingsNavPill(c, navList, items, a.settingsCategory, a.reduceMotion(c))
		ui.Box(c).Grow(1)
		ui.Row(c).FillWidth().BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(uikit.Color(t.Color.Border.Subtle)).
			Padding(t.Space.N10, t.Space.N16).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			uikit.Icon(c, infoIcon, uikit.IconProps{Tone: uikit.ToneTertiary})
			uikit.Text(c, "Burrow "+appVersion(), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		})
	})
}

func (a *app) settings(c *ui.Context) {
	if a.settingsUI.recording != "" {
		searching := strings.TrimSpace(a.settingsUI.query) != ""
		visible := !searching && a.settingsCategory == 4 || searching && a.settingsCategoryHasMatch(4)
		if visible {
			visible = slices.ContainsFunc(defaultShortcutActions(), func(action shortcutAction) bool {
				return action.id == a.settingsUI.recording && a.settingMatches(
					action.label,
					action.group+locales.MessageKeyboardShortcuts(),
				)
			})
		}
		if !visible {
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
		}
	}
	t := theme.Current()
	p := t.C.Settings
	searching := strings.TrimSpace(a.settingsUI.query) != ""
	var switchTransition ui.ElementTransition
	if !a.reduceMotion(c) {
		enter := ui.Motion{Opacity: 1, Y: p.SwitchY}
		anim := ui.Animation{Spring: uikit.Spring(p.Enter)}
		switchTransition = ui.ElementTransition{
			Animation: &anim,
			Enter:     &enter,
		}
	}
	stageKey := any(a.settingsCategory)
	if searching {
		stageKey = "search"
	}
	uikit.Card(c).Key("settings-content").Grow(1).MinWidth(0).FillHeight().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
			BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).AlignItems(ui.Center).Children(func() {
			category := settingsCategories()[a.settingsCategory]
			ui.Box(c).Key("settings-header-stage").Grow(1).Children(func() {
				ui.Column(c).Key(stageKey).FillWidth().Gap(t.Space.N4).Transition(switchTransition).Children(func() {
					title, subtitle := category.label, category.subtitle
					if searching {
						title, subtitle = locales.MessageSearchResults(), locales.MessageMatchingSettingNamesAndDescriptionsChangesApplyImmediately()
					}
					uikit.Text(c, title, uikit.TextProps{Style: uikit.TextTitle})
					uikit.Text(c, subtitle, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
				})
			})
		})
		viewport := ui.Box(c).Grow(1).MinHeight(0).FillWidth()
		viewport.Children(func() {
			scroll := &a.settingsUI.scroll[a.settingsCategory]
			contentScroll := ui.Scroll(c).Fill().TrackScroll(scroll).Children(func() {
				ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Box(c).Key("settings-content-stage").FillWidth().MaxWidth(p.ContentMax + 2*p.ContentPaddingX).Children(func() {
						content := ui.Column(c).Key(stageKey).FillWidth().
							Padding(p.ContentPaddingY, p.ContentPaddingX).Gap(p.GroupGap).Transition(switchTransition)
						content.Children(func() {
							if !a.settingsUI.notice.Empty() {
								uikit.Banner(c, a.settingsUI.notice.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
							}
							if a.terminalError != nil {
								uikit.Banner(c, a.terminalError.Error(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
							}
							if searching {
								count := 0
								for index := range settingsCategories() {
									if a.settingsCategoryHasMatch(index) {
										a.settingsCategoryContent(c, index)
										count++
									}
								}
								if count == 0 {
									uikit.EmptyMessage(c, locales.MessageNoMatchingSettings(), uikit.EmptySettingsSearch)
								}
							} else {
								a.settingsCategoryContent(c, a.settingsCategory)
							}
						})
					})
				})
			})
			uikit.ScrollFade(c, uikit.EdgeBottom, uikit.ScrollFadeProps{
				Height: p.FadeHeight, Gradient: true, Background: uikit.Color(t.Color.Surface.Card),
			})
			uikit.Scrollbars(
				c,
				viewport,
				contentScroll,
				scroll,
				uikit.ScrollbarProps{Label: locales.MessageSettingsContentScrollbar()},
			)
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
	needle := []rune(query)
	at := 0
	for _, r := range strings.ToLower(label + " " + description) {
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
		"语言 跟随系统 简体中文 English 启动时恢复会话 关闭最后一个选项卡时 关闭活动连接前确认 自动显示 AI 会话 agent MCP 后台运行 关闭主窗口 菜单栏 托盘 程序坞 隐藏 Dock Command Q ⌘Q",
		"应用主题 浅色 深色 跟随系统 强调色 预览 减少动态效果 窗口半透明 全局背景 毛玻璃 标题栏 玻璃栏 macOS",
		"配色方案 跟随应用明暗 固定一种 浅色外观时 深色外观时 配色 终端背景 字体 字号 行高 光标 样式 闪烁 回滚行数",
		"默认本地目录 同时传输数 同名冲突",
		"快捷键 操作 新建会话 关闭选项卡 重新打开关闭的选项卡 切换到 上一个 下一个 显示 隐藏侧栏 终端模式 文件传输模式 拆分窗格 搜索当前页面 设置",
		"私钥 导入 生成 ssh-agent 已保存的密码 添加到 agent 密钥与凭据",
		"关于 版本 GitHub 项目 地址 仓库 zeroy1024 Burrow 软件更新 检查更新 自动检查 更新渠道 正式版 预览版 系统代理 GitHub 镜像 开源许可 MyGo Catppuccin JetBrains Mono",
	}
	english := []string{"language system startup restore sessions close last tab active connection confirmation background main window menu bar tray hide dock icon command q cmd q", "app theme light dark system accent preview reduce motion window translucency global background blur macOS", "color scheme follow app fixed light dark terminal background font size line height cursor style blink scrollback", "default local directory concurrent transfers name conflict", "keyboard shortcuts new close reopen select tabs previous next sidebar terminal file mode split search current page settings", "private key import generate ssh-agent saved passwords add to agent credentials", "about version GitHub project repository URL zeroy1024 Burrow software update check for updates automatic channel stable preview system proxy GitHub mirror open source license MyGo Catppuccin JetBrains Mono"}
	return (index == 5 && a.securitySettingsMatch()) || a.settingMatches(settingsCategories()[index].label, texts[index]+english[index])
}

func (a *app) settingsText(c *ui.Context, text string, props uikit.TextProps, size float32) {
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

func (a *app) settingsGroup(c *ui.Context, id, title, help string, fields []settingField) {
	visible := make([]settingField, 0, len(fields))
	for _, field := range fields {
		if a.settingMatches(field.label, field.description) || a.settingMatches(field.label, field.help) {
			visible = append(visible, field)
		}
	}
	if len(visible) == 0 {
		return
	}
	t := theme.Current()
	p := t.C.Settings
	uikit.SettingsGroup(c, uikit.SettingsGroupProps{Key: id, Title: title, Help: help}, func() {
		for index, field := range visible {
			uikit.SettingsRow(c, uikit.SettingsRowProps{
				Key: field.id, Gap: p.RowGap, Title: field.label, Detail: field.description,
				Divider: index > 0,
				Text: func() {
					uikit.SettingsLabel(c, field.help, func() {
						a.settingsText(c, field.label, uikit.TextProps{}, p.RowLabelSize)
					})
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

func settingsSegment(c *ui.Context, id, label string, options []string, current int, change func(int)) {
	items := make([]uikit.SegItem, len(options))
	for index, label := range options {
		items[index] = uikit.SegItem{Label: label}
	}
	if uikit.Segmented(c.Key(id), &current, items, uikit.SegmentedProps{Label: label}).Changed() {
		change(current)
	}
}

func (a *app) preferenceSwitch(c *ui.Context, label string, value *bool) {
	p := theme.Current().C.Switch
	if uikit.Switch(c, value, uikit.ToggleProps{Label: label, Motion: uikit.MotionOverride(a.reduceMotion(c))}).Size(p.W, p.H).Label(label).Changed() {
		a.preferencesChanged()
	}
}

func (a *app) generalSettings(c *ui.Context) {
	a.settingsGroup(c, "SettingsLanguage", locales.SettingsLanguage(), "", []settingField{{"SettingsLanguage", locales.SettingsLanguage(), "", locales.MessageChangesApplyImmediately(), func() {
		options := []uikit.Option[string]{{Value: "", Label: locales.SettingsLanguageSystem()}}
		for _, tag := range mygo.App.Languages() {
			options = append(options, uikit.Option[string]{Value: tag, Label: i18n.Autonym(tag)})
		}
		if uikit.Select(c, &a.prefs.Locale, options, uikit.SelectProps{Label: locales.SettingsLanguage()}).
			Width(theme.Current().C.Settings.ContentMax / 3).Label(locales.SettingsLanguage()).Changed() {
			mygo.App.SetLocale(a.prefs.Locale)
			a.preferencesChanged()
		}
	}}})
	a.settingsGroup(c, "MessageStartupTabs", locales.MessageStartupTabs(), "", []settingField{
		{"MessageRestoreSessionsOnStartup", locales.MessageRestoreSessionsOnStartup(), "", locales.MessageReopenTabsFromThePreviousSessionWithoutSubmittingPasswords(), func() {
			a.preferenceSwitch(
				c,
				locales.MessageRestoreSessionsOnStartup(),
				&a.prefs.RestoreSessions,
			)
		}},
		{"MessageWhenTheLastTabCloses", locales.MessageWhenTheLastTabCloses(), "", "", func() {
			values := []string{"keep", "closeWindow", "quit"}
			settingsSegment(c, "last-tab-close", locales.MessageWhenTheLastTabCloses(), []string{locales.MessageKeepWindow(), locales.MessageCloseWindow(), locales.MessageQuitApp()}, slices.Index(values, a.prefs.LastTabClose), func(index int) { a.prefs.LastTabClose = values[index]; a.preferencesChanged() })
		}},
		{"MessageConfirmBeforeClosingAnActiveConnection", locales.MessageConfirmBeforeClosingAnActiveConnection(), "", "", func() {
			a.preferenceSwitch(c, locales.MessageConfirmBeforeClosingAnActiveConnection(), &a.prefs.ConfirmCloseActive)
		}},
		{"AgentSessionSetting", locales.AgentSessionSetting(), "", locales.AgentSessionSettingDetail(), func() {
			a.preferenceSwitch(c, locales.AgentSessionSetting(), &a.prefs.ShowAgentSessions)
		}},
	})
	backgroundFields := []settingField{
		{"MessageWhenTheMainWindowCloses", locales.MessageWhenTheMainWindowCloses(), "",
			locales.MessageBackgroundRunningKeepsServicesAndConnections(), func() {
				values := []string{"quit", "background"}
				settingsSegment(
					c,
					"window-close",
					locales.MessageWhenTheMainWindowCloses(),
					[]string{locales.MessageQuitApp(), locales.MessageRunInBackground()},
					slices.Index(values, a.prefs.WindowClose),
					func(index int) { a.setBackgroundPreferences(values[index], a.prefs.ShowTray, a.prefs.CmdQBackground) },
				)
			}},
		{"MessageShowTrayIcon", locales.MessageShowTrayIcon(), "", locales.MessageBackgroundRunningRequiresTrayIcon(), func() {
			shown := a.prefs.ShowTray
			if uikit.Switch(c, &shown, uikit.ToggleProps{
				Label: locales.MessageShowTrayIcon(), Disabled: a.backgroundEnabled(),
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}).Changed() {
				a.setBackgroundPreferences(a.prefs.WindowClose, shown, a.prefs.CmdQBackground)
			}
		}},
	}
	if runtime.GOOS == "darwin" {
		backgroundFields = slices.Insert(backgroundFields, 2, settingField{
			id: "MessageHideDockIcon", label: locales.MessageHideDockIcon(),
			help: locales.MessageHideDockIconHelp(), control: func() {
				a.preferenceSwitch(c, locales.MessageHideDockIcon(), &a.prefs.HideDockInBackground)
			},
		}, settingField{
			id: "MessageCommandQBackground", label: locales.MessageCommandQBackground(),
			help: locales.MessageCommandQBackgroundHelp(), control: func() {
				enabled := a.prefs.CmdQBackground
				if uikit.Switch(c, &enabled, uikit.ToggleProps{
					Label:  locales.MessageCommandQBackground(),
					Motion: uikit.MotionOverride(a.reduceMotion(c)),
				}).Changed() {
					a.setBackgroundPreferences(a.prefs.WindowClose, a.prefs.ShowTray, enabled)
				}
			},
		})
	}
	a.settingsGroup(c, "MessageBackgroundRunning", locales.MessageBackgroundRunning(), "", backgroundFields)
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "MessageAppTheme", locales.MessageAppTheme(), "", []settingField{{"MessageAppTheme", locales.MessageAppTheme(), "", "", func() {
		ui.Row(c).Gap(t.Space.N12).Width(t.C.Settings.ContentMax * 2 / 3).Children(func() {
			choice := a.appearance
			for index, label := range []string{locales.MessageLight2(), locales.MessageDark2(), locales.SettingsLanguageSystem()} {
				uikit.ThemeCard(c, uikit.ThemeCardProps{Key: index, Label: label, Appearance: index,
					Selected: a.appearance == index, Choice: &choice}).OnClick(func() { a.setAppearance(index) })
			}
			if choice != a.appearance {
				a.setAppearance(choice)
			}
		})
	}}})
	a.settingsGroup(c, "MessageAccentColor", locales.MessageAccentColor(), "", []settingField{{"MessageAccentColor", locales.MessageAccentColor(), a.accentName(), "", func() {
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
					Key: value, Label: a.accentLabel(index), Selected: strings.EqualFold(a.prefs.Accent, value),
					Colors: swatches, SystemGradient: index == 0,
				}).
					OnClick(func() { a.prefs.Accent = value; a.syncAppearance(); a.preferencesChanged() })
			}
		})
	}}, {"MessagePreview", locales.MessagePreview(), "", "", func() {
		ui.Row(c).Gap(t.Space.N12).AlignItems(ui.Center).Children(func() {
			uikit.Button(c, locales.MessagePrimary(), uikit.ButtonProps{Variant: uikit.ButtonPrimary})
			settingsSegment(c, "accent-preview", locales.MessagePreview(), []string{locales.MessageOne(), locales.MessageTwo()}, 0, func(int) {})
			on := true
			uikit.Progress(c.Key("accent-progress-preview"), uikit.ProgressProps{
				Value: 0.5, Height: t.C.Progress.Height, Track: t.C.Progress.Track, Fill: t.C.Progress.Fill,
			}).Width(t.Space.N24 * 2)
			uikit.Switch(
				c,
				&on,
				uikit.ToggleProps{Label: locales.MessageSwitchPreview(), Disabled: true, Motion: uikit.MotionOverride(a.reduceMotion(c))},
			)
		})
	}}})
	a.settingsGroup(c, "MessageAccessibility", locales.MessageAccessibility(), locales.MessageSystemReducedMotionStillAppliesToMygoControlsWhenAppReducedMotionIs(), []settingField{
		{"MessageReduceMotion", locales.MessageReduceMotion(), "", locales.MessageReduceMovementAndScalingWhileRetainingFadesAndColorTransitions(), func() {
			values := []string{"system", "on", "off"}
			settingsSegment(c, "reduce-motion", locales.MessageReduceMotion(), []string{locales.SettingsLanguageSystem(), locales.MessageAlwaysOn(), locales.CommonClose()}, slices.Index(values, a.prefs.ReduceMotion), func(index int) { a.prefs.ReduceMotion = values[index]; a.preferencesChanged() })
		}},
		{"MessageWindowTranslucency", locales.MessageWindowTranslucency(), "", locales.MessageFrostedWindowBackgroundOnMacosAndBlurWithinBarsDisabledWhenTheSystem(), func() {
			value := a.prefs.Translucency
			locked := a.reduceTransparency
			if locked {
				value = false
			}
			if uikit.Switch(
				c,
				&value,
				uikit.ToggleProps{Label: locales.MessageWindowTranslucency(), Disabled: locked, Motion: uikit.MotionOverride(a.reduceMotion(c))},
			).Changed() {
				a.prefs.Translucency = value
				a.preferencesChanged()
			}
		}},
	})
}

func (a *app) accentLabel(index int) string {
	return []string{locales.SettingsLanguageSystem(), locales.MessageBlue(), locales.MessagePurple(), locales.MessagePink(), locales.MessageRed(), locales.MessageOrange(), locales.MessageYellow(), locales.MessageGreen(), locales.MessageGraphite()}[index]
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
	uikit.SettingsGroupBody(c).Clip().Children(func() {
		ui.Row(c).FillWidth().Padding(t.Space.N10, t.Space.N14).Background(uikit.Color(t.Color.Surface.Card)).Children(func() {
			uikit.SettingsGroupTitle(c, locales.MessagePreview(), "", func() {
				uikit.Text(
					c,
					fmt.Sprintf(
						"%s · %s %.1f / %.0f · %s",
						scheme.Name,
						prefs.FontFamily[0],
						prefs.FontSize,
						prefs.LineHeight,
						[]string{locales.MessageBlockCursor(), locales.MessageBarCursor(), locales.MessageUnderlineCursor()}[prefs.CursorShape],
					),
					uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary},
				)
			})
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
	if a.settingMatches(
		locales.MessageColorSchemes(),
		"方案 跟随应用明暗 固定一种 配色 浅色外观时 深色外观时",
	) {
		uikit.SettingsGroup(c, uikit.SettingsGroupProps{
			Title: locales.MessageColorSchemes(),
			Help:  locales.MessageClickACardToAssignItsLightOrDarkSlotImportAcceptsGhostty(),
			Actions: func() {
				uikit.Button(
					c,
					locales.MessageImportScheme(),
					uikit.ButtonProps{Disabled: a.terminal.importing},
				).OnClick(a.chooseTerminalTheme)
			},
		}, func() {
			uikit.SettingsRow(c, uikit.SettingsRowProps{
				Gap: t.C.Settings.RowGap, Title: locales.MessageMode(), Help: locales.MessageFollowSwitchBetweenTheLightAndDarkSchemesWithAppAppearance(),
				Control: func() {
					index := 0
					if a.terminal.mode.Kind == termtheme.FixedScheme {
						index = 1
					}
					settingsSegment(c, "scheme-mode", locales.MessageColorSchemeMode(), []string{locales.MessageFollowAppAppearance(), locales.MessageFixedScheme()}, index, func(index int) {
						a.changeTerminalSettings(func() {
							if index == 0 {
								a.terminal.mode = termtheme.FollowAppTheme()
							} else {
								a.terminal.mode = termtheme.Fixed(termtheme.Current().Name)
							}
						})
					})
				},
			})
			a.schemeGrid(c)
			if a.terminal.mode.Kind == termtheme.FollowApp {
				a.schemeSelectRow(c, locales.MessageLightAppearance(), &a.terminal.followLight)
				a.schemeSelectRow(c, locales.MessageDarkAppearance(), &a.terminal.followDark)
			} else {
				selected := a.terminal.mode.Name
				a.schemeSelectRow(c, locales.MessageScheme(), &selected)
				if selected != a.terminal.mode.Name {
					a.changeTerminalSettings(func() { a.terminal.mode = termtheme.Fixed(selected) })
				}
			}
		})
	}
	a.settingsGroup(c, "MessageBackground", locales.MessageBackground(), "", []settingField{{"MessageTerminalBackground", locales.MessageTerminalBackground(), "", locales.MessageSchemeUseItsBackgroundInAnyAppearanceMatchCardUseTheCardBackground(), func() {
		settingsSegment(c, "terminal-bg", locales.MessageTerminalBackground(), []string{locales.MessageSchemeDefault(), locales.MessageMatchCard()}, int(a.terminal.prefs.Background), func(index int) {
			a.changeTerminalSettings(func() { a.terminal.prefs.Background = termtheme.BackgroundSource(index) })
		})
	}}})
}

func (a *app) schemeSelectRow(c *ui.Context, label string, value *string) {
	uikit.SettingsRow(c, uikit.SettingsRowProps{
		Key: label, Gap: theme.Current().C.Settings.RowGap, Divider: true, Title: label,
		Control: func() {
			selected := *value
			if a.settingsSchemeSelect(c, label, &selected) {
				a.changeTerminalSettings(func() { *value = selected })
			}
		},
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
			label := locales.SettingsSchemesCount(count)
			if a.settingsUI.allSchemes {
				label = locales.MessageShowFewer()
			}
			uikit.Button(c, label, uikit.ButtonProps{}).OnClick(func() { a.settingsUI.allSchemes = !a.settingsUI.allSchemes })
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
			badge = locales.MessageLight()
		}
		if name == a.terminal.followDark {
			selected = true
			badge = locales.MessageDark()
		}
	}
	item := uikit.SchemeCard(c, uikit.SchemeCardProps{
		Name: name, Badge: badge, Selected: selected, Background: scheme.Background, Colors: scheme.ANSI[:8],
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
	a.settingsGroup(c, "MessageFont", locales.MessageFont(), "", []settingField{
		{"MessageFont", locales.MessageFont(), "", "", func() {
			selected := a.terminal.prefs.FontFamily[0]
			if uikit.Select(
				c,
				&selected,
				uikit.StringOptions(a.settingsUI.fonts),
				uikit.SelectProps{Label: locales.MessageFont()},
			).Width(t.C.Settings.SelectWidth).Label(locales.MessageFont()).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.FontFamily = []string{selected, "monospace"} })
			}
		}},
		{"MessageFontSize", locales.MessageFontSize(), "", "", func() {
			value := float64(a.terminal.prefs.FontSize)
			if settingsNumberInput(c, locales.MessageFontSize(), &value, 9, 24, 0.5, t.C.Settings.NumberWidth, false) {
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
			uikit.Text(c, "px", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		}},
		{"MessageLineHeight", locales.MessageLineHeight(), "", "", func() {
			value := float64(a.terminal.prefs.LineHeight)
			if settingsNumberInput(c, locales.MessageLineHeight(), &value, math.Ceil(float64(a.terminal.prefs.FontSize)*1.2), 40, 1, t.C.Settings.NumberWidth, false) {
				a.changeTerminalSettings(func() { a.terminal.prefs.LineHeight = float32(value); a.prefs.LineHeightEdited = true })
			}
			uikit.Text(c, "px", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneTertiary})
		}},
	})
	a.settingsGroup(c, "MessageCursor", locales.MessageCursor(), "", []settingField{
		{"MessageStyle", locales.MessageStyle(), "", "", func() {
			settingsSegment(c, "cursor-shape", locales.MessageCursorStyle(), []string{locales.MessageBlock(), locales.MessageBar(), locales.MessageUnderline()}, int(a.terminal.prefs.CursorShape), func(index int) {
				a.changeTerminalSettings(func() { a.terminal.prefs.CursorShape = termtheme.CursorShape(index) })
			})
		}},
		{"MessageBlink", locales.MessageBlink(), "", "", func() {
			value := a.terminal.prefs.CursorBlink
			if uikit.Switch(
				c,
				&value,
				uikit.ToggleProps{Label: locales.MessageCursorBlink(), Motion: uikit.MotionOverride(a.reduceMotion(c))},
			).Changed() {
				a.changeTerminalSettings(func() { a.terminal.prefs.CursorBlink = value })
			}
		}},
	})
	a.settingsGroup(c, "MessageScrollback", locales.MessageScrollback(), "", []settingField{{"MessageScrollbackLines", locales.MessageScrollbackLines(), "", locales.MessageLinesOfOutputKeptPerSessionTheOldestAreDiscardedBeyondThis(), func() {
		value := float64(a.prefs.ScrollbackLines)
		if settingsNumberInput(c, locales.MessageScrollbackLines(), &value, 1000, 200000, 1000, t.C.Settings.ScrollbackWidth, true) {
			a.prefs.ScrollbackLines = int(value)
			a.preferencesChanged()
		}
	}}})
}

func (a *app) fileSettings(c *ui.Context) {
	t := theme.Current()
	a.settingsGroup(c, "MessageDirectories", locales.MessageDirectories(), "", []settingField{{"MessageDefaultLocalDirectory", locales.MessageDefaultLocalDirectory(), "", locales.MessageDirectoryOpenedWhenShowingTheLocalPane(), func() {
		if uikit.Input(
			c,
			&a.prefs.DefaultLocalDir,
			uikit.InputProps{Mono: true, Label: locales.MessageDefaultLocalDirectory()},
		).Width(t.C.Settings.SelectWidth).Changed() {
			a.preferencesChanged()
		}
		uikit.IconButton(
			c,
			filesIcon,
			uikit.IconButtonProps{Key: "default-local-folder", Label: locales.MessageChooseLocalDirectory()},
		).OnClick(a.chooseDefaultLocalDirectory)
	}}})
	a.settingsGroup(c, "MessageTransfers", locales.MessageTransfers(), "", []settingField{
		{"MessageConcurrentTransfers", locales.MessageConcurrentTransfers(), "", "", func() {
			value := float64(a.prefs.TransferParallel)
			if settingsNumberInput(c, locales.MessageConcurrentTransfers(), &value, 1, 8, 1, t.C.Settings.NumberWidth, false) {
				a.prefs.TransferParallel = int(value)
				a.preferencesChanged()
			}
		}},
		{"MessageNameConflicts", locales.MessageNameConflicts(), "", "", func() {
			values := []string{"ask", "overwrite", "skip", "rename"}
			settingsSegment(c, "conflict-policy", locales.MessageNameConflicts(), []string{locales.MessageAsk(), locales.MessageOverwrite(), locales.MessageSkip(), locales.MessageRename()}, slices.Index(values, a.prefs.ConflictPolicy), func(index int) { a.prefs.ConflictPolicy = values[index]; a.preferencesChanged() })
		}},
	})
}

func (a *app) chooseDefaultLocalDirectory() {
	window := a.window
	if window == nil {
		return
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: locales.MessageDefaultLocalDirectory(), Directory: true})
		window.Update(func() {
			if err != nil {
				if a.settingsUI.open {
					a.settingsUI.notice = locales.ErrorText(err)
				} else {
					a.files.notice = &uikit.ToastProps{
						ID: locales.MessageUnableToChooseLocalDirectory() + err.Error(), Tone: uikit.ToneDanger,
						Title: locales.Deferred(locales.MessageUnableToChooseLocalDirectory2), Detail: locales.ErrorText(err),
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
