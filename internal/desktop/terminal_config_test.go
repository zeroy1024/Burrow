package desktop

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sshdesk/internal/locales"
	"sshdesk/internal/termtheme"
)

func terminalAppForTest(t *testing.T, path string) *app {
	t.Helper()
	a := catalogAppForTest(t, path)
	if err := a.loadTerminalSettings(); err != nil {
		t.Fatal(err)
	}
	if err := a.loadAppPreferences(); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestTerminalSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "burrow.db")
	a := terminalAppForTest(t, path)
	a.terminal.mode = termtheme.Fixed("Dracula")
	a.terminal.followLight, a.terminal.followDark = "Nord", "Dracula"
	a.terminal.prefs.Background = termtheme.BackgroundCard
	a.terminal.prefs.FontSize = 14
	a.terminal.prefs.LineHeight = 22
	a.terminal.prefs.CursorShape = termtheme.CursorBar
	a.terminal.prefs.CursorBlink = false
	a.terminal.prefs.BoldIsBright = true
	if err := a.flushAppPreferences(); err != nil {
		t.Fatal(err)
	}
	var data []byte
	_, err := a.store.ReadSetting(t.Context(), "preferences", &data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"termMode":"fixed:Dracula"`)) || !bytes.Contains(data, []byte(`"termBackground":"card"`)) {
		t.Fatalf("settings do not use string encodings: %s", data)
	}
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := terminalAppForTest(t, path)
	if err := restarted.loadAppPreferences(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.terminal.config(), restarted.terminal.config()) {
		t.Fatalf("settings did not survive restart: %#v", restarted.terminal.config())
	}
}

func TestImportTerminalThemePreservesBytesAndFilename(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "source", "Unnamed Test Theme")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte("# source: test fixture\nbackground = #123456\nforeground = #abcdef\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config", "burrow.db")
	a := terminalAppForTest(t, configPath)
	if err := a.importTerminalTheme(path); err != nil {
		t.Fatal(err)
	}
	if err := a.flushAppPreferences(); err != nil {
		t.Fatal(err)
	}
	themes, err := a.store.Themes(t.Context())
	var copied []byte
	if len(themes) > 0 {
		copied = themes[0].Source
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, data) {
		t.Fatal("import changed the source bytes")
	}
	if a.terminal.mode != termtheme.Fixed(filepath.Base(path)) {
		t.Fatalf("unnamed theme lost its filename: %v", a.terminal.mode)
	}
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := terminalAppForTest(t, configPath)
	if err := restarted.loadTerminalThemes(); err != nil {
		t.Fatal(err)
	}
	if err := restarted.loadAppPreferences(); err != nil {
		t.Fatal(err)
	}
	scheme := termtheme.Resolve(restarted.terminal.mode, false)
	if scheme.Name != filepath.Base(path) || termtheme.Hex(scheme.Background) != "#123456" {
		t.Fatalf("imported theme changed after restart: %#v", scheme)
	}
	if err := restarted.importTerminalTheme(path); err != nil {
		t.Fatalf("identical repeat import failed: %v", err)
	}
}

func TestImportTerminalThemeRejectsInvalidAndConflictingFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Test Theme")
	data := []byte("background = #123456\nforeground = #abcdef\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	a := terminalAppForTest(t, filepath.Join(directory, "config", "burrow.db"))
	if err := a.importTerminalTheme(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("background = #654321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.importTerminalTheme(path); err == nil || err.Error() != fmt.Sprintf(
		locales.MessageAColorSchemeFileNamedAlreadyExistsRenameItBeforeImporting(),
		"Test Theme",
	) {
		t.Fatalf("expected filename conflict, got %v", err)
	}
	themes, err := a.store.Themes(t.Context())
	var copied []byte
	if len(themes) > 0 {
		copied = themes[0].Source
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, data) {
		t.Fatal("conflicting import overwrote the existing theme")
	}
	if err := os.WriteFile(path, []byte("# broken fixture\nbackground = invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.importTerminalTheme(path); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected an error with a line number, got %v", err)
	}
}

func TestImportTerminalThemeRejectsDuplicateSchemeName(t *testing.T) {
	directory := t.TempDir()
	a := terminalAppForTest(t, filepath.Join(directory, "config", "burrow.db"))
	for index, filename := range []string{"first", "second"} {
		path := filepath.Join(directory, filename)
		if err := os.WriteFile(path, []byte("# name: Duplicate Test Theme\nbackground = #123456\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := a.importTerminalTheme(path)
		if index == 0 && err != nil {
			t.Fatal(err)
		}
		if index == 1 && (err == nil || err.Error() != fmt.Sprintf(
			locales.MessageColorSchemeNameIsAlreadyUsedByFile(),
			"Duplicate Test Theme",
			"first",
		)) {
			t.Fatalf("expected scheme name conflict, got %v", err)
		}
	}
}
func (a *app) flushAppPreferences() error {
	data, err := a.preferencesData()
	if err != nil {
		return err
	}
	return a.store.SaveSetting(a.ctx, "preferences", data)
}

func (a *app) importTerminalTheme(path string) error {
	if a.terminal.loadErr != nil {
		return a.terminal.loadErr
	}
	source, filename, err := a.persistImportedTheme(path)
	if err != nil {
		return err
	}
	a.activateImportedTheme(source, filename)
	return a.terminalError
}
