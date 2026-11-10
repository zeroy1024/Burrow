# App unlock and credential vault design

[English](credential-vault-plan.md) | [简体中文](credential-vault-plan.zh.md)

Interface languages: related pages, forms, notifications, and business errors support English, Simplified Chinese, and System. Protocol fields, state codes, authorization decisions, and persisted values remain stable. Historical free text and original diagnostics are not rewritten. See [interface languages](i18n.md).

This page records the vault's schema v4 design and acceptance constraints. See [data and credentials](storage.md) for current schema v5 and later gateway migration.

Date: 2026-11-05. Status: integrated. Existing race tests and isolated encryption/migration checks passed; native UI and migration of real system credentials still await acceptance.

Burrow encrypts passwords, managed private keys, and optionally saved passphrases in its existing SQLite database. Automatic unlock keys use private local files on every platform; system credential services are legacy migration sources only. Users choose **Require unlocking when opening the app**, off by default for automatic unlock. When on, each launch requires an app password. The same page offers **Change Password**.

Sections 1–3 define product behavior. Later sections define supporting storage, startup, transition, and recovery constraints. Internal key mechanisms must not become extra user options.

## 1 Confirmed product behavior

| Item | Behavior |
|---|---|
| Location | Settings → Keys & Credentials, with App Unlock above existing private keys |
| Single switch | Require unlocking when opening the app |
| Default | Off; new users need no app password |
| Off | Unlock automatically on startup and enter the app |
| On | Set/confirm a password once, then unlock before subsequent launches |
| Change password | Show Change Password… on the same page only while enabled |
| Disable | Verify the current app password, restore automatic unlock, and retain credentials |
| Storage | Credentials are encrypted in both cases; no plaintext mode |
| Backend | One encryption/decryption/read/write implementation; only unlock-key acquisition differs |
| App updates | Retain vault, keys, and settings; binary versions/code hashes are not decryption inputs |
| Locked boundary | The entire app is unavailable; MCP, connections, transfers, and recovery cannot bypass unlock |

Opening the app means starting a new process in this version. Foregrounding, showing a window, restoring from minimize, or waking the system within one process does not prompt again. Existing window-close behavior still follows app shutdown.

Changing the switch affects the next startup requirement without immediately interrupting connections. This version adds no manual/idle/system-lock locking, Touch ID, password recovery, recovery codes, multiple vaults, or separate background unlock interface.

These limits keep startup unlock separate from session management. After quitting, the next launch performs the selected unlock process again.

## 2 Settings and user actions

### 2.1 Page contents

Reuse Keys & Credentials, settings rows, switches, and shared modals without a new category.

| Control | Copy or behavior |
|---|---|
| Group | App Unlock |
| Switch | Require unlocking when opening the app |
| Help | Require the unlock password each time Burrow starts. |
| Enabled action | Change Password… |
| Saved passwords | Replace system-Keychain wording with passwords/passphrases encrypted on this device |

Do not expose Plaintext mode, Automatic unlock mode, KEK, DEK, or algorithm choices. Explain password loss, key export, and backups in their relevant actions rather than normal connection flows.

The switch shows the last successfully committed state. Enabling first requests a password; disabling first verifies it. Cancellation/failure retains the previous state instead of appearing successful and rolling back.

Password forms use session-form validation: validate on blur, clear/update affected errors on edits, validate visible fields on submit, and focus the first failure. Use shared error styling and messages below their fields, including wrong-current-password errors. Whole-form banners are for save/database/file failures.

### 2.2 Enable startup unlock

1. Open Set Unlock Password when the switch is clicked.
2. Enter Password and Confirm password; allow visibility toggling, paste, and password managers.
3. Explain once that forgetting it prevents recovery of saved passwords and managed private keys.
4. Confirm to perform key transition/persistence in the background, blocking duplicate submission while keeping the window responsive.
5. After successful commit, enable the switch and show Change Password…. The current run continues; the next launch requires unlock.

Accept 6–64 Unicode characters, inclusive, including spaces. Do not require character classes, trim spaces, change case, or normalize Unicode. Confirmation must match exact input. This rule applies only to app passwords, not SSH passwords or existing key passphrases.

### 2.3 Change password

