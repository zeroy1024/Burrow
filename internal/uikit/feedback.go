package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

// Notify uses the native toast queue and the common UITheme. Its lifecycle stays
// with the window; no application callback or business state is retained here.
func Notify(c *ui.Context, message string) { c.Toast(message) }

func Divider(c *ui.Context) ui.Element { return ui.Divider(c) }

// Banner retains the existing edit-session notice geometry and neutral tone.
func Banner(c *ui.Context, message string, icon *ui.SVG) ui.Element {
	t := theme.Current()
	p := t.C.Modal
	return ui.Row(c).Shrink(0).Margin(0, p.BodyPaddingX, t.Space.N12, p.BodyPaddingX).Padding(t.Space.N8, t.Space.N10).
		Radius(t.Radius.Control).Background(Color(t.Color.Surface.Sunken)).Gap(t.Space.N6).Children(func() {
		ui.Icon(c, icon).Size(t.Size.IconSm, t.Size.IconSm).TextColor(Color(t.Color.Text.Secondary))
		ui.Text(c, message).FontSize(t.Font.Size.Small).TextColor(Color(t.Color.Text.Muted)).Grow(1).MinWidth(0)
	})
}

type TestResultState uint8

const (
	TestPending TestResultState = iota
	TestOK
	TestFailed
)

type TestResultProps struct {
	State                    TestResultState
	Visible                  bool
	Summary, Detail          string
	SuccessIcon, FailureIcon *ui.SVG
	InfoIcon                 *ui.SVG
}

type TestResultParts struct {
	Root, Label, Info ui.Element
}

func TestResult(c *ui.Context, props TestResultProps) TestResultParts {
	p := theme.Current().C.TestResult
	row := ui.Row(c).Key("test-result").MinWidth(0).Shrink(1).Gap(p.Gap).AlignItems(ui.Center).
		Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}})
	parts := TestResultParts{Root: row}
	shown := float32(0)
	if props.Visible {
		shown = 1
	}
	alpha := Visual(row, "result-alpha", shown, p.Crossfade)
	row.Opacity(alpha).Hidden(alpha == 0 && !props.Visible)
	row.Children(func() {
		stage := "testing"
		if props.State == TestOK {
			stage = "success"
		} else if props.State == TestFailed {
			stage = "failure"
		}
		ui.Row(c).Key(stage).MinWidth(0).Gap(p.Gap).AlignItems(ui.Center).
			Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
			if props.State == TestPending {
				ui.Spinner(c).Size(p.SpinnerSize, p.SpinnerSize).Shrink(0).Label("正在连接").TextColor(Color(p.Testing))
				ui.Text(c, "正在连接…").FontSize(p.FontSize).TextColor(Color(p.Testing)).MinWidth(0).SingleLine()
				return
			}
			icon, ink, glyph := props.SuccessIcon, p.Success, p.SuccessIcon
			if props.State == TestFailed {
				icon, ink, glyph = props.FailureIcon, p.Fail, p.FailIcon
			}
			ui.Icon(c, icon).Size(p.Icon, p.Icon).Shrink(0).TextColor(Color(glyph))
			parts.Label = ui.Text(c, props.Summary).FontSize(p.FontSize).TextColor(Color(ink)).MinWidth(0).SingleLine()
			if props.Detail != "" {
				TestResultTooltip(c, parts.Label, props.Detail)
			}
			if props.State == TestFailed {
				parts.Info = ui.Icon(c, props.InfoIcon).Size(p.InfoSize, p.InfoSize).Shrink(0).TextColor(Color(p.Info))
				TestResultTooltip(c, parts.Info, props.Detail)
			}
		})
	})
	return parts
}

func TestResultTooltip(c *ui.Context, anchor ui.Element, detail string) {
	t := theme.Current()
	lines := strings.Split(detail, "\n")
	if len(lines) > 3 {
		lines = append(lines[:2], lines[len(lines)-1])
	}
	text := strings.Join(lines, "\n")
	anchor.Description(text)
	ui.TooltipBase(c, anchor, func(panel ui.Element) {
		StyleTooltip(panel).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, t.C.Tooltip.Offset, 0).
			Children(func() { ui.Text(c, text).Font(strings.Join(t.C.Input.MonoFont, ", ")).FontSize(t.Font.Size.Footnote) })
	})
}
