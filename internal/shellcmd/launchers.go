package shellcmd

import (
	"encoding/base64"
	"slices"
	"strings"
	"unicode/utf16"

	"sshdesk/internal/locales"
)

// launcher describes a command that runs another command given in its
// arguments: sudo rm, timeout 10 rm, ssh host rm.
type launcher struct {
	values  []string // options consuming the next argument (or =value / attached)
	flags   []string // options known to take no value
	scripts []string // options whose value is a shell command string
	skip    []string // bare words that are options, such as env's "-"
	noRun   []string // options under which nothing is executed (command -v)
	// operands counts the arguments between the options and the command, such
	// as timeout's DURATION or ssh's destination.
	operands int
	assigns  bool // NAME=value words precede the command
	join     bool // the remaining words also form one command string
	stdin    bool // without a command, a shell reads the script from stdin
	noStart  bool // remaining words are files, not a command
}

var launchers = map[string]*launcher{
	"sudo": {
		values: []string{"-u", "-g", "-h", "-p", "-C", "-D", "-r", "-t", "-T", "-U", "-R",
			"--user", "--group", "--host", "--prompt", "--close-from", "--chdir", "--role", "--type",
			"--command-timeout", "--other-user", "--chroot"},
		flags: []string{"-A", "-b", "-B", "-E", "-e", "-H", "-i", "-K", "-k", "-l", "-N", "-n", "-P", "-S", "-s", "-V", "-v",
			"--askpass", "--background", "--bell", "--preserve-env", "--edit", "--set-home", "--login",
			"--remove-timestamp", "--reset-timestamp", "--list", "--non-interactive", "--no-update",
			"--preserve-groups", "--stdin", "--shell", "--version", "--validate"},
		assigns: true, stdin: true,
	},
	"doas":   {values: []string{"-u", "-C"}, flags: []string{"-L", "-n", "-s"}, stdin: true},
	"pkexec": {values: []string{"--user"}, flags: []string{"--disable-internal-agent", "--keep-cwd"}},
	"env": {
		values:  []string{"-u", "--unset", "-C", "--chdir", "-P"},
		scripts: []string{"-S", "--split-string"},
		flags:   []string{"-0", "-i", "-v", "--debug", "--ignore-environment", "--null"},
		skip:    []string{"-"}, assigns: true,
	},
	"nice":   {values: []string{"-n", "--adjustment"}},
	"ionice": {values: []string{"-c", "--class", "-n", "--classdata", "-p", "--pid", "-P", "--pgid", "-u", "--uid"}, flags: []string{"-t", "--ignore"}},
	"nohup":  {},
	"time": {
		values: []string{"-f", "--format", "-o", "--output"},
		flags:  []string{"-a", "-p", "-q", "-v", "--append", "--portability", "--quiet", "--verbose"},
	},
	"timeout": {
		values:   []string{"-s", "--signal", "-k", "--kill-after"},
		flags:    []string{"-v", "--foreground", "--preserve-status", "--verbose"},
		operands: 1,
	},
	"stdbuf":  {values: []string{"-i", "-o", "-e", "--input", "--output", "--error"}},
	"chrt":    {values: []string{"-T", "-P", "-D"}, flags: []string{"-a", "-b", "-d", "-e", "-f", "-i", "-m", "-o", "-R", "-r", "-v"}, operands: 1},
	"taskset": {flags: []string{"-a", "-c", "--all-tasks", "--cpu-list"}, operands: 1},
	"setsid":  {flags: []string{"-c", "-f", "-w", "--ctty", "--fork", "--wait"}},
	"flock": {
		values:  []string{"-w", "--timeout", "-E", "--conflict-exit-code"},
		scripts: []string{"-c", "--command"},
		flags: []string{"-e", "-F", "-n", "-o", "-s", "-u", "-x", "--close", "--exclusive", "--no-fork",
			"--nonblock", "--shared", "--unlock", "--verbose"},
		operands: 1,
	},
	"xargs": {
		values: []string{"-a", "-d", "-E", "-I", "-L", "-n", "-P", "-s", "--arg-file", "--delimiter",
			"--max-args", "--max-chars", "--max-lines", "--max-procs", "--process-slot-var"},
		flags: []string{"-0", "-e", "-i", "-l", "-o", "-p", "-r", "-t", "-x", "--exit", "--interactive",
			"--no-run-if-empty", "--null", "--open-tty", "--verbose"},
	},
	"exec":    {values: []string{"-a"}, flags: []string{"-c", "-l"}},
	"command": {flags: []string{"-p"}, noRun: []string{"-v", "-V"}},
	"builtin": {},
	"chroot":  {values: []string{"--userspec", "--groups"}, flags: []string{"--skip-chdir"}, operands: 1},
	"nsenter": {
		values: []string{"-t", "--target", "-S", "--setuid", "-G", "--setgid"},
		flags: []string{"-a", "-C", "-F", "-i", "-m", "-n", "-p", "-T", "-U", "-u", "--all", "--no-fork",
			"--preserve-credentials"},
	},
	"unshare": {
		values: []string{"-S", "--setuid", "-G", "--setgid", "-R", "--root", "-w", "--wd", "--map-user", "--map-group"},
		flags: []string{"-C", "-c", "-f", "-i", "-m", "-n", "-p", "-r", "-T", "-U", "-u", "--fork",
			"--map-current-user", "--map-root-user", "--mount-proc"},
	},
	"runuser": {
		values:  []string{"-u", "--user", "-g", "--group", "-G", "--supp-group", "-s", "--shell", "-w", "--whitelist-environment"},
		scripts: []string{"-c", "--command", "--session-command"},
		flags:   []string{"-l", "-m", "-P", "-p", "--login", "--preserve-environment", "--pty"},
		skip:    []string{"-"},
	},
	"su": {
		values:  []string{"-g", "--group", "-G", "--supp-group", "-s", "--shell", "-w", "--whitelist-environment"},
		scripts: []string{"-c", "--command", "--session-command"},
		flags:   []string{"-f", "-l", "-m", "-P", "-p", "--fast", "--login", "--preserve-environment", "--pty"},
		skip:    []string{"-"}, operands: 1, stdin: true, noStart: true,
	},
	"systemd-run": {
		values: []string{"-p", "--property", "-u", "--unit", "--description", "-E", "--setenv", "--uid", "--gid",
			"-M", "--machine", "-H", "--host", "--working-directory", "--slice", "--nice", "--on-active", "--on-calendar"},
		flags: []string{"-d", "-G", "-P", "-q", "-r", "-S", "-t", "--collect", "--no-block", "--pipe", "--pty", "--quiet",
			"--remain-after-exit", "--same-dir", "--scope", "--shell", "--system", "--user", "--wait"},
	},
	"script": {
		values:  []string{"-T", "--log-timing", "-I", "--log-in", "-O", "--log-out", "-B", "--log-io", "-E", "--echo", "-m", "--logging-format"},
		scripts: []string{"-c", "--command"},
		flags:   []string{"-a", "-e", "-f", "-q", "--append", "--flush", "--quiet", "--return"},
		noStart: true,
	},
	"caffeinate": {values: []string{"-t", "-w"}, flags: []string{"-d", "-i", "-m", "-s", "-u"}},
	"strace": {
		values: []string{"-a", "-b", "-E", "-e", "-I", "-O", "-o", "-P", "-p", "-S", "-s", "-u", "-X"},
		flags: []string{"-C", "-c", "-D", "-d", "-F", "-f", "-h", "-i", "-k", "-q", "-r", "-T", "-t",
			"-V", "-v", "-w", "-x", "-y", "-Z", "-z"},
	},
	"watch": {
		values: []string{"-n", "--interval", "-q", "--equexit"},
		flags: []string{"-b", "-c", "-d", "-e", "-g", "-p", "-r", "-t", "-w", "-x", "--beep", "--chgexit", "--color",
			"--differences", "--errexit", "--exec", "--no-linewrap", "--no-rerun", "--no-title", "--precise"},
		join: true,
	},
	"ssh": {
		values: []string{"-B", "-b", "-c", "-D", "-E", "-e", "-F", "-I", "-i", "-J", "-L", "-l", "-m", "-O",
			"-o", "-P", "-p", "-Q", "-R", "-S", "-W", "-w"},
		flags: []string{"-4", "-6", "-A", "-a", "-C", "-f", "-G", "-g", "-K", "-k", "-M", "-N", "-n", "-q",
			"-s", "-T", "-t", "-V", "-v", "-X", "-x", "-Y", "-y"},
		operands: 1, join: true, stdin: true,
	},
	"busybox": {}, "toybox": {},
}

