package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type MeterProps struct{ Label, Value string }

func Meter(c *ui.Context, value, low, high float64, props MeterProps) ui.Element {
	if high <= low {
		panic("uikit: Meter upper bound must exceed its lower bound")
	}
	t := theme.Current()
	p := t.C.Progress
	fraction := min(max((value-low)/(high-low), 0), 1)
	ink := p.MeterFill
	if fraction >= float64(p.MeterCritAt) {
		ink = t.Color.Danger.Default
	} else if fraction >= float64(p.MeterWarnAt) {
		ink = t.Color.Warning.Default
	}
	return ui.Box(c).Height(p.HeightMeter).FillWidth().Radius(p.Radius).Background(Color(p.Track)).Role(ui.RoleMeter).
		Label(props.Label).Value(props.Value).Range(low, high, value).Children(func() {
		ui.Box(c).WidthPercent(float32(fraction) * 100).Height(p.HeightMeter).Radius(p.Radius).Background(Color(ink))
	})
}
