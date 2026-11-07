# Sessions and settings

[English](session-settings.md) | [简体中文](session-settings.zh.md)

## Saved catalogs and running sessions

SSH and File Transfer maintain their own entries, groups, order, and collapsed state. Both catalogs may contain entries with the same name without affecting each other.

**Quick add to SFTP…** on an SSH host and **Quick add to SSH…** on an SFTP site open a new-session form for the other protocol. They prefill the name, address, original port, username, authentication, private key, and connection gateway. Protocol-specific fields use new-session defaults. Saving creates an independent ID and password credential; a managed private key continues to reference the same key. FTP / FTPS does not offer this action.

**Save Only** updates the catalog; **Save and Connect** also opens a new session. A single sidebar click focuses an entry, and a double-click opens an independent session. Editing an address or authentication settings does not redirect an active connection. Background transfers keep the endpoint and credential version captured when queued.

Deleting a saved host or site closes the saved sessions associated with that entry and removes its entries from closed-tab recovery. Cancel or remove associated transfer tasks and wait for them to leave the queue before deleting the entry. References still prevent deletion of dependencies such as SSH hosts used by gateways or managed keys in use. Other saved entries with the same name or address remain independent.

## Connection gateways

**Gateways** is a separate page below File Transfer in the app rail, rather than a Settings category. It manages SSH / SOCKS5 gateways and the app default. The header shows the default exit and its resolved route; the sidebar groups gateways by type, and the detail pane shows the selected route, configuration, and references.

On this page, `⌘N` (`Ctrl+N` on Windows/Linux) adds a gateway and `⌘F` searches gateways. Session close and switch shortcuts do not operate on background terminal tabs.

Hosts and file sites can follow the app default, connect directly, or use a specific gateway. The initial app default is direct. Edits affect later connections; running sessions and transfers retain their captured routes and credential versions. Terminals, file browsing, background transfers, connection tests, and MCP share the same routing rules. Gateway failure is reported; it does not silently switch to a direct connection.

An SSH gateway references a saved SSH host's address, authentication, and Keepalive settings without running its initial command. Its upstream route comes from that host's own **Connection gateway** setting, so reaching the host as a gateway follows the same route as connecting to it directly. This is consistent with recursive `ProxyJump` host configuration in OpenSSH. Changing the source host's gateway also changes gateways based on it. A SOCKS5 gateway can choose its own upstream gateway and supports no authentication or username/password authentication. Saved passwords enter the encrypted Vault. Both gateway types can be combined.

A host following the app default connects directly if it is itself on that default route, for example when it is the source host of the default SSH gateway. Explicit routes through the host itself, or mutually upstream gateways, form loops. They are disabled in selection and rejected on save.

Several targets can share an SSH gateway connection. Closing one terminal or file session releases only its own channels. SOCKS5 establishes a separate connection for each target. The selected exit handles target hostnames; the first gateway address is reached over the local network.

FTP / FTPS through a gateway requires passive mode. Its control and data connections follow the same route. Active mode remains available for direct connections.

A gateway test connects to the gateway itself: SSH checks the handshake, host trust, and authentication; SOCKS5 checks negotiation and the configured authentication method. Upstream gateways are used when configured. No target host is required. The route marks proven, connecting, and failed hops, and errors identify the failed hop. Results last for the current run and are cleared when any gateway is saved. Cancelling or leaving the page stops the test. Use the host/site's **Test connection** action to check target reachability.

A gateway cannot be deleted while referenced by defaults, catalog entries, downstream gateways, running or recoverable sessions, or transfers.

Database upgrades convert legacy SSH jump configurations into SSH gateways once, preserving multihop paths. Existing transfers migrate their captured jump snapshots. MCP connection identity includes the complete gateway route. Equivalent legacy SSH routes preserve identity checks; actual routing or authentication changes require authorization to be confirmed again.

## Authentication and advanced options

