package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type EmptyKind uint8

const (
	EmptyGeneric EmptyKind = iota
	EmptyTerminal
	EmptyFiles
	EmptyConnection
)

type EmptyProps struct {
	Kind                 EmptyKind
	Icon                 *ui.SVG
	IconName             IconName
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
	EmptyCatalogSearch
)

func EmptyMessage(c *ui.Context, text string, kind EmptyMessageKind) ui.Element {
	t := theme.Current()
	e := ui.Text(c, text)
	switch kind {
	case EmptyFolder:
		return e.FillWidth().Padding(t.Space.N24).TextAlign(ui.Center).TextColor(Color(t.Color.Text.Secondary))
	case EmptyCatalogSearch:
		return e.FontSize(t.Font.Size.Small).TextColor(Color(t.Color.Text.Secondary)).Padding(t.Space.N12)
	}
	return e.FontSize(t.Font.Size.Body).TextColor(Color(t.Color.Text.Tertiary))
}

func EmptyState(c *ui.Context, props EmptyProps) EmptyParts {
	t := theme.Current()
	p := t.C.EmptyState
	var root ui.Element
	if props.Kind != EmptyGeneric {
		root = Card(c)
	} else {
		root = ui.Column(c)
	}
	root.Role(ui.RoleStatus).Label(props.Title)
	if props.Kind == EmptyTerminal {
		root.Fill().MinWidth(0)
	} else if props.Kind == EmptyFiles {
		root.Grow(1).FillWidth()
	} else if props.Kind == EmptyConnection {
		root.FillWidth().Grow(1)
	} else {
		root.FillWidth()
	}
	parts := EmptyParts{Root: root.Center().Gap(t.Space.N12)}
	parts.Root.Children(func() {
		icon := props.Icon
		if props.IconName != "" {
			icon = IconSVG(props.IconName)
		}
		if icon != nil {
			ui.Box(c).Size(p.IconTile, p.IconTile).Radius(p.IconRadius).Background(Color(p.IconBg)).Center().Children(func() {
				ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(Color(p.Icon)).PassThrough()
			})
		}
		ui.Text(c, props.Title).FontSize(p.Title).FontWeight(t.Font.Weight.Semibold).MaxWidth(p.MaxWidth).TextAlign(ui.Center)
		if props.Error != "" {
			FieldMessage(c, props.Error, FieldMessageProps{Tone: ToneDanger}).MaxWidth(p.MaxWidth)
		}
		if props.Detail != "" {
			ui.Text(c, props.Detail).FontSize(t.Font.Size.Small).TextColor(Color(p.Detail)).MaxWidth(p.MaxWidth).TextAlign(ui.Center)
		}
		if props.Action != "" {
			parts.Action = Button(c, props.Action, ButtonProps{})
		}
	})
	return parts
}
