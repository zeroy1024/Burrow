package desktop

import (
	"cmp"
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

type fileEntry struct {
	name, path         string
	size               int64
	dir                bool
	symlink            bool
	modified           time.Time
	permissions, owner string
}

type filePaneState struct {
	loading      bool
	errorText    string
	path         string
	remote       bool
	directories  map[string][]fileEntry
	table        uikit.TableState
	scroll       ui.ScrollState
	focus        ui.Handle
	history      []string
	historyIndex int
	query        string
	searchOpen   bool
	search       ui.Handle
	reveal       string
	hoverFolder  string
	hoverStarted time.Time
}

type fileWorkspaceState struct {
	localOpen, activeRemote bool
	ratio                   float32
	localPaths              map[string]string
	resizing                bool
	notice                  *uikit.ToastProps
	dialog                  *fileActionDialog
	conflict                *fileConflict
	drag                    *fileDrag
	dragSeen                bool
}

type fileActionDialog struct {
	busy      bool
	session   *fileSession
	pane      *filePaneState
	preview   fileEntry
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
	ctx          context.Context
	reply        chan transferConflictChoice
	task         *transferTask
}

func newFilePane(remote bool) *filePaneState {
	p := &filePaneState{
		remote: remote, directories: map[string][]fileEntry{}, historyIndex: -1,
		table: uikit.TableState{Cursor: -1, Sort: ui.SortOrder{Column: "name"}},
	}
	if !remote {
		p.path = "~"
	}
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
		orders := p.table.SortBy
		if len(orders) == 0 && p.table.Sort.Column != "" {
			orders = []ui.SortOrder{p.table.Sort}
		}
		for _, order := range orders {
			comparison := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
			switch order.Column {
			case "size":
				comparison = cmp.Compare(a.size, b.size)
			case "modified":
				comparison = a.modified.Compare(b.modified)
			}
			if order.Descending {
				comparison = -comparison
			}
			if comparison != 0 {
				return comparison
			}
		}
		return 0
	})
	return entries
}

func (p *filePaneState) selection() []fileEntry {
	entries := []fileEntry{}
	for _, entry := range p.entries() {
		if p.table.Selection.Has(entry.path) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (a *app) navigateFiles(c *ui.Context, session *fileSession, pane *filePaneState, target string) {
	a.queueFileDirectory(session, pane, fileNavigation{path: target, historyIndex: -1})
}

func (a *app) fileHistory(c *ui.Context, session *fileSession, pane *filePaneState, delta int) {
	index := pane.historyIndex + delta
	if index < 0 || index >= len(pane.history) {
		return
	}
	a.queueFileDirectory(session, pane, fileNavigation{path: pane.history[index], historyIndex: index})
}

func fileWritable(pane *filePaneState, target string) bool {
	// ACLs and server permissions are authoritative at I/O time.
	return !pane.loading && target != ""
}

func (a *app) requestFileTransfer(c *ui.Context, drag fileDrag, target *filePaneState, directory string) {
	if drag.remote == target.remote || len(drag.entries) == 0 {
		return
	}
	a.enqueueFiles(c, drag.session, drag.remote, slices.Clone(drag.entries), directory)
}

func (a *app) resolveFileConflicts(c *ui.Context) {
	conflict := a.files.conflict
	if conflict == nil {
		return
	}
	if conflict.applyAll {
		conflict.task.job.batch.policy.Store(int32(conflict.choice))
	}
	select {
	case conflict.reply <- transferConflictChoice{choice: conflict.choice, applyAll: conflict.applyAll}:
	default:
	}
	a.transfers.conflicts = slices.DeleteFunc(a.transfers.conflicts, func(candidate *fileConflict) bool { return candidate == conflict })
	a.files.conflict = nil
	a.advanceTransferConflicts()
}

func (a *app) completeFileTransfer(task *transferTask) {
	view := task.view
	if view == nil || !slices.Contains(a.fileSessions, view) || !task.sourceRemote && !sameTransferView(view, task.session) {
		return
	}
	pane := view.remote
	if task.sourceRemote {
		pane = view.local
	}
	if pane.remote && view.state != connected {
		return
	}
	a.refreshFilePane(view, pane)
}

func (a *app) revealTransferredFile(c *ui.Context, task *transferTask) {
	if task.sourceRemote {
		location, err := localFileLocation(transferJoin(false, task.targetPath, task.entry.name))
		if err != nil {
			uikit.Toast(c, uikit.ToastProps{ID: "transfer-reveal-error", Tone: uikit.ToneDanger, Title: "无法定位文件", Detail: err.Error()})
			return
		}
		a.sshWorkers.Go(func() {
			_, err := os.Lstat(location)
			if a.ctx.Err() != nil {
				return
			}
			a.window.Update(func() {
				if err != nil {
					a.files.notice = &uikit.ToastProps{ID: "transfer-reveal-error", Tone: uikit.ToneDanger, Title: "无法定位文件", Detail: err.Error()}
					return
				}
				mygo.Shell.ShowItemInFolder(location)
			})
		})
		return
	}
	view := task.view
	if !sameTransferView(view, task.session) || !slices.Contains(a.fileSessions, view) {
		view = nil
	}
	index := slices.Index(a.fileSessions, view)
	if view == nil {
		endpoint := task.session.endpoint
		profile := hostID(slices.IndexFunc(a.hosts, func(candidate host) bool {
			return candidate.session.ID == endpoint.session.ID && candidate.protocol == endpoint.protocol
		}))
		view = a.newFileSession(profile, endpoint, task.session.temporary || profile < 0, task.session.connection)
		view.ftpSecret = task.session.ftpSecret
		a.fileSessions = append(a.fileSessions, view)
		index = len(a.fileSessions) - 1
	}
	// A separate destination browser does not acquire this task's ownership.
	a.switchSection(sectionFiles)
	a.activeFile = index
	if view.state != connected && view.state != connecting {
		a.startFileSession(view, false)
	}
	pane := view.remote
	pane.reveal = transferJoin(pane.remote, task.targetPath, task.entry.name)
	pane.query = ""
	a.files.activeRemote = pane.remote
	a.queueFileDirectory(view, pane, fileNavigation{path: task.targetPath, historyIndex: -1})
	pane.focus.Focus()
}

func (a *app) fileSiteBusy(id hostID) bool {
	return slices.ContainsFunc(a.transfers.tasks, func(task *transferTask) bool { return task.session.host == id })
}
