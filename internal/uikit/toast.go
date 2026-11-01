package uikit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type ToastProps struct {
	ID, Title, Detail, Action string
	Tone                      Tone
	Shortcut                  string
	Timeout                   time.Duration
	OnAction                  func()
}

// Toast uses the SDK queue, announcements, hover pause and action lifetime.
// Only the toast's presentation is owned here; the action belongs to its caller.
func Toast(c *ui.Context, props ToastProps) string {
	timeout := props.Timeout
	if timeout == 0 {
		timeout = theme.Current().C.Toast.Timeout
	}
	kind := "info"
	switch props.Tone {
	case ToneSuccess:
		kind = "success"
	case ToneDanger:
		kind = "error"
	case ToneWarning:
		kind = "warning"
	}
	return c.AddToast(ui.Toast{
		ID: props.ID, Title: props.Title, Description: props.Detail, Type: kind,
		Timeout: timeout, Action: props.Action, OnAction: props.OnAction, Data: props.Shortcut,
	})
}

// Notify retains message-based deduplication for simple notices.
func Notify(c *ui.Context, message string) {
	Toast(c, ToastProps{ID: message, Title: message})
}

var (
	toastOKIcon    = LineIcon(`<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>`)
	toastErrorIcon = LineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/>`)
	toastInfoIcon  = LineIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/>`)
	toastCloseIcon = LineIcon(`<path d="m6 6 12 12M6 18 18 6"/>`)
)

func ToastViewport(c *ui.Context, modes ...MotionMode) ui.Element {
	t := theme.Current()
	p := t.C.Toast
	mode := MotionSystem
	if len(modes) != 0 {
		mode = modes[0]
	}
	width, _ := c.Size()
	return ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Bottom(t.Space.N24).AlignItems(ui.Center).Gap(p.Gap).Padding(0, t.Space.N12)
		for _, toast := range toasts {
			parts := ui.ToastBase(c, toast)
			root := Shadow(parts.Root.Row().Width(p.Width).MaxWidth(width-2*t.Space.N12).
				Padding(p.PaddingY, p.PaddingX).Gap(p.Gap).Radius(p.Radius).
				Background(Color(p.Bg)).TextColor(Color(p.Text)), p.Shadow)
			enter := ui.Motion{}
			if !MotionReduced(c, mode) {
				enter.Scale = t.Opacity.PopoverEnterScale
			}
			root.Transition(ui.ElementTransition{Duration: t.Motion.Duration.Exit, Enter: &enter, Exit: &ui.Motion{}})
			root.Children(func() {
				if toast.Type != "" {
					icon, ink := toastOKIcon, p.OkIcon
					if toast.Type == "error" {
						icon, ink = toastErrorIcon, p.ErrIcon
					} else if toast.Type == "warning" {
						icon, ink = toastErrorIcon, p.WarnIcon
					} else if toast.Type == "info" {
						icon, ink = toastInfoIcon, p.Action
					}
					ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).Shrink(0).TextColor(Color(ink)).PassThrough()
				}
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
					ui.Text(c, toast.Title).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Medium).PassThrough()
					if toast.Description != "" {
						ui.Text(c, toast.Description).FontSize(t.Font.Size.Footnote).TextColor(Color(p.Detail)).PassThrough()
					}
				})
				if toast.Action != "" {
					parts.ActionButton().Label(toast.Action).Padding(t.Space.N4, t.Space.N6).Gap(p.Gap).
						Radius(t.Radius.ControlSm).TextColor(Color(p.Action)).Children(func() {
						ui.Text(c, toast.Action).FontSize(t.Font.Size.Small).SingleLine().PassThrough()
						if shortcut, ok := toast.Data.(string); ok && shortcut != "" {
							Kbd(c, shortcut, KbdProps{Tone: KbdOnInk})
						}
					})
				}
				close := StyleIconButton(parts.CloseButton().Label("关闭通知"), t).
					Size(t.C.IconButton.SizeXs, t.C.IconButton.SizeXs).TextColor(Color(p.Detail))
				Tip(c, close, "关闭通知")
				close.Children(func() { ui.Icon(c, toastCloseIcon).Size(t.Size.IconSm, t.Size.IconSm).PassThrough() })
			})
		}
	})
}
