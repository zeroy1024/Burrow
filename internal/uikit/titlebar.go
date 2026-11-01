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
	return ui.Row(c).Width(max(workspaceWidth, bar.Left+t.C.Titlebar.Height)).Shrink(0).
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
	return e.ScrollbarInsets(0, 0, -theme.Current().Space.N12, 0)
}

func SessionTabsRow(c *ui.Context) ui.Element {
	return ui.Row(c).Gap(theme.Current().C.Titlebar.TabsGap).FillHeight()
}

func TitleBarActions(c *ui.Context) ui.Element {
	p := theme.Current().C.Titlebar
	return ui.Row(c).Padding(0, p.ActionsPaddingRight).Gap(p.ActionsGap).Shrink(0)
}
