package desktop

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"sshdesk/internal/hostinfo"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

// statusLive is the indicator beside the host name: whether the panel shows
// live samples, is still collecting, or has nothing to collect.
type statusLive struct {
	label     string
	ink, halo color.NRGBA
	// beat is when the latest sample arrived; the dot jumps and settles
	// after it. Zero means it stays still.
	beat time.Time
}

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
		facts, factsMessage := a.statusHostFacts(session)
		m := &a.monitor
		ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
			switch {
			case session.state != connected:
				live, empty := statusOffline(session)
				a.statusHost(c, session, facts, live)
				uikit.EmptyState(c, empty)
				statusFacts(c, &a.statusFactsOpen, facts, factsMessage, nil)
			case m.session == session && m.unsupported:
				a.statusHost(c, session, facts, statusLive{label: "未监控", ink: t.Color.Text.Quaternary})
				uikit.EmptyState(c, uikit.EmptyProps{IconName: uikit.IconActivity, Title: m.platform.DisplayName() + " 暂不支持监控",
					Detail: "目前只有 Linux 提供监控指标，基础信息仍可查看"})
				statusFacts(c, &a.statusFactsOpen, facts, factsMessage, nil)
			default:
				metrics := hostinfo.Metrics{}
				var history monitorHistory
				var disk *float64
				var diskUsed, diskTotal uint64
				live := statusLive{label: "采集中", ink: t.Color.Warning.Default, halo: t.Color.Warning.Halo}
				notice := ""
				// A placeholder shimmers while its value is on the way: before
				// the first sample, or while a rate waits for its second. It stays
				// still once collection fails or the server cannot provide it.
				motion := !a.reduceMotion(c)
				shimmer, diskShimmer := func(string) bool { return motion }, motion
				if m.session == session {
					if m.metrics != nil {
						metrics, history = *m.metrics, m.history
						live = statusLive{label: "实时", ink: t.Color.Success.Default, halo: t.Color.Success.Halo}
						if motion {
							live.beat = m.updatedAt
						}
						if stats := m.diskStats; stats != nil && m.diskIdentity == metrics.DiskKey {
							diskTotal, diskUsed = stats.Total, stats.Total-stats.Free
							percent := float64(diskUsed) / float64(diskTotal) * 100
							disk = &percent
						}
						shimmer = func(item string) bool { return motion && !slices.Contains(metrics.Unavailable, item) }
						diskShimmer = shimmer("挂载信息") && m.diskError == ""
					} else if m.errorText != "" {
						live = statusLive{label: "采集失败", ink: t.Color.Danger.Default, halo: t.Color.Danger.Tint14}
						shimmer, diskShimmer = func(string) bool { return false }, false
					}
					notice = monitorNotice(m)
				}
				a.statusHost(c, session, facts, live)
				if notice != "" {
					uikit.Text(c, notice, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
				}
				statusCPU(c, metrics, history.cpu, m.session == session && m.metrics != nil, shimmer("CPU"))
				statusResources(c, metrics, disk, diskUsed, diskTotal, shimmer("内存"), diskShimmer)
				statusNetwork(c, metrics, history, shimmer("网络"))
				statusProcesses(c, metrics, shimmer("进程 Top"))
				statusFacts(c, &a.statusFactsOpen, facts, factsMessage, metrics.Uptime)
			}
			ui.Box(c).Height(t.Space.N8)
		})
	})
	if result.Close {
		a.statusVisible = false
		a.monitor.stop()
	}
}

// statusOffline describes a session that has no live connection.
func statusOffline(session *remoteSession) (statusLive, uikit.EmptyProps) {
	t := theme.Current()
	hint := ""
	if session.awaitingConnect() {
		hint = "在终端中按 Enter 重新连接"
	}
	switch session.state {
	case connecting:
		return statusLive{label: "连接中", ink: t.Color.Warning.Default, halo: t.Color.Warning.Halo},
			uikit.EmptyProps{IconName: uikit.IconServer, Title: "正在连接", Detail: "连接后开始采集服务器状态"}
	case connectionFailed:
		return statusLive{label: "连接失败", ink: t.Color.Danger.Default, halo: t.Color.Danger.Tint14},
			uikit.EmptyProps{IconName: uikit.IconTerminal, Title: "连接失败", Detail: hint}
	}
	return statusLive{label: "已断开", ink: t.Color.Text.Quaternary},
		uikit.EmptyProps{IconName: uikit.IconTerminal, Title: "会话已断开", Detail: hint}
}

