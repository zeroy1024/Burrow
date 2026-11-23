package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

const (
	monitorInterval      = 2 * time.Second
	monitorDiskInterval  = 10 * time.Second
	monitorSampleTimeout = 5 * time.Second
	monitorGap           = monitorInterval + monitorSampleTimeout
	monitorWindow        = 2 * time.Minute
)

type monitorHistory struct{ cpu, down, up []uikit.SparklinePoint }

func (h *monitorHistory) add(at time.Time, metrics hostinfo.Metrics) {
	h.cpu = appendMonitorSample(h.cpu, at, metrics.CPU)
	h.down = appendMonitorSample(h.down, at, metrics.Down)
	h.up = appendMonitorSample(h.up, at, metrics.Up)
}

func appendMonitorSample(samples []uikit.SparklinePoint, at time.Time, value *float64) []uikit.SparklinePoint {
	if len(samples) > 0 && at.Before(samples[len(samples)-1].At) {
		samples = nil
	}
	return append(monitorWindowSamples(samples, at), uikit.SparklinePoint{At: at, Value: value})
}

func monitorWindowSamples(samples []uikit.SparklinePoint, now time.Time) []uikit.SparklinePoint {
	start := now.Add(-monitorWindow)
	for len(samples) > 0 && samples[0].At.Before(start) {
		samples = samples[1:]
	}
	return samples
}

func (h monitorHistory) window(now time.Time) monitorHistory {
	return monitorHistory{
		cpu:  monitorWindowSamples(h.cpu, now),
		down: monitorWindowSamples(h.down, now),
		up:   monitorWindowSamples(h.up, now),
	}
}

// The window thread owns the published readings. The worker owns the rate baseline.
type monitorState struct {
	connection      *sshclient.Connection
	terminals       int
	cancel          context.CancelFunc
	ctx             context.Context
	sampler         hostinfo.Sampler
	platform        hostinfo.Platform
	unsupported     bool
	metrics         *hostinfo.Metrics
	history         monitorHistory
	errorText       locales.Text
	updatedAt       time.Time
	diskPending     bool
	diskNext        time.Time
	diskIdentity    string
	diskStats       *hostinfo.Usage
	diskError       string
	diskUnsupported bool
}

// sessionMonitor returns the samples of the session's current transport.
func (a *app) sessionMonitor(session *remoteSession) *monitorState {
	if run := session.ssh; run != nil {
		if m := run.monitor; m != nil && m.connection == session.connection.connection {
			return m
		}
	}
	return nil
}

func (a *app) attachMonitor(group *sshConnectionGroup) *monitorState {
	m := group.monitor
	if m == nil || m.connection != group.connection {
		if m != nil {
			m.cancel()
		}
		ctx, cancel := context.WithCancel(group.ctx)
		m = &monitorState{connection: group.connection, ctx: ctx, cancel: cancel}
		group.monitor = m
	}
	m.terminals++
	a.startMonitor(group)
	return m
}

func (run *sshRuntime) stopMonitor() {
	m := run.monitor
	if m == nil {
		return
	}
	run.monitor = nil
	m.terminals--
	if m.terminals > 0 {
		return
	}
	m.cancel()
	if group := run.request.group; group.monitor == m {
		group.monitor = nil
	}
}

func (a *app) startMonitor(group *sshConnectionGroup) {
	m := group.monitor
	if m == nil || m.sampler != nil {
		return
	}
	if m.connection != group.connection || m.ctx.Err() != nil {
		return
	}
	platform, known := a.groupPlatform(group)
	if !known {
		if !group.facts.pending {
			m.errorText = group.facts.errorText
		}
		return
	}
	m.platform = platform
	m.sampler = hostinfo.For(platform).NewSampler(sshHostRunner{m.connection})
	a.sshWorkers.Go(func() { a.runMonitor(m.ctx, m) })
}

func (a *app) runMonitor(ctx context.Context, m *monitorState) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	var lastSuccess time.Time
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-m.connection.Done():
			return
		case <-timer.C:
		}
		// Use wall time here because the monotonic clock may stop during sleep.
		started := time.Now().Round(0)
		metrics, err := readMonitor(ctx, m)
		at := time.Now().Round(0)
		if err == nil && (at.Sub(started) > monitorSampleTimeout || at.Before(started)) {
			// A suspended request may have read either side of the interruption.
			m.sampler.Reset()
			timer.Reset(0)
			continue
		}
		if err == nil {
			if monitorInterrupted(lastSuccess, at) {
				// This reading becomes the baseline for the next one.
				metrics.CPU, metrics.Down, metrics.Up, metrics.Top = nil, nil, nil, nil
			}
			lastSuccess = at
		}
		a.window.Update(func() { a.publishMonitor(ctx, m, at, metrics, err) })
		if errors.Is(err, hostinfo.ErrUnsupported) {
			return
		}
		timer.Reset(monitorInterval)
	}
}

