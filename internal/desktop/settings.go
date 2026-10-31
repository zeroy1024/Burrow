package desktop

import (
	"slices"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var settingsCategories = [...]struct {
	label string
	icon  *ui.SVG
}{
	{label: "外观", icon: appearanceIcon},
	{label: "终端配色", icon: paletteIcon},
	{label: "文字与光标", icon: textCursorIcon},
	{label: "关于 / 致谢", icon: infoIcon},
}

func (a *app) settingsTitleBar(c *ui.Context, shown float32) {
	t := theme.Current()
	bar := c.TitleBar()
	ui.Row(c).Height(max(t.C.Titlebar.Height, bar.Height)).Shrink(0).
		Gap(t.Space.N6).DragWindow().Label("设置标题栏").Children(func() {
		ui.Row(c).Width(max(t.C.Rail.Width+a.page().sidebarWidth*shown, bar.Left+t.C.Titlebar.Height)).
			Shrink(0).PaddingStart(bar.Left + t.C.Titlebar.LeftGap).DragWindow().Children(func() {
			label := "收起设置栏"
			if a.page().sidebarCollapsed {
				label = "展开设置栏"
			}
			keyedIconButton(c, "settings-sidebar-toggle", label, sidebarIcon).
				OnClick(func() { a.page().sidebarCollapsed = !a.page().sidebarCollapsed })
		})
		ui.Text(c, "设置").FontSize(t.C.Sider.TitleSize).FontWeight(t.C.Sider.TitleWeight)
	})
}

func (a *app) settingsSidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	ui.Column(c).Width(a.page().sidebarWidth).FillHeight().Left(a.page().sidebarWidth * (shown - 1)).
		Label("设置分类").Children(func() {
		ui.Text(c, "设置").FontSize(t.C.Sider.TitleSize).FontWeight(t.C.Sider.TitleWeight).
			Padding(t.C.Sider.HeaderPaddingTop, t.C.Sider.HeaderPaddingRight,
				t.C.Sider.HeaderPaddingBottom, t.C.Sider.HeaderPaddingLeft)
		ui.Column(c).FillWidth().Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).
			Gap(t.C.Catalog.RowGap).Children(func() {
			for index, category := range settingsCategories {
				item := ui.ButtonBase(c).Key(index).FillWidth().Height(t.C.GroupHeader.Height).
					Padding(0, t.C.GroupHeader.PaddingX).Radius(t.C.GroupHeader.Radius).
					Justify(ui.Start).AlignItems(ui.Center).Gap(t.C.SiderItem.Gap).
					Label(category.label).Selected(a.settingsCategory == index)
				bg, fg := ui.Transparent, tokenColor(t.Color.Text.Primary)
				if a.settingsCategory == index {
					bg, fg = tokenColor(t.Color.State.SelectedAccent), tokenColor(t.Color.Text.AccentStrong)
				} else if item.Hovered() {
					bg = tokenColor(t.Color.State.Hover)
				}
				animation := tokenTween(t.C.GroupHeader.Transition)
				item.Background(item.AnimateColor("background", bg, animation)).
					TextColor(item.AnimateColor("text", fg, animation)).OnClick(func() {
					a.settingsCategory = index
					a.terminal.scroll.Y = 0
				}).Children(func() {
					ui.Icon(c, category.icon).Size(t.Size.Icon, t.Size.Icon).Shrink(0).PassThrough()
					ui.Text(c, category.label).FontSize(t.Font.Size.Body).SingleLine().PassThrough()
				})
			}
		})
	})
}

func (a *app) syncAppearance() {
	dark := mygo.Theme.IsDark()
	if theme.Current().IsDark != dark {
		theme.Set(theme.ForAppearance(dark))
		termtheme.Apply(a.terminal.mode, dark)
	}
}

func (a *app) setAppearance(index int) {
	a.appearance = index
	sources := [...]mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeSystem}
	mygo.Theme.SetSource(sources[index])
	// SetSource updates the native appearance synchronously on the UI thread;
	// some backends do not emit OnUpdated for changes initiated by this app.
	a.syncAppearance()
}

func (a *app) settings(c *ui.Context) {
	t := theme.Current()
	workspaceCard(c).Key("settings").Grow(1).MinWidth(0).MinHeight(0).
		Children(func() {
			ui.Scroll(c).Fill().TrackScroll(&a.terminal.scroll).Children(func() {
				ui.Column(c).Key(a.settingsCategory).FillWidth().Padding(t.Space.N24).Gap(t.Space.N18).Children(func() {
					ui.Text(c, settingsCategories[a.settingsCategory].label).
						FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
					switch a.settingsCategory {
					case 0:
						a.appearanceSettings(c)
					case 1:
						a.terminalColorSettings(c)
					case 2:
						a.terminalTextSettings(c)
					case 3:
						ui.Text(c, termtheme.Licenses).FillWidth().FontSize(t.Font.Size.Body).
							TextColor(tokenColor(t.Color.Text.Primary))
					}
				})
			})
		})
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := theme.Current()
	settingsRow(c, "外观", func() {
		p := t.C.Segmented
		ui.Row(c).Padding(p.Padding).Radius(p.Radius).Background(tokenColor(p.Bg)).
			Children(func() {
				for index, label := range []string{"浅色", "深色", "跟随系统"} {
					button := ui.ButtonBase(c).Key(label).Label(label).
						Selected(a.appearance == index).FontSize(p.FontSize).
						Padding(p.ItemPaddingY, p.ItemPaddingX).Radius(p.ItemRadius).
						TextColor(tokenColor(p.Text)).OnClick(func() { a.setAppearance(index) })
					styleSegment(button, p, a.appearance == index)
					button.Children(func() { ui.Text(c, label) })
				}
			})
	})
}

