package uikit

import (
	"fmt"
	"slices"

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
	ID, Title, SortLabel                       string
	Width, MinWidth, MaxWidth, LeadingWidth    float32
	Align                                      Align
	Sortable, Fixed, RowHeader, MiddleEllipsis bool
	TextStyle                                  ColumnTextStyle
}

// TableState keeps application-owned choices, sorting and widths with the
// virtual list. Keys are complete item identities, never their display names.
type TableState struct {
	List        ui.ListState
	Selection   ui.Selection[string]
	Cursor      int
	Sort        ui.SortOrder
	SortBy      []ui.SortOrder
	dragging    bool
	choice      string
	anchor      string
	cursorKey   string
	initialized bool
}

type TableProps struct {
	Count                      int
	HeaderHeight, FooterHeight float32
	Width                      float32
	Focused, Zebra             bool
	Label                      string
	Key                        func(int) string
	RowLabel                   func(int) string
	Reveal                     string
	Menu                       func(*ui.Menu, int)
	DropTarget                 func(int) bool
	Underlay                   func()
	Backdrop                   func(int) TableBackdropRow
}

type TableParts struct {
	Root, Frame ui.Element
	Columns     []Column
	Widths      []float32
	State       *TableState
	RowKey      string
	Focused     bool
}

type TableRow struct {
	Context           *ui.Context
	Element           ui.Element
	Index             int
	Key               string
	Selected, Focused bool
}

type TableResult struct {
	TableParts
	Open, Menu, Preview, Delete int
}

type TableViewportProps struct {
	Label              string
	MinWidth, PaddingY float32
	Header             func()
	// Empty, set when the table has no rows, takes the place of body. It is
	// centred in the visible body rather than in the rows, which are as wide
	// as MinWidth and start at the top.
	Empty func()
}

func TableViewport(c *ui.Context, scroll *ui.ScrollState, props TableViewportProps, body func()) ui.Element {
	t := theme.Current()
	root := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).FillWidth()
	contentWidth := max(props.MinWidth, root.LayoutBounds().W)
	root.DrawOver(func(_ *ui.Painter, bounds ui.Rect) {
		if max(props.MinWidth, bounds.W) != contentWidth {
			c.Invalidate()
		}
	})
	return root.Children(func() {
		header := ui.Box(c).Key("header").FillWidth().MinWidth(0).Shrink(0).ClipX()
		viewport := ui.Box(c).Key("body").Grow(1).MinWidth(0).MinHeight(0).FillWidth()
		var scroller ui.Element
		viewport.Children(func() {
			scroller = ui.ScrollBoth(c).Fill().TrackScroll(scroll).Label(props.Label).
				Children(func() {
					rows := ui.Column(c).Width(contentWidth).Shrink(0).
						Padding(props.PaddingY, tableViewportInset(t))
					if props.Empty == nil {
						rows.Children(body)
					}
				})
			if props.Empty != nil {
				ui.Box(c).Key("empty").Absolute().Top(0).Bottom(0).Left(0).Right(0).
					Padding(props.PaddingY, tableViewportInset(t)).Center().Clip().PassThrough().Children(props.Empty)
			}
			Scrollbars(c, viewport, scroller, scroll, ScrollbarProps{
				Label: props.Label + "滚动条", Horizontal: true, Vertical: true,
			})
		})
		header.Children(func() {
			ui.Column(c).Width(contentWidth).Shrink(0).
				ScrollWith(scroller).Children(props.Header)
		})
	})
}

// tableViewportInset is the side padding TableViewport keeps around its rows.
func tableViewportInset(t *theme.Theme) float32 { return t.Space.N10 }

// TableToolbar holds a TableViewport table's filters above its column header.
func TableToolbar(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Column(c).FillWidth().Shrink(0).Padding(t.Space.N8, t.Space.N14).Gap(t.Space.N8).
		BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(Color(t.C.FileList.HeaderBorder))
}

