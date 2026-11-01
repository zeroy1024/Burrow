package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type ModalProps struct {
	Label    string
	Disabled bool
	Width    float32
}

type ModalBounds struct {
	Top, MaxWidth, MaxHeight float32
}

// Modal owns only the shell; callers retain dismissal, animation and save state.
func Modal(c *ui.Context, open *bool, props ModalProps, body func(ui.Element, ui.Element, ModalBounds)) ui.Element {
	p := theme.Current().C.Modal
	width, height := c.Size()
	top := max(p.TopMin, height*p.TopRatio)
	bounds := ModalBounds{Top: top, MaxWidth: width - 2*p.BodyPaddingX, MaxHeight: height - top - p.BottomMargin}
	panelWidth := props.Width
	if panelWidth == 0 {
		panelWidth = p.Width
	}
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Column().Justify(ui.Start).AlignItems(ui.Center).Padding(top, 0, 0, 0)
		panel.Column().Width(panelWidth).MaxWidth(bounds.MaxWidth).MaxHeight(bounds.MaxHeight).
			Padding(0).Gap(0).Radius(p.Radius).Background(Color(p.Bg)).Label(props.Label).Disabled(props.Disabled)
		Shadow(panel, p.Shadow)
		body(backdrop, panel, bounds)
	})
}

func StyleModalScrim(backdrop ui.Element, alpha float32) ui.Element {
	scrim := Color(theme.Current().C.Modal.Scrim)
	scrim.A = uint8(float32(scrim.A) * alpha)
	return backdrop.Background(scrim)
}

func Dialog(c *ui.Context, open *bool, body func()) ui.Element {
	p := theme.Current().C.Dialog
	width, height := c.Size()
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(Color(p.Backdrop))
		panel.Padding(p.Padding).Gap(p.Gap).Radius(p.Radius).Background(Color(p.Bg)).
			MaxWidth(width - 2*p.ViewportInset).MaxHeight(height - 2*p.ViewportInset)
		Shadow(panel, p.Shadow)
		body()
	})
}
