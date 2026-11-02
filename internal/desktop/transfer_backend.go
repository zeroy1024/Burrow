package desktop

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"math"
	"net"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sshdesk/internal/fileclient"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var errTransferConflictCanceled = errors.New("传输已取消")

type transferConflictChoice struct {
	choice   int
	applyAll bool
}

type transferClientResult struct {
	client fileclient.Client
	err    error
}

type transferRun struct {
	ctx            context.Context
	cancel         context.CancelFunc
	request        *sshConnectionRequest
	ready          chan transferClientResult
	bytes          atomic.Int64
	written        atomic.Int64
	total          atomic.Int64
	awaitingClient bool
}

// A job and its manifest have one worker owner at a time. The next attempt only
// starts after the preceding worker has returned and its UI callback ran.
type transferJob struct {
	sourceRemote           bool
	sourcePath, targetPath string
	name, destinationKey   string
	policy                 int
	batch                  *transferBatch
	root                   *transferNode
	total, doneBytes       int64
	temporary              map[string]struct{}
}

type transferBatch struct {
	id     int
	policy atomic.Int32
}

type transferNode struct {
	source, destination, temporary string
	entry                          fileclient.Entry
	children                       []*transferNode
	approved                       *fileclient.Entry
	staged                         *fileclient.Entry
	partial                        *fileclient.Entry
	checkpoint                     *transferCheckpoint
	digest                         string
	committing                     bool
	commitLegacy                   bool
	total                          int64
	done, skipped                  bool
}

// Only a digest and acknowledged offset are persisted, never file bytes or a
// hash implementation's internal buffer. Resume reconstructs the hash while
// verifying the target prefix and then reads the source from this offset.
type transferCheckpoint struct {
	Offset int64  `json:"offset"`
	Digest string `json:"digest"`
}

func transferPolicy(policy string) int {
	return map[string]int{"overwrite": 1, "skip": 2, "rename": 3}[policy]
}

func transferFraction(task *transferTask) float32 {
	if task.state == transferDone {
		return 1
	}
	if task.total == 0 {
		return 0
	}
	return float32(min(max(float64(task.transferred)/float64(task.total), 0), 1))
}

func (a *app) scheduleTransfers() {
	if a.ctx.Err() != nil || a.transfers.quitting {
		return
	}
	active := 0
	for _, task := range a.transfers.tasks {
		if task.run != nil {
			active++
		}
	}
	for _, task := range a.transfers.tasks {
		if active >= a.prefs.TransferParallel {
			break
		}
		if task.state != transferQueued || task.run != nil || task.removeRequested {
			continue
		}
		if task.session.endpoint.protocol == "ftp" && task.session.endpoint.session.Auth != "anonymous" && task.session.ftpSecret == "" {
			waiting := slices.ContainsFunc(a.transfers.tasks, func(other *transferTask) bool {
				return other.session == task.session && other.run != nil && other.run.awaitingClient
			})
			if waiting {
				continue
			}
		}
		a.startTransfer(task, false)
		active++
	}
}

func (a *app) startTransfer(task *transferTask, cleanup bool) {
	ctx, cancel := context.WithCancel(a.ctx)
	run := &transferRun{ctx: ctx, cancel: cancel, ready: make(chan transferClientResult, 1), awaitingClient: true}
	task.run, task.speed = run, 0
	if !cleanup {
		task.state, task.errorText = transferRunning, ""
		if task.started.IsZero() {
			task.started = time.Now()
		}
	}
	job := task.job
	if job.destinationKey == "" {
		job.destinationKey = "local"
		if !job.sourceRemote {
			h := task.session.endpoint
			job.destinationKey = fmt.Sprintf("%q/%q/%q/%d", h.protocol, h.user, h.address, h.port)
		}
	}
	locks := &a.transfers.locks
	// Start the worker before requesting a client: context cancellation must
	// release it even if an application-exit UI callback is never delivered.
	a.sshWorkers.Go(func() {
		var remote fileclient.Client
		var err error
		if cleanup && job.sourceRemote {
			// Download staging files live locally; no authentication is needed.
		} else {
			select {
			case result := <-run.ready:
				remote, err = result.client, result.err
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		local := fileclient.NewLocal()
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			source, target := local, remote
			if job.sourceRemote {
				source, target = remote, local
			}
			if cleanup {
				err = cleanupTransferFiles(ctx, target, job)
			} else {
				stopProgress := make(chan struct{})
				a.sshWorkers.Go(func() { a.trackTransferProgress(task, run, stopProgress) })
				err = a.performTransfer(ctx, task, run, job, source, target, locks)
				close(stopProgress)
			}
		}
		closeErr := local.Close()
		if remote != nil {
			closeErr = errors.Join(closeErr, remote.Close())
		}
		if err == nil {
			err = closeErr
		}
		if a.ctx.Err() != nil {
			return
		}
		a.window.Update(func() { a.finishTransfer(task, run, cleanup, err) })
	})
	if cleanup && job.sourceRemote {
		run.awaitingClient = false
		return
	}
	if task.session.endpoint.protocol == "ftp" && task.session.ftpSecret == "" && sameTransferView(task.view, task.session) {
		task.session.ftpSecret = task.view.ftpSecret
	}
	run.request = a.openFileClient(ctx, task.session, func(client fileclient.Client, err error) {
		if ctx.Err() != nil {
			if client != nil {
				client.Close()
			}
			return
		}
		run.awaitingClient = false
		run.ready <- transferClientResult{client, err}
		a.scheduleTransfers()
	})
}

// A browser's explicit reconnect may adopt edited saved settings. Jobs retain
// this private snapshot and the original shared transport group independently.
func (a *app) snapshotTransferSession(view *fileSession) *fileSession {
	snapshot := a.newFileSession(view.host, view.endpoint, view.temporary, view.connection)
	snapshot.restoreID = view.restoreID
	snapshot.ftpSecret = view.ftpSecret
	snapshot.local.path, snapshot.remote.path = view.local.path, view.remote.path
	snapshot.local.history, snapshot.remote.history = slices.Clone(view.local.history), slices.Clone(view.remote.history)
	snapshot.local.historyIndex, snapshot.remote.historyIndex = view.local.historyIndex, view.remote.historyIndex
	return snapshot
}

func sameTransferView(view, snapshot *fileSession) bool {
	if view == nil {
		return false
	}
	one, two := view.endpoint, snapshot.endpoint
	if one.address != two.address || one.port != two.port || one.user != two.user || one.protocol != two.protocol ||
		one.session.ID != two.session.ID || one.session.Auth != two.session.Auth || one.session.KeyID != two.session.KeyID ||
		one.session.PasswordSaved != two.session.PasswordSaved || one.session.PasswordRevision != two.session.PasswordRevision || !one.session.UpdatedAt.Equal(two.session.UpdatedAt) {
		return false
	}
	if (one.session.Files == nil) != (two.session.Files == nil) ||
		one.session.Files != nil && *one.session.Files != *two.session.Files {
		return false
	}
	return one.protocol == "ftp" || view.connection == snapshot.connection
}

func (a *app) trackTransferProgress(task *transferTask, run *transferRun, stopped <-chan struct{}) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	previous, previousAt := int64(0), time.Now()
	for {
		select {
		case now := <-ticker.C:
			bytes, total, written := run.bytes.Load(), run.total.Load(), run.written.Load()
			speed := float64(max(written-previous, 0)) / now.Sub(previousAt).Seconds()
			previous, previousAt = written, now
			a.window.Update(func() {
				if task.run != run || run.ctx.Err() != nil {
					return
				}
				task.transferred, task.total, task.speed = bytes, total, speed
				a.transfers.displaySpeed = a.transfers.summary().speed
			})
		case <-stopped:
			return
		case <-run.ctx.Done():
			return
		}
	}
}

