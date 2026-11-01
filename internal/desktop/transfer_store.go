package desktop

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"sshdesk/internal/fileclient"
)

type storedTransferQueue struct {
	Version     int                        `json:"version"`
	NextBatchID int                        `json:"nextBatchId"`
	Batches     []storedTransferBatch      `json:"batches"`
	NextID      int                        `json:"nextId"`
	Sessions    []storedTransferSession    `json:"sessions"`
	Connections []storedTransferConnection `json:"connections"`
	Tasks       []storedTransferTask       `json:"tasks"`
}

type storedTransferBatch struct {
	ID     int `json:"id"`
	Policy int `json:"policy"`
}

type storedTransferSession struct {
	ID         int                    `json:"id"`
	Session    storedWorkspaceSession `json:"session"`
	LocalPath  string                 `json:"localPath"`
	RemotePath string                 `json:"remotePath"`
}

type storedTransferConnection struct {
	ID       int             `json:"id"`
	Endpoint storedSession   `json:"endpoint"`
	Steps    []storedSession `json:"steps,omitempty"`
}

type storedTransferTask struct {
	ID             int                 `json:"id"`
	Batch          int                 `json:"batch"`
	Session        int                 `json:"session"`
	Entry          fileclient.Entry    `json:"entry"`
	State          transferState       `json:"state"`
	Transferred    int64               `json:"transferred"`
	Started        time.Time           `json:"started,omitempty"`
	Completed      time.Time           `json:"completed,omitempty"`
	Error          string              `json:"error,omitempty"`
	CleanupPending bool                `json:"cleanupPending,omitempty"`
	SourceRemote   bool                `json:"sourceRemote"`
	SourcePath     string              `json:"sourcePath"`
	TargetPath     string              `json:"targetPath"`
	Name           string              `json:"name"`
	Policy         int                 `json:"policy"`
	Total          int64               `json:"total"`
	DoneBytes      int64               `json:"doneBytes"`
	Root           *storedTransferNode `json:"root,omitempty"`
	Temporary      []string            `json:"temporary,omitempty"`
}

type storedTransferNode struct {
	Source       string                `json:"source"`
	Destination  string                `json:"destination,omitempty"`
	Temporary    string                `json:"temporary,omitempty"`
	Entry        fileclient.Entry      `json:"entry"`
	Committing   bool                  `json:"committing,omitempty"`
	CommitLegacy bool                  `json:"commitLegacy,omitempty"`
	Staged       *fileclient.Entry     `json:"staged,omitempty"`
	Digest       string                `json:"digest,omitempty"`
	Approved     *fileclient.Entry     `json:"approved,omitempty"`
	Children     []*storedTransferNode `json:"children,omitempty"`
	Total        int64                 `json:"total"`
	Done         bool                  `json:"done,omitempty"`
	Skipped      bool                  `json:"skipped,omitempty"`
}

// prepareTransferQuit leaves the application context alive until every transfer
// worker has relinquished its manifest. Saving then requires no authentication
// or filesystem/remote cleanup, and a failed write leaves usable paused rows.
func (a *app) prepareTransferQuit(done func(error)) {
	q := &a.transfers
	if q.quitting {
		if !q.storing && len(q.quitDone) == 0 {
			done(nil)
			return
		}
		q.quitDone = append(q.quitDone, done)
		return
	}
	q.quitDone = append(q.quitDone, done)
	q.quitting = true
	for _, task := range q.tasks {
		if task.state == transferRunning || task.state == transferQueued {
			task.state, task.speed = transferPaused, 0
		}
		if task.run != nil {
			task.run.cancel()
			cancelSSHRequest(task.run.request)
		}
	}
	q.displaySpeed = 0
	a.advanceTransferConflicts()
	a.finishTransferQuitIfReady()
}

