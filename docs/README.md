# Documentation index

[English](README.md) | [简体中文](README.zh.md)

This directory describes how Burrow currently works and how to develop it. Documentation follows the code and distinguishes implemented behavior, plans, validation actually performed, and outstanding acceptance.

## Using the app

| Document | Contents |
|---|---|
| [First steps](getting-started.md) | Running the app, the first SSH connection, file sites, and workspace operations |
| [Platform support](platform-support.md) | macOS, Windows, and Linux outputs and native acceptance status |
| [Sessions and settings](session-settings.md) | Catalogs, authentication, advanced options, shortcuts, and app preferences |
| [WireGuard gateways](wireguard.md) | App-local tunnels, configuration import, DNS, credentials, and route constraints |
| [Terminal and color schemes](terminal-themes.md) | Fonts, colors, cursor, scrollback, and terminal rendering |
| [File management and transfers](file-transfer.md) | Uploads, downloads, conflicts, task ownership, resume, and recovery |
| [Linux monitoring](monitoring.md) | Metrics, sampling scope, and unavailable states |
| [Data and credentials](storage.md) | Data directories, encrypted credentials, app unlock, migration, and backups |
| [Known limitations and validation](limitations.md) | Platform, protocol, recovery, and validation boundaries |

## Development and maintenance

| Document | Contents |
|---|---|
| [macOS signing and unlock](macos-signing.md) | Fixed signing identity, Keychain unlock keys, cross-version checks, and Touch ID boundaries |
| [Development and builds](development.md) | Toolchain, running, packaging, existing checks, and integration entry points |
| [Architecture and lifecycle](architecture.md) | Code structure, connection and task ownership, persistence, and local extensions |
| [Themes and design tokens](theme-integration.md) | Token sources, generation, and light/dark integration |
| [Shared UI components](uikit.md) | Component responsibilities, state, scrolling, drag and drop, and terminal containers |
| [Interface languages](i18n.md) | English, Simplified Chinese, catalogs, dynamic messages, and completeness checks |
| [Integration and native E2E](../dev/ssh-integration/README.md) | Local SSH / FTP fixtures, protocol tests, and native drivers |
| [App unlock and credential vault design](credential-vault-plan.md) | Optional startup unlock, password changes, encrypted storage, key transitions, migration, and acceptance scope |
| [Contributing](../CONTRIBUTING.md) | Change scope, validation, documentation, and dependency licenses |
| [Security policy](../SECURITY.md) | Private vulnerability reports and diagnostic data boundaries |
| [Third-party notices](../THIRD_PARTY_NOTICES.md) | Original full licenses distributed with the app |

## MCP integration and tool contracts

| Document | Contents |
|---|---|
| [MCP server design](mcp-server-plan.md) | Implemented features, authentication, authorization, tool and audit contracts, validation, and remaining work |

## Maintaining documentation

- Update affected topics when behavior changes. Keep the root README focused on an overview and entry points.
- Default `.md` files are English; `.zh.md` files beside them are Chinese counterparts. Update both in the same change, including validation status. Each language links to its matching documents; source code, screenshots, commands, and license references keep their original targets.
- Keep data and credential rules in Data and credentials, and build/check commands in Development and builds.
- Distinguish implementation, validation actually run, and outstanding acceptance. A test command's existence is not evidence that it passed.
- Retain current behavior, necessary design constraints, and reproducible validation entry points.
- [README screenshots](screenshots/readme/README.md) use isolated mock data and demonstrate UI only. They are not evidence of connection, transfer, or performance acceptance.