func (a *app) finishTransfer(task *transferTask, run *transferRun, cleanup bool, err error) {
	if task.run != run {
		return
	}
	task.run = nil
	run.cancel()
	cancelSSHRequest(run.request)
	task.speed = 0
	if cleanup {
		if err == nil {
			a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(candidate *transferTask) bool { return candidate == task })
		} else {
			task.removeRequested, task.cleanupPending = false, true
			task.state, task.errorText = transferFailed, "取消后的临时文件清理失败："+err.Error()
			task.failureNotice = true
		}
	} else {
		task.transferred, task.total = run.bytes.Load(), run.total.Load()
		if task.removeRequested || errors.Is(err, errTransferConflictCanceled) {
			task.removeRequested = true
			if len(task.job.temporary) == 0 {
				a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(candidate *transferTask) bool { return candidate == task })
			} else if a.transfers.quitting {
				task.removeRequested, task.cleanupPending = false, true
				task.state, task.errorText = transferFailed, "取消后的临时文件等待清理"
			} else {
				a.startTransfer(task, true)
			}
		} else if task.state == transferPaused || task.state == transferQueued {
			// A pause may already have been followed by resume. The old worker
			// still has to finish before the new attempt can own its manifest.
		} else if err != nil {
			task.state, task.errorText, task.failureNotice = transferFailed, err.Error(), true
		} else {
			task.state, task.completed = transferDone, time.Now()
			if destination := task.job.root.destination; destination != "" {
				task.entry.name = transferBase(!task.sourceRemote, destination)
				task.targetPath = transferDir(!task.sourceRemote, destination)
			}
			a.completeFileTransfer(task)
		}
	}
	a.transfers.displaySpeed = a.transfers.summary().speed
	a.advanceTransferConflicts()
	a.scheduleTransfers()
	a.finishTransferQuitIfReady()
}

func (a *app) pauseTransfer(task *transferTask) {
	if task.removeRequested || task.cleanupPending {
		return
	}
	task.state, task.speed = transferPaused, 0
	a.transfers.displaySpeed = a.transfers.summary().speed
	if task.run != nil {
		task.run.cancel()
		cancelSSHRequest(task.run.request)
	}
	a.advanceTransferConflicts()
}

func (a *app) resumeTransfer(task *transferTask) {
	if a.transfers.quitting {
		return
	}
	if task.state == transferRunning || task.state == transferDone {
		return
	}
	if task.cleanupPending {
		a.cancelTransfer(task)
		return
	}
	if task.removeRequested {
		return
	}
	task.state, task.errorText, task.failureNotice = transferQueued, "", false
	a.scheduleTransfers()
}

func (a *app) cancelTransfer(task *transferTask) {
	if a.transfers.quitting {
		return
	}
	if task.removeRequested {
		return
	}
	task.removeRequested, task.failureNotice = true, false
	if task.run != nil {
		task.run.cancel()
		cancelSSHRequest(task.run.request)
	} else if task.job == nil || len(task.job.temporary) == 0 {
		a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(candidate *transferTask) bool { return candidate == task })
		a.scheduleTransfers()
	} else {
		a.startTransfer(task, true)
	}
	a.advanceTransferConflicts()
}

