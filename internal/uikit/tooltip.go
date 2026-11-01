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

func Tip(c *ui.Context, anchor ui.Element, label string, shortcuts ...string) ui.Element {
	shortcut := ""
	if len(shortcuts) != 0 {
		shortcut = shortcuts[0]
	}
	description := label
	if shortcut != "" {
		description += " " + shortcut
	}
	anchor.Description(description)
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		StyleTooltip(tip).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).
			Margin(0, 0, theme.Current().C.Tooltip.Offset, 0).Children(func() {
			ui.Row(c).Gap(theme.Current().Space.N6).AlignItems(ui.Center).PassThrough().Children(func() {
				ui.Text(c, label).PassThrough()
				if shortcut != "" {
					Kbd(c, shortcut, KbdProps{Tone: KbdOnInk})
				}
			})
		})
	})
	return anchor
}
