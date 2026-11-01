package desktop

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

func (a *app) importedThemeUse(name string) string {
	if name == a.terminal.followLight || name == a.terminal.followDark {
		return "浅色或深色配色槽正在使用"
	}
	if a.terminal.mode.Kind == termtheme.FixedScheme && a.terminal.mode.Name == name {
		return "全局终端正在使用"
	}
	for _, h := range a.hosts {
		if h.session.SSH != nil && termtheme.ParseMode(h.session.SSH.TermScheme) == termtheme.Fixed(name) {
			return "已保存的会话正在使用"
		}
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if session.config.SSH != nil && termtheme.ParseMode(session.config.SSH.TermScheme) == termtheme.Fixed(name) {
				return "已打开的会话正在使用"
			}
		}
	}
	return ""
}

func (a *app) importedThemeMenu(c *ui.Context, name string) func(*ui.Menu) {
	return func(menu *ui.Menu) {
		reason := a.importedThemeUse(name)
		label := "删除配色"
		if reason != "" {
			label += "（" + reason + "）"
		}
		if menu.Item(label).Disabled(reason != "").Chosen() {
			if err := a.deleteImportedTheme(name); err != nil {
				c.Toast("删除配色失败：" + err.Error())
			}
		}
	}
}

func (a *app) deleteImportedTheme(name string) error {
	if termtheme.IsBundled(name) {
		return errors.New("不能删除内置配色")
	}
	if reason := a.importedThemeUse(name); reason != "" {
		return errors.New(reason)
	}
	path, ok := a.terminal.imported[name]
	if !ok {
		return fmt.Errorf("找不到配色文件 %q", name)
	}
	if err := mygo.Shell.TrashItem(path); err != nil {
		return err
	}
	if err := termtheme.Unregister(name); err != nil {
		return err
	}
	delete(a.terminal.imported, name)
	return nil
}

func (a *app) settingsSchemeSelect(c *ui.Context, label string, value *string) bool {
	t := theme.Current()
	p := t.C.Input
	previous := *value
	parts := ui.SelectBase(c.Key(label), value)
	uikit.StyleSelect(parts.Trigger, localize(c, label)).Width(t.C.Settings.SelectWidth).Gap(p.Gap).Children(func() {
		if scheme, ok := termtheme.Lookup(*value); ok {
			ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).Background(uikit.Color(scheme.Background)).Border(t.Size.Hairline, uikit.Color(p.Border)).PassThrough()
		}
		ui.Text(c, *value).FontSize(p.FontSize).Grow(1).MinWidth(0).SingleLine().PassThrough()
		ui.Icon(c, chevronIcon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(uikit.Color(p.Icon)).PassThrough()
	})
	uikit.StyleFormControl(parts.Trigger, "")
	parts.Popup(func(panel ui.Element) {
		uikit.StyleSelectPopup(panel).Radius(t.Radius.Popover).MaxHeight(t.C.Settings.ContentMax / 2)
		for _, name := range termtheme.Names() {
			item := parts.Item(name).Height(p.Height).Padding(0, t.C.Menu.ItemPaddingX).Gap(t.Space.N8).Radius(t.C.Menu.ItemRadius)
			if item.Highlighted() {
				item.Background(uikit.Color(t.Color.State.Hover))
			}
			item.Children(func() {
				scheme, _ := termtheme.Lookup(name)
				ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).Background(uikit.Color(scheme.Background)).PassThrough()
				ui.Text(c, name).FontSize(p.FontSize).PassThrough()
			})
		}
	})
	return *value != previous
}