The modal contains Current password, New password, and Confirm new password. Reverify the current password even while unlocked, rather than treating the in-memory data key as identity proof.

After validation, perform the section 8 transition in the background. Success immediately protects the current vault with the new password; later launches require it. Existing connections and remote passwords are unchanged. On commit failure, the old password remains valid and no success is shown.

### 2.4 Disable startup unlock

Request the current password and explain that Burrow will stop asking for it while retaining saved credentials.

After success, turn the switch off and hide Change Password…. Do not keep the old app password as a backup entry point. Re-enabling sets a password again.

### 2.5 Startup unlock screen

When enabled, startup shows only app name, password input, visibility control, Unlock, and Quit. Focus starts in the password field; Enter submits; closing the window quits.

Do not render hosts, sessions, files, terminal history, workspace previews, or MCP audits. Menus/shortcuts allow only unlock-screen actions. Cmd+number, recent sessions, Settings, drag/drop, or other window routes cannot enter the workspace.

Unlock calculation runs in the background, one request at a time. Results arriving after cancellation/quit cannot open the app. Wrong passwords and key-wrapper authentication failures share an unable-to-unlock message. Known unreadable-file/unsupported-format failures have specific messages, avoiding a false diagnosis of definite password error for corruption.

There is no Skip, reset-while-preserving-password, or remote password-input path. Users can quit and restore an unlockable backup. Creating a new vault is a separate destructive recovery operation, never an automatic response to ordinary unlock failure.

## 3 Startup and availability

Before this change, [Run](../internal/desktop/app.go) loaded catalogs/restored workspaces and transfers before window creation, then started MCP. Main-view ticks also advanced transfers, monitoring, shortcuts, and persistence. These operations now belong to business startup after unlock.

~~~mermaid
flowchart TD
    A[Start process and acquire exclusive data lock] --> B[Open DB and read vault metadata]
    B --> C{App password required?}
    C -->|Off| D[Read local unlock key]
    C -->|On| E[Show unlock screen]
    E --> F[Derive unlock key in background]
    D --> G[Verify and unwrap data key]
    F --> G
    G -->|Failure| H[Stay locked or report specific error]
    G -->|Success| I[Initialize catalog and business state]
    I --> J[Restore workspace and paused transfers]
    J --> K[Enter workspace and start configured MCP]
~~~

### 3.1 Two-phase startup

Phase one initializes only data paths, process lock, database, vault metadata, unlock theme/language, window, and exit flow. Ordinary reads must not trigger connections, task recovery, MCP, or automatic saving.

Phase two runs once after unlock: load catalog/business preferences, restore workspace/transfers, initialize MCP, enter the workspace, then start listening if enabled. Repeated unlock clicks or window display cannot rerun it.

Restored SSH sessions remain disconnected; unfinished transfers remain paused. Unlock success does not authorize automatic connections or transfer resumption.

### 3.2 Runtime states

| State | Allowed behavior |
|---|---|
| Booting | Read required startup data; no user business work |
| Locked | Enter password, quit, or inspect explicit recovery errors |
| Unlocking | Wait for one background result; no duplicate submission |
| Initializing | Unlock succeeded; loading business state |
| Ready | Normal app use |
| ChangingProtection | Pause new credential operations and related business changes |
| Blocked | Key, format, or commit state is unusable; retain evidence and block business |
| Closing | Stop admissions, complete shutdown, release resources |

The app instance owns state, not package globals. Only Ready accepts new user work. Section 8 describes established connections during ChangingProtection.

### 3.3 Background entry points

MCP HTTP does not listen before unlock, so clients cannot connect yet. After unlock, use existing listener settings without silently changing ports. No Unlock MCP or remote password interface is added.

Connection, transfer, and MCP desktop entry points check app availability, including anonymous FTP, external agents, connection reuse, and local file work that do not require Vault secrets. Checking only credential reads is insufficient.

The locked screen does not run main-view business ticks. Protection transitions return explicit temporary unavailability to new MCP requests rather than silently queuing them for later execution. Existing MCP authentication, grants, policies, and auditing remain independent.

### 3.4 Exit while locked

