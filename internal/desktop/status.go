package desktop

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

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
	uikit.SidePanel(c, uikit.SidePanelProps{
		Title: locales.MessageServerStatus(), Scroll: &a.statusScroll,
		ContentPaddingX: t.Space.N16,
		Scrollbar:       &a.statusScrollbar,
	}, func() {
		tab := a.currentTab()
		if tab == nil {
			uikit.Text(
				c,
				locales.MessageSelectATerminalToViewItsStatus(),
				uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
			).Margin(t.Space.N14, -t.Space.N2, t.Space.N14, -t.Space.N2)
			return
		}
		session := tab.focused
		facts, factsMessage := a.statusHostFacts(session)
		m := a.sessionMonitor(session)
		ui.Column(c).Gap(t.C.Stats.SectionGap).Children(func() {
			switch {
			case session.agent != nil:
				a.statusHost(c, session, facts, statusLive{label: locales.MessageNotMonitored(), ink: t.Color.Text.Quaternary})
				uikit.EmptyState(c, uikit.EmptyProps{IconName: uikit.IconBot, Title: locales.AgentSessionNoMonitoring(),
					Detail: locales.AgentSessionNoMonitoringDetail()})
			case session.state != connected:
				live, empty := statusOffline(session)
				a.statusHost(c, session, facts, live)
				uikit.EmptyState(c, empty)
				a.statusFacts(c, &a.statusFactsOpen, facts, factsMessage, nil)
			case m != nil && m.unsupported:
				a.statusHost(c, session, facts, statusLive{label: locales.MessageNotMonitored(), ink: t.Color.Text.Quaternary})
				uikit.EmptyState(c, uikit.EmptyProps{IconName: uikit.IconActivity, Title: m.platform.DisplayName() + locales.MessageDoesNotSupportMonitoringYet(),
					Detail: locales.MessageMonitoringMetricsAreCurrentlyAvailableOnlyForLinuxBasicInformationIsStillAvailable()})
				a.statusFacts(c, &a.statusFactsOpen, facts, factsMessage, nil)
			default:
				metrics := hostinfo.Metrics{}
				var history monitorHistory
				var disk *float64
				var diskUsed, diskTotal uint64
				live := statusLive{label: locales.MessageCollecting(), ink: t.Color.Warning.Default, halo: t.Color.Warning.Halo}
				notice := ""
				// A placeholder shimmers while its value is on the way: before
				// the first sample, or while a rate waits for its second. It stays
				// still once collection fails or the server cannot provide it.
				motion := !a.reduceMotion(c)
				shimmer, diskShimmer := func(string) bool { return motion }, motion
				if m != nil {
					if m.metrics != nil {
						metrics, history = *m.metrics, m.history
						live = statusLive{label: locales.MessageLive(), ink: t.Color.Success.Default, halo: t.Color.Success.Halo}
						if motion {
							live.beat = m.updatedAt
						}
						if stats := m.diskStats; stats != nil && m.diskIdentity == metrics.DiskKey {
							diskTotal, diskUsed = stats.Total, stats.Total-stats.Free
							percent := float64(diskUsed) / float64(diskTotal) * 100
							disk = &percent
						}
						shimmer = func(item string) bool { return motion && !slices.Contains(metrics.Unavailable, item) }
						diskShimmer = shimmer(locales.MessageMountInformation()) && m.diskError == ""
					} else if !m.errorText.Empty() {
						live = statusLive{label: locales.MessageCollectionFailed(), ink: t.Color.Danger.Default, halo: t.Color.Danger.Tint14}
						shimmer, diskShimmer = func(string) bool { return false }, false
					}
					notice = monitorNotice(m)
				}
				a.statusHost(c, session, facts, live)
				if notice != "" {
					uikit.Text(c, notice, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
				}
				statusCPU(c, metrics, history.cpu, m != nil && m.metrics != nil, shimmer("CPU"))
				statusResources(c, metrics, disk, diskUsed, diskTotal, shimmer(locales.MessageMemory()), diskShimmer)
				statusNetwork(c, metrics, history, shimmer(locales.MessageNetwork()))
				statusProcesses(c, metrics, shimmer(locales.MessageTopProcesses()))
				a.statusFacts(c, &a.statusFactsOpen, facts, factsMessage, metrics.Uptime)
			}
			ui.Box(c).Height(t.Space.N8)
		})
	})
}

