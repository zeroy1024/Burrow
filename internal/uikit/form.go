package uikit

import (
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type RowAlign uint8

const (
	RowAlignTop RowAlign = iota
	RowAlignCenter
)

type FormRowProps struct {
	Key         any
	Inline      bool
	Help, Error string
	Align       RowAlign
}

type FormRowParts struct {
	Root, Label, Body, Control ui.Element
}

// FormRow's renderer returns the editor, or a compound host that forwards an
// explicit Focus request to its current editor when the SDK rebuilds.
func FormRow(c *ui.Context, label string, props FormRowProps, body ...func() ui.Element) FormRowParts {
	p := theme.Current().C.FormRow
	parts := FormRowParts{Root: ui.Row(c).FillWidth().Gap(p.LabelGap).Role(ui.RoleGroup).Label(label)}
	if props.Inline || props.Align == RowAlignCenter {
		parts.Root.AlignItems(ui.Center)
	} else {
		parts.Root.AlignItems(ui.Start)
	}
	if props.Key != nil {
		parts.Root.Key(props.Key)
	}
	parts.Root.Children(func() {
		parts.Label = ui.Text(c, label).Width(p.LabelWidth).Shrink(0).FontSize(p.LabelSize).
			TextColor(Color(p.Label)).TextAlign(ui.End)
		if props.Inline {
			parts.Body = ui.Row(c).Grow(1).MinWidth(0)
		} else {
			parts.Label.FixedLineHeight(p.LabelHeight)
			parts.Body = ui.Column(c).Grow(1).MinWidth(0).Gap(p.DescGap)
		}
		if len(body) > 0 {
			parts.Body.Children(func() {
				parts.Control = body[0]().Label(label)
				if props.Help != "" {
					FieldMessage(c, props.Help, FieldMessageProps{})
				}
				if props.Error != "" {
					parts.Control.Error(props.Error)
					FieldMessage(c, props.Error, FieldMessageProps{Tone: ToneDanger})
				}
			})
			parts.Label.OnClick(func() { parts.Control.Focus() })
		}
	})
	return parts
}

func FormSection(c *ui.Context, title string, body func()) ui.Element {
	t := theme.Current()
	return ui.Column(c).FillWidth().Gap(t.C.FormRow.RowGap).Role(ui.RoleGroup).Label(title).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(t.C.Settings.GroupTitleSize).FontWeight(t.C.Settings.GroupTitleWeight).
				TextColor(Color(t.C.Settings.GroupTitle))
		}
		body()
	})
}

type FieldMessageProps struct {
	Tone Tone
	Icon *ui.SVG
}

func FieldMessage(c *ui.Context, message string, props FieldMessageProps) ui.Element {
	t := theme.Current()
	p := t.C.FormRow
	if props.Tone != ToneDanger {
		ink := p.Description
		switch props.Tone {
		case ToneAccent:
			ink = t.Color.Text.Accent
		case ToneSuccess:
			ink = t.Color.Success.Text
		case ToneWarning:
			ink = t.Color.Warning.Text
		}
		return ui.Text(c, message).FontSize(p.DescriptionSize).TextColor(Color(ink))
	}
	row := ui.Row(c).FillWidth().Gap(p.ErrorGap).Margin(p.ErrorGap-p.DescGap, 0, 0, 0).
		AlignItems(ui.Center).Role(ui.RoleStatus).Label(message).Children(func() {
		icon := props.Icon
		if icon == nil {
			icon = IconSVG(IconAlertCircle)
		}
		ui.Icon(c, icon).Size(p.ErrorIcon, p.ErrorIcon).TextColor(Color(p.Error)).PassThrough()
		ui.Text(c, message).Grow(1).MinWidth(0).FontSize(p.ErrorSize).TextColor(Color(p.Error)).PassThrough()
	})
	announced := ui.Local(row, "uikit.field-message.announced", func() string { return "" })
	if *announced != message {
		c.Announce(message)
		*announced = message
	}
	return row
}
