package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type IconButtonProps struct {
	Label string
	Key   any
}

// Icon leaves layout and inherited color to the caller, as the native primitive does.
func Icon(c *ui.Context, icon *ui.SVG) ui.Element { return ui.Icon(c, icon) }

func IconButton(c *ui.Context, icon *ui.SVG, props IconButtonProps) ui.Element {
	t := theme.Current()
	key := props.Key
	if key == nil {
		key = props.Label
	}
	button := StyleIconButton(ui.ButtonBase(c).Key(key).Label(props.Label), t)
	Tip(c, button, props.Label)
	button.Children(func() { ui.Icon(c, icon).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
	return button
}
