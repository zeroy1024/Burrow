package desktop

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type fileEntry struct {
	name, path         string
	size               int64
	dir                bool
	modified           time.Time
	permissions, owner string
}

type filePaneState struct {
	path           string
	remote         bool
	directories    map[string][]fileEntry
	selected       map[string]bool
	scroll         ui.ScrollState
	focus          ui.Handle
	history        []string
	historyIndex   int
	query          string
	searchOpen     bool
	search         ui.Handle
	sortColumn     int
	sortDescending bool
	anchor         string
	reveal         string
	hoverFolder    string
	hoverStarted   time.Time
}

type fileWorkspaceState struct {
	localOpen, activeRemote bool
	ratio                   float32
	localPaths              map[string]string
	resizing                bool
	notice                  string
	dialog                  *fileActionDialog
	conflict                *fileConflict
	drag                    *fileDrag
	dragSeen                bool
}

type fileActionDialog struct {
	session   *fileSession
	pane      *filePaneState
	kind      string
	name      string
	original  string
	errorText string
}

type fileDrag struct {
	session *fileSession
	remote  bool
	entries []fileEntry
}

type fileConflict struct {
	session      *fileSession
	sourceRemote bool
	entries      []fileEntry
	target       string
	index        int
	applyAll     bool
	choice       int
}

// These directories are an in-memory fixture. Finder drops only inspect metadata;
// no operation on this page writes, deletes or uploads the user's actual files.
func newFilePane(remote bool) *filePaneState {
	p := &filePaneState{
		remote: remote, selected: map[string]bool{}, directories: map[string][]fileEntry{},
		history: []string{},
	}
	p.path = "~"
	if remote {
		p.path = "/var/www/html"
	}
	stamp := time.Date(2026, time.October, 28, 18, 50, 0, 0, time.Local)
	add := func(dir string, entries []fileEntry) {
		for i := range entries {
			e := &entries[i]
			e.path, e.modified, e.owner = path.Join(dir, e.name), stamp, "deploy"
			e.permissions = "rw-r--r--"
			if e.dir {
				e.permissions = "rwxr-xr-x"
				p.directories[e.path] = []fileEntry{}
			}
		}
		p.directories[dir] = entries
	}
	if remote {
		add("/", []fileEntry{{name: "var", dir: true}, {name: "home", dir: true}, {name: "backups", dir: true}})
		add("/var", []fileEntry{{name: "www", dir: true}, {name: "backups", dir: true}})
		add("/var/www", []fileEntry{{name: "html", dir: true}})
		add(p.path, []fileEntry{
			{name: "static", dir: true}, {name: "uploads", dir: true},
			{name: ".well-known", dir: true}, {name: "wp-legacy", dir: true},
			{name: "404.html", size: 3200}, {name: "android-chrome-192.png", size: 9 << 10},
			{name: "android-chrome-512.png", size: 28 << 10}, {name: "app.a71b02.css", size: 1100 << 10},
			{name: "app.a71b02.js", size: 3500 << 10}, {name: "apple-touch-icon.png", size: 13 << 10},
			{name: "browserconfig.xml", size: 246}, {name: "favicon.ico", size: 15 << 10},
			{name: ".htaccess", size: 1 << 10}, {name: "humans.txt", size: 312},
			{name: "index.html", size: 8100}, {name: "logo.svg", size: 6 << 10},
			{name: "manifest.json", size: 1 << 10}, {name: "og-image.png", size: 83 << 10},
			{name: "robots.txt", size: 68}, {name: "sitemap.xml", size: 12 << 10},
		})
		add("/var/backups", []fileEntry{{name: "backup-2026-10-30.sql.gz", size: 240 << 20}})
	} else {
		add("~", []fileEntry{
			{name: "Desktop", dir: true}, {name: "Documents", dir: true},
			{name: "Downloads", dir: true}, {name: "Projects", dir: true},
		})
		add("~/Projects", []fileEntry{{name: "shop-site", dir: true}})
		add("~/Projects/shop-site", []fileEntry{{name: "dist", dir: true}})
		add("~/Projects/shop-site/dist", []fileEntry{
			{name: "assets", dir: true}, {name: "static", dir: true},
			{name: "app.3f9c1e.css", size: 1200 << 10}, {name: "app.3f9c1e.js", size: 3600 << 10},
			{name: "assets-v2.8.1.zip", size: 512 << 20}, {name: "favicon.ico", size: 15 << 10},
			{name: "index.html", size: 8400}, {name: "logo.svg", size: 6 << 10},
			{name: "manifest.json", size: 1 << 10}, {name: "README.md", size: 2 << 10},
			{name: "robots.txt", size: 68}, {name: "sitemap.xml", size: 12 << 10},
			{name: "vendor.81ab2d.js", size: 2900 << 10},
		})
	}
	p.history = append(p.history, p.path)
	return p
}