func (a *app) finishTransferQuitIfReady() {
	q := &a.transfers
	if !q.quitting || q.storing {
		return
	}
	for _, task := range q.tasks {
		if task.run != nil {
			return
		}
	}
	finish := func(err error) {
		q.storing = false
		if err != nil {
			q.quitting = false
		}
		callbacks := q.quitDone
		q.quitDone = nil
		for _, done := range callbacks {
			done(err)
		}
	}
	if q.storeErr != nil {
		if len(q.tasks) == 0 {
			// Preserve a journal rejected at load without trapping an otherwise
			// empty application in an exit/save loop.
			finish(nil)
			return
		}
		finish(fmt.Errorf("传输记录读取失败，已阻止覆盖：%w", q.storeErr))
		return
	}
	if q.storePath == "" {
		finish(errors.New("传输记录目录不可用"))
		return
	}
	snapshot := a.transferSnapshot()
	if err := validateTransferQueue(snapshot); err != nil {
		finish(fmt.Errorf("传输记录状态无效：%w", err))
		return
	}
	q.storing = true
	name := q.storePath
	a.sshWorkers.Go(func() {
		data, err := json.MarshalIndent(snapshot, "", "  ")
		if err == nil {
			err = writeCatalog(name, append(data, '\n'))
		}
		a.window.Update(func() { finish(err) })
	})
}

func (a *app) transferSnapshot() *storedTransferQueue {
	result := &storedTransferQueue{Version: 2, NextID: a.transfers.nextID, NextBatchID: a.transfers.nextBatchID,
		Sessions: []storedTransferSession{}, Connections: []storedTransferConnection{}, Tasks: []storedTransferTask{}}
	groups := map[*sshConnectionGroup]int{}
	groupID := func(group *sshConnectionGroup) int {
		if group == nil {
			return 0
		}
		if id := groups[group]; id != 0 {
			return id
		}
		id := len(groups) + 1
		groups[group] = id
		stored := storedTransferConnection{ID: id,
			Endpoint: snapshotWorkspaceSession(group.endpoint, group.endpoint.session, true, 0).Endpoint}
		for _, step := range group.plan.steps {
			stored.Steps = append(stored.Steps, snapshotWorkspaceSession(step.endpoint, step.endpoint.session, true, 0).Endpoint)
		}
		result.Connections = append(result.Connections, stored)
		return id
	}
	sessions := map[*fileSession]int{}
	batches := map[*transferBatch]bool{}
	for _, task := range a.transfers.tasks {
		sessionID := sessions[task.session]
		if sessionID == 0 {
			sessionID = len(sessions) + 1
			sessions[task.session] = sessionID
			session := task.session
			result.Sessions = append(result.Sessions, storedTransferSession{
				ID: sessionID, Session: snapshotWorkspaceSession(session.endpoint, session.config, session.temporary, groupID(session.connection)),
				LocalPath: session.local.path, RemotePath: session.remote.path,
			})
		}
		job := task.job
		if !batches[job.batch] {
			batches[job.batch] = true
			result.Batches = append(result.Batches, storedTransferBatch{ID: job.batch.id, Policy: int(job.batch.policy.Load())})
		}
		entry := task.entry
		if job.root != nil && job.root.destination != "" {
			entry.name = transferBase(!job.sourceRemote, job.root.destination)
		}
		stored := storedTransferTask{ID: task.id, Session: sessionID, Batch: job.batch.id,
			Entry: fileclient.Entry{Name: entry.name, Size: entry.size, Directory: entry.dir, Symlink: entry.symlink,
				Modified: entry.modified, Owner: entry.owner, Permissions: entry.permissions},
			State: task.state, Transferred: task.transferred, Started: task.started, Completed: task.completed,
			Error: task.errorText, CleanupPending: task.cleanupPending || task.removeRequested,
			SourceRemote: job.sourceRemote, SourcePath: job.sourcePath, TargetPath: job.targetPath,
			Name: job.name, Policy: int(job.batch.policy.Load()), Total: job.total, DoneBytes: job.doneBytes,
			Root: snapshotTransferNode(job.root)}
		for name := range job.temporary {
			stored.Temporary = append(stored.Temporary, name)
		}
		slices.Sort(stored.Temporary)
		result.Tasks = append(result.Tasks, stored)
	}
	return result
}

