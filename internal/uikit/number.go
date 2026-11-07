package uikit

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/theme"
)

type NumberInputProps struct {
	Key            any
	Label          string
	Min, Max, Step float64
	Width          float32
	Grouped        bool
}

// NumberInput keeps partial text edits while allowing the field width to differ
// from MyGo's fixed 80px NumberInput field.
func NumberInput(c *ui.Context, value *float64, props NumberInputProps) bool {
	t := theme.Current()
	row := ui.Row(c).Key(props.Key).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center)
	decimals := 0
	if stepText := strconv.FormatFloat(props.Step, 'f', -1, 64); strings.Contains(stepText, ".") {
		decimals = len(stepText) - strings.IndexByte(stepText, '.') - 1
	}
	format := func(v float64) string {
		text := strconv.FormatFloat(v, 'f', decimals, 64)
		if props.Grouped {
			for index := len(text) - 3; index > 0; index -= 3 {
				text = text[:index] + " " + text[index:]
			}
		}
		return text
	}
	text := ui.Local(row, "uikit.number.text", func() string { return format(*value) })
	previous, previousText := *value, *text
	set := func(v float64) {
		v = math.Max(props.Min, math.Min(props.Max, v))
		factor := math.Pow10(decimals)
		*value = math.Round(v*factor) / factor
	}
	row.Children(func() {
		input := Input(c.Key("value"), text, InputProps{Label: props.Label}).Width(props.Width).Shrink(0)
		if input.Changed() {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(*text), " ", ""), 64); err == nil && v >= props.Min && v <= props.Max {
				set(v)
			}
		}
		if input.Shortcut(0, ui.KeyUp) {
			set(*value + props.Step)
			*text = format(*value)
		}
		if input.Shortcut(0, ui.KeyDown) {
			set(*value - props.Step)
			*text = format(*value)
		}
		if !input.Focused() {
			*text = format(*value)
		}
		if ui.Stepper(c, value, props.Min, props.Max, props.Step).Width(t.Space.N20).Height(t.C.Input.Height).Label(props.Label).Changed() {
			*text = format(*value)
		}
	})
	if *text != previousText {
		c.AnimationFrame()
	}
	return *value != previous
}
