package desktop

import (
	"fmt"
	"image/color"
	"time"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func tokenColor(c color.NRGBA) ui.Color { return uikit.Color(c) }

func windowColor(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func applyShadow(e ui.Element, layers theme.Shadow) ui.Element { return uikit.Shadow(e, layers) }

func applyAnimatedShadow(e ui.Element, key any, layers theme.Shadow, animation ui.Animation) ui.Element {
	return uikit.AnimatedShadow(e, key, layers, animation)
}

func applyShadowOpacity(e ui.Element, layers theme.Shadow, opacity float32) ui.Element {
	return uikit.ShadowOpacity(e, layers, opacity)
}

func tokenSpring(s theme.Spring) ui.SpringOptions { return uikit.Spring(s) }

func tokenEasing(b theme.Bezier) ui.Easing { return uikit.Easing(b) }

func tokenTween(duration time.Duration) ui.Animation { return uikit.Tween(duration) }

func animateVisual(e ui.Element, key any, target float32, duration time.Duration) float32 {
	return uikit.Visual(e, key, target, duration)
}

func button(c *ui.Context, label string) ui.Element {
	return styledButton(c, label, false)
}

func primaryButton(c *ui.Context, label string) ui.Element {
	return styledButton(c, label, true)
}

func styledButton(c *ui.Context, label string, primary bool) ui.Element {
	variant := uikit.ButtonSecondary
	if primary {
		variant = uikit.ButtonPrimary
	}
	return uikit.Button(c, label, uikit.ButtonProps{Variant: variant})
}

func textInput(c *ui.Context, value *string) ui.Element {
	return uikit.Input(c, value, uikit.InputProps{})
}

func (a *app) sidebarSearch(c *ui.Context, value *string, focus *ui.Handle, label, placeholder string) {
	uikit.SearchField(c, value, uikit.SearchProps{Label: label, Placeholder: placeholder,
		Shortcut: shortcutDisplay(a.shortcutBinding("host.search")), Icon: searchIcon, Handle: focus})
}

func modal(c *ui.Context, open *bool, content func()) { uikit.Dialog(c, open, content) }

func styleTooltip(tip ui.Element) ui.Element { return uikit.StyleTooltip(tip) }

func tooltip(c *ui.Context, anchor ui.Element, label string) ui.Element {
	return uikit.Tip(c, anchor, label)
}

func styleIconButton(e ui.Element, t *theme.Theme) ui.Element { return uikit.StyleIconButton(e, t) }

func styleSegment(e ui.Element, p theme.SegmentedTokens, selected bool) ui.Element {
	return uikit.StyleSegment(e, p, selected)
}
