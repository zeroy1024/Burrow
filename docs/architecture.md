# Architecture and lifecycle

[English](architecture.md) | [简体中文](architecture.zh.md)

## Code entry points

| Location | Responsibility |
|---|---|
| [main.go](../main.go) | App entry point |
| [internal/desktop](../internal/desktop/) | Page composition, sessions, connection scheduling, persistence, credentials, and task lifecycle |
| [internal/sshclient](../internal/sshclient/) | SSH connections, authentication, host trust, Shell/Exec/subsystems, and connection leases |
| [internal/wggateway](../internal/wggateway/) | Pure user-space WireGuard/netstack, strict single-peer import, tunnel DNS, and authenticated probes |
| [internal/fileclient](../internal/fileclient/) | Local, SFTP, and FTP/FTPS file operations and optional capabilities |
| [internal/vault](../internal/vault/) | Credential encryption, unlock/data key lifecycle, protection transitions, and paired backups |
| [internal/store](../internal/store/) | SQLite schema, transactions, host trust, and consistent backups |
| [internal/mcpserver](../internal/mcpserver/) | Local Streamable HTTP, key validation, authorization, policies, tools, and auditing |
| [internal/localfs](../internal/localfs/) | Platform file rules: private creation, Windows handle rename, and ACLs |
| [internal/locales](../internal/locales/) | English/Chinese ARB/ICU catalogs, typed functions, retained messages, and localizable errors |
| [internal/uikit](../internal/uikit/) | Shared presentation components and UI state |
| [internal/theme](../internal/theme/) | App semantic/component tokens and generated output |
| [internal/termtheme](../internal/termtheme/) | Ghostty color parsing, built-in schemes, and terminal preferences |
| [third_party/mygo-terminal](../third_party/mygo-terminal/) | Native terminal view and minimal local extensions |
| [third_party/pkg-sftp](../third_party/pkg-sftp/) | Private SFTP staging file creation extension |

The app composes UI and business behavior. UIKit consumes themes, display data, and callbacks; the app and protocol layers own network, file, credential, and background task lifecycles.

## Saved entries, running sessions, and connections

Saved entries describe future connections; SSH and file catalogs are separate. A running session retains its endpoint and authentication snapshot. Catalog edits do not silently change an active connection's identity.

SSH transports and business channels are separate. Splits open independent Shell channels on the same connection. Moving panes, regrouping, and dragging them back into tabs preserves terminal objects. Closing one channel leaves sibling channels alive; the last consumer releases the shared connection.

Entry points: [ssh_sessions.go](../internal/desktop/ssh_sessions.go), [ssh_terminal.go](../internal/desktop/ssh_terminal.go), [file_connections.go](../internal/desktop/file_connections.go), and [pane_drag.go](../internal/desktop/pane_drag.go).

WireGuard transports are shared by client/peer identity across terminal, transfer, MCP, and test scopes. Matching settings share one UDP device; incompatible settings for an active identity produce an error until its consumers close. Each consumer owns a lease, and the final release closes the device and in-memory stack. This differs from scoped SSH transports because independent WireGuard devices with the same identity can compete for the peer endpoint. See [WireGuard gateways](wireguard.md).

## Startup and unlock

The process takes an exclusive lock, opens the database, and loads the unlock view's theme and language. A new vault creates local automatic unlock material by default. Password-protected vaults wait for a password; legacy vaults wait for one-time migration. Business initialization runs once after unlock, then restores the workspace, loads tasks, and starts MCP if enabled.

While locked, only startup UI is built; business ticks do not run. Menus, connections, MCP HTTP admission, and task admission are gated by app state. Exiting before initialization releases resources without overwriting saved catalogs, preferences, or workspaces with empty defaults.

The automatic unlock source, file or Keychain, is recorded in authenticated vault metadata. A missing Keychain item does not fall back to an old file. Portable backups use the in-memory data key to create an independent file wrapper.

Protection changes and password changes use the existing persistence worker. They rotate the data key, re-encrypt all records, and commit atomically. New business admissions are refused during transition, while authenticated connections, running tasks, and checkpoints continue. An uncertain commit or failed business initialization prevents further use, avoiding restoration or saving of duplicate workspaces from partial state.

