package uikit

import (
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// Pane uses the terminal's resolved surface; its caller owns session bindings
// and movement, including any position or size animation.
func Pane(c *ui.Context, key any, label string, rect ui.Rect, surface *theme.Theme) ui.Element {
	return ui.Box(c).Key(key).Absolute().Left(rect.X).Top(rect.Y).Size(rect.W, rect.H).
		MinWidth(0).MinHeight(0).Radius(surface.C.Pane.Radius).Label(label)
}

func PaneCard(c *ui.Context, surface *theme.Theme, background ui.Color, paintRadius float32) ui.Element {
	return ui.Column(c).Key("card").Absolute().Top(0).Right(0).Bottom(0).Left(0).
		Radius(surface.C.Pane.Radius).Background(background).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		// Absolute children paint after scroll content; the backdrop belongs here.
		p.Fill(r, background, paintRadius)
	})
}

type PaneAppearance struct {
	Opacity, Saturation, FooterOpacity float32
	TitleColor, LoginColor             ui.Color
	TitleWeight                        int
}

func PaneVisualStyle(pane ui.Element, focused, hovered, multiple bool, surface *theme.Theme) PaneAppearance {
	p, f := surface.C.Pane, surface.C.PaneFocus
	style := PaneAppearance{
		Opacity: 1, Saturation: 1, FooterOpacity: 1,
		TitleColor: Color(f.NameFocused), LoginColor: Color(p.Host), TitleWeight: f.NameWeightFocused,
	}
	if multiple && !focused {
		style.Opacity, style.Saturation, style.FooterOpacity = f.ContentOpacityUnfocused, f.ContentSaturateUnfocused, f.StatusOpacityUnfocused
		style.TitleColor, style.LoginColor, style.TitleWeight = Color(f.NameUnfocused), Color(f.HostUnfocused), f.NameWeightUnfocused
		if hovered {
			style.Opacity, style.Saturation = f.ContentOpacityHover, f.ContentSaturateHover
			style.TitleColor = Color(f.NameFocused)
		}
	}
	style.Opacity = Visual(pane, "output-opacity", style.Opacity, f.Transition)
	style.Saturation = Visual(pane, "output-saturation", style.Saturation, f.Transition)
	style.FooterOpacity = Visual(pane, "footer-opacity", style.FooterOpacity, f.Transition)
	if !multiple {
		style.Opacity, style.Saturation, style.FooterOpacity = 1, 1, 1
	}
	style.TitleColor = pane.AnimateColor("host-color", style.TitleColor, Tween(f.Transition))
	style.LoginColor = pane.AnimateColor("login-color", style.LoginColor, Tween(f.Transition))
	return style
}

func PaneHeader(c *ui.Context, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	return ui.Row(c).Key("identity").Absolute().Top(0).Left(0).Height(p.HeaderHeight).
		Padding(0, 0, 0, p.HeaderPaddingLeft).Gap(p.HeaderGap).PassThrough()
}

func PaneShortcut(c *ui.Context, keys string, focused bool, surface *theme.Theme) ui.Element {
	f := surface.C.PaneFocus
	text, background := Color(f.KbdText), Color(f.KbdBg)
	if focused {
		text, background = Color(f.KbdTextFocused), Color(f.KbdBgFocused)
	}
	return ui.Text(c, keys).FontSize(f.KbdSize).FontWeight(f.KbdWeight).
		FixedLineHeight(f.KbdLineHeight).SingleLine().Padding(f.KbdPaddingY, f.KbdPaddingX).Radius(f.KbdRadius).
		Background(background).TextColor(text)
}

func PaneTitle(c *ui.Context, title string, style PaneAppearance, surface *theme.Theme) ui.Element {
	return ui.Text(c, title).FontSize(surface.C.Pane.HeaderFontSize).FontWeight(style.TitleWeight).TextColor(style.TitleColor).
		SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
}

func PaneLogin(c *ui.Context, login string, style PaneAppearance, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	return ui.Text(c, login).Font(strings.Join(p.HostFamily, ", ")).FontSize(p.HostSize).TextColor(style.LoginColor).
		SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
}

func PaneActions(c *ui.Context, width float32, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	return ui.Box(c).Key("pane-actions").Absolute().Top(p.ActionTop).Right(p.ActionRight).Size(width, p.ActionHeight)
}

func PaneActionBackdrop(c *ui.Context, width, blurRadius float32, background ui.Color, surface *theme.Theme) {
	p := surface.C.Pane
	ui.Box(c).Absolute().Top(0).Left(0).Size(width, p.ActionHeight).Radius(p.ActionRadius).PassThrough().
		Material(glass.Blur{Radius: blurRadius})
	ui.Box(c).Absolute().Top(0).Left(0).Size(width, p.ActionHeight).Radius(p.ActionRadius).PassThrough().
		Background(background.Alpha(p.ActionTint))
}

func PaneIconButton(c *ui.Context, label string, icon *ui.SVG, surface *theme.Theme) ui.Element {
	button := StyleIconButton(ui.ButtonBase(c).Key(label).Label(label), surface)
	Tip(c, button, label)
	size := surface.C.IconButton.IconSize
	button.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return button
}

func StylePaneActions(actions ui.Element, target float32, surface *theme.Theme) float32 {
	visibility := Visual(actions, "visibility", target, surface.C.PaneFocus.Transition)
	actions.Opacity(visibility)
	return visibility
}

func PaneFooter(c *ui.Context, opacity float32, surface *theme.Theme) ui.Element {
	p := surface.C.StatusBar
	return ui.Row(c).Key("status").Absolute().Bottom(0).Left(0).Right(0).Height(p.HeightPane).Opacity(opacity).
		Padding(0, p.PaddingX).Gap(p.GapPane).PassThrough().TextColor(Color(p.Text))
}

func PaneHairline(c *ui.Context, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	hairline := ui.Box(c).Key("hairline").Absolute().Top(0).Right(0).Bottom(0).Left(0).Radius(p.Radius).PassThrough()
	return Shadow(hairline, p.Outline)
}

func StylePaneFocus(pane ui.Element, focused, multiple bool, surface *theme.Theme) {
	p := surface.C.PaneFocus
	focusTarget, hoverTarget := float32(0), float32(0)
	if multiple && focused {
		focusTarget = 1
	}
	if multiple && !focused && pane.Hovered() {
		hoverTarget = 1
	}
	focus := Visual(pane, "focus-ring", focusTarget, p.Transition)
	hover := Visual(pane, "hover-ring", hoverTarget, p.Transition)
	if !multiple {
		return
	}
	ShadowOpacity(pane, p.RingHover, hover)
	ShadowOpacity(pane, p.RingFocused, focus)
}
