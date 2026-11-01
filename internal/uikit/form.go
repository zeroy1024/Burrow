package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type FormRowProps struct {
	Key    any
	Inline bool
}

type FormRowParts struct {
	Root, Label, Body ui.Element
}

// FormRow exposes its body so validation messages can be read after its controls.
func FormRow(c *ui.Context, label string, props FormRowProps) FormRowParts {
	p := theme.Current().C.FormRow
	parts := FormRowParts{Root: ui.Row(c).FillWidth().Gap(p.LabelGap)}
	if props.Inline {
		parts.Root.AlignItems(ui.Center)
	} else {
		parts.Root.AlignItems(ui.Start).Role(ui.RoleGroup).Label(label)
	}
	if props.Key != nil {
		parts.Root.Key(props.Key)
	}
	parts.Root.Children(func() {
		parts.Label = ui.Text(c, label).Width(p.LabelWidth).FontSize(p.LabelSize).TextColor(Color(p.Label)).TextAlign(ui.End)
		if props.Inline {
			parts.Body = ui.Row(c).Grow(1).MinWidth(0)
		} else {
			parts.Label.Shrink(0).FixedLineHeight(p.LabelHeight)
			parts.Body = ui.Column(c).Grow(1).MinWidth(0).Gap(p.DescGap)
		}
	})
	return parts
}

type FieldMessageProps struct {
	Error bool
	Icon  *ui.SVG
}

func FieldMessage(c *ui.Context, message string, props FieldMessageProps) ui.Element {
	p := theme.Current().C.FormRow
	if !props.Error {
		return ui.Text(c, message).FontSize(p.DescriptionSize).TextColor(Color(p.Description))
	}
	return ui.Row(c).FillWidth().Gap(p.ErrorGap).Margin(p.ErrorGap-p.DescGap, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, props.Icon).Size(p.ErrorIcon, p.ErrorIcon).TextColor(Color(p.Error))
		ui.Text(c, message).Grow(1).MinWidth(0).FontSize(p.ErrorSize).TextColor(Color(p.Error))
	})
}
