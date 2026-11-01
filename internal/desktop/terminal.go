package desktop

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/plugins/glass"
	nativeterm "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

func (a *app) terminals(c *ui.Context, size ui.Point) {
	t := theme.Current()
	area := ui.Box(c).Key("terminals").Grow(1).MinWidth(0).MinHeight(0).Bind(&a.workArea)
	tab := a.currentTab()
	if tab == nil {
		_, over := ui.DragOver[hostDrag](area)
		source, dropped := ui.Drop[hostDrag](area)
		catalogSource, catalogOver, catalogDropped := a.catalogHostDropTarget(area)
		if catalogOver {
			source, over, dropped = catalogSource, true, catalogDropped
			area.Cursor(ui.CursorGrabbing)
		}
		if over {
			area.DrawOver(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, tokenColor(t.C.DropIndicator.ZoneBg), t.C.DropIndicator.ZoneRadius)
			})
		}
		if dropped {
			a.pendingDock = func() {
				a.openHost(source.id)
				startPaneReveal(a.currentTab().focused)
				if catalogDropped {
					a.page().catalogDrag.externalDrop = true
				}
				if a.drag != nil {
					a.drag.session, a.drag.phase = a.currentTab().focused, dragLanded
				}
			}
		}
		area.Children(func() {
			workspaceCard(c).Fill().MinWidth(0).Center().Gap(t.Space.N12).Children(func() {
				ui.Icon(c, terminalIcon).Size(t.C.Pane.EmptyIconSize, t.C.Pane.EmptyIconSize).TextColor(tokenColor(t.Color.Text.Secondary))
				ui.Text(c, "选择一台主机，开始会话").FontSize(t.C.Pane.EmptyTitleSize).FontWeight(t.C.Pane.EmptyTitleWeight)
				ui.Text(c, "左侧主机可拖入终端窗格").FontSize(t.Font.Size.Body).TextColor(tokenColor(t.Color.Text.Secondary))
				button(c, "添加 SSH 主机").OnClick(func() { a.startHostDraft() })
			})
		})
		return
	}
	minimum := tab.root.minimum()
	area.MinWidth(minimum.X).MinHeight(minimum.Y)
	size.X, size.Y = max(size.X, minimum.X), max(size.Y, minimum.Y)
	if tab.workspaceSize != size {
		// MyGo can rebuild several times at the same frame time after input.
		tab.workspaceSize, tab.workspaceResizedAt = size, c.Now()
	}
	bounds := ui.Rect{W: size.X, H: size.Y}
	panes, splits := []panePlacement{}, []splitPlacement{}
	a.terminalCellWidth()
	tab.root.placements(bounds, &panes, &splits)
	a.zoomProgress(c, tab)
	targets := make([]ui.Element, len(panes))
	// Text selection can change focus during a pane build. Keep this frame's
	// sibling order stable so that each session is built exactly once.
	focusedSession := tab.focused
	area.Children(func() {
		// Stable siblings let layout transitions follow sessions across arbitrary tree edits.
		for i, placement := range panes {
			placement.session.layoutRect = placement.rect
			if placement.session != focusedSession {
				targets[i] = a.terminalPane(c, tab, placement)
			}
		}
		for i, placement := range panes {
			if placement.session != focusedSession {
				continue
			}
			if tab.maximized {
				placement.rect = ui.Rect{W: bounds.W, H: bounds.H}
			}
			targets[i] = a.terminalPane(c, tab, placement)
		}
		if !tab.maximized && tab.zoomMix == 0 {
			for _, split := range splits {
				a.terminalDivider(c, split)
			}
		}
		// Register after every source has been built, including a source rendered last for focus.
		_, catalogActive, _ := a.catalogHostTransfer()
		if a.drag != nil && a.drag.phase == dragActive || catalogActive {
			for i, placement := range panes {
				if tab.maximized && placement.session != tab.focused {
					continue
				}
				targets[i].Children(func() {
					// Keep the scroll container out of the drag hit chain: MyGo auto-scrolls its ancestors.
					surface := ui.Box(c).Key("pane-drop-surface").Absolute().Top(0).Right(0).Bottom(0).Left(0)
					a.paneDrop(
						c,
						surface,
						placement.session,
						false,
					)
				})
			}
		}
	})
	if preview := a.dockPreview; preview != nil {
		area.DrawOver(func(p *ui.Painter, bounds ui.Rect) {
			r := *preview
			r.X, r.Y = r.X+bounds.X, r.Y+bounds.Y
			p.Fill(r, tokenColor(t.C.DropIndicator.ZoneBg), t.C.DropIndicator.ZoneRadius)
			p.Stroke(r, tokenColor(t.C.DropIndicator.ZoneBorder), t.C.DropIndicator.ZoneRadius, t.C.DropIndicator.ZoneBorderWidth)
		})
	}
}

