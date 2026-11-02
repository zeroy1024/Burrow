package uikit

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type PaneKind uint8

const (
	PaneTerminal PaneKind = iota
	PaneFile
)

type PaneProps struct {
	Key                       any
	Label                     string
	Kind                      PaneKind
	Focused, Multiple, Lifted bool
	Surface                   *theme.Theme
}

type PaneSlots struct {
	Header, Body, Footer func()
}

// Pane styles only the frame and presentation slots. Session handles, layout
// animation, terminal colors and work lifetimes remain with the application.
func Pane(c *ui.Context, props PaneProps, slots ...PaneSlots) ui.Element {
	t := props.Surface
	if t == nil {
		t = theme.Current()
	}
	root := ui.Box(c).MinWidth(0).MinHeight(0).Radius(t.C.Pane.Radius).Role(ui.RoleGroup).Label(props.Label)
	if props.Key != nil {
		root.Key(props.Key)
	}
	if props.Kind == PaneFile {
		root.Background(Color(t.C.Card.Bg)).Radius(t.C.Card.Radius).Clip()
		root.DrawOver(func(p *ui.Painter, rect ui.Rect) {
			ink, width := t.Color.Border.Card, t.Size.Divider
			if props.Focused {
				ink, width = t.C.FilePaneFocus.BorderFocused, t.C.FilePaneFocus.BorderWidthFocused
			}
			p.Stroke(rect, Color(ink), t.C.Card.Radius, width)
		})
	} else if !props.Lifted {
		StylePaneFocus(root, props.Focused, props.Multiple, t)
	}
	if len(slots) != 0 {
		root.Column().Children(func() {
			for _, slot := range []func(){slots[0].Header, slots[0].Body, slots[0].Footer} {
				if slot != nil {
					slot()
				}
			}
		})
	}
	return root
}

func StylePaneCard(card ui.Element, surface *theme.Theme, background ui.Color, paintRadius float32) ui.Element {
	return card.Column().Key("card").Absolute().Top(0).Right(0).Bottom(0).Left(0).
		Radius(surface.C.Pane.Radius).Background(background).Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		// Absolute children paint after scroll content; the backdrop belongs here.
		p.Fill(r, background, paintRadius)
	})
}

type PaneAppearance struct {
	Opacity, Saturation, FooterOpacity float32
	TitleColor, LoginColor             ui.Color
	TitleWeight                        int
}

func PaneVisualStyle(pane ui.Element, focused, hovered, multiple bool, surface *theme.Theme) PaneAppearance {
	p, f := surface.C.Pane, surface.C.PaneFocus
	style := PaneAppearance{
		Opacity: 1, Saturation: 1, FooterOpacity: 1,
		TitleColor: Color(f.NameFocused), LoginColor: Color(p.Host), TitleWeight: f.NameWeightFocused,
	}
	if multiple && !focused {
		style.Opacity, style.Saturation, style.FooterOpacity = f.ContentOpacityUnfocused, f.ContentSaturateUnfocused, f.StatusOpacityUnfocused
		style.TitleColor, style.LoginColor, style.TitleWeight = Color(f.NameUnfocused), Color(f.HostUnfocused), f.NameWeightUnfocused
		if hovered {
			style.Opacity, style.Saturation = f.ContentOpacityHover, f.ContentSaturateHover
			style.TitleColor = Color(f.NameFocused)
		}
	}
	style.Opacity = Visual(pane, "output-opacity", style.Opacity, f.Transition)
	style.Saturation = Visual(pane, "output-saturation", style.Saturation, f.Transition)
	style.FooterOpacity = Visual(pane, "footer-opacity", style.FooterOpacity, f.Transition)
	if !multiple {
		style.Opacity, style.Saturation, style.FooterOpacity = 1, 1, 1
	}
	style.TitleColor = pane.AnimateColor("host-color", style.TitleColor, Tween(f.Transition))
	style.LoginColor = pane.AnimateColor("login-color", style.LoginColor, Tween(f.Transition))
	return style
}

type HeaderAction struct {
	Key      any
	Icon     IconName
	Label    string
	Shortcut string
	On       *bool
	Disabled bool
}

type HeaderProps struct {
	Key             any
	Density         Density
	Title, Subtitle string
	Leading, Center func()
	Actions         []HeaderAction
	Close           bool
	CloseLabel      string
	Surface         *theme.Theme
}

