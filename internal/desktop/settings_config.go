package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
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
	open                           bool
	returnSection                  section
	previousFocus                  ui.Handle
	search                         ui.Handle
	query                          string
	locale                         string
	scroll                         [7]ui.ScrollState
	notice                         string
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
		return fmt.Errorf("读取设置：%w", err)
	}
	a.settingsCategory = slices.IndexFunc(settingsCategories[:], func(v settingsCategory) bool { return v.id == a.prefs.LastSettingsTab })
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
	if !slices.Contains([]string{"", "zh-Hans", "en"}, p.Locale) {
		return errors.New("无效的界面语言")
	}
	if p.Keymap == nil || p.ModalAdvancedOpen == nil {
		return errors.New("设置映射不能为 null")
	}
	if p.Version != 1 {
		return fmt.Errorf("不支持的设置版本 %d", p.Version)
	}
	if !slices.Contains([]string{"light", "dark", "system"}, p.AppTheme) {
		return errors.New("无效的应用主题")
	}
	if !slices.Contains([]string{"system", "on", "off"}, p.ReduceMotion) {
		return errors.New("无效的减少动态效果设置")
	}
	if !slices.Contains([]string{"keep", "closeWindow", "quit"}, p.LastTabClose) {
		return errors.New("无效的选项卡关闭设置")
	}
	if !slices.Contains([]string{"ask", "overwrite", "skip", "rename"}, p.ConflictPolicy) {
		return errors.New("无效的文件冲突设置")
	}
	if !slices.Contains([]string{"scheme", "card"}, p.TermBackground) {
		return errors.New("无效的终端背景来源")
	}
	if p.TermMode != "follow" {
		name, ok := strings.CutPrefix(p.TermMode, "fixed:")
		if !ok {
			return errors.New("无效的终端配色模式")
		}
		if _, ok := termtheme.Lookup(name); !ok {
			return errors.New("终端配色不存在")
		}
	}
	for _, name := range []string{p.TermFollowLight, p.TermFollowDark} {
		if _, ok := termtheme.Lookup(name); !ok {
			return errors.New("浅色或深色终端配色不存在")
		}
	}
	if p.TransferParallel < 1 || p.TransferParallel > 8 {
		return errors.New("同时传输数必须在 1–8 之间")
	}
	if p.ScrollbackLines < 1000 || p.ScrollbackLines > 200000 {
		return errors.New("回滚行数必须在 1000–200000 之间")
	}
	if p.Accent != "system" {
		if _, err := termtheme.ParseColor(p.Accent); err != nil {
			return fmt.Errorf("无效的强调色：%w", err)
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
				return fmt.Errorf("无效的快捷键 %q", binding)
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
		a.persistenceNotice("保存设置失败", err)
		return
	}
	a.persist(func() error { return a.store.SaveSetting(a.ctx, "preferences", data) }, func(err error) {
		if err != nil {
			a.settingsUI.notice = err.Error()
			a.persistenceNotice("保存设置失败", err)
		}
	})
}

func (a *app) preferencesData() ([]byte, error) {
	if a.store == nil {
		return nil, nil
	}
	if a.prefsLoadErr != nil {
		return nil, fmt.Errorf("原设置读取失败，已阻止覆盖：%w", a.prefsLoadErr)
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
	a.prefs.LastSettingsTab = settingsCategories[index].id
	a.preferencesChanged()
}

func (a *app) queuePreferences() { a.preferencesChanged() }
