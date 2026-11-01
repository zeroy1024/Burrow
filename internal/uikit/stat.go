package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func Stat(c *ui.Context, label, value string) ui.Element {
	t := theme.Current()
	return ui.Row(c).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Small)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(t.Font.Size.Caption).TextColor(Color(t.Color.Text.Secondary))
	})
}

func MeterStat(c *ui.Context, label, value string, percent float32) ui.Element {
	return ui.Column(c).Gap(theme.Current().Space.N7).Children(func() {
		Stat(c, label, value)
		Meter(c, percent)
	})
}
