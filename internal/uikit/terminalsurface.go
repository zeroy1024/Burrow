package uikit

import (
	"image/color"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type TerminalColors struct{ Background, Foreground, Selection, SelectionForeground color.NRGBA }

func ResolveTerminalColors(scheme *termtheme.Scheme, source termtheme.BackgroundSource) TerminalColors {
	return TerminalColors{Background: termtheme.TerminalBackground(scheme, theme.Current().Color.Surface.Terminal, source),
		Foreground: scheme.Foreground, Selection: scheme.SelectionBg, SelectionForeground: scheme.SelectionFg}
}

// TerminalSurface receives the session's scheme explicitly; another pane can
// use a different scheme without installing a window-wide theme.
func TerminalSurface(c *ui.Context, scheme *termtheme.Scheme, source termtheme.BackgroundSource, body func()) ui.Element {
	colors := ResolveTerminalColors(scheme, source)
	return ui.Box(c).Background(Color(colors.Background)).TextColor(Color(colors.Foreground)).SelectionColor(Color(colors.Selection)).Children(body)
}

// TerminalChrome adapts neutral chrome to the rendered terminal background.
// The caller owns the base surfaces, accent updates and any caching.
func TerminalChrome(background color.NRGBA, light, dark *theme.Theme) *theme.Theme {
	lightText := light.C.PaneFocus.NameFocused
	darkText := dark.C.PaneFocus.NameFocused
	base := light
	if ColorContrast(darkText, background) > ColorContrast(lightText, background) {
		base = dark
	}
	surface := *base
	primary := base.C.PaneFocus.NameFocused
	neutral := func(source color.NRGBA, minimum float64) color.NRGBA {
		if ColorContrast(source, background) < minimum {
			return primary
		}
		return source
	}
	// Imported middle-gray surfaces can be close to both themes' muted colors.
	// Promote only neutral chrome roles; semantic badges and terminal text keep their colors.
	surface.C.Pane.Name = primary
	surface.C.Pane.Host = neutral(surface.C.Pane.Host, 4.5)
	surface.C.Pane.ButtonIcon = neutral(surface.C.Pane.ButtonIcon, 3)
	surface.C.PaneFocus.NameFocused = primary
	surface.C.PaneFocus.NameUnfocused = neutral(surface.C.PaneFocus.NameUnfocused, 4.5)
	surface.C.PaneFocus.HostUnfocused = neutral(surface.C.PaneFocus.HostUnfocused, 4.5)
	surface.C.PaneFocus.KbdText = neutral(surface.C.PaneFocus.KbdText, 4.5)
	surface.C.PaneFocus.CursorRingUnfocused = neutral(surface.C.PaneFocus.CursorRingUnfocused, 3)
	surface.C.StatusBar.Text = neutral(surface.C.StatusBar.Text, 4.5)
	surface.C.StatusBar.HostColor = primary
	surface.C.Latency.Normal = neutral(surface.C.Latency.Normal, 4.5)
	surface.C.IconButton.Icon = neutral(surface.C.IconButton.Icon, 3)
	surface.C.IconButton.IconSubtle = neutral(surface.C.IconButton.IconSubtle, 3)
	surface.C.IconButton.IconFaint = neutral(surface.C.IconButton.IconFaint, 3)
	surface.C.TitleAction.Icon = neutral(surface.C.TitleAction.Icon, 3)
	surface.C.TitleAction.IconOn = primary
	return &surface
}

func ColorContrast(foreground, background color.NRGBA) float64 {
	alpha := float64(foreground.A) / 255
	channel := func(front, back uint8) uint8 {
		return uint8(math.Round(float64(front)*alpha + float64(back)*(1-alpha)))
	}
	var composed color.NRGBA
	composed.R = channel(foreground.R, background.R)
	composed.G = channel(foreground.G, background.G)
	composed.B = channel(foreground.B, background.B)
	composed.A = 255
	front, back := termtheme.Luminance(composed), termtheme.Luminance(background)
	return (max(front, back) + 0.05) / (min(front, back) + 0.05)
}
