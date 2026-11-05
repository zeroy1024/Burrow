package desktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

type shortcutAction struct{ id, label, group, binding string }

var defaultShortcutActions = []shortcutAction{
	{"session.new", "新建会话", "会话", "Cmd+N"},
	{"tab.close", "关闭分屏或选项卡", "会话", "Cmd+W"},
	{"tab.reopen", "重新打开关闭的选项卡", "会话", "Cmd+Shift+T"},
	{"session.reconnect", "重新连接当前会话", "会话", "Cmd+Shift+R"},
	{"tab.select.1", "切换到第 1 个分屏或选项卡", "会话", "Cmd+1"},
	{"tab.select.2", "切换到第 2 个分屏或选项卡", "会话", "Cmd+2"},
	{"tab.select.3", "切换到第 3 个分屏或选项卡", "会话", "Cmd+3"},
	{"tab.select.4", "切换到第 4 个分屏或选项卡", "会话", "Cmd+4"},
	{"tab.prev", "上一个分屏或选项卡", "会话", "Cmd+["},
	{"tab.next", "下一个分屏或选项卡", "会话", "Cmd+]"},
	{"sidebar.toggle", "显示/隐藏侧栏", "视图", "Cmd+\\"},
	{"mode.terminal", "终端模式", "视图", "Cmd+Alt+1"},
	{"mode.files", "文件传输模式", "视图", "Cmd+Alt+2"},
	{"pane.split", "拆分窗格", "视图", "Cmd+D"},
	{"host.search", "搜索当前页面", "搜索", "Cmd+F"},
	{"app.settings", "设置", "应用", "Cmd+,"},
}

func (a *app) shortcutBinding(id string) string {
	if binding, ok := a.prefs.Keymap[id]; ok {
		return binding
	}
	for _, action := range defaultShortcutActions {
		if action.id == id {
			return action.binding
		}
	}
	return ""
}

func (a *app) shortcutSettings(c *ui.Context) {
	t := theme.Current()
	uikit.SettingsGroupTitle(c, localize(c, "快捷键"), func() {
		uikit.Button(c, localize(c, "全部恢复默认"), uikit.ButtonProps{}).OnClick(func() {
			a.prefs.Keymap = map[string]string{}
			a.settingsUI.recording, a.settingsUI.pendingBinding, a.settingsUI.conflictAction = "", "", ""
			a.preferencesChanged()
		})
	})
	uikit.FieldMessage(c,
		localize(c, "SSH 多分屏时优先切换分屏；单窗格及文件传输页切换选项卡。Ctrl+Tab / Ctrl+Shift+Tab 始终切换选项卡。"),
		uikit.FieldMessageProps{},
	)
	actions := make([]shortcutAction, 0, len(defaultShortcutActions))
	for _, action := range defaultShortcutActions {
		if a.settingMatches(action.label, action.group+"快捷键") {
			actions = append(actions, action)
		}
	}
	actionKey := func(row int) any { return actions[row].id }
	uikit.ShortcutTable(c, &a.settingsUI.shortcutTable, localize(c, "快捷键"), localize(c, "操作"), localize(c, "快捷键"), len(actions), actionKey, func(row, col int) {
		action := actions[row]
		switch col {
		case 0:
			uikit.Text(c, localize(c, action.label), uikit.TextProps{})
		case 1:
			binding := a.shortcutBinding(action.id)
			label := shortcutDisplay(binding)
			if binding == "" {
				label = "未设置"
			}
			recording := a.settingsUI.recording == action.id
			if recording {
				label = "按下新的快捷键…"
			}
			buttonLabel := localize(c, label)
			props := uikit.ButtonProps{}
			if binding != "" && !recording {
				props.Kbd = label
				buttonLabel = ""
			}
			item := uikit.Button(c.Key(action.id), buttonLabel, props).Font(strings.Join(t.Font.Family.Mono, ", ")).Label(localize(c, action.label) + " " + localize(c, "快捷键")).
				OnClick(func() {
					a.settingsUI.recording = action.id
					a.settingsUI.pendingBinding = ""
					a.settingsUI.conflictAction = ""
				})
			if recording {
				uikit.Shadow(item, t.Shadow.InputFocus)
				item.Focus().HandleInput(func(ev ui.InputEvent) bool {
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
			if recording && a.settingsUI.conflictAction != "" {
				index := slices.IndexFunc(defaultShortcutActions, func(v shortcutAction) bool { return v.id == a.settingsUI.conflictAction })
				uikit.Text(c, localize(c, fmt.Sprintf(localize(c, "与「%s」冲突"), localize(c, defaultShortcutActions[index].label))), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneDanger})
				uikit.Button(c, localize(c, "替换"), uikit.ButtonProps{}).OnClick(func() {
					a.prefs.Keymap[a.settingsUI.conflictAction] = ""
					a.storeShortcut(action.id, a.settingsUI.pendingBinding)
				})
			}
		case 2:
			uikit.Button(c, localize(c, "恢复默认"), uikit.ButtonProps{}).Label(localize(c, "恢复默认：") + localize(c, action.label)).OnClick(func() { a.storeShortcut(action.id, action.binding) })
		}
	})
}

func (a *app) storeShortcut(id, binding string) {
	if binding != "" {
		for _, candidate := range defaultShortcutActions {
			if candidate.id != id && a.shortcutBinding(candidate.id) == binding {
				a.settingsUI.recording = id
				a.settingsUI.pendingBinding = binding
				a.settingsUI.conflictAction = candidate.id
				return
			}
		}
	}
	for _, action := range defaultShortcutActions {
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