// paneReadings shows a connected terminal's latency and latest samples in the
// pane footer. They appear together, with a dash until each has a value; a
// narrowing pane drops the wider network rates first, then the other samples.
func (a *app) paneReadings(c *ui.Context, session *remoteSession, surface *theme.Theme) {
	if session.state != connected {
		return
	}
	p := surface.C.PaneFooter
	ui.Row(c).Shrink(0).Gap(p.MetricGap).AlignItems(ui.Center).Children(func() {
		uikit.Latency(c, sshLatencyMS(session, c.Now()), uikit.LatencyProps{Surface: surface, Label: locales.MessageLatency()})
		m := a.sessionMonitor(session)
		if m != nil && m.unsupported || session.layoutRect.W < p.MetricsMinWidth {
			return
		}
		metrics := hostinfo.Metrics{}
		if m != nil && m.metrics != nil {
			metrics = *m.metrics
		}
		percent := func(value *float64) string {
			if value == nil {
				return "—"
			}
			return fmt.Sprintf("%.0f%%", *value)
		}
		rate := func(value *float64) string {
			if value == nil {
				return "—"
			}
			number, unit := monitorRate(*value)
			return number + " " + unit
		}
		uikit.PaneMetric(c.Key("cpu"), uikit.IconCPU, percent(metrics.CPU), "CPU", surface)
		uikit.PaneMetric(c.Key("memory"), uikit.IconMemory, percent(metrics.Memory), locales.MessageMemory(), surface)
		if session.layoutRect.W < p.MetricsNetMinWidth {
			return
		}
		uikit.PaneMetric(c.Key("down"), uikit.IconArrowDown, rate(metrics.Down), locales.MessageDownload3(), surface)
		uikit.PaneMetric(c.Key("up"), uikit.IconArrowUp, rate(metrics.Up), locales.MessageUpload3(), surface)
	})
}

// statusOffline describes a session that has no live connection.
func statusOffline(session *remoteSession) (statusLive, uikit.EmptyProps) {
	t := theme.Current()
	hint := ""
	if session.awaitingConnect() {
		hint = locales.MessagePressEnterInTheTerminalToReconnect()
	}
	switch session.state {
	case connecting:
		return statusLive{label: locales.MessageConnecting2(), ink: t.Color.Warning.Default, halo: t.Color.Warning.Halo},
			uikit.EmptyProps{IconName: uikit.IconServer, Title: locales.MessageConnecting3(), Detail: locales.MessageServerStatusCollectionStartsAfterConnecting()}
	case connectionFailed:
		return statusLive{label: locales.MessageConnectionFailed(), ink: t.Color.Danger.Default, halo: t.Color.Danger.Tint14},
			uikit.EmptyProps{IconName: uikit.IconTerminal, Title: locales.MessageConnectionFailed(), Detail: hint}
	}
	return statusLive{label: locales.MessageDisconnected(), ink: t.Color.Text.Quaternary},
		uikit.EmptyProps{IconName: uikit.IconTerminal, Title: locales.MessageSessionDisconnected(), Detail: hint}
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
		errorText = group.facts.errorText.String()
	}
	message := ""
	if pending {
		message = locales.MessageUpdatingHostInformation()
	}
	if errorText != "" {
		message = locales.MessageUnableToRetrieveHostInformation3() + errorText
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
	uikit.Tip(c, card, locales.MessageFollowsTheFocusedTerminal())
	card.Children(func() {
		uikit.OsLogo(c, style.name, uikit.SizeDefault)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N1).PassThrough().Children(func() {
			ui.Row(c).FillWidth().Gap(t.Space.N6).AlignItems(ui.Center).PassThrough().Children(func() {
				uikit.Text(c, session.endpoint.name, uikit.TextProps{Style: uikit.TextHeadline}).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0).PassThrough()
				statusLiveBadge(c, live)
			})
			if system != "" {
				uikit.Text(c, system, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).
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
		uikit.Text(c, live.label, uikit.TextProps{Style: uikit.TextCaption2, Tone: uikit.ToneSecondary}).SingleLine().PassThrough()
	})
}

// statusHeading is a section label with its context on the right.
func statusHeading(c *ui.Context, label, meta string) {
	t := theme.Current()
	ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Baseline).Children(func() {
		statusLabel(c, label)
		ui.Spacer(c)
		uikit.Text(c, meta, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).FontFeatures("tnum").
			SingleLine().Ellipsis("…").MinWidth(0)
	})
}

