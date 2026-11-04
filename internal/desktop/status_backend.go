package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"sshdesk/internal/fileclient"
	"sshdesk/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const (
	monitorInterval     = 2 * time.Second
	monitorDiskInterval = 10 * time.Second
	monitorMountLimit   = 131072
)

type monitorState struct {
	session         *remoteSession
	connection      *sshclient.Connection
	ctx             context.Context
	cancel          context.CancelFunc
	generation      uint64
	pending         bool
	next            time.Time
	previous        *monitorSample
	metrics         *serverMetrics
	history         []float64
	errorText       string
	updatedAt       time.Time
	diskPending     bool
	diskNext        time.Time
	diskIdentity    string
	diskStats       *fileclient.FilesystemStats
	diskError       string
	diskUnsupported bool
}

type processMetric struct {
	name string
	cpu  float64
}

type serverMetrics struct {
	cores, processes        int
	hasProcesses            bool
	load                    string
	uptime                  *float64
	cpu, memory, disk       *float64
	memoryUsed, memoryTotal uint64
	diskUsed, diskTotal     uint64
	diskDevice, filesystem  string
	root                    monitorRoot
	networkInterface        string
	down, up                *float64
	top                     []processMetric
	unavailable             []string
}

type monitorSample struct {
	metrics           serverMetrics
	cpuTotal, cpuIdle uint64
	hasCPU            bool
	bytesIn, bytesOut uint64
	hasNetwork        bool
	processes         map[monitorProcessID]monitorProcess
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
	a.updateMonitorSample(c)
	a.updateMonitorDisk(c)
}

func (a *app) updateMonitorSample(c *ui.Context) {
	m := &a.monitor
	if m.pending {
		return
	}
	if remaining := m.next.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	connection, generation, lifetime := m.connection, m.generation, m.ctx
	release, err := connection.Retain()
	if err != nil {
		m.metrics, m.previous, m.history = nil, nil, nil
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
		output, err := connection.Exec(ctx, monitorCommand)
		var sample monitorSample
		if err == nil {
			sample, err = parseMonitorSample(string(output))
		}
		a.window.Update(func() {
			if m.generation != generation || m.connection != connection || lifetime.Err() != nil {
				return
			}
			m.pending = false
			m.next = time.Now().Add(monitorInterval)
			if err != nil {
				m.metrics, m.previous, m.history = nil, nil, nil
				m.errorText = err.Error()
				return
			}
			completeMonitorRates(&sample, m.previous)
			m.previous, m.metrics = &sample, &sample.metrics
			m.errorText, m.updatedAt = "", time.Now()
			if sample.metrics.cpu != nil {
				m.history = append(m.history, *sample.metrics.cpu/100)
				if len(m.history) > 30 {
					m.history = append([]float64(nil), m.history[len(m.history)-30:]...)
				}
			}
		})
	})
}

// Filesystem space changes slowly. Its separate, short-lived SFTP channel and
// deadline cannot erase /proc samples or close a browser/transfer subsystem.
func (a *app) updateMonitorDisk(c *ui.Context) {
	m := &a.monitor
	if m.metrics == nil || m.metrics.root.identity == "" {
		m.diskStats = nil
		return
	}
	root := m.metrics.root
	if m.diskIdentity != root.identity {
		m.diskIdentity, m.diskStats, m.diskError = root.identity, nil, ""
		m.diskNext, m.diskUnsupported = time.Time{}, false
	}
	if m.diskPending || m.diskUnsupported {
		return
	}
	if remaining := m.diskNext.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	connection, generation, lifetime := m.connection, m.generation, m.ctx
	ctx, cancel := context.WithTimeout(lifetime, 3*time.Second)
	m.diskPending = true
	a.sshWorkers.Go(func() {
		defer cancel()
		stats, err := readMonitorDisk(ctx, connection, root)
		a.window.Update(func() {
			if m.generation != generation || m.connection != connection || lifetime.Err() != nil {
				return
			}
			m.diskPending = false
			if m.diskIdentity != root.identity {
				return
			}
			m.diskNext = time.Now().Add(monitorDiskInterval)
			if err != nil {
				m.diskStats, m.diskError = nil, err.Error()
				m.diskUnsupported = errors.Is(err, errors.ErrUnsupported)
				return
			}
			m.diskStats, m.diskError = &stats, ""
		})
	})
}

func readMonitorDisk(ctx context.Context, connection *sshclient.Connection, root monitorRoot) (fileclient.FilesystemStats, error) {
	var client fileclient.Client
	for {
		if err := connection.WaitChannelReady(ctx); err != nil {
			return fileclient.FilesystemStats{}, err
		}
		if err := fileclient.WaitSFTPReady(ctx, connection); err != nil {
			return fileclient.FilesystemStats{}, err
		}
		var err error
		client, err = fileclient.NewSFTP(ctx, connection)
		if !errors.Is(err, sshclient.ErrChannelPending) && !errors.Is(err, fileclient.ErrSFTPInitPending) {
			if err != nil {
				return fileclient.FilesystemStats{}, err
			}
			break
		}
	}
	defer client.Close()
	stats, err := client.(fileclient.FilesystemStatter).StatFilesystem(ctx, "/")
	if err != nil {
		return fileclient.FilesystemStats{}, err
	}
	// An SFTP server can be chrooted independently of the SSH command channel.
	// Confirm its root mount before labeling its statistics as the SSH root disk.
	stream, err := client.OpenRead(ctx, "/proc/self/mountinfo", 0)
	if err != nil {
		return fileclient.FilesystemStats{}, err
	}
	mounts, readErr := io.ReadAll(io.LimitReader(stream, monitorMountLimit+1))
	if err := errors.Join(readErr, stream.Close()); err != nil {
		return fileclient.FilesystemStats{}, err
	}
	if len(mounts) > monitorMountLimit {
		return fileclient.FilesystemStats{}, errors.New("SFTP 挂载信息超过采集上限")
	}
	if actual := parseMonitorRoot(string(mounts)); actual.identity == "" || actual.identity != root.identity {
		return fileclient.FilesystemStats{}, errors.New("SFTP 根挂载与当前 SSH 会话不一致")
	}
	return stats, nil
}

func monitorNumber(text string) (float64, bool) {
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
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
