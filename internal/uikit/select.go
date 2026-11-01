package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func Select(c *ui.Context, label string, selected *string, options []string) ui.Element {
	return StyleFormControl(StyleSelect(ui.Select(c, selected, options), label), "")
}

func SelectNative(c *ui.Context, label string, selected *string, options []string) ui.Element {
	return ui.Select(c, selected, options).Label(label)
}

func StyleSelect(e ui.Element, label string) ui.Element {
	p := theme.Current().C.Input
	return e.Height(p.Height).FillWidth().MinWidth(0).Padding(0, p.PaddingX).Radius(p.Radius).
		Background(Color(p.Bg)).Border(p.BorderWidth, Color(p.Border)).FontSize(p.FontSize).Label(label)
}

func StyleSelectPopup(panel ui.Element) ui.Element {
	t := theme.Current()
	return Shadow(panel.Padding(t.Space.N4).Radius(t.C.Menu.Radius).Background(Color(t.C.Menu.Bg)), t.C.Menu.Shadow)
}
