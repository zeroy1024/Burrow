package desktop

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func tokenColor(c color.NRGBA) ui.Color {
	return ui.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

func windowColor(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func applyShadow(e ui.Element, layers theme.Shadow) ui.Element {
	return applyShadowOpacity(e, layers, 1)
}

func applyAnimatedShadow(e ui.Element, key any, layers theme.Shadow, animation ui.Animation) ui.Element {
	animated := make(theme.Shadow, len(layers))
	for i, layer := range layers {
		shown := e.AnimateColor(struct {
			Key   any
			Layer int
		}{Key: key, Layer: i}, tokenColor(layer.Color), animation)
		layer.Color.R, layer.Color.G, layer.Color.B, layer.Color.A = shown.R, shown.G, shown.B, shown.A
		animated[i] = layer
	}
	return applyShadow(e, animated)
}

func applyShadowOpacity(e ui.Element, layers theme.Shadow, opacity float32) ui.Element {
	// Both APIs append layers in painting order. Token shadows put the first
	// layer on top, so append them back to front, including zero-blur rings.
	for i := len(layers) - 1; i >= 0; i-- {
		layer := layers[i]
		c := tokenColor(layer.Color)
		c.A = uint8(math.Round(float64(float32(c.A) * opacity)))
		if layer.Inset {
			e = e.InsetShadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
			continue
		}
		e = e.Shadow(layer.X, layer.Y, layer.Blur, layer.Spread, c)
	}
	return e
}

func tokenSpring(s theme.Spring) ui.SpringOptions {
	if s.Stiffness != 0 || s.Damping != 0 {
		return ui.SpringOptions{Stiffness: s.Stiffness, Damping: s.Damping}
	}
	return ui.SpringOptions{Duration: s.Duration, Bounce: s.Bounce}
}

func tokenEasing(b theme.Bezier) ui.Easing {
	return ui.CubicBezier(b[0], b[1], b[2], b[3])
}

func tokenTween(duration time.Duration) ui.Animation {
	return ui.Animation{Duration: duration, Ease: tokenEasing(theme.Current().Motion.Easing.Out)}
}

// MyGo suppresses scalar animations under ReduceMotion, but AnimateColor keeps
// color and alpha transitions. Encode visual-only values in its alpha channel.
func animateVisual(e ui.Element, key any, target float32, duration time.Duration) float32 {
	c := ui.Color{A: uint8(math.Round(float64(target * 255)))}
	value := e.AnimateColor(key, c, tokenTween(duration))
	if value.A == c.A {
		return target
	}
	return float32(value.A) / 255
}

func button(c *ui.Context, label string) ui.Element {
	return styledButton(c, label, false)
}

func primaryButton(c *ui.Context, label string) ui.Element {
	return styledButton(c, label, true)
}

func styledButton(c *ui.Context, label string, primary bool) ui.Element {
	p := theme.Current().C.Button
	e := ui.ButtonBase(c).Height(p.Height).Padding(0, p.PaddingX).Gap(p.Gap).
		Radius(p.Radius).FontSize(p.FontSize).Label(label)
	bg, fg := p.Bg, p.Text
	if primary {
		bg, fg = p.PrimaryBg, p.PrimaryText
	} else {
		e.Border(p.BorderWidth, tokenColor(p.Border))
		switch {
		case e.Pressed():
			bg = p.BgPressed
		case e.Hovered():
			bg = p.BgHover
		}
	}
	e.Background(e.AnimateColor("background", tokenColor(bg), tokenTween(p.Transition))).
		TextColor(tokenColor(fg)).Children(func() { ui.Text(c, label).SingleLine() })
	return e
}

func textInput(c *ui.Context, value *string) ui.Element {
	p := theme.Current().C.Input
	e := ui.TextInputBase(c, value).Height(p.Height).Padding(0, p.PaddingX).
		Radius(p.Radius).FontSize(p.FontSize).FixedLineHeight(p.LineHeight).
		TextColor(tokenColor(p.Text)).Background(tokenColor(p.Bg))
	border := p.Border
	switch {
	case e.Focused():
		border = p.BorderFocus
	case e.Hovered():
		border = p.BorderHover
	}
	return e.Border(p.BorderWidth, e.AnimateColor("border", tokenColor(border), tokenTween(p.Transition)))
}

func (a *app) sidebarSearch(c *ui.Context, value *string, focus *ui.Handle, label, placeholder string) {
	t := theme.Current()
	p := t.C.Search
	hint := shortcutDisplay(a.shortcutBinding("host.search"))
	rightPadding := p.PaddingX
	if hint != "" {
		width, _ := ui.ShapeText(hint, ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: p.KbdSize}, 0).Size()
		rightPadding += width + p.Gap
	}
	ui.Box(c).Height(p.Height).Shrink(0).Margin(0, p.MarginX, p.MarginBottom, p.MarginX).
		Radius(p.Radius).Background(tokenColor(p.Bg)).Children(func() {
		ui.TextInputBase(c.Key("input"), value).Bind(focus).Fill().MinWidth(0).
			Padding(0, rightPadding, 0, p.PaddingX+t.Size.Icon+p.Gap).Radius(p.Radius).
			FontSize(p.FontSize).FixedLineHeight(p.LineHeight).TextColor(tokenColor(t.Color.Text.Primary)).
			Placeholder(placeholder).Label(label)
		// Decorations share the field's bounds; every pixel still targets the input.
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Padding(0, p.PaddingX).
			Gap(p.Gap).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Icon(c, searchIcon).Size(t.Size.Icon, t.Size.Icon).Shrink(0).TextColor(tokenColor(p.Text)).PassThrough()
			ui.Spacer(c).PassThrough()
			if hint != "" {
				ui.Text(c, hint).FontSize(p.KbdSize).TextColor(tokenColor(p.KbdText)).Shrink(0).PassThrough()
			}
		})
	})
}

