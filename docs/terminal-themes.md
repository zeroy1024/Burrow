# Terminal and color schemes

[English](terminal-themes.md) | [简体中文](terminal-themes.zh.md)

## Fonts and cursor

In **Settings → Terminal**, adjust font, size, line height, cursor shape, and blinking. New configurations use JetBrains Mono 13px, a 19px line height, and a blinking block cursor, with Menlo and the system monospace font as fallbacks.

Font, cursor, and background preferences are independent of the color scheme. Changing a scheme does not reset them. Remote programs may change cursor style with terminal control sequences.

## Colors and background

Built-in schemes are Catppuccin Latte, Catppuccin Mocha, Dracula, and Nord. Fix one scheme or follow the app appearance with separate light/dark selections. Session advanced settings can also choose a scheme.

Terminal background has two sources:

| Option | Behavior |
|---|---|
| Scheme background | Use the scheme's background; this is the default |
| Match card | Use the app card background while retaining terminal text colors; check readability yourself |

Import Ghostty-format color scheme files. Scheme names, original filenames, and source bytes are saved in SQLite's `themes` table and reloaded on startup. See [data and credentials](storage.md). Built-in scheme licenses are available in About.

The native terminal theme interface receives ANSI colors 0–15. Indices 16–255 retain the terminal's default indexed palette. The parser can retain imported overrides for those indices, but they are not passed to the native terminal. True-color output keeps its own RGB values.

## Scrollback

The default limit is 4000 physical display lines, counting wrapped lines separately. Changing it updates open SSH terminals, including background tabs, without reconnecting.

Ghostty evicts history by page, so actual retention has a page-level error margin. Lowering the limit may discard old history immediately. Raising it allows more future output to be retained but cannot restore evicted content.

There is no separate letter-spacing setting or bold-bright toggle. The native terminal and shared app scrollbars both use translucent capsules, scrolling/edge visibility, and fading, but their parameters differ.

## Splits and frosted bars

Split headers, status bars, and text areas share the terminal background source. `C.Pane.ContentSide`, currently 8px, controls horizontal grid padding and spacing around the top/bottom bars.

With transparency enabled and adjacent history available, background text is drawn continuously through the bar gaps and the partial-row remainder at the bottom. Scrolling does not force these regions blank. Blur covers only the header/status bar heights. The original grid still determines the text viewport and remote PTY rows/columns.

Drag previews prefer terminal content captured on pointer press. Hidden sources or capture failures use an independent terminal-state snapshot; this fallback cannot guarantee pixel alignment in the middle of history.

Entry points: [ssh_terminal.go](../internal/desktop/ssh_terminal.go), [terminal.go](../internal/desktop/terminal.go), and the [terminal fork notes](../third_party/mygo-terminal/FORK.md). See [known limitations](limitations.md) for acceptance boundaries.
