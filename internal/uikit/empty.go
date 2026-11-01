package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type EmptyKind uint8

const (
	EmptyTerminal EmptyKind = iota
	EmptyFiles
	EmptyConnection
)

type EmptyProps struct {
	Kind                 EmptyKind
	Icon                 *ui.SVG
	Title, Detail, Error string
	Action               string
}

type EmptyParts struct {
	Root, Action ui.Element
}

type EmptyMessageKind uint8

const (
	EmptyFolder EmptyMessageKind = iota
	EmptySettingsSearch
)

func EmptyMessage(c *ui.Context, text string, kind EmptyMessageKind) ui.Element {
	t := theme.Current()
	e := ui.Text(c, text)
	if kind == EmptyFolder {
		e.FillWidth().Padding(t.Space.N24)
	} else {
		e.FontSize(t.Font.Size.Body)
	}
	return e.TextColor(Color(t.Color.Text.Tertiary))
}

func EmptyState(c *ui.Context, props EmptyProps) EmptyParts {
	t := theme.Current()
	root := Card(c)
	if props.Kind == EmptyTerminal {
		root.Fill().MinWidth(0)
	} else if props.Kind == EmptyFiles {
		root.Grow(1).FillWidth()
	} else {
		root.FillWidth().Grow(1)
	}
	parts := EmptyParts{Root: root.Center().Gap(t.Space.N12)}
	parts.Root.Children(func() {
		ui.Icon(c, props.Icon).Size(t.C.Pane.EmptyIconSize, t.C.Pane.EmptyIconSize).TextColor(Color(t.Color.Text.Secondary))
		title := ui.Text(c, props.Title).FontSize(t.C.Pane.EmptyTitleSize)
		if props.Kind == EmptyTerminal {
			title.FontWeight(t.C.Pane.EmptyTitleWeight)
		} else if props.Kind == EmptyConnection {
			title.FontWeight(t.Font.Weight.Semibold)
		}
		if props.Kind == EmptyConnection {
			if props.Error != "" {
				ui.Text(c, props.Error).TextColor(Color(t.Color.Danger.Text))
			}
			if props.Action != "" {
				parts.Action = Button(c, props.Action, ButtonProps{})
			}
			ui.Text(c, props.Detail).FontSize(t.Font.Size.Caption).TextColor(Color(t.Color.Text.Secondary))
			return
		}
		detail := ui.Text(c, props.Detail)
		if props.Kind == EmptyTerminal {
			detail.FontSize(t.Font.Size.Body)
		}
		detail.TextColor(Color(t.Color.Text.Secondary))
		if props.Action != "" {
			parts.Action = Button(c, props.Action, ButtonProps{})
		}
	})
	return parts
}
