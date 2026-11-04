package desktop

import (
	"context"
	"fmt"
	"slices"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

type sshPrompt struct {
	ctx                  context.Context
	title, address       string
	confirmLabel, detail string
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
	a.mcpPromptChanged(ctx, true, prompt.title)
	defer a.mcpPromptChanged(ctx, false, prompt.title)
	prompt.ctx, prompt.open, prompt.answer = ctx, true, make(chan sshAnswer, 1)
	a.window.Update(func() {
		if ctx.Err() == nil {
			a.sshPrompts = append(a.sshPrompts, prompt)
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
		if p.ctx.Err() == nil {
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
		p.secret, p.open = "", false
	}
	result := uikit.Modal(c.Key("ssh-prompt").Key(p), &p.open, uikit.ModalProps{
		Title: p.title, Compact: true, Width: t.C.Dialog.HostWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, p.address, uikit.TextProps{Style: uikit.TextSmall})
		if p.fingerprint != "" {
			uikit.Text(c, "首次连接此服务器。请与服务器管理员核对主机密钥指纹后再信任。", uikit.TextProps{Style: uikit.TextSmall})
			ui.Text(c, fmt.Sprintf("%s\n%s", p.keyType, p.fingerprint)).FontSize(t.Font.Size.Small).Selectable()
		} else {
			if p.echo {
				uikit.Input(c, &p.secret, uikit.InputProps{Label: p.title, Mono: true}).FillWidth().AutoFocus()
			} else {
				uikit.PasswordInput(c, &p.secret, uikit.InputProps{Label: p.title, Mono: true}).Input.FillWidth().AutoFocus()
			}
			detail := p.detail
			if detail == "" {
				detail = "仅用于本次连接，不保存到配置。"
			}
			uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		}
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			label := "连接"
			if p.fingerprint != "" {
				label = "信任并连接"
			}
			if p.confirmLabel != "" {
				label = p.confirmLabel
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
