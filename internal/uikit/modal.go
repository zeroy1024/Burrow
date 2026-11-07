package uikit

import (
	"time"

	"sshdesk/internal/locales"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ModalProps struct {
	Label, Title, Subtitle string
	Icon                   *ui.SVG
	Disabled, Dirty        bool
	Width                  float32
	// DismissDisabled blocks closing while keeping the content interactive.
	DismissDisabled bool
	// Compact keeps a centered confirmation/input layout. Width is its content
	// width, matching the Dialog.GroupWidth/HostWidth tokens.
	Compact        bool
	ContentGap     float32
	Motion         MotionMode
	Controller     *ModalController
	CloseKey       any
	CloseLabel     string
	Header, Footer func(ModalParts)
}

type ModalBounds struct {
	Top, MaxWidth, MaxHeight float32
}

type modalState struct {
	Requested, CloseRequested, Confirm, Discard, Closing, Dirty bool
	EnterStarted, CloseStarted                                  time.Time
}

type modalLayerKey struct {
	owner uint64
	part  string
}

// ModalController requests an exit after a successful asynchronous operation.
// Call Close on the UI thread and keep the modal mounted until Dismissed.
type ModalController struct{ requested bool }

func (controller *ModalController) Close() { controller.requested = true }

type ModalParts struct {
	Panel, Backdrop ui.Element
	Bounds          ModalBounds
	state           *modalState
	context         *ui.Context
}

func (parts ModalParts) SetDirty(dirty bool) {
	parts.state.Dirty = dirty
}

func (parts ModalParts) RequestDismiss() {
	parts.state.Requested = true
	parts.context.Invalidate()
}

// Close exits a completed dialog without asking to discard its saved fields.
func (parts ModalParts) Close() {
	parts.state.CloseRequested = true
	parts.context.Invalidate()
}

type ModalResult struct {
	Panel, Backdrop                    ui.Element
	Dismissed, Submit, DismissCanceled bool
	Closing                            bool
	Close                              func()
}

// Modal owns confirmation, animation and removal for every dismissal path.
func Modal(c *ui.Context, open *bool, props ModalProps, body func(ModalParts)) ModalResult {
	result := ModalResult{}
	if !*open {
		return result
	}
	p := theme.Current().C.Modal
	// Keep the controller outside DialogBase so a completed exit never mounts
	// a final backdrop. The anchor has no layout or painted content.
	anchor := ui.Box(c).Absolute().Size(0, 0).PassThrough().Role(ui.RoleNone)
	state := ui.Local(anchor, "uikit.modal.state", func() modalState { return modalState{} })
	state.Dirty = props.Dirty
	if props.Controller != nil && props.Controller.requested {
		props.Controller.requested = false
		state.CloseRequested = true
	}
	if state.CloseRequested && !state.Closing {
		state.CloseRequested = false
		state.Closing, state.CloseStarted = true, c.Now()
	}
	if state.Closing {
		remaining := p.Exit - c.Now().Sub(state.CloseStarted)
		if remaining <= 0 {
			*open, result.Dismissed = false, true
			return result
		}
		c.After(remaining)
	}
	requestClose := func() {
		state.CloseRequested = true
		c.Invalidate()
	}
	result.Close = requestClose
	compact := theme.Current().C.Dialog
	width, height := c.Size()
	top := max(p.TopMin, height*p.TopRatio)
	bounds := ModalBounds{Top: top, MaxWidth: width - 2*p.BodyPaddingX, MaxHeight: height - top - p.BottomMargin}
	panelWidth := props.Width
	if panelWidth == 0 {
		panelWidth = p.Width
	}
	if props.Compact {
		bounds = ModalBounds{Top: compact.ViewportInset, MaxWidth: width - 2*compact.ViewportInset, MaxHeight: height - 2*compact.ViewportInset}
		if props.Width == 0 {
			panelWidth = compact.GroupWidth
		}
		panelWidth += 2 * compact.Padding
	}
	label := props.Title
	if props.Label != "" {
		label = props.Label
	}
	dialogOpen := true
	anchor.Children(func() {
		ui.DialogBase(c.Key(modalLayerKey{owner: anchor.ID(), part: "dialog"}), &dialogOpen, func(backdrop, panel ui.Element) {
			dismissDisabled := props.Disabled || props.DismissDisabled || state.Closing
			result.Panel, result.Backdrop = panel, backdrop
			backdrop.Column().Justify(ui.Start).AlignItems(ui.Center).Padding(top, 0, 0, 0)
			panel.Column().Width(panelWidth).MaxWidth(bounds.MaxWidth).MaxHeight(bounds.MaxHeight).
				Padding(0).Gap(0).Radius(p.Radius).Background(Color(p.Bg)).Label(label)
			if props.Compact {
				backdrop.Justify(ui.Center).Padding(compact.ViewportInset)
				gap := props.ContentGap
				if gap == 0 {
					gap = theme.Current().Space.N14
				}
				panel.Padding(compact.Padding).Gap(gap).Radius(compact.Radius).Background(Color(compact.Bg))
			}
			if props.Compact {
				Shadow(panel, compact.Shadow)
			} else {
				Shadow(panel, p.Shadow)
			}
			if panel.Dismissed() {
				if !dismissDisabled {
					state.Requested = true
				}
			}
			fade, scale := float32(0), float32(1)
			if state.EnterStarted.IsZero() {
				state.EnterStarted = c.Now()
			}
			// A frame may rebuild the view before committing its first pose.
			// Keep that pose for every pass of the opening frame.
			if c.Now().Equal(state.EnterStarted) {
				fade, scale = 1, p.EnterScale
				c.AnimationFrame()
			}
			animation := Tween(p.Fade)
			if state.Closing {
				fade, scale = 1, p.ExitScale
				animation = ui.Animation{Duration: p.Exit, Ease: Easing(theme.Current().Motion.Easing.In)}
			}
			if MotionReduced(c, props.Motion) {
				scale = 1
			}
			if props.Compact {
				styleModalScrim(backdrop, 1, Color(compact.Backdrop))
			} else {
				StyleModalScrim(backdrop, 1)
			}
			backdrop.AnimatePose(ui.Pose{Fade: fade}, animation)
			panel.AnimatePose(ui.Pose{Scale: scale}, animation)
			parts := ModalParts{Panel: panel, Backdrop: backdrop, Bounds: bounds, state: state, context: c}
			if props.Header != nil {
				props.Header(parts)
			} else if props.Title != "" {
				if props.Compact {
					Text(c, props.Title, TextProps{Style: TextTitle}).FontSize(compact.TitleSize)
				} else {
					ModalHeader(c, props).Padding(p.HeaderPaddingTop, p.BodyPaddingX, p.HeaderPaddingBottom)
				}
			}
			body(parts)
			if props.Footer != nil {
				props.Footer(parts)
			}
			if !props.Compact {
				closeLabel := props.CloseLabel
				if closeLabel == "" {
					closeLabel = locales.MessageCloseWindow()
				}
				closeButton := IconButton(c, IconClose, IconButtonProps{
					Key: props.CloseKey, Label: closeLabel, Size: SizeSm, Disabled: dismissDisabled,
				}).
					Absolute().Top(p.HeaderPaddingTop).Right(p.HeaderPaddingRight)
				if closeButton.Clicked() {
					parts.RequestDismiss()
				}
			}
			if !props.Disabled && !state.Closing && !state.Confirm {
				result.Submit = panel.OverlayShortcut(ui.Super, ui.KeyEnter)
			}
			if state.Requested {
				state.Requested = false
				if !dismissDisabled {
					if state.Dirty {
						state.Confirm = true
					} else {
						requestClose()
					}
				}
			}
			if props.Disabled || state.Closing {
				ui.Overlay(c.Key(modalLayerKey{owner: anchor.ID(), part: "input"}), func() {
					InputShield(c).Modal().AutoFocus().Role(ui.RoleStatus).Label(locales.MessageProcessing2())
				})
			}
		})
	})
	if state.Confirm {
		confirm := Modal(c.Key(modalLayerKey{owner: anchor.ID(), part: "confirm"}), &state.Confirm, ModalProps{
			Title: locales.MessageDiscardUnsavedChanges(), Compact: true, Motion: props.Motion, ContentGap: compact.Gap,
		}, func(parts ModalParts) {
			t := theme.Current()
			Text(c, locales.MessageUnsavedChangesInThisWindowWillBeLostIfYouCloseIt(), TextProps{})
			ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
				if Button(c, locales.MessageKeepEditing(), ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				if Button(c, locales.MessageDiscardChanges(), ButtonProps{Variant: ButtonDanger}).Clicked() {
					state.Discard = true
					parts.Close()
				}
			})
		})
		confirm.Panel.Role(ui.RoleAlertDialog).Label(locales.MessageDiscardUnsavedChanges2())
		if confirm.Submit {
			state.Discard = true
			confirm.Close()
		}
		if confirm.Dismissed {
			if state.Discard {
				state.Discard = false
				requestClose()
			} else {
				result.DismissCanceled = true
			}
		}
	}
	result.Closing = state.Closing || state.CloseRequested
	return result
}

