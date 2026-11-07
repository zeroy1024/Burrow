package hostinfo

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
)

// Linux reads /proc through POSIX shell builtins, so it needs no procps.

const linuxMountLimit = 131072

type linuxCollector struct{}

func (linuxCollector) Facts(ctx context.Context, r Runner, _ Detection) (Facts, error) {
	return collectLinuxFacts(ctx, r)
}

func (linuxCollector) NewSampler(r Runner) Sampler { return &linuxSampler{r: r} }

// linuxSampler turns cumulative /proc counters into rates against the previous
// successful reading of the same transport.
type linuxSampler struct {
	r        Runner
	previous *linuxSample
}

func (s *linuxSampler) Sample(ctx context.Context) (Metrics, error) {
	output, err := s.r.Exec(ctx, monitorCommand)
	var sample linuxSample
	if err == nil {
		sample, err = parseMonitorSample(string(output))
	}
	if err != nil {
		s.previous = nil
		return Metrics{}, err
	}
	completeMonitorRates(&sample, s.previous)
	s.previous = &sample
	return sample.metrics.export(), nil
}

// Disk uses a separate short-lived SFTP channel. An SFTP server can be
// chrooted independently of the command channel, so its root mount must match
// the sample's before its statistics are labeled as the SSH root disk.
func (s *linuxSampler) Disk(ctx context.Context, key string) (Usage, error) {
	files, err := s.r.Files(ctx)
	if err != nil {
		return Usage{}, err
	}
	defer files.Close()
	usage, err := files.StatFilesystem(ctx, "/")
	if err != nil {
		return Usage{}, err
	}
	mounts, err := files.ReadFile(ctx, "/proc/self/mountinfo", linuxMountLimit)
	if err != nil {
		return Usage{}, err
	}
	if actual := parseMonitorRoot(string(mounts)); actual.identity == "" || actual.identity != key {
		return Usage{}, locales.NewError(locales.MessageTheSftpRootMountDoesNotMatchTheCurrentSshSession)
	}
	return usage, nil
}

type processMetric struct {
	name string
	cpu  float64
}

type linuxMetrics struct {
	cores, processes        int
	hasProcesses            bool
	load                    string
	uptime                  *float64
	cpu, memory             *float64
	memoryUsed, memoryTotal uint64
	diskDevice, filesystem  string
	root                    monitorRoot
	networkInterface        string
	down, up                *float64
	top                     []processMetric
	unavailable             []string
}

func (m linuxMetrics) export() Metrics {
	out := Metrics{CPU: m.cpu, Memory: m.memory, Uptime: m.uptime, Down: m.down, Up: m.up, Cores: m.cores,
		Load: m.load, MemoryUsed: m.memoryUsed, MemoryTotal: m.memoryTotal, DiskKey: m.root.identity,
		DiskDevice: m.diskDevice, Filesystem: m.filesystem, Unavailable: m.unavailable}
	if m.hasProcesses {
		processes := m.processes
		out.Processes = &processes
	}
	for _, p := range m.top {
		out.Top = append(out.Top, Process{Name: p.name, CPU: p.cpu})
	}
	return out
}

type linuxSample struct {
	metrics           linuxMetrics
	cpuTotal, cpuIdle uint64
	hasCPU            bool
	bytesIn, bytesOut uint64
	hasNetwork        bool
	processes         map[monitorProcessID]monitorProcess
}

