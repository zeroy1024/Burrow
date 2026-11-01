package desktop

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"sshdesk/internal/sshclient"

	"github.com/egoist/mygo/ui"
)

const monitorInterval = 2 * time.Second

// Only fixed, read-only commands run remotely. Optional sections report their
// absence separately, so one unavailable utility does not fabricate a metric.
const monitorCommand = `export LC_ALL=C
printf '__SSH_DESK_MONITOR_V1__\n'
section() { printf '\n__SSH_DESK_SECTION__%s\n' "$1"; }
if [ "$(uname -s 2>/dev/null)" != Linux ]; then
  section error
  printf '此服务器不支持 Linux /proc 指标采集\n'
  exit 0
fi
read_section() {
  section "$1"
  if ! cat "$2" 2>/dev/null; then printf '__UNAVAILABLE__\n'; fi
}
read_section cpu /proc/stat
read_section memory /proc/meminfo
read_section network /proc/net/dev
read_section uptime /proc/uptime
read_section load /proc/loadavg
read_section system /etc/os-release
section hostname
hostname 2>/dev/null || printf '__UNAVAILABLE__\n'
section kernel
uname -r 2>/dev/null || printf '__UNAVAILABLE__\n'
section disk
df -P -k / 2>/dev/null || printf '__UNAVAILABLE__\n'
section filesystem
stat -f -c %T / 2>/dev/null || printf '__UNAVAILABLE__\n'
section processes
set -- /proc/[0-9]*
if [ -d "$1" ]; then printf '%s\n' "$#"; else printf '__UNAVAILABLE__\n'; fi
section top
if processes=$(ps -eo pid=,comm=,pcpu= --sort=-pcpu 2>/dev/null); then
  printf '%s\n' "$processes" | head -n 4
else
  printf '__UNAVAILABLE__\n'
fi
`

type monitorState struct {
	session    *remoteSession
	runtime    *sshRuntime
	connection *sshclient.Connection
	cancel     context.CancelFunc
	generation uint64
	pending    bool
	next       time.Time
	previous   *monitorSample
	metrics    *serverMetrics
	history    []float64
	errorText  string
	updatedAt  time.Time
}

type processMetric struct {
	name string
	cpu  float64
}

type serverMetrics struct {
	hostname, system, kernel string
	cores, processes         int
	load                     string
	uptime                   *float64
	cpu, memory, disk        *float64
	memoryUsed, memoryTotal  uint64
	diskUsed, diskTotal      uint64
	diskDevice, filesystem   string
	down, up                 *float64
	top                      []processMetric
}

type monitorSample struct {
	metrics    serverMetrics
	cpuTotal   uint64
	cpuIdle    uint64
	hasCPU     bool
	bytesIn    uint64
	bytesOut   uint64
	hasNetwork bool
}

func (m *monitorState) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	generation := m.generation + 1
	*m = monitorState{generation: generation}
}

// updateMonitor owns one in-flight request. UI timers schedule the next sample;
// there is no background polling loop after the panel or its session disappears.
func (a *app) updateMonitor(c *ui.Context) {
	m := &a.monitor
	var session *remoteSession
	if a.statusVisible && a.activeSection() == sectionSSH {
		if tab := a.currentTab(); tab != nil {
			session = tab.focused
		}
	}
	if session == nil || session.closing || session.state != connected || session.ssh == nil ||
		session.ssh.conn == nil || session.connection == nil || session.connection.connection == nil {
		if m.session != nil {
			m.stop()
		}
		return
	}
	run, connection := session.ssh, session.connection.connection
	if m.session != session || m.runtime != run || m.connection != connection {
		m.stop()
		m.session, m.runtime, m.connection = session, run, connection
	}
	if m.pending {
		return
	}
	if remaining := m.next.Sub(c.Now()); remaining > 0 {
		c.After(remaining)
		return
	}
	release, err := connection.Retain()
	if err != nil {
		m.metrics, m.previous, m.errorText = nil, nil, err.Error()
		m.next = c.Now().Add(monitorInterval)
		c.After(monitorInterval)
		return
	}
	ctx, cancel := context.WithTimeout(run.ctx, 5*time.Second)
	m.cancel, m.pending = cancel, true
	generation := m.generation
	a.sshWorkers.Go(func() {
		defer release()
		defer cancel()
		started := time.Now()
		output, err := connection.Exec(ctx, monitorCommand)
		elapsed := time.Since(started)
		var sample monitorSample
		if err == nil {
			sample, err = parseMonitorSample(string(output))
		}
		a.window.Update(func() {
			if m.generation != generation || m.session != session || session.ssh != run || run.ctx.Err() != nil {
				return
			}
			m.pending, m.cancel = false, nil
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
			session.endpoint.latency = int(elapsed.Milliseconds())
		})
	})
}

