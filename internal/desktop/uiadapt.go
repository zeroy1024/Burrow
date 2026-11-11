package desktop

import (
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

// Shortcut policy and stable page handles stay with the application.
func (a *app) sidebarSearch(c *ui.Context, value *string, focus *ui.Handle, label, placeholder string) {
	uikit.SearchField(c, value, uikit.SearchProps{Label: label, Placeholder: placeholder,
		Kbd: shortcutDisplay(a.shortcutBinding("host.search")), Icon: searchIcon, Handle: focus})
}
