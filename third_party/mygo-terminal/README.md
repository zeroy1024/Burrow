# terminal

A terminal for [MyGo](https://github.com/egoist/mygo) apps of native UI: a
view that runs the user's shell, or any program, in a pseudo-terminal, with
[Ghostty](https://ghostty.org)'s terminal emulator, libghostty-vt, loaded
at run time with no cgo.

```go
import "github.com/egoist/mygo/sshdesk-terminal"

term, err := terminal.New(terminal.Options{})
// ...
mygo.NewWindow(mygo.WindowOptions{Title: "Terminal", Content: ui.View(func(c *ui.Context) {
	terminal.View(c, term).Fill().AutoFocus()
})})
```

This is Burrow's local terminal fork of MyGo v0.7.3. See Burrow's
[terminal documentation](../../docs/terminal-themes.md) and the
[upstream project](https://github.com/egoist/mygo) for background.

`go generate` here builds libghostty-vt for every platform with Zig, from
the commit of Ghostty in `internal/libbuild`, into `build/`, and writes
`mygo-plugin.json`, which names the files and their SHA-256 for the CLI and
this package: publish them as the assets of the release that file points
at. It writes `ghostty_themes.go` too, the themes that Ghostty ships, which
`go run ./internal/libbuild -themes` writes alone, without Zig.
