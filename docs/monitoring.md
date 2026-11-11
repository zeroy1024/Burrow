# Server monitoring

[English](monitoring.md) | [简体中文](monitoring.zh.md)

Burrow collects server information over each SSH connection that has a connected terminal. Sampling starts once the terminal is connected and continues whether or not the panel is open, so opening the panel shows the trend already collected. Samples are discarded when that connection ends; monitoring requests never close Shells or transfers.

## Remote platforms

Platform-specific collectors provide a shared host-information and metrics model. Linux, macOS, and Windows provide basic host information; only Linux currently provides monitoring metrics. Other platforms show that monitoring is unsupported without running Linux scripts or repeatedly reporting errors.

- Automatic detection: an SSH server ID containing `OpenSSH_for_Windows` selects Windows. Otherwise, `uname -snr` identifies `Linux` and `Darwin`; `MINGW` / `MSYS` / `CYGWIN` select Windows, and other results select Other. If `uname` cannot run, the app tries Windows collection. If both fail, connection tests and monitoring suggest selecting a platform manually.
- Manual selection: **Session → Advanced → Remote system** offers Auto, Linux, macOS, and Windows. A selection bypasses detection, useful when a Windows default shell is WSL/Cygwin or the server rewrites its identification.
- The platform is stored with host facts. Earlier records came from Linux collection and are read as Linux. First monitoring of a host waits for detection, then reuses its saved platform.
- MCP `connections_list` includes SSH `os` (`linux` / `darwin` / `windows` / `other`), the latest same-platform `os_name` and `arch`, and `shell` for Windows. Undetected facts are omitted.

## Metrics

| Metric | Meaning and scope |
|---|---|
| CPU | Aggregate utilization between valid samples, excluding duplicated guest accounting; idle includes iowait |
| Load | System load over 1, 5, and 15 minutes |
| Memory | `MemTotal - MemAvailable`; cache is not all counted as used |
| Network | Receive/transmit rate on the selected default-route interface, not a sum across interfaces |
| Top processes | Up to four readable processes with the highest interval CPU use; one full core is 100%, and multithreaded processes may exceed it |
| Process count | Processes visible to the SSH account, independent of the Top collection budget |
| System information | Hostname, kernel, and distribution come from shared host facts; uptime is live |
| Root disk | Root mount device/filesystem and, when SFTP statvfs is available, capacity and utilization |

CPU, network, and process rates need two valid samples. Interface changes, counters moving backward, or changed process identities rebuild the corresponding baseline. Processes use PID plus start time to avoid mixing samples after PID reuse.

## Basic host information

A successful SSH connection test detects the platform and reads hostname, OS name/version, kernel, and CPU architecture:

| Platform | Collection | Notes |
|---|---|---|
| Linux | `/bin/sh` reads `/proc/sys/kernel/hostname`, `osrelease`, `/etc/os-release`; `uname -m` supplies architecture | OS name uses `PRETTY_NAME` |
| macOS | `/bin/sh` runs `sw_vers -productName / -productVersion` and `uname -n / -r / -m` | OS name such as `macOS 27.2`; kernel uses Darwin version |
| Windows | `powershell.exe -NoProfile -NonInteractive -EncodedCommand`, using WMI `Win32_OperatingSystem`, registry `CurrentVersion`, and `OpenSSH\DefaultShell` | OS name and `DisplayVersion` follow system language; kernel is `Version.UBR`; architecture uses `PROCESSOR_ARCHITECTURE`; unset shell defaults to `cmd` |
| Other | Retain `uname` output | OS name, hostname, and kernel |

The Windows script supports Windows PowerShell 2.0+ without `Get-CimInstance`, `[ordered]`, or `ConvertTo-Json`. It captures errors in the script and returns them with the result. Fields use base64 UTF-8 to avoid console code-page issues. The encoded command is about 2.6 KB, below cmd.exe's command-line limit. Starting PowerShell adds roughly one second. Results remain in the edit draft until the session is saved to SQLite; cancelling leaves saved facts unchanged. Each new SSH connection refreshes facts in the background after its first terminal is ready. Terminals on the same connection share them regardless of panel visibility.

Session icons, system descriptions, and monitoring share these facts. Monochrome official OS logos use Simple Icons 16.35.0 (CC0), with the Windows four-pane logo drawn directly. Matching uses platform and `/etc/os-release` `ID`: Ubuntu, Debian, CentOS, Alpine, Fedora, Arch, Red Hat (`rhel`), Rocky Linux (`rocky`), AlmaLinux, openSUSE (`opensuse-*`), Kali, Linux Mint (`linuxmint`), Raspberry Pi OS (`raspbian`), deepin, macOS, Windows, and FreeBSD. Other Linux distributions and unknown hosts use Tux. Brand colors mix with the card background; glyphs retain official colors unless adjusted toward black/white to meet contrast thresholds (light 3:1, dark 4.5:1). Last successful facts remain visible offline. Collection time is used internally for freshness, not displayed. Uptime, process counts, and utilization are not cached as host facts.

