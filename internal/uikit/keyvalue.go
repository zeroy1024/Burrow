package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type KV struct {
	Key, Value      string
	Mono, ValueMono bool
	Dim             bool
	Render          func()
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
				if value.Render != nil {
					value.Render()
					return
				}
				ink := p.Value
				if value.Dim {
					ink = p.ValueDim
				}
				val := ui.Text(c, value.Value).FontSize(p.Font).TextColor(Color(ink)).FontFeatures("tnum").SingleLine().Ellipsis("…").MinWidth(0)
				if value.ValueMono {
					val.Font(strings.Join(t.Font.Family.Mono, ", "))
				}
			})
		}
	})
}

// KeyValueGrid renders key-value metadata pairs in a multi-column grid,
// used by detail drawers for compact structured inspection.
func KeyValueGrid(c *ui.Context, columns int, rows []KV) ui.Element {
	t := theme.Current()
	p := t.C.KeyValue
	if columns < 1 {
		columns = 1
	}
	return ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
		for start := 0; start < len(rows); start += columns {
			end := min(start+columns, len(rows))
			ui.Row(c).Key(start).FillWidth().Gap(t.Space.N16).AlignItems(ui.Center).Children(func() {
				for _, item := range rows[start:end] {
					ui.Row(c).Key(item.Key).Grow(1).Basis(0).MinWidth(0).MinHeight(p.RowHeight).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
						name := ui.Text(c, item.Key).Width(t.Size.ControlLg * 1.8).Shrink(0).FontSize(p.Font).TextColor(Color(p.Key)).SingleLine()
						if item.Mono {
							name.Font(strings.Join(t.Font.Family.Mono, ", "))
						}
						if item.Render != nil {
							ui.Row(c).Grow(1).MinWidth(0).Gap(t.Space.N6).AlignItems(ui.Center).Children(item.Render)
							return
						}
						ink := p.Value
						if item.Dim {
							ink = p.ValueDim
						}
						val := ui.Text(c, item.Value).Grow(1).MinWidth(0).FontSize(p.Font).TextColor(Color(ink)).FontFeatures("tnum").SingleLine().Ellipsis("…")
						if item.ValueMono {
							val.Font(strings.Join(t.Font.Family.Mono, ", "))
						}
					})
				}
				for pad := end - start; pad < columns; pad++ {
					ui.Box(c).Grow(1).Basis(0).MinWidth(0)
				}
			})
		}
	})
}