func (a *app) statusHostFacts(session *remoteSession) (*hostinfo.Facts, string) {
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
	message := ""
	if pending {
		message = "正在更新主机基础信息…"
	}
	if errorText != "" {
		message = "基础信息获取失败：" + errorText
	}
	return facts, message
}

func (a *app) statusHost(c *ui.Context, session *remoteSession, facts *hostinfo.Facts, live statusLive) {
	t := theme.Current()
	style, system := a.hostSystem(session.endpoint)
	if facts != nil {
		style, system = factsStyle(*facts), facts.OSName
	}
	card := ui.Row(c).FillWidth().Gap(t.Space.N10).Padding(t.Space.N9, t.Space.N10).AlignItems(ui.Center).
		Radius(t.C.Stats.HostRadius).Background(uikit.Color(t.C.Stats.HostBg))
	// The panel follows the focused terminal; the tooltip says so instead of
	// a permanent caption.
	uikit.Tip(c, card, "跟随当前聚焦的终端")
	card.Children(func() {
		uikit.OsLogo(c, style.name, uikit.SizeDefault)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N1).PassThrough().Children(func() {
			ui.Row(c).FillWidth().Gap(t.Space.N6).AlignItems(ui.Center).PassThrough().Children(func() {
				ui.Text(c, session.endpoint.name).FontSize(t.Font.Size.BodyLg).FontWeight(t.Font.Weight.Semibold).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
				statusLiveBadge(c, live)
			})
			if system != "" {
				ui.Text(c, system).FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary)).
					SingleLine().Ellipsis("…").PassThrough()
			}
		})
	})
}

func statusLiveBadge(c *ui.Context, live statusLive) {
	t := theme.Current()
	ui.Row(c).Shrink(0).Gap(t.Space.N5).AlignItems(ui.Center).Role(ui.RoleStatus).Label(live.label).PassThrough().Children(func() {
		size, ring := t.Size.DotSm, t.Size.DotHalo
		ui.Box(c).Size(size, size).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			// The dot and its halo start enlarged at each sample and ease
			// back, as the sketch's heartbeat.
			scale := float32(1)
			if s := t.C.Stats; !live.beat.IsZero() {
				if elapsed := p.Now().Sub(live.beat); elapsed < s.BeatDuration {
					progress := uikit.Easing(s.BeatEasing)(float32(max(elapsed, 0)) / float32(s.BeatDuration))
					scale = s.BeatScale + (1-s.BeatScale)*progress
					p.AnimationFrame()
				}
			}
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			dot, halo := size/2*scale, (size/2+ring)*scale
			if live.halo.A != 0 {
				p.Fill(ui.Rect{X: cx - halo, Y: cy - halo, W: 2 * halo, H: 2 * halo}, uikit.Color(live.halo), halo)
			}
			p.Fill(ui.Rect{X: cx - dot, Y: cy - dot, W: 2 * dot, H: 2 * dot}, uikit.Color(live.ink), dot)
		})
		ui.Text(c, live.label).FontSize(t.Font.Size.Caption2).FontWeight(t.Font.Weight.Medium).
			TextColor(uikit.Color(t.Color.Text.Secondary)).SingleLine().PassThrough()
	})
}

// statusHeading is a section label with its context on the right.
func statusHeading(c *ui.Context, label, meta string) {
	t := theme.Current()
	ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Baseline).Children(func() {
		statusLabel(c, label)
		ui.Spacer(c)
		ui.Text(c, meta).FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Tertiary)).FontFeatures("tnum").
			SingleLine().Ellipsis("…").MinWidth(0)
	})
}

