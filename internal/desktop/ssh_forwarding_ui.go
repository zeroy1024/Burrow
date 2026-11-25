package desktop

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type forwardPanel struct {
	visible bool
	height  float32
	dialog  *forwardDialog
	scroll  ui.ScrollState
	bar     scrollbarState
}

func (a *app) forwardSession() *remoteSession {
	if tab := a.currentTab(); tab != nil && tab.focused.agent == nil && !tab.focused.closing {
		return tab.focused
	}
	return nil
}

func (a *app) forwardRows(session *remoteSession) []*forwardRow {
	if state := session.forwarding(); state != nil {
		return state.rows
	}
	if session.temporary {
		return nil
	}
	return a.savedForwardRows(session.endpoint)
}

func (a *app) forwardToggle(c *ui.Context) {
	t := theme.Current()
	session := a.forwardSession()
	if session == nil {
		a.forwardUI.visible = false
	}
	count := 0
	if session != nil {
		if state := session.forwarding(); state != nil {
			for _, row := range state.rows {
				if row.run != nil {
					count++
				}
			}
		}
	}
	label := locales.ForwardTitle()
	if count > 0 {
		label += " · " + fmt.Sprintf(locales.ForwardActiveCount(), count)
	}
	button := uikit.IconButton(c, routeIcon, uikit.IconButtonProps{
		Key: "ssh-forward-toggle", Label: label, Variant: uikit.IconOnWindow,
		On: &a.forwardUI.visible, Disabled: session == nil,
	}).Expanded(a.forwardUI.visible)
	dotTarget := float32(0)
	if count > 0 && !a.forwardUI.visible {
		dotTarget = 1
	}
	if opacity := uikit.Visual(button, "forward-running-dot", dotTarget, t.C.IconButton.Transition); opacity > 0 {
		button.DrawOver(func(p *ui.Painter, r ui.Rect) {
			size := t.Size.DotSm
			p.Fill(ui.Rect{X: r.X + r.W - size - t.Space.N2, Y: r.Y + t.Space.N2, W: size, H: size},
				uikit.Color(t.Color.Success.Default).Alpha(opacity), size/2)
		})
	}
}

func forwardMinimumHeight() float32 {
	p := theme.Current().C.TransferDock
	return p.HeaderHeight + p.RowHeight
}

func (a *app) sshWorkspace(c *ui.Context, size ui.Point) {
	const heightMotion = "forward-dock-height"
	t := theme.Current()
	p := t.C.TransferDock
	work := ui.Column(c).Key("ssh-workspace").Grow(1).MinWidth(0).MinHeight(0)
	session := a.forwardSession()
	open := session != nil && a.forwardUI.visible
	minimum := float32(0)
	if tab := a.currentTab(); tab != nil {
		minimum = tab.root.minimum().Y
	}
	gutter := t.C.Window.Gutter
	available := max(0, size.Y-minimum)
	minHeight := forwardMinimumHeight()
	maxHeight := max(minHeight, available-gutter)
	lastHeight := ui.Local(work, "forward-dock-content-height", func() float32 { return 0 })
	var rows []*forwardRow
	var target, opacityTarget float32
	if open {
		rows = a.forwardRows(session)
		desired := p.HeaderHeight + min(float32(max(len(rows), 1)), p.MaxRows)*p.RowHeight
		if a.forwardUI.height > 0 {
			desired = a.forwardUI.height
		}
		*lastHeight = min(max(desired, minHeight), maxHeight)
		target, opacityTarget = *lastHeight+gutter, 1
	}
	reserved := work.AnimateTo(heightMotion, target,
		a.positionAnimation(ui.Animation{Spring: uikit.Spring(p.Collapse)}))
	reduced := a.reduceMotion(c)
	if reduced {
		reserved = target
	}
	reserved = min(max(0, reserved), available)
	opacity := uikit.Visual(work, "forward-dock-opacity", opacityTarget, t.C.Desktop.SidebarDuration)
	work.Children(func() {
		if open && maxHeight > minHeight && reserved >= minHeight+gutter {
			divider, height := a.forwardUI.resizeHandle(
				c,
				reserved-gutter,
				minHeight,
				maxHeight,
			)
			if height != reserved-gutter {
				*lastHeight = height
				reserved = work.Jump(heightMotion, height+gutter)
			}
			divider.Top(size.Y - reserved)
		}
		a.terminals(c, ui.Point{X: size.X, Y: size.Y - reserved})
		if !open && reserved == 0 && opacity == 0 {
			return
		}
		gap := gutter * min(1, reserved/(minHeight+gutter))
		viewport := ui.Box(c).Key("ssh-forward-dock-viewport").FillWidth().Height(reserved-gap).Shrink(0).
			Margin(gap, 0, 0, 0).Clip().Opacity(opacity)
		if !open {
			viewport.PassThrough()
			if reduced && opacity > 0 {
				// Match the monitor's reduced-motion exit: restore terminal space before the visual copy fades.
				viewport.Absolute().Left(0).Right(0).Bottom(0).Height(*lastHeight)
			}
		}
		viewport.Children(func() {
			if open {
				a.forwardDock(c, session, rows, size.X, *lastHeight)
			}
		})
	})
}

