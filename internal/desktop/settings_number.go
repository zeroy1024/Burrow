package desktop

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

func settingsNumberInput(c *ui.Context, label string, value *float64, lo, hi, step float64, width float32, grouped bool) bool {
	return uikit.NumberInput(c, value, uikit.NumberInputProps{
		Key: label, Label: label, Min: lo, Max: hi, Step: step, Width: width, Grouped: grouped,
	})
}
