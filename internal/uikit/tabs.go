package uikit

import (
	"math"
	"strconv"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

var sessionTabsFade = shader.MustCompile(`package main

var LeftExtent float
var RightExtent float

func ColorEffect(pos vec2, color vec4) vec4 {
	right := rectSize().x
	opacity := float(1)
	if RightExtent > 0 {
		opacity *= clamp((right-pos.x)/RightExtent, 0, 1)
	}
	if LeftExtent > 0 {
		opacity *= clamp(pos.x/LeftExtent, 0, 1)
	}
	return color * opacity
}
`)

type ConnState uint8

const (
	ConnDisconnected ConnState = iota
	ConnConnecting
	ConnConnected
	ConnError
)

func (state ConnState) Label() string {
	switch state {
	case ConnConnected:
		return locales.MessageConnected()
	case ConnConnecting:
		return locales.MessageConnecting2()
	case ConnError:
		return locales.MessageConnectionFailed()
	}
	return locales.MessageDisconnected()
}

type SessionTabVariant uint8

const (
	SessionTabTerminal SessionTabVariant = iota
	SessionTabFiles
)

type SessionTabProps struct {
	Key                   any
	Title, Hint           string
	Selected              bool
	Variant               SessionTabVariant
	State                 ConnState
	ReduceMotion          bool
	PositionAnimation     ui.Animation
	Handle                *ui.Handle
	Extra                 int
	CloseLabel, DragLabel string
	CloseIcon             *ui.SVG
}

func SessionTab(c *ui.Context, props SessionTabProps) ui.Element {
	t := theme.Current()
	p := t.C.Tab
	button := ui.ButtonBase(c).Key(props.Key).Height(p.Height).
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius).FocusRing(false)
	if props.Handle != nil {
		button.Bind(props.Handle)
	}
	if props.Variant == SessionTabTerminal {
		button.Transition(ui.ElementTransition{Position: true, Animation: &props.PositionAnimation})
	}
	button.AlignItems(ui.Center).Role(ui.RoleTab).Label(props.Title).Selected(props.Selected)
	selected := ui.Local(button, "uikit.tabs.selected", func() bool { return false })
	button.Children(func() {
		// Keep the selected tab clear of the scroller's edge fade.
		reveal := ui.Box(c).Key("tab-reveal").Absolute().Left(0).Top(0).Bottom(0).
			Right(-t.Size.FadeExtent).PassThrough().Role(ui.RoleNone)
		if props.Selected && !*selected {
			// Reveal once; subsequent frames must allow manual scrolling.
			reveal.ScrollIntoView()
		}
	})
	*selected = props.Selected
	foreground := sessionTabForeground(props)
	animation := Tween(p.StateTransition)
	hint := props.Hint
	if hint != "" {
		hint += " · "
	}
	hint += props.State.Label()
	if props.Variant == SessionTabFiles {
		Tip(c, button, hint)
		button.TextColor(button.AnimateColor("text", foreground, animation))
	}
	background := ui.Transparent
	if props.Selected {
		background = Color(p.BgSelected)
		Shadow(button, p.ShadowSelected)
	} else if button.Hovered() {
		background = Color(p.BgHover)
	}
	button.Background(button.AnimateColor("background", background, animation))
	if button.FocusVisible() {
		Shadow(button, t.Shadow.ControlFocus)
	}
	if props.Variant == SessionTabTerminal {
		button.TextColor(button.AnimateColor("text", foreground, animation))
		Tip(c, button, hint)
	}
	return button
}

type SessionTabParts struct {
	Close, DragHandle ui.Element
}

// SessionTabWidth measures the same label and close slot used by live tabs.
func SessionTabWidth(props SessionTabProps) float32 {
	t := theme.Current()
	p := t.C.Tab
	font := ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: p.FontSize, Weight: p.WeightSelected}
	titleWidth, _ := ui.ShapeText(props.Title, font, 0).Size()
	width := p.PaddingLeft + sessionTabLeadingWidth(props) + titleWidth + t.Size.Icon + 2*p.Gap + p.PaddingRight
	if props.Extra > 0 {
		font.Size, font.Weight, font.Features = t.Font.Size.Caption, t.Font.Weight.Regular, "tnum"
		countWidth, _ := ui.ShapeText("+"+strconv.Itoa(props.Extra), font, 0).Size()
		width += p.Gap + countWidth
	}
	return width
}

