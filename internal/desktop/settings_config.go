package desktop

import (
	"encoding/json"
	"image/color"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type appPreferences struct {
	Version            int               `json:"version"`
	Locale             string            `json:"locale"`
	RestoreSessions    bool              `json:"restoreSessions"`
	LastTabClose       string            `json:"lastTabClose"`
	ConfirmCloseActive bool              `json:"confirmCloseActive"`
	AppTheme           string            `json:"appTheme"`
	Accent             string            `json:"accent"`
	ReduceMotion       string            `json:"reduceMotion"`
	Translucency       bool              `json:"translucency"`
	TermMode           string            `json:"termMode"`
	TermFollowLight    string            `json:"termFollowLight"`
	TermFollowDark     string            `json:"termFollowDark"`
	TermBackground     string            `json:"termBackground"`
	Term               termtheme.Prefs   `json:"term"`
	LineHeightEdited   bool              `json:"lineHeightEdited"`
	ScrollbackLines    int               `json:"scrollbackLines"`
	DefaultLocalDir    string            `json:"defaultLocalDir"`
	TransferParallel   int               `json:"transferParallel"`
	ConflictPolicy     string            `json:"conflictPolicy"`
	Keymap             map[string]string `json:"keymap"`
	LastSettingsTab    string            `json:"lastSettingsTab"`
	ModalAdvancedOpen  map[string]bool   `json:"modalAdvancedOpen"`
}

type preferenceSaveState struct {
	dirty   bool
	dirtyAt time.Time
}

type settingsState struct {
	open          bool
	returnSection section
	previousFocus ui.Handle
	search        ui.Handle
	query         string

	scroll                         [7]ui.ScrollState
	notice                         locales.Text
	allSchemes                     bool
	recording                      string
	pendingBinding, conflictAction string
	fonts                          []string
	license                        int
}

func (s *settingsState) reset() {
	clean := settingsState{
		search: s.search,
	}
	for index := range s.scroll {
		clean.scroll[index].Handle = s.scroll[index].Handle
	}
	// MyGo keeps handle bindings for the window's lifetime. Reuse those bindings
	// while releasing page data and callbacks.
	*s = clean
}

func (a *app) loadAppPreferences() error {
	a.appearance = 2
	a.prefs = appPreferences{
		Version: 1, RestoreSessions: true, LastTabClose: "keep", ConfirmCloseActive: true,
		AppTheme: "system", Accent: "#0a84ff", ReduceMotion: "system", Translucency: true,
		TermMode: a.terminal.mode.String(), TermFollowLight: a.terminal.followLight,
		TermFollowDark: a.terminal.followDark, TermBackground: a.terminal.prefs.Background.String(),
		Term: a.terminal.prefs, ScrollbackLines: 4000, DefaultLocalDir: "~", TransferParallel: 3,
		ConflictPolicy: "ask", Keymap: map[string]string{},
		LastSettingsTab: "terminal", ModalAdvancedOpen: map[string]bool{},
	}
	previous := a.prefs
	var data []byte
	found, err := a.store.ReadSetting(a.ctx, "preferences", &data)
	if err == nil && found {
		candidate := previous
		candidate.Keymap = maps.Clone(previous.Keymap)
		candidate.ModalAdvancedOpen = maps.Clone(previous.ModalAdvancedOpen)
		candidate.Term.FontFamily = slices.Clone(previous.Term.FontFamily)
		err = json.Unmarshal(data, &candidate)
		if err == nil {
			a.prefs = candidate
			err = a.validateAppPreferences()
		}
	}
	if err != nil {
		a.prefs = previous
		a.prefsLoadErr = err
		return locales.Errorf(locales.MessageReadSettings, err)
	}
	a.settingsCategory = slices.IndexFunc(settingsCategories()[:], func(v settingsCategory) bool { return v.id == a.prefs.LastSettingsTab })
	if a.settingsCategory < 0 {
		a.settingsCategory = 2
	}
	a.terminal.mode = termtheme.ParseMode(a.prefs.TermMode)
	a.terminal.followLight, a.terminal.followDark = a.prefs.TermFollowLight, a.prefs.TermFollowDark
	a.terminal.prefs = a.prefs.Term
	a.terminal.prefs.Background = termtheme.ParseBackgroundSource(a.prefs.TermBackground)
	a.applyTerminalSettings()
	a.appearance = slices.Index([]string{"light", "dark", "system"}, a.prefs.AppTheme)
	if os.Getenv("MYGO_LOCALE") == "" {
		mygo.App.SetLocale(a.prefs.Locale)
	}
	return nil
}

func (a *app) validateAppPreferences() error {
	p := a.prefs
	if p.Locale != "" && !slices.Contains(mygo.App.Languages(), p.Locale) {
		return locales.NewError(locales.MessageInvalidInterfaceLanguage)
	}
	if p.Keymap == nil || p.ModalAdvancedOpen == nil {
		return locales.NewError(locales.MessageSettingsMapsCannotBeNull)
	}
	if p.Version != 1 {
		return locales.Errorf(locales.MessageUnsupportedSettingsVersion, p.Version)
	}
	if !slices.Contains([]string{"light", "dark", "system"}, p.AppTheme) {
		return locales.NewError(locales.MessageInvalidAppTheme)
	}
	if !slices.Contains([]string{"system", "on", "off"}, p.ReduceMotion) {
		return locales.NewError(locales.MessageInvalidReducedMotionSetting)
	}
	if !slices.Contains([]string{"keep", "closeWindow", "quit"}, p.LastTabClose) {
		return locales.NewError(locales.MessageInvalidTabCloseSetting)
	}
	if !slices.Contains([]string{"ask", "overwrite", "skip", "rename"}, p.ConflictPolicy) {
		return locales.NewError(locales.MessageInvalidFileConflictSetting)
	}
	if !slices.Contains([]string{"scheme", "card"}, p.TermBackground) {
		return locales.NewError(locales.MessageInvalidTerminalBackgroundSource)
	}
	if p.TermMode != "follow" {
		name, ok := strings.CutPrefix(p.TermMode, "fixed:")
		if !ok {
			return locales.NewError(locales.MessageInvalidTerminalColorSchemeMode)
		}
		if _, ok := termtheme.Lookup(name); !ok {
			return locales.NewError(locales.MessageTheTerminalColorSchemeDoesNotExist)
		}
	}
	for _, name := range []string{p.TermFollowLight, p.TermFollowDark} {
		if _, ok := termtheme.Lookup(name); !ok {
			return locales.NewError(locales.MessageTheLightOrDarkTerminalColorSchemeDoesNotExist)
		}
	}
	if p.TransferParallel < 1 || p.TransferParallel > 8 {
		return locales.NewError(locales.MessageConcurrentTransfersMustBeBetweenAnd)
	}
	if p.ScrollbackLines < 1000 || p.ScrollbackLines > 200000 {
		return locales.NewError(locales.MessageScrollbackMustBeBetweenAndLines)
	}
	if p.Accent != "system" {
		if _, err := termtheme.ParseColor(p.Accent); err != nil {
			return locales.Errorf(locales.MessageInvalidAccentColor, err)
		}
	}
	cfg := a.terminal.config()
	cfg.FontFamily = p.Term.FontFamily
	cfg.FontSize = p.Term.FontSize
	cfg.LineHeight = p.Term.LineHeight
	cfg.CursorShape = p.Term.CursorShape
	if err := cfg.validate(); err != nil {
		return err
	}
	for _, binding := range p.Keymap {
		if binding != "" {
			if _, _, ok := bindingToKey(binding); !ok {
				return locales.Errorf(locales.MessageInvalidShortcut, binding)
			}
		}
	}
	return nil
}

func (a *app) preferencesChanged() {
	a.syncWindowMaterial()
	a.prefsSave.dirty = true
	a.prefsSave.dirtyAt = time.Time{}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *app) tickPreferences(c *ui.Context) {
	if !a.prefsSave.dirty {
		return
	}
	if a.prefsSave.dirtyAt.IsZero() {
		a.prefsSave.dirtyAt = c.Now()
	}
	remaining := 300*time.Millisecond - c.Now().Sub(a.prefsSave.dirtyAt)
	if remaining > 0 {
		c.After(remaining)
		return
	}
	a.prefsSave.dirty = false
	data, err := a.preferencesData()
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToSaveSettings, err)
		return
	}
	a.persist(func() error { return a.store.SaveSetting(a.ctx, "preferences", data) }, func(err error) {
		if err != nil {
			a.settingsUI.notice = locales.ErrorText(err)
			a.persistenceNotice(locales.MessageUnableToSaveSettings, err)
		}
	})
}

