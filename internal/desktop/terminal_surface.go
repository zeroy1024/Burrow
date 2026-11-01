package desktop

import (
	"image/color"
	"math"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

var (
	terminalLightSurface = theme.Light()
	terminalDarkSurface  = theme.Dark()
	terminalSurfaceCache struct {
		background color.NRGBA
		theme      *theme.Theme
	}
)

func updateTerminalSurfaceAccent(t *theme.Theme) {
	terminalLightSurface = theme.WithAccent(theme.Light(), t.Color.Accent.Default)
	terminalDarkSurface = theme.WithAccent(theme.Dark(), t.Color.Accent.Default)
	terminalSurfaceCache.theme = nil
}

// Terminal chrome follows its rendered surface, independently of the app appearance.
func terminalSurface(background color.NRGBA) *theme.Theme {
	if terminalSurfaceCache.theme != nil && terminalSurfaceCache.background == background {
		return terminalSurfaceCache.theme
	}
	light := terminalLightSurface.C.PaneFocus.NameFocused
	dark := terminalDarkSurface.C.PaneFocus.NameFocused
	base := terminalLightSurface
	if terminalColorContrast(dark, background) > terminalColorContrast(light, background) {
		base = terminalDarkSurface
	}
	surface := *base
	primary := base.C.PaneFocus.NameFocused
	neutral := func(source color.NRGBA, minimum float64) color.NRGBA {
		if terminalColorContrast(source, background) < minimum {
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
	surface.C.IconButton.Icon = neutral(surface.C.IconButton.Icon, 3)
	surface.C.IconButton.IconSubtle = neutral(surface.C.IconButton.IconSubtle, 3)
	surface.C.IconButton.IconFaint = neutral(surface.C.IconButton.IconFaint, 3)
	surface.C.TitleAction.Icon = neutral(surface.C.TitleAction.Icon, 3)
	surface.C.TitleAction.IconOn = primary
	terminalSurfaceCache.background, terminalSurfaceCache.theme = background, &surface
	return &surface
}

func terminalScrollbarColor(background color.NRGBA) ui.Color {
	light := terminalLightSurface.C.Scrollbar.Color
	dark := terminalDarkSurface.C.Scrollbar.Color
	if terminalColorContrast(dark, background) > terminalColorContrast(light, background) {
		return tokenColor(dark)
	}
	return tokenColor(light)
}

func terminalColorContrast(foreground, background color.NRGBA) float64 {
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