func statusLabel(c *ui.Context, label string) ui.Element {
	return uikit.Text(c, label, uikit.TextProps{Style: uikit.TextLabel, Tone: uikit.ToneSecondary}).SingleLine().Shrink(0)
}

func statusCaption(c *ui.Context, text string) ui.Element {
	return uikit.Text(c, text, uikit.TextProps{Style: uikit.TextCaption2, Tone: uikit.ToneTertiary}).FontFeatures("tnum").SingleLine()
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
	p.AnimationFrameRate(30)
}

func statusCPU(c *ui.Context, metrics hostinfo.Metrics, samples []float64, sampled, shimmer bool) {
	t := theme.Current()
	cores := "—"
	if metrics.Cores > 0 {
		cores = locales.MonitorCoreCount(metrics.Cores)
	}
	ui.Column(c).Gap(t.Space.N8).Children(func() {
		statusHeading(c, "CPU", cores)
		ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.End).Children(func() {
			if metrics.CPU == nil {
				statusFigure(c, "—", "", t.C.Stats.ValueLarge, 0, t.Font.Weight.Medium, uikit.ToneTertiary)
			} else {
				statusFigure(c, fmt.Sprintf("%.0f", *metrics.CPU), "%", t.C.Stats.ValueLarge, t.Font.Size.Title,
					t.Font.Weight.Medium, statusTone(*metrics.CPU))
			}
			ui.Spacer(c)
			load, high := statusLoad(metrics)
			ui.Column(c).Shrink(0).Gap(t.Space.N2).AlignItems(ui.End).Description(locales.MessageAverageLoadOverAndMinutes()).Children(func() {
				statusCaption(c, locales.MessageLoad())
				tone := uikit.ToneNeutral
				if high {
					tone = uikit.ToneWarning
				}
				uikit.Text(c, load, uikit.TextProps{Style: uikit.TextCaption, Tone: tone, Mono: true}).SingleLine()
			})
		})
		if len(samples) == 0 {
			statusPlaceholder(c, t.C.Stats.ChartHeight, shimmer)
			if sampled && metrics.CPU == nil {
				statusCaption(c, locales.MessageTwoSamplesAreRequiredToEstablishABaseline())
			}
			return
		}
		scale, peak := statusCPUScale(samples)
		uikit.Sparkline(c, samples, uikit.SparklineProps{
			Height: t.C.Stats.ChartHeight, Max: scale, Slots: monitorSamples, Grid: true, Label: locales.MessageCpuUsageTrend(),
		})
		ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
			statusCaption(c, fmt.Sprintf(locales.MessageAboutMinutes(), scale))
			ui.Spacer(c)
			statusCaption(c, fmt.Sprintf(locales.MessagePeak(), peak))
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
			free = fmt.Sprintf(locales.MessageGbAvailable(), float64(metrics.MemoryTotal-metrics.MemoryUsed)/1e9)
		}
		statusResource(c, locales.MessageMemory(), "", metrics.Memory, memory, free, memoryShimmer)
		usage := "—"
		if disk != nil {
			usage = fmt.Sprintf("%.1f / %.1f GB", float64(diskUsed)/1e9, float64(diskTotal)/1e9)
		}
		statusResource(c, locales.MessageDisk(), "/", disk, usage, statusFilesystem(metrics), diskShimmer)
	})
}

func statusResource(c *ui.Context, label, mount string, percent *float64, usage, detail string, shimmer bool) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Row(c).FillWidth().Gap(t.Space.N4).AlignItems(ui.Baseline).Children(func() {
			statusLabel(c, label)
			if mount != "" {
				uikit.Text(c, mount, uikit.TextProps{Style: uikit.TextCaption2, Tone: uikit.ToneTertiary, Mono: true}).SingleLine()
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
			uikit.Text(c, usage, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).
				FontFeatures("tnum").SingleLine().Shrink(0)
			ui.Spacer(c)
			if detail != "" {
				uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).
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
		statusHeading(c, locales.MessageNetwork(), locales.MessageDefaultRouteInterface())
		ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
			statusRate(c, locales.MessageDownload3(), metrics.Down, history.down, top, shimmer)
			statusRate(c, locales.MessageUpload3(), metrics.Up, history.up, top, shimmer)
		})
	})
}