[persistBeforeClose](../internal/desktop/workspace_store.go) builds snapshots using load-error flags. Exiting before unlock must not write uninitialized slices, default preferences, or an empty workspace over saved data.

The exit path distinguishes completed initialization. Before Ready, cancel calculation and release window/database resources without saving workspace/catalog/transfer/preferences snapshots. After initialization, use normal shutdown saves.

## 4 Module ownership

Add only `internal/vault`, reusing store, desktop, localfs, and protocol clients. No generic pluggable credential backend or second database.

~~~mermaid
flowchart TD
    UI[Settings and unlock UI] --> APP[Desktop state and coordination]
    MCP[MCP desktop backend] --> APP
    APP --> V[Vault credential and key lifecycle]
    V -->|Read ciphertext| S[Store database interface]
    APP -->|Commit config and prepared ciphertext together| S
    S --> DB[(burrow.db)]
    V --> L[Private local unlock key files]
    APP -->|Password or signer for this authentication| CLIENT[SSH and file protocol clients]
~~~

| Module | Ownership and responsibility |
|---|---|
| internal/vault | Current data key, unlock state, crypto epoch; credential reads, prepared encrypted changes/protection transitions, local key files |
| internal/store | Metadata/ciphertext structures, atomic reads, version checks, transactions; no plaintext passwords or KDF execution |
| internal/desktop | Vault instance, UI, availability, business snapshots, serial commits, completion notifications |
| internal/sshclient | Authentication-scoped password/in-memory signer; no DB lookup or app-unlock requests |
| internal/fileclient | Current connection's authentication material; no long-lived unlock-key cache |
| internal/localfs | Existing Unix permissions/Windows ACLs for DB and local key protection |

Vault is the unified CredentialStore capability entry point, without another identically named wrapper. It opens/unlocks, reads credentials, prepares insert/update/delete/protection changes, and closes/releases. Only prepared results enter Store transactions; business code does not construct ciphertext.

Store ciphertext types do not depend on Vault. Vault may use concrete Store read interfaces. Prepared changes include expected crypto epochs checked at commit. UI has no API to obtain data or local unlock keys.

## 5 Data and persistence

### 5.1 Encryption scope

| Content | Storage |
|---|---|
| Saved SSH/FTP/FTPS connection passwords | Encrypted credential records |
| Complete managed private key bytes | Encrypted records instead of managed key files |
| Optionally saved passphrases | Separate encrypted records |
| Public keys, fingerprints, names, types, bit counts | Existing key metadata |
| Hosts, usernames, groups, layout, checkpoints, trust | Existing business tables |
| MCP access keys | Digests/public prefixes only, no recoverable full value |
| Arbitrary audit/command/config text | Existing rules, without claiming all text is encrypted |

Unlock limits in-app access, not offline reading of an entirely encrypted database. Hosts, users, initial commands, environment variables, and audits may remain visible. Credential encryption does not replace log redaction or confidentiality of configuration contents.

### 5.2 Added tables

This design upgraded schema v3 to v4. Create a consistent pre-upgrade backup, with initialization and migration as explicit states.

The single-row `vault_meta` stores:

| Field group | Contents |
|---|---|
| Identity/version | vault_id and format version |
| Initialization | pending_init, pending_migration, or ready |
| Unlock mode | local or password, internal only |
| Encryption epoch | crypto_epoch for rotation/concurrent commit validation |
| Data key wrapper | wrap_nonce, wrapped_dek |
| Password derivation | Algorithm version, salt, memory, iterations, parallelism |
| Automatic key reference | local_key_id for local mode; format v2 additionally identifies file/keychain source |

Ready must satisfy all fields required by its mode. Password metadata has no usable local key reference; local metadata does not depend on KDF parameters. Derive the UI switch from this record rather than another independently mutable preference boolean.

`vault_items` stores:

| Field | Purpose |
|---|---|
| secret_id | Stable credential ID |
| kind | session_password, private_key, or key_passphrase |
| revision | Business credential version for connection snapshots |
| crypto_epoch | Data-key generation used by the record |
| nonce | Fresh random encryption nonce |
| ciphertext | Ciphertext and authentication tag |

Retain IDs such as `session/<id>/credential` and `key/<id>/passphrase`, adding `key/<id>/private`. Each logical credential has one current record, not password history or automatic attempts across versions.

