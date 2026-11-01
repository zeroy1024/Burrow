package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func StylePopover(panel ui.Element) ui.Element {
	t := theme.Current()
	return Shadow(panel.Radius(t.Radius.Popover).Background(Color(t.Color.Surface.Card)), t.C.Menu.Shadow)
}
