package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func Stat(c *ui.Context, label, value string, body ...func()) ui.Element {
	t := theme.Current()
	return ui.Column(c).Gap(t.Space.N7).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			ui.Text(c, label).FontSize(t.Font.Size.Small)
			ui.Spacer(c)
			ui.Text(c, value).FontSize(t.Font.Size.Caption).TextColor(Color(t.Color.Text.Secondary)).FontFeatures("tnum")
		})
		for _, content := range body {
			content()
		}
	})
}

func MeterStat(c *ui.Context, label, value string, percent float32) ui.Element {
	return Stat(c, label, value, func() { Meter(c, float64(percent), 0, 100, MeterProps{Label: label, Value: value}) })
}