Separate business revision from encryption epoch: changing a remote password changes PasswordRevision; rotating encryption keys does not change PasswordRevision, host/key IDs, or public fingerprints.

`managed_keys` replaces private file paths with credential references. Runtime/business snapshots retain IDs and versions, not copied passwords, private keys, or app-unlock material.

### 5.3 Automatic unlock keys

~~~text
app-data-directory/
  burrow.db
  burrow.db.lock
  vault-keys/
    <local-key-id>.key
~~~

All platforms use private local unlock key files. The macOS release reads `dev.burrow.app.vault-unlock` only for old Keychain-backed vaults: unlock successfully, durably write and read back a new file key, rotate the DEK and records, then commit and verify the file-backed generation. Keep old Keychain items for old backups; failed or denied reads do not create replacement keys or fall back to stale files. Existing v1/v2 file-backed vaults remain unchanged. Format v2 authenticates the source; v1 authenticated data remains unchanged.

Items/files contain format version, vault_id, local_key_id, and random unlock key. Filenames use app-generated IDs, not arbitrary paths from the DB.

Unix directories use 0700 and files 0600; Windows reuses private current-account/SYSTEM ACLs. Write new files under independent names with exclusive creation, make them durable, then update DB references. Do not overwrite a currently referenced file.

Local unlock keys never enter packages, source, mygo.json, logs, CLI arguments, or environment variables. Updates do not create replacements. A missing/mismatched referenced key fails explicitly rather than being hidden by a new file.

Development/release directories and vault keys remain independent. Hardcoded keys, program hashes, usernames, and machine identifiers are not decryption secrets.

## 6 Cryptography and protection boundaries

### 6.1 Two key layers

Each vault has an independent random 32-byte data encryption key (DEK) for all credential records.

The key encryption key (KEK) only wraps/unwraps the DEK:

- local: `crypto/rand` generates 32 bytes, stored in a private local key file on every platform.
- In `password` mode, Argon2id derives 32 bytes from input and a random database salt; the result is not persisted.

Both databases store only a wrapped DEK and share the same record-read flow.

~~~text
random local KEK ───────────┐
                           ├─ unwrap wrapped_dek ─ DEK ─ decrypt credentials
password + salt → Argon2id ─┘
~~~

### 6.2 Algorithms and parameters

Use existing `golang.org/x/crypto` Argon2id and `chacha20poly1305.NewX`. XChaCha20-Poly1305 uses a 32-byte key, 24-byte nonce, and 16-byte authentication tag for both data-key wrapping and credential records.

Generate keys, salts, and nonces with `crypto/rand`. Every encryption gets a fresh nonce, never derived from IDs, timestamps, or business versions. New/changed passwords get a fresh salt of at least 16 bytes.

Initial Argon2id evaluation parameters are 64 MiB memory, three iterations, parallelism four, and 32 output bytes, corresponding to RFC 9106's second recommendation. Measure unlock time on minimum supported hardware before finalizing the first-release parameters. Persist actual parameters; upgrades must not use new defaults to decrypt old data.

Vault metadata is an external input boundary. Before KDF/AEAD calls, validate supported formats, parameter combinations, lengths, and resource limits. Reject unbounded allocation, invalid nonces, or records that would panic library calls. Accept only explicitly supported parameter sets, not user-entered algorithms.

### 6.3 Identity binding

Key-wrapper authenticated data binds purpose, format, vault_id, mode, crypto_epoch, and corresponding KDF parameters or local key ID.

Record authenticated data binds purpose, format, vault_id, secret_id, kind, revision, and crypto_epoch, with explicit lengths/fixed field ordering rather than unordered-map serialization.

This detects ciphertext moved between records, purposes, or vaults. It does not detect whole-database historical rollback: an attacker holding an old complete DB and its unlock material can still use it.

### 6.4 Memory and errors

Clear controllable app-password/derived-KEK buffers promptly after unlock/transition. Normal operation retains only the current DEK. Decrypt credentials on demand, not all secrets at startup or into global catalog models.

Prefer clearable byte buffers for short-lived plaintext. Strings/signers required by protocol libraries remain authentication-scoped. Clearing input buffers does not clear every library copy or guarantee no remnants in Go GC, swap, or process dumps.

