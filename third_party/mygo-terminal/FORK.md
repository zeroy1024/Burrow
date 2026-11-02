# SSH Desk terminal extension

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

The functional source changes are limited to `terminal.go` and
`internal/vt/terminal.go`:

- Bind the existing Ghostty C option
  `GHOSTTY_TERMINAL_OPT_SCROLLBACK_MAX_LINES = 28`.
- Add `Options.ScrollbackLines int` and the thread-safe
  `(*Terminal).SetScrollbackLines(n int) error`.
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
