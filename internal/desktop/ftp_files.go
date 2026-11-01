package desktop

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
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
	loading        bool
	errorText      string
}

type fileWorkspaceState struct {
	localOpen, activeRemote bool
	ratio                   float32
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

// Remote contents are populated only by successful protocol directory reads.
func newFilePane(remote bool) *filePaneState {
	pane := &filePaneState{remote: remote, selected: map[string]bool{}, directories: map[string][]fileEntry{}}
	if !remote {
		pane.path = "~"
	}
	return pane
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
	if !pane.remote {
		return
	}
	target = path.Clean(target)
	if pane.path == target && !pane.loading && pane.errorText == "" {
		return
	}
	a.queueRemoteDirectory(session, fileNavigation{path: target, historyIndex: -1})
}

func (a *app) fileHistory(c *ui.Context, session *fileSession, pane *filePaneState, delta int) {
	index := pane.historyIndex + delta
	if !pane.remote || index < 0 || index >= len(pane.history) {
		return
	}
	a.queueRemoteDirectory(session, fileNavigation{path: pane.history[index], historyIndex: index})
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
		c.Toast("此目录不可写")
		return
	}
	a.files.conflict = &fileConflict{
		session: drag.session, sourceRemote: drag.remote, entries: slices.Clone(drag.entries), target: directory,
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
		a.rememberFileLocalPath(c, task.session, pane.path)
	}
	pane.focus.Focus()
}