func monitorNumber(text string) (float64, bool) {
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// The login shell only launches /bin/sh. The collector uses POSIX shell builtins
// exclusively, emits NUL-delimited records, and never evaluates file contents.
// Per-file and process budgets keep an oversized optional section below Exec's
// output limit without discarding the other metrics.
const linuxCollectorPrelude = `LC_ALL=C
export LC_ALL
if ! { IFS= read -r monitor_os < /proc/sys/kernel/ostype; } 2>/dev/null || [ "$monitor_os" != Linux ]; then
  printf 'error\000linux_proc_unavailable\000'
  exit 0
fi
read_file() {
  monitor_body=
  monitor_ok=1
  if ! {
    while IFS= read -r monitor_line || [ -n "$monitor_line" ]; do
      monitor_body="$monitor_body$monitor_line
"
      if [ "${#monitor_body}" -gt "$2" ]; then monitor_ok=0; break; fi
    done < "$1"
  } 2>/dev/null; then monitor_ok=0; fi
  if [ "$monitor_ok" -eq 0 ]; then monitor_body=; fi
}
emit_file() {
  read_file "$2" "$3"
  printf '%s\000%s\000' "$1" "$monitor_body"
}
`

const monitorScript = "printf '__SSH_DESK_MONITOR_V2__\\000'\n" + linuxCollectorPrelude + `
emit_file cpu /proc/stat 65536
emit_file memory /proc/meminfo 16384
emit_file network /proc/net/dev 65536
emit_file route /proc/net/route 65536
emit_file route6 /proc/net/ipv6_route 65536
emit_file uptime /proc/uptime 1024
emit_file load /proc/loadavg 1024
emit_file mounts /proc/self/mountinfo 131072
monitor_count=0
monitor_bytes=0
monitor_top=ok
for monitor_dir in /proc/[0-9]*; do
  [ -d "$monitor_dir" ] || continue
  monitor_count=$((monitor_count + 1))
  [ "$monitor_top" = ok ] || continue
  read_file "$monitor_dir/stat" 4096
  [ -n "$monitor_body" ] || continue
  monitor_id="p/${monitor_dir##*/}"
  monitor_bytes=$((monitor_bytes + ${#monitor_id} + ${#monitor_body} + 2))
  if [ "$monitor_bytes" -gt 524288 ]; then monitor_top=limit; continue; fi
  printf '%s\000%s\000' "$monitor_id" "$monitor_body"
done
printf 'processes\000%s\000top\000%s\000' "$monitor_count" "$monitor_top"
`

func shellCommand(script string) string {
	return "exec /bin/sh -c '" + strings.ReplaceAll(script, "'", "'\\''") + "'"
}

var monitorCommand = shellCommand(monitorScript)

type monitorRoot struct {
	identity, device, filesystem string
}

type monitorProcessID struct {
	pid   int
	start uint64
}

type monitorProcess struct {
	name  string
	ticks uint64
}

func parseMonitorSample(output string) (linuxSample, error) {
	sections, err := parseRecords(output, "__SSH_DESK_MONITOR_V2__")
	if err != nil {
		return linuxSample{}, err
	}
	return parseMonitorSections(sections), nil
}

func parseRecords(output, marker string) (map[string]string, error) {
	parts := strings.Split(output, "\x00")
	if len(parts) < 2 || parts[0] != marker || parts[len(parts)-1] != "" || len(parts)%2 != 0 {
		return nil, locales.NewError(locales.MessageTheServerReturnedUnrecognizedMonitoringData)
	}
	sections := make(map[string]string, len(parts)/2)
	for i := 1; i < len(parts)-1; i += 2 {
		if _, exists := sections[parts[i]]; exists {
			return nil, locales.NewError(locales.MessageTheServerReturnedDuplicateMonitoringRecords)
		}
		sections[parts[i]] = parts[i+1]
	}
	if message := sections["error"]; message != "" {
		switch message {
		case "unsupported_macos":
			return nil, locales.NewError(locales.HostUnsupportedMacos)
		case "linux_proc_unavailable":
			return nil, locales.NewError(locales.HostLinuxProcUnavailable)
		default:
			return nil, errors.New(message)
		}
	}
	return sections, nil
}

func parseMonitorSections(sections map[string]string) linuxSample {
	var sample linuxSample
	metrics := &sample.metrics
	metrics.root = parseMonitorRoot(sections["mounts"])
	metrics.diskDevice, metrics.filesystem = metrics.root.device, metrics.root.filesystem
	parseMonitorCPU(&sample, sections["cpu"])
	parseMonitorMemory(metrics, sections["memory"])
	metrics.networkInterface = monitorRouteInterface(sections["route"], sections["route6"])
	for _, line := range strings.Split(sections["network"], "\n") {
		separator := strings.LastIndexByte(line, ':')
		if separator < 0 || strings.TrimSpace(line[:separator]) != metrics.networkInterface || metrics.networkInterface == "" {
			continue
		}
		fields := strings.Fields(line[separator+1:])
		if len(fields) != 16 {
			continue
		}
		var inErr, outErr error
		sample.bytesIn, inErr = strconv.ParseUint(fields[0], 10, 64)
		sample.bytesOut, outErr = strconv.ParseUint(fields[8], 10, 64)
		sample.hasNetwork = inErr == nil && outErr == nil
	}
	if fields := strings.Fields(sections["uptime"]); len(fields) > 0 {
		if value, ok := monitorNumber(fields[0]); ok {
			metrics.uptime = &value
		}
	}
	if fields := strings.Fields(sections["load"]); len(fields) >= 3 {
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
	if count, err := strconv.Atoi(sections["processes"]); err == nil && count >= 0 {
		metrics.processes, metrics.hasProcesses = count, true
	}
	if sections["top"] == "ok" {
		sample.processes = make(map[monitorProcessID]monitorProcess)
		for key, text := range sections {
			pidText, ok := strings.CutPrefix(key, "p/")
			if !ok {
				continue
			}
			if id, process, ok := parseMonitorProcess(pidText, text); ok {
				sample.processes[id] = process
			}
		}
	}
	for _, item := range []struct {
		name string
		ok   bool
	}{
		{"CPU", sample.hasCPU}, {locales.MessageMemory(), metrics.memory != nil},
		{locales.MessageNetwork(), sample.hasNetwork && metrics.uptime != nil}, {locales.MessageLoad2(), metrics.load != ""},
		{locales.MessageUptime2(), metrics.uptime != nil},
		{locales.MessageProcessCount(), metrics.hasProcesses}, {locales.MessageTopProcesses(), sample.processes != nil && len(sample.processes) > 0 && sample.hasCPU && metrics.cores > 0},
		{locales.MessageMountInformation(), metrics.root.identity != ""},
	} {
		if !item.ok {
			metrics.unavailable = append(metrics.unavailable, item.name)
		}
	}
	return sample
}

func parseMonitorCPU(sample *linuxSample, text string) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "cpu" {
			valid := true
			for index, field := range fields[1:min(len(fields), 9)] {
				value, err := strconv.ParseUint(field, 10, 64)
				if err != nil || value > ^uint64(0)-sample.cpuTotal {
					valid = false
					break
				}
				sample.cpuTotal += value
				if index == 3 || index == 4 {
					sample.cpuIdle += value
				}
			}
			sample.hasCPU = valid
		}
		if len(fields) >= 5 && strings.HasPrefix(fields[0], "cpu") && fields[0] != "cpu" {
			if id, err := strconv.Atoi(strings.TrimPrefix(fields[0], "cpu")); err == nil && id >= 0 {
				sample.metrics.cores++
			}
		}
	}
}