| Protocol | Authentication and configuration |
|---|---|
| SSH | Password, managed or encrypted private key, SSH agent; gateway, environment variables, initial command, Keepalive, and session terminal colors |
| SFTP | SSH authentication and gateway; initial local/remote directories and filename encoding |
| FTP / FTPS | Username/password or anonymous; no TLS, explicit TLS, or implicit TLS; active/passive mode, initial directories, and filename encoding |

**Test connection** uses the real protocol. SSH/SFTP share normal host trust and authentication rules; SFTP also checks the subsystem and target directory. Unknown hosts require confirmation. Editing the form or closing the dialog cancels the test. FTP TLS verifies certificates and provides no option to ignore certificate errors.

**Keys & Credentials** supports private key import, Ed25519 / RSA (4096-bit) generation, fingerprints, and public key copying. Imported private keys are encrypted in the app database and can be retrieved with **Export Private Key…**. Referenced keys cannot be deleted directly. See [data and credentials](storage.md).

On macOS/Linux, agent access uses the `SSH_AUTH_SOCK` Unix socket. Windows defaults to the local `\\.\pipe\openssh-ssh-agent` pipe. If `SSH_AUTH_SOCK` is set on Windows, it must name a local named pipe. Authentication and the agent check in Settings use the same entry point. They do not connect to remote pipes or start/configure the system agent service.

## App settings

Settings apply immediately and save after a short debounce. Save failures are reported.

| Category | Main settings |
|---|---|
| General | Language, startup restoration, last-tab behavior, and active-connection close confirmation |
| Appearance | Light/dark/system, accent, reduced motion, and window translucency |
| Terminal | [Fonts, colors, cursor, and scrollback](terminal-themes.md) |
| File Transfer | Default local directory, concurrency, and name conflict policy |
| Keyboard Shortcuts | Record bindings by group; Esc or clicking elsewhere cancels, Delete clears; replace conflicts or reset individual/all bindings |
| Keys & Credentials | Startup unlock, password changes, managed keys, and saved credentials |
| About | Version and open source licenses |

**General → Language** offers English, Simplified Chinese, and System. It applies immediately and saves in existing preferences. Menus, business messages, and accessibility labels use the current language. System and remote diagnostics, user-assigned names, and historical free text retain their original content. Some native system dialog text may follow the app language only after restarting. See [interface languages](i18n.md) for resources and validation boundaries.

New configurations enable startup restoration and active-connection confirmation, and keep the window when the last tab closes. Transfer concurrency defaults to 3, conflicts to Ask, and the local directory to the user's home. Existing configurations retain saved values.

System reduced transparency disables app translucency. Settings supports search. Leaving Settings does not disconnect terminals or cancel transfers.

## Default shortcuts

| Action | Default binding |
|---|---|
| New session on the current page | `⌘N` |
| Close the current pane or tab | `⌘W` |
| Reopen a closed tab | `⌘⇧T` |
| Select pane or tab 1–4 | `⌘1`–`⌘4` |
| Previous / next pane or tab | `⌘[` / `⌘]` |
| Always select previous / next tab | `Ctrl+Shift+Tab` / `Ctrl+Tab` |
| Toggle sidebar | `⌘\` |
| SSH / File Transfer page | `⌘⌥1` / `⌘⌥2` |
| Split SSH pane | `⌘D` |
| Search current page | `⌘F` |
| Open Settings | `⌘,` |

The table uses macOS notation. Windows uses Ctrl for the command modifier, with labels such as `Ctrl+N` and `Ctrl+D`. Number and previous/next bindings select panes first when split, and tabs in a single pane or file view. The Settings page is authoritative for recordable bindings; source: [settings_shortcuts.go](../internal/desktop/settings_shortcuts.go).

## App unlock

**Require unlocking when opening the app** is off by default. Enabling it sets an app unlock password and requires it on subsequent launches. **Change Password…** then appears on the same page. Changing or disabling it requires the current password; cancellation or failure preserves the previous setting. This password is separate from remote login passwords and private key passphrases, and does not modify server authentication.

The app unlocks once per process run; switching windows or waking the system does not prompt again. Credentials remain encrypted when startup unlock is off and are unlocked by a local key. See [data and credentials](storage.md) for backup and recovery requirements.
