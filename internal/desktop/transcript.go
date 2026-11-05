package desktop

import (
	"fmt"
	"strings"
	"time"

	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

// reconnectHint ends the output of a pane that has no shell, as a shell ends
// its output with a prompt: Enter connects. A live terminal gets the same
// hint fed into it (showReconnectHint).
func reconnectHint(c *ui.Context, session *remoteSession) []terminalLine {
	if !session.awaitingConnect() {
		return nil
	}
	if len(session.transcript) == 0 && len(session.output) == 0 {
		h := session.endpoint
		return []terminalLine{
			{{Text: localize(c, "会话已恢复，尚未连接。")}},
			{{Text: fmt.Sprintf(localize(c, "按 Enter 连接 %s"), fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port)), Dim: true}},
		}
	}
	return []terminalLine{{{Text: localize(c, "按 Enter 重新连接"), Dim: true}}}
}

func demoTranscript(h host) []terminalLine {
	green := terminalIndexed(2)
	blue := terminalIndexed(4)
	muted := terminalIndexed(8)
	lines := []terminalLine{}
	command := func(text string) {
		lines = append(lines, terminalLine{
			{Text: h.user + "@" + h.name, Foreground: green, Bold: true},
			{Text: ":/srv/app", Foreground: blue, Bold: true},
			{Text: "# " + text},
		})
	}
	output := func(text string) {
		for _, line := range strings.Split(text, "\n") {
			spans := terminalLine{{Text: line}}
			if before, after, ok := strings.Cut(line, "Up "); ok {
				spans = terminalLine{{Text: before}, {Text: "Up " + after, Foreground: green}}
			}
			lines = append(lines, spans)
		}
	}
	switch h.name {
	case "prod-db-master":
		output(`psql (16.2)
Type "help" for help.
`)
		command("psql -d shop")
		output(`shop=# SELECT status, count(*) FROM orders
shop-# GROUP BY status ORDER BY 2 DESC;`)
		output(`  status  | count
----------+--------
 shipped  | 18204
 paid     | 15890
 pending  |   312
 refunded |   127
(4 rows)
`)
		output(`shop=# SELECT id, total, created_at FROM orders
shop-# ORDER BY created_at DESC LIMIT 20;`)
		output(`  id   |  total  |     created_at
-------+---------+---------------------
 5521  |  443.45 | 2026-10-31 16:09:58
 5520  |  216.72 | 2026-10-31 16:09:55
 5519  |  536.50 | 2026-10-31 16:09:52
 5518  |  872.19 | 2026-10-31 16:09:49
`)
	case "prod-web-02":
		command("systemctl status nginx | head -3")
		output(`● nginx.service - A high performance web server
   Loaded: loaded (/usr/lib/systemd/system/nginx.service)`)
		lines = append(lines, terminalLine{{Text: "   Active: active (running) since Sat 2026-10-31", Foreground: green}})
		output("")
		command("tail -f /var/log/nginx/access.log")
		for i, path := range []string{
			"/api/orders", "/api/checkout", "/static/app.js", "/api/products?p=2", "/healthz",
			"/api/cart/812", "/api/orders/5521", "/favicon.ico", "/api/login", "/api/orders",
			"/api/cart", "/api/products?p=3", "/healthz", "/api/orders/5522", "/static/main.css",
		} {
			lines = append(lines, terminalLine{
				{Text: fmt.Sprintf("10.0.0.%-2d  ", i%9+1), Foreground: muted},
				{Text: fmt.Sprintf("GET %-24s ", path)},
				{Text: "200", Foreground: green},
				{Text: fmt.Sprintf("  %dms", 12+i*3)},
			})
		}
	case "staging-api":
		command("htop")
		output(`  CPU[|||||||             31.0%]  Tasks: 184, 352 thr; 1 running
  Mem[|||||               27.3%]  Load average: 0.42 0.38 0.35
  Swp[                     0.0%]  Uptime: 12 days, 04:11
`)
		lines = append(lines, terminalLine{{Text: "  PID USER      CPU% MEM%    TIME+ COMMAND", Foreground: green}})
		output(` 1823 ubuntu    18.2  6.1  2:14.33 node dist/server.js
 1907 ubuntu     6.4  3.8  0:48.02 node dist/worker.js
  812 root       2.1  1.2  5:02.11 /usr/bin/dockerd
 1102 redis      1.3  0.9  3:20.45 redis-server
  944 root       0.8  0.6  1:12.90 nginx: worker process
  945 root       0.7  0.4  1:11.03 nginx: worker process
    1 root       0.0  0.1  0:09.88 /sbin/init
  531 root       0.1  0.1  0:04.15 /lib/systemd/systemd
 2210 ubuntu     0.0  0.1  0:00.42 -bash
 2291 ubuntu     0.6  0.1  0:01.03 htop
`)
	default:
		command("cd /srv/app && ls -la")
		output(`total 64
drwxr-xr-x  8 deploy deploy 4096 Oct 31 14:20 .
drwxr-xr-x  4 root   root   4096 Sep 12 09:03 ..
-rw-r--r--  1 deploy deploy  412 Oct 30 20:11 .env
drwxr-xr-x  5 deploy deploy 4096 Oct 31 14:20 .git
-rw-r--r--  1 deploy deploy 1286 Oct 28 18:47 docker-compose.yml
-rw-r--r--  1 deploy deploy  734 Oct 28 18:45 Dockerfile
drwxr-xr-x  3 deploy deploy 4096 Oct 29 11:02 logs
drwxr-xr-x  2 deploy deploy 4096 Oct 27 10:15 nginx
-rw-r--r--  1 deploy deploy 2048 Oct 31 14:20 package.json
drwxr-xr-x  6 deploy deploy 4096 Oct 31 14:20 src
-rwxr-xr-x  1 deploy deploy  918 Oct 25 16:30 deploy.sh
`)
		command("docker ps --format 'table {{.Names}}\\t{{.Image}}\\t{{.Status}}'")
		output(`NAMES          IMAGE                          STATUS
app-web-1      registry.local/web:2.8.1        Up 3 days (healthy)
app-worker-1   registry.local/worker:2.8.1     Up 3 days
app-nginx-1    nginx:1.27-alpine               Up 3 days
app-redis-1    redis:7.2                      Up 12 days
app-postgres-1 postgres:16                    Up 2 hours (health: starting)
`)
		command("uptime && free -h")
		output(` 16:09:21 up 41 days,  6:13,  2 users,  load average: 0.42, 0.38, 0.35
              total     used     free   shared  buff/cache  available
Mem:           15Gi    6.1Gi    2.4Gi    312Mi       6.9Gi      8.8Gi
Swap:         2.0Gi       0B    2.0Gi
`)
		command("tail -n 4 logs/app.log")
		for i, line := range []string{
			"GET /api/orders 200 34ms", "POST /api/checkout 201 118ms",
			"slow query on orders_idx (412ms)", "GET /healthz 200 2ms",
		} {
			level, color := "INFO", green
			if i == 2 {
				level, color = "WARN", terminalIndexed(3)
			}
			lines = append(lines, terminalLine{
				{Text: fmt.Sprintf("2026-10-31T16:09:%02dZ ", 11+i), Foreground: muted},
				{Text: level + " ", Foreground: color},
				{Text: line},
			})
		}
		output("")
		command("git log --oneline -2")
		lines = append(lines,
			terminalLine{{Text: "a1f9c3e ", Foreground: terminalIndexed(5)}, {Text: "(HEAD -> main) fix: checkout retry on timeout"}},
			terminalLine{{Text: "7be2d10 ", Foreground: terminalIndexed(5)}, {Text: "feat: order export to CSV"}},
		)
		output("")
		command("df -h / /data")
		output(`Filesystem      Size  Used Avail Use% Mounted on
/dev/vda1       120G   68G   52G  57% /
/dev/vdb1       500G  211G  289G  43% /data
`)
	}
	return lines
}

