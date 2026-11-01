package uikit

import "github.com/egoist/mygo/ui"

// Text leaves typography and color inherited until the caller selects a style.
func Text(c *ui.Context, text string) ui.Element { return ui.Text(c, text) }
