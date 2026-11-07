# Local protocol integration and native E2E

[English](README.md) | [简体中文](README.zh.md)

This environment exercises real SSH/SFTP/FTP transfers and macOS native windows. Use only script-created local services and temporary credentials, never existing private sites. See [development and builds](../../docs/development.md).

## Start servers

From the repository root:

```sh
python3 scripts/ssh-integration-server.py
```

The script builds Ubuntu 24.04 / OpenSSH and starts a separate container labeled `dev.sshdesk.integration=true`. SSH maps to a random `127.0.0.1` port only. No host directories or Docker volumes are mounted. The container includes full `vim`, `htop`, and `top`, uses `C.UTF-8`, and logs in as `sshdesk`.

SSH enables `internal-sftp`. Fixtures include `hello.txt`, `你好.txt`, `nested/inside.txt`, and unreadable `denied/` for directory/permission checks.

The final JSON line provides `container`, `address`, and `directory`. Passwords, plain/encrypted private keys, and passphrases are generated there with directory mode `0700` and secret files `0600`, never in repository files or build arguments. The container receives only temporary public keys and a password through stdin.

Use those values in the task environment:

```sh
export SSHDESK_TEST_ADDRESS='127.0.0.1:<mapped-port>'
export SSHDESK_TEST_DIR='/private/tmp/sshdesk-integration-<temporary-directory>'
```

Start an independent FTP server using the same temporary password:

```sh
python3 -B scripts/ssh-integration-server.py --ftp
export SSHDESK_FTP_ADDRESS='127.0.0.1:<ftp-mapped-port>'
```

FTP uses a separate vsftpd container with init. Its control port and 64 passive data ports map locally only. Authenticated reads/writes are enabled at 1 MiB/s per connection to make pause/cancel/recovery observable. SSH metadata is `server.json`; FTP metadata is `ftp-server.json` in the same fixture directory. Credentials are shared, but connections/sessions are independent.

Use `python3 -B scripts/ssh-integration-server.py --ftps` for explicit FTPS and append `--implicit` for implicit FTPS. Temporary CA, server certificate with loopback SANs, and metadata stay in the fixture directory. TLS verification and data-channel session reuse remain enabled. See [MCP integration](../mcp-integration/README.md) for real HTTP, native controls, and recovery.

## Transport validation

```sh
mise exec -- go test -race -tags=integration ./... -count=1 -timeout=180s
```

Coverage includes password, plain/encrypted private keys, isolated SSH Agent, wrong credentials, first-key rejection/save/reuse/change rejection, UTF-8/raw color sequences, remote `stty size`, four-session isolation, 128 KiB output and exit status, and cancellation/closure during handshake or active sessions.

It also covers independent Shell/SFTP channels sharing one connection, final-reference release, controlled cancellation of pending channel creation, and SFTP/FTP Chinese names, navigation, permission errors, cancellation, and parsing boundaries. Blocked SFTP initialization uses a controlled stream; it is not claimed to inject a real SSH zero window.

File security checks cover local source/destination directories replaced by out-of-tree links during read/write/cleanup, final-file replacement, and actual SFTP OPEN requests carrying exclusive creation and `0600` atomically. Old parts with `0644`, `0400`, or `0000` are restricted before resume writes while retaining confirmed contents. Local file tests do not need Docker; SFTP security tests use the real fixture.

Docker tests use the `integration` build tag. Ordinary `go test ./...` does not connect to them; untagged SDK lifecycle tests use local in-process SSH services only. Docker targets must be local. If the build cache is unwritable, set `GOCACHE=/private/tmp/sshdesk-go-cache`.

## Native window validation

```sh
mise exec -- go tool mygo build -skip-dmg
python3 -B -u scripts/ssh-integration-ui.py
python3 -B -u scripts/ssh-integration-ui.py --concurrent-split
python3 -B -u scripts/ssh-integration-ui.py --file-protocols
python3 -B -u scripts/ssh-integration-ui.py --core-e2e
```

