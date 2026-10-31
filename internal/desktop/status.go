package desktop

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) serverStatus(c *ui.Context) {
	ui.Row(c).Height(44).Shrink(0).Padding(0, 14).Children(func() {
		ui.Text(c, "服务器状态").FontSize(13).FontWeight(650)
		ui.Spacer(c)
		iconButton(c, "关闭服务器状态", closeIcon).Size(22, 24).
			OnClick(func() { a.statusVisible = false })
	})
	id, ok := a.focusedHost()
	if !ok {
		ui.Text(c, "选择一个终端查看状态").FontSize(12).TextColor(a.theme.TextMuted).Padding(14)
		return
	}
	h := a.hosts[id]
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 16).Children(func() {
		ui.Column(c).Gap(20).Children(func() {
			ui.Text(c, h.name+" · 跟随焦点").FontSize(10.5).TextColor(a.theme.TextMuted).
				SingleLine().Ellipsis("…")
			if h.uptime == "" {
				ui.Text(c, "此演示主机暂无状态数据").FontSize(12).TextColor(a.theme.TextMuted)
				return
			}
			ui.Column(c).Gap(8).Children(func() {
				metricBar(
					c,
					"CPU",
					fmt.Sprintf("%.0f%%", h.cpu),
					h.cpu,
				)
				ui.Box(c).Height(34).Draw(func(p *ui.Painter, r ui.Rect) {
					values := []float32{0.28, 0.34, 0.3, 0.46, 0.38, 0.53, 0.35, 0.44, 0.3, 0.64, 0.42, 0.5, 0.36, 0.57, 0.47}
					var line, fill ui.Path
					for i, value := range values {
						x := r.X + float32(i)*r.W/float32(len(values)-1)
						y := r.Y + r.H*(1-value)
						if i == 0 {
							line.MoveTo(x, y)
							fill.MoveTo(x, r.Y+r.H).LineTo(x, y)
							continue
						}
						line.LineTo(x, y)
						fill.LineTo(x, y)
					}
					fill.LineTo(r.X+r.W, r.Y+r.H).Close()
					p.FillPath(&fill, ui.Hex("#f5f5f7"))
					p.StrokePath(&line, 1.3, ui.Hex("#414145"))
				})
				ui.Text(c, "4 核 · 负载 0.42 / 0.38 / 0.35").FontSize(10).TextColor(a.theme.TextMuted)
			})
			metricBar(
				c,
				"内存",
				fmt.Sprintf("%.1f / 15 GB", h.memory*15/100),
				h.memory,
			)
			ui.Column(c).Gap(6).Children(func() {
				metricBar(
					c,
					"磁盘",
					fmt.Sprintf("%.0f / 120 GB", h.disk*120/100),
					h.disk,
				)
				ui.Text(c, "/dev/vda1 · ext4").FontSize(10).TextColor(a.theme.TextMuted)
			})
			ui.Column(c).Gap(10).Children(func() {
				ui.Text(c, "网络").FontSize(12)
				statusPair(c, "↓ 下行", "2.4 MB/s")
				statusPair(c, "↑ 上行", "380 KB/s")
			})
			ui.Divider(c)
			ui.Column(c).Gap(10).Children(func() {
				ui.Row(c).Children(func() {
					ui.Text(c, "进程 Top").FontSize(12)
					ui.Spacer(c)
					ui.Text(c, "CPU").FontSize(11).TextColor(a.theme.TextMuted)
				})
				for _, process := range []struct{ name, cpu string }{
					{name: "node", cpu: "11.2%"},
					{name: "postgres", cpu: "5.8%"},
					{name: "nginx", cpu: "1.4%"},
					{name: "dockerd", cpu: "0.9%"},
				} {
					ui.Row(c).Children(func() {
						ui.Text(c, process.name).Font("Menlo, monospace").FontSize(11)
						ui.Spacer(c)
						ui.Text(c, process.cpu).FontSize(11).TextColor(a.theme.TextMuted)
					})
				}
			})
			ui.Divider(c)
			ui.Column(c).Gap(11).Children(func() {
				statusPair(c, "主机名", h.name)
				statusPair(c, "系统", h.system)
				statusPair(c, "内核", "5.15.0-105")
				statusPair(c, "运行时间", h.uptime)
				statusPair(c, "进程", "184")
			})
			ui.Text(c, "演示数据").FontSize(10).TextColor(a.theme.TextMuted)
			ui.Box(c).Height(8)
		})
	})
}

func metricBar(c *ui.Context, label, value string, percent float32) {
	ui.Column(c).Gap(7).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, label).FontSize(12)
			ui.Spacer(c)
			ui.Text(c, value).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		ui.Box(c).Height(5).Radius(99).Background(ui.Hex("#f0f0f3")).Children(func() {
			ui.Box(c).WidthPercent(percent).Height(5).Radius(99).Background(ui.Hex("#29292d"))
		})
	})
}

func statusPair(c *ui.Context, label, value string) {
	ui.Row(c).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted).Shrink(0)
		ui.Spacer(c)
		ui.Text(c, value).FontSize(11).SingleLine().Ellipsis("…").MinWidth(0)
	})
}
