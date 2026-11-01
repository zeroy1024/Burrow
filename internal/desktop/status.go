package desktop

import (
	"fmt"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) serverStatus(c *ui.Context) {
	t := theme.Current()
	result := uikit.SidePanel(c, uikit.SidePanelProps{
		Title: "服务器状态", CloseLabel: "关闭服务器状态", Scroll: &a.statusScroll,
		ContentPaddingX: t.Space.N16,
		Scrollbar:       &a.statusScrollbar,
	}, func() {
		id, ok := a.focusedHost()
		if !ok {
			uikit.Text(c, "选择一个终端查看状态", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Margin(t.Space.N14, -t.Space.N2, t.Space.N14, -t.Space.N2)
			return
		}
		state := a.currentTab().focused.state
		if state != connected {
			message := "会话未连接"
			if state == connecting {
				message = "正在连接，等待服务器状态…"
			}
			uikit.Text(c, message, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Margin(t.Space.N14, -t.Space.N2, t.Space.N14, -t.Space.N2)
			return
		}
		h := a.hosts[id]
		ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
			uikit.Text(c, h.name+" · 跟随焦点", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).FontSize(t.Font.Size.Caption2).
				SingleLine().Ellipsis("…")
			if h.uptime == "" {
				uikit.Text(c, "此演示主机暂无状态数据", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
				return
			}
			ui.Column(c).Gap(t.Space.N8).Children(func() {
				uikit.MeterStat(
					c,
					"CPU",
					fmt.Sprintf("%.0f%%", h.cpu),
					h.cpu,
				)
				values := []float64{0.28, 0.34, 0.3, 0.46, 0.38, 0.53, 0.35, 0.44, 0.3, 0.64, 0.42, 0.5, 0.36, 0.57, 0.47}
				uikit.Sparkline(c, values, uikit.SparklineProps{Label: "CPU 使用率趋势"})
				uikit.Text(c, "4 核 · 负载 0.42 / 0.38 / 0.35", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
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
				uikit.Text(c, "/dev/vda1 · ext4", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			})
			ui.Column(c).Gap(t.Space.N10).Children(func() {
				uikit.Text(c, "网络", uikit.TextProps{Style: uikit.TextSmall})
				uikit.KeyValue(c, []uikit.KV{{Key: "↓ 下行", Value: "2.4 MB/s"}, {Key: "↑ 上行", Value: "380 KB/s"}})
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
					uikit.KeyValue(c, []uikit.KV{{Key: process.name, Value: process.cpu, Mono: true}})
				}
			})
			uikit.Divider(c)
			ui.Column(c).Gap(t.Space.N12).Children(func() {
				uikit.KeyValue(c, []uikit.KV{{Key: "主机名", Value: h.name}, {Key: "系统", Value: h.system},
					{Key: "内核", Value: "5.15.0-105", Dim: true}, {Key: "运行时间", Value: h.uptime}, {Key: "进程", Value: "184"}})
			})
			uikit.Text(c, "演示数据", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			ui.Box(c).Height(t.Space.N8)
		})
	})
	if result.Close {
		a.statusVisible = false
	}
}
