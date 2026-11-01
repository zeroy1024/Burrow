package desktop

import (
	"context"
	"fmt"
	"slices"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type sshPrompt struct {
	ctx                  context.Context
	title, address       string
	fingerprint, keyType string
	secret               string
	open                 bool
	responded            bool
	answer               chan sshAnswer
}

type sshAnswer struct {
	accepted bool
	secret   string
}

// SSH workers wait here; only the window's main thread owns the prompt queue.
func (a *app) askSSH(ctx context.Context, prompt *sshPrompt) sshAnswer {
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
		p.responded = true
		p.answer <- sshAnswer{accepted: accepted, secret: p.secret}
		p.secret, p.open = "", false
	}
	modal(c.Key(p), &p.open, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N14).Children(func() {
			ui.Text(c, p.title).FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, p.address).FontSize(t.Font.Size.Small)
			if p.fingerprint != "" {
				ui.Text(c, "首次连接此服务器。请与服务器管理员核对主机密钥指纹后再信任。").FontSize(t.Font.Size.Small)
				ui.Text(c, fmt.Sprintf("%s\n%s", p.keyType, p.fingerprint)).FontSize(t.Font.Size.Small).Selectable()
			} else {
				textInput(c, &p.secret).Label(p.title).Password().FillWidth().AutoFocus()
				ui.Text(c, "仅用于本次连接，不保存到配置。").FontSize(t.Font.Size.Small).
					TextColor(tokenColor(t.Color.Text.Secondary))
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				button(c, "取消").OnClick(func() { answer(false) })
				label := "连接"
				if p.fingerprint != "" {
					label = "信任并连接"
				}
				primaryButton(c, label).OnClick(func() { answer(true) })
			})
		})
	})
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
