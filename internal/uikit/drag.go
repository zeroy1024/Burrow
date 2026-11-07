package uikit

import (
	"image/color"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
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

type DragMorphGhostProps struct {
	Key               any
	PaneRect, TabRect ui.Rect
	Tab, Settling     bool
	Motion            MotionMode
}

const (
	dragMorphShape = "drag-morph-shape"
	dragMorphX     = "drag-morph-x"
	dragMorphY     = "drag-morph-y"
	dragMorphW     = "drag-morph-width"
	dragMorphH     = "drag-morph-height"
	dragMorphAlpha = "drag-morph-tab"
)

// DragMorphGhost keeps pointer movement direct while the same content changes shape.
func DragMorphGhost(c *ui.Context, props DragMorphGhostProps, pane func(), tab func()) ui.Element {
	t := theme.Current()
	ghost := ui.Box(c).Key(props.Key).Absolute().Clip().PassThrough()
	target := float32(0)
	if props.Tab {
		target = 1
	}
	reduced := MotionReduced(c, props.Motion)
	mix := ghost.AnimateTo(dragMorphShape, target, Anim(c, t.C.DragLift.LiftDuration, props.Motion))
	if reduced {
		mix = ghost.Jump(dragMorphShape, target)
	}
	lerp := func(from, to float32) float32 { return from + (to-from)*mix }
	rect := ui.Rect{
		X: lerp(props.PaneRect.X, props.TabRect.X),
		Y: lerp(props.PaneRect.Y, props.TabRect.Y),
		W: lerp(props.PaneRect.W, props.TabRect.W),
		H: lerp(props.PaneRect.H, props.TabRect.H),
	}
	animation := ui.Animation{Spring: Spring(t.C.ReorderMotion.Cancel)}
	if props.Tab {
		animation = ui.Animation{Spring: Spring(t.C.ReorderMotion.PaneDrop)}
	}
	geometry := func(key string, value float32) float32 {
		if !props.Settling || reduced {
			return ghost.Jump(key, value)
		}
		return ghost.AnimateTo(key, value, animation)
	}
	ghost.Left(geometry(dragMorphX, rect.X)).Top(geometry(dragMorphY, rect.Y)).
		Size(geometry(dragMorphW, rect.W), geometry(dragMorphH, rect.H)).
		Radius(lerp(t.C.DragGhost.Radius, t.C.Tab.Radius))
	visible := Visual(ghost, dragMorphAlpha, target, t.C.DragLift.LiftDuration)
	return ghost.Children(func() {
		ui.Box(c).Key("pane").Absolute().Left(0).Top(0).Size(props.PaneRect.W, props.PaneRect.H).
			Opacity(1 - visible).Clip().PassThrough().Children(pane)
		ui.Box(c).Key("tab").Absolute().Left(0).Top(0).Right(0).Height(t.C.Tab.Height).
			Opacity(visible).Clip().PassThrough().Children(tab)
	})
}

// DragMorphSettled waits for the shape, placement, and content crossfade.
func DragMorphSettled(ghost ui.Element) bool {
	shape := ghost.Settled(dragMorphShape)
	position := ghost.Settled(dragMorphX) && ghost.Settled(dragMorphY)
	size := ghost.Settled(dragMorphW) && ghost.Settled(dragMorphH)
	return shape && position && size && ghost.Settled(dragMorphAlpha)
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

type DragLiftProps struct {
	Scale, Rotation, Shadow float32
	Animation               ui.Animation
	Motion                  MotionMode
}

func StyleDragLift(c *ui.Context, ghost ui.Element, props DragLiftProps) {
	p := theme.Current().C.DragLift
	pose := ui.Pose{Scale: props.Scale, Rotate: props.Rotation}
	if MotionReduced(c, props.Motion) {
		pose.Scale, pose.Rotate = 1, 0
	}
	ghost.AnimatePose(pose, props.Animation)
	alpha := Visual(ghost, "shadow", props.Shadow, p.LiftDuration)
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

type DragGroupLabelProps struct {
	Dragging, Expanded bool
}

func DragGroupLabel(c *ui.Context, title string, count int, chevron *ui.SVG, props DragGroupLabelProps) {
	t := theme.Current()
	p := t.C.GroupHeader
	ink, weight := Color(p.Text), p.Weight
	if props.Dragging {
		ink, weight = Color(t.C.SiderItem.NameColor), t.Font.Weight.Semibold
	}
	rotation := float32(-90)
	if !props.Dragging && props.Expanded {
		rotation = 0
	}
	// The return preview uses the live header's expansion and count instead
	// of keeping a collapsed stack until its position animation ends.
	ui.Icon(c, chevron).Size(t.Font.Size.Caption, t.Font.Size.Caption).TextColor(ink).Rotate(rotation).PassThrough()
	ui.Text(c, title).FontSize(p.FontSize).FontWeight(weight).TextColor(ink).PassThrough()
	ui.Spacer(c).PassThrough()
	if props.Dragging {
		ui.Text(c, strconv.Itoa(count)+locales.MessageHosts2()).FontSize(p.CountSize).TextColor(Color(p.Count)).PassThrough()
	} else {
		Count(c, count, CountProps{Style: CountQuiet})
	}
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
			TextColor(Color(t.C.DropIndicator.HintText)).FontSize(t.Font.Size.Footnote).PassThrough()
	})
}
