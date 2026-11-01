package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type KV struct {
	Key, Value string
	Mono, Dim  bool
}

func KeyValue(c *ui.Context, rows []KV) ui.Element {
	t := theme.Current()
	p := t.C.KeyValue
	return ui.Column(c).FillWidth().Children(func() {
		for _, value := range rows {
			ui.Row(c).Key(value.Key).FillWidth().MinHeight(p.RowHeight).Gap(t.Space.N8).Children(func() {
				name := ui.Text(c, value.Key).FontSize(p.Font).TextColor(Color(p.Key)).Shrink(0)
				if value.Mono {
					name.Font(strings.Join(t.Font.Family.Mono, ", "))
				}
				ui.Spacer(c)
				ink := p.Value
				if value.Dim {
					ink = p.ValueDim
				}
				ui.Text(c, value.Value).FontSize(p.Font).TextColor(Color(ink)).FontFeatures("tnum").SingleLine().Ellipsis("…").MinWidth(0)
			})
		}
	})
}
