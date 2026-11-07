package uikit

import (
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SortHeaderProps struct {
	Columns  []Column
	State    *TableState
	Width    float32
	Scroller ui.Element
	Chevron  *ui.SVG
	// Translucent leaves the background to the table's shared glass underlay.
	Translucent bool
}

type SortHeaderParts struct {
	Root             ui.Element
	Buttons          []ui.Element
	Changed, Resized bool
}

func SortHeader(c *ui.Context, props SortHeaderProps) SortHeaderParts {
	t := theme.Current()
	parts := SortHeaderParts{Buttons: make([]ui.Element, len(props.Columns))}
	widths := resolveColumnWidths(props.Columns, props.State.List.Columns.Widths, props.Width-2*t.C.FilePane.ContentPadding)
	parts.Root = ui.Row(c).Height(t.C.FileList.HeaderHeight).Padding(0, t.C.FilePane.ContentPadding+t.C.FileList.RowPaddingX).
		Gap(t.C.FileList.ColumnGap).FillWidth().Background(Color(t.C.FileList.HeaderBg)).Role(ui.RoleRow)
	if props.Translucent {
		parts.Root.Background(ui.Transparent)
	}
	if props.Scroller.Valid() {
		parts.Root.ScrollWith(props.Scroller)
	}
	parts.Root.Children(func() {
		for index, column := range props.Columns {
			if column.MinWidth == 0 && column.Width == 0 {
				column.MinWidth = t.C.FileList.NameMinWidth
			}
			cell := ui.Box(c).Key(column.ID).Width(widths[index]).Height(t.C.FileList.HeaderHeight).Shrink(0).Role(ui.RoleColumnHeader).Label(column.Title)
			order, priority, sorted := columnOrder(column, props.State)
			if sorted {
				direction := locales.MessageAscending()
				if order.Descending {
					direction = locales.MessageDescending()
				}
				cell.Value(direction).Description(column.Title + "，" + direction)
			}
			cell.Children(func() {
				button := ui.ButtonBase(c).Key("sort").Fill().Justify(ui.Start).
					TextColor(Color(t.C.FileList.HeaderText)).Label(columnSortLabel(column, props.State))
				if column.Align == AlignEnd {
					button.Justify(ui.End)
				}
				if column.Align == AlignCenter {
					button.Justify(ui.Center)
				}
				button.Disabled(!column.Sortable || column.RowHeader)
				if button.Clicked() {
					props.State.Sorted(column.ID, button.ClickModifiers())
					parts.Changed = true
				}
				button.Children(func() {
					ui.Text(c, column.Title).FontSize(t.C.FileList.HeaderSize).SingleLine()
					if sorted {
						rotation := float32(180)
						if order.Descending {
							rotation = 0
						}
						ui.Icon(c, props.Chevron).Size(t.Font.Size.Caption, t.Font.Size.Caption).Rotate(rotation)
						if len(props.State.SortBy) > 1 {
							Count(c, priority+1, CountProps{}).FontSize(t.Font.Size.Caption2).TextColor(Color(t.C.FileList.HeaderText))
						}
					}
				})
				parts.Buttons[index] = button
				if !column.Fixed {
					gap := float32(0)
					if index < len(props.Columns)-1 {
						gap = t.C.FileList.ColumnGap
					}
					edge := ui.Box(c).Key("resize").Absolute().Top(0).Bottom(0).Right(-(gap + t.C.Window.Gutter) / 2).Width(t.C.Window.Gutter).
						Cursor(ui.CursorResizeEW).Focusable().FocusRing(false).Role(ui.RoleSplitter).Label(locales.TableResizeColumn(
						column.Title,
					)).Center()
					dx, _, dragging := edge.Dragged()
					width := widths[index]
					if dragging && dx != 0 {
						width += dx
					}
					if edge.Shortcut(0, ui.KeyLeft) {
						width -= t.C.Desktop.SiderKeyboardStep
					}
					if edge.Shortcut(0, ui.KeyRight) {
						width += t.C.Desktop.SiderKeyboardStep
					}
					if edge.DoubleClicked() {
						delete(props.State.List.Columns.Widths, column.ID)
						parts.Resized = true
					} else {
						width = max(column.MinWidth, width)
						if column.MaxWidth > 0 {
							width = min(width, column.MaxWidth)
						}
						if width != widths[index] {
							if props.State.List.Columns.Widths == nil {
								props.State.List.Columns.Widths = map[string]float32{}
							}
							// A preceding flexible column would otherwise absorb this delta,
							// leaving the dragged boundary in place. Keep its shown width.
							for previous := range index {
								prior := props.Columns[previous]
								if prior.Width <= 0 && !prior.Fixed {
									props.State.List.Columns.Widths[prior.ID] = widths[previous]
								}
							}
							props.State.List.Columns.Widths[column.ID] = width
							parts.Resized = true
						}
					}
					maximum := column.MaxWidth
					if maximum == 0 {
						maximum = max(props.Width, width)
					}
					edge.Range(float64(column.MinWidth), float64(maximum), float64(width))
					ink := Color(t.C.FileList.HeaderBorder)
					if edge.Hovered() || dragging || edge.FocusVisible() {
						ink = Color(t.Color.Accent.Default)
					}
					edge.Children(func() {
						Divider(c, DividerProps{Vertical: true}).Background(ink)
					})
				}
			})
		}
	})
	return parts
}