func (a *app) performTransfer(ctx context.Context, task *transferTask, run *transferRun, job *transferJob,
	source, target fileclient.Client, locks *transferLocks,
) error {
	if job.root == nil {
		sourcePath, err := transferAbsolute(ctx, source, job.sourceRemote, job.sourcePath)
		if err != nil {
			return err
		}
		targetPath, err := transferAbsolute(ctx, target, !job.sourceRemote, job.targetPath)
		if err != nil {
			return err
		}
		if !transferSafeName(job.name) {
			return errors.New("文件名不能安全地用于目标路径")
		}
		root, err := buildTransferManifest(ctx, source, job.sourceRemote, sourcePath)
		if err != nil {
			return err
		}
		job.root, job.total, job.sourcePath, job.targetPath = root, root.total, sourcePath, targetPath
	}
	run.total.Store(job.total)
	run.bytes.Store(job.doneBytes)
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	reserve := func(name string, directory bool) error {
		release, err := locks.acquire(ctx, job.destinationKey, name, directory, job)
		if err == nil {
			releases = append(releases, release)
		}
		return err
	}
	if job.root.destination != "" {
		if err := reserve(job.root.destination, job.root.entry.Directory); err != nil {
			return err
		}
	}
	if err := reconcileTransferCommits(ctx, source, target, job, job.root, run); err != nil {
		return err
	}
	if err := verifyCompletedTransfers(ctx, target, job.root); err != nil {
		return err
	}
	return a.copyTransferNode(ctx, task, run, job, source, target, job.root,
		transferJoin(!job.sourceRemote, job.targetPath, job.name), reserve)
}

func buildTransferManifest(ctx context.Context, client fileclient.Client, remote bool, source string) (*transferNode, error) {
	return buildTransferManifestAt(ctx, client, remote, source, 0)
}

func buildTransferManifestAt(ctx context.Context, client fileclient.Client, remote bool, source string, depth int) (*transferNode, error) {
	if depth > 256 {
		return nil, errors.New("目录层级过深，已停止传输")
	}
	entry, err := client.Stat(ctx, source)
	if err != nil {
		return nil, err
	}
	if entry.Symlink {
		return nil, fmt.Errorf("符号链接不会被递归跟随：%s", source)
	}
	if entry.Size < 0 {
		return nil, fmt.Errorf("文件大小无效：%s", source)
	}
	node := &transferNode{source: source, entry: entry}
	if !entry.Directory {
		node.total = entry.Size
		return node, nil
	}
	entries, err := client.ReadDir(ctx, source)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, child := range entries {
		if !transferSafeName(child.Name) || seen[child.Name] {
			return nil, errors.New("目录响应包含无效或重复文件名")
		}
		seen[child.Name] = true
		nested, err := buildTransferManifestAt(ctx, client, remote, transferJoin(remote, source, child.Name), depth+1)
		if err != nil {
			return nil, err
		}
		// The checked directory component determines the destination name; a
		// protocol Stat response must not replace it with another server path.
		nested.entry.Name = child.Name
		if nested.total > math.MaxInt64-node.total {
			return nil, errors.New("传输总大小超过支持范围")
		}
		node.total += nested.total
		node.children = append(node.children, nested)
	}
	return node, nil
}