func (f *forwardPanel) resizeHandle(c *ui.Context, height, minHeight, maxHeight float32) (ui.Element, float32) {
	divider := uikit.TransferDockDivider(c, locales.ForwardResize(), locales.MessageDragToResizeDoubleClickToReset())
	_, dy, dragging := divider.Dragged()
	delta := -dy
	step := theme.Current().C.Desktop.SiderKeyboardStep
	if divider.Shortcut(0, ui.KeyUp) {
		delta += step
	}
	if divider.Shortcut(0, ui.KeyDown) {
		delta -= step
	}
	resized := min(max(height+delta, minHeight), maxHeight)
	if resized != height {
		f.height = resized
	}
	if divider.DoubleClicked() {
		f.height = 0
		c.AnimationFrame()
	}
	divider.Range(float64(minHeight), float64(maxHeight), float64(resized))
	uikit.TransferDockGrip(c, divider, dragging || divider.FocusVisible())
	return divider, resized
}

func (a *app) forwardDock(c *ui.Context, session *remoteSession, rows []*forwardRow, width, height float32) {
	t := theme.Current()
	state := session.forwarding()
	dock := uikit.TransferDock(c).Key("ssh-forwarding").Height(height).Label(locales.ForwardTitle()).
		Transition(ui.ElementTransition{Colors: true, Duration: t.C.Desktop.SidebarDuration, Exit: &ui.Motion{}})
	count := 0
	for _, row := range rows {
		if row.run != nil {
			count++
		}
	}
	dock.Children(func() {
		card := uikit.TransferDockCard(c)
		card.Children(func() {
			uikit.TransferHeader(c).Children(func() {
				uikit.TransferHeaderTitle(c, locales.ForwardTitle(), routeIcon)
				uikit.TransferHeaderSummary(c, session.endpoint.name)
				ui.Spacer(c)
				if state != nil {
					uikit.TransferHeaderSummary(c, fmt.Sprintf(locales.ForwardActiveCount(), count))
				} else {
					uikit.TransferHeaderSummary(c, locales.MessageDisconnected())
				}
				uikit.TransferAction(c, "forward-new", locales.ForwardNew(), plusIcon, state == nil).
					OnClick(func() { a.openForwardDialog(session, state, nil, forwardEdit) })
				uikit.TransferAction(c, "forward-hide", locales.ForwardHide(), closeIcon, false).
					OnClick(func() { a.forwardUI.visible = false })
			})
			if len(rows) == 0 {
				uikit.TransferDockEmpty(c, locales.ForwardEmpty())
				return
			}
			viewport := ui.Box(c).Fill().MinHeight(0).Clip()
			viewport.Children(func() {
				scroll := ui.Scroll(c).Fill().TrackScroll(&a.forwardUI.scroll).Children(func() {
					for i, row := range rows {
						a.forwardDockRow(c.Key(row.rule.ID), session, state, row, width, i%2 == 1)
					}
				})
				uikit.Scrollbars(c, viewport, scroll, &a.forwardUI.scroll, uikit.ScrollbarProps{
					Label: locales.ForwardTitle(), State: &a.forwardUI.bar,
				})
			})
		})
	})
}