func parseMonitorMemory(metrics *linuxMetrics, text string) {
	memory := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == "kB" {
			if value, err := strconv.ParseUint(fields[1], 10, 64); err == nil && value <= ^uint64(0)/1024 {
				memory[strings.TrimSuffix(fields[0], ":")] = value * 1024
			}
		}
	}
	if available, ok := memory["MemAvailable"]; ok && memory["MemTotal"] > 0 && available <= memory["MemTotal"] {
		metrics.memoryTotal, metrics.memoryUsed = memory["MemTotal"], memory["MemTotal"]-available
		percent := float64(metrics.memoryUsed) / float64(metrics.memoryTotal) * 100
		metrics.memory = &percent
	}
}

func parseMonitorProcess(pidText, text string) (monitorProcessID, monitorProcess, bool) {
	pid, err := strconv.Atoi(pidText)
	start, end := strings.IndexByte(text, '('), strings.LastIndex(text, ") ")
	if err != nil || pid <= 0 || start < 0 || end <= start || strings.TrimSpace(text[:start]) != pidText {
		return monitorProcessID{}, monitorProcess{}, false
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 20 {
		return monitorProcessID{}, monitorProcess{}, false
	}
	user, userErr := strconv.ParseUint(fields[11], 10, 64)
	system, systemErr := strconv.ParseUint(fields[12], 10, 64)
	birth, birthErr := strconv.ParseUint(fields[19], 10, 64)
	if userErr != nil || systemErr != nil || birthErr != nil || system > ^uint64(0)-user {
		return monitorProcessID{}, monitorProcess{}, false
	}
	name := strings.Join(strings.Fields(text[start+1:end]), " ")
	return monitorProcessID{pid: pid, start: birth}, monitorProcess{name: name, ticks: user + system}, name != ""
}

func completeMonitorRates(sample, previous *linuxSample) {
	if previous == nil {
		return
	}
	if sample.hasCPU && previous.hasCPU && sample.metrics.cores == previous.metrics.cores &&
		sample.cpuTotal > previous.cpuTotal && sample.cpuIdle >= previous.cpuIdle {
		total, idle := sample.cpuTotal-previous.cpuTotal, sample.cpuIdle-previous.cpuIdle
		if idle <= total {
			percent := float64(total-idle) / float64(total) * 100
			sample.metrics.cpu = &percent
		}
		if sample.metrics.cores > 0 {
			type ranked struct {
				processMetric
				pid int
			}
			var top []ranked
			for id, process := range sample.processes {
				before, exists := previous.processes[id]
				if !exists || process.ticks < before.ticks {
					continue
				}
				percent := float64(process.ticks-before.ticks) / float64(total) * float64(sample.metrics.cores) * 100
				top = append(top, ranked{processMetric{process.name, percent}, id.pid})
			}
			sort.Slice(top, func(i, j int) bool {
				if top[i].cpu != top[j].cpu {
					return top[i].cpu > top[j].cpu
				}
				return top[i].pid < top[j].pid
			})
			for _, process := range top[:min(4, len(top))] {
				sample.metrics.top = append(sample.metrics.top, process.processMetric)
			}
		}
	}
	if sample.hasNetwork && previous.hasNetwork && sample.metrics.networkInterface == previous.metrics.networkInterface &&
		sample.metrics.uptime != nil && previous.metrics.uptime != nil &&
		sample.bytesIn >= previous.bytesIn && sample.bytesOut >= previous.bytesOut {
		seconds := *sample.metrics.uptime - *previous.metrics.uptime
		if seconds > 0 {
			down, up := float64(sample.bytesIn-previous.bytesIn)/seconds, float64(sample.bytesOut-previous.bytesOut)/seconds
			sample.metrics.down, sample.metrics.up = &down, &up
		}
	}
}

func osReleaseValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		value, ok := strings.CutPrefix(line, key+"=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "'") {
			if len(value) >= 2 && value[len(value)-1] == '\'' {
				return value[1 : len(value)-1]
			}
			return ""
		}
		quoted := strings.HasPrefix(value, "\"")
		if quoted {
			if len(value) < 2 || value[len(value)-1] != '"' {
				return ""
			}
			value = value[1 : len(value)-1]
		}
		var decoded strings.Builder
		for i := 0; i < len(value); i++ {
			if value[i] == '\\' {
				if i+1 == len(value) {
					return ""
				}
				if !quoted || strings.ContainsRune("\\\"$`", rune(value[i+1])) {
					i++
				}
			} else if value[i] == '"' || !quoted && strings.ContainsRune(" \t'", rune(value[i])) {
				return ""
			}
			decoded.WriteByte(value[i])
		}
		return decoded.String()
	}
	return ""
}

