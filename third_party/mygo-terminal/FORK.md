# Burrow terminal extension

This directory is the terminal plugin from `github.com/egoist/mygo v0.7.3`,
copied from its published Go module. The original MIT license is retained in
`LICENSE`; the upstream plugin sources, resources, generator and existing tests
are retained. No new permanent test suite is added here.

Only the terminal plugin is a local module. The rest of MyGo remains the pinned
`v0.7.3` dependency. The local module name is
`github.com/egoist/mygo/sshdesk-terminal`; its private plugin imports are renamed
from `github.com/egoist/mygo/plugins/terminal/internal/...`. Keeping the MyGo
module prefix lets the unchanged plugin legally use MyGo's internal localization
and resource download packages, without reflection or external private access.

`mygo-plugin.json` and the native library checksums are unchanged. The library
remains libghostty-vt built from Ghostty commit
`befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`; it is not rebuilt or upgraded.
The MyGo CLI discovers this plugin's native library manifest through the actual
Go dependency directory, as it does for the upstream plugin.

The unchanged raw VT capture in `testdata/` is marked binary in `.gitattributes`
so Git preserves its protocol CR/escape bytes without treating them as source
whitespace or converting line endings.

## Functional patch

The scrollback and initial-grid changes are in `terminal.go` and
`internal/vt/terminal.go`:

- Bind the existing Ghostty C option
  `GHOSTTY_TERMINAL_OPT_SCROLLBACK_MAX_LINES = 28`.
- Add `Options.ScrollbackLines int` and the thread-safe
  `(*Terminal).SetScrollbackLines(n int) error`.
- Add `Options.Columns` and `Options.Rows`, initializing the VT grid before the
  reader starts. SSH supplies its actual PTY dimensions; unspecified axes keep
  the upstream 80-column/24-row defaults. This prevents early cursor-addressed
  output from being clipped before the first view frame.
- Allow the byte cap to be removed through the existing C option's null value,
  so the old implicit 10 MiB cap cannot silently override the requested rows.
- Return the underlying C option errors and reject updates after terminal close.

Positive `ScrollbackLines` limits physical history rows, so a wrapped line counts
as multiple rows. Ghostty evicts whole pages: the retained history can be slightly
above or below the configured limit. Shrinking the limit removes
eligible old pages immediately. Increasing it cannot restore discarded output.

The byte policy remains explicit and compatible:

| `Options.Scrollback` | Positive row limit | Nonpositive row limit |
|---|---|---|
| `0` | No byte cap; row limit controls history | Upstream default 10 MiB cap |
| Positive | Both caps apply; whichever is reached first | Explicit byte cap |
| Negative | History disabled, as upstream | History disabled, as upstream |

A nonpositive live setter removes the row cap and restores that original byte
policy; it does not replace the terminal. The setter updates the existing VT
under its mutex and requests repaint after releasing it. It does not recreate
the connection, write to its stream, resize the PTY, reset the viewport, clear
selection, end search or release synchronized-output holds. Ghostty's existing
tracked pins handle retained history and clamp pins into deliberately evicted
pages through the normal native behavior.

Source reference:
[fixed Ghostty public option](https://github.com/ghostty-org/ghostty/blob/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028/include/ghostty/vt/terminal.h#L1914).

## Scrollback behind overlay bars

`ViewportBackdrop(c, terminal, ViewportBackdropOptions{Top, Bottom, Gap, Radius})` is a
decorative sibling built after the original `View`, in its full pane wrapper.
It draws actual adjacent history rows continuously from the visible grid to the
overlay bars, including the layout gaps and fractional bottom slack.
The terminal View, grid, font metrics, input coordinates, cursor, selection,
scroll position and scrollbar keep their original geometry. The application
supplies its content gap through `ViewportBackdropOptions.Gap`.
There is no extra terminal, history replay, viewport movement or PTY resize.

`backdrop.go`, `view.go` and `paint.go` retain a few outside rows from the same
frame render state. `internal/vt/{lib,render,types}.go` binds the already-shipped
`ghostty_render_state_set`, `GHOSTTY_RENDER_STATE_OPTION_OVERSCAN = 1`, and
`GHOSTTY_RENDER_STATE_ROW_DATA_VIEWPORT_Y = 6`; ABI validation includes the
two-uint16 `GhosttyRenderStateOverscan` structure. The native manifest, library
checksums and Ghostty revision remain unchanged.

Extra rows carry the original cell styles, graphemes and drawn box characters;
their signed viewport coordinates align them with the live grid. Content that
does not exist (before history starts or after the bottom) is never invented.
Top overscan is enabled only while scrolled back. Following the live screen
leaves the header blank, including after clear-screen sequences that preserve
scrollback; scrolling back still exposes that retained history.
Zero bar heights remove the overscan request. Closing a terminal drops the edge
cache along with its normal frame. The application applies its shared glass
material above this drawing and respects its translucency/accessibility switch.

The grid origin comes from the backdrop subtree's own paint rectangle and the
unchanged top bar/gap inset. It never copies a painted View rectangle across
separate saturation-effect canvases. At paint time, the top band ends at the
pixel-aligned grid origin and the bottom band starts after the last complete
grid row. This prevents a bar boundary or unpainted gap from cutting a glyph.
An outer rectangular clip limits each band; the innermost clip carries the full
card's rounded outline. MyGo's stationary GPU path retains only the innermost
rounded shape, so reversing these clips would lose the card's rounded corners.

Source reference:
[fixed Ghostty overscan contract](https://github.com/ghostty-org/ghostty/blob/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028/include/ghostty/vt/render.h#L98).