func (a *app) forwardDockRow(c *ui.Context, session *remoteSession, state *sshForwardState, row *forwardRow, width float32, zebra bool) {
	t := theme.Current()
	motion := uikit.MotionOverride(a.reduceMotion(c))
	compact := width < t.C.TransferDock.CompactBreakpoint
	address, _ := row.rule.target()
	if row.rule.SOCKS {
		address = locales.ForwardClientDestination()
	}
	detail, tone := row.forwardDetail()
	container := ui.Row(c).FillWidth().Height(t.C.TransferDock.RowHeight).Shrink(0).
		Padding(0, t.Space.N8).Gap(t.Space.N8).AlignItems(ui.Center)
	if zebra {
		container.Background(uikit.Color(t.C.FileList.Zebra))
	}
	container.Children(func() {
		label := ui.Column(c).MinWidth(0).Gap(t.Space.N2)
		if compact {
			label.Grow(1)
		} else {
			label.Width(width * .22)
		}
		label.Children(func() {
			uikit.Text(c, row.rule.title(), uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
			sub := row.rule.kind()
			if !row.saved {
				sub += " · " + locales.ForwardTemporary()
			}
			if compact {
				sub = row.rule.localAddress() + " · " + address
				if detail != "" {
					sub = detail
				}
			}
			text := uikit.Text(c, sub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary, Mono: compact}).SingleLine().Ellipsis("…")
			uikit.Tip(c, text, sub)
		})
		if !compact {
			uikit.Text(c, row.rule.localAddress(), uikit.TextProps{Style: uikit.TextMono}).Width(width * .22).SingleLine().Ellipsis("…")
			ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
				uikit.Text(c, address, uikit.TextProps{Style: uikit.TextFootnote, Mono: !row.rule.SOCKS}).SingleLine().Ellipsis("…")
				if detail != "" {
					text := uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextCaption, Tone: tone}).SingleLine().Ellipsis("…")
					uikit.Tip(c, text, detail)
				}
			})
		}
		status, statusTone := locales.ForwardStopped(), uikit.ToneMuted
		if !row.startError.Empty() {
			status, statusTone = locales.ForwardFailed(), uikit.ToneDanger
		} else if row.run != nil {
			status, statusTone = locales.ForwardListening(), uikit.ToneSuccess
		}
		uikit.Badge(c, status, uikit.BadgeProps{Dot: true, Tone: statusTone})
		a.forwardRowActions(c, session, state, row, address, motion)
	})
}

func (row *forwardRow) forwardDetail() (string, uikit.Tone) {
	switch {
	case !row.startError.Empty():
		return row.startError.String(), uikit.ToneDanger
	case !row.targetError.Empty():
		return row.targetError.String(), uikit.ToneDanger
	case row.testing:
		return locales.ForwardTesting(), uikit.ToneSecondary
	case row.tested:
		return locales.ForwardTargetReachable(), uikit.ToneSuccess
	default:
		return "", uikit.ToneNeutral
	}
}

