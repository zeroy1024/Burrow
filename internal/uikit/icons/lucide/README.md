# Lucide icon assets

[English](README.md) | [简体中文](README.zh.md)

Shared UI icons use official [Lucide](https://lucide.dev/) SVGs, selected as needed and embedded in Go. Runtime/builds do not fetch icons online or require a JavaScript package.

- Version: `1.67.0`
- Upstream commit: `4b203ec9b6002fd74fd5dadc8612220e64289305`
- Source: <https://github.com/lucide-icons/lucide/tree/4b203ec9b6002fd74fd5dadc8612220e64289305/icons>
- License: [LICENSE](LICENSE), retaining full upstream ISC and Feather-derived MIT notices, also shown in the app's Open Source Licenses.

SVGs remain unchanged. [iconsource.go](../../iconsource.go) maps logical names to filenames in `iconFiles`. Rendering retains geometry and applies theme stroke width, color, and dimensions. Parsed results are cached by logical name/stroke width.

For a new icon, obtain its SVG from the pinned commit and add `IconName`/`iconFiles` mappings. Pages use `IconSVG`, `Icon`, or `IconButton` rather than writing general SVG paths. Version updates must synchronize assets, mappings, `LucideVersion`, this document, and licenses.

Refresh uses `refresh-cw`; retry uses `rotate-ccw`. Sidebar controls use `panel-left` and `panel-left-close`. Splitting/maximizing/restoring use `columns-2`, `maximize-2`, and `minimize-2`. Settings help uses `circle-question-mark`. Pane footer readings use `zap`, `cpu`, `memory-stick`, `arrow-down`, and `arrow-up`. AI sessions use `bot`.

Lucide excludes brand marks. Burrow, OS, and MCP marks remain separate and render through `BrandIcon` or dedicated asset entry points.