func parseMonitorRoot(text string) monitorRoot {
	type mount struct {
		id   string
		root monitorRoot
	}
	var mounts []mount
	parents := map[string]bool{}
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[4] != "/" {
			continue
		}
		separator := -1
		for index := 6; index < len(fields); index++ {
			if fields[index] == "-" {
				separator = index
				break
			}
		}
		if separator < 0 || len(fields) < separator+4 {
			continue
		}
		filesystem, device := fields[separator+1], unescape.Replace(fields[separator+2])
		identity := strings.Join([]string{fields[0], fields[2], unescape.Replace(fields[3]), filesystem, device}, "\x00")
		mounts = append(mounts, mount{fields[0], monitorRoot{identity, device, filesystem}})
		if fields[1] != fields[0] {
			parents[fields[1]] = true
		}
	}
	var result monitorRoot
	for _, mount := range mounts {
		if parents[mount.id] {
			continue
		}
		if result.identity != "" {
			return monitorRoot{}
		}
		result = mount.root
	}
	return result
}

func monitorRouteInterface(ipv4, ipv6 string) string {
	type route struct {
		name   string
		metric uint64
	}
	choose := func(routes []route) string {
		best, name, ambiguous := ^uint64(0), "", false
		for _, candidate := range routes {
			if candidate.metric < best || name == "" {
				best, name, ambiguous = candidate.metric, candidate.name, false
			} else if candidate.metric == best && name != candidate.name {
				ambiguous = true
			}
		}
		if ambiguous {
			return ""
		}
		return name
	}
	var routes []route
	for _, line := range strings.Split(ipv4, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" || fields[0] == "lo" {
			continue
		}
		flags, flagErr := strconv.ParseUint(fields[3], 16, 64)
		metric, metricErr := strconv.ParseUint(fields[6], 10, 64)
		if flagErr == nil && metricErr == nil && flags&1 != 0 && flags&0x200 == 0 {
			routes = append(routes, route{fields[0], metric})
		}
	}
	if len(routes) > 0 {
		return choose(routes)
	}
	for _, line := range strings.Split(ipv6, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[0] != strings.Repeat("0", 32) || fields[1] != "00" ||
			fields[2] != strings.Repeat("0", 32) || fields[3] != "00" || fields[9] == "lo" {
			continue
		}
		flags, flagErr := strconv.ParseUint(fields[8], 16, 64)
		metric, metricErr := strconv.ParseUint(fields[5], 16, 64)
		if flagErr == nil && metricErr == nil && flags&1 != 0 && flags&0x200 == 0 {
			routes = append(routes, route{fields[9], metric})
		}
	}
	return choose(routes)
}