func (a *app) forwardRowActions(c *ui.Context, session *remoteSession, state *sshForwardState, row *forwardRow, address string, motion uikit.MotionMode) {
	uikit.TransferAction(c, "copy", locales.ForwardCopyAddress(), uikit.IconSVG(uikit.IconCopy), row.run == nil).OnClick(func() {
		value := row.rule.localAddress()
		if row.rule.SOCKS {
			value = (&url.URL{Scheme: "socks5", Host: value}).String()
		}
		if err := mygo.Clipboard.Write(transfer.TextData(value)); err != nil {
			uikit.Toast(c, uikit.ToastProps{
				ID: "forward-copy-address", Tone: uikit.ToneDanger, Title: locales.ErrorText(err),
			})
			return
		}
		uikit.Toast(c, uikit.ToastProps{
			ID: "forward-copy-address", Tone: uikit.ToneSuccess, Title: locales.Deferred(locales.MessageCopied),
		})
	})
	uikit.TransferAction(c, "test", locales.ForwardTestTarget(), activityIcon, row.run == nil || row.testing).
		OnClick(func() {
			if row.rule.SOCKS {
				a.openForwardDialog(session, state, row, forwardTest)
			} else {
				a.testForward(state, row, address)
			}
		})
	if row.run != nil {
		uikit.TransferAction(c, "stop", locales.ForwardStop(), uikit.IconSVG(uikit.IconPause), false).OnClick(row.stop)
	} else {
		uikit.TransferAction(c, "start", locales.ForwardStart(), uikit.IconSVG(uikit.IconPlay), state == nil).
			OnClick(func() { a.startForward(state, row) })
	}
	menu := uikit.TransferAction(c, "options", locales.ForwardRuleOptions(), moreIcon, row.run != nil)
	uikit.MenuOn(c, menu, []uikit.MenuEntry{
		{ID: "edit", Label: locales.ForwardEdit(), Icon: uikit.IconEdit, Disabled: state == nil,
			OnChoose: func() { a.openForwardDialog(session, state, row, forwardEdit) }},
		{ID: "delete", Label: locales.ForwardDelete(), Icon: uikit.IconTrash, Danger: true,
			OnChoose: func() { a.openForwardDialog(session, state, row, forwardDelete) }},
	}, motion)
}

type forwardDialogAction int

const (
	forwardEdit forwardDialogAction = iota
	forwardTest
	forwardDelete
)

type forwardDialog struct {
	action     forwardDialogAction
	modal      uikit.ModalController
	scroll     ui.ScrollState
	state      *sshForwardState
	endpoint   host
	row        *forwardRow
	kind       int
	name       string
	listen     string
	port       string
	target     string
	targetPort string
	saved      bool
	canSave    bool
	busy       bool
	err        locales.Text
}

func (a *app) openForwardDialog(session *remoteSession, state *sshForwardState, row *forwardRow, action forwardDialogAction) {
	d := &forwardDialog{
		action: action, state: state, endpoint: session.endpoint, row: row, target: "127.0.0.1",
		listen:  defaultForwardListenHost,
		canSave: !session.temporary && a.forwardingHost(session.endpoint) >= 0,
	}
	if row != nil && action == forwardEdit {
		d.name, d.saved = row.rule.Name, row.saved
		d.listen = row.rule.listenHost()
		d.port = strconv.Itoa(row.rule.LocalPort)
		if row.rule.SOCKS {
			d.kind = 1
		} else {
			d.target, d.targetPort = row.rule.TargetHost, strconv.Itoa(row.rule.TargetPort)
		}
	}
	a.forwardUI.dialog = d
}

