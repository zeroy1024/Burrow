package desktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type shortcutAction struct{ id, label, group, binding string }

func defaultShortcutActions() []shortcutAction {
	return []shortcutAction{
		{"session.new", locales.MessageNewSession(), locales.MessageSessions(), "Cmd+N"},
		{"tab.close", locales.MessageClosePaneOrTab(), locales.MessageSessions(), "Cmd+W"},
		{"tab.reopen", locales.MessageReopenClosedTab(), locales.MessageSessions(), "Cmd+Shift+T"},
		{"session.reconnect", locales.MessageReconnectCurrentSession(), locales.MessageSessions(), "Cmd+Shift+R"},
		{"tab.select.1", locales.MessageSelectPaneOrTab(), locales.MessageSessions(), "Cmd+1"},
		{"tab.select.2", locales.MessageSelectPaneOrTab2(), locales.MessageSessions(), "Cmd+2"},
		{"tab.select.3", locales.MessageSelectPaneOrTab3(), locales.MessageSessions(), "Cmd+3"},
		{"tab.select.4", locales.MessageSelectPaneOrTab4(), locales.MessageSessions(), "Cmd+4"},
		{"tab.prev", locales.MessagePreviousPaneOrTab(), locales.MessageSessions(), "Cmd+["},
		{"tab.next", locales.MessageNextPaneOrTab(), locales.MessageSessions(), "Cmd+]"},
		{"sidebar.toggle", locales.MessageToggleSidebar(), locales.MessageView2(), "Cmd+\\"},
		{"mode.terminal", locales.MessageTerminalMode(), locales.MessageView2(), "Cmd+Alt+1"},
		{"mode.files", locales.MessageFileTransferMode(), locales.MessageView2(), "Cmd+Alt+2"},
		{"pane.split", locales.MessageSplitPane(), locales.MessageView2(), "Cmd+D"},
		{"host.search", locales.MessageSearchCurrentPage(), locales.MessageSearch(), "Cmd+F"},
		{"app.settings", locales.SettingsTitle(), locales.MessageApp(), "Cmd+,"},
	}
}

func (a *app) shortcutBinding(id string) string {
	if binding, ok := a.prefs.Keymap[id]; ok {
		return binding
	}
	for _, action := range defaultShortcutActions() {
		if action.id == id {
			return action.binding
		}
	}
	return ""
}

func (a *app) shortcutSettings(c *ui.Context) {
	var groups []string
	actions := map[string][]shortcutAction{}
	for _, action := range defaultShortcutActions() {
		if !a.settingMatches(action.label, action.group+locales.MessageKeyboardShortcuts()) {
			continue
		}
		if _, ok := actions[action.group]; !ok {
			groups = append(groups, action.group)
		}
		actions[action.group] = append(actions[action.group], action)
	}
	if len(groups) == 0 {
		return
	}
	ui.Row(c).Key("shortcut-toolbar").FillWidth().Justify(ui.End).Children(func() {
		uikit.Button(c, locales.MessageResetAll(), uikit.ButtonProps{Disabled: len(a.prefs.Keymap) == 0}).OnClick(func() {
			a.prefs.Keymap = map[string]string{}
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
			a.preferencesChanged()
		})
	})
	for _, group := range groups {
		help := ""
		if group == locales.MessageSessions() {
			help = locales.MessageWithMultipleSshPanesSwitchPanesWithOnePaneOrInFileTransfer()
		}
		uikit.SettingsGroup(c, uikit.SettingsGroupProps{Key: group, Title: group, Help: help}, func() {
			for index, action := range actions[group] {
				a.shortcutRow(c, action, index > 0)
			}
		})
	}
}

func (a *app) shortcutRow(c *ui.Context, action shortcutAction, divider bool) {
	t := theme.Current()
	p := t.C.Settings
	row := ui.Column(c).Key(action.id).FillWidth()
	// Recording ends once the focus leaves the row: a click elsewhere moves it
	// or clears it. The row holds the conflict's Replace button too.
	if a.settingsUI.recording == action.id && !row.FocusWithin() {
		a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
	}
	recording := a.settingsUI.recording == action.id
	conflict := ""
	if recording && a.settingsUI.conflictAction != "" {
		index := slices.IndexFunc(defaultShortcutActions(), func(v shortcutAction) bool { return v.id == a.settingsUI.conflictAction })
		conflict = fmt.Sprintf(locales.MessageConflictsWith(), defaultShortcutActions()[index].label)
	}
	binding := a.shortcutBinding(action.id)
	row.Children(func() {
		uikit.SettingsRow(c, uikit.SettingsRowProps{
			Gap: p.RowGap, Divider: divider,
			Text: func() {
				a.settingsText(c, action.label, uikit.TextProps{}, p.RowLabelSize)
				if conflict == "" {
					return
				}
				ui.Row(c).FillWidth().Gap(t.C.ShortcutField.ControlGap).Children(func() {
					uikit.FieldMessage(c, conflict, uikit.FieldMessageProps{Tone: uikit.ToneDanger}).Grow(1)
					uikit.Button(c, locales.MessageReplace(), uikit.ButtonProps{Size: uikit.SizeSm}).OnClick(func() {
						a.prefs.Keymap[a.settingsUI.conflictAction] = ""
						a.storeShortcut(action.id, a.settingsUI.pendingBinding)
					})
				})
			},
			Control: func() {
				ui.Row(c).Shrink(0).Gap(t.C.ShortcutField.ControlGap).AlignItems(ui.Center).Children(func() {
					a.shortcutField(c, action, binding, recording, conflict)
					size := t.C.IconButton.SizeSm
					if binding == action.binding {
						ui.Box(c).Size(size, size)
						return
					}
					uikit.IconButton(
						c,
						uikit.IconRetry,
						uikit.IconButtonProps{Key: "reset", Label: locales.MessageReset(), Size: uikit.SizeSm},
					).
						Label(locales.MessageReset2() + action.label).OnClick(func() { a.storeShortcut(action.id, action.binding) })
				})
			},
		})
	})
}

