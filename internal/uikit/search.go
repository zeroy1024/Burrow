package uikit

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type SearchProps struct {
	Label, Placeholder, Shortcut string
	Icon                         *ui.SVG
	Handle                       *ui.Handle
}

type SearchParts struct {
	Root, Input ui.Element
}

// SearchField keeps the caller's handle and the input's full hit area.
func SearchField(c *ui.Context, value *string, props SearchProps) SearchParts {
	t := theme.Current()
	p := t.C.Search
	rightPadding := p.PaddingX
	if props.Shortcut != "" {
		width, _ := ui.ShapeText(props.Shortcut, ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: p.KbdSize}, 0).Size()
		rightPadding += width + p.Gap
	}
	parts := SearchParts{}
	parts.Root = ui.Box(c).Height(p.Height).Shrink(0).Margin(0, p.MarginX, p.MarginBottom, p.MarginX).
		Radius(p.Radius).Background(Color(p.Bg))
	parts.Root.Children(func() {
		parts.Input = ui.TextInputBase(c.Key("input"), value).Bind(props.Handle).Fill().MinWidth(0).
			Padding(0, rightPadding, 0, p.PaddingX+t.Size.Icon+p.Gap).Radius(p.Radius).
			FontSize(p.FontSize).FixedLineHeight(p.LineHeight).TextColor(Color(t.Color.Text.Primary)).
			Placeholder(props.Placeholder).Label(props.Label)
		if props.Shortcut != "" {
			parts.Input.Description(props.Shortcut)
		}
		// PassThrough is per element; decorations must never intercept input clicks.
		ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).Padding(0, p.PaddingX).
			Gap(p.Gap).AlignItems(ui.Center).PassThrough().Children(func() {
			ui.Icon(c, props.Icon).Size(t.Size.Icon, t.Size.Icon).Shrink(0).TextColor(Color(p.Text)).PassThrough()
			ui.Spacer(c).PassThrough()
			if props.Shortcut != "" {
				Kbd(c, props.Shortcut, KbdProps{Tone: KbdFlat, FontSize: p.KbdSize, Text: &p.KbdText}).Shrink(0)
			}
		})
	})
	return parts
}
