package desktop

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// MyGo's NumberInput fixes its inner field at 80px. Compose the same native
// input and Stepper so the design's field width does not constrain the whole row.
func settingsNumberInput(c *ui.Context, label string, value *float64, lo, hi, step float64, width float32, grouped bool) bool {
	t := theme.Current()
	row := ui.Row(c).Key(label).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center)
	decimals := 0
	if stepText := strconv.FormatFloat(step, 'f', -1, 64); strings.Contains(stepText, ".") {
		decimals = len(stepText) - strings.IndexByte(stepText, '.') - 1
	}
	format := func(v float64) string {
		text := strconv.FormatFloat(v, 'f', decimals, 64)
		if grouped {
			for index := len(text) - 3; index > 0; index -= 3 {
				text = text[:index] + " " + text[index:]
			}
		}
		return text
	}
	text := ui.Local(row, "text", func() string { return format(*value) })
	previous := *value
	previousText := *text
	set := func(v float64) {
		v = math.Max(lo, math.Min(hi, v))
		factor := math.Pow10(decimals)
		*value = math.Round(v*factor) / factor
	}
	row.Children(func() {
		input := textInput(c.Key("value"), text).Width(width).Shrink(0).Label(localize(c, label))
		if input.Changed() {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(*text), " ", ""), 64); err == nil && v >= lo && v <= hi {
				set(v)
			}
		}
		if input.Shortcut(0, ui.KeyUp) {
			set(*value + step)
			*text = format(*value)
		}
		if input.Shortcut(0, ui.KeyDown) {
			set(*value - step)
			*text = format(*value)
		}
		if !input.Focused() {
			*text = format(*value)
		}
		if ui.Stepper(c, value, lo, hi, step).Width(t.Space.N20).Height(t.C.Input.Height).Label(localize(c, label)).Changed() {
			*text = format(*value)
		}
	})
	if *text != previousText {
		c.AnimationFrame()
	}
	return *value != previous
}