func init() {
	launchers["gtimeout"], launchers["ltrace"] = launchers["timeout"], launchers["strace"]
}

// Container exec/run take a container or image operand, then the command.
var containerRun = &launcher{
	values: []string{"-a", "--attach", "--add-host", "--cap-add", "--cap-drop", "--cgroupns", "--cidfile", "--cpus",
		"--detach-keys", "--device", "--dns", "-e", "--env", "--env-file", "--entrypoint", "--gpus", "-h",
		"--hostname", "--ipc", "-l", "--label", "--label-file", "--log-driver", "--log-opt", "-m", "--memory",
		"--mount", "--name", "--network", "--net", "-p", "--publish", "--pid", "--platform", "--pull",
		"--restart", "--runtime", "--security-opt", "--shm-size", "--stop-signal", "--stop-timeout", "--tmpfs",
		"-u", "--user", "--ulimit", "-v", "--volume", "--volumes-from", "-w", "--workdir"},
	flags: []string{"-d", "-i", "-P", "-q", "-t", "--detach", "--init", "--interactive", "--no-healthcheck",
		"--privileged", "--publish-all", "--quiet", "--read-only", "--rm", "--tty"},
	operands: 1,
}

var shells = []string{"sh", "bash", "zsh", "dash", "ksh", "ash", "mksh", "fish"}