// SessionTabPreview draws a selected tab without input, focus, or tooltip ownership.
func SessionTabPreview(c *ui.Context, props SessionTabProps) ui.Element {
	t := theme.Current()
	p := t.C.Tab
	props.Selected = true
	preview := ui.Row(c).Key(props.Key).Height(p.Height).Shrink(0).
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius).
		Center().Background(Color(p.BgSelected)).TextColor(sessionTabForeground(props)).Clip().PassThrough()
	Shadow(preview, p.ShadowSelected)
	return preview.Children(func() {
		sessionTabLabel(c, props)
		ui.Box(c).Key("close-tab").Size(t.Size.Icon, p.ContentHeight).Shrink(0).Center().PassThrough().Children(func() {
			ui.Icon(c, props.CloseIcon).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize).
				TextColor(Color(t.C.IconButton.Icon)).PassThrough()
		})
	})
}

func sessionTabForeground(props SessionTabProps) ui.Color {
	p := theme.Current().C.Tab
	foreground := Color(p.Text)
	if props.Selected {
		foreground = Color(p.TextSelected)
	}
	if props.State == ConnDisconnected {
		foreground = Color(p.TextDisconnected)
	} else if props.Variant == SessionTabFiles && props.State == ConnError && props.Selected {
		foreground = Color(p.TextErrorSelected)
	}
	return foreground
}

func sessionTabLeadingWidth(props SessionTabProps) float32 {
	t := theme.Current()
	if props.Variant == SessionTabTerminal && props.Extra > 0 {
		return t.Size.IconXs
	}
	return t.C.Tab.SpinnerSize
}

func sessionTabLabel(c *ui.Context, props SessionTabProps) {
	p := theme.Current().C.Tab
	ui.Box(c).Width(sessionTabLeadingWidth(props)).Height(p.ContentHeight).Shrink(0).Center().PassThrough().Children(func() {
		if props.Variant == SessionTabTerminal && props.Extra > 0 {
			Icon(c, IconGrid, IconProps{Size: SizeXs, Tone: ToneSecondary}).PassThrough()
			return
		}
		StatusDot(c.Key("connection-indicator"), props.State, StatusDotProps{
			Motion: MotionOverride(props.ReduceMotion),
		}).PassThrough()
	})
	ui.Text(c, props.Title).FontSize(p.FontSize).FontWeight(p.WeightSelected).
		FixedLineHeight(p.ContentHeight).Top(p.TitleOffset).SingleLine().PassThrough()
	if props.Extra > 0 {
		Count(c, props.Extra, CountProps{Style: CountInline}).FixedLineHeight(p.ContentHeight).Top(p.CountOffset)
	}
}

