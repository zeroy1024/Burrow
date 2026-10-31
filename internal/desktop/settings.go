package desktop

import (
	"slices"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
	workspaceCard(c).Key("settings").Grow(1).MinHeight(0).
		Margin(0, t.C.Window.BodyPaddingRight, t.C.Window.BodyPaddingBottom, 0).
		Children(func() {
			ui.Scroll(c).Fill().TrackScroll(&a.terminal.scroll).Children(func() {
				ui.Column(c).FillWidth().Padding(t.Space.N24).Gap(t.Space.N18).Children(func() {
					ui.Text(c, "设置").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
					a.appearanceSettings(c)
					a.terminalColorSettings(c)
					a.terminalTextSettings(c)
					ui.Button(c, "关于 / 致谢").OnClick(func() { a.terminal.licenses = !a.terminal.licenses })
					if a.terminal.licenses {
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
					if a.appearance == index {
						button.Background(tokenColor(p.BgOn)).TextColor(tokenColor(p.TextOn))
						applyShadow(button, p.ShadowOn)
					}
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
			ui.Button(c, "导入文件…").Disabled(a.terminal.importing).OnClick(a.chooseTerminalTheme)
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
	if selected {
		button.Background(tokenColor(p.BgOn)).TextColor(tokenColor(p.TextOn))
	} else if button.Hovered() {
		button.Background(tokenColor(t.C.Button.BgHover))
	}
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
