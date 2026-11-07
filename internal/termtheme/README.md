# Burrow terminal color package (termtheme)

[English](README.md) | [简体中文](README.zh.md)

Terminal colors and app UI tokens are separate:

- **App tokens** (`package theme`) control pane containers: radii, inner borders, focus rings, headers, and `C.Pane.Bg`, which fills the terminal only when background is set to Match card.
- **Terminal colors** (`package termtheme`) control background, foreground, ANSI/256 colors, cursor, and selection inside the terminal.
- The pane layer still applies focus scheme D to dim unfocused panes without modifying color schemes.

## Files

| File | Purpose |
|---|---|
| `scheme.go` | `Scheme`, `Parse` / `Load` / `Format` / `Write`, `Color(0–255)`, `Xterm256`, `ParseColor` / `Hex` |
| `state.go` | Embedded schemes; `Register` / `Import` / `Lookup` / `Names`; follow/fixed `Mode`; `Resolve` / `Apply`; `Current` / `Set` / `OnChange`; `BackgroundSource`, `TerminalBackground`, `Prefs` |
| `termtheme_test.go` | Unit tests |
| `schemes/` | Built-in Ghostty-format files without extensions |
| `LICENSES.md` | Original full MIT licenses for schemes |
| `licenses.go` | Embedded licenses for Settings |

This package depends only on the Go standard library and is imported as `sshdesk/internal/termtheme`. `schemes/` must be beside `state.go` for `//go:embed`. See [terminal usage](../../docs/terminal-themes.md) and [development checks](../../docs/development.md).

## Built-in schemes

| Name | Use | Source | License |
|---|---|---|---|
| Catppuccin Latte | Default light | catppuccin/ghostty `themes/catppuccin-latte.conf` (commit b0b03cc) | MIT |
| Catppuccin Mocha | Default dark | catppuccin/ghostty `themes/catppuccin-mocha.conf` (commit b0b03cc) | MIT |
| Dracula | Optional | iTerm2-Color-Schemes `ghostty/Dracula` (commit 3d8a6a1) | MIT |
| Nord | Optional | iTerm2-Color-Schemes `ghostty/Nord` (commit 3d8a6a1) | MIT |

Changes from upstream:

- Latte `background` and `cursor-text` change from `#eff1f5` to `#fbfbfc` to match the terminal card; `# upstream:` retains original values. Its text colors were designed for `#eff1f5`, so the lighter background increases contrast.
- Files add `# name:`, `# source:`, and `# license:` comments. Catppuccin `split-divider-color` is omitted because app tokens own split dividers. Colors use `#rrggbb` consistently.
- Other colors retain upstream values.

## Terminal background option

`Prefs.Background` stores the user's backing-color source:

| Option | Value | Effect |
|---|---|---|
| Scheme background (default) | `BackgroundScheme`, stored as `"scheme"` | Use `scheme.Background` under every mode/app appearance |
| Match card | `BackgroundCard`, stored as `"card"` | Use app card `C.Pane.Bg` under every scheme |

- The app resolves color with `termtheme.TerminalBackground(s, theme.Current().Color.Surface.Terminal, prefs.Background)`.
- Top/bottom fades must match this color to avoid visible blocks.
- Defaults use Latte `#fbfbfc` in light follow mode, Mocha `#1e1e2e` in dark follow mode, and `#282a36` for fixed Dracula regardless of app appearance.
- Match card keeps scheme text/cursor/selection colors only. A dark scheme on a light card may be unreadable; the app does not automatically alter that user choice.
- Container radii, borders, focus rings, and headers always use app tokens.

## Import more schemes

Ghostty format uses `key = value`, `#` comments, and ignores unknown keys. Recognized keys are `background`, `foreground`, `cursor-color`, `cursor-text`, `selection-background`, `selection-foreground`, `palette = N=#rrggbb` (0–15 plus 16–255 overrides), `bold-color`, and the nonstandard `link-color`. Colors accept `#rrggbb`, `rrggbb`, or `#rgb`.

Missing background/foreground use Ghostty defaults. Cursor defaults to foreground, cursor text to background, selection to reversed foreground/background, and missing ANSI colors to xterm defaults.

1. Download a scheme from [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes) under `ghostty/`, such as `ghostty/GitHub Light Default`, or use a Ghostty theme file.
2. Import it in the app or call `termtheme.Import(path)`. Names come from `# name:` or the filename if absent.
3. The app saves names, original filenames, and source bytes in SQLite's `themes` table and reloads them on startup. See [data and credentials](../../docs/storage.md).
4. `termtheme.Write(path, s)` writes Ghostty format; this does not imply an app export button. Ghostty does not recognize the nonstandard `link-color`.

The parser retains indices 16–255, but the native theme interface receives only 0–15. Parser capability is not equivalent to displayed terminal behavior.

The iTerm2-Color-Schemes repository is MIT, but individual schemes may have their own licenses. Before bundling another scheme, verify its license and add the complete text to `LICENSES.md`. User imports are unaffected.

## Fonts and cursor settings

Font, size, line height, cursor shape/blink, and background source belong to `Prefs`, not a scheme, and survive scheme changes. `DefaultPrefs()` uses JetBrains Mono 13px, a 19px line height, and a blinking block cursor. Letter-spacing fields and their old rendering path have been removed.
