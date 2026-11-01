package desktop

import (
	"fmt"
	"strings"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) serverStatus(c *ui.Context) {
	t := theme.Current()
	header := uikit.SidePanelHeader(c, "服务器状态", "关闭服务器状态", closeIcon)
	header.Close.OnClick(func() { a.statusVisible = false })
	id, ok := a.focusedHost()
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
	h := a.hosts[id]
	viewport := uikit.SidePanelViewport(c)
	viewport.Children(func() {
		ui.Scroll(c).Fill().MinHeight(0).TrackScroll(&a.statusScroll).
			ScrollbarInsets(0, -t.C.Scrollbar.TrackWidth, 0, 0).
			HandleInput(func(ev ui.InputEvent) bool {
				return a.statusScrollbar.input(ev, &a.statusScroll)
			}).Padding(0, t.Space.N16).Children(func() {
			ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
				ui.Text(c, h.name+" · 跟随焦点").FontSize(t.Font.Size.Caption2).TextColor(tokenColor(t.Color.Text.Secondary)).
					SingleLine().Ellipsis("…")
				if h.uptime == "" {
					ui.Text(c, "此演示主机暂无状态数据").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
					return
				}
				ui.Column(c).Gap(t.Space.N8).Children(func() {
					metricBar(
						c,
						"CPU",
						fmt.Sprintf("%.0f%%", h.cpu),
						h.cpu,
					)
					ui.Box(c).Height(t.C.Stats.ChartHeight).Draw(func(p *ui.Painter, r ui.Rect) {
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
						p.FillPath(&fill, tokenColor(t.Color.Surface.Zebra))
						p.StrokePath(&line, t.C.Stats.ChartStroke, tokenColor(t.Color.Fill.Strong))
					})
					ui.Text(c, "4 核 · 负载 0.42 / 0.38 / 0.35").FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
				})
				metricBar(
					c,
					"内存",
					fmt.Sprintf("%.1f / 15 GB", h.memory*15/100),
					h.memory,
				)
				ui.Column(c).Gap(t.Space.N6).Children(func() {
					metricBar(
						c,
						"磁盘",
						fmt.Sprintf("%.0f / 120 GB", h.disk*120/100),
						h.disk,
					)
					ui.Text(c, "/dev/vda1 · ext4").FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
				})
				ui.Column(c).Gap(t.Space.N10).Children(func() {
					ui.Text(c, "网络").FontSize(t.Font.Size.Small)
					statusPair(c, "↓ 下行", "2.4 MB/s")
					statusPair(c, "↑ 上行", "380 KB/s")
				})
				ui.Divider(c)
				ui.Column(c).Gap(t.Space.N10).Children(func() {
					ui.Row(c).Children(func() {
						ui.Text(c, "进程 Top").FontSize(t.Font.Size.Small)
						ui.Spacer(c)
						ui.Text(c, "CPU").FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
					})
					for _, process := range []struct{ name, cpu string }{
						{name: "node", cpu: "11.2%"},
						{name: "postgres", cpu: "5.8%"},
						{name: "nginx", cpu: "1.4%"},
						{name: "dockerd", cpu: "0.9%"},
					} {
						ui.Row(c).Children(func() {
							ui.Text(c, process.name).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption)
							ui.Spacer(c)
							ui.Text(c, process.cpu).FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
						})
					}
				})
				ui.Divider(c)
				ui.Column(c).Gap(t.Space.N12).Children(func() {
					statusPair(c, "主机名", h.name)
					statusPair(c, "系统", h.system)
					statusPair(c, "内核", "5.15.0-105")
					statusPair(c, "运行时间", h.uptime)
					statusPair(c, "进程", "184")
				})
				ui.Text(c, "演示数据").FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
				ui.Box(c).Height(t.Space.N8)
			})
		})
		fadingScrollbar(c, scrollbarView{
			element: viewport, offset: &a.statusScroll, label: "监控栏滚动条", color: tokenColor(t.C.Scrollbar.Color),
		}, &a.statusScrollbar)
	})
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