func snapshotTransferNode(node *transferNode) *storedTransferNode {
	if node == nil {
		return nil
	}
	stored := &storedTransferNode{Source: node.source, Destination: node.destination, Temporary: node.temporary,
		Entry: node.entry, Approved: node.approved, Committing: node.committing, CommitLegacy: node.commitLegacy, Staged: node.staged, Digest: node.digest, Total: node.total, Done: node.done, Skipped: node.skipped}
	for _, child := range node.children {
		stored.Children = append(stored.Children, snapshotTransferNode(child))
	}
	return stored
}

func (a *app) restoreTransfers() (err error) {
	q := &a.transfers
	defer func() { q.storeErr = err }()
	if a.catalogConfig.path == "" {
		return errors.New("传输记录目录不可用")
	}
	q.storePath = filepath.Join(filepath.Dir(a.catalogConfig.path), "transfers.json")
	info, err := os.Lstat(q.storePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("传输记录必须为普通文件")
	}
	data, err := os.ReadFile(q.storePath)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var saved storedTransferQueue
	if err := decoder.Decode(&saved); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errors.New("传输记录包含多个JSON值")
		}
		return err
	}
	if err := migrateTransferQueue(&saved); err != nil {
		return err
	}
	if err := validateTransferQueue(&saved); err != nil {
		return err
	}
	groups := map[int]*sshConnectionGroup{}
	for _, stored := range saved.Connections {
		endpoint := transferStoredHost(stored.Endpoint)
		group := a.newSSHGroup(endpoint)
		if len(stored.Steps) > 0 {
			group.plan, group.planErr = sshDialPlan{knownHosts: filepath.Join(filepath.Dir(q.storePath), "known_hosts")}, nil
			for _, value := range stored.Steps {
				step := sshDialStep{endpoint: transferStoredHost(value)}
				if step.endpoint.session.Auth == "key" {
					var keyErr error
					step.key, keyErr = a.keyByID(step.endpoint.session.KeyID)
					group.planErr = errors.Join(group.planErr, keyErr)
				}
				group.plan.steps = append(group.plan.steps, step)
			}
		}
		groups[stored.ID] = group
	}
	sessions := map[int]*fileSession{}
	for _, stored := range saved.Sessions {
		endpoint := transferStoredHost(stored.Session.Endpoint)
		profile := hostID(slices.IndexFunc(a.hosts, func(h host) bool { return h.session.ID == stored.Session.ProfileID }))
		session := a.newFileSession(profile, endpoint, stored.Session.Temporary || profile < 0, groups[stored.Session.Connection])
		session.local.path, session.remote.path = stored.LocalPath, stored.RemotePath
		if stored.LocalPath != "" {
			session.local.history, session.local.historyIndex = []string{stored.LocalPath}, 0
		}
		if stored.RemotePath != "" {
			session.remote.history, session.remote.historyIndex = []string{stored.RemotePath}, 0
		}
		sessions[stored.ID] = session
	}
	batches := map[int]*transferBatch{}
	for _, stored := range saved.Batches {
		batch := &transferBatch{id: stored.ID}
		batch.policy.Store(int32(stored.Policy))
		batches[stored.ID] = batch
	}
	for _, stored := range saved.Tasks {
		job := &transferJob{sourceRemote: stored.SourceRemote, sourcePath: stored.SourcePath,
			targetPath: stored.TargetPath, name: stored.Name, policy: stored.Policy, batch: batches[stored.Batch],
			root: restoreTransferNode(stored.Root), total: stored.Total, doneBytes: stored.DoneBytes,
			temporary: map[string]struct{}{}}
		for _, name := range stored.Temporary {
			job.temporary[name] = struct{}{}
		}
		entry := stored.Entry
		state := stored.State
		if state != transferDone && state != transferFailed {
			state = transferPaused
		}
		q.tasks = append(q.tasks, &transferTask{id: stored.ID, session: sessions[stored.Session],
			entry: fileEntry{name: entry.Name, path: stored.SourcePath, size: entry.Size, dir: entry.Directory,
				symlink: entry.Symlink, modified: entry.Modified, owner: entry.Owner, permissions: entry.Permissions},
			sourceRemote: stored.SourceRemote, sourcePath: stored.SourcePath, targetPath: stored.TargetPath,
			state: state, transferred: stored.Transferred, total: stored.Total, started: stored.Started,
			completed: stored.Completed, errorText: stored.Error, cleanupPending: stored.CleanupPending, job: job})
	}
	q.nextID, q.nextBatchID = saved.NextID, saved.NextBatchID
	return nil
}

