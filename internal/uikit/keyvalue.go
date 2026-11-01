package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type KeyValueProps struct {
	Mono bool
}

func KeyValue(c *ui.Context, label, value string, props KeyValueProps) ui.Element {
	t := theme.Current()
	row := ui.Row(c)
	if !props.Mono {
		row.Gap(t.Space.N8)
	}
	return row.Children(func() {
		name := ui.Text(c, label)
		if props.Mono {
			name.Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption)
		} else {
			name.FontSize(t.Font.Size.Caption).TextColor(Color(t.Color.Text.Secondary)).Shrink(0)
		}
		ui.Spacer(c)
		text := ui.Text(c, value).FontSize(t.Font.Size.Caption)
		if props.Mono {
			text.TextColor(Color(t.Color.Text.Secondary))
		} else {
			text.SingleLine().Ellipsis("…").MinWidth(0)
		}
	})
}
