package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type CodeBlockResult struct {
	Root ui.Element
	Copy bool
}

// CodeBlock keeps complete text selectable and scrollable within a bounded view.
func CodeBlock(c *ui.Context, label, value string) CodeBlockResult {
	t := theme.Current()
	result := CodeBlockResult{Root: ui.Column(c).FillWidth().MinWidth(0).Gap(t.Space.N6)}
	scroll := ui.Local(result.Root, "uikit.codeblock.scroll", func() ui.ScrollState { return ui.ScrollState{} })
	shown := ui.Local(result.Root, "uikit.codeblock.shown", func() string { return "" })
	result.Root.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			Text(c, label, TextProps{Style: TextFootnote, Tone: ToneSecondary})
			ui.Spacer(c)
			result.Copy = IconButton(c, IconCopy, IconButtonProps{
				Key: "copy", Label: "复制" + label, Size: SizeSm,
			}).Clicked()
		})
		content := ui.TextAreaBase(c, &value).FillWidth().MinWidth(0).Lines(2, 12).ReadOnly(true).TrackScroll(scroll).
			Label(label).Padding(t.Space.N8).Radius(t.Radius.ControlSm).
			Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Footnote).
			FixedLineHeight(t.C.Input.LineHeight).TextColor(Color(t.Color.Text.Primary)).
			Background(Color(t.Color.Surface.Sunken)).Border(t.Size.Hairline, Color(t.Color.Border.Subtle))
		// MyGo starts a text area's caret at the end of its text and scrolls
		// the caret into view, as it does again when the text changes: show
		// each new text from its start.
		if *shown != value {
			content.SetTextSelection(0, 0)
			*shown = value
		}
		Scrollbars(c, result.Root, content, scroll, ScrollbarProps{Label: label + "滚动条", Contain: true})
	})
	return result
}
