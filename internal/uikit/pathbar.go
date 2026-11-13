package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type PathSegment struct {
	Label, Target string
}

type PathBarProps struct {
	Path      string
	Segments  []PathSegment
	Icon      *ui.SVG
	Separator *ui.SVG
	Motion    MotionMode
}

type PathItem struct {
	Element ui.Element
	Target  string
}

type PathBarParts struct {
	Root   ui.Element
	Items  []PathItem
	Chosen bool
	Target string
	Index  int
}

// PathBar folds by measured width. Navigation and path ownership stay with the caller.
func PathBar(c *ui.Context, props PathBarProps) PathBarParts {
	t := theme.Current()
	family := strings.Join(t.Font.Family.Ui, ", ")
	font := ui.Font{Family: family, Size: t.Font.Size.Body, Weight: t.Font.Weight.Regular}
	measure := func(label string) float32 {
		width, _ := ui.ShapeText(label, font, 0).Size()
		return width
	}
	widths := make([]float32, len(props.Segments))
	visible := make([]int, len(props.Segments))
	for i, segment := range props.Segments {
		widths[i], visible[i] = measure(segment.Label), i
		if i == len(props.Segments)-1 {
			current := font
			current.Weight = t.Font.Weight.Semibold
			widths[i], _ = ui.ShapeText(segment.Label, current, 0).Size()
		}
	}
	ellipsisWidth := measure("…")
	bar := Tip(c, ui.Box(c).Key("file-breadcrumbs").Grow(1).MinWidth(0).Clip().Height(t.C.Breadcrumb.Height).
		Radius(t.C.Breadcrumb.Radius).Background(Color(t.C.Breadcrumb.BgOnGlass)).
		Font(family).FontSize(font.Size).FontWeight(font.Weight).Label(locales.MessageCurrentDirectory()+props.Path), props.Path)
	parts := PathBarParts{Root: bar, Index: -1}
	width := bar.LayoutBounds().W
	available := max(0, width-2*t.C.Breadcrumb.PaddingX)
	required := func(indices []int) float32 {
		result := t.C.Breadcrumb.IconSize + float32(len(indices)-1)*t.Font.Size.Caption +
			float32(2*len(indices)-1)*t.C.Breadcrumb.Gap
		for _, index := range indices {
			if index < 0 {
				result += ellipsisWidth
			} else {
				result += widths[index]
			}
		}
		return result
	}
	bestWidth := required(visible)
	if bestWidth > available {
		for tail := 2; tail < len(props.Segments); tail++ {
			candidate := []int{0, -1}
			for index := tail; index < len(props.Segments); index++ {
				candidate = append(candidate, index)
			}
			if size := required(candidate); size < bestWidth {
				visible, bestWidth = candidate, size
			}
			if bestWidth <= available {
				break
			}
		}
	}
	bar.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		if r.W != width {
			c.Invalidate()
		}
	})
	bar.Children(func() {
		ui.Row(c).Absolute().Top(0).Bottom(0).Left(t.C.Breadcrumb.PaddingX).Right(t.C.Breadcrumb.PaddingX).
			Gap(t.C.Breadcrumb.Gap).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, props.Icon).Size(t.C.Breadcrumb.IconSize, t.C.Breadcrumb.IconSize).TextColor(Color(t.C.Breadcrumb.Text)).Shrink(0).PassThrough()
			for position, index := range visible {
				if position > 0 {
					ui.Icon(c, props.Separator).Size(t.Font.Size.Caption, t.Font.Size.Caption).
						TextColor(Color(t.C.Breadcrumb.Separator)).Shrink(0)
				}
				label, target := "…", ""
				if index < 0 {
					target = props.Segments[visible[position+1]-1].Target
				} else {
					label, target = props.Segments[index].Label, props.Segments[index].Target
				}
				key := struct {
					target   string
					overflow bool
				}{target: target, overflow: index < 0}
				item := Tip(c, ui.ButtonBase(c).Key(key).Label(label), target).Shrink(1).MinWidth(0).
					TextColor(Color(t.C.Breadcrumb.Text))
				if index == len(props.Segments)-1 {
					item.TextColor(Color(t.C.Breadcrumb.Current)).FontWeight(t.Font.Weight.Semibold)
				}
				item.Children(func() { ui.Text(c, label).SingleLine().MinWidth(0).FontSize(font.Size).PassThrough() })
				if index < 0 {
					var entries []MenuEntry
					for hidden := visible[position-1] + 1; hidden < visible[position+1]; hidden++ {
						segment := props.Segments[hidden]
						entries = append(entries, MenuEntry{ID: segment.Target, Label: segment.Label, Icon: IconFolder,
							OnChoose: func() { parts.Chosen, parts.Target, parts.Index = true, segment.Target, hidden }})
					}
					MenuOn(c, item, entries, props.Motion)
				} else {
					parts.Items = append(parts.Items, PathItem{Element: item, Target: target})
					if item.Clicked() {
						parts.Chosen, parts.Target, parts.Index = true, target, index
					}
				}
			}
		})
	})
	return parts
}