func (a *app) copyTransferNode(ctx context.Context, task *transferTask, run *transferRun, job *transferJob,
	source, target fileclient.Client, node *transferNode, proposed string, reserve func(string, bool) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.done || node.skipped {
		return nil
	}
	fresh, err := source.Stat(ctx, node.source)
	if err != nil {
		return err
	}
	if fresh.Symlink || fresh.Directory != node.entry.Directory || (!fresh.Directory && !sameTransferEntry(fresh, node.entry)) {
		return fmt.Errorf("源文件在传输期间已改变：%s", node.source)
	}
	if node.destination == "" {
		destination, approved, skip, err := a.chooseTransferDestination(ctx, task, job, target,
			proposed, node.entry, reserve)
		if err != nil {
			return err
		}
		if skip {
			node.skipped = true
			job.total -= node.total
			run.total.Store(job.total)
			return nil
		}
		node.destination, node.approved = destination, approved
		if node == job.root {
			a.publishTransferDestination(task, run, job.sourceRemote, destination)
		}
	} else if err := reserve(node.destination, node.entry.Directory); err != nil {
		return err
	}
	if node.entry.Directory {
		existing, err := target.Stat(ctx, node.destination)
		if errors.Is(err, fs.ErrNotExist) {
			err = target.Mkdir(ctx, node.destination)
		} else if err == nil && (!existing.Directory || existing.Symlink) {
			err = fmt.Errorf("目标不是普通目录：%s", node.destination)
		}
		if err != nil {
			return err
		}
		for _, child := range node.children {
			if err := a.copyTransferNode(ctx, task, run, job, source, target, child,
				transferJoin(!job.sourceRemote, node.destination, child.entry.Name), reserve); err != nil {
				return err
			}
		}
		node.done = true
		return nil
	}
	if node.temporary == "" {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return err
		}
		name := ".sshdesk-" + hex.EncodeToString(suffix[:]) + ".part"
		temporary := transferJoin(!job.sourceRemote, transferDir(!job.sourceRemote, node.destination), name)
		if temporary == node.destination {
			return errors.New("临时文件名称与目标相同")
		}
		if _, err := target.Stat(ctx, temporary); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return errors.New("临时文件名称已被占用")
			}
			return err
		}
		node.temporary = temporary
		job.temporary[node.temporary] = struct{}{}
	}
	resume, err := prepareTransferResume(ctx, source, target, node)
	if err != nil {
		return err
	}
	offset := resume.offset
	run.bytes.Store(job.doneBytes + offset)
	if offset < node.entry.Size || !resume.exists {
		input := resume.input
		if input == nil {
			input, err = source.OpenRead(ctx, node.source, offset)
			if err != nil {
				return err
			}
		}
		node.partial = nil
		output, err := target.OpenWrite(ctx, node.temporary, offset, offset == 0)
		if err != nil {
			return errors.Join(err, input.Close())
		}
		writer := transferCountingWriter{
			writer: output, bytes: &run.bytes, written: &run.written,
			hash: resume.hash, offset: offset, node: node,
		}
		reader := transferSizedReader(input, node.entry.Size-offset)
		copied, copyErr := io.CopyBuffer(&writer, reader, make([]byte, 64<<10))
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if copied != node.entry.Size-offset {
			return fmt.Errorf("源文件大小在传输期间已改变：%s", node.source)
		}
	} else if resume.input != nil {
		// A legacy full-length part may have needed a source-prefix comparison.
		// Consume EOF before Close so FTP can finish its data command normally.
		extra, readErr := io.Copy(io.Discard, transferSizedReader(resume.input, 0))
		closeErr := resume.input.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		if extra != 0 {
			return fmt.Errorf("源文件大小在传输期间已改变：%s", node.source)
		}
	}
	digest := hex.EncodeToString(resume.hash.Sum(nil))
	fresh, err = source.Stat(ctx, node.source)
	if err != nil {
		return err
	}
	if !sameTransferEntry(fresh, node.entry) || node.digest != "" && node.digest != digest {
		return fmt.Errorf("源文件在传输期间已改变：%s", node.source)
	}
	written, err := target.Stat(ctx, node.temporary)
	if err != nil {
		return err
	}
	if written.Symlink || written.Directory || written.Size != node.entry.Size {
		return fmt.Errorf("目标临时文件大小不匹配：%s", node.temporary)
	}
	node.staged, node.partial, node.digest = &written, &written, digest
	for {
		current, err := target.Stat(ctx, node.destination)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if errors.Is(err, fs.ErrNotExist) {
			// Approval belonged to the previous inode. A later arrival must win
			// an exclusive commit and get its own conflict/permission decision.
			node.approved = nil
		}
		if err == nil && (node.approved == nil || !sameTransferEntry(current, *node.approved)) {
			destination, approved, skip, err := a.chooseTransferDestination(ctx, task, job, target,
				node.destination, node.entry, reserve)
			if err != nil {
				return err
			}
			if skip {
				if err := discardTransferStage(ctx, target, job, node); err != nil {
					return err
				}
				node.skipped = true
				job.total -= node.total
				run.total.Store(job.total)
				run.bytes.Store(job.doneBytes)
				return nil
			}
			node.destination, node.approved = destination, approved
			if node == job.root {
				a.publishTransferDestination(task, run, job.sourceRemote, destination)
			}
			continue // Refresh metadata if conflict handling selected another name.
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := preserveTransferPermissions(ctx, target, node, current, err == nil); err != nil {
			return err
		}
		node.committing = true
		noReplace := node.approved == nil
		err = target.Rename(ctx, node.temporary, node.destination, fileclient.RenameOptions{NoReplace: noReplace})
		if err == nil {
			markTransferCommitted(job, node, run)
			return nil
		}
		if noReplace && errors.Is(err, fs.ErrExist) {
			node.committing = false
			continue // The exclusive commit lost a race; ask about the real target.
		}
		if !uncertainTransferCommit(ctx, target, err) {
			node.committing = false
			return fmt.Errorf("提交传输文件失败：%w", err)
		}
		return fmt.Errorf("提交结果不确定，重试时将核对暂存文件和目标：%w", err)
	}
}

type transferResume struct {
	offset int64
	hash   hash.Hash
	input  io.ReadCloser // Only a legacy part needs a source read from zero.
	exists bool
}

