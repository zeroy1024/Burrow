package uikit

import (
	"image/color"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type SettingsGroupProps struct {
	Key       any
	Title     string
	Footnote  string
	HeaderRow bool
	Actions   func()
}

func SettingsGroup(c *ui.Context, props SettingsGroupProps, content func()) ui.Element {
	t := theme.Current()
	p := t.C.Settings
	group := ui.Column(c).FillWidth().Gap(p.GroupTitleGap)
	if props.Key != nil {
		group.Key(props.Key)
	}
	group.Children(func() {
		heading := func() ui.Element {
			return ui.Text(c, props.Title).FontSize(p.GroupTitleSize).FontWeight(p.GroupTitleWeight).TextColor(Color(p.GroupTitle))
		}
		if props.HeaderRow {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
				heading().Grow(1)
				if props.Actions != nil {
					props.Actions()
				}
			})
		} else {
			heading()
		}
		SettingsGroupBody(c).Children(content)
		if props.Footnote != "" {
			ui.Text(c, props.Footnote).FontSize(p.GroupFootnoteSize).TextColor(Color(p.GroupFootnote))
		}
	})
	return group
}

func SettingsGroupBody(c *ui.Context) ui.Element {
	p := theme.Current().C.Settings
	return ui.Column(c).FillWidth().Radius(p.GroupRadius).Background(Color(p.GroupBg)).Border(p.GroupBorderWidth, Color(p.GroupBorder))
}

func SettingsDivider(c *ui.Context) ui.Element {
	t := theme.Current()
	p := t.C.Settings
	return ui.Row(c).FillWidth().Padding(0, 0, 0, p.RowDividerInset).Children(func() {
		ui.Box(c).FillWidth().Height(t.Size.Divider).Background(Color(p.RowDivider))
	})
}

type SettingsRowProps struct {
	Key    any
	Gap    float32
	Inline bool
}

func SettingsRow(c *ui.Context, props SettingsRowProps) ui.Element {
	p := theme.Current().C.Settings
	row := ui.Row(c).FillWidth().Gap(props.Gap).AlignItems(ui.Center)
	if props.Key != nil {
		row.Key(props.Key)
	}
	if !props.Inline {
		row.MinHeight(p.RowMinHeight).Padding(p.RowPaddingY, p.RowPaddingX)
	}
	return row
}

func SettingsTextColumn(c *ui.Context) ui.Element {
	return ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Current().Space.N4)
}

type SettingsNavItemProps struct {
	Key      any
	Label    string
	Icon     *ui.SVG
	Selected bool
}

func SettingsNavItem(c *ui.Context, props SettingsNavItemProps) ui.Element {
	t := theme.Current()
	p := t.C.Settings
	item := ui.ButtonBase(c).Key(props.Key).FillWidth().Height(p.NavItemHeight).
		Padding(0, t.C.GroupHeader.PaddingX).Radius(p.NavItemRadius).Justify(ui.Start).AlignItems(ui.Center).
		Gap(t.C.SiderItem.Gap).Label(props.Label).Selected(props.Selected)
	bg, fg, icon := ui.Transparent, Color(t.Color.Text.Primary), Color(t.Color.Text.Secondary)
	if props.Selected {
		bg, fg, icon = Color(p.NavItemOn), Color(p.NavTextOn), Color(p.NavIconOn)
	} else if item.Hovered() {
		bg = Color(t.Color.State.Hover)
	}
	return item.Background(bg).TextColor(fg).Children(func() {
		ui.Box(c).Size(p.NavIconTile, p.NavIconTile).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, props.Icon).Size(t.Size.Icon, t.Size.Icon).TextColor(icon).PassThrough()
		})
		ui.Text(c, props.Label).FontSize(t.Font.Size.Body).FontWeight(t.Font.Weight.Medium).PassThrough()
	})
}

type ThemeCardProps struct {
	Key        any
	Label      string
	Appearance int
	Selected   bool
}

func ThemeCard(c *ui.Context, props ThemeCardProps) ui.Element {
	t := theme.Current()
	p := t.C.ThemeCard
	var item ui.Element
	ui.Column(c).Grow(1).Gap(t.Space.N8).Children(func() {
		item = ui.ButtonBase(c).Key(props.Key).FillWidth().AspectRatio(p.Aspect).Radius(p.Radius).Clip().Label(props.Label).Selected(props.Selected).
			Border(p.BorderWidth, Color(p.Border))
		if props.Selected {
			item.Border(p.SelectedWidth, Color(p.Selected))
			Shadow(item, p.SelectedShadow)
		}
		item.Draw(func(painter *ui.Painter, rect ui.Rect) { drawThemeThumbnail(painter, rect, props.Appearance, t) })
		ui.Row(c).Justify(ui.Center).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
			radio := ui.Box(c).Size(p.Radio, p.Radio).Radius(p.Radio).Border(t.Size.Divider, Color(t.Color.Border.Control))
			if props.Selected {
				radio.Border(p.RadioRing, Color(p.Selected))
			}
			ui.Text(c, props.Label).FontSize(t.Font.Size.Small)
		})
	})
	return item
}

