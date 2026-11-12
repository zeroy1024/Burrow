# Known limitations and validation scope

[English](limitations.md) | [简体中文](limitations.zh.md)

This page records current boundaries. Validation applies only to the code version, environment, and scenarios actually run. It does not automatically establish acceptance for later versions or other platforms.

## Platforms and UI

- macOS requires version 13 or later. Windows amd64/arm64 has compilation and EXE/portable ZIP packaging entry points; native runtime acceptance requires a Windows environment. Linux has compilation checks without a guarantee of desktop runtime support. See [platform support](platform-support.md).
- Linux monitoring requires suitable remote `/proc` data. Root disk capacity also requires usable SFTP statvfs support and a matching root mount.
- App UI and business messages support English, Simplified Chinese, and System. Historical free text and raw system, third-party, and remote diagnostics retain their original content. Some native system text updates only after restarting. Native language switching, English layouts, and accessibility behavior have not been accepted; see [interface languages](i18n.md).
- In-app updates come from GitHub Releases; About checks for updates and chooses the channel, system proxy, and GitHub mirror. Versions 0.1.8 and earlier need a manual install of a newer version. Cross-version upgrades have not had native acceptance on each platform, and the Preview channel depends on the GitHub API. About shows the current version and licenses.

WireGuard currently supports a single peer as the first route hop, app-local DNS with IP servers, and passive FTP/FTPS. It does not manage OS VPN interfaces or network settings, execute imported hooks, support DNS search domains, or nest WireGuard after another gateway. See [WireGuard gateways](wireguard.md).

## Transfers and recovery

- Checkpoints are saved at safe points and throttled intervals. After forced termination or power loss, recovery uses only the last successfully committed state that can be verified against real files.
- Resuming requires server capabilities. FTP without REST, or a destination that cannot meet permission/commit requirements, reports a failure.
- FTP cannot provide an atomic conditional rename against other clients. Preflight checks cannot eliminate external changes around final commit.
- Overwriting preserves and verifies known destination permissions. Missing capabilities, unknown permissions, or inability to create private staging files safely may prevent it.
- Symbolic links and special-file contents are not transferred. Directory links can be browsed explicitly; recursive transfers and deletion do not follow them.
- Startup restoration stays offline. Tasks lacking their original tab identity are not assigned by guessing a site name; use **Recover tasks** to reassociate them.

See [file management and transfers](file-transfer.md) for the complete behavior.

## Terminal

- Scrollback counts physical display lines, but eviction of whole history pages introduces an error margin. The retained amount is not guaranteed to equal the setting exactly. Raising the limit cannot recover evicted history.
- Custom imported colors at indices 16–255 are not passed to the native terminal; it retains its default indexed palette there.
- The native terminal scrollbar and shared UIKit scrollbars have a consistent style but different dimensions and visibility durations.
- The fallback drag Snapshot lacks a historical viewport position and cannot guarantee pixel-for-pixel alignment in the middle of history.

See [terminal and color schemes](terminal-themes.md).

## Interpreting validation

| Method | What it establishes | What it does not replace |
|---|---|---|
| Go build, vet, ordinary tests | Affected packages compile and pass static checks and existing assertions | Real server compatibility, native window interactions, or visuals |
| Local protocol integration tests | Protocol and file behavior in the specified SSH/SFTP/FTP fixtures | Untested servers, networks, or filesystems |
| Native E2E driver | Window input, terminal, and transfer assertions actually executed | Uncovered scenarios, system Keychain behavior, or visual acceptance |
| Software-rendered screenshots | Rendering and layout of the sample | Native GPU, system windows, or real authentication |

Integration tests skipped for missing environment variables do not count as protocol acceptance. Existing ordinary tests do not cover every monitoring parser/scheduler or statvfs path. Historical monitoring records did not complete an end-to-end successful Linux/OpenWrt collection check.

Windows ACLs, handle-based rename, agent, system credentials, window interactions, and installation/uninstallation have not received native acceptance. Existing local filesystem tests were run only on macOS/Linux. Cross-builds and PE/ZIP resource checks do not establish these behaviors.

Recent ShellCard scroll-edge fixes passed existing tests, static checks, and builds, but have not received another native GPU visual check. Record device, server, and visual results separately; do not infer them from old logs or the presence of code.

See [development and builds](development.md) for commands, and [integration instructions](../dev/ssh-integration/README.md) for controlled fixtures and native drivers.