func (a *app) sshForwardDialog(c *ui.Context) {
	d := a.forwardUI.dialog
	if d == nil {
		return
	}
	title, action := locales.ForwardNew(), locales.ForwardStart()
	variant := uikit.ButtonPrimary
	switch d.action {
	case forwardTest:
		title, action = locales.ForwardTestTarget(), locales.ForwardTest()
	case forwardDelete:
		title, action, variant = locales.ForwardDeleteConfirm(), locales.ForwardDelete(), uikit.ButtonDanger
	default:
		if d.row != nil {
			title, action = locales.ForwardEdit(), locales.CommonSave()
		}
	}
	p := theme.Current().C.Modal
	open, submitted := true, false
	props := uikit.ModalProps{
		Title: title, Width: p.GenerateWidth, Controller: &d.modal,
		Disabled: d.busy, Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Justify(ui.End).Children(func() {
				uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{Variant: uikit.ButtonPlain, Disabled: d.busy}).
					Padding(0, p.ButtonPaddingX).OnClick(parts.RequestDismiss)
				submitted = uikit.Button(c, action, uikit.ButtonProps{Variant: variant, Disabled: d.busy}).
					Padding(0, p.ButtonPaddingX).Shrink(0).Clicked()
			})
		},
	}
	if d.action == forwardEdit {
		props.Width = p.Width
		props.Header = func(uikit.ModalParts) { forwardRuleHeader(c, title, d.endpoint.name) }
	}
	result := uikit.Modal(c.Key("ssh-forward-dialog").Key(d), &open, props, func(parts uikit.ModalParts) {
		fields := func() {
			if d.action == forwardDelete {
				uikit.Text(c, d.row.rule.title(), uikit.TextProps{})
			} else {
				a.forwardFields(c, d)
			}
			if !d.err.Empty() {
				uikit.FieldMessage(c, d.err.String(), uikit.FieldMessageProps{Tone: uikit.ToneDanger})
			}
		}
		if d.action == forwardEdit {
			a.forwardRuleForm(c, d, parts, fields)
			return
		}
		uikit.ModalBody(c).Children(func() {
			uikit.Text(c, d.endpoint.name, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
			fields()
		})
	})
	if (submitted || result.Submit && d.action != forwardDelete) && !d.busy && !result.Closing && !result.Dismissed {
		a.submitForwardDialog(d)
	}
	if result.Dismissed {
		a.forwardUI.dialog = nil
	}
}

func forwardRuleHeader(c *ui.Context, title, hostName string) {
	t := theme.Current()
	p := t.C.Modal
	ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
		Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
		uikit.IconTile(c, uikit.IconTileProps{Size: uikit.SizeLg}, func() {
			uikit.Icon(c, routeIcon, uikit.IconProps{}).TextColor(uikit.Color(t.Color.Text.Strong))
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
			uikit.Text(c, title, uikit.TextProps{Style: uikit.TextTitle}).SingleLine().Ellipsis("…")
			uikit.Text(c, hostName, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
		})
	})
}

func (a *app) forwardRuleForm(c *ui.Context, d *forwardDialog, parts uikit.ModalParts, fields func()) {
	t := theme.Current()
	p := t.C.Modal
	headerHeight := p.HeaderPaddingTop + p.IconTile + p.HeaderPaddingBottom
	footerHeight := p.FooterPaddingTop + t.C.Button.Height + p.FooterPaddingBottom
	scroller := ui.Scroll(c).Key("forward-form-scroll").FillWidth().MinHeight(0).Shrink(1).
		MaxHeight(parts.Bounds.MaxHeight - headerHeight - footerHeight).TrackScroll(&d.scroll).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(0, p.BodyPaddingX, t.Space.N8, p.BodyPaddingX).Children(func() {
			uikit.Segmented(
				c.Key("forward-kind"),
				&d.kind,
				[]uikit.SegItem{
					{Label: locales.ForwardLocal(), IconSVG: routeIcon},
					{Label: locales.ForwardSOCKS(), IconSVG: globeIcon},
				},
				uikit.SegmentedProps{
					Label: locales.ForwardType(), Fill: true, Cards: true, SelectedIconTone: uikit.ToneAccent,
					Motion: uikit.MotionOverride(a.reduceMotion(c)),
				},
			).Disabled(d.busy)
		})
		uikit.FormSection(c, "", fields).Padding(t.Space.N8, p.BodyPaddingX, t.Space.N16, p.BodyPaddingX)
	})
	uikit.Scrollbars(
		c,
		parts.Panel,
		scroller,
		&d.scroll,
		uikit.ScrollbarProps{Label: locales.ForwardTitle()},
	)
}