func statusRate(c *ui.Context, label string, value *float64, samples []float64, top float64, shimmer bool) {
	t := theme.Current()
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(t.Space.N3).Children(func() {
		uikit.Text(c, label, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine()
		if value == nil {
			statusFigure(c, "—", "", t.Font.Size.Title, 0, t.Font.Weight.Semibold, uikit.ToneTertiary)
			statusPlaceholder(c, t.C.Stats.NetChartHeight, shimmer)
			return
		}
		number, unit := monitorRate(*value)
		statusFigure(c, number, " "+unit, t.Font.Size.Title, t.Font.Size.Caption2, t.Font.Weight.Semibold, uikit.ToneNeutral)
		uikit.Sparkline(c, samples, uikit.SparklineProps{
			Height: t.C.Stats.NetChartHeight, Max: top, Slots: monitorSamples, Label: label + locales.MessageTrend(),
		})
	})
}

func statusProcesses(c *ui.Context, metrics hostinfo.Metrics, shimmer bool) {
	t := theme.Current()
	meta := "CPU"
	if metrics.Processes != nil {
		meta = locales.MonitorProcessesCount(*metrics.Processes)
	}
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		statusHeading(c, locales.MessageTopProcesses(), meta)
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
	style, tone := uikit.TextSmall, uikit.ToneTertiary
	if active {
		style, tone = uikit.TextLabel, uikit.ToneNeutral
	}
	row := ui.Row(c).Key(key).FillWidth().Height(t.C.KeyValue.RowHeight).Gap(t.Space.N8).AlignItems(ui.Center)
	shownShare := row.AnimateTo("share", share, uikit.Tween(t.Motion.Duration.Focus))
	row.Draw(func(p *ui.Painter, r ui.Rect) {
		if shownShare <= 0 {
			return
		}
		// The bar reaches into the panel padding so the text keeps the column's edge.
		inset := t.Space.N6
		p.Fill(ui.Rect{X: r.X - inset, Y: r.Y, W: (r.W + 2*inset) * shownShare, H: r.H}, uikit.Color(t.C.Stats.ProcessBar), t.C.Stats.ProcessRadius)
	}).Children(func() {
		uikit.Text(c, name, uikit.TextProps{Style: uikit.TextMono, Tone: tone}).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		uikit.Text(c, value, uikit.TextProps{Style: style, Tone: tone}).FontFeatures("tnum").SingleLine().Shrink(0)
	})
}

// statusFacts shows the host's basic information, which outlives the
// connection; uptime is live and only given while sampling.
func (a *app) statusFacts(
	c *ui.Context,
	open *bool,
	facts *hostinfo.Facts,
	message string,
	uptime *float64,
) {
	t := theme.Current()
	meta := ""
	if uptime != nil {
		meta = locales.MessageUp() + monitorUptime(uptime, true)
	}
	uikit.Disclosure(c, open, uikit.DisclosureProps{
		Title: locales.MessageOperatingSystem(), TitleStyle: uikit.TextLabel, TitleTone: uikit.ToneSecondary, Meta: meta,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func() {
		hostname, kernel, arch := "—", "—", "—"
		if facts != nil {
			hostname, kernel, arch = monitorText(facts.Hostname), monitorText(facts.Kernel), monitorText(facts.Arch)
		}
		ui.Column(c).FillWidth().Gap(t.Space.N10).Children(func() {
			statusFact(c, locales.MessageHostname(), hostname, true)
			statusFact(c, locales.MessageKernel(), kernel, true)
			ui.Row(c).FillWidth().Gap(t.Space.N16).Children(func() {
				statusFact(c, locales.MessageArchitecture(), arch, true).Shrink(0)
				if uptime != nil {
					statusFact(c, locales.MessageUptime2(), monitorUptime(uptime, false), false)
				}
			})
			if facts != nil && facts.Platform == hostinfo.Windows {
				statusFact(c, locales.MessageDefaultShell(), monitorText(facts.Shell), true)
			}
			if message != "" {
				uikit.Text(c, message, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			}
		})
	})
}

// statusFact stacks a label over its value, which wraps rather than being cut.
func statusFact(c *ui.Context, label, value string, mono bool) ui.Element {
	t := theme.Current()
	return ui.Column(c).MinWidth(0).Gap(t.Space.N1).Children(func() {
		statusCaption(c, label)
		style := uikit.TextSmall
		if mono {
			style = uikit.TextMono
		}
		uikit.Text(c, value, uikit.TextProps{Style: style})
	})
}