func restoreTransferNode(stored *storedTransferNode) *transferNode {
	if stored == nil {
		return nil
	}
	node := &transferNode{source: stored.Source, destination: stored.Destination, temporary: stored.Temporary,
		entry: stored.Entry, approved: stored.Approved, committing: stored.Committing, commitLegacy: stored.CommitLegacy, staged: stored.Staged, digest: stored.Digest, total: stored.Total, done: stored.Done, skipped: stored.Skipped}
	for _, child := range stored.Children {
		node.children = append(node.children, restoreTransferNode(child))
	}
	return node
}

func transferStoredHost(value storedSession) host {
	return cloneHostEndpoint(host{name: value.Name, address: value.Host, port: value.Port, user: value.User,
		protocol: value.Protocol, os: value.OS, system: value.System, session: value.sessionConfig})
}

func validateTransferEndpoint(endpoint storedSession, files bool) error {
	if strings.TrimSpace(endpoint.Name) == "" || strings.TrimSpace(endpoint.Host) == "" ||
		strings.ContainsAny(endpoint.Host+endpoint.User, "\r\n\x00") || endpoint.Port < 1 || endpoint.Port > 65535 ||
		endpoint.ID == "" || endpoint.OS < 0 || endpoint.OS >= len(systems) {
		return errors.New("无效的传输会话快照")
	}
	if files && endpoint.Protocol != "sftp" && endpoint.Protocol != "ftp" ||
		!files && endpoint.Protocol != "" && endpoint.Protocol != "sftp" {
		return errors.New("传输会话协议无效")
	}
	if !slices.Contains([]string{"password", "key", "agent", "anonymous"}, endpoint.Auth) ||
		endpoint.Protocol == "ftp" && endpoint.Auth != "password" && endpoint.Auth != "anonymous" ||
		endpoint.Protocol != "ftp" && endpoint.Auth == "anonymous" {
		return errors.New("传输认证方式无效")
	}
	if endpoint.Auth == "key" && (endpoint.KeyID == "" || endpoint.KeyID == "." || endpoint.KeyID == ".." ||
		filepath.Base(endpoint.KeyID) != endpoint.KeyID) {
		return errors.New("传输私钥引用无效")
	}
	if options := endpoint.SSH; options != nil {
		if options.KeepAlive < 0 || options.KeepAlive > 3600 {
			return errors.New("传输Keepalive无效")
		}
	}
	if options := endpoint.Files; options != nil {
		if !slices.Contains([]string{"UTF-8", "GBK", "GB18030", "Big5", "Shift_JIS", "EUC-KR", "ISO-8859-1"}, options.Encoding) {
			return errors.New("传输文件名编码无效")
		}
		if endpoint.Protocol == "ftp" && (!slices.Contains([]string{"none", "explicit", "implicit"}, options.FTPTLS) ||
			!slices.Contains([]string{"passive", "active"}, options.FTPMode)) {
			return errors.New("FTP传输选项无效")
		}
	}
	return nil
}

func sameTransferEndpoint(left, right storedSession) bool {
	return left.Host == right.Host && left.Port == right.Port && left.User == right.User &&
		left.Auth == right.Auth && left.KeyID == right.KeyID && left.ID == right.ID
}

