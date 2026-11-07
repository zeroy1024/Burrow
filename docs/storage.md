# Data and credentials

[English](storage.md) | [简体中文](storage.zh.md)

## Default data locations

Structured business data lives in `burrow.db`, using SQLite schema v5. Existing v1/v2/v3 databases receive a consistent pre-upgrade backup, then schema upgrades and one-time credential migration. Before upgrading v4, a `.before-gateways-<timestamp>.bak` is created; one transaction converts legacy jump settings, workspace/transfer snapshots, and hash references while retaining business data. App data directories are:

| System | Release | Development |
|---|---|---|
| macOS | `~/Library/Application Support/Burrow/` | `~/Library/Application Support/Burrow Dev/` |
| Windows | `%LOCALAPPDATA%\Burrow\` | `%LOCALAPPDATA%\Burrow Dev\` |
| Linux | `$XDG_DATA_HOME/Burrow/`, or `~/.local/share/Burrow/` if unset | Corresponding `Burrow Dev/` directory |

Ordinary `go run` and MyGo development packages use the development namespace; release packages use Burrow. Each directory has an independent encrypted vault and unlock key. The old `dev.burrow.app` and `dev.burrow.app.dev` credential services are read only during migration. The macOS release additionally uses `dev.burrow.app.vault-unlock` for automatic unlock keys; development/integration builds keep file keys. Tests use temporary directories; native E2E uses an overlay with isolated data and credential providers.

Only one Burrow process may hold a database at a time, preventing stale in-memory snapshots in separate windows from overwriting each other. `burrow.db.lock` carries an OS lock, which is released on normal exit or process termination. The file does not need to be deleted.

## Stored content and boundaries

| Location | Contents |
|---|---|
| SQLite `catalog_groups` / `profiles` | Groups, stable IDs, ordering, connection settings, and credential-version references |
| SQLite `gateways` / `gateway_settings` | SSH / SOCKS5 gateways, source host/upstream references, and the app default exit |
| SQLite `managed_keys` | Public key text, name, type, fingerprint, and private credential references |
| SQLite `settings` | One set of app/terminal settings, shortcuts, and file-pane preferences |
| SQLite `workspace` / `endpoints` | Layout, focus, file-tab paths, and independent connection snapshots |
| SQLite `endpoint_keys` | Gateway private key references in snapshots, protecting managed keys needed for recovery/transfers |
| SQLite `transfer_*` | Batches, tab ownership, endpoint/gateway route snapshots, tasks, recursive manifests, commit state, and checkpoints |
| SQLite `host_keys` | Normalized hostname/IP, port, complete public key, algorithm, revocation flag, and confirmation time |
| SQLite `themes` | Imported scheme names, original filenames, and source bytes with attribution/license comments |
| SQLite `settings` entry `mcp` | MCP enabled state, listening Host/Port, and audit retention |
| SQLite `mcp_keys` / `mcp_policies` | Access key digests, connection grants, local directory scope, and interception policies |
| SQLite `mcp_operations` / `mcp_files` | Operations, target/rule snapshots, lifecycle, and raw per-file events |
| SQLite `mcp_payloads` | Requests/responses associated by operation ID, loaded on demand and deleted with their audits |
| SQLite `mcp_tasks` / existing `transfer_*` | MCP task ownership and transfer manifests, committed together before task acceptance |
| SQLite `vault_meta` / `vault_items` | Unlock parameters, wrapped data key, and ciphertext for session/gateway passwords, managed private keys, and saved passphrases |
| macOS release login Keychain | A random unlock key when startup unlock is off, bound to vault ID/key ID; no remote passwords or private keys |
| `vault-keys/<id>.key` | Random unlock keys for Windows, Linux, development/integration, and portable backups; no active reference when app password unlock is enabled |
| Old system credentials and `keys/` | Migration sources only; old copies are retained and no longer used for normal connections |
| Actual transfer destinations | Uploaded/downloaded content and unfinished `.part` files |

Connection credentials enter SQLite only as ciphertext. File read/write bodies, terminal screens/scrollback, and monitoring curves are not persisted. The database is not encrypted as a whole. Treat site addresses, usernames, paths, environment variables, and initial commands as private data; do not put secrets in environment or command settings.

A complete MCP access key is handed to the UI only at creation. The database stores its SHA-256 digest and public prefix. MCP audits retain command text, request parameters, structured responses, and stdout/stderr. Authentication headers are not collected; common authentication fields are redacted, and file bodies are replaced with omission markers and byte counts. Commands and output can themselves contain sensitive data; automatic detection cannot identify every secret.

File-body omission applies to text parameters/results of `files_read` and `files_write`. File contents read through an SSH command remain stdout and cannot automatically be identified and removed as file bodies.

Lists load operation summaries only. Large request/response data lives in `mcp_payloads`, is loaded when details expand, and is processed in the background. Old records retain their available summaries; missing historical response bodies cannot be reconstructed. Response capture is limited to 32 MiB per response. Larger responses retain size/truncation metadata rather than saving truncated JSON as a complete body.

Caches and logs have separate locations. macOS uses system Cache/Logs directories; Windows uses `cache/` and `logs/` under the data root; Linux caches follow XDG. MyGo still manages system language preferences and may create its `locale` file on Windows/Linux. App preferences in the database remain authoritative.

## Consistency and recovery

[internal/store](../internal/store/) centralizes `database/sql + modernc.org/sqlite` access without system SQLite or CGo. Foreign keys, WAL, `synchronous=FULL`, and a five-second busy timeout are enabled. Each Store keeps one database connection.

Groups, profiles, key metadata, and associated workspace changes commit in transactions. UI creates immutable snapshots and database writes run serially in the background. Save failures are reported, and catalog operations restore their in-memory snapshot. Settings/layout saves use a 300ms debounce and are explicitly flushed before closing/quitting.

New tasks are saved before starting. A transfer worker exclusively owns its manifest and saves confirmed offsets and SHA-256 digests at safe points. Continuous writes checkpoint at roughly one-second or 4 MiB intervals. Commit intent is saved before final rename; pause/completion/cancellation/shutdown save separately. Crash recovery uses the last successful checkpoint and still verifies the actual source, `.part`, and destination. The final byte is not guaranteed to be recoverable.

Credential ciphertext, version references, and catalog settings commit together. Security changes rotate the data key and atomically replace metadata/ciphertext after re-encryption, preserving business password versions. New local key files are fully written and made durable before database references change. Filesystem and remote operations are outside SQLite transactions. Unix files use `0600`, directories `0700`; Windows uses private ACLs for the current account and SYSTEM.

Read failures, corruption, or unsupported schemas produce explicit errors rather than deleting/rebuilding the database. `PRAGMA user_version` records the schema; future upgrades require explicit migration implementations.

## App unlock

**Settings → Keys & Credentials → Require unlocking when opening the app** is off by default. Automatic mode reads an unlock key from Keychain in macOS release builds or a local file elsewhere. Enabling it requires an app password on each launch. The same page changes the password or disables protection after checking the current password. Passwords are 6–64 Unicode characters, inclusive; spaces are not trimmed and Unicode normalization is not applied.

Credentials always use an independent random data key and authenticated encryption. Argon2id derives the unlock key from an app password; neither password nor derived result is persisted. Automatic unlock uses a separate random key in the macOS release login Keychain or a private local file for other builds. Vault metadata v2 authenticates the key source. v1 file keys remain readable; macOS releases rotate the data key after successful unlock and explicitly migrate to Keychain. See [vault design](credential-vault-plan.md) for algorithms, rotation, and failure rules.

While locked, the app shows only unlock UI: no workspace restoration, connection/transfer scheduling, or MCP listener. Exiting while locked does not save an empty workspace or default settings. Unlock follows existing recovery rules: SSH remains disconnected and unfinished transfers paused. Returning to the foreground, waking, or showing another window in the same process does not prompt again.

Automatic unlock protects against a database-only leak. With file keys, possession of both database and key file permits decryption. macOS Keychain access depends on the system and fixed signing identity. Password protection depends on password strength and cannot protect against an attacker controlling the process, recording input, or reading unlocked memory. Hosts, usernames, audits, and other metadata are not encrypted as a whole.

Legacy upgrades show a one-time migration action at first launch and may require old credential-store authorization. Missing saved passwords can be re-entered explicitly. Encrypted private keys require their original passphrases; temporary entry does not force passphrase saving. Normal use stops reading old credential namespaces afterward; macOS releases access only the new automatic unlock service. Old private keys, system credentials, and pre-upgrade backups remain. Handle these copies separately after confirming migration.

Managed private keys are decrypted into memory for authentication without connection-time temporary key files. Use **Export Private Key…** to choose a file explicitly. Import source files remain untouched.

## Host trust

SSH callbacks query the database. The client does not invoke system OpenSSH or read `.ssh/known_hosts`. Trust records are separate from catalog entries; editing/deleting an entry does not automatically clear trust.

Unknown hosts require fingerprint confirmation. No database transaction is held while waiting. On write, trust is checked again in case another connection stored a different key first. Changed and revoked keys are rejected and cannot be overwritten through first-trust confirmation.

## Legacy formats and backups

The new model does not read, migrate, or dual-write old `sessions.json`, `prefs.json`, `files.json`, `terminal.json`, `transfers.json`, `catalog.json`, `themes/`, or `known_hosts`. Existing files remain untouched and require reconfiguration in the new version. There is no automatic legacy-format fallback.

For file automatic unlock or password mode, quit normally before copying the entire data directory. The macOS release Keychain key is outside that directory; copying it alone does not guarantee recovery on another machine. Use a paired Vault backup. Do not copy a live `burrow.db` while ignoring WAL. Store's `Backup` uses SQLite `VACUUM INTO` for a consistent standalone database; there is currently no backup UI.

Database backups include encrypted credentials and managed private keys. With startup unlock off, Vault's `Backup` creates an independent file unlock key and rewraps backup metadata while preserving credential versions and generations. The backup can be restored without the original Mac's Keychain. Password-mode backups require the password used when the backup was created. An unlocked session creates backups from its in-memory data key without rereading the original Keychain item. Store's `Backup` alone creates only the database.

Old backups keep their original unlock material. Password changes do not rewrite them or revoke leaked remote passwords/private keys. Without the password or an unlockable backup, the app cannot recover credentials. Actual transferred files, `.part` files, original external private keys, and external ssh-agent contents require separate handling. See [file management and transfers](file-transfer.md).

See [macOS signing and unlock](macos-signing.md) for fixed self-signing, certificate backups, and cross-version checks. Protected Touch ID Keychain access needs separate validation; an ordinary file key plus a fingerprint UI is not hardware-backed key protection.
