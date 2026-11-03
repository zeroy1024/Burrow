package uikit

import (
	"time"

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
	Compact           bool
	ContentGap        float32
	Motion            MotionMode
	ExternalLifecycle bool
	CloseKey          any
	CloseLabel        string
	Header, Footer    func(ModalParts)
}

type ModalBounds struct {
	Top, MaxWidth, MaxHeight float32
}

type modalState struct {
	Requested, Confirm, Closing, Entered, Dirty bool
	CloseStarted                                time.Time
}

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

type ModalResult struct {
	Panel, Backdrop                    ui.Element
	Dismissed, Submit, DismissCanceled bool
}

// Modal owns transient confirmation and its default animation. Applications
// with an existing exit controller receive an accepted dismissal intent through
// ExternalLifecycle and keep their single resource/animation owner.
func Modal(c *ui.Context, open *bool, props ModalProps, body func(ModalParts)) ModalResult {
	result := ModalResult{}
	if !*open {
		return result
	}
	p := theme.Current().C.Modal
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
	var state *modalState
	accept := func() {
		if props.ExternalLifecycle {
			result.Dismissed = true
		} else if !state.Closing {
			state.Closing, state.CloseStarted = true, c.Now()
			c.Invalidate()
		}
	}
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		state = ui.Local(panel, "uikit.modal.state", func() modalState { return modalState{} })
		state.Dirty = props.Dirty
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
		StyleDisabled(panel, props.Disabled || state.Closing)
		if props.Compact {
			Shadow(panel, compact.Shadow)
		} else {
			Shadow(panel, p.Shadow)
		}
		if panel.Dismissed() {
			*open = true
			if !dismissDisabled {
				state.Requested = true
			}
		}
		if !props.ExternalLifecycle {
			alpha, scale := float32(1), float32(1)
			if !state.Entered {
				alpha, scale, state.Entered = 0, p.EnterScale, true
				c.AnimationFrame()
			}
			if state.Closing {
				alpha, scale = 0, p.ExitScale
				if c.Now().Sub(state.CloseStarted) >= p.Exit {
					*open, result.Dismissed = false, true
					panel.Hidden(true)
					// DialogBase has already mounted its modal backdrop this frame.
					// Rebuild once without it so the SDK can restore the trigger focus.
					c.AnimationFrame()
					return
				}
				c.AnimationFrame()
			}
			shown := Visual(panel, "uikit.modal.alpha", alpha, p.Fade)
			if MotionReduced(c, props.Motion) {
				scale = 1
			}
			opacity := shown
			if props.Disabled || state.Closing {
				opacity *= theme.Current().Opacity.Disabled / 0.5
			}
			panel.AnimatePose(ui.Pose{Scale: scale}, Anim(c, p.Fade, props.Motion)).Opacity(opacity)
			if props.Compact {
				styleModalScrim(backdrop, shown, Color(compact.Backdrop))
			} else {
				StyleModalScrim(backdrop, shown)
			}
		} else {
			StyleModalScrim(backdrop, 1)
		}
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
				closeLabel = "关闭窗口"
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
					accept()
				}
			}
		}
	})
	if state != nil && state.Confirm {
		discard := false
		confirm := Modal(c.Key("discard-confirm"), &state.Confirm, ModalProps{
			Title: "放弃未保存的更改？", Compact: true, Motion: props.Motion, ContentGap: compact.Gap,
		}, func(parts ModalParts) {
			t := theme.Current()
			Text(c, "关闭后将丢失此窗口中尚未保存的修改。", TextProps{})
			ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
				if Button(c, "继续编辑", ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				if Button(c, "放弃更改", ButtonProps{Variant: ButtonDanger}).Clicked() {
					discard = true
				}
			})
		})
		confirm.Panel.Role(ui.RoleAlertDialog).Label("放弃未保存的更改")
		if discard || confirm.Submit {
			state.Confirm = false
			accept()
		} else if confirm.Dismissed {
			result.DismissCanceled = true
		}
	}
	return result
}

func ModalHeader(c *ui.Context, props ModalProps) ui.Element {
	t := theme.Current()
	p := t.C.Modal
	return ui.Row(c).FillWidth().Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
		if props.Icon != nil {
			ui.Box(c).Size(p.IconTile, p.IconTile).Radius(p.IconTileRadius).
				Background(Color(t.Color.Surface.Control)).Center().Children(func() {
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
