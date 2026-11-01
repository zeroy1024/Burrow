package desktop

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

type scrollbarState = uikit.ScrollbarState

type terminalScrollState struct {
	offset ui.ScrollState
	scrollbarState
}

func terminalScrollbar(c *ui.Context, frame, output ui.Element, state *terminalScrollState, background color.NRGBA) {
	t := theme.Current()
	uikit.Scrollbars(c, frame, output, &state.offset, uikit.ScrollbarProps{
		Label: "终端滚动条", State: &state.scrollbarState,
		Top: t.C.Pane.HeaderHeight, Bottom: t.C.StatusBar.HeightPane,
		Color: terminalScrollbarColor(background),
	})
}