func prepareTransferResume(
	ctx context.Context,
	source, target fileclient.Client,
	node *transferNode,
) (resume transferResume, err error) {
	resume.hash = sha256.New()
	defer func() {
		if err != nil && resume.input != nil {
			err = errors.Join(err, resume.input.Close())
			resume.input = nil
		}
	}()
	partial, err := target.Stat(ctx, node.temporary)
	if errors.Is(err, fs.ErrNotExist) {
		node.staged, node.partial, node.checkpoint, node.digest = nil, nil, nil, ""
		node.checkpoint = &transferCheckpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
		return resume, nil
	}
	if err != nil {
		return resume, err
	}
	resume.exists = true
	if partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.entry.Size {
		return resume, fmt.Errorf("临时文件状态无效：%s", node.temporary)
	}
	if checkpoint := node.checkpoint; checkpoint != nil && checkpoint.Offset > partial.Size {
		// A failed network write may have acknowledged more bytes than the server
		// retained. None of that prefix is trusted; rewrite this owned part fully.
		node.staged, node.partial, node.digest = nil, nil, ""
		node.checkpoint = &transferCheckpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
		return resume, nil
	}
	if node.staged != nil && !sameTransferEntry(partial, *node.staged) {
		return resume, fmt.Errorf("已验证的临时文件已改变：%s", node.temporary)
	}
	if node.partial != nil && !sameTransferEntry(partial, *node.partial) {
		return resume, fmt.Errorf("已记录的部分文件已改变：%s", node.temporary)
	}
	checkpoint := node.checkpoint
	legacyCompared := false
	if checkpoint == nil {
		checkpoint = &transferCheckpoint{Offset: partial.Size}
		if partial.Size == node.entry.Size && node.digest != "" {
			checkpoint.Digest = node.digest
		} else if partial.Size == 0 {
			checkpoint.Digest = hex.EncodeToString(resume.hash.Sum(nil))
		} else {
			// Old journals did not know the prefix digest. Compare it once with
			// the source and keep this stream positioned for the remaining copy.
			// Read the target first so a large verification cannot leave an open
			// FTP source data connection idle while the target prefix is scanned.
			targetDigest, readErr := hashTransferFile(ctx, target, node.temporary, partial.Size)
			if readErr != nil {
				return resume, fmt.Errorf("无法校验旧版已传部分，未继续复用：%w", readErr)
			}
			resume.input, err = source.OpenRead(ctx, node.source, 0)
			if err != nil {
				return resume, err
			}
			if _, err = io.CopyN(resume.hash, resume.input, partial.Size); err != nil {
				return resume, err
			}
			checkpoint.Digest = hex.EncodeToString(resume.hash.Sum(nil))
			fresh, statErr := target.Stat(ctx, node.temporary)
			if statErr != nil {
				return resume, statErr
			}
			if checkpoint.Digest != targetDigest || !sameTransferEntry(fresh, partial) {
				return resume, fmt.Errorf("旧版已传部分已改变，未继续复用：%s", node.temporary)
			}
			legacyCompared = true
		}
	}
	if checkpoint.Offset > 0 && !legacyCompared {
		resume.hash, err = verifiedTransferPrefix(ctx, target, node.temporary, partial, checkpoint)
		if err != nil {
			return resume, err
		}
	} else if checkpoint.Digest != hex.EncodeToString(resume.hash.Sum(nil)) {
		return resume, errors.New("传输前缀检查点无效")
	}
	resume.offset = checkpoint.Offset
	node.checkpoint, node.partial = checkpoint, &partial
	return resume, nil
}

func verifiedTransferPrefix(
	ctx context.Context,
	target fileclient.Client,
	name string,
	partial fileclient.Entry,
	checkpoint *transferCheckpoint,
) (hash.Hash, error) {
	input, err := target.OpenRead(ctx, name, 0)
	if err != nil {
		return nil, fmt.Errorf("无法校验已传部分，未继续复用：%s：%w", name, err)
	}
	h := sha256.New()
	prefix, readErr := io.CopyN(h, input, checkpoint.Offset)
	tail := int64(0)
	if readErr == nil {
		// FTP requires the RETR data stream to reach EOF before its final reply.
		// Unacknowledged trailing bytes are discarded and overwritten on resume.
		tail, readErr = io.Copy(io.Discard, transferSizedReader(input, partial.Size-checkpoint.Offset))
	}
	closeErr := input.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if prefix+tail != partial.Size || hex.EncodeToString(h.Sum(nil)) != checkpoint.Digest {
		return nil, fmt.Errorf("已传部分的内容已改变，未继续复用：%s", name)
	}
	fresh, err := target.Stat(ctx, name)
	if err != nil {
		return nil, err
	}
	if !sameTransferEntry(fresh, partial) {
		return nil, fmt.Errorf("部分文件在核对期间已改变：%s", name)
	}
	return h, nil
}

func preserveTransferPermissions(
	ctx context.Context,
	target fileclient.Client,
	node *transferNode,
	current fileclient.Entry,
	exists bool,
) error {
	if !exists {
		// A new target keeps its client's creation policy. Requiring chmod here
		// would reject otherwise valid uploads to FTP servers without SITE CHMOD.
		return nil
	}
	if !current.ModeKnown {
		return fmt.Errorf("无法确认已有目标的权限，未执行覆盖：%s", node.destination)
	}
	mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
	mode := current.Mode & mask
	setter, ok := target.(fileclient.PermissionSetter)
	if !ok {
		return fmt.Errorf("目标不支持保留文件权限，未提交：%s", node.destination)
	}
	if err := setter.Chmod(ctx, node.temporary, mode); err != nil {
		return fmt.Errorf("保留目标权限失败，未提交：%s：%w", node.destination, err)
	}
	staged, err := target.Stat(ctx, node.temporary)
	if err != nil {
		return err
	}
	if staged.Symlink || staged.Directory || staged.Size != node.entry.Size ||
		!staged.ModeKnown || staged.Mode&mask != mode {
		return fmt.Errorf("无法确认暂存文件权限，未提交：%s", node.temporary)
	}
	node.staged, node.partial = &staged, &staged
	return nil
}

func transferSizedReader(reader io.Reader, size int64) io.Reader {
	if size < math.MaxInt64 {
		return io.LimitReader(reader, size+1)
	}
	return reader
}

