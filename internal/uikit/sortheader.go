package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SortHeaderProps struct {
	Columns    []Column
	Column     int
	Descending bool
	Chevron    *ui.SVG
}

type SortHeaderParts struct {
	Root    ui.Element
	Buttons []ui.Element
}

func SortHeader(c *ui.Context, props SortHeaderProps) SortHeaderParts {
	t := theme.Current()
	parts := SortHeaderParts{Buttons: make([]ui.Element, len(props.Columns))}
	parts.Root = ui.Row(c).Height(t.C.FileList.HeaderHeight).Padding(0, t.C.FilePane.ContentPadding+t.C.FileList.RowPaddingX).
		Gap(t.C.FileList.RowGap).FillWidth()
	parts.Root.Children(func() {
		for index, column := range props.Columns {
			button := ui.ButtonBase(c).Key(column.ID).Justify(ui.Start).
				TextColor(Color(t.C.FileList.HeaderText)).Label(column.SortLabel)
			if column.Width == 0 {
				button.Grow(1).MinWidth(column.HeaderMinWidth)
			} else {
				button.Width(column.Width).Shrink(0)
			}
			button.Disabled(!column.Sortable)
			button.Children(func() {
				ui.Text(c, column.Title).FontSize(t.C.FileList.HeaderSize).SingleLine()
				if props.Column == index {
					rotation := float32(180)
					if props.Descending {
						rotation = 0
					}
					ui.Icon(c, props.Chevron).Size(t.Font.Size.Caption, t.Font.Size.Caption).Rotate(rotation)
				}
			})
			parts.Buttons[index] = button
		}
	})
	return parts
}