func (a *analyzer) launch(name string, args []Arg, f frame) {
	switch {
	case slices.Contains(shells, name):
		a.shell(name, args, f)
	case name == "eval":
		joined, ok := joinArgs(args[1:])
		a.nested(Arg{Value: joined, Dynamic: !ok}, locales.MessageEvalArgumentsContainDynamicContent(), f.through("eval"))
	case name == "find":
		a.find(args, f)
	case name == "cmd":
		a.cmd(args, f)
	case name == "powershell" || name == "pwsh":
		a.powershell(name, args, f)
	case name == "docker" || name == "podman" || name == "nerdctl":
		a.container(name, args, f)
	case name == "kubectl" || name == "oc":
		a.kubectl(name, args, f)
	default:
		if l := launchers[name]; l != nil {
			a.wrap(l, name, args, 1, f)
		}
	}
}

func (a *analyzer) wrap(l *launcher, label string, args []Arg, from int, f frame) {
	var starts []int
	var scripts []Arg
	if !l.resolve(args, from, &starts, &scripts) {
		return
	}
	f = f.through(label)
	for _, s := range scripts {
		a.nested(s, label+locales.MessageCommandStringContainsDynamicContent(), f)
	}
	for _, start := range starts {
		a.invoke(args[start:], f)
		if l.join {
			joined, ok := joinArgs(args[start:])
			a.nested(Arg{Value: joined, Dynamic: !ok}, label+locales.MessageCommandContainsDynamicContent(), f)
		}
	}
	if l.stdin && len(starts) == 0 && len(scripts) == 0 {
		a.stdinScripts(f)
	}
}

type scanState struct {
	i, operands int
	optionsDone bool
}

// resolve finds every index where the wrapped command may begin. An unknown
// option might consume the next argument, so both readings are explored;
// matching either one only broadens a denial. It reports false when an option
// such as command -v means nothing runs.
func (l *launcher) resolve(args []Arg, from int, starts *[]int, scripts *[]Arg) bool {
	visited := map[scanState]bool{}
	runs := true
	var scan func(scanState)
	scan = func(s scanState) {
		for ; s.i < len(args); s.i++ {
			if visited[s] {
				return
			}
			visited[s] = true
			arg := args[s.i]
			v := arg.Value
			if !arg.Dynamic && !s.optionsDone && slices.Contains(l.skip, v) {
				continue
			}
			if !arg.Dynamic && !s.optionsDone && len(v) > 1 && v[0] == '-' {
				if v == "--" {
					s.optionsDone = true
					continue
				}
				if slices.Contains(l.noRun, v) {
					runs = false
					return
				}
				next, known := l.option(v, args, s.i, scripts)
				if !known {
					alternative := s
					alternative.i += 2
					scan(alternative)
					continue
				}
				if next {
					s.i++
				}
				continue
			}
			if !arg.Dynamic && l.assigns && isAssignment(v) {
				continue
			}
			if s.operands > 0 {
				s.operands--
				continue
			}
			if !l.noStart && !slices.Contains(*starts, s.i) {
				*starts = append(*starts, s.i)
			}
			return
		}
	}
	scan(scanState{i: from, operands: l.operands})
	return runs
}