func statusLabel(c *ui.Context, label string) ui.Element {
	t := theme.Current()
	return ui.Text(c, label).FontSize(t.Font.Size.Small).FontWeight(t.Font.Weight.Medium).
		TextColor(uikit.Color(t.Color.Text.Secondary)).SingleLine().Shrink(0)
}

func statusCaption(c *ui.Context, text string) ui.Element {
	t := theme.Current()
	return ui.Text(c, text).FontSize(t.Font.Size.Caption2).TextColor(uikit.Color(t.Color.Text.Tertiary)).FontFeatures("tnum").SingleLine()
}

// statusFigure sets a value with a smaller unit after it.
func statusFigure(c *ui.Context, value, unit string, size, unitSize float32, weight int, tone uikit.Tone) ui.Element {
	t := theme.Current()
	spans := []ui.Span{{Text: value, Size: size, Weight: weight, Color: uikit.ToneColor(tone), Features: "tnum"}}
	if unit != "" {
		spans = append(spans, ui.Span{Text: unit, Size: unitSize, Weight: t.Font.Weight.Medium, Color: uikit.Color(t.Color.Text.Secondary)})
	}
	return ui.RichText(c, spans...).SingleLine().Shrink(0)
}

// statusTone colors a usage percentage at the Meter's thresholds.
func statusTone(percent float64) uikit.Tone {
	p := theme.Current().C.Progress
	switch {
	case percent >= float64(p.MeterCritAt)*100:
		return uikit.ToneDanger
	case percent >= float64(p.MeterWarnAt)*100:
		return uikit.ToneWarning
	}
	return uikit.ToneNeutral
}

// statusPlaceholder stands in for a bar or chart that has no data yet.
func statusPlaceholder(c *ui.Context, height float32, shimmer bool) {
	ui.Box(c).Height(height).FillWidth().Draw(func(p *ui.Painter, r ui.Rect) {
		statusSkeleton(p, r, shimmer)
	})
}

// statusSkeleton paints a placeholder block. A shimmering one carries a soft
// band three widths wide that sweeps left to right, crossing the block in
// about half of each period, as the sketch's CSS does.
func statusSkeleton(p *ui.Painter, r ui.Rect, shimmer bool) {
	t := theme.Current()
	s, radius := t.C.Stats, t.C.Progress.Radius
	base := uikit.Color(s.Skeleton)
	p.Fill(r, base, radius)
	if !shimmer {
		return
	}
	phase := float32(p.Now().UnixNano()%int64(s.ShimmerPeriod)) / float32(s.ShimmerPeriod)
	band := ui.Rect{X: r.X + r.W*(6*phase-2), Y: r.Y, W: 3 * r.W, H: r.H}
	p.Clip(r, radius, func() {
		p.FillGradient(band, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
			{Color: base, At: 0}, {Color: uikit.Color(s.Shimmer), At: .4}, {Color: base, At: .8},
		}}, 0)
	})
	p.AnimationFrame()
}

