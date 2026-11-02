package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func TitleBar(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Row(c).Height(max(t.C.Titlebar.Height, c.TitleBar().Height)).Shrink(0).Gap(t.Space.N6).DragWindow()
}

func TitleBarLeading(c *ui.Context, workspaceWidth float32) ui.Element {
	t := theme.Current()
	bar := c.TitleBar()
	// The title row contributes its own gap before the tab strip.
	return ui.Row(c).Width(max(workspaceWidth-t.Space.N6, bar.Left+t.C.Titlebar.Height)).Shrink(0).
		PaddingStart(bar.Left + t.C.Titlebar.LeftGap).DragWindow()
}

func SessionTabsViewport(c *ui.Context) ui.Element {
	return ui.Box(c).Grow(1).MinWidth(0).Height(theme.Current().C.Tab.StripHeight).Clip()
}

func SessionTabsScroll(c *ui.Context, scroll *ui.ScrollState) ui.Element {
	e := ui.ScrollHorizontal(c).Fill()
	if scroll != nil {
		e.TrackScroll(scroll)
	}
	// Tabs scroll by wheel/trackpad and programmatic reveal. Native thumbs
	// paint outside the node's clip, so place them beyond the window.
	width, height := c.Size()
	return e.ScrollbarInsets(0, -width, -height, -width)
}

func SessionTabsRow(c *ui.Context) ui.Element {
	return ui.Row(c).Gap(theme.Current().C.Titlebar.TabsGap).FillHeight().Role(ui.RoleTabList).FocusGroup(ui.Horizontal)
}

func TitleBarActions(c *ui.Context) ui.Element {
	p := theme.Current().C.Titlebar
	return ui.Row(c).Padding(0, p.ActionsPaddingRight).Gap(p.ActionsGap).Shrink(0)
}

func CountTabs(c *ui.Context, selected *int, items []SegItem) ui.Element {
	// Segmented's Start alignment keeps form columns compact; these filters
	// live in a horizontal header and must share its vertical center.
	return Segmented(c, selected, items, SegmentedProps{Size: SizeSm}).AlignSelf(ui.Center)
}
