package desktop

import (
	"fmt"

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
		ui.Text(c, "选择一个终端查看状态").FontSize(t.Font.Size.Small).TextColor(uikit.Color(t.Color.Text.Secondary)).Padding(t.Space.N14)
		return
	}
	state := a.currentTab().focused.state
	if state != connected {
		message := "会话未连接"
		if state == connecting {
			message = "正在连接，等待服务器状态…"
		}
		ui.Text(c, message).FontSize(t.Font.Size.Small).
			TextColor(uikit.Color(t.Color.Text.Secondary)).Padding(t.Space.N14)
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
				ui.Text(c, h.name+" · 跟随焦点").FontSize(t.Font.Size.Caption2).TextColor(uikit.Color(t.Color.Text.Secondary)).
					SingleLine().Ellipsis("…")
				if h.uptime == "" {
					ui.Text(c, "此演示主机暂无状态数据").FontSize(t.Font.Size.Small).TextColor(uikit.Color(t.Color.Text.Secondary))
					return
				}
				ui.Column(c).Gap(t.Space.N8).Children(func() {
					uikit.MeterStat(
						c,
						"CPU",
						fmt.Sprintf("%.0f%%", h.cpu),
						h.cpu,
					)
					values := []float32{0.28, 0.34, 0.3, 0.46, 0.38, 0.53, 0.35, 0.44, 0.3, 0.64, 0.42, 0.5, 0.36, 0.57, 0.47}
					uikit.Sparkline(c, values, uikit.SparklineProps{Height: t.C.Stats.ChartHeight, StrokeWidth: t.C.Stats.ChartStroke, Fill: t.Color.Surface.Zebra, Line: t.Color.Fill.Strong})
					ui.Text(c, "4 核 · 负载 0.42 / 0.38 / 0.35").FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary))
				})
				uikit.MeterStat(
					c,
					"内存",
					fmt.Sprintf("%.1f / 15 GB", h.memory*15/100),
					h.memory,
				)
				ui.Column(c).Gap(t.Space.N6).Children(func() {
					uikit.MeterStat(
						c,
						"磁盘",
						fmt.Sprintf("%.0f / 120 GB", h.disk*120/100),
						h.disk,
					)
					ui.Text(c, "/dev/vda1 · ext4").FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary))
				})
				ui.Column(c).Gap(t.Space.N10).Children(func() {
					ui.Text(c, "网络").FontSize(t.Font.Size.Small)
					uikit.KeyValue(c, "↓ 下行", "2.4 MB/s", uikit.KeyValueProps{})
					uikit.KeyValue(c, "↑ 上行", "380 KB/s", uikit.KeyValueProps{})
				})
				uikit.Divider(c)
				ui.Column(c).Gap(t.Space.N10).Children(func() {
					uikit.Stat(c, "进程 Top", "CPU")
					for _, process := range []struct{ name, cpu string }{
						{name: "node", cpu: "11.2%"},
						{name: "postgres", cpu: "5.8%"},
						{name: "nginx", cpu: "1.4%"},
						{name: "dockerd", cpu: "0.9%"},
					} {
						uikit.KeyValue(c, process.name, process.cpu, uikit.KeyValueProps{Mono: true})
					}
				})
				uikit.Divider(c)
				ui.Column(c).Gap(t.Space.N12).Children(func() {
					uikit.KeyValue(c, "主机名", h.name, uikit.KeyValueProps{})
					uikit.KeyValue(c, "系统", h.system, uikit.KeyValueProps{})
					uikit.KeyValue(c, "内核", "5.15.0-105", uikit.KeyValueProps{})
					uikit.KeyValue(c, "运行时间", h.uptime, uikit.KeyValueProps{})
					uikit.KeyValue(c, "进程", "184", uikit.KeyValueProps{})
				})
				ui.Text(c, "演示数据").FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary))
				ui.Box(c).Height(t.Space.N8)
			})
		})
		fadingScrollbar(c, scrollbarView{
			element: viewport, offset: &a.statusScroll, label: "监控栏滚动条", color: uikit.Color(t.C.Scrollbar.Color),
		}, &a.statusScrollbar)
	})
}