// TableHeaderRow titles the columns of a TableViewport table. Its rows are
// ListRow{Table: true} with a TableCell per column, so each width is written
// once and the header and rows share the same padding and gaps.
func TableHeaderRow(c *ui.Context, columns []Column) ui.Element {
	t := theme.Current()
	p := t.C.FileList
	return ui.Row(c).FillWidth().Height(p.HeaderHeight).Shrink(0).Role(ui.RoleRow).
		Padding(0, tableViewportInset(t)+p.RowPaddingX).Gap(p.ColumnGap).
		BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(Color(p.HeaderBorder)).Children(func() {
		for _, column := range columns {
			TableCell(c, column).Role(ui.RoleColumnHeader).Label(column.Title).Children(func() {
				ui.Text(c, column.Title).FontSize(p.HeaderSize).TextColor(Color(p.HeaderText)).SingleLine().Ellipsis("…")
			})
		}
	})
}

// TableCell contains one cell of a TableViewport row: its column's width, or
// the remaining width when the column has none. Stacked lines share one gap.
func TableCell(c *ui.Context, column Column) ui.Element {
	cell := ui.Column(c).Key(column.ID).Gap(theme.Current().Space.N2)
	if column.Width > 0 {
		cell.Width(column.Width).Shrink(0)
	} else {
		cell.Grow(1).MinWidth(0)
	}
	switch column.Align {
	case AlignEnd:
		cell.AlignItems(ui.End)
	case AlignCenter:
		cell.AlignItems(ui.Center)
	}
	return cell
}

