# First steps

[English](getting-started.md) | [简体中文](getting-started.zh.md)

## Run the app

macOS requires version 13 or later. See [platform support](platform-support.md) for Windows build entry points and native acceptance scope. See [development and builds](development.md) for running from source and creating app packages.

The first launch has no preset hosts or file sites. The app provides SSH, File Transfer, Gateways, MCP, and Settings pages. Switching pages preserves established connections and background transfers.

Choose English, Simplified Chinese, or System in **Settings → General → Language**. Changes apply immediately without resetting connections, transfers, or form state. Raw system and remote diagnostics, user input, and historical free text retain their original content. See [interface languages](i18n.md).

## Create your first SSH session

1. Open SSH and use the add action or `⌘N` to create a session.
2. Enter a name, host address, port, and username. Choose password, private key, or SSH agent authentication. You can create a group in the form.
3. Optionally choose **Test connection** to check authentication and reachability, then **Save Only** or **Save and Connect**. Testing contacts the server and follows the same host trust rules as a normal connection.
4. For an unknown host key, verify the fingerprint before trusting it. Changed or revoked host keys are rejected.
5. Once connected, enter commands in the terminal. Double-click a saved sidebar entry to open it later.

A single sidebar click focuses an entry; a double-click opens an independent session. An entry can have several independent sessions. See [sessions and settings](session-settings.md) for authentication, advanced options, and credentials.

## Use splits and tabs

- Select a connected SSH pane and use the split button or `⌘D` to create an independent Shell on the same SSH connection.
- Drag panes or tabs to regroup them. Moving a pane preserves its terminal and connection.
- Use maximize to show one pane, or close to end that pane's session.
- `⌘1` through `⌘4` select panes first in a split workspace. `Ctrl+Tab` and `Ctrl+Shift+Tab` always switch tabs.

Closing one Shell does not close other Shells on its connection. More bindings are listed under [default shortcuts](session-settings.md#default-shortcuts).

## Open a file site

1. Open File Transfer and create an FTP/SFTP session.
2. Choose SFTP or FTP. SFTP uses SSH authentication. For FTP, configure TLS, active/passive mode, and filename encoding as required by the server.
3. Set initial local and remote directories, then save and connect.
4. Drag files or directories between the panes to upload or download. Manage the current tab's tasks in the transfer panel below.

SSH and file catalogs are independent. Matching names or addresses do not synchronize edits or deletion. See [file management and transfers](file-transfer.md) for pausing, conflicts, and recovery after quitting.

## Close and restore

By default, closing the main window quits the app: it saves the workspace and preferences, stops MCP, pauses and saves transfers, and closes all session connections. macOS, Windows, and Linux use the same behavior. The last-tab action is configured separately in Settings.

In **Settings → General → Background running**, choose whether closing the main window quits or runs in the background. Background running hides the window while keeping enabled MCP, connections, transfers, and the currently unlocked credentials active. The menu bar / tray icon is shown by default and can be hidden when closing the window is set to quit; background running requires the icon. On macOS and Windows, clicking it opens a panel: start or stop MCP and see its listening address and uptime, list connected SSH sessions from every tab with their latency (click one to jump to it), follow active transfers, and open Burrow, Settings, or quit. Right-clicking (or Control-clicking on macOS) still shows the native menu. Linux trays only support menus, so clicking the icon there shows the native menu. Launching Burrow again brings the existing window forward. Authentication requests show the window when user input is needed. On Linux, tray support requires `libayatana-appindicator3` and a desktop panel that displays AppIndicator icons; tray creation failures are reported and the window stays accessible.

macOS has two independent options, both off by default. **Hide Dock icon** hides Burrow from the Dock when the main window runs in the background and restores it when the window reopens. **⌘Q only closes the window** makes Cmd+Q hide the main window while Burrow stays in the menu bar and enabled MCP, connections, and transfers keep running. The latter option requires the menu bar icon regardless of the window-close setting; the app menu shows **Run in background** with the Cmd+Q shortcut. Neither option appears on Windows or Linux.

Quit in the app menu, tray panel, and tray menu always exits completely through the same shutdown and save process, including while the window is hidden; Cmd+Q follows that process by default. With **Restore sessions on startup** enabled, the next launch restores the workspace and associated tasks offline. Connect or resume transfers manually.

For connection, resume, or monitoring failures, inspect the actual error and [known limitations](limitations.md).