Facts are bound to session ID, address, port, user, and the complete jump route. Changing a target does not reuse old-target facts; connected terminals still describe their actual target. Separate `hostFacts` database records do not change connection configuration revision timestamps. Older collections cannot overwrite newer records.

A collection has a five-second timeout. Failure does not affect SSH or clear previous successful facts for the same target. Tests and monitoring display failure reasons; database errors use existing error reporting.

## Unavailable data and permissions

`—` means no valid data yet or an unavailable server capability. One failed metric does not clear other valid metrics. After a whole collection fails, old values are not presented as current.

Network selection prefers a valid IPv4 default route, otherwise IPv6. Missing default interfaces or ambiguity among lowest-metric routes do not trigger a substitute sum of bridges, VLANs, or virtual interfaces.

Process visibility depends on account permissions, hidepid, and kernel configuration. If raw Top data exceeds its budget, the whole Top view is temporarily unavailable rather than presenting a truncated subset as a complete ranking.

Root capacity requires SFTP statvfs and confirmation that SFTP and Shell see the same root mount. Missing SFTP, missing extensions, chroot isolation, or permissions do not prevent other metrics.

## Latency and sampling intervals

Shell status-bar latency is sampled independently of monitoring. Visible terminals share an SSH application-level round-trip measurement per connection group every five seconds. Results above three seconds, or no valid sample for ten seconds, display as unknown. This is not ICMP ping.

Main metrics sample roughly every two seconds with a five-second Exec timeout and at most one request in flight; actual intervals include collection time. Root capacity samples roughly every ten seconds with an independent three-second timeout. Hidden windows and non-SSH pages do not start new latency samples. Monitoring keeps sampling on other pages; sampling pauses whenever the app builds no frames, and a trend resumed after more than its two-minute window starts over.

## Panel presentation

The panel follows the focused terminal. Its top card shows session name, OS icon, and system name. Status dots distinguish live, collecting, failed collection, connecting, failed connection, disconnected, and unmonitored states. Live status pulses with each sample; failures and partial unavailability show reasons below the card. Offline sessions retain only host facts, with an Enter-to-reconnect hint when applicable. Metrics without values use skeletons: shimmer before the first sample or while waiting for a second rate sample; static after failures, unsupported metrics, or reduced motion.

- CPU shows 1/5/15-minute load beside utilization; one-minute load above core count uses a warning color. The trend axis chooses 5, 10, 25, 50, or 100% from the peak; scale and peak appear below.
- Memory/disk use percentages and themed bars, warning at 70% and danger at 90%. Identical disk device/filesystem names display once.
- Upload/download each have a small trend with a shared vertical scale.
- Top-process row backgrounds reflect one-core utilization; rows rounding to 0.0% use secondary text.
- Host facts can collapse while the heading retains uptime.

CPU/network trends retain the latest 60 valid samples per connection, roughly two minutes, which is the whole chart. Hiding the panel, switching terminals, or switching pages preserves them; collection failure or the end of the connection clears them.

A connected terminal's pane footer shows latency, then the latest CPU and memory percentages and download/upload rates, each as an icon followed by its value. They appear together as soon as the terminal connects, with `—` until a value is available, and are hidden while it is not connected; hosts without monitoring support show latency only. Panes narrower than 560 px omit the network rates, and panes narrower than 380 px omit all four metrics.

## Development notes

Main metrics read `/proc/stat`, `loadavg`, `meminfo`, `net/dev`, routes, `uptime`, process stat, and mountinfo. Host facts separately read `/proc/sys/kernel/hostname`, `/proc/sys/kernel/osrelease`, and literal `/etc/os-release` values. Both use fixed `/bin/sh` collection and record parsing. Except for `uname -m` in basic facts, collection uses shell built-ins, not `ps`, `top`, or `df`, and does not execute text returned by the server.

Raw process records have a 512 KiB budget within the existing 1 MiB Exec output limit. Root capacity uses an independent short-lived SFTP subsystem rather than a browsing/transfer client.

Platform implementations live in `internal/hostinfo`. `Collector` provides facts and creates a `Sampler`; `Sampler` retains differential baselines and outputs `Metrics`. Files end in `_host.go` so Go does not interpret them as local-target build constraints.

Entry points: [hostinfo.go](../internal/hostinfo/hostinfo.go), [linux_host.go](../internal/hostinfo/linux_host.go), [darwin_host.go](../internal/hostinfo/darwin_host.go), [windows_host.go](../internal/hostinfo/windows_host.go), [host_facts.go](../internal/desktop/host_facts.go), [status_backend.go](../internal/desktop/status_backend.go), and [ssh_latency.go](../internal/desktop/ssh_latency.go). See [known limitations](limitations.md) for validation boundaries.
