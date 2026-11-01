package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func StyleTooltip(tip ui.Element) ui.Element {
	p := theme.Current().C.Tooltip
	return Shadow(tip.MaxWidth(p.MaxWidth).Padding(p.PaddingY, p.PaddingX).
		Radius(p.Radius).Background(Color(p.Bg)).TextColor(Color(p.Text)).FontSize(p.FontSize), p.Shadow)
}

func Tip(c *ui.Context, anchor ui.Element, label string) ui.Element {
	anchor.Description(label)
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		StyleTooltip(tip).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).
			Margin(0, 0, theme.Current().C.Tooltip.Offset, 0).Children(func() { ui.Text(c, label) })
	})
	return anchor
}
