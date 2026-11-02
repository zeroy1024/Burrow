package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type terminalSettingsState struct {
	mode        termtheme.Mode
	prefs       termtheme.Prefs
	followLight string
	followDark  string
	path        string
	loadErr     error
	scroll      ui.ScrollState
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
	directory, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.terminal.loadErr = fmt.Errorf("locate terminal configuration directory: %w", err)
		a.applyTerminalSettings()
		return a.terminal.loadErr
	}
	a.terminal.path = filepath.Join(directory, "terminal.json")
	importErr := a.loadTerminalThemes()
	a.terminal.loadErr = a.readTerminalSettings()
	a.applyTerminalSettings()
	return errors.Join(importErr, a.terminal.loadErr)
}

func (a *app) loadTerminalThemes() error {
	a.terminal.imported = map[string]string{}
	directory := filepath.Join(filepath.Dir(a.terminal.path), "themes")
	files, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read imported terminal themes: %w", err)
	}
	var importErr error
	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".terminal-") {
			continue
		}
		path := filepath.Join(directory, file.Name())
		scheme, err := termtheme.Import(path)
		if err != nil {
			importErr = errors.Join(importErr, fmt.Errorf("import %s: %w", file.Name(), err))
		} else {
			a.terminal.imported[scheme.Name] = path
		}
	}
	return importErr
}

func (a *app) readTerminalSettings() error {
	data, err := os.ReadFile(a.terminal.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read terminal settings: %w", err)
	}
	config := a.terminal.config()
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode terminal settings: %w", err)
	}
	if err := config.validate(); err != nil {
		return fmt.Errorf("validate terminal settings: %w", err)
	}
	a.terminal.mode = termtheme.ParseMode(config.Mode)
	a.terminal.followLight, a.terminal.followDark = config.FollowLight, config.FollowDark
	a.terminal.prefs = termtheme.Prefs{
		FontFamily: config.FontFamily, FontSize: config.FontSize, LineHeight: config.LineHeight,
		CursorShape: config.CursorShape, CursorBlink: config.CursorBlink,
		Background: termtheme.ParseBackgroundSource(config.Background), BoldIsBright: config.BoldBright,
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
	if a.prefsPath != "" {
		if err := a.terminal.config().validate(); err != nil {
			return err
		}
		a.preferencesChanged()
		return nil
	}
	if a.terminal.loadErr != nil {
		return fmt.Errorf("terminal settings were not loaded: %w", a.terminal.loadErr)
	}
	config := a.terminal.config()
	if err := config.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode terminal settings: %w", err)
	}
	return writeTerminalFile(a.terminal.path, append(data, '\n'))
}

func (a *app) chooseTerminalTheme() {
	a.terminal.importing = true
	window := a.window
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: window, Title: "导入终端配色", Message: "选择 Ghostty 格式的配色文件",
		})
		window.Update(func() {
			a.terminal.importing = false
			if err != nil {
				a.terminalError = fmt.Errorf("open terminal theme: %w", err)
				return
			}
			if len(paths) == 0 {
				return
			}
			if err := a.importTerminalTheme(paths[0]); err != nil {
				a.terminalError = err
			}
		})
	}()
}

func (a *app) importTerminalTheme(path string) error {
	if a.terminal.loadErr != nil {
		return fmt.Errorf("terminal settings were not loaded: %w", a.terminal.loadErr)
	}
	// Validate before copying; keep the original bytes, including source and license comments.
	source, err := termtheme.Load(path)
	if err != nil {
		return fmt.Errorf("import terminal theme: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read terminal theme: %w", err)
	}
	if termtheme.IsBundled(source.Name) {
		return fmt.Errorf("内置配色 %q 不能覆盖，请修改 name 后导入", source.Name)
	}
	// Retain the filename: themes without a name comment derive their name from it.
	destination := filepath.Join(filepath.Dir(a.terminal.path), "themes", filepath.Base(path))
	if err := checkTerminalThemeName(destination, source.Name); err != nil {
		return err
	}
	existing, err := os.ReadFile(destination)
	switch {
	case err == nil:
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("已存在同名配色文件 %q，请重命名后导入", filepath.Base(destination))
		}
	case errors.Is(err, os.ErrNotExist):
		if err := writeTerminalFile(destination, data); err != nil {
			return fmt.Errorf("copy terminal theme: %w", err)
		}
	default:
		return fmt.Errorf("read existing terminal theme: %w", err)
	}
	scheme, err := termtheme.Import(destination)
	if err != nil {
		return fmt.Errorf("import terminal theme: %w", err)
	}
	if a.terminal.imported == nil {
		a.terminal.imported = map[string]string{}
	}
	a.terminal.imported[scheme.Name] = destination
	previous := a.terminal.mode
	a.terminal.mode = termtheme.Fixed(scheme.Name)
	if err := a.saveTerminalSettings(); err != nil {
		a.terminal.mode = previous
		return err
	}
	a.applyTerminalSettings()
	return nil
}

// A scheme name must identify one persisted file, so restart order cannot select older colors.
func checkTerminalThemeName(destination, name string) error {
	files, err := os.ReadDir(filepath.Dir(destination))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read imported terminal themes: %w", err)
	}
	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".terminal-") || file.Name() == filepath.Base(destination) {
			continue
		}
		other, err := termtheme.Load(filepath.Join(filepath.Dir(destination), file.Name()))
		if err != nil {
			return fmt.Errorf("read imported terminal theme %s: %w", file.Name(), err)
		}
		if other.Name == name {
			return fmt.Errorf("配色名称 %q 已由文件 %q 使用，请修改 name 后导入", name, file.Name())
		}
	}
	return nil
}

func writeTerminalFile(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create terminal configuration directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".terminal-*")
	if err != nil {
		return fmt.Errorf("create pending terminal configuration: %w", err)
	}
	closed, committed := false, false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close pending terminal configuration: %w", closeErr))
			}
		}
		if !committed {
			if removeErr := os.Remove(file.Name()); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("remove pending terminal configuration: %w", removeErr))
			}
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write pending terminal configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync pending terminal configuration: %w", err)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return fmt.Errorf("close pending terminal configuration: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace terminal configuration: %w", err)
	}
	committed = true
	return nil
}