// SessionTabContents leaves close, activation, menus and drag ownership with the caller.
func SessionTabContents(c *ui.Context, root ui.Element, props SessionTabProps) SessionTabParts {
	t := theme.Current()
	p := t.C.Tab
	sessionTabLabel(c, props)
	parts := SessionTabParts{Close: IconButton(c, props.CloseIcon, IconButtonProps{Key: "close-tab", Label: props.CloseLabel}).Size(t.Size.Icon, p.ContentHeight)}
	visible := float32(0)
	if props.Selected || root.Hovered() || parts.Close.FocusVisible() {
		visible = 1
	}
	parts.Close.Opacity(Visual(parts.Close, "visible", visible, p.StateTransition))
	if props.Variant == SessionTabTerminal {
		parts.DragHandle = ui.Box(c).Key("tab-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(p.DragHandleInset).Label(props.DragLabel)
	}
	return parts
}

type TabInputResult struct {
	Select int
	Close  bool
}

type tabMiddleInput struct {
	Up      ui.Point
	Target  uint64
	Pending bool
	Hits    []tabHit
}

type tabHit struct {
	ID       uint64
	Rect     ui.Rect
	Selected bool
}

// SessionTabInput keeps keyboard and middle-click events independent from
// the caller's left-button drag source and workspace lifecycle.
func SessionTabInput(c *ui.Context, list, tab ui.Element, index, count int) TabInputResult {
	follow := ui.Local(list, "uikit.tabs.follow", func() int { return -1 })
	if *follow == index {
		tab.Focus().ScrollIntoView()
		*follow = -1
	}
	middle := ui.Local(list, "uikit.tabs.middle", func() tabMiddleInput { return tabMiddleInput{} })
	bounds := tab.Bounds()
	inside := func(point ui.Point) bool {
		return point.X >= bounds.X && point.Y >= bounds.Y && point.X < bounds.X+bounds.W && point.Y < bounds.Y+bounds.H
	}
	result := TabInputResult{Select: -1, Close: middle.Pending && middle.Target == tab.ID() && inside(middle.Up)}
	slot := len(middle.Hits)
	selected := ui.Local(tab, "uikit.tabs.selected", func() bool { return false })
	middle.Hits = append(middle.Hits, tabHit{ID: tab.ID(), Selected: *selected})
	// Paint follows commit, so this records the current transformed hit box;
	// clipped-out tabs keep an empty rectangle and cannot catch a middle press.
	tab.DrawOver(func(_ *ui.Painter, _ ui.Rect) {
		if tab.Valid() {
			middle.Hits[slot].Rect = tab.Bounds()
		}
	})
	if result.Close {
		middle.Pending = false
	}
	if count == 0 {
		return result
	}
	// These scoped registrations also cover the close button's focus. The SDK
	// checks them before its FocusGroup moves focus, so selection follows once.
	switch {
	case tab.Shortcut(0, ui.KeyLeft):
		result.Select = (index - 1 + count) % count
	case tab.Shortcut(0, ui.KeyRight):
		result.Select = (index + 1) % count
	case tab.Shortcut(0, ui.KeyHome):
		result.Select = 0
	case tab.Shortcut(0, ui.KeyEnd):
		result.Select = count - 1
	}
	if result.Select >= 0 {
		*follow = result.Select
		c.Invalidate()
	}
	return result
}

type TabItem struct {
	ID          any
	Title, Hint string
	State       ConnState
	Extra       int
}

type TabsResult struct {
	Root, Row, Scroller ui.Element
	Close               int
	Select              int
	Add                 bool
	Moved               [2]int
}

type tabReorder struct{ ID any }

type SessionTabsProps struct {
	Scroll     *ui.ScrollState
	ScrollKey  any
	Motion     MotionMode
	StripInput func(ui.InputEvent) bool
	// BuildTabs preserves the caller's tab sources. BuildAdd places the action
	// beside the scroller so it stays visible without following scroll offsets.
	BuildTabs  func(list ui.Element)
	BuildAdd   func()
	AfterBuild func(strip ui.Element)
}

// SessionTabs owns the shared viewport, scroller and keyboard group. BuildTabs
// adapts an existing docking controller without changing its source hierarchy.
func SessionTabs(c *ui.Context, items []TabItem, active *int, options ...SessionTabsProps) TabsResult {
	t := theme.Current()
	props := SessionTabsProps{}
	if len(options) != 0 {
		props = options[0]
	}
	result := TabsResult{
		Root:  SessionTabsViewport(c).Row().Gap(t.C.Titlebar.TabsGap),
		Close: -1, Select: -1, Moved: [2]int{-1, -1},
	}
	scroll := props.Scroll
	if scroll == nil {
		scroll = ui.Local(result.Root, "uikit.tabs.scroll", func() ui.ScrollState { return ui.ScrollState{} })
	}
	result.Root.Children(func() {
		if props.ScrollKey != nil {
			c = c.Key(props.ScrollKey)
		}
		result.Scroller = SessionTabsScroll(c, scroll)
		result.Scroller.Children(func() {
			result.Row = SessionTabsRow(c)
			middle := ui.Local(result.Row, "uikit.tabs.middle", func() tabMiddleInput { return tabMiddleInput{} })
			middle.Hits = middle.Hits[:0]
			result.Row.Children(func() {
				if props.BuildTabs != nil {
					props.BuildTabs(result.Row)
				} else {
					for index, item := range items {
						props := SessionTabProps{Key: item.ID, Title: item.Title, Hint: item.Hint, State: item.State, Extra: item.Extra,
							Selected: index == *active, CloseLabel: locales.MessageCloseTab2() + item.Title, CloseIcon: IconSVG(IconClose),
							ReduceMotion: MotionReduced(c, props.Motion), PositionAnimation: Anim(c, t.C.Tab.StateTransition, props.Motion)}
						tab := SessionTab(c, props)
						input := SessionTabInput(c, result.Row, tab, index, len(items))
						if tab.Clicked() {
							*active = index
						}
						if input.Select >= 0 {
							*active = input.Select
						}
						if input.Close {
							result.Close = index
						}
						tab.Drag(tabReorder{item.ID}).Cursor(ui.CursorDefault)
						if tab.Dragging() {
							tab.Cursor(ui.CursorGrabbing)
						}
						if source, dropped := ui.Drop[tabReorder](tab); dropped && source.ID != item.ID {
							for from, candidate := range items {
								if candidate.ID == source.ID {
									result.Moved = [2]int{from, index}
									break
								}
							}
						}
						tab.Children(func() {
							parts := SessionTabContents(c, tab, props)
							if parts.Close.Clicked() {
								result.Close = index
							}
							parts.DragHandle.PassThrough()
						})
					}
				}
			})
		})
		middle := ui.Local(result.Row, "uikit.tabs.middle", func() tabMiddleInput { return tabMiddleInput{} })
		if count := len(middle.Hits); count != 0 {
			switch {
			case result.Row.Shortcut(0, ui.KeyLeft), result.Row.Shortcut(0, ui.KeyEnd):
				result.Select = count - 1
			case result.Row.Shortcut(0, ui.KeyRight), result.Row.Shortcut(0, ui.KeyHome):
				result.Select = 0
			}
			if result.Select >= 0 {
				follow := ui.Local(result.Row, "uikit.tabs.follow", func() int { return -1 })
				*follow = result.Select
				if props.BuildTabs == nil {
					*active = result.Select
				}
				c.Invalidate()
			}
		}
		middle.Pending = false
		// MyGo delivers to the nearest handler without bubbling false results.
		// Keep middle presses and the existing strip controller in this one handler.
		stripInput := func(event ui.InputEvent) bool {
			if event.Button == 2 && (event.Kind == ui.InputPointerDown || event.Kind == ui.InputPointerUp) {
				bounds := result.Scroller.Bounds()
				point := ui.Point{X: bounds.X + event.X, Y: bounds.Y + event.Y}
				if event.Kind == ui.InputPointerDown {
					middle.Target = 0
					for index := len(middle.Hits) - 1; index >= 0; index-- {
						hit := middle.Hits[index]
						if point.X >= hit.Rect.X && point.Y >= hit.Rect.Y && point.X < hit.Rect.X+hit.Rect.W && point.Y < hit.Rect.Y+hit.Rect.H {
							middle.Target = hit.ID
							break
						}
					}
				} else {
					middle.Up = point
					middle.Pending = event.X >= 0 && event.Y >= 0 && event.X < bounds.W && event.Y < bounds.H
					c.Invalidate()
				}
				return true
			}
			if props.StripInput != nil && props.StripInput(event) {
				return true
			}
			if event.Kind == ui.InputScroll && math.Abs(float64(event.DY)) > math.Abs(float64(event.DX)) {
				scroll.X = min(max(scroll.X+event.DY, 0), scroll.MaxX)
				c.Invalidate()
				return true
			}
			return false
		}
		if props.AfterBuild != nil {
			props.AfterBuild(result.Scroller)
		}
		result.Scroller.HandleInput(stripInput)
		if len(middle.Hits) > 0 {
			sessionTabsActions(c, &result, scroll, props)
		}
	})
	return result
}

func sessionTabsActions(c *ui.Context, parts *TabsResult, scroll *ui.ScrollState, props SessionTabsProps) {
	t := theme.Current()
	maxX, scrollX := scroll.MaxX, scroll.X
	if maxX > 0 {
		parts.Scroller.Effect(sessionTabsFade.Effect(shader.Options{Uniforms: map[string]any{
			"LeftExtent":  min(t.Size.FadeExtent, scrollX),
			"RightExtent": min(t.Size.FadeExtent, maxX-scrollX),
		}}))
	}
	ui.Row(c).Key("tab-add").Size(t.C.IconButton.Size, t.C.IconButton.Size).
		Shrink(0).Center().Children(func() {
		if props.BuildAdd != nil {
			props.BuildAdd()
			return
		}
		add := IconButton(c, IconPlus, IconButtonProps{Key: "add-tab", Label: locales.MessageNewTab()})
		parts.Add = add.Clicked()
	})
	// Layout updates the scroll range after the view built the edge fades.
	ui.Box(c).Key("tab-action-layout").Absolute().Fill().PassThrough().Role(ui.RoleNone).
		DrawOver(func(_ *ui.Painter, _ ui.Rect) {
			if scroll.MaxX != maxX || scroll.X != scrollX {
				c.Invalidate()
			}
		})
}
