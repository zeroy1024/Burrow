package desktop

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

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

func parseMonitorSample(output string) (monitorSample, error) {
	parts := strings.Split(output, "\x00")
	if len(parts) < 2 || parts[0] != "__SSH_DESK_MONITOR_V2__" || parts[len(parts)-1] != "" || len(parts)%2 != 0 {
		return monitorSample{}, errors.New("服务器返回了无法识别的监控数据")
	}
	sections := make(map[string]string, len(parts)/2)
	for i := 1; i < len(parts)-1; i += 2 {
		if _, exists := sections[parts[i]]; exists {
			return monitorSample{}, errors.New("服务器返回了重复的监控记录")
		}
		sections[parts[i]] = parts[i+1]
	}
	if message := sections["error"]; message != "" {
		return monitorSample{}, errors.New(message)
	}
	var sample monitorSample
	metrics := &sample.metrics
	metrics.hostname, metrics.kernel = strings.TrimSpace(sections["hostname"]), strings.TrimSpace(sections["kernel"])
	metrics.system = monitorOSName(sections["system"])
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
		{"CPU", sample.hasCPU}, {"内存", metrics.memory != nil},
		{"网络", sample.hasNetwork && metrics.uptime != nil}, {"负载", metrics.load != ""},
		{"主机名", metrics.hostname != ""}, {"系统", metrics.system != ""},
		{"内核", metrics.kernel != ""}, {"运行时间", metrics.uptime != nil},
		{"进程数量", metrics.hasProcesses}, {"进程 Top", sample.processes != nil && len(sample.processes) > 0 && sample.hasCPU && metrics.cores > 0},
		{"挂载信息", metrics.root.identity != ""},
	} {
		if !item.ok {
			metrics.unavailable = append(metrics.unavailable, item.name)
		}
	}
	return sample, nil
}

func parseMonitorCPU(sample *monitorSample, text string) {
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

func parseMonitorMemory(metrics *serverMetrics, text string) {
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

func completeMonitorRates(sample, previous *monitorSample) {
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

func monitorOSName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		value, ok := strings.CutPrefix(line, "PRETTY_NAME=")
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

func monitorNotice(m *monitorState) string {
	if m.errorText != "" {
		return "监控采集失败：" + m.errorText
	}
	if m.metrics == nil {
		return "正在采集服务器状态…"
	}
	unavailable := append([]string(nil), m.metrics.unavailable...)
	if m.diskError != "" {
		unavailable = append(unavailable, "磁盘容量")
	}
	if len(unavailable) > 0 {
		return "不可用：" + strings.Join(unavailable, "、")
	}
	return ""
}