type HeaderResult struct {
	Root, CloseButton ui.Element
	Buttons           []ui.Element
	Action            int
	Close             bool
}

func PaneHeader(c *ui.Context, props HeaderProps) HeaderResult {
	t := props.Surface
	if t == nil {
		t = theme.Current()
	}
	p := t.C.PaneHeader
	height := p.HeightCompact
	if props.Density == DensityToolbar {
		height = p.HeightToolbar
	} else if props.Density == DensityTitle {
		height = p.HeightTitle
	}
	result := HeaderResult{Action: -1, Buttons: make([]ui.Element, 0, len(props.Actions))}
	result.Root = ui.Row(c).Height(height).MinWidth(0).Shrink(0).Padding(0, p.PaddingRight, 0, p.PaddingLeft).
		Gap(p.Gap).AlignItems(ui.Center)
	if props.Key != nil {
		result.Root.Key(props.Key)
	}
	result.Root.Children(func() {
		if props.Leading != nil {
			props.Leading()
		}
		if props.Title != "" || props.Subtitle != "" {
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
				font := t.Font.Size.Body
				if props.Density == DensityTitle {
					font = p.TitleFont
				}
				if props.Title != "" {
					ui.Text(c, props.Title).FontSize(font).FontWeight(p.TitleWeight).SingleLine().Ellipsis("…").MinWidth(0).PassThrough()
				}
				if props.Subtitle != "" {
					ui.Text(c, props.Subtitle).FontSize(t.Font.Size.Footnote).TextColor(Color(t.Color.Text.Secondary)).
						SingleLine().Ellipsis("…").PassThrough()
				}
			})
		}
		if props.Center != nil {
			props.Center()
		}
		if props.Title == "" && props.Subtitle == "" && props.Center == nil && (len(props.Actions) != 0 || props.Close) {
			ui.Spacer(c).PassThrough()
		}
		for index, action := range props.Actions {
			key := action.Key
			if key == nil {
				key = action.Label
			}
			button := IconButton(c.Key(key), action.Icon, IconButtonProps{
				Label: action.Label, Shortcut: action.Shortcut, On: action.On,
				Disabled: action.Disabled, Size: SizeSm, Surface: t,
			})
			result.Buttons = append(result.Buttons, button)
			if button.Clicked() || button.Changed() {
				result.Action = index
			}
		}
		if props.Close {
			label := props.CloseLabel
			if label == "" {
				label = "关闭面板"
			}
			result.CloseButton = IconButton(c, IconClose, IconButtonProps{Label: label, Size: SizeSm, Surface: t})
			result.Close = result.CloseButton.Clicked()
		}
	})
	return result
}

func PaneShortcut(c *ui.Context, keys string, focused bool, surface *theme.Theme) ui.Element {
	f := surface.C.PaneFocus
	text, background := Color(f.KbdText), Color(f.KbdBg)
	if focused {
		text, background = Color(f.KbdTextFocused), Color(f.KbdBgFocused)
	}
	return ui.Text(c, keys).FontSize(f.KbdSize).FontWeight(f.KbdWeight).
		FixedLineHeight(f.KbdLineHeight).SingleLine().Padding(f.KbdPaddingY, f.KbdPaddingX).Radius(f.KbdRadius).
		Background(background).TextColor(text)
}

func PaneTitle(c *ui.Context, title string, style PaneAppearance, surface *theme.Theme) ui.Element {
	return ui.Text(c, title).FontSize(surface.C.Pane.HeaderFontSize).FontWeight(style.TitleWeight).TextColor(style.TitleColor).
		SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
}

func PaneLogin(c *ui.Context, login string, style PaneAppearance, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	return ui.Text(c, login).Font(strings.Join(p.HostFamily, ", ")).FontSize(p.HostSize).TextColor(style.LoginColor).
		SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
}

func PaneActions(c *ui.Context, width float32, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	return ui.Box(c).Key("pane-actions").Absolute().Top(p.ActionTop).Right(p.ActionRight).Size(width, p.ActionHeight)
}

func PaneActionBackdrop(c *ui.Context, width, blurRadius float32, background ui.Color, surface *theme.Theme) {
	p := surface.C.Pane
	material := ui.Box(c).Absolute().Top(0).Left(0).Size(width, p.ActionHeight).Radius(p.ActionRadius).PassThrough()
	tint := background
	if blurRadius > 0 {
		material.Material(glass.Blur{Radius: blurRadius})
		tint = background.Alpha(p.ActionTint)
	}
	ui.Box(c).Absolute().Top(0).Left(0).Size(width, p.ActionHeight).Radius(p.ActionRadius).PassThrough().
		Background(tint)
}

