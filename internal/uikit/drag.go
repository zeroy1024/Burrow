package uikit

import (
	"image/color"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type DragGhostProps struct {
	Key       any
	Rect      ui.Rect
	Radius    float32
	Animation ui.Animation
}

// DragGhost only draws an already positioned preview; it does not drive a gesture.
func DragGhost(c *ui.Context, props DragGhostProps) ui.Element {
	r := props.Rect
	return ui.Box(c).Key(props.Key).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).
		Radius(props.Radius).PassThrough().Transition(ui.ElementTransition{Position: true, Animation: &props.Animation})
}

func StyleDragGhost(ghost ui.Element, visible float32, duration time.Duration) {
	ghost.Opacity(Visual(ghost, "visible", visible, duration))
	Shadow(ghost, theme.Current().C.DragGhost.Shadow)
}

func DragTabGhost(c *ui.Context, radius float32) ui.Element {
	t := theme.Current()
	return ui.Row(c).Fill().Padding(0, t.C.Tab.PaddingLeft).Gap(t.C.Tab.Gap).
		Radius(radius).Background(Color(t.C.DragGhost.Bg)).PassThrough()
}

func DragTabTitle(c *ui.Context, title string) ui.Element {
	p := theme.Current().C.Tab
	return ui.Text(c, title).FontSize(p.FontSize).FontWeight(p.WeightSelected).TextColor(Color(p.TextSelected)).SingleLine().PassThrough()
}

func DragPaneGhost(c *ui.Context, background color.NRGBA, surface *theme.Theme) ui.Element {
	p := surface.C.DragGhost
	return ui.Column(c).Fill().Radius(p.Radius).Background(Color(background)).Clip().PassThrough().
		Padding(p.PaddingY, p.PaddingRight, p.PaddingY, p.PaddingLeft).Gap(theme.Current().Space.N8)
}

func DragPaneHeader(c *ui.Context, surface *theme.Theme) ui.Element {
	return ui.Row(c).Height(surface.C.DragGhost.HeaderHeight).Shrink(0).Gap(theme.Current().Space.N8).PassThrough()
}

func DragPaneTitle(c *ui.Context, title string, surface *theme.Theme) ui.Element {
	return ui.Text(c, title).FontSize(surface.C.DragGhost.FontSize).FontWeight(theme.Current().Font.Weight.Medium).
		TextColor(Color(surface.C.Pane.Name)).SingleLine().Ellipsis("…").MinWidth(0).PassThrough()
}

func DragSkeleton(c *ui.Context, surface *theme.Theme) ui.Element {
	p := surface.C.DragGhost
	return ui.Column(c).FillWidth().Grow(1).Justify(ui.Center).Gap(theme.Current().Space.N8).PassThrough().Children(func() {
		for _, width := range []float32{p.LineLong, p.LineMedium, p.LineLong, p.LineShort} {
			ui.Box(c).WidthPercent(width * 100).Height(p.LineHeight).Shrink(0).Radius(p.LineRadius).Background(Color(p.LineColor)).PassThrough()
		}
	})
}

func DragLift(c *ui.Context, key any, size ui.Point) ui.Element {
	t := theme.Current()
	return ui.Box(c).Key(key).Absolute().Radius(t.C.DragLift.Radius).
		Height(size.Y).Width(size.X).TextColor(Color(t.C.SiderItem.NameColor)).PassThrough()
}

func StyleDragLift(ghost ui.Element, scale, shadow float32, animation ui.Animation) {
	p := theme.Current().C.DragLift
	ghost.AnimatePose(ui.Pose{Scale: scale}, animation)
	alpha := Visual(ghost, "shadow", shadow, p.LiftDuration)
	ShadowOpacity(ghost, p.Shadow, alpha)
}

func DragLiftSheet(c *ui.Context, offset, height float32) ui.Element {
	p := theme.Current().C.DragLift
	sheet := ui.Box(c).Absolute().Top(offset).Left(offset).Right(offset).Height(height).
		Radius(p.Radius).Background(Color(p.StackBg)).Opacity(p.StackBackOpacity).PassThrough()
	return Shadow(sheet, p.StackShadow)
}

func DragLiftCard(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).
		Padding(0, t.C.SiderItem.PaddingRight, 0, t.C.SiderItem.PaddingLeft).Gap(t.C.SiderItem.Gap).
		Radius(t.C.DragLift.Radius).Background(Color(t.C.DragLift.Bg)).PassThrough()
}

func DragGroupLabel(c *ui.Context, title, count string, chevron *ui.SVG) {
	t := theme.Current()
	ui.Icon(c, chevron).Size(t.Font.Size.Caption, t.Font.Size.Caption).Rotate(-90)
	ui.Text(c, title).FontSize(t.C.GroupHeader.FontSize).FontWeight(t.Font.Weight.Semibold)
	ui.Spacer(c)
	ui.Text(c, count).FontSize(t.C.GroupHeader.CountSize).TextColor(Color(t.C.GroupHeader.Count))
}

func CatalogPlaceholder(c *ui.Context, key any, height float32, transition ui.ElementTransition) ui.Element {
	return ui.Box(c).Key(key).FillWidth().Height(height).Shrink(0).Radius(theme.Current().C.DropIndicator.ZoneRadius).Transition(transition)
}

func StyleCatalogPlaceholder(row ui.Element, background ui.Color, shadow theme.Shadow, animation ui.Animation) {
	row.Background(row.AnimateColor("placeholder-bg", background, animation))
	AnimatedShadow(row, "placeholder-stroke", shadow, animation)
}

func StylePanePlaceholder(pane ui.Element) {
	pane.Draw(func(p *ui.Painter, r ui.Rect) {
		t := theme.Current()
		p.Fill(r, Color(t.C.DropIndicator.PlaceholderBg), t.C.Pane.Radius)
		layer := t.C.DropIndicator.PlaceholderShadow[0]
		p.StrokeDashed(r, Color(layer.Color), t.C.Pane.Radius, layer.Spread)
	})
}

type DropIndicatorProps struct {
	Background, Border  ui.Color
	Radius, BorderWidth float32
}

// DrawDropIndicator paints the application's already computed target rectangle.
func DrawDropIndicator(p *ui.Painter, rect ui.Rect, props DropIndicatorProps) {
	p.Fill(rect, props.Background, props.Radius)
	if props.BorderWidth > 0 {
		p.Stroke(rect, props.Border, props.Radius, props.BorderWidth)
	}
}

func DropHint(c *ui.Context, label string, bottom float32) ui.Element {
	t := theme.Current()
	return ui.Row(c).Absolute().Bottom(bottom).Left(t.Space.N12).Right(t.Space.N12).Center().PassThrough().Children(func() {
		ui.Text(c, label).Padding(t.Space.N6, t.Space.N12).Radius(t.Radius.Pill).Background(Color(t.C.DropIndicator.HintBg)).
			TextColor(Color(t.C.DropIndicator.HintText)).FontSize(t.Font.Size.Footnote)
	})
}
