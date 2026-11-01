package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func SidePanelBody(c *ui.Context) ui.Element {
	return ui.Column(c).Width(theme.Current().Size.StatsPanel).FillHeight()
}

type SidePanelHeaderParts struct {
	Root, Close ui.Element
}

func SidePanelHeader(c *ui.Context, title, closeLabel string, closeIcon *ui.SVG) SidePanelHeaderParts {
	t := theme.Current()
	parts := SidePanelHeaderParts{Root: ui.Row(c).Height(t.C.Stats.HeaderHeight).Shrink(0).Padding(0, t.Space.N14)}
	parts.Root.Children(func() {
		ui.Text(c, title).FontSize(t.Font.Size.BodyLg).FontWeight(t.Font.Weight.Semibold)
		ui.Spacer(c)
		parts.Close = IconButton(c, closeIcon, IconButtonProps{Label: closeLabel}).Size(t.C.IconButton.SizeXs, t.C.IconButton.SizeSm)
	})
	return parts
}

func SidePanelViewport(c *ui.Context) ui.Element {
	return ui.Box(c).Key("status-viewport").Grow(1).MinHeight(0).Clip()
}
