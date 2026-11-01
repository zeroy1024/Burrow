package desktop

import (
	"fmt"
	"math"
	"slices"
	"strings"

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
				uikit.DrawDropIndicator(p, r, uikit.DropIndicatorProps{Background: uikit.Color(t.C.DropIndicator.ZoneBg), Radius: t.C.DropIndicator.ZoneRadius})
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
			parts := uikit.EmptyState(c, uikit.EmptyProps{Kind: uikit.EmptyTerminal, Icon: terminalIcon,
				Title: "选择一台主机，开始会话", Detail: "左侧主机可拖入终端窗格", Action: "添加 SSH 主机"})
			parts.Action.OnClick(func() { a.startHostDraft() })
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
			uikit.DrawDropIndicator(p, r, uikit.DropIndicatorProps{Background: uikit.Color(t.C.DropIndicator.ZoneBg),
				Border: uikit.Color(t.C.DropIndicator.ZoneBorder), Radius: t.C.DropIndicator.ZoneRadius, BorderWidth: t.C.DropIndicator.ZoneBorderWidth})
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
	colors := uikit.ResolveTerminalColors(s, prefs.Background)
	terminalBackground := colors.Background
	background := uikit.Color(terminalBackground)
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
	pane := uikit.Pane(c, uikit.PaneProps{
		Key: fmt.Sprintf("pane-wrap-%d", session.id), Label: h.name + " SSH 终端", Surface: surface,
		Focused: tab.focused == session, Multiple: multiple, Lifted: a.lifted(session),
	}).Absolute().Left(rect.X).Top(rect.Y).Size(rect.W, rect.H).Bind(&session.frame)
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
	appearance := uikit.PaneVisualStyle(pane, focused, hovered, multiple, surface)
	var card, hairline ui.Element
	pane.Children(func() {
		card = uikit.TerminalSurface(c, s, prefs.Background, func() {})
		uikit.StylePaneCard(card, surface, background, t.C.Pane.Radius)
		card.Children(func() {
			if session.terminal != nil {
				output := a.sshTerminalView(c.Key("live-output"), session).
					Absolute().Left(0).Top(topPadding).Width(session.contentWidth).
					Height(max(1, session.contentHeight-topPadding-bottomPadding)).Opacity(appearance.Opacity)
				a.recordTerminalCapture(c, session, output, card, topPadding, bottomPadding)
				if appearance.Saturation != 1 {
					output.Effect(terminalSaturation.Effect(shader.Options{Uniforms: map[string]any{"Saturation": appearance.Saturation}}))
				}
			} else {
				// Scrollbars decorates this same scroll node without replacing its handle.
				output := ui.Scroll(c.Key("output")).Fill().MinWidth(0).MinHeight(0).
					Bind(&session.focus).Focusable().FocusRing(false).
					TrackScroll(&scroll.offset).Padding(
					topPadding,
					p.ContentSide,
					bottomPadding,
					p.ContentSide,
				).Font(strings.Join(prefs.FontFamily, ", ")).FontSize(prefs.FontSize).
					LetterSpacing(prefs.Tracking).TextColor(uikit.Color(colors.Foreground)).SelectionColor(uikit.Color(colors.Selection)).
					Opacity(appearance.Opacity).Cache().
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
				terminalScrollbar(c, card, output, scroll, terminalBackground)
			}
			var header ui.Element
			if multiple {
				// A shared drag ancestor lets the title and bell keep their own hover targets.
				header = uikit.PaneHeader(c, uikit.HeaderProps{Key: "identity", Density: uikit.DensityCompact, Surface: surface}).Root.
					Absolute().Top(0).Left(0).Padding(0, 0, 0, p.HeaderPaddingLeft).Label("拖动窗格 " + h.name).
					OnClick(func() { a.focusSession(tab, session) })
				a.paneDragHandle(c, header, paneTransfer{tab: tab, session: session})
				header.Children(func() {
					content := ui.Row(c).Key("drag-header-content").Grow(1).MinWidth(0).
						FillHeight().AlignItems(ui.Center).Gap(surface.C.PaneHeader.Gap)
					if header.Dragging() {
						// The SDK redraws the drag source with root opacity ignored.
						// Hide its content instead, leaving our one card preview visible.
						content.Opacity(0)
					}
					content.Children(func() {
						number := slices.Index(tab.root.sessions(), session) + 1
						uikit.PaneBadge(c, number, focused, surface)
						uikit.Tip(c, uikit.PaneTitle(c, h.name, appearance, surface), fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port))
						uikit.PaneLogin(c, fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port), appearance, surface)
						if !focused {
							terminalActivity(c, session, surface)
						}
					})
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
				splitShortcut := ""
				if focused {
					splitShortcut = shortcutDisplay(a.shortcutBinding("pane.split"))
				}
				split := uikit.PaneIconButton(c, "分栏 "+h.name, sidebarIcon, surface, uikit.PaneIconButtonProps{
					Shortcut: splitShortcut,
					Disabled: count >= maxTerminalPanes || session.closing || !canSplit(session.layoutRect, dropRight) && !canSplit(session.layoutRect, dropBottom),
				}).Absolute().Top(0).Left(0).Size(p.ActionWidth, p.ActionHeight)
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
			uikit.PaneFooter(c, uikit.FooterProps{Key: "status", Surface: surface}).Absolute().Bottom(0).Left(0).Right(0).
				Opacity(appearance.FooterOpacity).PassThrough().Children(func() {
				uikit.ConnectionBadge(c, connectionIndicatorState(session.state), surface)
				latency := h.latency
				if session.state != connected {
					latency = -1
				}
				uikit.Latency(c, latency, uikit.LatencyProps{Surface: surface, Prefix: "延迟"})
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
		uikit.StylePanePlaceholder(pane)
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
	edge := uikit.EdgeTop
	if bottom {
		edge = uikit.EdgeBottom
	}
	uikit.ScrollFade(c, edge, uikit.ScrollFadeProps{
		Height: barHeight, Background: background, MaskTint: uikit.Color(theme.Current().C.FadeBar.Tint), Translucent: a.translucencyEnabled(c),
	})
}
