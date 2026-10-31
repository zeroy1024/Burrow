package desktop

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const (
	terminalHeaderHeight = 36
	terminalFooterHeight = 28
)

func (a *app) terminals(c *ui.Context) {
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
			area.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Fill(r, a.theme.Accent.Alpha(.12), 10) })
		}
		if dropped {
			a.pendingDock = func() {
				a.openHost(source.id)
				if catalogDropped {
					a.catalogDrag.externalDrop = true
				}
				if a.drag != nil {
					a.drag.session, a.drag.phase = a.currentTab().focused, dragLanded
				}
			}
		}
		area.Children(func() {
			workspaceCard(c).Fill().MinWidth(0).Center().Gap(12).Children(func() {
				ui.Icon(c, terminalIcon).Size(32, 32).TextColor(a.theme.TextMuted)
				ui.Text(c, "选择一台主机，开始会话").FontSize(15).FontWeight(550)
				ui.Text(c, "左侧主机可拖入终端窗格").FontSize(12).TextColor(a.theme.TextMuted)
				ui.Button(c, "添加 SSH 主机").OnClick(func() { a.startHostDraft() })
			})
		})
		return
	}
	minimum := tab.root.minimum()
	area.MinWidth(minimum.X).MinHeight(minimum.Y)
	bounds := area.Bounds()
	if bounds.W <= 0 || bounds.H <= 0 {
		c.AnimationFrame()
		return
	}
	panes, splits := []panePlacement{}, []splitPlacement{}
	if a.cellWidth == 0 {
		a.cellWidth, _ = ui.ShapeText("M", ui.Font{Family: "Menlo, monospace", Size: 11}, 0).Size()
	}
	tab.root.placements(ui.Rect{W: max(bounds.W, minimum.X), H: max(bounds.H, minimum.Y)}, &panes, &splits)
	a.zoomProgress(c, tab)
	targets := make([]ui.Element, len(panes))
	area.Children(func() {
		// Stable siblings let layout transitions follow sessions across arbitrary tree edits.
		for i, placement := range panes {
			placement.session.layoutRect = placement.rect
			if placement.session != tab.focused {
				targets[i] = a.terminalPane(c, tab, placement)
			}
		}
		for i, placement := range panes {
			if placement.session != tab.focused {
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
			p.Fill(r, a.theme.Accent.Alpha(.12), 8)
			p.Stroke(r, a.theme.Accent, 8, 1.5)
		})
	}
}