const hostFactsScript = "printf '__BURROW_HOST_FACTS_V1__\\000'\n" + linuxCollectorPrelude + `
emit_file hostname /proc/sys/kernel/hostname 4096
emit_file kernel /proc/sys/kernel/osrelease 4096
emit_file system /etc/os-release 16384
printf 'arch\000%s\000' "$(uname -m 2>/dev/null)"
`

var hostFactsCommand = shellCommand(hostFactsScript)

func collectLinuxFacts(ctx context.Context, r Runner) (Facts, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	collectedAt := time.Now().UTC()
	output, err := r.Exec(ctx, hostFactsCommand)
	if err != nil {
		return Facts{}, err
	}
	sections, err := parseRecords(string(output), "__BURROW_HOST_FACTS_V1__")
	if err != nil {
		return Facts{}, err
	}
	facts := Facts{Platform: Linux,
		Hostname: strings.TrimSpace(sections["hostname"]), Kernel: strings.TrimSpace(sections["kernel"]),
		OSID: osReleaseValue(sections["system"], "ID"), OSVersion: osReleaseValue(sections["system"], "VERSION_ID"),
		OSName: osReleaseValue(sections["system"], "PRETTY_NAME"), Arch: strings.TrimSpace(sections["arch"]),
		CollectedAt: collectedAt,
	}
	if facts.OSName == "" {
		facts.OSName = "Linux"
	}
	if !facts.Valid() {
		return Facts{}, locales.NewError(locales.MessageUnableToReadLinuxHostnameOrKernelVersion)
	}
	return facts, nil
}
