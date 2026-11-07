# README screenshots

[English](README.md) | [简体中文](README.zh.md)

Current images use the Chinese UI and do not establish English layout or language-switching acceptance.

These images come from an isolated native Burrow demo build using `Window.CapturePage`, at 2848 × 1800 pixels (1424 × 900 logical pixels, 2×). System menu bars, window buttons, and screen-sharing indicators are outside window content and are not exported.

| Image | Scene |
|---|---|
| [ssh-dark.png](ssh-dark.png) | Dark SSH splits and server status |
| [ssh-light.png](ssh-light.png) | Light SSH splits and server status |
| [files.png](files.png) | Local/remote panes and three transfer states |
| [mcp.png](mcp.png) | MCP audit and command details |

Hosts, usernames, directories, files, metrics, progress, and audits all come from [mock scenes](../../../dev/readme-gallery/scene.go.txt). Addresses use documentation range `192.0.2.0/24`. Connected, live, running-service, and transfer states are static samples; no corresponding servers are contacted or MCP listener started. Images are not evidence of protocol, performance, or native feature acceptance.

The temporary demo uses isolated database/SDK settings without reading release/development sites, credentials, or system Keychain. Normal startup and data sources are unchanged.

## Regenerate

On macOS, prepare an existing `build/darwin-arm64/Burrow.app` native resource template, then run:

```sh
mise exec -- python3 -B dev/readme-gallery/prepare.py
```

Without a template, first build an ad-hoc-signed app solely for its icon/native resources; it need not be opened:

```sh
mise exec -- go tool mygo build -skip-dmg -sign -
```

The script prints the temporary `Burrow Gallery.app` path. Open it, choose a scene from the gallery menu, wait for layout, then use Save Current Screenshot (`⌘⇧S`). Images are written to the current directory:

- `⌘1`: SSH, dark
- `⌘2`: SSH, light
- `⌘3`: File Transfer
- `⌘4`: MCP audit; open the first record before saving details

A Go overlay injects the demo entry point without adding mock data to normal builds or modifying module locks/global caches. Rebuild after changing mock data.