func monitorInterrupted(last, now time.Time) bool {
	elapsed := now.Sub(last)
	return !last.IsZero() && (elapsed > monitorGap || elapsed < 0)
}

func readMonitor(ctx context.Context, m *monitorState) (hostinfo.Metrics, error) {
	release, err := m.connection.Retain()
	if err != nil {
		return hostinfo.Metrics{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, monitorSampleTimeout)
	defer cancel()
	return m.sampler.Sample(ctx)
}

func (a *app) publishMonitor(ctx context.Context, m *monitorState, at time.Time, metrics hostinfo.Metrics, err error) {
	if ctx.Err() != nil {
		return
	}
	if errors.Is(err, hostinfo.ErrUnsupported) {
		m.unsupported = true
		return
	}
	m.history.add(at, metrics)
	if err != nil {
		m.errorText = locales.ErrorText(err)
		return
	}
	m.metrics = &metrics
	m.errorText, m.updatedAt = locales.Text{}, at
	a.updateMonitorDisk(ctx, m, metrics.DiskKey)
}

func (m *monitorState) isFresh(c *ui.Context) bool {
	if m.metrics == nil || !m.errorText.Empty() {
		return false
	}
	age := c.Now().Round(0).Sub(m.updatedAt)
	if age < 0 || age >= monitorGap {
		return false
	}
	c.After(monitorGap - age)
	return true
}

// Filesystem space changes slowly. Its separate, short-lived channel and
// deadline cannot erase samples or close a browser/transfer subsystem.
func (a *app) updateMonitorDisk(lifetime context.Context, m *monitorState, key string) {
	if m.diskIdentity != key {
		m.diskIdentity, m.diskStats, m.diskError = key, nil, ""
		m.diskNext, m.diskUnsupported = time.Time{}, false
	}
	if key == "" || m.diskPending || m.diskUnsupported || time.Now().Before(m.diskNext) {
		return
	}
	sampler := m.sampler
	ctx, cancel := context.WithTimeout(lifetime, 3*time.Second)
	m.diskPending = true
	a.sshWorkers.Go(func() {
		defer cancel()
		usage, err := sampler.Disk(ctx, key)
		a.window.Update(func() {
			if lifetime.Err() != nil {
				return
			}
			m.diskPending = false
			if m.diskIdentity != key {
				return
			}
			m.diskNext = time.Now().Add(monitorDiskInterval)
			if err != nil {
				m.diskStats, m.diskError = nil, err.Error()
				m.diskUnsupported = errors.Is(err, errors.ErrUnsupported)
				return
			}
			m.diskStats, m.diskError = &usage, ""
		})
	})
}

// monitorNotice explains failed or missing metrics. Collection progress and
// unsupported platforms show in the panel's host indicator instead.
func monitorNotice(m *monitorState) string {
	if !m.errorText.Empty() {
		return locales.MessageMonitoringCollectionFailed() + m.errorText.String()
	}
	if m.metrics == nil {
		return ""
	}
	unavailable := append([]string(nil), m.metrics.Unavailable...)
	if m.diskError != "" {
		unavailable = append(unavailable, locales.MessageDiskCapacity())
	}
	if len(unavailable) > 0 {
		return locales.MessageUnavailable() + strings.Join(unavailable, locales.CommonListSeparator())
	}
	return ""
}

func monitorText(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

// monitorRate splits a byte rate into its number and unit, which the panel
// sets in different sizes.
func monitorRate(value float64) (number, unit string) {
	for _, step := range []struct {
		size float64
		name string
	}{{1e9, "GB/s"}, {1e6, "MB/s"}, {1e3, "KB/s"}} {
		if value >= step.size {
			return fmt.Sprintf("%.1f", value/step.size), step.name
		}
	}
	return fmt.Sprintf("%.0f", value), "B/s"
}

// monitorUptime drops the minutes when brief and the host has been up a day.
func monitorUptime(value *float64, brief bool) string {
	if value == nil {
		return "—"
	}
	minutes := int64(*value) / 60
	days, hours := minutes/1440, minutes/60%24
	if brief && days > 0 {
		return fmt.Sprintf(locales.MessageDH(), days, hours)
	}
	if brief {
		return fmt.Sprintf(locales.MessageHMin(), hours, minutes%60)
	}
	return fmt.Sprintf(locales.MessageDHMin(), days, hours, minutes%60)
}
