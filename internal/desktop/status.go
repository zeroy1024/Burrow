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
		tab := a.currentTab()
		if tab == nil {
			uikit.Text(c, "选择一个终端查看状态", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Margin(t.Space.N14, -t.Space.N2, t.Space.N14, -t.Space.N2)
			return
		}
		session := tab.focused
		state := session.state
		if state != connected {
			message := "会话未连接"
			if state == connecting {
				message = "正在连接，等待服务器状态…"
			}
			uikit.Text(c, message, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Margin(t.Space.N14, -t.Space.N2, t.Space.N14, -t.Space.N2)
			return
		}
		h := session.endpoint
		metrics := serverMetrics{}
		var history []float64
		notice := "正在采集服务器状态…"
		if a.monitor.session == session {
			if a.monitor.metrics != nil {
				metrics, history = *a.monitor.metrics, a.monitor.history
				notice = "每 2 秒采样 · 进程 CPU 为累计均值"
			}
			if a.monitor.errorText != "" {
				notice = "监控不可用：" + a.monitor.errorText
			}
		}
		ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
			uikit.Text(c, h.name+" · 跟随焦点", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).FontSize(t.Font.Size.Caption2).
				SingleLine().Ellipsis("…")
			ui.Column(c).Gap(t.Space.N8).Children(func() {
				value := "—"
				if metrics.cpu != nil {
					value = fmt.Sprintf("%.0f%%", *metrics.cpu)
				}
				monitorMeter(c, "CPU", value, metrics.cpu)
				uikit.Sparkline(c, history, uikit.SparklineProps{Label: "CPU 使用率趋势"})
				cores := "—"
				if metrics.cores > 0 {
					cores = fmt.Sprint(metrics.cores)
				}
				uikit.Text(c, cores+" 核 · 负载 "+monitorText(metrics.load), uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			})
			memory := "—"
			if metrics.memory != nil {
				memory = fmt.Sprintf("%.1f / %.1f GB", float64(metrics.memoryUsed)/1e9, float64(metrics.memoryTotal)/1e9)
			}
			monitorMeter(c, "内存", memory, metrics.memory)
			ui.Column(c).Gap(t.Space.N6).Children(func() {
				disk := "—"
				if metrics.disk != nil {
					disk = fmt.Sprintf("%.1f / %.1f GB", float64(metrics.diskUsed)/1e9, float64(metrics.diskTotal)/1e9)
				}
				monitorMeter(c, "磁盘", disk, metrics.disk)
				uikit.Text(c, monitorText(metrics.diskDevice)+" · "+monitorText(metrics.filesystem), uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			})
			ui.Column(c).Gap(t.Space.N10).Children(func() {
				uikit.Text(c, "网络", uikit.TextProps{Style: uikit.TextSmall})
				uikit.KeyValue(c, []uikit.KV{{Key: "↓ 下行", Value: monitorRate(metrics.down)}, {Key: "↑ 上行", Value: monitorRate(metrics.up)}})
			})
			uikit.Divider(c)
			ui.Column(c).Gap(t.Space.N10).Children(func() {
				uikit.Stat(c, "进程 Top", "CPU")
				for index := range 4 {
					name, cpu := "—", "—"
					if index < len(metrics.top) {
						process := metrics.top[index]
						name, cpu = process.name, fmt.Sprintf("%.1f%%", process.cpu)
					}
					uikit.KeyValue(c, []uikit.KV{{Key: name, Value: cpu, Mono: true}})
				}
			})
			uikit.Divider(c)
			ui.Column(c).Gap(t.Space.N12).Children(func() {
				processes := "—"
				if metrics.processes > 0 {
					processes = fmt.Sprint(metrics.processes)
				}
				uikit.KeyValue(c, []uikit.KV{{Key: "主机名", Value: monitorText(metrics.hostname)}, {Key: "系统", Value: monitorText(metrics.system)},
					{Key: "内核", Value: monitorText(metrics.kernel), Dim: true}, {Key: "运行时间", Value: monitorUptime(metrics.uptime)}, {Key: "进程", Value: processes}})
			})
			uikit.Text(c, notice, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			ui.Box(c).Height(t.Space.N8)
		})
	})
	if result.Close {
		a.statusVisible = false
		a.monitor.stop()
	}
}

func monitorMeter(c *ui.Context, label, value string, percent *float64) {
	if percent != nil {
		uikit.MeterStat(c, label, value, float32(*percent))
		return
	}
	t := theme.Current().C.Progress
	uikit.Stat(c, label, value, func() {
		ui.Box(c).Height(t.HeightMeter).FillWidth().Radius(t.Radius).Background(uikit.Color(t.Track))
	})
}
