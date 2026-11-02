package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sshdesk/internal/termtheme"
)

func terminalSettingsForTest(path string) terminalSettingsState {
	return terminalSettingsState{
		mode: termtheme.FollowAppTheme(), prefs: termtheme.DefaultPrefs(),
		followLight: termtheme.DefaultLight, followDark: termtheme.DefaultDark, path: path,
	}
}

func TestTerminalSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.json")
	a := &app{terminal: terminalSettingsForTest(path)}
	a.terminal.mode = termtheme.Fixed("Dracula")
	a.terminal.followLight, a.terminal.followDark = "Nord", "Dracula"
	a.terminal.prefs.Background = termtheme.BackgroundCard
	a.terminal.prefs.FontSize = 14
	a.terminal.prefs.LineHeight = 22
	a.terminal.prefs.CursorShape = termtheme.CursorBar
	a.terminal.prefs.CursorBlink = false
	a.terminal.prefs.BoldIsBright = true
	if err := a.saveTerminalSettings(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"mode": "fixed:Dracula"`)) || !bytes.Contains(data, []byte(`"background": "card"`)) {
		t.Fatalf("settings do not use string encodings: %s", data)
	}
	restarted := &app{terminal: terminalSettingsForTest(path)}
	if err := restarted.readTerminalSettings(); err != nil {
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
	configPath := filepath.Join(directory, "config", "terminal.json")
	a := &app{terminal: terminalSettingsForTest(configPath)}
	if err := a.importTerminalTheme(path); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "config", "themes", filepath.Base(path))
	copied, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, data) {
		t.Fatal("import changed the source bytes")
	}
	if a.terminal.mode != termtheme.Fixed(filepath.Base(path)) {
		t.Fatalf("unnamed theme lost its filename: %v", a.terminal.mode)
	}
	restarted := &app{terminal: terminalSettingsForTest(configPath)}
	if err := restarted.loadTerminalThemes(); err != nil {
		t.Fatal(err)
	}
	if err := restarted.readTerminalSettings(); err != nil {
		t.Fatal(err)
	}
	scheme := termtheme.Resolve(restarted.terminal.mode, false)
	if scheme.Name != filepath.Base(path) || termtheme.Hex(scheme.Background) != "#123456" {
		t.Fatalf("imported theme changed after restart: %#v", scheme)
	}
	if err := a.importTerminalTheme(path); err != nil {
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
	a := &app{terminal: terminalSettingsForTest(filepath.Join(directory, "config", "terminal.json"))}
	if err := a.importTerminalTheme(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("background = #654321\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.importTerminalTheme(path); err == nil || !strings.Contains(err.Error(), "同名配色文件") {
		t.Fatalf("expected filename conflict, got %v", err)
	}
	copied, err := os.ReadFile(filepath.Join(directory, "config", "themes", filepath.Base(path)))
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
	a := &app{terminal: terminalSettingsForTest(filepath.Join(directory, "config", "terminal.json"))}
	for index, filename := range []string{"first", "second"} {
		path := filepath.Join(directory, filename)
		if err := os.WriteFile(path, []byte("# name: Duplicate Test Theme\nbackground = #123456\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := a.importTerminalTheme(path)
		if index == 0 && err != nil {
			t.Fatal(err)
		}
		if index == 1 && (err == nil || !strings.Contains(err.Error(), "配色名称")) {
			t.Fatalf("expected scheme name conflict, got %v", err)
		}
	}
}