A Go overlay enables the pinned MyGo automation plugin and redirects configuration only in temporary builds. It does not change `HOME`, release builds, or existing Burrow settings. A copied SDK is referenced through a separate modfile without changing global module caches or project dependencies. Data, SQLite loading, local defaults, caches, and logs are isolated. A test credential provider stores migration sources in `0600` files. The overlay performs one-time migration with fixture passphrases, then uses real Vault ciphertext/version checks. These tests do not cover macOS Keychain integration.

It starts a real macOS window, sends clicks/keys/menu actions through a local Unix socket, and reads text/grid state from the actual Ghostty instance. Snapshot is read-only state observation, not screenshot or visual acceptance.

The pinned automation plugin normally formats arbitrary Keys in the background; reflecting pointer Keys could read objects during layout. The overlay encodes pointer Keys as type/address only, avoiding concurrent reads and disclosure while retaining scalar Keys, UI behavior, and race checks. Failure diagnostics retain node labels, roles, state, and geometry, not input values or Key contents.

Coverage includes authentication dialogs, first-trust rejection/acceptance, terminal input, Japanese editing in `vim`, `top`, initial split focus, independent Shells, resize, pane closure, disconnection, reconnect history, and four authentication modes. Resize compares app state, Ghostty grid, and remote `stty size`. The script owns its SSH Agent with temporary keys and closes only its own app/Agent afterward.

Default mode also checks independent add/edit/delete in both catalogs, SQLite schema v1 catalog loading and host trust, running-session identity snapshots, and no extra TCP/authentication for splits sharing a connection. `--concurrent-split` checks three connected channels created in one action, one TCP/authentication, and shutdown cleanup. It invokes splitting through a test menu and establishes model/protocol integration, not real split-input acceptance.

`--file-protocols` uses saved-site UI for SFTP/FTP Chinese entries, directory navigation/history, connection survival after permission errors, file controls, local-pane toggling, reconnect after wrong FTP passwords, and independent SSH/SFTP/FTP closure/release. The current UI lacks the old driver's cross-page shortcut buttons, so this mode does not cover temporary shortcut views or cross-page transport reuse. Go protocol tests still check shared SSH/SFTP channel lifecycle.

`--core-e2e` uses production double-clicks, forms, keyboard input, real pointer drags, and Quit. Test menus provide focus/read-only observations only. It covers:

- New-Shell environment isolation, initial focus, survival after closing another pane, and closing/reopening while dragging tabs.
- Real SFTP/FTP two-pane drag uploads/downloads with independent server/local SHA-256 checks. Switching pages mid-drag must not change the source or create accidental tasks.
- Real FTP pause/resume/cancel/partial cleanup and independent task views for two tabs of the same site.
- Pausing the controlled FTP container so refresh replies arrive while a file row is held; sources remain until release and cancelled drops create no transfer. The container is unpaused whether assertions pass or fail.
- Normal shutdown and restart of the same isolated app/config, offline task ownership restoration, unchanged confirmed offsets, explicit resume, actual content, and partial cleanup.

Core/file modes require `SSHDESK_FTP_ADDRESS`. Each core run uses independent directories in both containers. The launcher checks every native process's exit record, race log, and released SSH/FTP connections, writing `native-run-*/result.json`. Partial failure cannot be recorded as success.

Do not run SDK Docker tests concurrently: connection/authentication counters would interfere. Acceptance requires the actual app's exit state and race logs, not only LaunchServices `open` returning success.

The current arm64 bundle is a native-library/icon template for a separate race-enabled overlay app. `--prepare-only` builds without opening a window. Native integration requires an unlocked, focusable macOS graphical session and fails quickly when locked. Each run gets isolated configuration; native logs and observations remain in temporary directories without authentication input.

## Running resources

Servers remain running for manual checks after the script exits. Inspect fixture containers and stop only the exact names from the launch result:

```sh
docker ps --filter label=dev.sshdesk.integration=true
docker stop <container-name>
```

Do not run automation against unrelated servers/existing credentials or commit temporary credentials/logs.

Record the commit, fixture environment, commands, exit status, skips, and uncovered scope. These are instructions, not proof that a version passed every mode. See [interpreting validation](../../docs/limitations.md#interpreting-validation).
