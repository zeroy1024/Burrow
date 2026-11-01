package uikit

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type BannerProps struct {
	Tone   Tone
	Icon   *ui.SVG
	Action string
}

type BannerResult struct {
	Root   ui.Element
	Action bool
}

func Banner(c *ui.Context, message string, props BannerProps) BannerResult {
	t := theme.Current()
	p := t.C.Banner
	background, ink, glyph := p.NeutralBg, p.NeutralText, p.NeutralIcon
	role := ui.RoleStatus
	switch props.Tone {
	case ToneAccent:
		background, ink, glyph = p.InfoBg, p.InfoText, p.InfoIcon
	case ToneWarning:
		background, ink, glyph = p.WarnBg, p.WarnText, p.WarnIcon
	case ToneDanger:
		background, ink, glyph = p.DangerBg, p.DangerText, p.DangerIcon
	}
	result := BannerResult{Root: ui.Row(c).FillWidth().Shrink(0).Padding(p.PaddingY, p.PaddingX).
		Radius(p.Radius).Background(Color(background)).Gap(p.Gap).Role(role).Label(message)}
	if props.Tone == ToneDanger {
		// This SDK exposes Status, but no Alert role. Announce a changed error
		// explicitly instead of misrepresenting an inline banner as a dialog.
		announced := ui.Local(result.Root, "uikit.banner.announced", func() string { return "" })
		if *announced != message {
			c.Announce(message)
			*announced = message
		}
	}
	result.Root.Children(func() {
		if props.Icon != nil {
			ui.Icon(c, props.Icon).Size(p.Icon, p.Icon).TextColor(Color(glyph)).PassThrough()
		}
		ui.Text(c, message).FontSize(t.Font.Size.Small).TextColor(Color(ink)).Grow(1).MinWidth(0).PassThrough()
		if props.Action != "" {
			result.Action = Button(c, props.Action, ButtonProps{Variant: ButtonPlain}).TextColor(Color(ink)).Clicked()
		}
	})
	return result
}

type TestResultState uint8

const (
	TestPending TestResultState = iota
	TestOK
	TestFailed
)

type TestState struct {
	State               TestResultState
	Visible             bool
	Summary, Detail, ID string
	Elapsed             time.Duration
}

type TestResultProps struct {
	SuccessIcon, FailureIcon *ui.SVG
	InfoIcon                 *ui.SVG
	Motion                   MotionMode
}

type TestResultParts struct {
	Root, Label, Info ui.Element
}

func TestResult(c *ui.Context, state *TestState, props TestResultProps) TestResultParts {
	p := theme.Current().C.TestResult
	row := ui.Row(c).MinWidth(0).Shrink(1).Gap(p.Gap).AlignItems(ui.Center).
		Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}})
	parts := TestResultParts{Root: row}
	shown := float32(0)
	if state.Visible {
		shown = 1
	}
	alpha := Visual(row, "result-alpha", shown, p.Crossfade)
	row.Opacity(alpha).Hidden(alpha == 0 && !state.Visible)
	summary := state.Summary
	if summary == "" {
		switch state.State {
		case TestOK:
			summary = fmt.Sprintf("连接成功 · %d ms", state.Elapsed.Milliseconds())
		case TestFailed:
			summary = "连接失败"
		default:
			summary = "正在连接…"
		}
	}
	announced := ui.Local(row, "uikit.testresult.announced", func() string { return "" })
	if state.State == TestPending {
		*announced = ""
	} else if state.Visible {
		identity := fmt.Sprintf("%d|%s|%s|%s", state.State, state.ID, summary, state.Detail)
		if *announced != identity {
			c.Announce(summary)
			*announced = identity
		}
	}
	if props.SuccessIcon == nil {
		props.SuccessIcon = IconSVG(IconCheckCircle)
	}
	if props.FailureIcon == nil {
		props.FailureIcon = IconSVG(IconAlertCircle)
	}
	if props.InfoIcon == nil {
		props.InfoIcon = IconSVG(IconInfo)
	}
	row.Children(func() {
		stage := "testing"
		if state.State == TestOK {
			stage = "success"
		} else if state.State == TestFailed {
			stage = "failure"
		}
		ui.Row(c).Key(stage).MinWidth(0).Gap(p.Gap).AlignItems(ui.Center).
			Transition(ui.ElementTransition{Duration: p.Crossfade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
			if state.State == TestPending {
				Spinner(c, SpinnerProps{Size: SizeXs, Tone: ToneSecondary, Label: "正在连接", Motion: props.Motion}).Size(p.SpinnerSize, p.SpinnerSize).Shrink(0)
				ui.Text(c, "正在连接…").FontSize(p.FontSize).TextColor(Color(p.Testing)).MinWidth(0).SingleLine()
				return
			}
			icon, ink, glyph := props.SuccessIcon, p.Success, p.SuccessIcon
			if state.State == TestFailed {
				icon, ink, glyph = props.FailureIcon, p.Fail, p.FailIcon
			}
			ui.Icon(c, icon).Size(p.Icon, p.Icon).Shrink(0).TextColor(Color(glyph))
			parts.Label = ui.Text(c, summary).FontSize(p.FontSize).TextColor(Color(ink)).MinWidth(0).SingleLine()
			if state.Detail != "" {
				TestResultTooltip(c, parts.Label, state.Detail)
			}
			if state.State == TestFailed {
				parts.Info = ui.Icon(c, props.InfoIcon).Size(p.InfoSize, p.InfoSize).Shrink(0).TextColor(Color(p.Info))
				TestResultTooltip(c, parts.Info, state.Detail)
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
