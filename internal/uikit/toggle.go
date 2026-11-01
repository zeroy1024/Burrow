package uikit

import "github.com/egoist/mygo/ui"

// Switch keeps native sizing unless the caller selects the compact token size.
func Switch(c *ui.Context, value *bool, label string) ui.Element {
	return ui.Switch(c, value).Label(label)
}