func parseMonitorSample(output string) (monitorSample, error) {
	var sample monitorSample
	lines := strings.Split(output, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "__SSH_DESK_MONITOR_V1__" {
		return sample, errors.New("服务器返回了无法识别的监控数据")
	}
	sections := map[string][]string{}
	section := ""
	for _, line := range lines[1:] {
		if next, ok := strings.CutPrefix(line, "__SSH_DESK_SECTION__"); ok {
			section = strings.TrimSpace(next)
		} else if section != "" && strings.TrimSpace(line) != "" {
			sections[section] = append(sections[section], strings.TrimSpace(line))
		}
	}
	if lines := sections["error"]; len(lines) > 0 {
		return sample, errors.New(strings.Join(lines, " "))
	}
	metrics := &sample.metrics
	metrics.hostname = monitorSectionText(sections["hostname"])
	metrics.kernel = monitorSectionText(sections["kernel"])
	metrics.filesystem = monitorSectionText(sections["filesystem"])
	for _, line := range sections["system"] {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			if decoded, err := strconv.Unquote(value); err == nil {
				value = decoded
			}
			metrics.system = strings.Trim(value, "'")
		}
	}
	for _, line := range sections["cpu"] {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "cpu" {
			valid := true
			for i, field := range fields[1:min(len(fields), 9)] {
				value, err := strconv.ParseUint(field, 10, 64)
				if err != nil {
					valid = false
					break
				}
				sample.cpuTotal += value
				if i == 3 || i == 4 {
					sample.cpuIdle += value
				}
			}
			sample.hasCPU = valid
		}
		if len(fields) > 0 && strings.HasPrefix(fields[0], "cpu") && fields[0] != "cpu" {
			if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err == nil {
				metrics.cores++
			}
		}
	}
	memory := map[string]uint64{}
	for _, line := range sections["memory"] {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == "kB" {
			if value, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				memory[strings.TrimSuffix(fields[0], ":")] = value * 1024
			}
		}
	}
	if available, ok := memory["MemAvailable"]; ok && memory["MemTotal"] > 0 && available <= memory["MemTotal"] {
		metrics.memoryTotal, metrics.memoryUsed = memory["MemTotal"], memory["MemTotal"]-available
		percent := float64(metrics.memoryUsed) / float64(metrics.memoryTotal) * 100
		metrics.memory = &percent
	}
	for _, line := range sections["network"] {
		name, values, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) != 16 {
			continue
		}
		received, inErr := strconv.ParseUint(fields[0], 10, 64)
		transmitted, outErr := strconv.ParseUint(fields[8], 10, 64)
		if inErr == nil && outErr == nil {
			sample.bytesIn += received
			sample.bytesOut += transmitted
			sample.hasNetwork = true
		}
	}
	if fields := strings.Fields(monitorSectionText(sections["uptime"])); len(fields) >= 1 {
		if value, ok := monitorNumber(fields[0]); ok {
			metrics.uptime = &value
		}
	}
	if fields := strings.Fields(monitorSectionText(sections["load"])); len(fields) >= 3 {
		valid := true
		for _, field := range fields[:3] {
			if _, ok := monitorNumber(field); !ok {
				valid = false
			}
		}
		if valid {
			metrics.load = strings.Join(fields[:3], " / ")
		}
	}
	for _, line := range sections["disk"] {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[len(fields)-1] != "/" {
			continue
		}
		total, totalErr := strconv.ParseUint(fields[1], 10, 64)
		used, usedErr := strconv.ParseUint(fields[2], 10, 64)
		percent, validPercent := monitorNumber(strings.TrimSuffix(fields[4], "%"))
		if totalErr == nil && usedErr == nil && validPercent && total > 0 && used <= total && percent <= 100 {
			metrics.diskTotal, metrics.diskUsed, metrics.diskDevice = total*1024, used*1024, fields[0]
			metrics.disk = &percent
		}
	}
	if count, err := strconv.Atoi(monitorSectionText(sections["processes"])); err == nil && count > 0 {
		metrics.processes = count
	}
	for _, line := range sections["top"] {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		if cpu, ok := monitorNumber(fields[len(fields)-1]); ok {
			metrics.top = append(metrics.top, processMetric{name: strings.Join(fields[1:len(fields)-1], " "), cpu: cpu})
		}
	}
	return sample, nil
}

func monitorNumber(text string) (float64, bool) {
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func completeMonitorRates(sample, previous *monitorSample) {
	if previous == nil {
		return
	}
	if sample.hasCPU && previous.hasCPU && sample.cpuTotal > previous.cpuTotal && sample.cpuIdle >= previous.cpuIdle {
		total, idle := sample.cpuTotal-previous.cpuTotal, sample.cpuIdle-previous.cpuIdle
		if idle <= total {
			percent := float64(total-idle) / float64(total) * 100
			sample.metrics.cpu = &percent
		}
	}
	if sample.hasNetwork && previous.hasNetwork && sample.metrics.uptime != nil && previous.metrics.uptime != nil &&
		sample.bytesIn >= previous.bytesIn && sample.bytesOut >= previous.bytesOut {
		seconds := *sample.metrics.uptime - *previous.metrics.uptime
		if seconds > 0 {
			down, up := float64(sample.bytesIn-previous.bytesIn)/seconds, float64(sample.bytesOut-previous.bytesOut)/seconds
			sample.metrics.down, sample.metrics.up = &down, &up
		}
	}
}

func monitorSectionText(lines []string) string {
	if len(lines) == 0 || lines[0] == "__UNAVAILABLE__" {
		return ""
	}
	return strings.Join(lines, " ")
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