// DataTable virtualizes rows while keeping selection, ordering and scroll
// state with its caller. Row builders only describe visible presentation.
func DataTable(
	c *ui.Context, state *TableState, scroll *ui.ScrollState, columns []Column,
	props TableProps, row func(TableRow, TableParts), empty func(),
) TableResult {
	t := theme.Current()
	if props.HeaderHeight == 0 {
		props.HeaderHeight = t.C.FileList.HeaderHeight
	}
	result := TableResult{Open: -1, Menu: -1, Preview: -1, Delete: -1}
	parts := TableParts{Columns: columns, State: state}
	parts.Frame = ui.Box(c).Fill().MinWidth(0).MinHeight(0).Role(ui.RoleTable).Label(props.Label).
		Description(fmt.Sprintf("%d 行，%d 列，已选 %d 项；支持多选", props.Count, len(columns), state.Selection.Len()))
	if scroll == nil {
		scroll = ui.Local(parts.Frame, "uikit.table.scroll", func() ui.ScrollState { return ui.ScrollState{} })
	}
	width := parts.Frame.Bounds().W
	if width == 0 {
		width = props.Width
	}
	parts.Frame.DrawOver(func(_ *ui.Painter, rect ui.Rect) {
		if rect.W != width {
			c.Invalidate()
		}
	})
	if !state.initialized {
		if state.Selection.Len() == 0 {
			state.Cursor = -1
		}
		state.initialized = true
	}
	wasDragging := state.dragging
	state.dragging = false
	state.List.Key = func(index int) any { return props.Key(index) }
	state.List.Label = props.RowLabel
	// Custom Drag sources receive the pointer before the SDK's private row
	// wrapper. Keep a single selection in this adapter, avoiding two consumers.
	state.List.Selection, state.List.Selected = nil, nil
	state.List.Sort, state.List.SortBy = &state.Sort, &state.SortBy
	state.List.KeepOffset = true
	if state.cursorKey != "" && (state.Cursor < 0 || state.Cursor >= props.Count || props.Key(state.Cursor) != state.cursorKey) {
		state.Cursor = -1
		for index := range props.Count {
			if props.Key(index) == state.cursorKey {
				state.Cursor = index
				break
			}
		}
	}
	if state.choice != "" {
		for index := range props.Count {
			if props.Key(index) == state.choice {
				state.Cursor = index
				break
			}
		}
		state.choice = ""
	}
	if props.Reveal != "" {
		for index := range props.Count {
			if props.Key(index) == props.Reveal {
				state.List.ScrollIntoView(index)
				break
			}
		}
	}
	parts.Widths = resolveColumnWidths(columns, state.List.Columns.Widths, width-2*t.C.FilePane.ContentPadding)
	parts.Frame.Children(func() {
		parts.Root = ui.List(c.Key("rows"), &state.List, props.Count).
			Absolute().Top(props.HeaderHeight).Bottom(props.FooterHeight).Left(0).Right(0).
			Focusable().FocusRing(false).TrackScroll(scroll).Label(props.Label+"数据行").
			Padding(0, t.C.FilePane.ContentPadding).Gap(t.C.FileList.RowGap)
		parts.Root.RowWidth(sumColumnWidths(parts.Widths) + 2*t.C.FileList.RowPaddingX + float32(max(0, len(columns)-1))*t.C.FileList.ColumnGap)
		parts.Root.Rows(func(native ui.ListRow) {
			index := native.Index
			rowContext := native.Context
			key := props.Key(index)
			selected := state.Selection.Has(key)
			element := ListRow(rowContext, ListRowProps{Key: key, Label: props.RowLabel(index), Table: true,
				Odd: props.Zebra && index%2 != 0, Selected: selected, Focused: props.Focused, FocusVisible: state.Cursor == index && parts.Root.FocusVisible()})
			if element.Clicked() {
				state.choose(index, element.ClickModifiers(), props)
				state.List.Focus(rowContext)
			}
			if element.DoubleClicked() {
				result.Open = index
			}
			if element.RightClicked() {
				if !state.Selection.Has(key) {
					state.Select(key)
				}
				result.Menu = index
			}
			if props.Menu != nil {
				element.ContextMenu(func(menu *ui.Menu) {
					if !state.Selection.Has(key) {
						state.Select(key)
					}
					result.Menu = index
					props.Menu(menu, index)
				})
			}
			rowParts := parts
			rowParts.RowKey, rowParts.Focused = key, props.Focused
			row(TableRow{Context: rowContext, Element: element, Index: index, Key: key, Selected: selected, Focused: props.Focused}, rowParts)
			if element.Dragging() {
				// MyGo builds a focused row even outside its virtual viewport.
				// Retaining that row also retains its custom native Drag source.
				element.Focus().FocusRing(false)
				state.dragging = true
			}
			if props.DropTarget != nil && props.DropTarget(index) {
				element.DrawOver(func(painter *ui.Painter, rect ui.Rect) {
					DrawDropIndicator(painter, rect, DropIndicatorProps{Background: Color(t.C.DropIndicator.PlaceholderAccentBg),
						Border: Color(t.C.DropIndicator.ZoneBorder), Radius: t.C.FileList.RowRadius, BorderWidth: t.C.DropIndicator.ZoneBorderWidth})
				})
			}
		})
		if props.Count == 0 && empty != nil {
			ui.Box(c).Key("empty").Absolute().Top(props.HeaderHeight).Bottom(props.FooterHeight).
				Left(t.C.FilePane.ContentPadding).Right(t.C.FilePane.ContentPadding).Center().Clip().PassThrough().Children(empty)
		}
		if props.Underlay != nil {
			if props.Backdrop != nil {
				paintTableBackdrop(c, parts, scroll, props)
			}
			props.Underlay()
		}
		ui.Box(c).Key("header").Absolute().Top(props.HeaderHeight - t.C.FileList.HeaderHeight).Left(0).Right(0).
			Height(t.C.FileList.HeaderHeight).ClipX().Children(func() {
			SortHeader(c, SortHeaderProps{Columns: columns, State: state, Width: width,
				Scroller: parts.Root, Chevron: IconSVG(IconChevronDown),
				Translucent: props.Underlay != nil && props.Backdrop != nil})
		})
		Scrollbars(c, parts.Frame, parts.Root, scroll, ScrollbarProps{
			Label: props.Label + "滚动条", Horizontal: true, Vertical: true,
		})
	})
	if wasDragging && !state.dragging {
		parts.Root.Focus()
	}
	state.tableKeys(parts.Root, props)
	if parts.Root.Shortcut(0, ui.KeyEnter) {
		result.Open = state.Cursor
	}
	if parts.Root.Shortcut(0, ui.KeySpace) {
		result.Preview = state.Cursor
	}
	backspace := parts.Root.Shortcut(0, ui.KeyBackspace)
	forwardDelete := parts.Root.Shortcut(0, ui.KeyDelete)
	if backspace || forwardDelete {
		result.Delete = state.Cursor
	}
	result.TableParts = parts
	return result
}