type PaneIconButtonProps struct {
	Shortcut string
	Disabled bool
}

func PaneIconButton(c *ui.Context, label string, icon *ui.SVG, surface *theme.Theme, options ...PaneIconButtonProps) ui.Element {
	props := PaneIconButtonProps{}
	if len(options) != 0 {
		props = options[0]
	}
	return IconButton(c, icon, IconButtonProps{Label: label, Surface: surface, Shortcut: props.Shortcut, Disabled: props.Disabled})
}

func StylePaneActions(actions ui.Element, target float32, surface *theme.Theme) float32 {
	visibility := Visual(actions, "visibility", target, surface.C.PaneFocus.Transition)
	actions.Opacity(visibility)
	return visibility
}

type FooterProps struct {
	Key        any
	Leading    IconName
	LeadingSVG *ui.SVG
	Source     string
	Items      []string
	Trailing   string
	TrailingFn func()
	NoDividers bool
	Surface    *theme.Theme
}

func PaneFooter(c *ui.Context, props FooterProps) ui.Element {
	t := props.Surface
	if t == nil {
		t = theme.Current()
	}
	p := t.C.PaneFooter
	root := ui.Row(c).Height(p.Height).Padding(0, p.PaddingX).Gap(p.Gap).
		TextColor(Color(p.Text)).FontSize(p.Font).AlignItems(ui.Center)
	if props.Key != nil {
		root.Key(props.Key)
	}
	root.Children(func() {
		if props.Leading != "" {
			Icon(c, props.Leading, IconProps{Tone: ToneSecondary}).Shrink(0).PassThrough()
		} else if props.LeadingSVG != nil {
			Icon(c, props.LeadingSVG, IconProps{Tone: ToneSecondary}).Shrink(0).PassThrough()
		}
		font := ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: p.Font}
		if props.Source != "" {
			sourceFont := font
			sourceFont.Weight = t.C.StatusBar.HostWeight
			middleLabel(c, props.Source, sourceFont, Color(t.C.StatusBar.HostColor), 0, false)
		}
		for index, text := range props.Items {
			if (index > 0 || props.Source != "") && !props.NoDividers {
				Divider(c, DividerProps{Vertical: true})
			}
			middleLabel(c, text, font, Color(p.Text), 0, false)
		}
		if props.Trailing != "" || props.TrailingFn != nil {
			ui.Spacer(c).PassThrough()
		}
		if props.Trailing != "" {
			ui.Text(c, props.Trailing).SingleLine().PassThrough()
		}
		if props.TrailingFn != nil {
			props.TrailingFn()
		}
	})
	return root
}

func FormatSelection(total, selected int, bytes int64) string {
	text := fmt.Sprintf("%d 项", total)
	if selected == 0 {
		return text
	}
	unit, amount := "B", float64(bytes)
	for _, next := range []string{"KB", "MB", "GB", "TB"} {
		if amount < 1024 {
			break
		}
		amount, unit = amount/1024, next
	}
	if unit == "B" {
		return fmt.Sprintf("%s · 已选 %d 项（%d B）", text, selected, bytes)
	}
	return fmt.Sprintf("%s · 已选 %d 项（%.1f %s）", text, selected, amount, unit)
}

func PaneHairline(c *ui.Context, surface *theme.Theme) ui.Element {
	p := surface.C.Pane
	hairline := ui.Box(c).Key("hairline").Absolute().Top(0).Right(0).Bottom(0).Left(0).Radius(p.Radius).PassThrough()
	return Shadow(hairline, p.Outline)
}

func StylePaneFocus(pane ui.Element, focused, multiple bool, surface *theme.Theme) {
	p := surface.C.PaneFocus
	focusTarget, hoverTarget := float32(0), float32(0)
	if multiple && focused {
		focusTarget = 1
	}
	if multiple && !focused && pane.Hovered() {
		hoverTarget = 1
	}
	focus := Visual(pane, "focus-ring", focusTarget, p.Transition)
	hover := Visual(pane, "hover-ring", hoverTarget, p.Transition)
	if !multiple {
		return
	}
	ShadowOpacity(pane, p.RingHover, hover)
	ShadowOpacity(pane, p.RingFocused, focus)
}