func (a *app) terminalLines(c *ui.Context, session *remoteSession, focused bool) {
	h := session.endpoint
	scheme, prefs := a.sessionScheme(session), a.terminal.prefs
	column := ui.Column(c).FillWidth().MinWidth(0).Selectable().SelectionColor(uikit.Color(scheme.SelectionBg))
	if session.contentWidth > 0 {
		column.Width(max(1, session.contentWidth-2*theme.Current().C.Pane.ContentSide)).Shrink(0)
	}
	// Native text selection owns keyboard focus. Sync the pane without moving
	// that focus to the scroll view, which would hide the selection.
	if column.FocusWithin() && a.currentTab().focused != session {
		a.currentTab().focused = session
		session.hasNewOutput, session.hasBell = false, false
		session.cursorStarted = time.Time{}
		c.AnimationFrame()
	}
	selected := make(map[uint64]ui.SelectedText)
	if column.FocusWithin() {
		for _, part := range column.SelectedTexts() {
			selected[part.ID] = part
		}
	}
	lineView := func(line terminalLine) {
		paragraph := ui.RichText(c).FillWidth().MinWidth(0).FixedLineHeight(prefs.LineHeight).Shrink(0)
		selection := selected[paragraph.ID()]
		spans := terminalSpans(line, scheme, prefs, selection.Start, selection.End)
		paragraph.Children(func() { ui.RichText(c, spans...) })
	}
	column.Children(func() {
		for _, line := range session.transcript {
			lineView(line)
		}
		for _, line := range session.output {
			lineView(line)
		}
		if hint := reconnectHint(c, session); len(hint) > 0 {
			for _, line := range hint[:len(hint)-1] {
				lineView(line)
			}
			// The last hint line takes the cursor, as a shell prompt does.
			ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).Children(func() {
				paragraph := ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight)
				selection := selected[paragraph.ID()]
				paragraph.Children(func() {
					ui.RichText(c, terminalSpans(hint[len(hint)-1], scheme, prefs, selection.Start, selection.End)...)
				})
				a.terminalCursor(c, session, focused, " ")
			})
			return
		}
		if session.state != connected {
			return
		}
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).Children(func() {
			prompt := terminalLine{
				{Text: h.user + "@" + h.name, Foreground: terminalIndexed(2), Bold: true},
				{Text: ":/srv/app", Foreground: terminalIndexed(4), Bold: true},
				{Text: "# "},
			}
			paragraph := ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight)
			selection := selected[paragraph.ID()]
			paragraph.Children(func() {
				ui.RichText(c, terminalSpans(prompt, scheme, prefs, selection.Start, selection.End)...)
			})
			a.terminalCursor(c, session, focused, " ")
		})
	})
}