func resolveColumnWidths(columns []Column, stored map[string]float32, available float32) []float32 {
	widths := make([]float32, len(columns))
	fixed, flexible := float32(0), 0
	for i, column := range columns {
		if column.MinWidth == 0 && column.Width == 0 {
			column.MinWidth = theme.Current().C.FileList.NameMinWidth
		}
		width := column.Width
		if resized, ok := stored[column.ID]; ok && !column.Fixed {
			width = resized
		}
		if width <= 0 {
			flexible++
			continue
		}
		width = max(width, column.MinWidth)
		if column.MaxWidth > 0 {
			width = min(width, column.MaxWidth)
		}
		widths[i] = width
		fixed += width
	}
	gap := theme.Current().C.FileList.ColumnGap*float32(max(0, len(columns)-1)) + 2*theme.Current().C.FileList.RowPaddingX
	for i, column := range columns {
		if column.MinWidth == 0 && column.Width == 0 {
			column.MinWidth = theme.Current().C.FileList.NameMinWidth
		}
		if widths[i] == 0 {
			widths[i] = max(column.MinWidth, (available-fixed-gap)/float32(max(flexible, 1)))
			if column.MaxWidth > 0 {
				widths[i] = min(widths[i], column.MaxWidth)
			}
		}
	}
	return widths
}

func sumColumnWidths(widths []float32) float32 {
	var total float32
	for _, width := range widths {
		total += width
	}
	return total
}

