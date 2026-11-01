package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// Meter preserves neutral metric colors; the percentage does not imply a warning.
func Meter(c *ui.Context, percent float32) ui.Element {
	p := theme.Current().C.Progress
	return ui.Box(c).Height(p.Height).Radius(p.Radius).Background(Color(p.Track)).Children(func() {
		ui.Box(c).WidthPercent(percent).Height(p.Height).Radius(p.Radius).Background(Color(p.FillNeutral))
	})
}
