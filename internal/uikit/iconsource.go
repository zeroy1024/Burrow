package uikit

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func LineIcon(shapes string) *ui.SVG {
	return strokedIcon(shapes, theme.Current().C.IconButton.Stroke)
}

// BrandIcon keeps brand geometry independent from the application's UI stroke.
func BrandIcon(shapes string) *ui.SVG { return strokedIcon(shapes, 1.6) }

func strokedIcon(shapes string, stroke float32) *ui.SVG {
	return ui.MustParseSVG(fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"
fill="none" stroke="currentColor" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		stroke, shapes))
}