func (a *app) shortcutField(c *ui.Context, action shortcutAction, binding string, recording bool, conflict string) {
	props := uikit.ShortcutFieldProps{Label: action.label + " " + locales.MessageKeyboardShortcuts(), Recording: recording, Error: conflict}
	switch {
	case conflict != "":
		props.Keys = uikit.ShortcutKeys(a.settingsUI.pendingBinding)
	case recording:
		props.Text = locales.MessagePressANewShortcut()
	case binding == "":
		props.Text = locales.MessageUnassigned()
	default:
		props.Keys = uikit.ShortcutKeys(binding)
	}
	field := uikit.ShortcutField(c.Key("binding"), props)
	if field.Clicked() {
		a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = action.id, "", ""
		// Take the focus once; the row checks it in later frames.
		field.Focus()
	}
	if !recording {
		return
	}
	field.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown {
			return false
		}
		if ev.Key == ui.KeyEscape {
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
			return true
		}
		if ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete {
			a.storeShortcut(action.id, "")
			return true
		}
		binding, ok := keyToBinding(ev.Mods, ev.Key)
		if !ok {
			return true
		}
		a.storeShortcut(action.id, binding)
		return true
	})
}

func (a *app) storeShortcut(id, binding string) {
	if binding != "" {
		for _, candidate := range defaultShortcutActions() {
			if candidate.id != id && a.shortcutBinding(candidate.id) == binding {
				a.settingsUI.recording = id
				a.settingsUI.pendingBinding = binding
				a.settingsUI.conflictAction = candidate.id
				return
			}
		}
	}
	for _, action := range defaultShortcutActions() {
		if action.id == id && binding == action.binding {
			delete(a.prefs.Keymap, id)
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
			a.preferencesChanged()
			return
		}
	}
	a.prefs.Keymap[id] = binding
	a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
	a.preferencesChanged()
}

var shortcutKeys = map[string]ui.Key{
	"Enter": ui.KeyEnter, "Space": ui.KeySpace, "Tab": ui.KeyTab, "Backspace": ui.KeyBackspace, "Delete": ui.KeyDelete,
	"Left": ui.KeyLeft, "Right": ui.KeyRight, "Up": ui.KeyUp, "Down": ui.KeyDown,
	"[": ui.KeyBracketLeft, "]": ui.KeyBracketRight, "\\": ui.KeyBackslash, ",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash,
	"-": ui.KeyMinus, "=": ui.KeyEqual, ";": ui.KeySemicolon, "'": ui.KeyQuote, "`": ui.KeyBackquote,
	"0": ui.Key0, "1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4, "5": ui.Key5, "6": ui.Key6, "7": ui.Key7, "8": ui.Key8, "9": ui.Key9,
	"A": ui.KeyA, "B": ui.KeyB, "C": ui.KeyC, "D": ui.KeyD, "E": ui.KeyE, "F": ui.KeyF, "G": ui.KeyG, "H": ui.KeyH, "I": ui.KeyI, "J": ui.KeyJ, "K": ui.KeyK, "L": ui.KeyL, "M": ui.KeyM, "N": ui.KeyN, "O": ui.KeyO, "P": ui.KeyP, "Q": ui.KeyQ, "R": ui.KeyR, "S": ui.KeyS, "T": ui.KeyT, "U": ui.KeyU, "V": ui.KeyV, "W": ui.KeyW, "X": ui.KeyX, "Y": ui.KeyY, "Z": ui.KeyZ,
	"F1": ui.KeyF1, "F2": ui.KeyF2, "F3": ui.KeyF3, "F4": ui.KeyF4, "F5": ui.KeyF5, "F6": ui.KeyF6, "F7": ui.KeyF7, "F8": ui.KeyF8, "F9": ui.KeyF9, "F10": ui.KeyF10, "F11": ui.KeyF11, "F12": ui.KeyF12,
}

func bindingToKey(binding string) (ui.Modifiers, ui.Key, bool) {
	parts := strings.Split(binding, "+")
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Cmd":
			mods |= ui.Cmd
		case "Ctrl":
			mods |= ui.Ctrl
		case "Alt":
			mods |= ui.Alt
		case "Shift":
			mods |= ui.Shift
		case "Super":
			mods |= ui.Super
		default:
			return 0, 0, false
		}
	}
	key, ok := shortcutKeys[parts[len(parts)-1]]
	return mods, key, ok
}

func keyToBinding(mods ui.Modifiers, key ui.Key) (string, bool) {
	label := ""
	for text, candidate := range shortcutKeys {
		if candidate == key {
			label = text
			break
		}
	}
	if label == "" {
		return "", false
	}
	var parts []string
	if mods&ui.Cmd != 0 {
		parts = append(parts, "Cmd")
		mods &^= ui.Cmd
	}
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Super != 0 {
		parts = append(parts, "Super")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, label), "+"), true
}

func shortcutDisplay(binding string) string {
	return uikit.ShortcutDisplay(binding)
}