// Version 1 used one private session per enqueue batch, but only stored policy
// per task and had no commit acknowledgement state. Preserve those boundaries;
// a pending old part must be reconciled before it can be uploaded again.
func migrateTransferQueue(saved *storedTransferQueue) error {
	if saved.Version == 2 {
		return nil
	}
	if saved.Version != 1 {
		return errors.New("传输记录版本无效")
	}
	policies := map[int]map[int]bool{}
	for _, task := range saved.Tasks {
		if policies[task.Session] == nil {
			policies[task.Session] = map[int]bool{}
		}
		if task.Policy != 0 {
			policies[task.Session][task.Policy] = true
		}
	}
	decisions := map[int]int{}
	for id, values := range policies {
		policy := 0
		if len(values) == 1 {
			for value := range values {
				policy = value
			}
		}
		decisions[id] = policy
		saved.Batches = append(saved.Batches, storedTransferBatch{ID: id, Policy: policy})
		saved.NextBatchID = max(saved.NextBatchID, id)
	}
	var markPending func(*storedTransferNode)
	markPending = func(node *storedTransferNode) {
		if node == nil {
			return
		}
		if node.Temporary != "" && !node.Done && !node.Skipped && !node.Committing {
			node.CommitLegacy = true
		}
		for _, child := range node.Children {
			markPending(child)
		}
	}
	for index := range saved.Tasks {
		task := &saved.Tasks[index]
		task.Batch, task.Policy = task.Session, decisions[task.Session]
		markPending(task.Root)
	}
	saved.Version = 2
	return nil
}

func validateTransferQueue(saved *storedTransferQueue) error {
	if saved.Version != 2 || saved.NextID < 0 || saved.NextBatchID < 0 {
		return errors.New("传输记录版本或计数无效")
	}
	batches := map[int]storedTransferBatch{}
	for _, batch := range saved.Batches {
		if batch.ID <= 0 || batch.ID > saved.NextBatchID || batches[batch.ID].ID != 0 || batch.Policy < 0 || batch.Policy > 3 {
			return errors.New("传输批次无效")
		}
		batches[batch.ID] = batch
	}
	groups := map[int]storedTransferConnection{}
	for _, group := range saved.Connections {
		if group.ID <= 0 || groups[group.ID].ID != 0 {
			return errors.New("传输连接ID重复或无效")
		}
		if err := validateTransferEndpoint(group.Endpoint, false); err != nil {
			return err
		}
		seen := map[string]bool{}
		for index, step := range group.Steps {
			if err := validateTransferEndpoint(step, false); err != nil {
				return err
			}
			if seen[step.ID] {
				return errors.New("传输跳板机快照成环")
			}
			seen[step.ID] = true
			if index > 0 && (step.SSH == nil || step.SSH.JumpHostID != group.Steps[index-1].ID) {
				return errors.New("传输跳板机快照顺序无效")
			}
		}
		if len(group.Steps) > 0 && !sameTransferEndpoint(group.Endpoint, group.Steps[len(group.Steps)-1]) {
			return errors.New("传输连接目标与快照不同")
		}
		groups[group.ID] = group
	}
	sessions := map[int]storedTransferSession{}
	for _, session := range saved.Sessions {
		if session.ID <= 0 || sessions[session.ID].ID != 0 {
			return errors.New("传输文件会话ID重复或无效")
		}
		if err := validateTransferEndpoint(session.Session.Endpoint, true); err != nil {
			return err
		}
		if strings.ContainsRune(session.LocalPath+session.RemotePath, '\x00') {
			return errors.New("传输浏览路径无效")
		}
		if session.Session.ProfileID != "" && session.Session.ProfileID != session.Session.Endpoint.ID {
			return errors.New("传输来源会话ID无效")
		}
		if session.Session.Endpoint.Protocol == "ftp" {
			if session.Session.Connection != 0 {
				return errors.New("FTP不能引用SSH连接")
			}
		} else {
			group, ok := groups[session.Session.Connection]
			if !ok || !sameTransferEndpoint(session.Session.Endpoint, group.Endpoint) {
				return errors.New("SFTP连接快照不匹配")
			}
		}
		sessions[session.ID] = session
	}
	ids := map[int]bool{}
	for _, task := range saved.Tasks {
		if task.ID <= 0 || task.ID > saved.NextID || ids[task.ID] || sessions[task.Session].ID == 0 ||
			task.State < transferQueued || task.State > transferFailed || task.Transferred < 0 || batches[task.Batch].ID == 0 || task.Policy != batches[task.Batch].Policy ||
			task.Total < 0 || task.DoneBytes < 0 || task.Entry.Size < 0 || task.Policy < 0 || task.Policy > 3 ||
			task.CleanupPending && task.State != transferFailed ||
			!transferSafeName(task.Name) || !transferSafeName(task.Entry.Name) ||
			task.SourcePath == "" || task.TargetPath == "" || strings.ContainsRune(task.SourcePath+task.TargetPath, '\x00') {
			return errors.New("传输任务记录无效")
		}
		ids[task.ID] = true
		owned := map[string]bool{}
		if task.Root == nil {
			if task.Total != 0 || task.DoneBytes != 0 || len(task.Temporary) != 0 || task.State == transferDone {
				return errors.New("无清单的传输任务包含部分文件")
			}
		} else {
			if !canonicalTransferPath(task.SourceRemote, task.SourcePath) || !canonicalTransferPath(!task.SourceRemote, task.TargetPath) || task.Root.Source != task.SourcePath {
				return errors.New("传输根目录路径无效")
			}
			if task.Name != task.Root.Entry.Name || task.Name != transferBase(task.SourceRemote, task.SourcePath) {
				return errors.New("传输源名称与清单不一致")
			}
			expectedName := task.Name
			if task.Root.Destination != "" {
				expectedName = transferBase(!task.SourceRemote, task.Root.Destination)
			}
			if task.Entry.Name != expectedName {
				return errors.New("传输显示目标与清单不一致")
			}
			total, done, err := validateTransferNode(task.Root, task.SourceRemote, task.SourcePath, task.TargetPath, owned, 0)
			if err != nil {
				return err
			}
			if total != task.Total || done != task.DoneBytes {
				return errors.New("传输清单总量不一致")
			}
			if task.State == transferDone && !task.Root.Done && !task.Root.Skipped {
				return errors.New("已完成传输仍有未完成清单")
			}
		}
		if len(owned) != len(task.Temporary) {
			return errors.New("传输临时文件集合不匹配")
		}
		for _, name := range task.Temporary {
			if !owned[name] {
				return errors.New("传输临时文件不属于清单")
			}
			delete(owned, name)
		}
	}
	return nil
}