func ModalHeader(c *ui.Context, props ModalProps) ui.Element {
	t := theme.Current()
	p := t.C.Modal
	return ui.Row(c).FillWidth().Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
		if props.Icon != nil {
			IconTile(c, IconTileProps{Size: SizeLg}, func() {
				ui.Icon(c, props.Icon).Size(t.Size.Icon, t.Size.Icon).PassThrough()
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Current().Space.N4).Children(func() {
			ui.Text(c, props.Title).FontSize(p.TitleSize).FontWeight(p.TitleWeight)
			if props.Subtitle != "" {
				ui.Text(c, props.Subtitle).FontSize(p.SubtitleSize).TextColor(Color(p.Subtitle))
			}
		})
	})
}

func ModalBody(c *ui.Context) ui.Element {
	t := theme.Current()
	return ui.Column(c).FillWidth().MinHeight(0).Padding(t.Space.N8, t.C.Modal.BodyPaddingX, t.Space.N16).
		Gap(t.C.FormRow.RowGap)
}

func ModalFooter(c *ui.Context) ui.Element {
	t := theme.Current()
	p := t.C.Modal
	return ui.Row(c).FillWidth().Shrink(0).Padding(p.FooterPaddingTop, p.FooterPaddingRight, p.FooterPaddingBottom, p.FooterPaddingLeft).
		BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(Color(p.Divider)).Gap(p.FooterGap).AlignItems(ui.Center)
}

func StyleModalScrim(backdrop ui.Element, alpha float32) ui.Element {
	return styleModalScrim(backdrop, alpha, Color(theme.Current().C.Modal.Scrim))
}

func styleModalScrim(backdrop ui.Element, alpha float32, scrim ui.Color) ui.Element {
	scrim.A = uint8(float32(scrim.A) * alpha)
	return backdrop.Background(scrim)
}
