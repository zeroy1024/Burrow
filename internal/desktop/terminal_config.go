package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo"
)

type terminalSettingsState struct {
	mode        termtheme.Mode
	prefs       termtheme.Prefs
	followLight string
	followDark  string
	loadErr     error
	importing   bool
	imported    map[string]string
}

type terminalConfig struct {
	Mode        string                `json:"mode"`
	FollowLight string                `json:"follow_light"`
	FollowDark  string                `json:"follow_dark"`
	Background  string                `json:"background"`
	FontFamily  []string              `json:"font_family"`
	FontSize    float32               `json:"font_size"`
	LineHeight  float32               `json:"line_height"`
	CursorShape termtheme.CursorShape `json:"cursor_shape"`
	CursorBlink bool                  `json:"cursor_blink"`
	BoldBright  bool                  `json:"bold_is_bright"`
}

func (s terminalSettingsState) config() terminalConfig {
	return terminalConfig{
		Mode: s.mode.String(), FollowLight: s.followLight, FollowDark: s.followDark,
		Background: s.prefs.Background.String(), FontFamily: s.prefs.FontFamily,
		FontSize: s.prefs.FontSize, LineHeight: s.prefs.LineHeight,
		CursorShape: s.prefs.CursorShape, CursorBlink: s.prefs.CursorBlink, BoldBright: s.prefs.BoldIsBright,
	}
}

func (a *app) loadTerminalSettings() error {
	a.terminal = terminalSettingsState{
		mode: termtheme.FollowAppTheme(), prefs: termtheme.DefaultPrefs(),
		followLight: termtheme.DefaultLight, followDark: termtheme.DefaultDark,
	}
	a.terminal.loadErr = a.loadTerminalThemes()
	a.applyTerminalSettings()
	return a.terminal.loadErr
}

func parseImportedTheme(filename string, data []byte) (*termtheme.Scheme, error) {
	scheme, err := termtheme.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if scheme.Name == "" {
		scheme.Name = filepath.Base(filename)
		if ext := filepath.Ext(scheme.Name); ext == ".conf" || ext == ".ghostty" {
			scheme.Name = strings.TrimSuffix(scheme.Name, ext)
		}
	}
	return scheme, nil
}

func (a *app) loadTerminalThemes() error {
	a.terminal.imported = map[string]string{}
	themes, err := a.store.Themes(a.ctx)
	if err != nil {
		return err
	}
	for _, saved := range themes {
		scheme, err := parseImportedTheme(saved.Filename, saved.Source)
		if err != nil {
			return err
		}
		if scheme.Name != saved.Name || termtheme.IsBundled(scheme.Name) {
			return fmt.Errorf("invalid imported theme %q", saved.Name)
		}
		termtheme.Register(scheme)
		a.terminal.imported[scheme.Name] = saved.Filename
	}
	return nil
}

func (config terminalConfig) validate() error {
	if len(config.FontFamily) == 0 {
		return errors.New("font_family must contain a font")
	}
	for _, family := range config.FontFamily {
		if strings.TrimSpace(family) == "" {
			return errors.New("font_family must not contain an empty font")
		}
	}
	if config.FontSize < 6 || config.FontSize > 48 {
		return errors.New("font_size must be between 6 and 48")
	}
	if config.LineHeight < 6 || config.LineHeight > 80 {
		return errors.New("line_height must be between 6 and 80")
	}
	if config.CursorShape < termtheme.CursorBlock || config.CursorShape > termtheme.CursorUnderline {
		return errors.New("unknown cursor_shape")
	}
	return nil
}

func (a *app) applyTerminalSettings() {
	a.cellWidth = 0
	termtheme.SetFollowPair(a.terminal.followLight, a.terminal.followDark)
	termtheme.Apply(a.terminal.mode, theme.Current().IsDark)
}

func (a *app) changeTerminalSettings(change func()) {
	previous := a.terminal
	change()
	if err := a.saveTerminalSettings(); err != nil {
		a.terminal = previous
		a.terminalError = err
		return
	}
	a.applyTerminalSettings()
}

func (a *app) saveTerminalSettings() error {
	if a.terminal.loadErr != nil {
		return a.terminal.loadErr
	}
	if a.prefsLoadErr != nil {
		return a.prefsLoadErr
	}
	if err := a.terminal.config().validate(); err != nil {
		return err
	}
	a.preferencesChanged()
	return nil
}

func (a *app) chooseTerminalTheme() {
	if a.terminal.loadErr != nil {
		a.terminalError = a.terminal.loadErr
		return
	}
	a.terminal.importing = true
	window := a.window
	a.sshWorkers.Go(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: locales.MessageImportTerminalColorScheme(), Message: locales.MessageChooseAGhosttyColorSchemeFile()})
		var source *termtheme.Scheme
		filename := ""
		if err == nil && len(paths) > 0 {
			source, filename, err = a.persistImportedTheme(paths[0])
		}
		window.Update(func() {
			a.terminal.importing = false
			if err != nil {
				a.terminalError = err
				return
			}
			if source != nil {
				a.activateImportedTheme(source, filename)
			}
		})
	})
}

func (a *app) persistImportedTheme(path string) (*termtheme.Scheme, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	filename := filepath.Base(path)
	source, err := parseImportedTheme(filename, data)
	if err != nil {
		return nil, "", fmt.Errorf("import terminal theme: %w", err)
	}
	if termtheme.IsBundled(source.Name) {
		return nil, "", locales.Errorf(
			locales.MessageBuiltInColorSchemeCannotBeOverwrittenChangeItsNameBeforeImporting,
			source.Name,
		)
	}
	if err := a.store.SaveTheme(a.ctx, store.Theme{Name: source.Name, Filename: filename, Source: data}); err != nil {
		return nil, "", err
	}
	return source, filename, nil
}

func (a *app) activateImportedTheme(source *termtheme.Scheme, filename string) {
	termtheme.Register(source)
	if a.terminal.imported == nil {
		a.terminal.imported = map[string]string{}
	}
	a.terminal.imported[source.Name] = filename
	previous := a.terminal.mode
	a.terminal.mode = termtheme.Fixed(source.Name)
	if err := a.saveTerminalSettings(); err != nil {
		a.terminal.mode = previous
		a.terminalError = err
		return
	}
	a.applyTerminalSettings()
}