func hashTransferFile(ctx context.Context, client fileclient.Client, name string, size int64) (string, error) {
	input, err := client.OpenRead(ctx, name, 0)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	count, copyErr := io.CopyBuffer(hash, transferSizedReader(input, size), make([]byte, 64<<10))
	closeErr := input.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.Join(copyErr, closeErr)
	}
	if count != size {
		return "", fmt.Errorf("核对文件大小已改变：%s", name)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func uncertainTransferCommit(ctx context.Context, target fileclient.Client, err error) bool {
	if ctx.Err() != nil || errors.Is(err, fs.ErrClosed) || errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var network net.Error
	var protocol textproto.ProtocolError
	if errors.As(err, &network) || errors.As(err, &protocol) {
		return true
	}
	select {
	case <-target.Done():
		return true
	default:
		return false
	}
}

func markTransferCommitted(job *transferJob, node *transferNode, run *transferRun) {
	delete(job.temporary, node.temporary)
	node.temporary, node.done, node.committing, node.commitLegacy = "", true, false, false
	node.partial, node.checkpoint = nil, nil
	job.doneBytes += node.entry.Size
	run.bytes.Store(job.doneBytes)
}

func discardTransferStage(ctx context.Context, target fileclient.Client, job *transferJob, node *transferNode) error {
	if node.temporary != "" {
		if err := target.Remove(ctx, node.temporary, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		delete(job.temporary, node.temporary)
	}
	node.temporary, node.digest, node.staged = "", "", nil
	node.partial, node.checkpoint = nil, nil
	node.committing, node.commitLegacy = false, false
	return nil
}

func verifyCompletedTransfers(ctx context.Context, target fileclient.Client, node *transferNode) error {
	if node.skipped {
		return nil
	}
	if node.done {
		entry, err := target.Stat(ctx, node.destination)
		if err != nil {
			return fmt.Errorf("已完成目标已改变，请重新排队：%s：%w", node.destination, err)
		}
		if entry.Symlink || entry.Directory != node.entry.Directory || !entry.Directory && entry.Size != node.entry.Size {
			return fmt.Errorf("已完成目标类型或大小已改变，请重新排队：%s", node.destination)
		}
	}
	for _, child := range node.children {
		if err := verifyCompletedTransfers(ctx, target, child); err != nil {
			return err
		}
	}
	return nil
}

func reconcileTransferCommits(ctx context.Context, source, target fileclient.Client, job *transferJob, node *transferNode, run *transferRun) error {
	if node.committing || node.commitLegacy {
		if err := recoverTransferCommit(ctx, source, target, job, node, run); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := reconcileTransferCommits(ctx, source, target, job, child, run); err != nil {
			return err
		}
	}
	return nil
}

func recoverTransferCommit(ctx context.Context, source, target fileclient.Client, job *transferJob, node *transferNode, run *transferRun) error {
	uncertain := func(err error) error {
		return fmt.Errorf("提交结果仍不确定，请核对后重试或取消并重新排队：%s：%w", node.destination, err)
	}
	partial, partialErr := target.Stat(ctx, node.temporary)
	if partialErr != nil && !errors.Is(partialErr, fs.ErrNotExist) {
		return uncertain(partialErr)
	}
	partialExists := partialErr == nil
	if partialExists && (partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.entry.Size) {
		return uncertain(errors.New("暂存文件类型或大小已改变"))
	}
	if partialExists && node.staged != nil && !sameTransferEntry(partial, *node.staged) {
		return uncertain(errors.New("已验证的暂存文件已改变"))
	}
	entry, targetErr := target.Stat(ctx, node.destination)
	if errors.Is(targetErr, fs.ErrNotExist) {
		if partialExists {
			node.committing, node.commitLegacy = false, false
			return nil // The owned bytes remain staged; no final target exists.
		}
		return uncertain(errors.New("暂存文件和目标都不存在"))
	}
	if targetErr != nil {
		return uncertain(targetErr)
	}
	if entry.Symlink || entry.Directory || entry.Size != node.entry.Size {
		if partialExists {
			node.committing, node.commitLegacy = false, false
			return nil
		}
		return uncertain(errors.New("目标类型或大小不匹配"))
	}
	if node.digest == "" {
		// Version 1 journals did not record commit state. Reconstruct only the
		// expected source digest after validating its original metadata baseline.
		fresh, err := source.Stat(ctx, node.source)
		if err != nil {
			return uncertain(err)
		}
		if !sameTransferEntry(fresh, node.entry) {
			return uncertain(errors.New("原始源文件已改变"))
		}
		digest, err := hashTransferFile(ctx, source, node.source, node.entry.Size)
		if err != nil {
			return uncertain(err)
		}
		fresh, err = source.Stat(ctx, node.source)
		if err != nil {
			return uncertain(err)
		}
		if !sameTransferEntry(fresh, node.entry) {
			return uncertain(errors.New("原始源文件已改变"))
		}
		node.digest = digest
	}
	digest, err := hashTransferFile(ctx, target, node.destination, node.entry.Size)
	if err != nil {
		return uncertain(fmt.Errorf("无法读回目标核对：%w", err))
	}
	fresh, err := target.Stat(ctx, node.destination)
	if err != nil {
		return uncertain(err)
	}
	if !sameTransferEntry(fresh, entry) {
		return uncertain(errors.New("目标在核对期间已改变"))
	}
	if digest != node.digest {
		if partialExists {
			node.committing, node.commitLegacy = false, false
			return nil
		}
		return uncertain(errors.New("目标内容不匹配"))
	}
	if partialExists {
		if err := target.Remove(ctx, node.temporary, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return uncertain(fmt.Errorf("目标内容已确认，但暂存副本清理失败：%w", err))
		}
	}
	markTransferCommitted(job, node, run)
	return nil
}

func (a *app) publishTransferDestination(task *transferTask, run *transferRun, sourceRemote bool, destination string) {
	name, directory := transferBase(!sourceRemote, destination), transferDir(!sourceRemote, destination)
	a.window.Update(func() {
		if task.run == run {
			task.entry.name, task.targetPath = name, directory
		}
	})
}

type transferCountingWriter struct {
	writer  io.Writer
	bytes   *atomic.Int64
	written *atomic.Int64
	hash    hash.Hash
	offset  int64
	node    *transferNode
}

func (w *transferCountingWriter) Write(buffer []byte) (int, error) {
	n, err := w.writer.Write(buffer)
	w.bytes.Add(int64(n))
	w.written.Add(int64(n))
	if n > 0 {
		w.hash.Write(buffer[:n])
		w.offset += int64(n)
		w.node.partial = nil
		if w.offset <= w.node.entry.Size {
			w.node.checkpoint = &transferCheckpoint{Offset: w.offset, Digest: hex.EncodeToString(w.hash.Sum(nil))}
		} else {
			// The extra EOF-check byte detects a growing source. Do not persist
			// an acknowledged offset outside the immutable manifest's bounds.
			w.node.checkpoint = nil
		}
	}
	return n, err
}

func (a *app) chooseTransferDestination(ctx context.Context, task *transferTask, job *transferJob,
	target fileclient.Client, proposed string, source fileclient.Entry, reserve func(string, bool) error,
) (string, *fileclient.Entry, bool, error) {
	remote := !job.sourceRemote
	parent, original := transferDir(remote, proposed), transferBase(remote, proposed)
	candidate, index := proposed, 2
	for {
		if err := reserve(candidate, source.Directory); err != nil {
			return "", nil, false, err
		}
		existing, err := target.Stat(ctx, candidate)
		if errors.Is(err, fs.ErrNotExist) {
			return candidate, nil, false, nil
		}
		if err != nil {
			return "", nil, false, err
		}
		choice := int(job.batch.policy.Load())
		if choice == 0 {
			answer, err := a.askTransferConflict(ctx, task, transferDir(remote, candidate),
				fileEntry{name: transferBase(remote, candidate), size: source.Size, dir: source.Directory})
			if err != nil {
				return "", nil, false, err
			}
			choice = answer.choice
		}
		switch choice {
		case 1:
			if existing.Symlink || existing.Directory != source.Directory {
				return "", nil, false, fmt.Errorf("目标类型不同或为符号链接，请选择保留两者：%s", candidate)
			}
			return candidate, &existing, false, nil
		case 2:
			return "", nil, true, nil
		case 3:
			for {
				name := transferDuplicateName(original, index, source.Directory)
				index++
				candidate = transferJoin(remote, parent, name)
				if err := reserve(candidate, source.Directory); err != nil {
					return "", nil, false, err
				}
				_, err := target.Stat(ctx, candidate)
				if errors.Is(err, fs.ErrNotExist) {
					return candidate, nil, false, nil
				}
				if err != nil {
					return "", nil, false, err
				}
			}
		default:
			return "", nil, false, errTransferConflictCanceled
		}
	}
}

func transferDuplicateName(name string, index int, directory bool) string {
	ext := ""
	if !directory && !strings.HasPrefix(name, ".") {
		ext = path.Ext(name)
	}
	return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), index, ext)
}