func (p *filePaneState) entries() []fileEntry {
	entries := []fileEntry{}
	for _, entry := range p.directories[p.path] {
		if strings.Contains(strings.ToLower(entry.name), strings.ToLower(p.query)) {
			entries = append(entries, entry)
		}
	}
	slices.SortStableFunc(entries, func(a, b fileEntry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		comparison := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
		switch p.sortColumn {
		case 1:
			comparison = cmp.Compare(a.size, b.size)
		case 2:
			comparison = a.modified.Compare(b.modified)
		}
		if p.sortDescending {
			return -comparison
		}
		return comparison
	})
	return entries
}

func (p *filePaneState) selection() []fileEntry {
	entries := []fileEntry{}
	for _, entry := range p.entries() {
		if p.selected[entry.name] {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (a *app) navigateFiles(c *ui.Context, session *fileSession, pane *filePaneState, target string) {
	target = path.Clean(target)
	if pane.path == target {
		return
	}
	pane.path, pane.query, pane.anchor = target, "", ""
	pane.selected, pane.scroll.Y = map[string]bool{}, 0
	pane.history = append(pane.history[:pane.historyIndex+1], target)
	pane.historyIndex = len(pane.history) - 1
	if !pane.remote {
		a.files.localPaths[a.fileSiteKey(session)] = target
		a.saveFilePreferences(c)
	}
}

func (a *app) fileHistory(c *ui.Context, session *fileSession, pane *filePaneState, delta int) {
	index := pane.historyIndex + delta
	if index < 0 || index >= len(pane.history) {
		return
	}
	pane.path, pane.historyIndex = pane.history[index], index
	pane.selected, pane.scroll.Y, pane.query = map[string]bool{}, 0, ""
	if !pane.remote {
		a.files.localPaths[a.fileSiteKey(session)] = pane.path
		a.saveFilePreferences(c)
	}
}

func (a *app) selectFile(pane *filePaneState, entry fileEntry, modifiers ui.Modifiers) {
	a.files.activeRemote = pane.remote
	pane.focus.Focus()
	if modifiers&ui.Shift != 0 && pane.anchor != "" {
		entries := pane.entries()
		start, end := -1, -1
		for i, item := range entries {
			if item.name == pane.anchor {
				start = i
			}
			if item.name == entry.name {
				end = i
			}
		}
		if start >= 0 && end >= 0 {
			if modifiers&ui.Super == 0 {
				pane.selected = map[string]bool{}
			}
			for _, item := range entries[min(start, end) : max(start, end)+1] {
				pane.selected[item.name] = true
			}
			return
		}
	}
	if modifiers&ui.Super != 0 {
		if pane.selected[entry.name] {
			delete(pane.selected, entry.name)
		} else {
			pane.selected[entry.name] = true
		}
	} else {
		pane.selected = map[string]bool{entry.name: true}
	}
	pane.anchor = entry.name
}

func fileWritable(pane *filePaneState, target string) bool {
	return !pane.remote || (target != "/" && target != "/var" && target != "/var/www")
}

func (a *app) requestFileTransfer(c *ui.Context, drag fileDrag, target *filePaneState, directory string) {
	if drag.remote == target.remote || drag.session.state != connected || len(drag.entries) == 0 {
		return
	}
	if !fileWritable(target, directory) {
		uikit.Notify(c, "此目录不可写")
		return
	}
	a.files.conflict = &fileConflict{
		session: drag.session, sourceRemote: drag.remote, entries: slices.Clone(drag.entries), target: directory,
	}
	if a.prefs.ConflictPolicy != "ask" {
		a.files.conflict.choice = map[string]int{"overwrite": 1, "skip": 2, "rename": 3}[a.prefs.ConflictPolicy]
		a.files.conflict.applyAll = true
	}
	a.resolveFileConflicts(c)
}

func (a *app) resolveFileConflicts(c *ui.Context) {
	conflict := a.files.conflict
	target := conflict.session.remote
	if conflict.sourceRemote {
		target = conflict.session.local
	}
	for conflict.index < len(conflict.entries) {
		entry := conflict.entries[conflict.index]
		collision := slices.ContainsFunc(target.directories[conflict.target], func(e fileEntry) bool {
			return e.name == entry.name
		})
		for _, task := range a.transfers.tasks {
			if task.session == conflict.session && task.sourceRemote == conflict.sourceRemote &&
				task.targetPath == conflict.target && task.entry.name == entry.name {
				collision = true
			}
		}
		if collision && conflict.choice == 0 {
			return
		}
		if collision && conflict.choice == 2 {
			conflict.index++
			if !conflict.applyAll {
				conflict.choice = 0
			}
			continue
		}
		if collision && conflict.choice == 3 {
			ext := path.Ext(entry.name)
			base := strings.TrimSuffix(entry.name, ext)
			for index := 2; ; index++ {
				name := fmt.Sprintf("%s (%d)%s", base, index, ext)
				exists := slices.ContainsFunc(target.directories[conflict.target], func(e fileEntry) bool { return e.name == name })
				for _, task := range a.transfers.tasks {
					if task.session == conflict.session && task.sourceRemote == conflict.sourceRemote &&
						task.targetPath == conflict.target && task.entry.name == name {
						exists = true
					}
				}
				if !exists {
					entry.name = name
					break
				}
			}
		}
		a.enqueueFiles(c, conflict.session, conflict.sourceRemote, []fileEntry{entry}, conflict.target)
		conflict.index++
		if !conflict.applyAll {
			conflict.choice = 0
		}
	}
	a.files.conflict = nil
}

func (a *app) completeFileTransfer(task *transferTask) {
	target, source := task.session.remote, task.session.local
	if task.sourceRemote {
		target, source = task.session.local, task.session.remote
	}
	entry := task.entry
	entry.path, entry.modified = path.Join(task.targetPath, entry.name), task.completed
	entries := target.directories[task.targetPath]
	index := slices.IndexFunc(entries, func(existing fileEntry) bool { return existing.name == entry.name })
	if index >= 0 {
		entries[index] = entry
	} else {
		entries = append(entries, entry)
	}
	target.directories[task.targetPath] = entries
	if entry.dir {
		copyDemoDirectory(source, target, task.sourcePath, entry.path)
	}
}

func copyDemoDirectory(source, target *filePaneState, from, to string) {
	entries := slices.Clone(source.directories[from])
	for i := range entries {
		oldPath := entries[i].path
		entries[i].path = path.Join(to, entries[i].name)
		if entries[i].dir {
			copyDemoDirectory(source, target, oldPath, entries[i].path)
		}
	}
	target.directories[to] = entries
}

func (a *app) revealTransferredFile(c *ui.Context, task *transferTask) {
	index := slices.Index(a.fileSessions, task.session)
	if index < 0 {
		a.fileSessions = append(a.fileSessions, task.session)
		index = len(a.fileSessions) - 1
	}
	a.switchSection(sectionFiles)
	a.activeFile = index
	pane := task.session.remote
	if task.sourceRemote {
		pane, a.files.localOpen = task.session.local, true
	}
	if pane.path != task.targetPath {
		pane.path = task.targetPath
		pane.history = append(pane.history[:pane.historyIndex+1], task.targetPath)
		pane.historyIndex = len(pane.history) - 1
	}
	pane.selected, pane.reveal = map[string]bool{task.entry.name: true}, task.entry.name
	pane.query, pane.scroll.Y = "", 0
	a.files.activeRemote = pane.remote
	if !pane.remote {
		a.files.localPaths[a.fileSiteKey(task.session)] = pane.path
		a.saveFilePreferences(c)
	}
	pane.focus.Focus()
}

func (a *app) fileSiteBusy(id hostID) bool {
	return slices.ContainsFunc(a.transfers.tasks, func(task *transferTask) bool { return task.session.host == id })
}
