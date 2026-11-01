package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type DividerProps struct {
	Vertical bool
	Inset    float32
}

func Divider(c *ui.Context, options ...DividerProps) ui.Element {
	props := DividerProps{}
	if len(options) != 0 {
		props = options[0]
	}
	p := theme.Current().C.Divider
	e := ui.Box(c).Shrink(0).Background(Color(p.Color)).PassThrough()
	if props.Vertical {
		return e.Width(p.Width).Height(p.VerticalHeight).Margin(props.Inset, 0)
	}
	return e.Height(p.Width).FillWidth().Margin(0, props.Inset)
}
