package desktop

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

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
			uikit.Card(c).Fill().MinWidth(0).Center().Gap(t.Space.N12).Children(func() {
				ui.Icon(c, terminalIcon).Size(t.C.Pane.EmptyIconSize, t.C.Pane.EmptyIconSize).TextColor(tokenColor(t.Color.Text.Secondary))
				ui.Text(c, "选择一台主机，开始会话").FontSize(t.C.Pane.EmptyTitleSize).FontWeight(t.C.Pane.EmptyTitleWeight)
				ui.Text(c, "左侧主机可拖入终端窗格").FontSize(t.Font.Size.Body).TextColor(tokenColor(t.Color.Text.Secondary))
				uikit.Button(c, "添加 SSH 主机", uikit.ButtonProps{}).OnClick(func() { a.startHostDraft() })
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
	s := a.sessionScheme(placement.session)
	prefs := a.terminal.prefs
	terminalBackground := termtheme.TerminalBackground(s, t.C.Pane.Bg, prefs.Background)
	background := tokenColor(terminalBackground)
	surface := terminalSurface(terminalBackground)
	p, f, status := surface.C.Pane, surface.C.PaneFocus, surface.C.StatusBar
	session, rect := placement.session, placement.rect
	h := a.hosts[session.host]
	count := tab.root.count()
	multiple := count > 1
	contentGap := t.Space.N4
	topPadding := contentGap
	bottomPadding := status.HeightPane + contentGap
	if multiple {
		topPadding += p.HeaderHeight
	}
	// Only the inner card clips. Its wrapper owns the focus border and overlay hairline.
	pane := uikit.Pane(c, fmt.Sprintf("pane-wrap-%d", session.id), h.name+" 演示终端", rect, surface).Bind(&session.frame)
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
		session.columns = max(1, int((rect.W-2*p.ContentSide)/a.cellWidth))
		session.rows = max(1, int((rect.H-topPadding-bottomPadding)/prefs.LineHeight))
	} else {
		c.After(p.FrameInterval)
	}
	pane.OnClick(func() { a.focusSession(tab, session) })
	pane.ContextMenu(func(m *ui.Menu) {
		if m.Item("追加演示输出").Chosen() {
			a.receiveSessionOutput(session, fmt.Sprintf("[%s] INFO 收到新的演示输出\n", c.Now().Format("15:04:05")))
		}
		if m.Item("触发演示响铃").Chosen() {
			a.receiveSessionOutput(session, "\a")
		}
	})
	scroll := &session.scroll
	focused, hovered := tab.focused == session, pane.Hovered()
	appearance := uikit.PaneVisualStyle(pane, focused, hovered, multiple, surface)
	var card, hairline ui.Element
	pane.Children(func() {
		card = uikit.PaneCard(c, surface, background, t.C.Pane.Radius)
		card.Children(func() {
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
				Opacity(appearance.Opacity).Cache().HandleInput(scroll.input).
				OnClick(func() { a.focusSession(tab, session) })
			if appearance.Saturation != 1 {
				output.Effect(terminalSaturation.Effect(shader.Options{
					Uniforms: map[string]any{"Saturation": appearance.Saturation},
				}))
			}
			output.Children(func() {
				a.terminalLines(c, session, focused)
			})
			if multiple {
				a.terminalScrollEdge(c, p.HeaderHeight, false, background)
			}
			a.terminalScrollEdge(c, status.HeightPane, true, background)
			terminalScrollbar(c, pane, scroll, terminalBackground)
			var header ui.Element
			if multiple {
				header = uikit.PaneHeader(c, surface)
				header.Children(func() {
					number := slices.Index(tab.root.sessions(), session) + 1
					uikit.PaneShortcut(c, fmt.Sprintf("⌘%d", number), focused, surface)
					statusDot(c, session.state, t.Size.Dot)
					uikit.Tip(c, uikit.PaneTitle(c, h.name, appearance, surface), fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port))
					uikit.PaneLogin(c, fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port), appearance, surface)
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
			actions := uikit.PaneActions(c, actionsWidth, surface)
			x, y, over := pane.PointerPosition()
			inActionBand := x >= pane.Bounds().W-actionsWidth-p.ActionRevealPadding && y <= p.HeaderHeight+p.ActionRight
			nearActions := over && inActionBand
			var keyboardFocus bool
			bounds := pane.Bounds()
			stacked := count > 1 && bounds.H > bounds.W
			actions.Children(func() {
				if !multiple {
					uikit.PaneActionBackdrop(c, actionsWidth, a.glassRadius(c, p.ActionBlur), background, surface)
				}
				split := uikit.PaneIconButton(c, "分栏 "+h.name, sidebarIcon, surface).Absolute().Top(0).Left(0).Size(p.ActionWidth, p.ActionHeight).
					Disabled(count >= maxTerminalPanes || session.closing || !canSplit(session.layoutRect, dropRight) && !canSplit(session.layoutRect, dropBottom))
				keyboardFocus = split.FocusVisible()
				split.OnClick(func() { a.splitSession(session, stacked) })
				if multiple {
					expand := uikit.PaneIconButton(c, "放大或恢复 "+h.name, expandIcon, surface).Absolute().Top(0).Left(p.ActionWidth+p.ActionGap).Size(p.ActionWidth, p.ActionHeight)
					keyboardFocus = keyboardFocus || expand.FocusVisible()
					expand.OnClick(func() {
						a.togglePaneZoom(tab, session)
					})
					close := uikit.PaneIconButton(c, "关闭窗格 "+h.name, closeIcon, surface).Absolute().Top(0).Left(2*(p.ActionWidth+p.ActionGap)).Size(p.ActionWidth, p.ActionHeight)
					keyboardFocus = keyboardFocus || close.FocusVisible()
					close.OnClick(func() { a.closePane(session) })
				}
			})
			target := f.ButtonsOpacityUnfocused
			showForPane := multiple && (focused || hovered)
			if showForPane || nearActions || keyboardFocus {
				target = 1
			}
			actionVisibility := uikit.StylePaneActions(actions, target, surface)
			if multiple {
				header.Right(p.HeaderPaddingRight + (actionsWidth+t.Space.N10)*actionVisibility)
			}
			uikit.PaneFooter(c, appearance.FooterOpacity, surface).Children(func() {
				badge := surface.C.Badge
				label, tint := "已连接", tokenColor(badge.OkBg)
				color := tokenColor(badge.OkText)
				switch {
				case session.state == connecting:
					label, tint, color = "连接中", tokenColor(surface.Color.Warning.Subtle), tokenColor(surface.Color.Warning.Text)
				case session.state != connected:
					label, tint, color = "未连接", tokenColor(badge.NeutralBg), tokenColor(badge.NeutralText)
				}
				ui.Row(c).Padding(badge.PaddingY, badge.PaddingXCompact).Gap(badge.Gap).Radius(badge.Radius).Background(tint).Children(func() {
					ui.Box(c).Size(t.Size.DotSm, t.Size.DotSm).Radius(badge.Radius).Background(color)
					ui.Text(c, label).FontSize(badge.FontSizeCompact).TextColor(color)
				})
				latency := "延迟 —"
				if session.state == connected {
					latency = fmt.Sprintf("延迟 %d ms", h.latency)
				}
				ui.Text(c, latency).FontSize(status.FontSize).SingleLine()
				ui.Spacer(c)
				ui.Text(c, a.paneSizeLabel(session)).FontSize(status.FontSize).SingleLine()
			})
		})
		hairline = uikit.PaneHairline(c, surface)
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

func (a *app) terminalScrollEdge(c *ui.Context, barHeight float32, bottom bool, background ui.Color) {
	uikit.ScrollFade(c, uikit.ScrollFadeProps{
		Height: barHeight, Bottom: bottom, Background: background, MaskTint: uikit.Color(theme.Current().C.FadeBar.Tint), Translucent: a.translucencyEnabled(c),
	})
}
