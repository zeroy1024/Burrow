package desktop

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/termtheme"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

// agentCards draws an agent's record in its pane: a card for each operation,
// with the request and the output each in a panel of its own. Colors come
// from the terminal scheme, as the pane's background does.
func (a *app) agentCards(c *ui.Context, session *remoteSession, scheme *termtheme.Scheme, foreground color.NRGBA) {
	t := theme.Current()
	view := session.agent
	column := ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space.N10).Selectable().SelectionColor(uikit.Color(scheme.SelectionBg))
	if session.contentWidth > 0 {
		column.Width(max(1, session.contentWidth-2*t.C.Pane.ContentSide)).Shrink(0)
	}
	column.Children(func() {
		for _, block := range view.blocks {
			a.agentCard(c, view, block, scheme, uikit.Color(foreground))
		}
	})
}

func (a *app) agentCard(c *ui.Context, view *agentView, block *agentBlock, scheme *termtheme.Scheme, ink ui.Color) {
	t := theme.Current()
	prefs := a.terminal.prefs
	request := block.request
	family := strings.Join(t.Font.Family.Ui, ", ")
	caption := func(text string) ui.Element {
		return ui.Text(c, text).Font(family).FontSize(t.Font.Size.Caption).TextColor(ink.Alpha(.55)).SingleLine().Shrink(0)
	}
	// A panel holds terminal text; long lines wrap inside it.
	panel := func(key string, lines []terminalLine) {
		ui.Column(c).Key(key).FillWidth().MinWidth(0).Radius(t.Radius.ControlSm).Background(ink.Alpha(.06)).
			Padding(t.Space.N8, t.Space.N10).Children(func() {
			for _, line := range lines {
				ui.RichText(c).FillWidth().MinWidth(0).FixedLineHeight(prefs.LineHeight).Shrink(0).Children(func() {
					ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...)
				})
			}
		})
	}
	ui.Column(c).Key(request.OperationID).FillWidth().MinWidth(0).Radius(t.Radius.Card).Background(ink.Alpha(.05)).
		Padding(t.Space.N10, t.Space.N12, t.Space.N12, t.Space.N12).Gap(t.Space.N8).Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			icon := uikit.IconDocument
			if request.Tool == "ssh_exec" {
				icon = uikit.IconTerminal
			}
			size := t.C.IconButton.IconSizeSm
			ui.Icon(c, uikit.IconSVG(icon)).Size(size, size).TextColor(ink.Alpha(.7)).Shrink(0)
			ui.Text(c, mcpActionName(request.Tool)).Font(family).FontSize(t.Font.Size.Footnote).FontWeight(t.Font.Weight.Semibold).
				TextColor(ink).SingleLine().Shrink(0)
			caption(request.At.Local().Format(time.TimeOnly)).FontFeatures("tnum")
			ui.Spacer(c)
			if view.running[request.OperationID] == block && !block.queued {
				agentPill(c, locales.MessageRunning(), uikit.Color(scheme.Color(4)))
			}
			for index, result := range block.results {
				agentResultPill(c.Key(index), result, scheme)
				caption(result.elapsed.Round(10 * time.Millisecond).String()).FontFeatures("tnum")
			}
		})
		caption(locales.AgentSessionInput())
		var input []terminalLine
		for line := range strings.SplitSeq(agentRequestText(request), "\n") {
			input = append(input, terminalLine{{Text: line, Bold: true}})
		}
		panel("input", input)
		for index, result := range block.results {
			if result.detail == "" || result.exitCode != nil {
				continue
			}
			ui.Text(c.Key(index), strings.Join(strings.Fields(agentText(result.detail)), " ")).Font(family).
				FontSize(t.Font.Size.Footnote).TextColor(agentResultColor(result, scheme)).FillWidth().MinWidth(0)
		}
		lines := append(block.lines[:len(block.lines):len(block.lines)], block.tail()...)
		if len(lines) == 0 {
			return
		}
		ui.Row(c).FillWidth().MinWidth(0).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			caption(locales.AgentSessionOutput())
			if block.dropped > 0 {
				caption(fmt.Sprintf(locales.AgentSessionDropped(), block.dropped))
			}
		})
		panel("output", lines)
	})
}

// agentResultColor is the palette's green, yellow or red for how an operation ended.
func agentResultColor(result agentResult, scheme *termtheme.Scheme) ui.Color {
	index := 1
	switch result.status {
	case "completed", "queued":
		index = 2
	case "intercepted", "partial_intercepted", "permission_denied":
		index = 3
	}
	if code := result.exitCode; code != nil && *code != 0 {
		index = 1
	}
	return uikit.Color(scheme.Color(index))
}

func agentResultPill(c *ui.Context, result agentResult, scheme *termtheme.Scheme) {
	label := mcpStageName(result.status)
	if code := result.exitCode; code != nil && (result.status == "completed" || result.status == "failed") {
		label = fmt.Sprintf(locales.AgentSessionExit(), *code)
	}
	agentPill(c, label, agentResultColor(result, scheme))
}

// agentPill is a status label tinted with its own color, which stays legible
// on any terminal background.
func agentPill(c *ui.Context, label string, ink ui.Color) {
	t := theme.Current()
	p := t.C.Badge
	ui.Row(c).Height(p.HeightSm).Shrink(0).Padding(0, p.PaddingXCompact).AlignItems(ui.Center).Radius(p.Radius).
		Background(ink.Alpha(.16)).Role(ui.RoleStatus).Label(label).Children(func() {
		ui.Text(c, label).Font(strings.Join(t.Font.Family.Ui, ", ")).FontSize(p.FontSizeCompact).TextColor(ink).SingleLine().PassThrough()
	})
}