// option classifies one option word, recording any command string it carries.
// next reports that the following argument is the option's value.
func (l *launcher) option(v string, args []Arg, i int, scripts *[]Arg) (next, known bool) {
	value := func(text string) {
		*scripts = append(*scripts, Arg{Value: text, raw: text})
	}
	following := func() {
		if i+1 < len(args) {
			*scripts = append(*scripts, args[i+1])
		}
	}
	if strings.HasPrefix(v, "--") {
		name, attached, hasValue := strings.Cut(v, "=")
		switch {
		case slices.Contains(l.scripts, name):
			if hasValue {
				value(attached)
				return false, true
			}
			following()
			return true, true
		case slices.Contains(l.values, name):
			return !hasValue, true
		case slices.Contains(l.flags, name), hasValue:
			return false, true
		}
		return false, false
	}
	// Short options cluster (-it) or carry an attached value (-uroot).
	for j := 1; j < len(v); j++ {
		opt, rest := "-"+v[j:j+1], v[j+1:]
		switch {
		case slices.Contains(l.scripts, opt):
			if rest != "" {
				value(rest)
				return false, true
			}
			following()
			return true, true
		case slices.Contains(l.values, opt):
			return rest == "", true
		case !slices.Contains(l.flags, opt):
			return false, false
		}
	}
	return false, true
}

