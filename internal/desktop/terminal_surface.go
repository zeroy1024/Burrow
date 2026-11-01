package desktop

import (
	"image/color"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

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
	surface := uikit.TerminalChrome(background, terminalLightSurface, terminalDarkSurface)
	terminalSurfaceCache.background, terminalSurfaceCache.theme = background, surface
	return surface
}

func terminalScrollbarColor(background color.NRGBA) ui.Color {
	light := terminalLightSurface.C.Scrollbar.Color
	dark := terminalDarkSurface.C.Scrollbar.Color
	if uikit.ColorContrast(dark, background) > uikit.ColorContrast(light, background) {
		return uikit.Color(dark)
	}
	return uikit.Color(light)
}
