package desktop

import (
	"context"
	"fmt"
	"slices"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

type sshPrompt struct {
	modal                uikit.ModalController
	ctx                  context.Context
	address              string
	title                locales.Text
	confirmLabel, detail locales.Text
	fingerprint, keyType string
	secret               string
	open                 bool
	responded            bool
	echo                 bool
	answer               chan sshAnswer
}

type sshAnswer struct {
	accepted bool
	secret   string
}

// SSH workers wait here; only the window's main thread owns the prompt queue.
func (a *app) askSSH(ctx context.Context, prompt *sshPrompt) sshAnswer {
	a.mcpPromptChanged(ctx, true, prompt.title.String())
	defer a.mcpPromptChanged(ctx, false, prompt.title.String())
	prompt.ctx, prompt.open, prompt.answer = ctx, true, make(chan sshAnswer, 1)
	a.window.Update(func() {
		if ctx.Err() == nil {
			a.sshPrompts = append(a.sshPrompts, prompt)
			if !a.window.IsVisible() {
				a.showMainWindow()
			}
		}
	})
	select {
	case answer := <-prompt.answer:
		return answer
	case <-ctx.Done():
		a.window.Invalidate()
		return sshAnswer{}
	}
}

func (a *app) sshDialogs(c *ui.Context) {
	a.sshPrompts = slices.DeleteFunc(a.sshPrompts, func(p *sshPrompt) bool {
		if p.responded || p.ctx.Err() == nil {
			return false
		}
		p.secret = ""
		return true
	})
	if len(a.sshPrompts) == 0 {
		return
	}
	p := a.sshPrompts[0]
	t := theme.Current()
	answer := func(accepted bool) {
		if p.responded {
			return
		}
		p.responded = true
		p.answer <- sshAnswer{accepted: accepted, secret: p.secret}
		p.secret = ""
		p.modal.Close()
	}
	result := uikit.Modal(c.Key("ssh-prompt").Key(p), &p.open, uikit.ModalProps{
		Controller: &p.modal,
		Title:      p.title.String(), Compact: true, Width: t.C.Dialog.HostWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, p.address, uikit.TextProps{Style: uikit.TextSmall})
		if p.fingerprint != "" {
			uikit.Text(
				c,
				locales.MessageThisIsYourFirstConnectionToThisServerVerifyTheHostKeyFingerprint(),
				uikit.TextProps{Style: uikit.TextSmall},
			)
			uikit.Text(c, fmt.Sprintf("%s\n%s", p.keyType, p.fingerprint), uikit.TextProps{Style: uikit.TextMono}).Selectable()
		} else {
			if p.echo {
				uikit.Input(c, &p.secret, uikit.InputProps{Label: p.title.String(), Mono: true}).FillWidth().AutoFocus()
			} else {
				uikit.PasswordInput(c, &p.secret, uikit.InputProps{Label: p.title.String(), Mono: true}).Input.FillWidth().AutoFocus()
			}
			detail := p.detail.String()
			if detail == "" {
				detail = locales.MessageUsedOnlyForThisConnectionAndNotSavedInTheConfiguration()
			}
			uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		}
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			label := locales.MessageConnect()
			if p.fingerprint != "" {
				label = locales.MessageTrustAndConnect()
			}
			if !p.confirmLabel.Empty() {
				label = p.confirmLabel.String()
			}
			uikit.Button(c, label, uikit.ButtonProps{Variant: uikit.ButtonPrimary}).OnClick(func() { answer(true) })
		})
	})
	if result.Submit {
		answer(true)
	}
	if !p.open {
		// Escape and clicking the backdrop cancel through the same path as the button.
		if !p.responded {
			answer(false)
		}
		p.secret = ""
		a.sshPrompts = a.sshPrompts[1:]
		if len(a.sshPrompts) > 0 {
			c.Invalidate()
		}
	}
}