func (a *app) terminalPane(c *ui.Context, tab *workspaceTab, placement panePlacement) ui.Element {
	session, rect := placement.session, placement.rect
	h := a.hosts[session.host]
	count := tab.root.count()
	multiple := count > 1
	topPadding := float32(10)
	if multiple {
		topPadding = terminalHeaderHeight + 2
	}
	// Only the inner card clips. Its wrapper owns the focus halo and the overlay hairline.
	pane := ui.Box(c).Key(fmt.Sprintf("pane-wrap-%d", session.id)).Absolute().
		Left(rect.X).Top(rect.Y).Size(rect.W, rect.H).Bind(&session.frame).
		MinWidth(0).MinHeight(0).Radius(terminalPaneRadius).Label(h.name + " 演示终端")
	a.paneMotion(c, pane, tab, session)
	_, catalogActive, _ := a.catalogHostTransfer()
	frozen := a.drag != nil || catalogActive || a.resizing != nil ||
		pane.Transitioning() || c.Now().Before(tab.layoutUntil)
	if session.contentWidth == 0 || !frozen {
		if session.contentWidth != rect.W || session.contentHeight != rect.H {
			if session.scroll.offset.Y >= session.scroll.offset.MaxY {
				session.scroll.offset.Y = math.MaxFloat32
			}
			session.contentWidth, session.contentHeight = rect.W, rect.H
			session.columns = max(1, int((rect.W-28)/a.cellWidth))
			session.rows = max(1, int((rect.H-topPadding-50)/19))
		}
	} else {
		c.After(16 * time.Millisecond)
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
	opacity, saturation, footerOpacity := float32(1), float32(1), float32(1)
	titleColor, loginColor := a.theme.Text, a.theme.TextMuted
	titleWeight := 600
	if multiple && !focused {
		opacity, saturation, footerOpacity = 0.5, 0.35, 0.55
		titleColor, loginColor, titleWeight = ui.Hex("#6e6e73"), ui.Hex("#aeaeb2"), 500
		if hovered {
			opacity, saturation = 0.78, 0.7
			titleColor = a.theme.Text
		}
	}
	opacity = pane.Animate("output-opacity", opacity, paneTransition)
	saturation = pane.Animate("output-saturation", saturation, paneTransition)
	footerOpacity = pane.Animate("footer-opacity", footerOpacity, paneTransition)
	if !multiple {
		opacity, saturation, footerOpacity = 1, 1, 1
	}
	titleColor = pane.AnimateColor("host-color", titleColor, ui.Animation{Duration: paneTransition})
	loginColor = pane.AnimateColor("login-color", loginColor, ui.Animation{Duration: paneTransition})
	var card, hairline ui.Element
	pane.Children(func() {
		card = ui.Column(c).Key("card").Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Radius(terminalPaneRadius).Background(ui.Hex(cardBackground)).Clip().
			Draw(func(p *ui.Painter, r ui.Rect) {
				// Absolute children paint after the scroll content, so paint its backdrop here.
				p.Fill(r, ui.Hex("#fbfbfc"), terminalPaneRadius)
			})
		card.Children(func() {
			// Move the native thumb outside the card's clip; the fading thumb is drawn below.
			output := ui.Scroll(c.Key("output")).Fill().MinWidth(0).MinHeight(0).
				Bind(&session.focus).Focusable().FocusRing(false).
				TrackScroll(&scroll.offset).Padding(
				topPadding,
				14,
				50,
				14,
			).ScrollbarInsets(
				terminalHeaderHeight,
				-12,
				terminalFooterHeight,
				0,
			).Font("Menlo, monospace").FontSize(11).TextColor(a.theme.Text).
				Opacity(opacity).Cache().HandleInput(scroll.input).
				OnClick(func() { a.focusSession(tab, session) })
			if saturation != 1 {
				output.Effect(terminalSaturation.Effect(shader.Options{
					Uniforms: map[string]any{"Saturation": saturation},
				}))
			}
			output.Children(func() {
				a.terminalLines(c, session, focused)
			})
			if multiple {
				terminalScrollEdge(c, terminalHeaderHeight, false)
			}
			terminalScrollEdge(c, terminalFooterHeight, true)
			terminalScrollbar(c, pane, scroll)
			var header ui.Element
			if multiple {
				header = ui.Row(c).Key("identity").Absolute().Top(0).Left(0).Height(terminalHeaderHeight).Padding(
					0,
					0,
					0,
					10,
				).Gap(5).PassThrough()
				header.Children(func() {
					labelColor, labelBackground := a.theme.TextMuted, ui.RGB(0, 0, 0).Alpha(0.05)
					if focused {
						labelColor, labelBackground = ui.Hex(cardBackground), a.theme.Accent
					}
					number := slices.Index(tab.root.sessions(), session) + 1
					ui.Text(c, fmt.Sprintf("⌘%d", number)).FontSize(10.5).FontWeight(500).
						FixedLineHeight(12).SingleLine().Padding(3, 5).Radius(5).
						Background(labelBackground).TextColor(labelColor)
					statusDot(c, session.state, 6)
					ui.Text(c, h.name).FontSize(12).FontWeight(titleWeight).TextColor(titleColor).
						SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).
						Tooltip(fmt.Sprintf("%s · %s@%s:%d", h.name, h.user, h.address, h.port))
					ui.Text(c, fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)).
						Font("Menlo, monospace").FontSize(10).TextColor(loginColor).
						SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
					if !focused {
						terminalActivity(c, session)
					}
					handle := ui.Box(c).Key("pane-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(0).
						Label("拖动窗格 " + h.name).OnClick(func() { a.focusSession(tab, session) })
					a.paneDragHandle(c, handle, paneTransfer{tab: tab, session: session})
				})
			}
			actionsWidth := float32(22)
			if multiple {
				actionsWidth = 70
			}
			actions := ui.Box(c).Key("pane-actions").Absolute().Top(6).Right(8).Size(actionsWidth, 24)
			x, y, over := pane.PointerPosition()
			inActionBand := x >= pane.Bounds().W-actionsWidth-40 && y <= terminalHeaderHeight+8
			nearActions := over && inActionBand
			var keyboardFocus bool
			bounds := pane.Bounds()
			stacked := count > 1 && bounds.H > bounds.W
			actions.Children(func() {
				if !multiple {
					ui.Box(c).Absolute().Top(0).Left(0).Size(actionsWidth, 24).Radius(7).PassThrough().
						Material(glass.Blur{Radius: 4})
					ui.Box(c).Absolute().Top(0).Left(0).Size(actionsWidth, 24).Radius(7).PassThrough().
						Background(ui.Hex(cardBackground).Alpha(0.9))
				}
				split := iconButton(c, "分栏 "+h.name, sidebarIcon).Absolute().Top(0).Left(0).Size(22, 24).
					Disabled(count >= maxTerminalPanes || session.closing || !canSplit(session.layoutRect, dropRight) && !canSplit(session.layoutRect, dropBottom))
				keyboardFocus = split.FocusVisible()
				split.OnClick(func() { a.splitSession(session, stacked) })
				if multiple {
					expand := iconButton(c, "放大或恢复 "+h.name, expandIcon).Absolute().Top(0).Left(24).Size(22, 24)
					keyboardFocus = keyboardFocus || expand.FocusVisible()
					expand.OnClick(func() {
						a.togglePaneZoom(tab, session)
					})
					close := iconButton(c, "关闭窗格 "+h.name, closeIcon).Absolute().Top(0).Left(48).Size(22, 24)
					keyboardFocus = keyboardFocus || close.FocusVisible()
					close.OnClick(func() { a.closePane(session) })
				}
			})
			target := float32(0)
			showForPane := multiple && (focused || hovered)
			if showForPane || nearActions || keyboardFocus {
				target = 1
			}
			actionVisibility := actions.Animate("visibility", target, paneTransition)
			actions.Opacity(actionVisibility)
			if multiple {
				header.Right(8 + (actionsWidth+10)*actionVisibility)
			}
			ui.Row(c).Key("status").Absolute().Bottom(0).Left(0).Right(0).Height(terminalFooterHeight).
				Opacity(footerOpacity).
				Padding(0, 10).Gap(8).PassThrough().TextColor(a.theme.TextMuted).Children(func() {
				label, tint := "已连接", ui.Hex("#e4f5ea")
				color := ui.Hex("#2aa255")
				if session.state != connected {
					label, tint, color = "未连接", ui.Hex("#f0f0f3"), a.theme.TextMuted
				}
				ui.Row(c).Padding(2, 6).Gap(4).Radius(99).Background(tint).Children(func() {
					ui.Box(c).Size(5, 5).Radius(99).Background(color)
					ui.Text(c, label).FontSize(10).TextColor(color)
				})
				ui.Text(c, fmt.Sprintf("延迟 %d ms", h.latency)).FontSize(10).SingleLine()
				ui.Spacer(c)
				ui.Text(c, a.paneSizeLabel(session)).FontSize(10).SingleLine()
			})
		})
		hairline = ui.Box(c).Key("hairline").Absolute().Top(0).Right(0).Bottom(0).Left(0).
			Radius(terminalPaneRadius).PassThrough().InsetShadow(
			0,
			0,
			0,
			0.5,
			ui.RGB(0, 0, 0).Alpha(0.08),
		)
	})
	if a.lifted(session) {
		card.Opacity(0)
		hairline.Opacity(0)
		if a.drag.phase != dragLanded {
			a.dragPlaceholder(pane)
		}
	} else {
		paneFocusShadows(pane, focused, multiple)
	}
	a.paneDrop(
		c,
		pane,
		session,
		true,
	)
	return pane
}

func terminalScrollEdge(c *ui.Context, barHeight float32, bottom bool) {
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
	// Blur the scrolling output before fading it into the bar's background.
	band().Material(glass.Blur{
		Radius: 4,
		Mask: &ui.LinearGradient{
			From: ui.RGB(0, 0, 0), To: ui.Transparent,
			Angle: angle, End: 1,
		},
	})
	background := ui.Hex(cardBackground)
	stops := make([]ui.GradientStop, 9)
	for i := range stops {
		at := float32(i) / float32(len(stops)-1)
		opacity := 1 - at*at*(3-2*at)
		stops[i] = ui.GradientStop{Color: background.Alpha(0.96 * opacity), At: at}
	}
	band().LinearGradient(ui.LinearGradient{
		Angle: angle,
		Stops: stops,
	})
}