func validTransferDestinationName(source, target string, directory bool) bool {
	if !transferSafeName(target) {
		return false
	}
	if source == target {
		return true
	}
	ext := ""
	if !directory && !strings.HasPrefix(source, ".") {
		ext = path.Ext(source)
	}
	if !strings.HasSuffix(target, ext) {
		return false
	}
	base := strings.TrimSuffix(source, ext)
	stem := strings.TrimSuffix(target, ext)
	if !strings.HasPrefix(stem, base) {
		return false
	}
	suffix := strings.TrimPrefix(stem, base)
	if suffix == "" {
		return false
	}
	for suffix != "" {
		if !strings.HasPrefix(suffix, " (") {
			return false
		}
		digits, rest, ok := strings.Cut(suffix[2:], ")")
		index, err := strconv.Atoi(digits)
		if !ok || err != nil || index < 2 || strconv.Itoa(index) != digits {
			return false
		}
		suffix = rest
	}
	return true
}

func sameTransferEntry(left, right fileclient.Entry) bool {
	modeMatches := !left.ModeKnown || !right.ModeKnown || left.Mode.Perm() == right.Mode.Perm()
	return left.Size == right.Size && left.Directory == right.Directory && left.Symlink == right.Symlink &&
		left.Modified.Equal(right.Modified) && modeMatches
}

func transferSafeName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00") && filepath.IsLocal(name)
}

func transferJoin(remote bool, parent, name string) string {
	if remote {
		return path.Join(parent, name)
	}
	return filepath.Join(parent, name)
}

func transferDir(remote bool, name string) string {
	if remote {
		return path.Dir(name)
	}
	return filepath.Dir(name)
}

func transferBase(remote bool, name string) string {
	if remote {
		return path.Base(name)
	}
	return filepath.Base(name)
}

func transferAbsolute(ctx context.Context, client fileclient.Client, remote bool, name string) (string, error) {
	if remote {
		if path.IsAbs(name) {
			return path.Clean(name), nil
		}
		home, err := client.InitialPath(ctx)
		if err != nil {
			return "", err
		}
		if name == "~" {
			return home, nil
		}
		return path.Join(home, strings.TrimPrefix(name, "~/")), nil
	}
	if name == "~" || strings.HasPrefix(name, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		name = filepath.Join(home, strings.TrimPrefix(name, "~"))
	}
	if !filepath.IsAbs(name) {
		return "", fs.ErrInvalid
	}
	return filepath.Clean(name), nil
}

