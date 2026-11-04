package desktop

import "strings"

// The login shell only launches /bin/sh. The collector uses POSIX shell builtins
// exclusively, emits NUL-delimited records, and never evaluates file contents.
// Per-file and process budgets keep an oversized optional section below Exec's
// output limit without discarding the other metrics.
const linuxCollectorPrelude = `LC_ALL=C
export LC_ALL
if ! { IFS= read -r monitor_os < /proc/sys/kernel/ostype; } 2>/dev/null || [ "$monitor_os" != Linux ]; then
  printf 'error\000Linux /proc 不可用\000'
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

func linuxCollectorCommand(script string) string {
	return "exec /bin/sh -c '" + strings.ReplaceAll(script, "'", "'\\''") + "'"
}

var monitorCommand = linuxCollectorCommand(monitorScript)
