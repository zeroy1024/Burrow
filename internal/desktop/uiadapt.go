package desktop

import (
	"fmt"
	"image/color"

	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

// Window colors use the native API's string format rather than a UI element.
func windowColor(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// Shortcut policy and stable page handles stay with the application.
func (a *app) sidebarSearch(c *ui.Context, value *string, focus *ui.Handle, label, placeholder string) {
	uikit.SearchField(c, value, uikit.SearchProps{Label: label, Placeholder: placeholder,
		Shortcut: shortcutDisplay(a.shortcutBinding("host.search")), Icon: searchIcon, Handle: focus})
}