func modal(c *ui.Context, open *bool, content func()) {
	p := theme.Current().C.Dialog
	width, height := c.Size()
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(tokenColor(p.Backdrop))
		panel.Padding(p.Padding).Gap(p.Gap).Radius(p.Radius).Background(tokenColor(p.Bg)).
			MaxWidth(width - 2*p.ViewportInset).MaxHeight(height - 2*p.ViewportInset)
		applyShadow(panel, p.Shadow)
		content()
	})
}

func styleTooltip(tip ui.Element) ui.Element {
	p := theme.Current().C.Tooltip
	return applyShadow(tip.MaxWidth(p.MaxWidth).Padding(p.PaddingY, p.PaddingX).
		Radius(p.Radius).Background(tokenColor(p.Bg)).TextColor(tokenColor(p.Text)).FontSize(p.FontSize), p.Shadow)
}

func tooltip(c *ui.Context, anchor ui.Element, label string) ui.Element {
	anchor.Description(label)
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		styleTooltip(tip).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).
			Margin(0, 0, theme.Current().C.Tooltip.Offset, 0).Children(func() { ui.Text(c, label) })
	})
	return anchor
}

func styleIconButton(e ui.Element, t *theme.Theme) ui.Element {
	p := t.C.IconButton
	bg := ui.Transparent
	switch {
	case e.Pressed():
		bg = tokenColor(p.BgPressed)
	case e.Hovered():
		bg = tokenColor(p.BgHover)
	}
	return e.Size(p.Size, p.Size).Radius(p.Radius).TextColor(tokenColor(p.Icon)).
		Background(e.AnimateColor("background", bg, tokenTween(p.Transition)))
}

func styleSegment(e ui.Element, p theme.SegmentedTokens, selected bool) ui.Element {
	bg, fg, shadow := ui.Transparent, tokenColor(p.Text), float32(0)
	if selected {
		bg, fg, shadow = tokenColor(p.BgOn), tokenColor(p.TextOn), 1
	} else if e.Hovered() {
		bg = tokenColor(p.BgHover)
	}
	animation := tokenTween(p.Transition)
	e.Background(e.AnimateColor("background", bg, animation)).
		TextColor(e.AnimateColor("text", fg, animation))
	return applyShadowOpacity(e, p.ShadowOn, animateVisual(e, "shadow", shadow, p.Transition))
}

func nativeTheme(t *theme.Theme) *ui.Theme {
	return &ui.Theme{
		Dark:           t.IsDark,
		Background:     tokenColor(t.C.Window.Bg),
		Surface:        tokenColor(t.C.Button.Bg),
		SurfaceHover:   tokenColor(t.C.Button.BgHover),
		SurfacePressed: tokenColor(t.C.Button.BgPressed),
		Border:         tokenColor(t.C.Button.Border),
		Text:           tokenColor(t.Color.Text.Primary),
		TextMuted:      tokenColor(t.Color.Text.Secondary),
		Accent:         tokenColor(t.C.Button.PrimaryBg),
		AccentHover:    tokenColor(t.C.Button.PrimaryBg),
		AccentPressed:  tokenColor(t.C.Button.PrimaryBg),
		AccentText:     tokenColor(t.C.Button.PrimaryText),
		Danger:         tokenColor(t.C.Desktop.Danger),
		Warning:        tokenColor(t.C.Tab.Spinner),
		Success:        tokenColor(t.C.Tab.DotConnected),
		Selection:      tokenColor(t.C.FileList.BgSelected),
		Focus:          tokenColor(t.Color.Accent.Tint50),
		Inverse:        tokenColor(t.C.Tooltip.Bg),
		InverseText:    tokenColor(t.C.Tooltip.Text),
		Scrollbar:      tokenColor(t.C.Scrollbar.Color),
		ScrollbarWidth: t.C.Scrollbar.ThumbWidth,
		Radius:         t.C.Button.Radius,
		Spacing:        t.Space.N4,
		FontSize:       t.C.Button.FontSize,
		Font:           strings.Join(t.Font.Family.Ui, ", "),
	}
}