func statusCPU(c *ui.Context, metrics hostinfo.Metrics, samples []float64, sampled, shimmer bool) {
	t := theme.Current()
	cores := "—"
	if metrics.Cores > 0 {
		cores = strconv.Itoa(metrics.Cores)
	}
	ui.Column(c).Gap(t.Space.N8).Children(func() {
		statusHeading(c, "CPU", cores+" 核")
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.End).Children(func() {
			if metrics.CPU == nil {
				statusFigure(c, "—", "", t.C.Stats.ValueLarge, 0, t.Font.Weight.Medium, uikit.ToneTertiary)
			} else {
				statusFigure(c, fmt.Sprintf("%.0f", *metrics.CPU), "%", t.C.Stats.ValueLarge, t.Font.Size.Title,
					t.Font.Weight.Medium, statusTone(*metrics.CPU))
			}
			ui.Spacer(c)
			load, high := statusLoad(metrics)
			ui.Column(c).Shrink(0).Gap(t.Space.N2).AlignItems(ui.End).Description("1、5、15 分钟平均负载").Children(func() {
				statusCaption(c, "负载 1 · 5 · 15")
				ink := t.Color.Text.Strong
				if high {
					ink = t.Color.Warning.TextStrong
				}
				ui.Text(c, load).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption).
					TextColor(uikit.Color(ink)).SingleLine()
			})
		})
		if len(samples) == 0 {
			statusPlaceholder(c, t.C.Stats.ChartHeight, shimmer)
			if sampled && metrics.CPU == nil {
				statusCaption(c, "需要两次采样建立基线")
			}
			return
		}
		scale, peak := statusCPUScale(samples)
		uikit.Sparkline(c, samples, uikit.SparklineProps{
			Height: t.C.Stats.ChartHeight, Max: scale, Slots: monitorSamples, Grid: true, Label: "CPU 使用率趋势",
		})
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			statusCaption(c, fmt.Sprintf("约 2 分钟 · 0–%.0f%%", scale))
			ui.Spacer(c)
			statusCaption(c, fmt.Sprintf("峰值 %.0f%%", peak))
		})
	})
}

// statusCPUScale picks the chart's top from a few steps above the peak, so a
// lightly loaded host still shows its variation; the caption names the step.
func statusCPUScale(samples []float64) (scale, peak float64) {
	for _, value := range samples {
		peak = max(peak, value)
	}
	for _, step := range []float64{5, 10, 25, 50} {
		if peak*1.25 <= step {
			return step, peak
		}
	}
	return 100, peak
}

// statusLoad lists the 1, 5 and 15 minute loads; high reports a 1-minute
// load above the core count.
func statusLoad(metrics hostinfo.Metrics) (text string, high bool) {
	fields := strings.Fields(strings.ReplaceAll(metrics.Load, "/", " "))
	if len(fields) == 0 {
		return "—", false
	}
	if load, err := strconv.ParseFloat(fields[0], 64); err == nil && metrics.Cores > 0 {
		high = load > float64(metrics.Cores)
	}
	return strings.Join(fields, " "), high
}

func statusResources(c *ui.Context, metrics hostinfo.Metrics, disk *float64, diskUsed, diskTotal uint64, memoryShimmer, diskShimmer bool) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N14).Children(func() {
		memory, free := "—", ""
		if metrics.Memory != nil {
			memory = fmt.Sprintf("%.1f / %.1f GB", float64(metrics.MemoryUsed)/1e9, float64(metrics.MemoryTotal)/1e9)
			free = fmt.Sprintf("可用 %.1f GB", float64(metrics.MemoryTotal-metrics.MemoryUsed)/1e9)
		}
		statusResource(c, "内存", "", metrics.Memory, memory, free, memoryShimmer)
		usage := "—"
		if disk != nil {
			usage = fmt.Sprintf("%.1f / %.1f GB", float64(diskUsed)/1e9, float64(diskTotal)/1e9)
		}
		statusResource(c, "磁盘", "/", disk, usage, statusFilesystem(metrics), diskShimmer)
	})
}

func statusResource(c *ui.Context, label, mount string, percent *float64, usage, detail string, shimmer bool) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space.N4).AlignItems(ui.Baseline).Children(func() {
			statusLabel(c, label)
			if mount != "" {
				ui.Text(c, mount).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption2).
					TextColor(uikit.Color(t.Color.Text.Tertiary)).SingleLine()
			}
			ui.Spacer(c)
			if percent == nil {
				statusFigure(c, "—", "", t.Font.Size.BodyLg, 0, t.Font.Weight.Semibold, uikit.ToneTertiary)
				return
			}
			statusFigure(c, fmt.Sprintf("%.0f", *percent), "%", t.Font.Size.BodyLg, t.Font.Size.Caption2,
				t.Font.Weight.Semibold, statusTone(*percent))
		})
		if percent == nil {
			statusPlaceholder(c, t.C.Progress.HeightMeter, shimmer)
		} else {
			uikit.Meter(c, *percent, 0, 100, uikit.MeterProps{Label: label, Value: usage})
		}
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			ui.Text(c, usage).FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary)).
				FontFeatures("tnum").SingleLine().Shrink(0)
			ui.Spacer(c)
			if detail != "" {
				ui.Text(c, detail).FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Tertiary)).
					FontFeatures("tnum").SingleLine().Ellipsis("…").MinWidth(0)
			}
		})
	})
}

