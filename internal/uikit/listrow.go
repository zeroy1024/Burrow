package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ListRowProps struct {
	Key                    any
	Label                  string
	Title, Meta            string
	Leading                func()
	Table                  bool
	FocusVisible           bool
	Odd, Selected, Focused bool
}

func ListRow(c *ui.Context, props ListRowProps) ui.Element {
	t := theme.Current()
	p := t.C.ListRow
	row := ui.Row(c).Key(props.Key).Height(p.Height).FillWidth().Shrink(0).
		Padding(0, p.PaddingX).Gap(p.Gap).Radius(p.Radius).Label(props.Label).Role(ui.RoleListItem).Selected(props.Selected)
	if props.Table {
		row.Padding(0, t.C.FileList.RowPaddingX).Gap(t.C.FileList.ColumnGap).Radius(t.C.FileList.RowRadius).Role(ui.RoleRow)
	}
	background, text := ui.Transparent, Color(t.Color.Text.Primary)
	if props.Odd {
		background = Color(t.Color.Surface.Zebra)
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
	row.Background(row.AnimateColor("selection", background, animation)).
		TextColor(row.AnimateColor("selection-text", text, animation))
	row.FocusRing(false)
	if props.FocusVisible || row.FocusVisible() {
		Shadow(row, t.C.Button.FocusShadow)
	}
	if props.Title != "" || props.Meta != "" || props.Leading != nil {
		row.Children(func() {
			if props.Leading != nil {
				props.Leading()
			}
			ui.Text(c, props.Title).FontSize(t.Font.Size.Body).SingleLine().Grow(1).MinWidth(0)
			if props.Meta != "" {
				ink := Color(p.Meta)
				if props.Selected {
					ink = text
				}
				ui.Text(c, props.Meta).FontSize(t.Font.Size.Footnote).TextColor(ink).SingleLine().FontFeatures("tnum")
			}
		})
	}
	return row
}
