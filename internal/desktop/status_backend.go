package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sshdesk/internal/hostinfo"
	"sshdesk/internal/locales"
	"sshdesk/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const (
	monitorInterval     = 2 * time.Second
	monitorDiskInterval = 10 * time.Second
	// monitorSamples is the trend window: about two minutes at monitorInterval.
	monitorSamples = 60
)

// monitorHistory keeps recent rates for the trend charts: CPU in percent,
// network in bytes per second. A sample without a value adds nothing.
type monitorHistory struct{ cpu, down, up []float64 }

func (h *monitorHistory) add(metrics hostinfo.Metrics) {
	h.cpu = appendMonitorSample(h.cpu, metrics.CPU)
	h.down = appendMonitorSample(h.down, metrics.Down)
	h.up = appendMonitorSample(h.up, metrics.Up)
}

func appendMonitorSample(samples []float64, value *float64) []float64 {
	if value == nil {
		return samples
	}
	samples = append(samples, *value)
	if len(samples) > monitorSamples {
		samples = append([]float64(nil), samples[len(samples)-monitorSamples:]...)
	}
	return samples
}

type monitorState struct {
	session    *remoteSession
	connection *sshclient.Connection
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	// sampler belongs to connection and keeps its counter baseline; platform
	// is what it was created for.
	sampler         hostinfo.Sampler
	platform        hostinfo.Platform
	unsupported     bool
	pending         bool
	next            time.Time
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

func (m *monitorState) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	generation := m.generation + 1
	*m = monitorState{generation: generation}
}

// Samples belong to the actual transport. Focusing another Shell on that same
// connection preserves history; hiding the panel cancels only its own channels.
func (a *app) updateMonitor(c *ui.Context) {
	m := &a.monitor
	var session *remoteSession
	if a.statusVisible && a.activeSection() == sectionSSH && a.window != nil && a.window.IsVisible() {
		if tab := a.currentTab(); tab != nil {
			session = tab.focused
		}
	}
	if session == nil || session.connection == nil || session.connection.connection == nil {
		if m.connection != nil {
			m.stop()
		}
		return
	}
	connection := session.connection.connection
	if m.connection != connection {
		m.stop()
		m.ctx, m.cancel = context.WithCancel(a.ctx)
		m.connection = connection
	}
	m.session = session
	// A newly focused split may still be opening its Shell on this transport.
	// Pause new samples without throwing away the existing connection baseline.
	if session.closing || session.state != connected || session.ssh == nil || session.ssh.conn == nil {
		return
	}
	if m.sampler == nil {
		platform, known := a.groupPlatform(session.connection)
		if !known {
			// The first connection to a host waits for platform detection.
			if group := session.connection; !group.facts.pending && !group.facts.errorText.Empty() {
				m.errorText = group.facts.errorText
			}
			return
		}
		m.sampler, m.platform = hostinfo.For(platform).NewSampler(sshHostRunner{connection}), platform
	}
	a.updateMonitorSample(c)
	a.updateMonitorDisk(c)
}

func (a *app) updateMonitorSample(c *ui.Context) {
	m := &a.monitor
	if m.pending || m.unsupported {
		return
	}
	if remaining := m.next.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	connection, sampler, generation, lifetime := m.connection, m.sampler, m.generation, m.ctx
	release, err := connection.Retain()
	if err != nil {
		m.metrics, m.history = nil, monitorHistory{}
		m.errorText = locales.ErrorText(err)
		m.next = c.Now().Add(monitorInterval)
		c.After(monitorInterval)
		return
	}
	ctx, cancel := context.WithTimeout(lifetime, 5*time.Second)
	m.pending = true
	a.sshWorkers.Go(func() {
		defer release()
		defer cancel()
		metrics, err := sampler.Sample(ctx)
		a.window.Update(func() {
			if m.generation != generation || m.connection != connection || lifetime.Err() != nil {
				return
			}
			m.pending = false
			m.next = time.Now().Add(monitorInterval)
			if errors.Is(err, hostinfo.ErrUnsupported) {
				m.unsupported, m.metrics, m.history = true, nil, monitorHistory{}
				return
			}
			if err != nil {
				m.metrics, m.history = nil, monitorHistory{}
				m.errorText = locales.ErrorText(err)
				return
			}
			m.metrics = &metrics
			m.errorText, m.updatedAt = locales.Text{}, time.Now()
			m.history.add(metrics)
		})
	})
}

// Filesystem space changes slowly. Its separate, short-lived channel and
// deadline cannot erase samples or close a browser/transfer subsystem.
func (a *app) updateMonitorDisk(c *ui.Context) {
	m := &a.monitor
	if m.metrics == nil || m.metrics.DiskKey == "" {
		m.diskStats = nil
		return
	}
	key := m.metrics.DiskKey
	if m.diskIdentity != key {
		m.diskIdentity, m.diskStats, m.diskError = key, nil, ""
		m.diskNext, m.diskUnsupported = time.Time{}, false
	}
	if m.diskPending || m.diskUnsupported {
		return
	}
	if remaining := m.diskNext.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	connection, sampler, generation, lifetime := m.connection, m.sampler, m.generation, m.ctx
	ctx, cancel := context.WithTimeout(lifetime, 3*time.Second)
	m.diskPending = true
	a.sshWorkers.Go(func() {
		defer cancel()
		usage, err := sampler.Disk(ctx, key)
		a.window.Update(func() {
			if m.generation != generation || m.connection != connection || lifetime.Err() != nil {
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
