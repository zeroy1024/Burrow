package uikit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
)

type ToastProps struct {
	ID                    string
	Title, Detail, Action locales.Text
	Tone                  Tone
	Shortcut              string
	Timeout               time.Duration
	OnAction              func()
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
		ID: props.ID, Title: props.Title.String(), Description: props.Detail.String(), Type: kind,
		Timeout: timeout, Action: props.Action.String(), OnAction: props.OnAction, Data: props,
	})
}

// Notify retains message-based deduplication for simple notices.
func Notify(c *ui.Context, message string) {
	Toast(c, ToastProps{ID: message, Title: locales.Raw(message)})
}

var (
	toastOKIcon    = IconSVG(IconCheckCircle)
	toastErrorIcon = IconSVG(IconAlertCircle)
	toastInfoIcon  = IconSVG(IconInfo)
	toastCloseIcon = IconSVG(IconClose)
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
			content := toast.Data.(ToastProps)
			toast.Title, toast.Description, toast.Action = content.Title.String(), content.Detail.String(), content.Action.String()
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
						if content.Shortcut != "" {
							Kbd(c, content.Shortcut, KbdProps{Tone: KbdOnInk})
						}
					})
				}
				close := StyleIconButton(parts.CloseButton().Label(locales.MessageDismissNotification()), t).
					Size(t.C.IconButton.SizeXs, t.C.IconButton.SizeXs).TextColor(Color(p.Detail))
				Tip(c, close, locales.MessageDismissNotification())
				close.Children(func() { ui.Icon(c, toastCloseIcon).Size(t.Size.IconSm, t.Size.IconSm).PassThrough() })
			})
		}
	})
}