Distinguish missing credentials, business-version mismatch, locked app, integrity failure, unsupported formats, and I/O errors. Only actual absence or expired business versions use existing re-entry flows. Corrupt ciphertext must not silently become an unsaved password.

### 6.5 What is protected

Local mode protects a database-only leak, but DB plus local key file permits decryption. Malicious software under the same account with appropriate file access may read both.

Password mode adds password/KDF cost to data-directory decryption, but depends on password strength and cannot prevent process control, keylogging, or unlocked-memory reads.

Credential encryption, password mode, and file automatic unlock do not use Apple signing as a decryption secret. Only one-time legacy Keychain reads depend on OS authorization. Gatekeeper, notarization, and release signing are unchanged.

## 7 Credential operations and database consistency

[session_store.go](../internal/desktop/session_store.go) previously compensated for separate system-credential/SQLite writes. The new design commits ciphertext and business references together and removes superseded compensation after switching affected call chains.

Prepare ciphertext and a new business revision before saving a password. One Store transaction saves ciphertext, PasswordSaved, PasswordRevision, configuration, and affected snapshots. Publish corresponding in-memory state only after commit succeeds.

Deletion, Clear All Passwords, private key creation, and deletion use the same transaction boundary. Failure retains records/references; there is no intermediate UI success with missing credentials.

[SaveCatalog](../internal/store/catalog.go) rebuilds catalog table entries within transactions. New relationships must not cascade-delete credentials on every catalog save. Commit encrypted changes explicitly and order inserts/deletes for foreign keys.

Use the existing persistence worker rather than a second write queue. Encryption, user interaction, and key parsing remain outside transactions. Transactions check expected crypto_epoch and commit prepared data only.

PasswordRevision constraints remain effective. Old workspace/transfer snapshots cannot silently use a new password or bypass a mismatch by trying multiple old ciphertexts.

## 8 Protection and password transitions

### 8.1 Rotate data keys consistently

Enabling, disabling, and changing passwords share one flow: verify identity, prepare the unlock mode, generate a new DEK, re-encrypt current credentials, atomically commit metadata/all ciphertext, then update memory.

Rewrapping the same DEK could change a password, but old metadata/unlock material could still recover that DEK and current records. Credential data is limited in size, so explicit security changes and Keychain-to-file transitions rotate DEKs to sever that relationship.

This cuts old keys off from the new epoch only. Passwords/private keys already in old backups remain readable with old unlock material. Leaked remote passwords/SSH keys still require replacement; changing the app password does not revoke them.

### 8.2 Shared transition steps

1. Validate input. Disabling/changing passwords verifies the current password against current metadata.
2. Enter ChangingProtection, stop new credential reads/writes and new business admission, and wait for admitted short credential operations.
3. Establish a write boundary through existing serial commits and capture complete ciphertext/current crypto_epoch. Authenticated SSH sessions/transfers and checkpoint saves may continue; new authentication waits.
4. Generate a new DEK and KEK or password parameters. Decrypt/re-encrypt records one at a time without retaining all plaintext together.
5. For local mode, fully write, sync, and read back a private file under a new local_key_id. Password mode writes no local unlock key.
6. In one SQLite transaction, check the old epoch and replace all ciphertext/metadata, retaining credential IDs/business revisions.
7. After confirmed commit, publish the new DEK/settings/UI, release old key references, and resume admission.
8. Retire old automatic file references. Report cleanup separately without rolling back a committed epoch. Old files are not fallback unlock paths.

Users may cancel before confirming, leaving the vault unchanged. After confirmation, wait for background transition and short commit, disabling duplicate submission, cancellation, and normal quit. Do not forcibly cancel halfway and claim rollback.

SQLite cannot atomically commit filesystem changes. Prepare new files before committing references. Do not delete old files, overwrite active files, or update UI first.

### 8.3 Crash and failure outcomes

| Failure point | Authoritative restart state |
|---|---|
| Input/verification | Original password/mode |
| New local file written before DB commit | Old metadata still unlocks; new file has no active reference |
| Re-encryption before transaction commit | Original vault remains intact |
| Commit succeeds before UI update | New metadata unlocks |
| Old file cleanup fails | New metadata remains authoritative; report inactive leftovers |
| Commit/new metadata uncertain | Stay blocked and inspect DB; do not restore old metadata or guess success |