func (a *app) terminalCellWidth() float32 {
	prefs := a.terminal.prefs
	font := ui.Font{Family: strings.Join(prefs.FontFamily, ", "), Size: prefs.FontSize}
	if a.cellWidth == 0 || a.cellFont != font || a.cellTracking != prefs.Tracking {
		a.cellFont = font
		a.cellTracking = prefs.Tracking
		a.cellWidth, _ = ui.ShapeText("M", font, 0).Size()
		a.cellWidth += prefs.Tracking
	}
	return a.cellWidth
}

func (a *app) terminalPane(c *ui.Context, tab *workspaceTab, placement panePlacement) ui.Element {
	t := theme.Current()
	s := termtheme.Current()
	prefs := a.terminal.prefs
	terminalBackground := termtheme.TerminalBackground(s, t.C.Pane.Bg, prefs.Background)
	background := tokenColor(terminalBackground)
	surface := terminalSurface(terminalBackground)
	p, f, status := surface.C.Pane, surface.C.PaneFocus, surface.C.StatusBar
	session, rect := placement.session, placement.rect
	h := session.endpoint
	count := tab.root.count()
	multiple := count > 1
	contentGap := t.Space.N4
	topPadding := contentGap
	bottomPadding := status.HeightPane + contentGap
	if multiple {
		topPadding += p.HeaderHeight
	}
	// Only the inner card clips. Its wrapper owns the focus border and overlay hairline.
	pane := ui.Box(c).Key(fmt.Sprintf("pane-wrap-%d", session.id)).Absolute().
		Left(rect.X).Top(rect.Y).Size(rect.W, rect.H).Bind(&session.frame).
		MinWidth(0).MinHeight(0).Radius(p.Radius).Label(h.name + " SSH 终端")
	a.paneMotion(c, pane, tab, session)
	_, catalogActive, _ := a.catalogHostTransfer()
	frozen := !session.revealing && (a.drag != nil || catalogActive || a.resizing != nil ||
		tab.workspaceResizedAt.Equal(c.Now()) || pane.Transitioning() || c.Now().Before(tab.layoutUntil))
	if session.contentWidth == 0 || !frozen {
		if session.contentWidth != rect.W || session.contentHeight != rect.H {
			if session.scroll.offset.Y >= session.scroll.offset.MaxY {
				session.scroll.offset.Y = math.MaxFloat32
			}
			session.contentWidth, session.contentHeight = rect.W, rect.H
		}
		if session.terminal == nil {
			session.columns = max(1, int((rect.W-2*p.ContentSide)/a.cellWidth))
			session.rows = max(1, int((rect.H-topPadding-bottomPadding)/prefs.LineHeight))
		}
	} else {
		c.After(p.FrameInterval)
	}
	pane.OnClick(func() { a.focusSession(tab, session) })
	scroll := &session.scroll
	focused, hovered := tab.focused == session, pane.Hovered()
	opacity, saturation, footerOpacity := float32(1), float32(1), float32(1)
	titleColor, loginColor := tokenColor(f.NameFocused), tokenColor(p.Host)
	titleWeight := f.NameWeightFocused
	if multiple && !focused {
		opacity, saturation, footerOpacity = f.ContentOpacityUnfocused, f.ContentSaturateUnfocused, f.StatusOpacityUnfocused
		titleColor, loginColor, titleWeight = tokenColor(f.NameUnfocused), tokenColor(f.HostUnfocused), f.NameWeightUnfocused
		if hovered {
			opacity, saturation = f.ContentOpacityHover, f.ContentSaturateHover
			titleColor = tokenColor(f.NameFocused)
		}
	}
	opacity = animateVisual(pane, "output-opacity", opacity, f.Transition)
	saturation = animateVisual(pane, "output-saturation", saturation, f.Transition)
	footerOpacity = animateVisual(pane, "footer-opacity", footerOpacity, f.Transition)
	if !multiple {
		opacity, saturation, footerOpacity = 1, 1, 1
	}
	titleColor = pane.AnimateColor("host-color", titleColor, tokenTween(f.Transition))
	loginColor = pane.AnimateColor("login-color", loginColor, tokenTween(f.Transition))
	var card, hairline ui.Element
	pane.Children(func() {
		card = ui.Column(c).Key("card").Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Radius(p.Radius).Background(background).Clip().
			Draw(func(p *ui.Painter, r ui.Rect) {
				// Absolute children paint after the scroll content, so paint its backdrop here.
				p.Fill(r, background, t.C.Pane.Radius)
			})
		card.Children(func() {
			if live := session.terminal; live != nil {
				a.syncSSHTerminal(live)
				ui.Box(c).Key("live-output").Absolute().Left(0).Top(topPadding).
					Width(session.contentWidth).Height(max(1, session.contentHeight-topPadding-bottomPadding)).Children(func() {
					output := nativeterm.View(c, live.view).Fill().Bind(&session.focus).
						Label(h.name + " SSH 终端").Opacity(opacity)
					if session.focusPending {
						if a.sshSessionFocused(session) {
							output.Focus()
						}
						session.focusPending = false
					}
					if saturation != 1 {
						output.Effect(terminalSaturation.Effect(shader.Options{
							Uniforms: map[string]any{"Saturation": saturation},
						}))
					}
					if output.Focused() && !tab.focused.focusPending && tab.focused != session {
						tab.focused = session
						session.hasNewOutput, session.hasBell = false, false
						c.AnimationFrame()
					}
				})
			} else {
				// Move the native thumb outside the card's clip; the fading thumb is drawn below.
				output := ui.Scroll(c.Key("output")).Fill().MinWidth(0).MinHeight(0).
					Bind(&session.focus).Focusable().FocusRing(false).
					TrackScroll(&scroll.offset).Padding(
					topPadding,
					p.ContentSide,
					bottomPadding,
					p.ContentSide,
				).ScrollbarInsets(
					p.HeaderHeight,
					-t.C.Scrollbar.TrackWidth,
					status.HeightPane,
					0,
				).Font(strings.Join(prefs.FontFamily, ", ")).FontSize(prefs.FontSize).
					LetterSpacing(prefs.Tracking).TextColor(tokenColor(s.Foreground)).
					Opacity(opacity).Cache().HandleInput(scroll.input).
					OnClick(func() { a.focusSession(tab, session) })
				if session.focusPending && tab.focused == session {
					output.Focus()
					session.focusPending = false
				}
				if saturation != 1 {
					output.Effect(terminalSaturation.Effect(shader.Options{
						Uniforms: map[string]any{"Saturation": saturation},
					}))
				}
				output.Children(func() {
					a.terminalLines(c, session, focused)
				})
				if multiple {
					terminalScrollEdge(c, p.HeaderHeight, false, background)
				}
				terminalScrollEdge(c, status.HeightPane, true, background)
				terminalScrollbar(c, pane, scroll, terminalBackground)
			}
			var header ui.Element
			if multiple {
				header = ui.Row(c).Key("identity").Absolute().Top(0).Left(0).Height(p.HeaderHeight).Padding(
					0,
					0,
					0,
					p.HeaderPaddingLeft,
				).Gap(p.HeaderGap).PassThrough()
				header.Children(func() {
					labelColor, labelBackground := tokenColor(f.KbdText), tokenColor(f.KbdBg)
					if focused {
						labelColor, labelBackground = tokenColor(f.KbdTextFocused), tokenColor(f.KbdBgFocused)
					}
					number := slices.Index(tab.root.sessions(), session) + 1
					ui.Text(c, fmt.Sprintf("⌘%d", number)).FontSize(f.KbdSize).FontWeight(f.KbdWeight).
						FixedLineHeight(f.KbdLineHeight).SingleLine().Padding(f.KbdPaddingY, f.KbdPaddingX).Radius(f.KbdRadius).
						Background(labelBackground).TextColor(labelColor)
					tooltip(c, ui.Text(c, h.name).FontSize(p.HeaderFontSize).FontWeight(titleWeight).TextColor(titleColor).
						SingleLine().Ellipsis("…").MinWidth(0).Shrink(1), fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port))
					ui.Text(c, fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)).
						Font(strings.Join(p.HostFamily, ", ")).FontSize(p.HostSize).TextColor(loginColor).
						SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					if !focused {
						terminalActivity(c, session, surface)
					}
					handle := ui.Box(c).Key("pane-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(0).
						Label("拖动窗格 " + h.name).OnClick(func() { a.focusSession(tab, session) })
					a.paneDragHandle(c, handle, paneTransfer{tab: tab, session: session})
				})
			}
			actionsWidth := p.ActionWidth
			if multiple {
				actionsWidth = 3*p.ActionWidth + 2*p.ActionGap
			}
			actions := ui.Box(c).Key("pane-actions").Absolute().Top(p.ActionTop).Right(p.ActionRight).Size(actionsWidth, p.ActionHeight)
			x, y, over := pane.PointerPosition()
			inActionBand := x >= pane.Bounds().W-actionsWidth-p.ActionRevealPadding && y <= p.HeaderHeight+p.ActionRight
			nearActions := over && inActionBand
			var keyboardFocus bool
			bounds := pane.Bounds()
			stacked := count > 1 && bounds.H > bounds.W
			actions.Children(func() {
				if !multiple {
					ui.Box(c).Absolute().Top(0).Left(0).Size(actionsWidth, p.ActionHeight).Radius(p.ActionRadius).PassThrough().
						Material(glass.Blur{Radius: p.ActionBlur})
					ui.Box(c).Absolute().Top(0).Left(0).Size(actionsWidth, p.ActionHeight).Radius(p.ActionRadius).PassThrough().
						Background(background.Alpha(p.ActionTint))
				}
				split := terminalIconButton(c, "分栏 "+h.name, sidebarIcon, surface).Absolute().Top(0).Left(0).Size(p.ActionWidth, p.ActionHeight).
					Disabled(count >= maxTerminalPanes || session.closing || !canSplit(session.layoutRect, dropRight) && !canSplit(session.layoutRect, dropBottom))
				keyboardFocus = split.FocusVisible()
				split.OnClick(func() { a.splitSession(session, stacked) })
				if multiple {
					expand := terminalIconButton(c, "放大或恢复 "+h.name, expandIcon, surface).Absolute().Top(0).Left(p.ActionWidth+p.ActionGap).Size(p.ActionWidth, p.ActionHeight)
					keyboardFocus = keyboardFocus || expand.FocusVisible()
					expand.OnClick(func() {
						a.togglePaneZoom(tab, session)
					})
					close := terminalIconButton(c, "关闭窗格 "+h.name, closeIcon, surface).Absolute().Top(0).Left(2*(p.ActionWidth+p.ActionGap)).Size(p.ActionWidth, p.ActionHeight)
					keyboardFocus = keyboardFocus || close.FocusVisible()
					close.OnClick(func() { a.closePane(session) })
				}
			})
			target := f.ButtonsOpacityUnfocused
			showForPane := multiple && (focused || hovered)
			if showForPane || nearActions || keyboardFocus {
				target = 1
			}
			actionVisibility := animateVisual(actions, "visibility", target, f.Transition)
			actions.Opacity(actionVisibility)
			if multiple {
				header.Right(p.HeaderPaddingRight + (actionsWidth+t.Space.N10)*actionVisibility)
			}
			ui.Row(c).Key("status").Absolute().Bottom(0).Left(0).Right(0).Height(status.HeightPane).
				Opacity(footerOpacity).
				Padding(0, status.PaddingX).Gap(status.GapPane).PassThrough().TextColor(tokenColor(status.Text)).Children(func() {
				badge := surface.C.Badge
				label, tint := "已连接", tokenColor(badge.OkBg)
				color := tokenColor(badge.OkText)
				switch {
				case session.state == connecting:
					label, tint, color = "连接中", tokenColor(surface.Color.Warning.Subtle), tokenColor(surface.Color.Warning.Text)
				case session.state == connectionFailed:
					label, tint, color = "连接失败", tokenColor(surface.Color.Danger.Subtle), tokenColor(surface.Color.Danger.Text)
				case session.state != connected:
					label, tint, color = "未连接", tokenColor(badge.NeutralBg), tokenColor(badge.NeutralText)
				}
				ui.Row(c).Padding(badge.PaddingY, badge.PaddingXCompact).Gap(badge.Gap).Radius(badge.Radius).Background(tint).Children(func() {
					ui.Box(c).Size(t.Size.DotSm, t.Size.DotSm).Radius(badge.Radius).Background(color)
					ui.Text(c, label).FontSize(badge.FontSizeCompact).TextColor(color)
				})
				latency := "延迟 —"
				ui.Text(c, latency).FontSize(status.FontSize).SingleLine()
				ui.Spacer(c)
				ui.Text(c, a.paneSizeLabel(session)).FontSize(status.FontSize).SingleLine()
			})
		})
		hairline = ui.Box(c).Key("hairline").Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Radius(p.Radius).PassThrough()
		applyShadow(hairline, p.Outline)
	})
	paneReveal(c, card, session)
	if a.lifted(session) {
		card.Opacity(0)
		hairline.Opacity(0)
		a.dragPlaceholder(pane)
	} else {
		paneFocusShadows(pane, focused, multiple, surface)
	}
	a.paneDrop(
		c,
		pane,
		session,
		true,
	)
	return pane
}