type terminalDragViewport struct {
	Width, Height, ScrollY float32
}

func terminalDragViewportFor(session *remoteSession) terminalDragViewport {
	return terminalDragViewport{
		Width: session.contentWidth, Height: session.contentHeight,
		ScrollY: min(session.scroll.offset.Y, session.scroll.offset.MaxY),
	}
}

func (a *app) terminalDragContents(
	c *ui.Context,
	session *remoteSession,
	viewport terminalDragViewport,
	width, height float32,
) {
	if a.nativeTerminalDragContents(c, session, viewport, width, height) {
		return
	}
	t := theme.Current()
	prefs, scheme := a.terminal.prefs, a.sessionScheme(session)
	colors := uikit.ResolveTerminalColors(scheme, prefs.Background)
	sourceWidth := viewport.Width
	sourceHeight := viewport.Height - t.C.Pane.HeaderHeight - t.C.StatusBar.HeightPane
	scale := min(width/sourceWidth, height/sourceHeight)
	ui.Box(c).Size(width, height).Clip().PassThrough().Children(func() {
		ui.Box(c).Absolute().Left(0).Top(0).Size(sourceWidth, sourceHeight).
			Scale(scale, scale).TransformOrigin(0, 0).Clip().PassThrough().
			Font(strings.Join(prefs.FontFamily, ", ")).FontSize(prefs.FontSize).
			TextColor(uikit.Color(colors.Foreground)).Children(func() {
			ui.Column(c).Absolute().Left(t.C.Pane.ContentSide).Top(t.C.Pane.ContentSide - viewport.ScrollY).
				Width(sourceWidth - 2*t.C.Pane.ContentSide).MinWidth(0).Shrink(0).PassThrough().Children(func() {
				lineView := func(line terminalLine) {
					paragraph := ui.RichText(c).FillWidth().MinWidth(0).
						FixedLineHeight(prefs.LineHeight).Shrink(0).PassThrough()
					paragraph.Children(func() {
						ui.RichText(c, terminalSpans(line, scheme, prefs, 0, 0)...).PassThrough()
					})
				}
				for _, line := range session.transcript {
					lineView(line)
				}
				for _, line := range session.output {
					lineView(line)
				}
				for _, line := range reconnectHint(c, session) {
					lineView(line)
				}
				if session.state != connected {
					return
				}
				ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).PassThrough().Children(func() {
					h := session.endpoint
					prompt := terminalLine{
						{Text: h.user + "@" + h.name, Foreground: terminalIndexed(2), Bold: true},
						{Text: ":/srv/app", Foreground: terminalIndexed(4), Bold: true},
						{Text: "# "},
					}
					ui.RichText(c).MinWidth(0).Shrink(1).FixedLineHeight(prefs.LineHeight).PassThrough().
						Children(func() {
							ui.RichText(c, terminalSpans(prompt, scheme, prefs, 0, 0)...).PassThrough()
						})
					cursorHeight := min(prefs.LineHeight, prefs.FontSize+termtheme.CursorHeightPadding)
					ui.Box(c).Size(a.cellWidth, cursorHeight).Shrink(0).Radius(termtheme.CursorRadius).
						Margin(0, 0, termtheme.CursorBottomInset, 0).PassThrough().Children(func() {
						mark := ui.Box(c).Absolute().Left(0).Bottom(0).
							Background(uikit.Color(scheme.Cursor)).PassThrough()
						switch prefs.CursorShape {
						case termtheme.CursorBar:
							mark.Size(termtheme.CursorBarWidth, cursorHeight)
						case termtheme.CursorUnderline:
							mark.Size(a.cellWidth, termtheme.CursorUnderlineSize)
						default:
							mark.Size(a.cellWidth, cursorHeight)
						}
					})
				})
			})
		})
	})
}