func canonicalTransferPath(remote bool, name string) bool {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return false
	}
	if remote {
		return strings.HasPrefix(name, "/") && transferJoin(true, name, "") == name
	}
	return filepath.IsAbs(name) && filepath.Clean(name) == name
}

func validTransferTemporary(remote bool, name, destination string) bool {
	if name == destination || !canonicalTransferPath(remote, name) || transferDir(remote, name) != transferDir(remote, destination) {
		return false
	}
	base := transferBase(remote, name)
	if !strings.HasPrefix(base, ".sshdesk-") || !strings.HasSuffix(base, ".part") {
		return false
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(base, ".sshdesk-"), ".part")
	value, err := hex.DecodeString(encoded)
	return err == nil && len(value) == 16 && hex.EncodeToString(value) == encoded
}

// Every child's source is the named child of its parent. Every destination is
// one safe component below its parent, and only generated staging names in that
// same directory may be owned/deleted. A journal cannot nominate arbitrary files.
func validateTransferNode(node *storedTransferNode, sourceRemote bool, source, destinationParent string,
	owned map[string]bool, depth int,
) (int64, int64, error) {
	if node == nil || depth > 256 || node.Source != source || !canonicalTransferPath(sourceRemote, source) ||
		!transferSafeName(node.Entry.Name) || node.Entry.Symlink || node.Entry.Size < 0 || node.Total < 0 || node.Done && node.Skipped {
		return 0, 0, errors.New("传输清单节点无效")
	}
	if node.Committing && node.CommitLegacy || (node.Committing || node.CommitLegacy) &&
		(node.Entry.Directory || node.Done || node.Skipped || node.Temporary == "") {
		return 0, 0, errors.New("传输提交阶段无效")
	}
	if node.Digest != "" {
		decoded, err := hex.DecodeString(node.Digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != node.Digest || node.Entry.Directory {
			return 0, 0, errors.New("传输内容基线无效")
		}
	}
	if node.Staged != nil && (node.Entry.Directory || node.Staged.Directory || node.Staged.Symlink ||
		node.Staged.Size != node.Entry.Size || node.Digest == "") {
		return 0, 0, errors.New("传输暂存基线无效")
	}
	if node.Committing && (node.Staged == nil || node.Digest == "") {
		return 0, 0, errors.New("传输提交缺少已验证基线")
	}
	targetRemote := !sourceRemote
	if node.Destination != "" {
		if !canonicalTransferPath(targetRemote, node.Destination) || transferDir(targetRemote, node.Destination) != destinationParent ||
			!validTransferDestinationName(node.Entry.Name, transferBase(targetRemote, node.Destination), node.Entry.Directory) {
			return 0, 0, errors.New("传输目标越过所属目录")
		}
	}
	if node.Temporary != "" {
		if node.Entry.Directory || node.Destination == "" || node.Done || node.Skipped ||
			!validTransferTemporary(targetRemote, node.Temporary, node.Destination) || owned[node.Temporary] {
			return 0, 0, errors.New("传输部分文件路径无效")
		}
		owned[node.Temporary] = true
	}
	if node.Approved != nil && (node.Destination == "" || node.Approved.Symlink || node.Approved.Directory != node.Entry.Directory || node.Approved.Size < 0) {
		return 0, 0, errors.New("传输冲突基线无效")
	}
	if !node.Entry.Directory {
		if len(node.Children) != 0 || node.Total != node.Entry.Size {
			return 0, 0, errors.New("文件清单包含子节点或错误大小")
		}
		if node.Skipped {
			return 0, 0, nil
		}
		if node.Done {
			if node.Destination == "" {
				return 0, 0, errors.New("已完成文件没有目标路径")
			}
			return node.Total, node.Total, nil
		}
		return node.Total, 0, nil
	}
	if node.Done && node.Destination == "" {
		return 0, 0, errors.New("已完成目录没有目标路径")
	}
	if node.Skipped && node.Destination != "" {
		return 0, 0, errors.New("跳过目录已包含目标状态")
	}
	var original, total, done int64
	seen := map[string]bool{}
	for _, child := range node.Children {
		if child == nil || seen[child.Entry.Name] || !transferSafeName(child.Entry.Name) {
			return 0, 0, errors.New("传输清单包含重复或无效子节点")
		}
		seen[child.Entry.Name] = true
		if node.Skipped && (child.Destination != "" || child.Done || child.Skipped) {
			return 0, 0, errors.New("跳过目录包含已处理子项")
		}
		if child.Destination != "" && node.Destination == "" {
			return 0, 0, errors.New("子节点目标没有所属目录")
		}
		childTotal, childDone, err := validateTransferNode(child, sourceRemote, transferJoin(sourceRemote, source, child.Entry.Name), node.Destination, owned, depth+1)
		if err != nil {
			return 0, 0, err
		}
		if child.Total > math.MaxInt64-original || childTotal > math.MaxInt64-total || childDone > math.MaxInt64-done {
			return 0, 0, errors.New("传输清单大小溢出")
		}
		original, total, done = original+child.Total, total+childTotal, done+childDone
		if node.Done && !child.Done && !child.Skipped {
			return 0, 0, errors.New("已完成目录仍有未完成子项")
		}
	}
	if original != node.Total {
		return 0, 0, errors.New("目录清单总大小不一致")
	}
	if node.Skipped {
		if done != 0 {
			return 0, 0, errors.New("跳过目录包含已提交子项")
		}
		return 0, 0, nil
	}
	return total, done, nil
}