Identify orphan files only by exact operation provenance, not by scanning/deleting unknown files. Recycling old keys, databases, or backups is not secure erasure.

## 9 Managed keys and protocol integration

Store complete managed private keys as encrypted records. Import retains original bytes and existing passphrase protection. Saved original passphrases are separate encrypted records. Unsaved passphrases still prompt normally; app passwords and SSH key passphrases are distinct.

Decrypt keys into memory and build `golang.org/x/crypto/ssh` signers for direct client use. Replace path dependencies in [ssh_runtime_config.go](../internal/desktop/ssh_runtime_config.go) and [auth.go](../internal/sshclient/auth.go), without temporary-file adaptation of old APIs.

FTP/FTPS retain per-authentication passwords and existing error redaction. `ftpSecret` in [file_connections.go](../internal/desktop/file_connections.go) belongs only to the unlocked run, is not persisted in workspace/tasks, and is released on quit.

Agent discovery/use/addition also requires an unlocked app. Retain explicit options without new default export. Keys handed to an external agent belong to that agent's lifecycle; quitting Burrow or changing an app password does not invalidate them.

After private keys enter SQLite, Show in file manager cannot point at a fabricated path. Replace it with **Export Private Key…**: create a file only after explicit destination selection, preserve original passphrase protection, use private permissions, and confirm overwrite. Normal connections create no private key files.

Show managed status rather than internal ciphertext IDs. Retain public key copying, fingerprints, renaming, and deletion prevention while referenced. Deleting a managed key removes ciphertext/metadata and is explicitly irreversible through the app, rather than claiming to move a file to Trash.

Do not move/delete external import source files automatically. Old managed files/backups may contain original private keys; migration does not improve those copies' protection.

## 10 One-time migration

### 10.1 Scope

Retain catalogs, IDs, groups, settings, workspaces, transfers, trust, MCP configuration, and audits. Migrate only system credentials, managed private keys, and storage references.

Release/development read their original namespaces independently. Query only known credential IDs from the DB, without enumerating/importing other apps' secrets. Each platform's existing provider is a one-time source.

### 10.2 Separate schema upgrades from external reads

Under the exclusive data-directory lock, create a recoverable consistent DB backup before schema upgrades, then add tables/explicit migration markers. Existing schema v3 enters pending_migration; truly new vaults enter pending_init.

Store initialization transactions do not call credential services, show authorization, or read key files. Desktop migration coordinates external reads.

Missing required metadata/key files in a ready vault mean corruption or incomplete recovery, not first launch. Pending states do not start business services either.

### 10.3 Steps

1. Build a password/key/saved-passphrase manifest from saved entries/references, retaining business versions.
2. Explain the one-time read of old system credentials and their removal from normal operation. macOS may still request old Keychain authorization during migration.
3. Read credentials/managed keys and verify format/public fingerprints. Expired snapshots retain version mismatch behavior; do not relabel current passwords as old revisions.
4. Create independent Vault keys, prepare ciphertext in default local mode, and durably save new unlock material.
5. Commit metadata, ciphertext, new private key references, and ready state in one transaction.
6. Read back metadata/records, verify decryption, counts, and public keys, then enter normal startup.

When key verification needs an original passphrase, temporary input is sufficient. Saving remains the user's existing choice. Do not skip unverifiable keys or force passphrase retention to complete migration.

List missing saved passwords, unreadable keys, and unexplained versions explicitly. Users can repair/re-enter data. Discarding old credentials requires explicit confirmation, never silent skips, empty-string migration, or altered saved flags to manufacture success.

The switch remains off after migration; users can enable startup unlock afterward. Migration does not force password creation.

### 10.4 Failure and retained copies

Failure before commit leaves original system credentials/managed files unchanged. An unreferenced prepared key file does not change original ownership. Failure to verify after commit preserves evidence and blocks business rather than silently switching back to old providers.

Old programs must reject unsupported schemas. Downgrading requires restoring the pre-upgrade database and matching files; old versions cannot continue writing the new vault.

