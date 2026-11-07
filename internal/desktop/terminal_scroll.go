package desktop

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type scrollbarState = uikit.ScrollbarState

type terminalScrollState struct {
	offset ui.ScrollState
	scrollbarState
}

func terminalScrollbar(c *ui.Context, frame, output ui.Element, state *terminalScrollState, background color.NRGBA, input func(ui.InputEvent) bool) {
	t := theme.Current()
	uikit.Scrollbars(c, frame, output, &state.offset, uikit.ScrollbarProps{
		Label: locales.MessageTerminalScrollbar(), State: &state.scrollbarState,
		Top: t.C.Pane.HeaderHeight, Bottom: t.C.StatusBar.HeightPane,
		Color: terminalScrollbarColor(background), Input: input,
	})
}