func isAssignment(v string) bool {
	name, _, ok := strings.Cut(v, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func joinArgs(args []Arg) (string, bool) {
	values := make([]string, len(args))
	for i, arg := range args {
		if arg.Dynamic {
			return "", false
		}
		values[i] = arg.Value
	}
	return strings.Join(values, " "), true
}

// shell handles sh -c STRING, a script file operand (not inspected), and a
// script read from stdin.
func (a *analyzer) shell(name string, args []Arg, f frame) {
	command, readsStdin, optionsDone := false, false, false
	var operands []Arg
	for i := 1; i < len(args); i++ {
		arg := args[i]
		v := arg.Value
		if arg.Dynamic || optionsDone || len(operands) > 0 || len(v) < 2 || v[0] != '-' && v[0] != '+' {
			operands = append(operands, arg)
			continue
		}
		switch {
		case v == "--":
			optionsDone = true
		case v == "-o" || v == "+o" || v == "-O" || v == "+O" || v == "--rcfile" || v == "--init-file":
			i++
		case v == "--command":
			command = true
		case strings.HasPrefix(v, "--command="):
			a.nested(Arg{Value: v[len("--command="):]}, "", f.through(name+" -c"))
			return
		case strings.HasPrefix(v, "--"):
		case v[0] == '-':
			command = command || strings.ContainsRune(v[1:], 'c')
			readsStdin = readsStdin || strings.ContainsRune(v[1:], 's')
		}
	}
	switch {
	case command && len(operands) > 0:
		a.nested(operands[0], name+locales.MessageCCommandStringContainsDynamicContent(), f.through(name+" -c"))
	case len(operands) == 0 || readsStdin:
		a.stdinScripts(f.through(name))
	}
}

func (a *analyzer) find(args []Arg, f frame) {
	for i := 1; i < len(args); i++ {
		v := args[i].Value
		if args[i].Dynamic || v != "-exec" && v != "-execdir" && v != "-ok" && v != "-okdir" {
			continue
		}
		end := i + 1
		for end < len(args) && (args[end].Dynamic || args[end].Value != ";" && args[end].Value != "+") {
			end++
		}
		a.invoke(args[i+1:end], f.through("find "+v))
		i = end
	}
}

func (a *analyzer) kubectl(name string, args []Arg, f frame) {
	exec := slices.IndexFunc(args, func(arg Arg) bool { return !arg.Dynamic && arg.Value == "exec" })
	if exec < 0 {
		return
	}
	for j := exec + 1; j < len(args); j++ {
		if !args[j].Dynamic && args[j].Value == "--" {
			a.invoke(args[j+1:], f.through(name+" exec"))
			return
		}
	}
}

// cmd handles cmd.exe /c and /k, whose remaining words form one command line.
func (a *analyzer) cmd(args []Arg, f frame) {
	for i := 1; i < len(args); i++ {
		v := strings.ToLower(args[i].Value)
		if args[i].Dynamic || !strings.HasPrefix(v, "/c") && !strings.HasPrefix(v, "/k") {
			continue
		}
		rest := args[i+1:]
		if len(v) > 2 {
			rest = append([]Arg{{Value: args[i].Value[2:]}}, rest...)
		}
		joined, ok := joinArgs(rest)
		a.nested(Arg{Value: joined, Dynamic: !ok}, locales.MessageCmdCCommandContainsDynamicContent(), f.through("cmd /c"))
		return
	}
}

// powershell handles -Command, -EncodedCommand and the implicit command of
// trailing words. PowerShell accepts any unambiguous parameter prefix.
func (a *analyzer) powershell(name string, args []Arg, f frame) {
	f = f.through(name)
	for i := 1; i < len(args); i++ {
		if args[i].Dynamic {
			a.uncertain(f.script, name+locales.MessageArgumentsContainDynamicContent(), f.via)
			return
		}
		v := strings.ToLower(args[i].Value)
		switch {
		case !strings.HasPrefix(v, "-"):
			a.psCommand(args[i:], f)
			return
		case v == "-c" || len(v) >= 4 && strings.HasPrefix("-command", v):
			a.psCommand(args[i+1:], f)
			return
		case v == "-e" || v == "-ec" || len(v) >= 4 && strings.HasPrefix("-encodedcommand", v):
			decoded, ok := "", false
			if i+1 < len(args) && !args[i+1].Dynamic {
				decoded, ok = decodeUTF16(args[i+1].Value)
			}
			a.nested(Arg{Value: decoded, Dynamic: !ok}, name+locales.MessageEncodedCommandCannotBeDecoded(), f)
			return
		case v == "-f" || len(v) >= 3 && strings.HasPrefix("-file", v):
			return
		case slices.Contains([]string{"-ep", "-ex", "-executionpolicy", "-w", "-windowstyle", "-of", "-outputformat",
			"-if", "-inputformat", "-wd", "-workingdirectory", "-config", "-configurationname", "-version", "-v",
			"-psconsolefile", "-settingsfile", "-custompipename"}, v):
			i++
		}
	}
}

func (a *analyzer) psCommand(args []Arg, f frame) {
	if len(args) == 1 && !args[0].Dynamic && args[0].Value == "-" {
		a.stdinScripts(f)
		return
	}
	joined, ok := joinArgs(args)
	a.nested(Arg{Value: joined, Dynamic: !ok}, locales.MessagePowershellCommandContainsDynamicContent(), f)
}

func decodeUTF16(text string) (string, bool) {
	data, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(data)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
	}
	return string(utf16.Decode(units)), true
}

// container handles docker/podman exec and run, including compose and the
// container management group; other subcommands (docker rm) run nothing.
func (a *analyzer) container(name string, args []Arg, f frame) {
	globals := []string{"-H", "--host", "-c", "--context", "--config", "-l", "--log-level", "--url", "--connection"}
	composeValues := []string{"-f", "--file", "-p", "--project-name", "--env-file", "--profile", "--project-directory"}
	label := name
	for i := 1; i < len(args); i++ {
		if args[i].Dynamic {
			return
		}
		v := args[i].Value
		switch {
		case slices.Contains(globals, v) || label == name+" compose" && slices.Contains(composeValues, v):
			i++
		case strings.HasPrefix(v, "-"):
		case label == name && (v == "compose" || v == "container"):
			label += " " + v
		case v == "exec" || v == "run":
			a.wrap(containerRun, label+" "+v, args, i+1, f)
			return
		default:
			return
		}
	}
}