func (a *app) forwardFields(c *ui.Context, d *forwardDialog) {
	motion := uikit.MotionOverride(a.reduceMotion(c))
	field := func(key, label string, value *string, mono bool) uikit.FormRowParts {
		return uikit.FormRow(c, label, uikit.FormRowProps{Key: key}, func() ui.Element {
			return uikit.Input(c, value, uikit.InputProps{Label: label, Mono: mono, Disabled: d.busy}).FillWidth().MinWidth(0)
		})
	}
	if d.action == forwardEdit {
		field("forward-name", locales.CommonName(), &d.name, false)
		listener := field("forward-listen", locales.ForwardListenHost(), &d.listen, true)
		if ip, err := parseForwardListenIP(d.listen); err == nil && !ip.IsLoopback() {
			hint := locales.ForwardNonLoopbackHelp()
			if d.kind == 1 {
				hint = locales.ForwardSOCKSNonLoopbackHelp()
			}
			listener.Body.Children(func() { uikit.FieldMessage(c, hint, uikit.FieldMessageProps{Tone: uikit.ToneWarning}) })
		}
		field("forward-local", locales.ForwardLocalPort(), &d.port, true)
	}
	if d.action == forwardTest || d.kind == 0 {
		field("forward-target", locales.ForwardTargetHost(), &d.target, true)
		field("forward-target-port", locales.ForwardTargetPort(), &d.targetPort, true)
	}
	if d.action == forwardEdit && d.canSave {
		row := theme.Current().C.FormRow
		ui.Box(c).FillWidth().PaddingStart(row.LabelWidth + row.LabelGap).Children(func() {
			uikit.Checkbox(c, &d.saved, locales.ForwardSaveHost(), uikit.CheckProps{Disabled: d.busy || a.catalogSaving, Motion: motion})
		})
	}
}

func (a *app) submitForwardDialog(d *forwardDialog) {
	d.err = locales.Text{}
	if d.action == forwardDelete {
		a.commitForwardDialog(d, nil)
		return
	}
	if d.state == nil || d.state.Context().Err() != nil {
		d.err = locales.Deferred(locales.ForwardNeedsSSH)
		return
	}
	targetPort, _ := strconv.Atoi(strings.TrimSpace(d.targetPort))
	if d.action == forwardTest {
		address, err := sshclient.ForwardTarget(d.target, targetPort)
		if err != nil {
			d.err = locales.Deferred(locales.ForwardInvalidTarget)
			return
		}
		a.testForward(d.state, d.row, address)
		d.modal.Close()
		return
	}
	port, _ := strconv.Atoi(strings.TrimSpace(d.port))
	rule := &sshForwardRule{
		Name: strings.TrimSpace(d.name), ListenHost: strings.TrimSpace(d.listen), LocalPort: port, SOCKS: d.kind == 1,
	}
	if rule.ListenHost == "" {
		d.err = locales.Deferred(locales.ForwardInvalidListenHost)
		return
	}
	bind, err := rule.bindAddress()
	if err != nil {
		d.err = locales.ErrorText(err)
		return
	}
	rule.ListenHost = bind.Addr().String()
	if !rule.SOCKS {
		rule.TargetHost, rule.TargetPort = strings.TrimSpace(d.target), targetPort
	}
	if _, err := rule.target(); err != nil {
		d.err = locales.ErrorText(err)
		return
	}
	if d.row == nil {
		rule.ID = newSessionID()
	} else {
		rule.ID = d.row.rule.ID
	}
	a.commitForwardDialog(d, rule)
}

func (a *app) commitForwardDialog(d *forwardDialog, rule *sshForwardRule) {
	var before, after *sshForwardRule
	if d.row != nil && d.row.saved {
		before = &d.row.rule
	}
	if rule != nil && d.saved && d.canSave {
		after = rule
	}
	apply := func(err error) {
		d.busy = false
		if err != nil {
			d.err = locales.ErrorText(err)
			return
		}
		d.modal.Close()
		if d.state == nil || d.state.Context().Err() != nil {
			return
		}
		if d.row != nil {
			d.row.stop()
			if rule == nil {
				d.state.rows = slices.DeleteFunc(d.state.rows, func(item *forwardRow) bool { return item == d.row })
			} else {
				d.row.rule, d.row.saved = *rule, after != nil
			}
			return
		}
		row := &forwardRow{rule: *rule, saved: after != nil}
		d.state.rows = append(d.state.rows, row)
		a.startForward(d.state, row)
	}
	if before == nil && after == nil {
		apply(nil)
		return
	}
	d.busy = true
	a.persistForward(d.endpoint, before, after, apply)
}