func (a *app) preferencesData() ([]byte, error) {
	if a.store == nil {
		return nil, nil
	}
	if a.prefsLoadErr != nil {
		return nil, locales.Errorf(locales.MessageTheOriginalSettingsCouldNotBeReadOverwritingBlocked, a.prefsLoadErr)
	}
	a.prefs.TermMode = a.terminal.mode.String()
	a.prefs.TermFollowLight, a.prefs.TermFollowDark = a.terminal.followLight, a.terminal.followDark
	a.prefs.TermBackground = a.terminal.prefs.Background.String()
	a.prefs.Term = a.terminal.prefs
	if err := a.validateAppPreferences(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(a.prefs)
	if err != nil {
		return nil, err
	}
	return data, nil
}
func (a *app) reduceMotion(c *ui.Context) bool {
	return uikit.Reduced(c, a.prefs.ReduceMotion)
}

func (a *app) syncAppearance() {
	dark := mygo.Theme.IsDark()
	a.reduceTransparency = mygo.Theme.Preferences().ReduceTransparency
	t := theme.ForAppearance(dark)
	var accent color.NRGBA
	if a.prefs.Accent == "system" {
		accent = mygo.Theme.Preferences().Accent
	} else if a.prefs.Accent != "" {
		accent, _ = termtheme.ParseColor(a.prefs.Accent)
	}
	if accent.A == 0 {
		accent = t.Color.AccentChoices.Blue
	}
	theme.Set(theme.WithAccent(t, accent))
	termtheme.Apply(a.terminal.mode, dark)
	a.syncWindowMaterial()
}

func (a *app) setAppearance(index int) {
	a.appearance = index
	a.prefs.AppTheme = []string{"light", "dark", "system"}[index]
	mygo.Theme.SetSource([]mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeSystem}[index])
	a.syncAppearance()
	a.preferencesChanged()
}

