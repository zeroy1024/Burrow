package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sshdesk/internal/hostinfo"
	"sshdesk/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const (
	monitorInterval     = 2 * time.Second
	monitorDiskInterval = 10 * time.Second
)

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
	history         []float64
	errorText       string
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
			if group := session.connection; !group.facts.pending && group.facts.errorText != "" {
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
		m.metrics, m.history = nil, nil
		m.errorText = err.Error()
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
				m.unsupported, m.metrics, m.history = true, nil, nil
				return
			}
			if err != nil {
				m.metrics, m.history = nil, nil
				m.errorText = err.Error()
				return
			}
			m.metrics = &metrics
			m.errorText, m.updatedAt = "", time.Now()
			if metrics.CPU != nil {
				m.history = append(m.history, *metrics.CPU/100)
				if len(m.history) > 30 {
					m.history = append([]float64(nil), m.history[len(m.history)-30:]...)
				}
			}
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

func monitorNotice(m *monitorState) string {
	if m.unsupported {
		return m.platform.DisplayName() + " 暂不支持监控"
	}
	if m.errorText != "" {
		return "监控采集失败：" + m.errorText
	}
	if m.metrics == nil {
		return "正在采集服务器状态…"
	}
	unavailable := append([]string(nil), m.metrics.Unavailable...)
	if m.diskError != "" {
		unavailable = append(unavailable, "磁盘容量")
	}
	if len(unavailable) > 0 {
		return "不可用：" + strings.Join(unavailable, "、")
	}
	return ""
}

func monitorText(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func monitorRate(value *float64) string {
	if value == nil {
		return "—"
	}
	for _, unit := range []struct {
		size float64
		name string
	}{{1e9, "GB/s"}, {1e6, "MB/s"}, {1e3, "KB/s"}} {
		if *value >= unit.size {
			return fmt.Sprintf("%.1f %s", *value/unit.size, unit.name)
		}
	}
	return fmt.Sprintf("%.0f B/s", *value)
}

func monitorUptime(value *float64) string {
	if value == nil {
		return "—"
	}
	minutes := int64(*value) / 60
	return fmt.Sprintf("%d 天 %d 小时 %d 分钟", minutes/1440, minutes/60%24, minutes%60)
}
