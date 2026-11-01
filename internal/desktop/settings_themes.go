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
				uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "删除配色失败", Detail: err.Error()})
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
	previous := *value
	options := make([]uikit.Option[string], 0, len(termtheme.Names()))
	for _, name := range termtheme.Names() {
		scheme, _ := termtheme.Lookup(name)
		options = append(options, uikit.Option[string]{Value: name, Label: name, Leading: func() {
			ui.Box(c).Size(t.Size.IconSm, t.Size.IconSm).Radius(t.Radius.Chip).
				Background(uikit.Color(scheme.Background)).Border(t.Size.Hairline, uikit.Color(t.C.Input.Border)).PassThrough()
		}})
	}
	uikit.Select(c.Key(label), value, options, uikit.SelectProps{Label: localize(c, label)}).Width(t.C.Settings.SelectWidth)
	return *value != previous
}
