package uikit

import "github.com/egoist/mygo/ui"

// InputShield blocks a transient surface without applying Disabled's visual
// dimming. A modal shield also keeps keyboard and accessibility focus out of
// the content while an operation or a dialog exit is in progress.
func InputShield(c *ui.Context) ui.Element {
	return ui.Box(c).Absolute().Fill().Focusable().FocusRing(false).
		HandleInput(func(ui.InputEvent) bool { return true }).OnClick(func() {})
}