Migration does not delete system credentials, old private keys, `.pub` files, or external originals. Cleanup is a separately confirmed action after verification. Leftover copies keep their old access paths, including directly readable unencrypted old keys; migration results must explain this.

Adapters serve pending_migration only. Ready has no credential fallback, dual writes, or alternate-source retries after reads fail.

## 11 Backup and restore

Reuse [Store.Backup](../internal/store/store.go) for consistent SQLite snapshots. No general backup page is added, but storage documentation and caller contracts must change.

| Current mode | Required backup contents | Restore requirement |
|---|---|---|
| local | Vault.Backup DB snapshot and independent file key, without the original machine's Keychain | Both complete and matching |
| password | Consistent DB snapshot | Password corresponding to that backup |

Local backup parts together form an automatically unlockable secret and require protection accordingly. A DB-only backup lacks automatic unlock; restore cannot recreate the original key.

Serialize backups with protection transitions. With memory key and DB snapshot in the same epoch, create an independent file key and rewrap backup metadata while retaining record revisions/epochs. Do not mix key files between snapshots. Password backups do not gather inactive old automatic files.

Copying the whole directory after normal quit is still possible, but old copies may remain and actual contents determine protection. Copying a live `burrow.db` without WAL is invalid.

Cross-machine restore uses existing directory permission protection, not original hardware binding. Managed private keys restore with the vault; actual transferred files, external originals, and external agent state remain outside database backups.

Old backups use their original unlock material. Changing passwords or disabling startup unlock does not rewrite them or provide a password-recovery backdoor.

## 12 Affected implementation locations

| Location | Change |
|---|---|
| [app.go](../internal/desktop/app.go) | Two-phase startup, availability, locked routing, exit branches, once-only business initialization |
| [settings.go](../internal/desktop/settings.go), [credentials_ui.go](../internal/desktop/credentials_ui.go), unlock module, `internal/locales` | Unlock group/switch/password forms, search/languages, managed-key menus |
| internal/vault | Crypto, metadata validation, key loading, record access, transitions, short-lived memory |
| [store.go](../internal/store/store.go) and schema upgrade | Pre-upgrade backup, initialization markers, metadata/ciphertext tables, format constraints |
| [catalog.go](../internal/store/catalog.go) | Joint config/ciphertext transactions and references, without cascade deletion during catalog rebuild |
| [data_paths.go](../internal/desktop/data_paths.go) | Release/development isolation; normal paths stop returning mygo.Secrets services |
| [session_store.go](../internal/desktop/session_store.go), [credentials.go](../internal/desktop/credentials.go) | Unified access, business versions, removal of superseded cross-store compensation |
| [ssh_runtime_config.go](../internal/desktop/ssh_runtime_config.go), [auth.go](../internal/sshclient/auth.go) | In-memory authentication instead of paths, retaining interaction/trust |
| [file_connections.go](../internal/desktop/file_connections.go), [transfer_backend.go](../internal/desktop/transfer_backend.go) | Unlock admission/cache lifecycle without changing pause/recovery |
| [mcp_data.go](../internal/desktop/mcp_data.go), [mcp_backend.go](../internal/desktop/mcp_backend.go) | Post-unlock initialization/listening, admission, temporary transition unavailability |
| [workspace_store.go](../internal/desktop/workspace_store.go), [persistence.go](../internal/desktop/persistence.go) | No empty snapshots before unlock, existing serial writes with epoch checks |
| One-time migration adapters | Old provider reads only in pending_migration, outside normal access |
| [storage.md](storage.md), [session-settings.md](session-settings.md), [architecture.md](architecture.md), integration scripts | Current behavior, paired backups, isolated test credential initialization |

No unrelated dependency upgrades, token redesign, whole-database encryption, installer signing, or general password-manager expansion.

## 13 Implementation sequence