// statusFilesystem names the root device and file system, once when they
// are the same, as an overlay root is.
func statusFilesystem(metrics hostinfo.Metrics) string {
	device, filesystem := metrics.DiskDevice, metrics.Filesystem
	switch {
	case device == "" || device == filesystem:
		return filesystem
	case filesystem == "":
		return device
	}
	return device + " · " + filesystem
}

func statusNetwork(c *ui.Context, metrics hostinfo.Metrics, history monitorHistory, shimmer bool) {
	t := theme.Current()
	// Both directions share one scale so their heights compare.
	var top float64
	for _, value := range history.down {
		top = max(top, value)
	}
	for _, value := range history.up {
		top = max(top, value)
	}
	ui.Column(c).Gap(t.Space.N8).Children(func() {
		statusHeading(c, "网络", "默认路由接口")
		ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
			statusRate(c, "↓ 下行", metrics.Down, history.down, top, shimmer)
			statusRate(c, "↑ 上行", metrics.Up, history.up, top, shimmer)
		})
	})
}

func statusRate(c *ui.Context, label string, value *float64, samples []float64, top float64, shimmer bool) {
	t := theme.Current()
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(t.Space.N3).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Caption).TextColor(uikit.Color(t.Color.Text.Secondary)).SingleLine()
		if value == nil {
			statusFigure(c, "—", "", t.Font.Size.Title, 0, t.Font.Weight.Semibold, uikit.ToneTertiary)
			statusPlaceholder(c, t.C.Stats.NetChartHeight, shimmer)
			return
		}
		number, unit := monitorRate(*value)
		statusFigure(c, number, " "+unit, t.Font.Size.Title, t.Font.Size.Caption2, t.Font.Weight.Semibold, uikit.ToneNeutral)
		uikit.Sparkline(c, samples, uikit.SparklineProps{
			Height: t.C.Stats.NetChartHeight, Max: top, Slots: monitorSamples, Label: label + "趋势",
		})
	})
}

func statusProcesses(c *ui.Context, metrics hostinfo.Metrics, shimmer bool) {
	t := theme.Current()
	meta := "CPU"
	if metrics.Processes != nil {
		meta = fmt.Sprintf("%d 个进程 · CPU", *metrics.Processes)
	}
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		statusHeading(c, "进程 Top", meta)
		ui.Column(c).Gap(t.Space.N2).Children(func() {
			if len(metrics.Top) == 0 && shimmer {
				// Uneven name lengths, as fractions of the row, read as a list
				// still loading; the value column is a fixed short bar.
				const value = .17
				for index, name := range []float32{.36, .28, .41, .23} {
					ui.Box(c).Key(index).FillWidth().Height(t.C.KeyValue.RowHeight).Draw(func(p *ui.Painter, r ui.Rect) {
						line := t.C.Stats.SkeletonLine
						y := r.Y + (r.H-line)/2
						statusSkeleton(p, ui.Rect{X: r.X, Y: y, W: r.W * name, H: line}, true)
						statusSkeleton(p, ui.Rect{X: r.X + r.W*(1-value), Y: y, W: r.W * value, H: line}, true)
					})
				}
				return
			}
			for index := range 4 {
				name, value, share, active := "—", "—", float32(0), false
				if index < len(metrics.Top) {
					process := metrics.Top[index]
					name, value = process.Name, fmt.Sprintf("%.1f%%", process.CPU)
					share = float32(min(process.CPU/100, 1))
					// A process shown as 0.0% recedes with the empty rows.
					active = process.CPU >= 0.05
				}
				statusProcess(c, index, name, value, share, active)
			}
		})
	})
}

