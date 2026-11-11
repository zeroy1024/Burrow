# Development and builds

[English](development.md) | [简体中文](development.zh.md)

## Environment and running

Install mise and run from the repository root. See [platform support](platform-support.md) for actual runtime acceptance by system:

```sh
mise install
mise exec -- go run .
```

[mise.toml](../mise.toml) pins Go 1.27.3 and uv 0.13.6. [go.mod](../go.mod) pins MyGo v0.7.3 and project dependencies. Keep these versions and the existing dependency workflow.

Ordinary `go run` uses Burrow Dev data and a separate credential namespace; release packages use Burrow. See [data and credentials](storage.md) for data models, backups, and legacy-format boundaries. SQLite uses pinned `modernc.org/sqlite v1.61.1`, without system SQLite or CGo.

The first run/build downloads the pinned terminal plugin's native Ghostty VT libraries; later runs use caches. The main module is `github.com/zeroy1024/Burrow`, so plain `go run .` generates an executable named `Burrow`. The app still uses the separate Burrow Dev data namespace. The local terminal fork is `github.com/egoist/mygo/burrow-terminal`; its MyGo prefix permits the existing internal SDK imports.

## UI icons

Shared icons use pinned Lucide SVGs. See [Lucide assets](../internal/uikit/icons/lucide/README.md) for sources and licenses. Use `uikit.IconName`, `IconSVG`, `Icon`, and `IconButton` with theme dimensions, colors, and stroke settings instead of defining general SVG paths in pages. App, OS, and MCP brand marks remain separate.

The macOS menu bar uses [tray-template.svg](../assets/tray-template.svg), a monochrome Burrow mark with transparent terminal details. AppKit tints its black-and-transparent template for the menu bar appearance and selection. The embedded PNG is 72×72 pixels, displayed at 18×18 points; Windows and Linux use the app icon. After editing the SVG, regenerate the PNG with `mise exec -- uv run --script scripts/render-tray-icon.py`; uv supplies the pinned resvg renderer for this development command.

## Packaging

```sh
mise run build
```

This calls [scripts/build.py](../scripts/build.py), targeting the host OS/architecture by default. Multiple targets are supported:

```sh
mise run build -- --platform windows/amd64
mise run build -- --platform windows/amd64,windows/arm64
```

Targets are `darwin`, `windows`, and `linux` on `amd64` / `arm64`. Full macOS packaging must run on macOS using [scripts/build-macos.py](../scripts/build-macos.py): MyGo builds the app, pinned `dmgbuild==1.6.7` creates the DMG, and system `ditto` creates ZIPs. uv prepares compatible Python (3.10+) and script dependencies on first use.

Name, slug, version, and output directory come from [mygo.json](../mygo.json). The current 0.1.6 Apple Silicon example is:

```text
build/darwin-arm64/Burrow.app
build/darwin-arm64/burrow-0.1.6-arm64.zip
build/darwin-arm64/burrow-0.1.6-arm64.dmg
```

Windows packaging requires UPX 5.3.0+ on the host PATH for Windows ARM64 support. The build temporarily compiles a [Go command proxy](../scripts/upx-go/main.go), which runs UPX LZMA compression and integrity verification after MyGo's Windows `go build`, before signing/installer creation. Icons and relocation information are retained; `ghostty-vt.dll` is unchanged. Neither the Go toolchain nor MyGo source is modified. The proxy is added only to child-process PATH during the build.

Local builds also generate a portable ZIP containing the compressed EXE and `ghostty-vt.dll`:

```text
build/windows-amd64/Burrow.exe
build/windows-amd64/ghostty-vt.dll
build/windows-amd64/burrow-0.1.6-windows-amd64.zip
```

On Windows, MyGo prepares NSIS. Cross-building installers from macOS/Linux requires `makensis` on the host. Without NSIS, EXE/portable ZIP outputs remain available, but the missing installer is explicitly reported and does not count as installer validation. The installer is `burrow-setup-0.1.6-amd64.exe`; ARM64 uses corresponding `arm64` paths/names.

macOS release packages use the fixed self-signed identity in [mygo.json](../mygo.json). The public certificate is [assets/signing/burrow-local.pem](../assets/signing/burrow-local.pem); the private key is managed by the default user Keychain. First use of a new key may require personal codesign authorization. Full packaging checks the identity first and fails if the original private key is missing, without generating another identity or falling back to temporary signing. Other build machines must import the same identity. See [macOS signing and unlock](macos-signing.md).

Build only the app bundle:

```sh
mise exec -- go tool mygo build -skip-dmg
```

`mygo.json` installs root `LICENSE` and complete `THIRD_PARTY_NOTICES.md` into app resources: `.app/Contents/Resources/` on macOS, beside the executable on Windows/Linux. After dependency license changes, run `python3 scripts/licenses.py` before building. GitHub CI checks the notice summary and packaged resources.

DMGs use the light [assets/dmg/background.tiff](../assets/dmg/background.tiff) and Applications shortcut for drag-to-install. ZIPs retain bundle structure and resources. `mygo.json` is the packaged version source. The Settings footer, About/copy action, and MCP server identity read the same runtime package version. Plain `go run` has no package metadata and displays `dev`.

