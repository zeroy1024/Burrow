# Themes and design tokens

[English](theme-integration.md) | [简体中文](theme-integration.zh.md)

## Sources and generated output

[internal/theme/tokens_src.py](../internal/theme/tokens_src.py) is the single source for app design tokens: primitive palettes, semantic values, dark overrides, and component mappings.

| File | Purpose |
|---|---|
| `tokens_src.py` | Token source |
| `gen.py` | Generates `theme.go` and `tokens.json` |
| `mdgen.py` | Generates English [tokens.md](../internal/theme/tokens.md) and Chinese [tokens.zh.md](../internal/theme/tokens.zh.md) |
| `theme.go` | Go types, light/dark themes, and component mappings consumed by the app |
| `accent.go` | Accent adaptation |

The layers are `palette → semantic → component`. UI reads component tokens first, then semantic tokens when needed, rather than reading the primitive palette directly.

## Edit and generate

Edit `tokens_src.py`, then run from the repository root:

```sh
(
  cd internal/theme
  python3 -B gen.py
  python3 -B mdgen.py
  gofmt -w theme.go
)
```

English token descriptions live in `tokens_docs.en.json`. Update them with new/changed Chinese descriptions; missing translations fail documentation generation.

Inspect changes in both source and generated files, then run [appropriate checks](development.md#existing-checks). Do not edit only `theme.go`, `tokens.json`, or `tokens.md`; generation will overwrite those changes.

## Appearance and component adaptation

Each root view build calls [uikit.Install](../internal/uikit/theme.go) to map current semantic colors into MyGo's theme. App preferences control light/dark/system appearance and accent. Components read `theme.Current()` while building.

UIKit centralizes color conversion, layered shadows, motion, and native theme adaptation. Shadow layers are appended in reverse so the first layer remains on top. App and system reduced-motion preferences both participate; layout movement and presentation opacity are handled separately.

For new colors, dimensions, or motion, define semantics in the source and map components to them. Rendering, hit areas, row calculations, and drag previews of one interaction must use consistent geometry sources.

## App themes and terminal colors

App themes control navigation, controls, panes, and layout. [internal/termtheme](../internal/termtheme/) controls terminal foreground/background, ANSI colors, cursor, and selection. Changing app appearance does not reset fonts or a fixed terminal scheme.

`TerminalBackground` selects the actual terminal background. Text backing, backdrop under bars, and fade tint must match it. `TerminalChrome` chooses surrounding UI appearance and neutral text from the actual background. The app owns cache and accent invalidation.

`C.Pane.ContentSide` supplies terminal spacing. Top/bottom blur covers only its own bar height. History backdrop uses actual grid boundaries; bar edges must not be treated as text clip boundaries.

See [shared UI components](uikit.md) for component responsibilities.