// statusProcess draws a Top row over a bar of its share of one core.
func statusProcess(c *ui.Context, key int, name, value string, share float32, active bool) {
	t := theme.Current()
	ink, weight := t.Color.Text.Tertiary, t.Font.Weight.Regular
	if active {
		ink, weight = t.Color.Text.Primary, t.Font.Weight.Medium
	}
	ui.Row(c).Key(key).FillWidth().Height(t.C.KeyValue.RowHeight).Gap(t.Space.N8).AlignItems(ui.Center).
		Draw(func(p *ui.Painter, r ui.Rect) {
			if share <= 0 {
				return
			}
			// The bar reaches into the panel padding so the text keeps the column's edge.
			inset := t.Space.N6
			p.Fill(ui.Rect{X: r.X - inset, Y: r.Y, W: (r.W + 2*inset) * share, H: r.H}, uikit.Color(t.C.Stats.ProcessBar), t.C.Stats.ProcessRadius)
		}).Children(func() {
		ui.Text(c, name).Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Footnote).
			TextColor(uikit.Color(ink)).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		ui.Text(c, value).FontSize(t.Font.Size.Small).FontWeight(weight).TextColor(uikit.Color(ink)).
			FontFeatures("tnum").SingleLine().Shrink(0)
	})
}

// statusFacts shows the host's basic information, which outlives the
// connection; uptime is live and only given while sampling.
func statusFacts(c *ui.Context, open *bool, facts *hostinfo.Facts, message string, uptime *float64) {
	t := theme.Current()
	p := t.C.Collapse
	ui.Column(c).FillWidth().Children(func() {
		parts := ui.CollapsibleBase(c, open)
		parts.Trigger.FillWidth().Gap(t.Space.N4).AlignItems(ui.Center).Label("系统").Children(func() {
			statusLabel(c, "系统").PassThrough()
			ui.Spacer(c).PassThrough()
			if uptime != nil {
				ui.Text(c, "运行 "+monitorUptime(uptime, true)).FontSize(t.Font.Size.Caption).
					TextColor(uikit.Color(t.Color.Text.Tertiary)).FontFeatures("tnum").SingleLine().PassThrough()
			}
			ui.Icon(c, uikit.IconSVG(uikit.IconChevronDown)).Size(p.Chevron, p.Chevron).
				Rotate(-p.ChevronRotate * (1 - parts.Progress())).TextColor(uikit.Color(t.Color.Text.Tertiary)).PassThrough()
		})
		parts.Panel(func() {
			hostname, kernel, arch := "—", "—", "—"
			if facts != nil {
				hostname, kernel, arch = monitorText(facts.Hostname), monitorText(facts.Kernel), monitorText(facts.Arch)
			}
			ui.Column(c).FillWidth().Gap(t.Space.N10).Padding(t.Space.N10, 0, 0, 0).Children(func() {
				statusFact(c, "主机名", hostname, true)
				statusFact(c, "内核", kernel, true)
				ui.Row(c).FillWidth().Gap(t.Space.N16).Children(func() {
					statusFact(c, "架构", arch, true).Shrink(0)
					if uptime != nil {
						statusFact(c, "运行时间", monitorUptime(uptime, false), false)
					}
				})
				if facts != nil && facts.Platform == hostinfo.Windows {
					statusFact(c, "默认 Shell", monitorText(facts.Shell), true)
				}
				if message != "" {
					uikit.Text(c, message, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
				}
			})
		})
	})
}

// statusFact stacks a label over its value, which wraps rather than being cut.
func statusFact(c *ui.Context, label, value string, mono bool) ui.Element {
	t := theme.Current()
	return ui.Column(c).MinWidth(0).Gap(t.Space.N1).Children(func() {
		statusCaption(c, label)
		text := ui.Text(c, value).FontSize(t.Font.Size.Small).TextColor(uikit.Color(t.Color.Text.Primary))
		if mono {
			text.Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Footnote)
		}
	})
}
