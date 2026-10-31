package desktop

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

func demoTranscript(h host) [][]ui.Span {
	green := ui.Hex("#229f4a")
	blue := ui.Hex("#087df0")
	muted := ui.Hex(terminalSecondaryText)
	lines := [][]ui.Span{}
	command := func(text string) {
		lines = append(lines, []ui.Span{
			{Text: h.user + "@" + h.name, Color: green, Weight: 600},
			{Text: ":/srv/app", Color: blue, Weight: 600},
			{Text: "# " + text},
		})
	}
	output := func(text string) {
		for _, line := range strings.Split(text, "\n") {
			spans := []ui.Span{{Text: line}}
			if before, after, ok := strings.Cut(line, "Up "); ok {
				spans = []ui.Span{{Text: before}, {Text: "Up " + after, Color: green}}
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
		lines = append(lines, []ui.Span{{Text: "   Active: active (running) since Sat 2026-10-31", Color: green}})
		output("")
		command("tail -f /var/log/nginx/access.log")
		for i, path := range []string{
			"/api/orders", "/api/checkout", "/static/app.js", "/api/products?p=2", "/healthz",
			"/api/cart/812", "/api/orders/5521", "/favicon.ico", "/api/login", "/api/orders",
			"/api/cart", "/api/products?p=3", "/healthz", "/api/orders/5522", "/static/main.css",
		} {
			lines = append(lines, []ui.Span{
				{Text: fmt.Sprintf("10.0.0.%-2d  ", i%9+1), Color: muted},
				{Text: fmt.Sprintf("GET %-24s ", path)},
				{Text: "200", Color: green},
				{Text: fmt.Sprintf("  %dms", 12+i*3)},
			})
		}
	case "staging-api":
		command("htop")
		output(`  CPU[|||||||             31.0%]  Tasks: 184, 352 thr; 1 running
  Mem[|||||               27.3%]  Load average: 0.42 0.38 0.35
  Swp[                     0.0%]  Uptime: 12 days, 04:11
`)
		lines = append(lines, []ui.Span{{Text: "  PID USER      CPU% MEM%    TIME+ COMMAND", Color: green}})
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
				level, color = "WARN", ui.Hex("#c58b13")
			}
			lines = append(lines, []ui.Span{
				{Text: fmt.Sprintf("2026-10-31T16:09:%02dZ ", 11+i), Color: muted},
				{Text: level + " ", Color: color},
				{Text: line},
			})
		}
		output("")
		command("git log --oneline -2")
		lines = append(lines,
			[]ui.Span{{Text: "a1f9c3e ", Color: ui.Hex("#a060e8")}, {Text: "(HEAD -> main) fix: checkout retry on timeout"}},
			[]ui.Span{{Text: "7be2d10 ", Color: ui.Hex("#a060e8")}, {Text: "feat: order export to CSV"}},
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
	h := a.hosts[session.host]
	column := ui.Column(c).FillWidth().MinWidth(0)
	if session.contentWidth > 0 {
		column.Width(max(1, session.contentWidth-28)).Shrink(0)
	}
	column.Children(func() {
		for _, line := range demoTranscript(h) {
			ui.RichText(c, line...).FillWidth().MinWidth(0).FixedLineHeight(19).Shrink(0)
		}
		for _, line := range session.output {
			ui.Text(c, line).FillWidth().MinWidth(0).FixedLineHeight(19).Shrink(0)
		}
		ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.End).Children(func() {
			ui.RichText(c,
				ui.Span{Text: h.user + "@" + h.name, Color: ui.Hex("#229f4a"), Weight: 600},
				ui.Span{Text: ":/srv/app", Color: ui.Hex("#087df0"), Weight: 600},
				ui.Span{Text: "# "},
			).MinWidth(0).Shrink(1).FixedLineHeight(19)
			terminalCursor(c, session, focused)
		})
	})
}
