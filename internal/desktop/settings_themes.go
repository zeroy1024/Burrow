package desktop

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

func (a *app) importedThemeUse(name string) string {
	if name == a.terminal.followLight || name == a.terminal.followDark {
		return locales.MessageUsedByTheLightOrDarkSchemeSlot()
	}
	if a.terminal.mode.Kind == termtheme.FixedScheme && a.terminal.mode.Name == name {
		return locales.MessageUsedByTheGlobalTerminal()
	}
	for _, h := range a.hosts {
		if h.session.SSH != nil && termtheme.ParseMode(h.session.SSH.TermScheme) == termtheme.Fixed(name) {
			return locales.MessageUsedByASavedSession()
		}
	}
	for _, tab := range a.tabs {
		for _, session := range tab.root.sessions() {
			if session.config.SSH != nil && termtheme.ParseMode(session.config.SSH.TermScheme) == termtheme.Fixed(name) {
				return locales.MessageUsedByAnOpenSession()
			}
		}
	}
	for _, closed := range a.closedTabs {
		if options := closed.endpoint.session.SSH; options != nil && termtheme.ParseMode(options.TermScheme) == termtheme.Fixed(name) {
			return locales.MessageUsedByARecoverableSession()
		}
	}
	for _, group := range a.runtimeSSHGroups() {
		if options := group.endpoint.session.SSH; options != nil && termtheme.ParseMode(options.TermScheme) == termtheme.Fixed(name) {
			return locales.MessageUsedByAConnectionSnapshot()
		}
		for _, step := range group.plan.steps {
			if options := step.endpoint.session.SSH; options != nil && termtheme.ParseMode(options.TermScheme) == termtheme.Fixed(name) {
				return locales.MessageUsedByAConnectionSnapshot()
			}
		}
	}

	return ""
}

func (a *app) importedThemeMenu(c *ui.Context, name string) func(*ui.Menu) {
	return func(menu *ui.Menu) {
		reason := a.importedThemeUse(name)
		label := locales.MessageDeleteScheme()
		if reason != "" {
			label = locales.CommonNameWithReason(label, reason)
		}
		if menu.Item(label).Disabled(reason != "").Chosen() {
			if err := a.deleteImportedTheme(name); err != nil {
				uikit.Toast(
					c,
					uikit.ToastProps{Tone: uikit.ToneDanger, Title: locales.Deferred(
						locales.MessageUnableToDeleteColorScheme,
					), Detail: locales.ErrorText(
						err,
					)},
				)
			}
		}
	}
}

func (a *app) deleteImportedTheme(name string) error {
	if a.terminal.importing {
		return locales.NewError(locales.MessageProcessingColorScheme)
	}
	if termtheme.IsBundled(name) {
		return locales.NewError(locales.MessageBuiltInSchemesCannotBeDeleted)
	}
	if reason := a.importedThemeUse(name); reason != "" {
		return errors.New(reason)
	}
	_, ok := a.terminal.imported[name]
	if !ok {
		return locales.Errorf(locales.MessageColorSchemeFileNotFound, name)
	}
	a.terminal.importing = true
	a.persist(func() error { return a.store.DeleteTheme(a.ctx, name) }, func(err error) {
		a.terminal.importing = false
		if err == nil {
			err = termtheme.Unregister(name)
		}
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToDeleteColorScheme, err)
			return
		}
		delete(a.terminal.imported, name)
	})
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
	// The value may be a per-frame copy; keep the control's identity stable.
	uikit.Select(c.Key("settings-scheme"), value, options, uikit.SelectProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Label: label}).Width(t.C.Settings.SelectWidth)
	return *value != previous
}