## Existing checks

After changing UI copy, run `mise run i18n` and commit catalogs/generated output together. See [interface languages](i18n.md).

From the repository root:

```sh
mise run i18n
mise exec -- go test ./...
MYGO_LOCALE=en mise exec -- go test ./...
mise exec -- go test -race ./...
mise exec -- go vet ./...
mise exec -- go build ./...
git diff --check
```

The terminal extension is a nested module, so root `./...` does not run its tests. When modifying it, also run:

```sh
mise exec -- go -C third_party/mygo-terminal test ./...
mise exec -- go -C third_party/mygo-terminal test -race ./...
mise exec -- go -C third_party/mygo-terminal vet ./...
mise exec -- go -C third_party/mygo-terminal build ./...
```

Use `gofmt -l` for a read-only check of changed Go files. Do not format unrelated work across the repository. After staging, run `git diff --cached --check` to include added files.

If a restricted environment cannot write the default Go build cache, use a task-specific directory, for example:

```sh
GOCACHE=/private/tmp/burrow-go-build mise exec -- go test ./...
```

Passing checks establish only the scope actually run. See [interpreting validation](limitations.md#interpreting-validation) for protocol, native window, and visual distinctions.

## GitHub CI

[Commit checks](../.github/workflows/check.yml) run on branch pushes, PRs, and manual dispatch using hosted `ubuntu-24.04`. They retain Gitea's Go formatting, Python syntax, existing root/terminal race tests and vet, Linux build, and Windows cross-builds for both architectures. They also check third-party notices, English/Chinese catalogs, and generated-file consistency. Pinned Gitleaks, verified by SHA-256, scans working files and all checked-out history references. Checkout retains full history without persisted credentials. The workflow grants only `contents: read`, uses ordinary `pull_request`, and needs no custom repository Secrets.

[Packaging](../.github/workflows/package.yml) runs on `v*` tag pushes or manual dispatch naming an existing version tag. Both check out that tag and require it to match `mygo.json`, such as `0.1.6` / `v0.1.6`. Manual dispatch can rebuild an existing version after workflow fixes without moving its tag. Hosted `ubuntu-24.04` cross-builds UPX NSIS installers for Windows amd64/arm64 and extracts them with the `7zip` package's `7z` for verification. Apple Silicon `macos-15` builds the ARM64 DMG. Installer checks include EXE, DLL, and both license files; macOS checks bundle license resources and DMG. Artifacts include `SHA256SUMS`, remain for 14 days, and do not create/update GitHub Releases.

GitHub needs no self-hosted runner registration. Configure two repository **Settings → Secrets and variables → Actions** Secrets for macOS packaging:

| Secret | Contents |
|---|---|
| `MACOS_CERTIFICATE_BASE64` | Base64 of the encrypted PKCS#12 (`.p12`) export containing the fixed certificate and private key |
| `MACOS_CERTIFICATE_PASSWORD` | Export password for that `.p12` |

Export the original identity from the local Keychain instead of creating a certificate. See [signing and unlock](macos-signing.md) for export/storage requirements. Set the Base64 and password separately in Secrets, never code/logs. Configuring them entrusts the encrypted private key to GitHub and its build jobs and must be an explicit maintainer action. Ordinary branch/PR checks do not need them.

Tag jobs import the identity into a temporary Keychain under `$RUNNER_TEMP` using a random Keychain password, then run `scripts/setup-macos-signing.py --check` against `mygo.json`. Missing Secrets or an incorrect identity fail explicitly without replacement identities or temporary-signing fallback. Hosted VMs are destroyed afterward and signing material is not uploaded. Restrict creation of `v*` tags in GitHub. The temporary Keychain approach follows [GitHub's guidance](https://docs.github.com/en/actions/how-tos/deploy/deploy-to-third-party-platforms/sign-xcode-applications).

Actions are pinned to commit SHAs and use `mise.toml`. Hosted Ubuntu uses `sudo` for APT, not Gitea's rootless-container workaround. Native windows, Docker protocol integration, Windows installation/upgrade/uninstallation, signing, and notarization require separate acceptance; workflow success alone does not establish them.

### Before publication and release

GitHub Release notes must include a complete English section first, followed by the equivalent Simplified Chinese section. Keep changes, downloads, validation, and limitations consistent between both sections. Apply this order to every release.

- Update the version in `mygo.json`, and check notices with `python3 scripts/licenses.py --check`.
- Use Gitleaks 8.30.1 or a compatible version to scan intended public history/files locally. Inspect all local refs for stashes, internal snapshots, or unprepared branches. Keep fully redacted results outside the repository.

  ```sh
  gitleaks git --log-opts='HEAD --full-history' --redact=100 --no-banner .
  gitleaks git --log-opts='--all --full-history' --redact=100 --no-banner .
  gitleaks dir --redact=100 --no-banner .
  ```

- Manually review screenshots, binary assets, author emails, historical messages, and diagnostic publication scope. Regex scanning cannot prove they are nonsensitive. Do not publish SQLite databases, private keys, Keychain contents, or raw MCP audits as issue attachments.
- Enable private vulnerability reporting, Secret scanning / Push protection, and required main-branch checks in GitHub. Local files do not configure these repository settings automatically.
- Initially push only selected branches and required tags; avoid `--mirror` exposing internal refs. After checking GitHub results/artifacts, manually prepare Release attachments and notes with actual platform acceptance and signing status.

## Gitea CI

Checks and Windows packaging use the existing `ubuntu-latest` runner and prepare tools in job containers. macOS DMG uses a native `macos-arm64` host runner, requiring registration of an Apple Silicon machine labeled `macos-arm64:host`, with Xcode Command Line Tools and Node for JavaScript actions. This runner remains unconfigured; macOS jobs wait for a match until it is online. mise reads pinned Go/uv from `mise.toml`. Actions use commit pins; mise itself is pinned to 2026.10.25.

The rootless runner restricts UID/GID switching. APT uses `APT::Sandbox::User=root` and job-local `RUNNER_TEMP` caches, avoiding `_apt` switching or image-owned `_apt` directories without extra container capabilities or host permission changes.

| Workflow | Trigger | Contents |
|---|---|---|
| [Commit checks](../.gitea/workflows/check.yml) | Branch pushes, PRs, manual dispatch; not tag pushes | App/terminal Go formatting, Python syntax, language resources/generated-file consistency, existing race tests, vet, Linux build, and Windows checks for both architectures |
| [Packaging](../.gitea/workflows/package.yml) | `v*` tag pushes only | UPX NSIS installers for Windows amd64/arm64, macOS ARM64 DMG, and SHA-256 sums |

`pkg-sftp` retains upstream formatting and is excluded from formatting gates. Its nested module disables only vet's `stdmethods`: upstream `RawPacket.ReadFrom` and `RequestPacket.ReadFrom` take buffers/length limits and are not `io.ReaderFrom`, producing false positives. Other analyzers remain enabled; app/terminal vet stays complete. Native UI, visual acceptance, and Docker-dependent SSH/FTP integration are outside basic commit checks.

Windows jobs install NSIS and 7-Zip, download official pinned UPX 5.3.0 Linux tools, verify the preset SHA-256, and call the same `mise run build`. They extract installers for both architectures and verify EXE/DLL bytes against the current build before creating `SHA256SUMS`. A missing installer fails the job; a portable package alone is not a completed installer build.

macOS retains APP signing, LZMA DMG, and ZIP generation, without UPX or transparent filesystem compression. CI uploads only verified DMGs and sums.

Artifacts are named `burrow-windows-amd64`, `burrow-windows-arm64`, and `burrow-macos-arm64`, containing installers/DMGs plus sums for 14 days. Local portable ZIPs are not uploaded. Workflows do not create Releases or publish to external repositories. Upload uses Runner 5's action compatibility layer; retain default `runner.patch_actions: true`. Tags do not change `mygo.json`; update the project version before tagging.

UPX integrity/installer-content checks do not replace native Windows startup, DLL loading, installation, upgrades, uninstallation, or antivirus checks. macOS host-runner DMG build/install must be validated after it comes online. MSI is not integrated.

Cross-build/static checks from macOS/Linux include:

```sh
GOOS=windows GOARCH=amd64 mise exec -- go build ./...
GOOS=windows GOARCH=amd64 mise exec -- go vet ./...
```

Cross-compilation cannot run native Windows tests. Run existing checks and the [native checklist](platform-support.md#windows-native-acceptance) on Windows separately. Current automated window drivers remain macOS-specific.

## Protocol and native E2E

[dev/ssh-integration](../dev/ssh-integration/) and [dev/ftp-integration](../dev/ftp-integration/) provide Docker fixtures through [scripts/ssh-integration-server.py](../scripts/ssh-integration-server.py). They use random local ports and temporary credentials rather than existing private sites.

After setting fixture-provided `BURROW_TEST_ADDRESS`, `BURROW_TEST_DIR`, and, when needed, `BURROW_FTP_ADDRESS`, run:

```sh
mise exec -- go test -race -tags=integration ./... -count=1 -timeout=180s
```

The native driver is `python3 -B -u scripts/ssh-integration-ui.py`, with `--concurrent-split`, `--file-protocols`, and `--core-e2e`. It requires an unlocked macOS graphical session that can receive keyboard focus. Run separately from SDK Docker tests so connection counters do not interfere.

Arguments, isolation, assertions, and fixture shutdown are maintained in [integration and native E2E](../dev/ssh-integration/README.md).

## Where to make changes

See [architecture and lifecycle](architecture.md) for ownership. Use the [theme generation workflow](theme-integration.md#edit-and-generate) rather than editing derived files; read `FORK.md` before modifying local dependencies. Documents describe current behavior. Record actual commits, environments, and scope with validation results; see [interpreting validation](limitations.md#interpreting-validation).