## Pages, windows, and tasks

SSH, File Transfer, MCP, and Settings build the current page exclusively; mounting a page does not own business tasks. Switching preserves connections, directory history, terminal scrollback, and transfers. Closing Settings clears temporary UI while reusing stable window-lifetime handles. Workspace state can restore SSH, file, or MCP page selection; Settings remains temporary.

Closing the main window and quitting share one process on every platform: save the workspace and settings, stop MCP, pause and save transfers, close session connections, then exit. Last-tab behavior is a separate preference.

Transfers use independent contexts and protocol clients/subsystems. Tasks are grouped by file-tab identity. Closing a browsing view or editing a site does not redirect them. Entry points: [transfer_backend.go](../internal/desktop/transfer_backend.go), [transfer_store.go](../internal/desktop/transfer_store.go), and [workspace_store.go](../internal/desktop/workspace_store.go).

## File consistency and persistence

- Writes use task-owned `.part` files and complete only after close and final commit succeed. Resume checks source baselines, partial-content digests, and completed directory items.
- Destination conflicts use real metadata. The app coordinates identical/overlapping targets; local targets conservatively account for case and Unicode normalization equivalence.
- Local upload sources, download destinations, and cleanup resolve beneath retained directory handles. Recursion does not follow symbolic links. SFTP private staging permissions are specified in the creation request, before content is sent.
- Structured data lives in SQLite. Catalog/workspace changes use transactions; settings/layout saves are debounced. Transfer workers commit manifests, digests, and rename intent at safe points. Read failures block overwrite rather than rebuilding the database.
- Vault encrypts passwords, managed private keys, and saved passphrases into the database. Ciphertext and configuration commit together; endpoint snapshots retain business credential versions. Legacy system credentials are migration sources only. The macOS release automatic unlock key has a separate Keychain service; other platforms and development builds use files. See [data and credentials](storage.md) for formats and locations.

Platform files isolate Unix metadata/mode bits and Windows ACLs. The Windows local client implements `PermissionPreserver` to copy and verify destination ACLs at commit. Private creation and native access live in `localfs`; queue logic does not embed Win32 calls. SSH agent uses separate Unix socket and local Windows named pipe entry points while sharing authentication lifecycle.

See [file management and transfers](file-transfer.md) for user-visible recovery/failure semantics and [known limitations](limitations.md) for missing protocol capabilities.

## Interface languages

The app reuses MyGo's language state and catalog generator. `mygo.json` declares `internal/locales`, with Simplified Chinese as the source and complete English resources. Language preferences use stable BCP 47 tags or an empty value for System. Menus and MCP presentation snapshots refresh on changes. Retained errors/notices store message functions and arguments for display-time formatting; business errors preserve their chains. Display copy is not an MCP filter, grant, or protocol value. See [interface languages](i18n.md).

## Terminal and local dependencies

Ghostty maintains terminal state; MyGo draws the grid, selection, cursor, and native scrollbar. The app owns pane layout, focus, input bindings, colors, and top/bottom bars. Actual font metrics and pixel size determine grid dimensions and remote PTY size.

`ViewportBackdrop` draws adjacent history lines continuously beneath the bars using the same frame's grid coordinates. It does not create another terminal or move the real viewport. Blur is an overlay. See [terminal and color schemes](terminal-themes.md).

[go.mod](../go.mod) replaces two local modules while the main MyGo dependency remains pinned to v0.7.3:

| Local module | Extension and maintenance basis |
|---|---|
| `mygo-terminal` | Live scrollback limits, history backdrop, and pinned Ghostty ABI; see [FORK.md](../third_party/mygo-terminal/FORK.md) |
| `pkg-sftp` | `Client.OpenFileMode` sets permissions during creation; see [FORK.md](../third_party/pkg-sftp/FORK.md) |

Upstream licenses, resource manifests, and source attribution are retained. See [theme integration](theme-integration.md) and [UIKit](uikit.md) for component conventions.