| Stage | Deliverable | Completion condition |
|---|---|---|
| 1 Storage/crypto core | Schema, Vault, local initialization, password unlock, epochs | Create/read/recover in isolated data; failures do not overwrite |
| 2 Credentials/keys | Unified ciphertext, joint config transactions, in-memory SSH signers | No normal system-credential reads or connection-time key files |
| 3 Startup/settings | One default-off switch, password creation/change, two phases, MCP/exit admission | No locked business access; locked exit preserves saved state |
| 4 Transitions/migration | Three protection changes, crash convergence, migration/copy notices | Defined recovery at every commit boundary; no dual writes/implicit fallback |
| 5 Integration/docs | Protocol regression, backups, permissions, startup checks | Record passed/failed/unverified matrix items |

Together these form one complete feature. Do not ship forced storage changes without migration, exit protection, or backup instructions.

## 14 Acceptance matrix

This remains the full matrix. Existing race tests and isolated encryption, rotation, backup/restore, and migration checks passed. They do not establish every native/protocol case. New automated cases and native UI acceptance require explicit validation authorization.

### 14.1 User behavior

| Scenario | Expected |
|---|---|
| New install, first saved credential, restart | Default off; no app password prompt; credential usable |
| Cancel enabling or mismatch confirmation | Still off; original credentials usable |
| Enable, quit, restart | Unlock screen first; correct password enters app |
| Wrong password, repeated clicks, quit during derivation | No business entry/parallel unlock; late results cannot open app |
| Foreground/show after unlock | No repeat prompt |
| Change password and restart | New password works; old cannot unlock current vault |
| Wrong current password on change/disable | State/old password unchanged |
| Disable and restart | Automatic entry; credentials retained; Change Password hidden |
| Upgrade binary with retained data | Same unlock requirements/credentials; no ordinary legacy credential authorization |

### 14.2 App and protocol boundaries

| Scenario | Expected |
|---|---|
| Menus/shortcuts/drag/window activation while locked | No workspace access or connection |
| MCP previously enabled while locked | No listener; starts configured after unlock |
| Anonymous FTP, external agent, local files | Same availability constraints |
| Workspace restoration after unlock | No new automatic SSH connections; unfinished transfers paused |
| Quit before unlock and restart | Catalog/preferences/workspace/transfers preserved |
| Password/key/unsaved passphrase/jump chain/interactive SSH | Existing authentication behavior |
| SFTP/FTP/explicit and implicit FTPS/concurrency | Unified credentials, existing redaction/lifecycle |
| Old snapshots after password change | Version mismatch, not new password substituted as old |
| Protection change with active work | Authenticated work continues; new authentication pauses; checkpoints retained |

### 14.3 Key and data consistency

| Scenario | Expected |
|---|---|
| Inspect temporary fixture DB/WAL | No fixture plaintext passwords/keys/passphrases; no scan of real secrets |
| Tamper ciphertext/nonce/purpose/ID/epoch | Integrity/format failure; no silent rebuild |
| Invalid KDF/missing key/mismatched ID | Bounded failure; no replacement keys or business startup |
| DB write failure during config/delete/key creation | Ciphertext and references remain transactionally consistent |
| Crash at transition steps | Section 8.3 outcomes, without old-key fallback |
| Use old unlock material on new epoch | Cannot decrypt new ciphertext; old backups remain readable with their own material |
| Missing credentials/denied Keychain/unreadable keys | Explicit migration blocker; no fabricated success |
| Cross-machine local/password backups | Restore with complete matching material; explicit failure otherwise |
| Concurrent release/development and second process | Isolated keys and effective exclusive lock |
| Unix/Windows permissions | Current-account constraints and explicit failures |

Build/static checks do not replace native startup, permissions, system-credential migration, or protocol acceptance. Implementation checks used temporary fixtures only, not real credentials, system Keychain, or native visuals.

## 15 Design references

- [Current data and credentials](storage.md): SQLite, unified encryption, one-time migration, directories, and backups.
- [RFC 9106 parameter selection](https://www.rfc-editor.org/rfc/rfc9106.html#section-4): Argon2id evaluation parameters.
- [Go chacha20poly1305](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305): NewX key, nonce, and AEAD contracts.
- [OWASP cryptographic storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html#encrypting-stored-keys): separating data/unlock keys and password-derived KEKs.
- [Apple code signing and Keychain](https://developer.apple.com/library/archive/technotes/tn2206/_index.html): signing-related access identity background.

See [macOS signing and unlock](macos-signing.md) for fixed identity, deployment, and actual OS permission validation.
