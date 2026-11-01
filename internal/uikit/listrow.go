package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ListRowProps struct {
	Key                    any
	Label                  string
	Odd, Selected, Focused bool
}

func ListRow(c *ui.Context, props ListRowProps) ui.Element {
	t := theme.Current()
	p := t.C.FileList
	row := ui.Row(c).Key(props.Key).Height(p.RowHeight).FillWidth().Shrink(0).
		Padding(0, p.RowPaddingX).Gap(p.RowGap).Radius(p.RowRadius).Label(props.Label).Role(ui.RoleListItem)
	background, text := ui.Transparent, Color(t.Color.Text.Primary)
	if props.Odd {
		background = Color(p.Zebra)
	}
	if row.Hovered() {
		background = Color(t.Color.State.Hover)
	}
	if props.Selected {
		if props.Focused {
			background, text = Color(t.C.FilePaneFocus.SelBgActive), Color(t.C.FilePaneFocus.SelTextActive)
		} else {
			background, text = Color(t.C.FilePaneFocus.SelBgInactive), Color(t.C.FilePaneFocus.SelTextInactive)
		}
	}
	animation := Tween(t.C.FilePaneFocus.Transition)
	return row.Background(row.AnimateColor("selection", background, animation)).
		TextColor(row.AnimateColor("selection-text", text, animation))
}