func (a *app) terminalColorSettings(c *ui.Context) {
	t := theme.Current()
	ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
		ui.Text(c, "终端配色").FontSize(t.C.Button.FontSize).FontWeight(t.Font.Weight.Semibold)
		ui.Column(c).FillWidth().Gap(t.Space.N2).Children(func() {
			a.terminalSchemeRow(c, "跟随主题", termtheme.FollowAppTheme(), termtheme.Resolve(
				termtheme.FollowAppTheme(), t.IsDark,
			))
			for _, name := range termtheme.Names() {
				scheme, _ := termtheme.Lookup(name)
				a.terminalSchemeRow(c, name, termtheme.Fixed(name), scheme)
			}
			button(c, "导入文件…").Disabled(a.terminal.importing).OnClick(a.chooseTerminalTheme)
		})
	})
	settingsRow(c, "终端背景", func() {
		options := []string{"配色自带背景（默认）", "与卡片底色一致"}
		selected := options[int(a.terminal.prefs.Background)]
		if ui.Select(c, &selected, options).Label("终端背景").Changed() {
			a.changeTerminalSettings(func() {
				a.terminal.prefs.Background = termtheme.BackgroundSource(slices.Index(options, selected))
			})
		}
	})
	if a.terminal.mode.Kind == termtheme.FollowApp {
		settingsRow(c, "浅色配色", func() {
			selected := a.terminal.followLight
			if ui.Select(c, &selected, termtheme.Names()).Label("浅色配色").Changed() {
				a.changeTerminalSettings(func() { a.terminal.followLight = selected })
			}
		})
		settingsRow(c, "深色配色", func() {
			selected := a.terminal.followDark
			if ui.Select(c, &selected, termtheme.Names()).Label("深色配色").Changed() {
				a.changeTerminalSettings(func() { a.terminal.followDark = selected })
			}
		})
	}
}

func (a *app) terminalSchemeRow(c *ui.Context, label string, mode termtheme.Mode, scheme *termtheme.Scheme) {
	t := theme.Current()
	p := t.C.Segmented
	selected := a.terminal.mode == mode
	button := ui.ButtonBase(c).Key(label).FillWidth().Padding(p.ItemPaddingY, p.ItemPaddingX).
		Radius(p.ItemRadius).Label(label).Selected(selected).TextColor(tokenColor(p.Text)).
		OnClick(func() { a.changeTerminalSettings(func() { a.terminal.mode = mode }) })
	styleSegment(button, p, selected)
	button.Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space.N16).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Text(c, label).FontSize(p.FontSize).Grow(1)
			ui.Row(c).Gap(0).PassThrough().Children(func() {
				for _, col := range scheme.ANSI {
					ui.Box(c).Size(t.Space.N8, t.Space.N8).Background(tokenColor(col)).PassThrough()
				}
			})
		})
	})
}

func (a *app) terminalTextSettings(c *ui.Context) {
	t := theme.Current()
	ui.Text(c, "终端文字与光标").FontSize(t.C.Button.FontSize).FontWeight(t.Font.Weight.Semibold)
	settingsRow(c, "字体", func() {
		selected := a.terminal.prefs.FontFamily[0]
		options := []string{"JetBrains Mono", "Menlo", "Monaco", "SF Mono"}
		if !slices.Contains(options, selected) {
			options = append(options, selected)
		}
		if ui.Select(c, &selected, options).Label("终端字体").Changed() {
			a.changeTerminalSettings(func() {
				a.terminal.prefs.FontFamily = []string{selected, "Menlo", "monospace"}
			})
		}
	})
	a.terminalNumberSetting(c, "字号", &a.terminal.prefs.FontSize, 6, 48)
	a.terminalNumberSetting(c, "行高", &a.terminal.prefs.LineHeight, 6, 80)
	a.terminalNumberSetting(c, "字距", &a.terminal.prefs.Tracking, -4, 8)
	settingsRow(c, "光标形状", func() {
		options := []string{"块状", "竖线", "下划线"}
		selected := options[int(a.terminal.prefs.CursorShape)]
		if ui.Select(c, &selected, options).Label("光标形状").Changed() {
			a.changeTerminalSettings(func() {
				a.terminal.prefs.CursorShape = termtheme.CursorShape(slices.Index(options, selected))
			})
		}
	})
	settingsRow(c, "光标闪烁", func() {
		checked := a.terminal.prefs.CursorBlink
		if ui.Checkbox(c, &checked, "启用闪烁").Changed() {
			a.changeTerminalSettings(func() { a.terminal.prefs.CursorBlink = checked })
		}
	})
	settingsRow(c, "粗体颜色", func() {
		checked := a.terminal.prefs.BoldIsBright
		if ui.Checkbox(c, &checked, "使用 ANSI 亮色").Changed() {
			a.changeTerminalSettings(func() { a.terminal.prefs.BoldIsBright = checked })
		}
	})
}

func (a *app) terminalNumberSetting(c *ui.Context, label string, destination *float32, lo, hi float64) {
	settingsRow(c, label, func() {
		value := float64(*destination)
		if ui.NumberInput(c, &value, lo, hi, 0.1).Label(label).Changed() {
			a.changeTerminalSettings(func() { *destination = float32(value) })
		}
	})
}

func settingsRow(c *ui.Context, label string, content func()) {
	t := theme.Current()
	ui.Row(c).FillWidth().Gap(t.Space.N16).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(t.C.Button.FontSize).Width(t.C.Sider.Width / 2).Shrink(0)
		content()
	})
}
