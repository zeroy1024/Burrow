package uikit

import (
	"github.com/zeroy1024/Burrow/internal/theme"

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
	attachTip(c, anchor, label, shortcut)
	return anchor
}

// attachTip shows label, with any shortcut, above anchor while the pointer
// is over it.
func attachTip(c *ui.Context, anchor ui.Element, label, shortcut string) {
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
}

// HelpTip is the question mark after a settings label. Its hint shows only
// while the pointer is over it: it takes neither clicks nor the focus. The
// hint also names the icon, since assistive technology skips an unnamed icon
// as decoration.
func HelpTip(c *ui.Context, hint string) ui.Element {
	p := theme.Current().C.HelpTip
	icon := ui.Icon(c, IconSVG(IconHelp)).Size(p.Size, p.Size).Shrink(0).Label(hint)
	ink := p.Icon
	if icon.Hovered() {
		ink = p.IconHover
	}
	icon.TextColor(Color(ink))
	attachTip(c, icon, hint, "")
	return icon
}
