package desktop

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (a *app) serverStatus(c *ui.Context) {
	t := theme.Current()
	ui.Row(c).Height(t.C.Stats.HeaderHeight).Shrink(0).Padding(0, t.Space.N14).Children(func() {
		ui.Text(c, "服务器状态").FontSize(t.Font.Size.BodyLg).FontWeight(t.Font.Weight.Semibold)
		ui.Spacer(c)
		iconButton(c, "关闭服务器状态", closeIcon).Size(t.C.IconButton.SizeXs, t.C.IconButton.SizeSm).
			OnClick(func() { a.statusVisible = false })
	})
	_, ok := a.focusedHost()
	if !ok {
		ui.Text(c, "选择一个终端查看状态").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary)).Padding(t.Space.N14)
		return
	}
	state := a.currentTab().focused.state
	if state != connected {
		message := "会话未连接"
		if state == connecting {
			message = "正在连接，等待服务器状态…"
		}
		ui.Text(c, message).FontSize(t.Font.Size.Small).
			TextColor(tokenColor(t.Color.Text.Secondary)).Padding(t.Space.N14)
		return
	}
	ui.Text(c, "服务器监控尚未接入真实指标采集。").FontSize(t.Font.Size.Small).
		TextColor(tokenColor(t.Color.Text.Secondary)).Padding(t.Space.N14)
}

func metricBar(c *ui.Context, label, value string, percent float32) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N7).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(t.Font.Size.Small)
			ui.Spacer(c)
			ui.Text(c, value).FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
		})
		ui.Box(c).Height(t.C.Progress.Height).Radius(t.C.Progress.Radius).Background(tokenColor(t.C.Progress.Track)).Children(func() {
			ui.Box(c).WidthPercent(percent).Height(t.C.Progress.Height).Radius(t.C.Progress.Radius).Background(tokenColor(t.C.Progress.FillNeutral))
		})
	})
}

func statusPair(c *ui.Context, label, value string) {
	t := theme.Current()
	ui.Row(c).Gap(t.Space.N8).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary)).Shrink(0)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(t.Font.Size.Caption).SingleLine().Ellipsis("…").MinWidth(0)
	})
}