func terminalIconButton(c *ui.Context, label string, icon *ui.SVG, surface *theme.Theme) ui.Element {
	button := styleIconButton(ui.ButtonBase(c).Key(label).Label(label), surface)
	tooltip(c, button, label)
	size := surface.C.IconButton.IconSize
	button.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return button
}

func terminalScrollEdge(c *ui.Context, barHeight float32, bottom bool, background ui.Color) {
	t := theme.Current()
	fade := t.C.FadeBar
	angle := float32(180)
	if bottom {
		angle = 0
	}
	band := func() ui.Element {
		element := ui.Box(c).Absolute().Left(0).Right(0).Height(barHeight).PassThrough()
		if bottom {
			return element.Bottom(0)
		}
		return element.Top(0)
	}
	// Each layer progressively masks a stronger blur before tinting with the pane surface.
	layers := [...]struct{ radius, solid, end float32 }{
		{radius: fade.Blur1, solid: fade.Mask1Solid, end: fade.Mask1End},
		{radius: fade.Blur2, solid: fade.Mask2Solid, end: fade.Mask2End},
		{radius: fade.Blur3, solid: fade.Mask3Solid, end: fade.Mask3End},
	}
	for _, layer := range layers {
		band().Material(glass.Blur{
			Radius: layer.radius,
			Mask: &ui.LinearGradient{
				From: tokenColor(fade.Tint), To: ui.Transparent,
				Angle: angle, Start: layer.solid, End: layer.end,
			},
		})
	}
	band().LinearGradient(ui.LinearGradient{
		Angle: angle,
		Stops: []ui.GradientStop{
			{Color: background.Alpha(fade.TintStart), At: 0},
			{Color: background.Alpha(fade.TintMid), At: fade.TintMidStop},
			{Color: background.Alpha(fade.TintEnd), At: 1},
		},
	})
}