func cleanupTransferFiles(ctx context.Context, target fileclient.Client, job *transferJob) error {
	var failures []error
	for name := range job.temporary {
		entry, err := target.Stat(ctx, name)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.temporary, name)
			clearTransferTemporary(job.root, name)
			continue
		}
		if err == nil && (entry.Directory || entry.Symlink) {
			err = errors.New("临时文件已被替换，未执行删除")
		}
		if err == nil {
			err = target.Remove(ctx, name, false)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		} else {
			delete(job.temporary, name)
			clearTransferTemporary(job.root, name)
		}
	}
	return errors.Join(failures...)
}

func clearTransferTemporary(node *transferNode, name string) {
	if node == nil {
		return
	}
	if node.temporary == name {
		node.temporary = ""
		node.staged, node.digest = nil, ""
		node.partial, node.checkpoint = nil, nil
		node.committing, node.commitLegacy = false, false
	}
	for _, child := range node.children {
		clearTransferTemporary(child, name)
	}
}

func (a *app) askTransferConflict(ctx context.Context, task *transferTask, target string, entry fileEntry) (transferConflictChoice, error) {
	conflict := &fileConflict{session: task.session, sourceRemote: task.sourceRemote, target: target,
		entries: []fileEntry{entry}, reply: make(chan transferConflictChoice, 1), ctx: ctx, task: task}
	a.window.Update(func() {
		if ctx.Err() != nil {
			return
		}
		if policy := int(task.job.batch.policy.Load()); policy != 0 {
			conflict.reply <- transferConflictChoice{choice: policy}
			return
		}
		a.transfers.conflicts = append(a.transfers.conflicts, conflict)
		a.advanceTransferConflicts()
	})
	select {
	case answer := <-conflict.reply:
		if answer.choice == 0 {
			return answer, errTransferConflictCanceled
		}
		return answer, nil
	case <-ctx.Done():
		return transferConflictChoice{}, ctx.Err()
	}
}

func (a *app) advanceTransferConflicts() {
	q := &a.transfers
	session := a.activeTransferSession()
	if a.files.conflict != nil && (session == nil || a.files.conflict.task.view != session) {
		a.files.conflict = nil
	}
	q.conflicts = slices.DeleteFunc(q.conflicts, func(conflict *fileConflict) bool {
		if conflict.ctx.Err() != nil {
			if a.files.conflict == conflict {
				a.files.conflict = nil
			}
			return true
		}
		if policy := int(conflict.task.job.batch.policy.Load()); policy != 0 {
			select {
			case conflict.reply <- transferConflictChoice{choice: policy}:
			default:
			}
			if a.files.conflict == conflict {
				a.files.conflict = nil
			}
			return true
		}
		return false
	})
	if session != nil && a.files.dialog == nil && a.files.conflict == nil {
		for _, conflict := range q.conflicts {
			if conflict.task.view == session {
				a.files.conflict = conflict
				break
			}
		}
	}
}

func (a *app) cancelFileConflict() {
	conflict := a.files.conflict
	if conflict == nil {
		return
	}
	batch := conflict.task.job.batch
	tasks := slices.Clone(a.transfers.tasks)
	// Individual removals may schedule other work. Pause this batch's queued
	// rows first so no remaining selection starts during the cancellation loop.
	for _, task := range tasks {
		if task.job.batch == batch && task.state == transferQueued {
			task.state = transferPaused
		}
	}
	select {
	case conflict.reply <- transferConflictChoice{}:
	default:
	}
	a.transfers.conflicts = slices.DeleteFunc(a.transfers.conflicts, func(candidate *fileConflict) bool {
		return candidate.task.job.batch == batch
	})
	a.files.conflict = nil
	for _, task := range tasks {
		if task.job.batch == batch && task.state != transferDone {
			a.cancelTransfer(task)
		}
	}
	a.advanceTransferConflicts()
}

type transferLocks struct {
	mu      sync.Mutex
	held    map[*transferLock]struct{}
	changed chan struct{}
}

type transferLock struct {
	key, path string
	directory bool
	owner     *transferJob
}

func (l *transferLocks) acquire(ctx context.Context, key, name string, directory bool, owner *transferJob) (func(), error) {
	if key == "local" {
		// Serialize aliases conservatively; actual filesystem paths and conflict
		// decisions retain their original spelling. Exclusive rename seals the
		// remaining race with processes outside this queue.
		name = norm.NFC.String(cases.Fold().String(norm.NFC.String(filepath.ToSlash(name))))
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		l.mu.Lock()
		if l.changed == nil {
			l.changed = make(chan struct{})
			l.held = map[*transferLock]struct{}{}
		}
		blocked := false
		for existing := range l.held {
			if existing.owner == owner || existing.key != key {
				continue
			}
			if existing.path == name || (existing.directory && transferWithin(existing.path, name)) ||
				(directory && transferWithin(name, existing.path)) {
				blocked = true
				break
			}
		}
		if !blocked {
			lock := &transferLock{key, name, directory, owner}
			l.held[lock] = struct{}{}
			l.mu.Unlock()
			return sync.OnceFunc(func() {
				l.mu.Lock()
				delete(l.held, lock)
				close(l.changed)
				l.changed = make(chan struct{})
				l.mu.Unlock()
			}), nil
		}
		changed := l.changed
		l.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func transferWithin(parent, child string) bool {
	return strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}
