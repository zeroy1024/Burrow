package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func SidePanelBody(c *ui.Context) ui.Element {
	return ui.Column(c).Width(theme.Current().Size.StatsPanel).FillHeight()
}

type SidePanelHeaderParts struct {
	Root, Close ui.Element
}

func SidePanelHeader(c *ui.Context, title, closeLabel string, closeIcon *ui.SVG) SidePanelHeaderParts {
	t := theme.Current()
	header := PaneHeader(c, HeaderProps{Density: DensityTitle, Title: title, Close: true, CloseLabel: closeLabel})
	header.Root.Height(t.C.Stats.HeaderHeight).Padding(0, t.Space.N14)
	header.CloseButton.Size(t.C.IconButton.SizeXs, t.C.IconButton.SizeSm)
	return SidePanelHeaderParts{Root: header.Root, Close: header.CloseButton}
}

func SidePanelViewport(c *ui.Context) ui.Element {
	return ui.Box(c).Key("status-viewport").Grow(1).MinHeight(0).Clip()
}

type SidePanelProps struct {
	Title, Subtitle, CloseLabel string
	Open                        *bool
	Scroll                      *ui.ScrollState
	Scrollbar                   *ScrollbarState
	ScrollInput                 func(ui.InputEvent) bool
	ContentPaddingX             float32
	ScrollOverlay               func(ui.Element, *ui.ScrollState)
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
	result := SidePanelResult{Root: ui.Column(c).Width(p.Width).FillHeight()}
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
		header := PaneHeader(c, HeaderProps{
			Density: DensityTitle, Title: props.Title, Subtitle: props.Subtitle,
			Close: true, CloseLabel: props.CloseLabel,
		})
		header.Root.FillWidth().Height(p.HeaderHeight).Padding(0, p.PaddingX)
		result.Close = header.Close
		if result.Close && props.Open != nil {
			*props.Open = false
		}
		result.Viewport = SidePanelViewport(c)
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
					Label: props.Title + "滚动条", State: props.Scrollbar, Input: props.ScrollInput,
				})
			}
		})
	})
	return result
}