type AccentSwatchProps struct {
	Key            any
	Label          string
	Selected       bool
	Colors         []color.NRGBA
	SystemGradient bool
}

func AccentSwatch(c *ui.Context, props AccentSwatchProps) ui.Element {
	t := theme.Current()
	p := t.C.AccentSwatch
	swatch := ui.ButtonBase(c).Key(props.Key).Size(p.Size, p.Size).Radius(p.Size).Label(props.Label).Selected(props.Selected)
	if props.SystemGradient {
		swatch.Draw(func(painter *ui.Painter, r ui.Rect) {
			for i, ink := range props.Colors {
				slice := r
				slice.X += float32(i) * r.W / float32(len(props.Colors))
				slice.W = r.W / float32(len(props.Colors))
				painter.Fill(slice, Color(ink), 0)
			}
		})
	} else {
		swatch.Background(Color(props.Colors[0]))
	}
	if props.Selected {
		Shadow(swatch, theme.Shadow{{Spread: p.RingGap, Color: p.RingInset}, {Spread: p.RingGap + p.RingWidth, Color: t.Color.Accent.Default}})
	}
	return Tip(c, swatch, props.Label)
}

func drawThemeThumbnail(p *ui.Painter, r ui.Rect, index int, t *theme.Theme) {
	light, dark := theme.ForAppearance(false), theme.ForAppearance(true)
	sample := light
	if index == 1 {
		sample = dark
	}
	p.Fill(r, Color(sample.C.Window.Bg), t.C.ThemeCard.Radius)
	body := ui.Rect{X: r.X + t.Space.N6, Y: r.Y + t.Space.N12, W: r.W - 2*t.Space.N6, H: r.H - t.Space.N18}
	p.Fill(body, Color(sample.Color.Surface.Card), t.C.Input.Radius)
	sidebar := body
	sidebar.W = body.W / 4
	p.Fill(sidebar, Color(sample.Color.Surface.Control), t.C.Input.Radius)
	if index == 2 {
		right := r
		right.X += r.W / 2
		right.W = r.W / 2
		p.Fill(right, Color(dark.C.Window.Bg), 0)
		right.X += t.Space.N4
		right.Y += t.Space.N12
		right.W -= t.Space.N10
		right.H -= t.Space.N18
		p.Fill(right, Color(dark.Color.Surface.Card), t.C.Input.Radius)
	}
	for i := range 3 {
		line := ui.Rect{X: body.X + body.W/3, Y: body.Y + t.Space.N8 + float32(i)*t.Space.N8, W: body.W / 2, H: t.Space.N2}
		p.Fill(line, Color(sample.Color.Border.Control), t.Space.N2)
	}
}

type SchemeCardProps struct {
	Name       string
	Badge      string
	Selected   bool
	Background color.NRGBA
	Colors     []color.NRGBA
}

func SchemeCard(c *ui.Context, props SchemeCardProps, preview func()) ui.Element {
	t := theme.Current()
	p := t.C.SchemeCard
	item := ui.ButtonBase(c).Key(props.Name).Grow(1).Basis(0).MinWidth(0).Padding(p.Padding).Radius(p.Radius).
		Label(props.Name).Selected(props.Selected).Background(Color(t.Color.Surface.Card)).TextColor(Color(t.Color.Text.Primary)).
		Border(p.BorderWidth, Color(p.Border))
	if props.Selected {
		item.Border(p.SelectedWidth, Color(p.Selected))
	}
	item.Children(func() {
		ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
			ui.Column(c).FillWidth().Height(p.PreviewHeight).Radius(p.PreviewRadius).Padding(t.Space.N6).
				Background(Color(props.Background)).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(p.PreviewFontSize).Clip().Children(preview)
			ui.Row(c).FillWidth().Gap(p.StripeGap).Children(func() {
				for _, ink := range props.Colors {
					ui.Box(c).Grow(1).Height(p.StripeHeight).Radius(p.StripeRadius).Background(Color(ink)).PassThrough()
				}
			})
			ui.Row(c).FillWidth().Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, props.Name).Grow(1).MinWidth(0).FontSize(p.NameSize).FontWeight(p.NameWeight).SingleLine().PassThrough()
				if props.Badge != "" {
					ui.Text(c, props.Badge).FontSize(p.BadgeSize).FontWeight(p.BadgeWeight).TextColor(Color(p.BadgeText)).Background(Color(p.BadgeBg)).Radius(t.Radius.Chip).PassThrough()
				}
			})
		})
	})
	return item
}
