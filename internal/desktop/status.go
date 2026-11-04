package desktop

import (
	"fmt"

	"sshdesk/internal/hostinfo"
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
			a.serverHostFacts(c, session)
			return
		}
		h := session.endpoint
		metrics := hostinfo.Metrics{}
		var history []float64
		var disk *float64
		var diskUsed, diskTotal uint64
		notice := "正在采集服务器状态…"
		if a.monitor.session == session {
			if a.monitor.metrics != nil {
				metrics, history = *a.monitor.metrics, a.monitor.history
				if stats := a.monitor.diskStats; stats != nil && a.monitor.diskIdentity == metrics.DiskKey {
					diskTotal, diskUsed = stats.Total, stats.Total-stats.Free
					percent := float64(diskUsed) / float64(diskTotal) * 100
					disk = &percent
				}
			}
			notice = monitorNotice(&a.monitor)
		}
		ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
			uikit.Text(c, h.name+" · 跟随焦点", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).FontSize(t.Font.Size.Caption2).
				SingleLine().Ellipsis("…")
			ui.Column(c).Gap(t.Space.N8).Children(func() {
				value := "—"
				if metrics.CPU != nil {
					value = fmt.Sprintf("%.0f%%", *metrics.CPU)
				}
				monitorMeter(c, "CPU", value, metrics.CPU)
				uikit.Sparkline(c, history, uikit.SparklineProps{Label: "CPU 使用率趋势"})
				cores := "—"
				if metrics.Cores > 0 {
					cores = fmt.Sprint(metrics.Cores)
				}
				uikit.Text(c, cores+" 核 · 负载 "+monitorText(metrics.Load), uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			})
			memory := "—"
			if metrics.Memory != nil {
				memory = fmt.Sprintf("%.1f / %.1f GB", float64(metrics.MemoryUsed)/1e9, float64(metrics.MemoryTotal)/1e9)
			}
			monitorMeter(c, "内存", memory, metrics.Memory)
			ui.Column(c).Gap(t.Space.N6).Children(func() {
				value := "—"
				if disk != nil {
					value = fmt.Sprintf("%.1f / %.1f GB", float64(diskUsed)/1e9, float64(diskTotal)/1e9)
				}
				monitorMeter(c, "磁盘", value, disk)
				uikit.Text(c, monitorText(metrics.DiskDevice)+" · "+monitorText(metrics.Filesystem), uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			})
			ui.Column(c).Gap(t.Space.N10).Children(func() {
				uikit.Text(c, "网络", uikit.TextProps{Style: uikit.TextSmall})
				uikit.KeyValue(c, []uikit.KV{{Key: "↓ 下行", Value: monitorRate(metrics.Down)}, {Key: "↑ 上行", Value: monitorRate(metrics.Up)}})
			})
			uikit.Divider(c)
			ui.Column(c).Gap(t.Space.N10).Children(func() {
				uikit.Stat(c, "进程 Top", "CPU")
				for index := range 4 {
					name, cpu := "—", "—"
					if index < len(metrics.Top) {
						process := metrics.Top[index]
						name, cpu = process.Name, fmt.Sprintf("%.1f%%", process.CPU)
					}
					uikit.KeyValue(c, []uikit.KV{{Key: name, Value: cpu, Mono: true}})
				}
			})
			uikit.Divider(c)
			ui.Column(c).Gap(t.Space.N12).Children(func() {
				processes := "—"
				if metrics.Processes != nil {
					processes = fmt.Sprint(*metrics.Processes)
				}
				a.serverHostFacts(c, session)
				uikit.KeyValue(c, []uikit.KV{
					{Key: "运行时间", Value: monitorUptime(metrics.Uptime)}, {Key: "进程", Value: processes},
				})
			})
			if notice != "" {
				uikit.Text(c, notice, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			}
			ui.Box(c).Height(t.Space.N8)
		})
	})
	if result.Close {
		a.statusVisible = false
		a.monitor.stop()
	}
}

func (a *app) serverHostFacts(c *ui.Context, session *remoteSession) {
	var facts *hostinfo.Facts
	var pending bool
	var errorText string
	if group := session.connection; group != nil {
		if value, ok := a.hostFacts.values[hostFactsKey(group.plan)]; ok && group.planErr == nil {
			facts = &value
		}
		pending = group.facts.pending && group.connection != nil && group.facts.connection == group.connection
		errorText = group.facts.errorText
	}
	rows := []uikit.KV{{Key: "主机名", Value: "—"}, {Key: "系统", Value: "—"}, {Key: "内核", Value: "—"}, {Key: "架构", Value: "—"}}
	if facts != nil {
		rows = []uikit.KV{{Key: "主机名", Value: monitorText(facts.Hostname)}, {Key: "系统", Value: facts.OSName},
			{Key: "内核", Value: monitorText(facts.Kernel)}, {Key: "架构", Value: monitorText(facts.Arch)}}
		if facts.Platform == hostinfo.Windows {
			rows = append(rows, uikit.KV{Key: "默认 Shell", Value: monitorText(facts.Shell)})
		}
	}
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N12).Children(func() {
		uikit.KeyValue(c, rows)
		message := ""
		if pending {
			message = "正在更新主机基础信息…"
		}
		if errorText != "" {
			message = "基础信息获取失败：" + errorText
		}
		if message != "" {
			uikit.Text(c, message, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
		}
	})
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
