# Platform support and acceptance

[English](platform-support.md) | [简体中文](platform-support.zh.md)

SSH, transfer queues, sessions, and persistence share implementations. Platform files and MyGo native interfaces isolate OS differences. Successful builds, startup, and functional acceptance are recorded separately.

## Current targets

| System / architecture | Builds and outputs | Native validation status |
|---|---|---|
| macOS arm64 | APP, DMG, ZIP; macOS 13 baseline; tag CI uploads a DMG | Existing tests/race, static checks, and standard packaging; GitHub-hosted builds require signing Secrets and remote verification; Gitea host runner configuration remains outstanding |
| macOS amd64 | Retained by the same build entry point | Not run on an Intel machine in this validation |
| Windows amd64 / arm64 | Tag CI generates UPX-compressed NSIS installers with matching `ghostty-vt.dll`; local builds retain portable ZIPs | No native Windows runtime results; minimum OS version, UPX compatibility, and antivirus behavior need native acceptance |
| Linux amd64 | Target compilation checked; build entry point reuses MyGo | Distribution dependencies, X11/Wayland, system credentials, and desktop interactions unverified |
| Linux arm64 | Selectable build target | Not validated in this run |

See [development and builds](development.md) for commands and output paths. Windows packaging hosts require UPX 5.3.0 or later. Native Windows builds have MyGo prepare NSIS; cross-building installers requires `makensis` on the host. GitHub tag workflows use hosted `ubuntu-24.04` and Apple Silicon `macos-15`, importing the macOS signing identity from repository Secrets into a temporary Keychain. Gitea retains `ubuntu-latest` and a separately configured `macos-arm64:host` runner. Workflow configuration is not evidence of successful tag execution, artifact upload, or native runtime acceptance; record remote results separately.

## Integrated platform differences

- Local metadata isolates Unix uid/gid/mode and Windows ACLs without referencing nonexistent `syscall.Stat_t` fields.
- Windows private files are created with a protected DACL for the current account and SYSTEM. Existing staging files have access restricted before writing. Download overwrite copies and verifies the destination DACL first.
- Opens and renames use retained directory/file handles rather than check-then-rename for no-overwrite commits. Filename components are validated for the destination system.
- Agent authentication and detection use the same platform entry point: Unix socket or local Windows named pipe, preserving timeout, closure, and cancellation behavior.
- SQLite uses a pure Go driver. Windows data resides in `%LOCALAPPDATA%`, separate from caches. Development/release data and credential services are isolated.
- System credential access uses MyGo. Shortcuts, native window-button insets, and ordinary window closing are handled per platform.

Windows named pipes use pinned `go-winio v0.6.4`; its license appears in About. The main MyGo and Ghostty versions were not upgraded.

## Interface language validation

The current i18n change passed existing tests, race, English-environment tests, vet, local builds, and Windows amd64/arm64 cross-builds and vet. Catalog checks cover English/Chinese keys, arguments, and missed source text. This does not establish acceptance of native language switching, English layouts, system dialogs, or accessibility. See [interface languages](i18n.md) for maintenance and checks.

## Windows native acceptance

The following still require an actual Windows session; cross-compilation cannot replace them:

- Start the EXE or portable package; check DLL loading, scaling, window buttons, minimizing, and taskbar restoration.
- SSH password, plain/encrypted private key, agent, first trust, and changed-host-key rejection.
- Terminal input, IME, copy/paste, split focus, resize, and isolated pane closure.
- SFTP/FTP upload/download, new destination commit, overwrite and Keep both, permission/file-sharing errors, pause/resume/cancel, and recovery after quitting.
- Private key/staging ACLs, destination ACLs after overwrite, path replacement, reserved names, and unsupported filename errors.
- Credential Manager access, SQLite locks/WAL/private ACLs, normal shutdown, and offline restoration after restart.
- Installation, shortcuts, upgrade, uninstallation, and retained data using the generated installer.

Existing native E2E launchers use macOS `.app` bundles and an automation overlay; they have not been ported to Windows. Record the actual commit, OS version, architecture, and uncovered cases. See [interpreting validation](limitations.md#interpreting-validation).