func (a *app) openSettings() {
	if a.settingsUI.open {
		return
	}
	if a.section == sectionGateways {
		a.cancelGatewayTest()
	}
	a.settingsUI.returnSection = a.section
	if a.section == sectionFiles {
		if session := a.currentFileSession(); session != nil {
			a.settingsUI.previousFocus = session.remote.focus
			if !a.files.activeRemote {
				a.settingsUI.previousFocus = session.local.focus
			}
		}
	} else if tab := a.currentTab(); tab != nil {
		a.settingsUI.previousFocus = tab.focused.focus
	}
	a.clearWorkspaceGestures()
	a.settingsUI.open = true
	a.settingsUI.search.Focus()
}

func (a *app) openSettingsCategory(index int) {
	a.openSettings()
	a.selectSettingsCategory(index)
}

func (a *app) closeSettings() {
	if !a.settingsUI.open {
		return
	}
	previousFocus := a.settingsUI.previousFocus
	a.settingsUI.reset()
	previousFocus.Focus()
}

func (a *app) selectSettingsCategory(index int) {
	if index == a.settingsCategory {
		return
	}
	a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
	a.settingsCategory = index
	a.prefs.LastSettingsTab = settingsCategories()[index].id
	a.preferencesChanged()
}

func (a *app) queuePreferences() { a.preferencesChanged() }
