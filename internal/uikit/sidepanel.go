package uikit

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
)

func SidePanelBody(c *ui.Context) ui.Element {
	return ui.Column(c).Width(theme.Current().Size.StatsPanel).FillHeight()
}

func SidePanelViewport(c *ui.Context) ui.Element {
	return ui.Box(c).Key("status-viewport").Grow(1).MinHeight(0).Clip()
}

type SidePanelProps struct {
	Title, Subtitle string
	Width           float32
	Open            *bool
	Scroll          *ui.ScrollState
	Scrollbar       *ScrollbarState
	ScrollInput     func(ui.InputEvent) bool
	ContentPaddingX float32
	ScrollOverlay   func(ui.Element, *ui.ScrollState)
	// Header replaces the title row and reports a close request.
	Header func() bool
	Footer func()
}

type SidePanelResult struct {
	Root, Viewport ui.Element
	Close          bool
}

func SidePanel(c *ui.Context, props SidePanelProps, body func()) SidePanelResult {
	t := theme.Current()
	p := t.C.Panel
	padding := props.ContentPaddingX
	if padding == 0 {
		padding = p.PaddingX
	}
	width := props.Width
	if width == 0 {
		width = p.Width
	}
	result := SidePanelResult{Root: ui.Column(c).Width(width).FillHeight()}
	if props.Open != nil && !*props.Open {
		result.Root.Hidden(true)
		return result
	}
	scroll := props.Scroll
	if scroll == nil {
		scroll = ui.Local(result.Root, "uikit.sidepanel.scroll", func() ui.ScrollState { return ui.ScrollState{} })
	}
	previousMax := scroll.MaxY
	result.Root.DrawOver(func(_ *ui.Painter, _ ui.Rect) {
		if scroll.MaxY != previousMax {
			c.Invalidate()
		}
	})
	result.Root.Children(func() {
		if props.Header != nil {
			result.Close = props.Header()
		} else {
			header := PaneHeader(c, HeaderProps{Density: DensityTitle, Title: props.Title, Subtitle: props.Subtitle})
			header.Root.FillWidth().Height(p.HeaderHeight).Padding(0, padding)
		}
		if result.Close && props.Open != nil {
			*props.Open = false
		}
		result.Viewport = SidePanelViewport(c)
		if props.Footer == nil {
			// The panel sits in a Card, and without a footer the viewport reaches
			// its bottom corners. Only the innermost clip draws rounded corners
			// (outer clips cut rectangles), so the viewport rounds them itself or
			// the bottom fade covers the card's.
			card := t.C.Card.Radius
			result.Viewport.Radius(0, 0, card, card)
		}
		result.Viewport.Children(func() {
			content := ui.Scroll(c).Fill().MinHeight(0).TrackScroll(scroll).Padding(0, padding)
			content.Children(body)
			if scroll.Y < scroll.MaxY {
				ScrollFade(c, EdgeBottom, ScrollFadeProps{
					Height: t.Size.FadeExtent, Gradient: true, Background: Color(t.Color.Surface.Card),
				})
			}
			if props.ScrollOverlay != nil {
				content.ScrollbarInsets(0, -t.C.Scrollbar.TrackWidth, 0, 0)
				if props.ScrollInput != nil {
					content.HandleInput(props.ScrollInput)
				}
				props.ScrollOverlay(result.Viewport, scroll)
			} else {
				Scrollbars(c, result.Viewport, content, scroll, ScrollbarProps{
					Label: props.Title + locales.MessageScrollbar(), State: props.Scrollbar, Input: props.ScrollInput,
				})
			}
		})
		if props.Footer != nil {
			ui.Row(c).FillWidth().Shrink(0).Padding(t.Space.N10, padding).
				BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(Color(t.Color.Border.Divider)).
				AlignItems(ui.Center).Gap(t.Space.N8).Children(props.Footer)
		}
	})
	return result
}
