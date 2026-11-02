package uikit

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// TableBackdropRow supplies real display data for rows just outside the list's
// viewport. It has no selection, drag, keyboard or accessibility ownership.
type TableBackdropRow struct {
	Values  []string
	Leading func(*ui.Painter, ui.Rect)
}

func tableCellContent(column Column, value string, width float32) (string, ui.Font) {
	t := theme.Current()
	font := ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: t.C.FileList.FontSize}
	if column.TextStyle == ColumnMonoMetadata {
		font.Family, font.Size = strings.Join(t.Font.Family.Mono, ", "), t.Font.Size.Caption
	}
	if column.Align == AlignEnd {
		font.Features = "tnum"
	}
	if column.MiddleEllipsis {
		value = MiddleText(value, width, font)
	}
	return value, font
}

// Paint against the live scroll offsets at paint time. The native List and its
// body bounds stay intact, including virtual rows, reveal, hit tests and the
// auto-scroll edge used by file drags. Only the two chrome bands are drawn.
func paintTableBackdrop(c *ui.Context, parts TableParts, scroll *ui.ScrollState, props TableProps) {
	ui.Box(c).Key("table-backdrop").Absolute().Fill().PassThrough().Role(ui.RoleNone).
		Draw(func(p *ui.Painter, frame ui.Rect) {
			t := theme.Current()
			height := t.C.ListRow.Height
			width := max(frame.W-2*t.C.FilePane.ContentPadding, sumColumnWidths(parts.Widths)+
				2*t.C.FileList.RowPaddingX+float32(max(0, len(parts.Columns)-1))*t.C.FileList.ColumnGap)
			origin := frame.Y + props.HeaderHeight - scroll.Y
			for _, band := range []ui.Rect{
				{X: frame.X, Y: frame.Y, W: frame.W, H: props.HeaderHeight},
				{X: frame.X, Y: frame.Y + frame.H - props.FooterHeight, W: frame.W, H: props.FooterHeight},
			} {
				if band.H <= 0 {
					continue
				}
				first := max(0, int(math.Floor(float64((band.Y-origin)/height))))
				last := min(props.Count, int(math.Ceil(float64((band.Y+band.H-origin)/height))))
				p.Clip(band, 0, func() {
					for index := first; index < last; index++ {
						row := ui.Rect{X: frame.X + t.C.FilePane.ContentPadding - scroll.X,
							Y: origin + float32(index)*height, W: width, H: height}
						selected := parts.State.Selection.Has(props.Key(index))
						background, ink := ui.Transparent, Color(t.Color.Text.Primary)
						if props.Zebra && index%2 != 0 {
							background = Color(t.Color.Surface.Zebra)
						}
						if selected {
							background, ink = Color(t.C.FilePaneFocus.SelBgInactive), Color(t.C.FilePaneFocus.SelTextInactive)
							if props.Focused {
								background, ink = Color(t.C.FilePaneFocus.SelBgActive), Color(t.C.FilePaneFocus.SelTextActive)
							}
						}
						p.Fill(row, background, t.C.FileList.RowRadius)
						data := props.Backdrop(index)
						x := row.X + t.C.FileList.RowPaddingX
						if data.Leading != nil {
							data.Leading(p, ui.Rect{X: x, Y: row.Y + (height-t.Size.Icon)/2, W: t.Size.Icon, H: t.Size.Icon})
						}
						for i, column := range parts.Columns {
							cell := ui.Rect{X: x + column.LeadingWidth, Y: row.Y, W: max(0, parts.Widths[i]-column.LeadingWidth), H: height}
							value, font := tableCellContent(column, data.Values[i], cell.W)
							layout := ui.ShapeText(value, font, 0)
							w, h := layout.Size()
							tx := cell.X
							if column.Align == AlignEnd {
								tx += cell.W - w
							} else if column.Align == AlignCenter {
								tx += (cell.W - w) / 2
							}
							color := ink
							if column.TextStyle != ColumnText && !selected {
								color = Color(t.C.FileList.Meta)
							}
							p.Clip(cell, 0, func() { p.TextLayout(layout, tx, cell.Y+(cell.H-h)/2, color) })
							x += parts.Widths[i] + t.C.FileList.ColumnGap
						}
					}
				})
			}
		})
}
