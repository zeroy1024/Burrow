package uikit

import (
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ColumnTextStyle uint8

const (
	ColumnText ColumnTextStyle = iota
	ColumnMetadata
	ColumnMonoMetadata
)

type Column struct {
	ID, Title, SortLabel string
	Width                float32
	HeaderMinWidth       float32
	Sortable             bool
	TextStyle            ColumnTextStyle
}

type TableProps struct {
	Count                      int
	HeaderHeight, FooterHeight float32
}

type TableParts struct {
	Root, Rows ui.Element
	Columns    []Column
}

// DataTable renders every row so caller-owned drag sources and offscreen
// reveal requests retain their existing lifetime. Selection and sorting stay
// with the caller; this component owns the scroll shell and column presentation.
func DataTable(
	c *ui.Context,
	scroll *ui.ScrollState,
	columns []Column,
	props TableProps,
	row func(int, TableParts),
	empty func(),
) TableParts {
	t := theme.Current()
	parts := TableParts{Columns: columns}
	parts.Root = ui.Scroll(c).Fill().TrackScroll(scroll).
		Padding(props.HeaderHeight, t.C.FilePane.ContentPadding, props.FooterHeight, t.C.FilePane.ContentPadding).
		ScrollbarInsets(props.HeaderHeight, 0, props.FooterHeight, 0)
	parts.Root.Children(func() {
		parts.Rows = ui.Column(c).FillWidth()
		parts.Rows.Children(func() {
			for index := range props.Count {
				row(index, parts)
			}
		})
		if props.Count == 0 && empty != nil {
			empty()
		}
	})
	return parts
}

// Cells shares column widths with SortHeader, retaining caller-provided order
// and text. A zero-width column takes the remaining space.
func (parts TableParts) Cells(c *ui.Context, values []string) {
	t := theme.Current()
	for index, column := range parts.Columns {
		cell := ui.Text(c, values[index]).FontSize(t.C.FileList.FontSize).SingleLine()
		if column.Width == 0 {
			cell.Grow(1).MinWidth(0)
		} else {
			cell.Width(column.Width)
		}
		if column.TextStyle != ColumnText {
			cell.TextColor(Color(t.C.FileList.Meta))
		}
		if column.TextStyle == ColumnMonoMetadata {
			cell.Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption)
		}
	}
}
