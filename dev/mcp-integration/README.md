# Real MCP protocol and task lifecycle integration

[English](README.md) | [简体中文](README.zh.md)

The driver traverses real local MCP HTTP, production `mcpDesktop`, native window threads, connection management, and transfer queues to isolated Docker SSH/SFTP/FTP/FTPS services. Independent server reads and SHA-256 verify content. Builds use `-race`, with no substitute backend, screenshots, or visual acceptance.

## Start fixtures

Follow [local protocol integration](../ssh-integration/README.md). Set `BURROW_TEST_DIR` from the final JSON directory and set `BURROW_TEST_ADDRESS`. Temporary credentials/certificates stay in the `0700` fixture directory and must not be committed.

```sh
python3 -B scripts/ssh-integration-server.py
python3 -B scripts/ssh-integration-server.py --ftp
python3 -B scripts/ssh-integration-server.py --ftps
python3 -B scripts/ssh-integration-server.py --ftps --implicit
```

FTPS verifies the temporary CA and hostname without installing system certificates or using `InsecureSkipVerify`. Only the test overlay injects that run's CA into existing `FTPConfig.TLSConfig`. Passive ports bind to loopback. Repeated cancellation briefly occupies several data connections, so fixtures reserve enough ports; port exhaustion must not be reported as a product resume failure.

## Run native MCP scenarios

Build a current-platform `Burrow.app` resource template or pass an existing bundle with `--template`. An unlocked, focusable macOS session is required:

```sh
GOCACHE=/private/tmp/burrow-go-build mise exec -- python3 -B -u scripts/mcp-integration-ui.py \
  --fixture "$BURROW_TEST_DIR" \
  --template /absolute/path/to/Burrow.app
```

Production UI stays unchanged. The native overlay supplies isolated SQLite data, a temporary credential provider, and test-build-only menus. Pause/resume also use actual MCP detail buttons. Protocol calls include full version metadata/authentication headers. Diagnostics log FTP verbs/response codes only, never arguments, passwords, keys, or file bodies.

Coverage includes first SSH trust; output/exit status; timeout and HTTP-disconnect cancellation for new/old MCP versions; no PTY/terminal initial command; command/file policies and historical snapshots; all four file protocols, overwrite and directory transfers; task discovery after disconnecting before decoding an ID; native pause/resume, queued/running cancellation, cleanup after source directory loss; key disable/revoke, grant reduction, cross-key isolation; service stop/start; audit failure blocking new execution while preserving cancellation; and offline recovery, manual resume, and content verification after normal quit or forced crash.

After the basic SSH cases it also switches to the SSH page and checks that the read-only AI session tab opened in the background lists the agent's commands, output, exit codes and timeouts, with the name of the key operating it.

Each run writes `mcp-native-run-*/report.json` under the fixture directory. Failure retains phase logs and read-only node/model diagnostics. Build success, app startup, or partial scenario success does not establish complete integration. `--reuse-run` is only for investigating a known issue in an already isolated config; final full acceptance requires fresh data. `--scenario lifecycle` narrows lifecycle reproductions.

`--scenario recovery` checks normal/forced-crash recovery separately. Its report names the selected scenario and does not establish others.

Run separately from protocol Docker tests to avoid counter/port interference. Stop only exact container names from this run's metadata, never unrelated containers or user sites.

Source compatibility checked on 2026-11-07: the shared SSH probe and startup/terminal overlays were adapted to current localized message types, and both complete overlays built with `-race`. These were compilation checks with temporary fixtures, not a new native/remote E2E run. The results below remain historical records.

## Validation completed on 2026-11-03

The default `all` scenario passed **30/30** on macOS arm64 with fresh isolated SQLite/credential directories. Real MCP HTTP used cancellation semantics for `2026-07-28` and `2025-06-18`. Servers were local OpenSSH/internal-sftp and vsftpd. Explicit/implicit FTPS retained certificate, hostname, and data-channel session reuse checks.

| Area | Observed in that run |
|---|---|
| SSH | First trust, no PTY/terminal initial side effects, separate stdout/stderr, nonzero exit code, timeout, and HTTP-disconnect cancellation under both versions |
| Grants/policies | Remote command side effects blocked; snapshots survive rule deletion; SFTP create/overwrite distinction and REALPATH aliases protected; read-only/local source scope denial; policy tightening during slow reads prevents body return |
| Four file protocols | UTF-8 lists/text reads/writes; create conflict does not overwrite; replacement; directory transfers with Chinese names, nesting, and empty files; independent server SHA-256; untrusted FTPS certificate rejection |
| Tasks/revocation | Accepted tasks discoverable before response decoding; real detail-button pause/resume; queued/running cancel; remote cleanup after local source loss; key disable/enable/revoke; grant reduction cancels affected tasks only; other keys/connections remain independent |
| Service/audit | Stop saves paused tasks; restart does not resume automatically; injected failures prevent SSH effects without start audits while still allowing own-task cancellation and reporting unpersisted audit |
| App recovery | Normal restart retains private FTP overwrite staging and original `0600` destination before manual resume, with matching final digest; forced-crash restart stays offline and matches digest after manual resume |

Full report: `/private/tmp/sshdesk-integration-lj9pctsy/mcp-native-run-sm2x49hr/report.json`, `status=passed`, `scenario=all`. Four `native-app-*.log` files in that directory had no Go race reports. Independent fixture-capacity evidence is `/private/tmp/sshdesk-integration-lj9pctsy/ftp-passive-capacity-evidence.json`. The old five-port fixture and failed part were retained, without automatically retrying it as a product defect.

Also passed: `go test -race ./...`, `go vet ./...`, local macOS build, Windows amd64 cross-build, and all `internal/sshclient` / `internal/fileclient` regressions with `-race -tags=integration` (9.250 s and 34.417 s in that run). Additional coverage included wrong CA/hostname rejection, FTP private-directory takeover prevention and ownership after restricted-permission failure, stable partial-upload verification/resume, and 16 concurrent passive listeners.

Native Windows, third-party AI client UIs, real system Keychain, other servers, and production scale were not covered. Native control used existing overlays and did not establish visual acceptance.

## Request/response audit enhancement on 2026-11-04

The earlier MCP implementation was committed as `9bc13ca`. This enhancement added schema v3 `mcp_payloads` for requests/structured responses, with multiline viewing/copy of commands, stdout/stderr, and JSON. File bodies/authentication data are excluded, and older summary-only records are distinguished.

A temporary overlay with real local HTTP, the official SDK, and isolated SQLite checked old-version initialization, new-version discovery/ping/calls, unknown methods, invalid arguments, single audit per tool call, retained command output, omitted file bodies, and exclusion of authentication fields/known MCP keys. The business adapter used fixed SSH output and local file clients; this does not replace real remote SSH/FTP acceptance.

Database checks covered fresh/v1/v2/v3 upgrades/reopen, retained settings, independent request/response updates, later summaries not clearing bodies, explicit JSON null, cascading cleanup, and rollback of start records after detail-write failure. Fault injection confirmed own-task cancellation remained available after audit admission failure and returned `audit_persisted=false` explicitly.

The Mac was locked when native regression started. Per the user's instruction, native windows and full Docker task regression were skipped. The earlier 30/30 result was not reused as evidence for this enhancement.