func MiddleText(text string, width float32, font ui.Font) string {
	if measured, _ := ui.ShapeText(text, font, 0).Size(); measured <= width {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		keep := (lo + hi + 1) / 2
		left := keep / 2
		right := keep - left
		candidate := string(runes[:left]) + "…" + string(runes[len(runes)-right:])
		measured, _ := ui.ShapeText(candidate, font, 0).Size()
		if measured <= width {
			lo = keep
		} else {
			hi = keep - 1
		}
	}
	left := lo / 2
	right := lo - left
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

// Cells shares column widths with SortHeader, retaining caller-provided order
// and text. A zero-width column takes the remaining space.
func (parts TableParts) Cells(c *ui.Context, values []string) {
	t := theme.Current()
	for index, column := range parts.Columns {
		width := max(0, parts.Widths[index]-column.LeadingWidth)
		value, font := tableCellContent(column, values[index], width)
		cell := ui.Text(c, value).Width(width).Shrink(0).Font(font.Family).FontSize(font.Size).FontFeatures(font.Features).SingleLine().Role(ui.RoleCell).
			Label(column.Title + "：" + values[index]).Value(values[index])
		if column.RowHeader {
			cell.Description("行标题 · " + column.Title)
		}
		if column.Align == AlignEnd {
			cell.TextAlign(ui.End).FontFeatures("tnum")
		}
		if column.Align == AlignCenter {
			cell.TextAlign(ui.Center)
		}
		if column.TextStyle != ColumnText {
			ink := t.C.FileList.Meta
			if parts.State.Selection.Has(parts.RowKey) {
				ink = t.C.FilePaneFocus.SelTextInactive
				if parts.Focused {
					ink = t.C.FilePaneFocus.SelTextActive
				}
			}
			cell.TextColor(Color(ink))
		}
	}
}

func (state *TableState) Select(key string) {
	state.Selection.Clear()
	state.Selection.Add(key)
	state.Cursor = -1
	state.choice = key
	state.anchor = key
	state.cursorKey = key
}

func (state *TableState) Clear() {
	state.Selection.Clear()
	state.Cursor = -1
	state.choice = ""
	state.anchor = ""
	state.cursorKey = ""
}

func (state *TableState) choose(index int, modifiers ui.Modifiers, props TableProps) {
	key := props.Key(index)
	if modifiers&ui.Shift != 0 && state.anchor != "" {
		anchor := -1
		for i := range props.Count {
			if props.Key(i) == state.anchor {
				anchor = i
				break
			}
		}
		if anchor >= 0 {
			if modifiers&ui.Cmd == 0 {
				state.Selection.Clear()
			}
			for i := min(anchor, index); i <= max(anchor, index); i++ {
				state.Selection.Add(props.Key(i))
			}
			state.Cursor = index
			state.cursorKey = key
			return
		}
	}
	if modifiers&ui.Cmd != 0 {
		if state.Selection.Has(key) {
			state.Selection.Remove(key)
		} else {
			state.Selection.Add(key)
		}
	} else {
		state.Selection.Clear()
		state.Selection.Add(key)
	}
	state.Cursor, state.anchor = index, key
	state.cursorKey = key
}

func (state *TableState) tableKeys(list ui.Element, props TableProps) {
	if list.Shortcut(ui.Cmd, ui.KeyA) {
		state.Selection.Clear()
		for i := range props.Count {
			state.Selection.Add(props.Key(i))
		}
		if state.Cursor < 0 && props.Count > 0 {
			state.Cursor = 0
			state.anchor = props.Key(0)
			state.cursorKey = props.Key(0)
		}
		list.SetChanged()
	}
	fileList := theme.Current().C.FileList
	page := max(1, int(list.Bounds().H/(fileList.RowHeight+fileList.RowGap)))
	for _, mods := range []ui.Modifiers{0, ui.Shift} {
		to := -1
		if list.Shortcut(mods, ui.KeyDown) {
			to = min(max(state.Cursor+1, 0), props.Count-1)
		}
		if list.Shortcut(mods, ui.KeyUp) {
			to = max(state.Cursor-1, 0)
		}
		if list.Shortcut(mods, ui.KeyHome) {
			to = 0
		}
		if list.Shortcut(mods, ui.KeyEnd) {
			to = props.Count - 1
		}
		if list.Shortcut(mods, ui.KeyPageDown) {
			to = min(max(state.Cursor+page, 0), props.Count-1)
		}
		if list.Shortcut(mods, ui.KeyPageUp) {
			to = max(state.Cursor-page, 0)
		}
		if to >= 0 && to < props.Count {
			state.choose(to, mods, props)
			state.List.ScrollIntoView(to)
			list.SetChanged()
		}
	}
}

func (state *TableState) Sorted(column string, modifiers ui.Modifiers) {
	if len(state.SortBy) == 0 && state.Sort.Column != "" {
		state.SortBy = []ui.SortOrder{state.Sort}
	}
	index := slices.IndexFunc(state.SortBy, func(order ui.SortOrder) bool { return order.Column == column })
	if modifiers&(ui.Cmd|ui.Shift) != 0 {
		if index < 0 {
			state.SortBy = append(state.SortBy, ui.SortOrder{Column: column})
		} else if !state.SortBy[index].Descending {
			state.SortBy[index].Descending = true
		} else {
			state.SortBy = slices.Delete(state.SortBy, index, index+1)
		}
	} else {
		descending := state.Sort.Column == column && !state.Sort.Descending
		state.SortBy = []ui.SortOrder{{Column: column, Descending: descending}}
	}
	state.Sort = ui.SortOrder{}
	if len(state.SortBy) > 0 {
		state.Sort = state.SortBy[0]
	}
}

func columnSortLabel(column Column, state *TableState) string {
	label := column.SortLabel
	if label == "" {
		label = "按" + column.Title + "排序"
	}
	if order, priority, sorted := columnOrder(column, state); sorted {
		direction := "升序"
		if order.Descending {
			direction = "降序"
		}
		if len(state.SortBy) > 1 {
			return fmt.Sprintf("%s，当前%s，第%d排序条件", label, direction, priority+1)
		}
		return fmt.Sprintf("%s，当前%s", label, direction)
	}
	return label
}

func columnOrder(column Column, state *TableState) (ui.SortOrder, int, bool) {
	for index, order := range state.SortBy {
		if order.Column == column.ID {
			return order, index, true
		}
	}
	if len(state.SortBy) == 0 && state.Sort.Column == column.ID {
		return state.Sort, 0, true
	}
	return ui.SortOrder{}, -1, false
}
