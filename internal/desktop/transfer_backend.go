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

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/localfs"
	"github.com/zeroy1024/Burrow/internal/mcpserver"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var errTransferConflictCanceled = locales.NewError(locales.MessageTransferCancelled)

type transferConflictChoice struct {
	choice   int
	applyAll bool
}

type transferClientResult struct {
	client fileclient.Client
	err    error
}

type transferRun struct {
	mcp            *mcpTransferOwner
	manifest       atomic.Pointer[transferManifestSnapshot]
	journal        func() error
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
	mcpFailConflict        bool
	mcpCheck               func(context.Context, fileclient.Client, fileclient.Client, *transferNode, string) error
	mcpFinish              func(context.Context, *transferNode, error) error
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
	stageDirectory                 string
	mcpPath, mcpSide, mcpAction    string
	source, destination, temporary string
	entry                          fileclient.Entry
	children                       []*transferNode
	approved                       *fileclient.Entry
	staged                         *fileclient.Entry
	partial                        *fileclient.Entry
	checkpoint                     *transferCheckpoint
	digest                         string
	committing                     bool
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
	if !a.businessReady() {
		return
	}
	if a.ctx.Err() != nil || a.transfers.quitting || a.transfers.storeErr != nil || a.transfers.writeErr != nil {
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
		if task.run == nil && task.removeRequested && (a.store == nil || task.persisted) {
			a.startTransfer(task, true)
			active++
			continue
		}
		if task.state != transferQueued || task.run != nil || task.removeRequested || a.store != nil && !task.persisted {
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
	base := a.ctx
	if task.mcp != nil {
		base = mcpserver.WithOperation(base, task.mcp.OperationID, task.id)
	}
	ctx, cancel := context.WithCancel(base)
	run := &transferRun{ctx: ctx, cancel: cancel, ready: make(chan transferClientResult, 1), awaitingClient: true, mcp: task.mcp}
	run.bytes.Store(task.transferred)
	run.total.Store(task.total)
	task.run, task.speed = run, 0
	if !cleanup {
		task.state, task.errorText = transferRunning, locales.Text{}
		if task.started.IsZero() {
			task.started = time.Now()
		}
	}
	job := task.job
	run.manifest.Store(transferManifest(job))
	run.journal = func() error { return a.journalTransfer(task.id, run, job) }
	a.persistTransfers()
	if !cleanup {
		a.mcpTransferChanged(task, nil)
	}
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
		var local fileclient.Client
		if task.mcp != nil && !cleanup {
			err = a.mcpBackend.preflightTransfer(ctx, task)
			if err == nil {
				a.window.Update(func() {
					if ctx.Err() != nil || task.run != run {
						return
					}
					if !a.mcpBackend.service.Accepting() {
						run.awaitingClient = false
						run.ready <- transferClientResult{err: locales.NewError(locales.MessageMcpServiceStopped)}
						return
					}
					a.requestTransferClient(task, run)
				})
			}
		}
		if err == nil {
			if task.mcp != nil && !(cleanup && !job.sourceRemote) {
				local, err = fileclient.OpenLocalRoot(ctx, task.mcp.LocalRoot)
			} else if job.sourceRemote {
				// Anchor the selected destination before authentication or a conflict
				// can leave time for another process to replace its descendants.
				local, err = fileclient.OpenLocalRoot(ctx, job.targetPath)
			} else if !cleanup {
				// Keep upload reads inside the selected source while authentication
				// and conflict dialogs run. Remote cleanup does not need the source.
				local, err = fileclient.OpenLocalSource(ctx, job.sourcePath)
			}
		}
		if err == nil && !(cleanup && job.sourceRemote) {
			select {
			case result := <-run.ready:
				remote, err = result.client, result.err
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			source, target := local, remote
			if job.sourceRemote {
				source, target = remote, local
			}
			if task.mcp != nil && !cleanup {
				source = a.mcpBackend.sourceClient(task, source)
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
		var closeErr error
		if local != nil {
			closeErr = local.Close()
		}
		if remote != nil {
			closeErr = errors.Join(closeErr, remote.Close())
		}
		if err == nil {
			err = closeErr
		}
		if journalErr := run.journal(); journalErr != nil {
			err = errors.Join(err, locales.Errorf(locales.MessageSaveTransferCheckpoint, journalErr))
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
	if task.mcp != nil && !cleanup {
		return
	}
	a.requestTransferClient(task, run)
}

func (a *app) requestTransferClient(task *transferTask, run *transferRun) {
	ctx := run.ctx
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
	snapshot.gatewayScope = view.gatewayScope
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
	if !sameGatewayRoute(one.session.GatewayRoute, two.session.GatewayRoute) {
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
			task.state, task.errorText = transferFailed, locales.Concat(
				locales.Deferred(locales.MessageUnableToCleanUpTemporaryFilesAfterCancellation),
				locales.ErrorText(err),
			)
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
				task.state, task.errorText = transferFailed, locales.Deferred(
					locales.MessageTemporaryFilesAwaitingCleanupAfterCancellation,
				)
			} else {
				task.cleanupPending = true
				task.state = transferFailed
			}
		} else if task.state == transferPaused || task.state == transferQueued {
			// A pause may already have been followed by resume. The old worker
			// still has to finish before the new attempt can own its manifest.
		} else if err != nil {
			task.state, task.errorText, task.failureNotice = transferFailed, locales.ErrorText(err), true
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
	setMCPTransferOutcome(task, err)
	a.persistTransfers()
	a.scheduleTransfers()
	a.finishTransferQuitIfReady()
	a.mcpTransferChanged(task, err)
	if task.mcp != nil && task.state == transferDone {
		a.transfers.tasks = slices.DeleteFunc(a.transfers.tasks, func(t *transferTask) bool { return t == task })
		a.persistTransfers()
	}
	a.advanceSiteDeletion()
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
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
}

func (a *app) resumeTransfer(task *transferTask) {
	if a.transfers.quitting || a.siteDeletionPending(task.session.endpoint.session.ID) {
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
	task.state, task.errorText, task.failureNotice = transferQueued, locales.Text{}, false
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
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
	} else {
		task.cleanupPending = true
		task.state = transferFailed
	}
	a.advanceTransferConflicts()
	a.persistTransfers()
	a.mcpTransferChanged(task, nil)
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
			return locales.NewError(locales.MessageTheFilenameCannotBeSafelyUsedInTheDestinationPath)
		}
		root, err := buildTransferManifest(ctx, source, job.sourceRemote, sourcePath)
		if err != nil {
			return err
		}
		job.root, job.total, job.sourcePath, job.targetPath = root, root.total, sourcePath, targetPath
	}
	run.total.Store(job.total)
	run.bytes.Store(job.doneBytes)
	if run.journal != nil {
		if err := run.journal(); err != nil {
			return err
		}
	}
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
	if err := reconcileTransferCommits(ctx, target, job, job.root, run); err != nil {
		return err
	}
	if err := verifyCompletedTransfers(ctx, target, job.root); err != nil {
		return err
	}
	if err := a.copyTransferNode(ctx, task, run, job, source, target, job.root,
		transferJoin(!job.sourceRemote, job.targetPath, job.name), reserve); err != nil {
		return err
	}
	return cleanupTransferDirectories(ctx, target, job)
}

func buildTransferManifest(ctx context.Context, client fileclient.Client, remote bool, source string) (*transferNode, error) {
	return buildTransferManifestAt(ctx, client, remote, source, 0)
}

func buildTransferManifestAt(ctx context.Context, client fileclient.Client, remote bool, source string, depth int) (*transferNode, error) {
	if depth > 256 {
		return nil, locales.NewError(locales.MessageDirectoryNestingIsTooDeepTransferStopped)
	}
	entry, err := client.Stat(ctx, source)
	if err != nil {
		return nil, err
	}
	if entry.Symlink {
		return nil, locales.Errorf(locales.MessageSymbolicLinksAreNotFollowedRecursively, source)
	}
	if entry.ModeKnown && !entry.Directory && !entry.Mode.IsRegular() {
		return nil, locales.Errorf(locales.MessageTransferringSpecialFilesIsNotSupported, source)
	}
	if entry.Size < 0 {
		return nil, locales.Errorf(locales.MessageInvalidFileSize, source)
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
			return nil, locales.NewError(locales.MessageTheDirectoryResponseContainsInvalidOrDuplicateFilenames)
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
			return nil, locales.NewError(locales.MessageTheTotalTransferSizeExceedsTheSupportedRange)
		}
		node.total += nested.total
		node.children = append(node.children, nested)
	}
	return node, nil
}

func (a *app) copyTransferNode(ctx context.Context, task *transferTask, run *transferRun, job *transferJob,
	source, target fileclient.Client, node *transferNode, proposed string, reserve func(string, bool) error,
) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.done || node.skipped {
		return nil
	}
	if job.mcpFinish != nil {
		defer func() { result = errors.Join(result, job.mcpFinish(context.WithoutCancel(ctx), node, result)) }()
	}
	if job.mcpCheck != nil {
		if err := job.mcpCheck(ctx, source, target, node, proposed); err != nil {
			return err
		}
	}
	fresh, err := source.Stat(ctx, node.source)
	if err != nil {
		return err
	}
	if fresh.ModeKnown && !fresh.Directory && !fresh.Mode.IsRegular() && !fresh.Symlink {
		return locales.Errorf(locales.MessageTransferringSpecialFilesIsNotSupported, node.source)
	}
	if fresh.Symlink || fresh.Directory != node.entry.Directory || (!fresh.Directory && !sameTransferEntry(fresh, node.entry)) {
		return locales.Errorf(locales.MessageTheSourceFileChangedDuringTransfer, node.source)
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
	if job.mcpCheck != nil && node.destination != proposed {
		if err := job.mcpCheck(ctx, source, target, node, node.destination); err != nil {
			return err
		}
	}
	if node.entry.Directory {
		existing, err := target.Stat(ctx, node.destination)
		if errors.Is(err, fs.ErrNotExist) {
			err = target.Mkdir(ctx, node.destination)
		} else if err == nil && (!existing.Directory || existing.Symlink) {
			err = locales.Errorf(locales.MessageTheDestinationIsNotARegularDirectory, node.destination)
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
	var privateWriter fileclient.PrivateStageWriter
	privateDirectory := false
	if !job.sourceRemote && node.approved != nil {
		if !node.approved.ModeKnown {
			return locales.Errorf(
				locales.MessageUnableToVerifyTheExistingDestinationSPermissionsOverwritingBlocked,
				node.destination,
			)
		}
		if node.approved.Mode.Perm()&0044 != 0044 {
			var supported bool
			privateWriter, supported = target.(fileclient.PrivateStageWriter)
			if !supported {
				_, privateDirectory = target.(fileclient.PrivateDirectoryMaker)
				if !privateDirectory {
					return locales.Errorf(
						locales.MessageThisConnectionCannotSafelyCreateAPrivateTemporaryFileBeforeUploadingOverwritingThe,
						node.destination,
					)
				}
			}
		}
	}
	if node.stageDirectory != "" {
		entry, err := target.Stat(ctx, node.stageDirectory)
		if err != nil {
			return err
		}
		if !entry.Directory || entry.Symlink || !entry.ModeKnown || entry.Mode.Perm() != 0700 {
			return locales.NewError(locales.MessagePrivateStagingDirectoryPermissionsChangedUploadWillNotContinue)
		}
		if node.temporary == "" {
			node.temporary = transferJoin(!job.sourceRemote, node.stageDirectory, "content")
			job.temporary[node.temporary] = struct{}{}
		}
	}
	if privateDirectory && node.temporary != "" && node.stageDirectory == "" {
		return locales.NewError(
			locales.MessageThePreviousTemporaryFileIsOutsideThePrivateDirectoryTheRestrictedDestinationCannot,
		)
	}
	if privateWriter != nil && node.temporary != "" {
		// Old parts may still have the server's public creation permissions.
		// Restrict them before prefix verification or another source read.
		output, _, err := privateWriter.OpenPrivateWrite(ctx, node.temporary, 0, false, false)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.temporary, node.temporary)
			clearTransferTemporary(node, node.temporary)
		} else {
			if err != nil {
				return err
			}
			mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
			for _, baseline := range []*fileclient.Entry{node.staged, node.partial} {
				if baseline != nil {
					baseline.Mode = baseline.Mode&^mask | 0600
					baseline.ModeKnown = true
				}
			}
			if err := output.Close(); err != nil {
				return err
			}
		}
	}
	if node.temporary == "" {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return err
		}
		name := ".burrow-" + hex.EncodeToString(suffix[:]) + ".part"
		temporary := transferJoin(!job.sourceRemote, transferDir(!job.sourceRemote, node.destination), name)
		if temporary == node.destination {
			return locales.NewError(locales.MessageTheTemporaryFilenameIsTheSameAsTheDestination)
		}
		if _, err := target.Stat(ctx, temporary); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return locales.NewError(locales.MessageTheTemporaryFilenameIsAlreadyInUse)
			}
			return err
		}
		if privateWriter != nil {
			output, created, err := privateWriter.OpenPrivateWrite(ctx, temporary, 0, false, true)
			if created {
				node.temporary = temporary
				job.temporary[temporary] = struct{}{}
			}
			if err != nil {
				return err
			}
			if err := output.Close(); err != nil {
				return err
			}
		} else if privateDirectory {
			created, err := target.(fileclient.PrivateDirectoryMaker).MkdirPrivate(ctx, temporary)
			if created {
				node.stageDirectory = temporary
				job.temporary[temporary] = struct{}{}
			}
			if err != nil {
				return err
			}
			node.temporary = transferJoin(!job.sourceRemote, temporary, "content")
			job.temporary[node.temporary] = struct{}{}
		} else {
			node.temporary = temporary
			job.temporary[temporary] = struct{}{}
		}
	}
	if run.journal != nil {
		if err := run.journal(); err != nil {
			return err
		}
	}
	resume, err := prepareTransferResume(ctx, target, node)
	if err != nil {
		return err
	}
	offset := resume.offset
	run.bytes.Store(job.doneBytes + offset)
	if offset < node.entry.Size || !resume.exists {
		input, err := source.OpenRead(ctx, node.source, offset)
		if err != nil {
			return err
		}
		node.partial = nil
		var output io.WriteCloser
		if privateWriter != nil {
			output, _, err = privateWriter.OpenPrivateWrite(ctx, node.temporary, offset, offset == 0, false)
		} else {
			output, err = target.OpenWrite(ctx, node.temporary, offset, offset == 0)
		}
		if err != nil {
			return errors.Join(err, input.Close())
		}
		writer := transferCountingWriter{
			writer: output, bytes: &run.bytes, written: &run.written,
			hash: resume.hash, offset: offset, node: node,
			journal: run.journal, lastJournal: time.Now(), journalOffset: offset,
		}
		reader := transferSizedReader(input, node.entry.Size-offset)
		copied, copyErr := io.CopyBuffer(&writer, reader, make([]byte, 64<<10))
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if copied != node.entry.Size-offset {
			return locales.Errorf(locales.MessageTheSourceFileSizeChangedDuringTransfer, node.source)
		}
	}
	digest := hex.EncodeToString(resume.hash.Sum(nil))
	fresh, err = source.Stat(ctx, node.source)
	if err != nil {
		return err
	}
	if !sameTransferEntry(fresh, node.entry) || node.digest != "" && node.digest != digest {
		return locales.Errorf(locales.MessageTheSourceFileChangedDuringTransfer, node.source)
	}
	written, err := target.Stat(ctx, node.temporary)
	if err != nil {
		return err
	}
	if written.Symlink || written.Directory || written.Size != node.entry.Size ||
		written.ModeKnown && !written.Mode.IsRegular() {
		return locales.Errorf(locales.MessageTheDestinationTemporaryFileSizeDoesNotMatch, node.temporary)
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
		if job.mcpCheck != nil {
			if err := job.mcpCheck(ctx, source, target, node, node.destination); err != nil {
				return err
			}
		}
		if err := preserveTransferPermissions(ctx, target, node, current, err == nil); err != nil {
			return err
		}
		node.committing = true
		if run.journal != nil {
			if err := run.journal(); err != nil {
				return err
			}
		}
		noReplace := node.approved == nil
		err = target.Rename(ctx, node.temporary, node.destination, fileclient.RenameOptions{NoReplace: noReplace})
		if err == nil {
			markTransferCommitted(job, node, run)
			if run.journal != nil {
				return run.journal()
			}
			return nil
		}
		if noReplace && errors.Is(err, fs.ErrExist) {
			node.committing = false
			continue // The exclusive commit lost a race; ask about the real target.
		}
		if !uncertainTransferCommit(ctx, target, err) {
			node.committing = false
			return locales.Errorf(locales.MessageUnableToCommitTransferredFile, err)
		}
		return locales.Errorf(locales.MessageTheCommitResultIsUncertainRetryingWillVerifyTheStagedFileAndDestination, err)
	}
}

type transferResume struct {
	offset int64
	hash   hash.Hash
	exists bool
}

func prepareTransferResume(
	ctx context.Context,
	target fileclient.Client,
	node *transferNode,
) (resume transferResume, err error) {
	resume.hash = sha256.New()
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
	if partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.entry.Size ||
		partial.ModeKnown && !partial.Mode.IsRegular() {
		return resume, locales.Errorf(locales.MessageInvalidTemporaryFileState, node.temporary)
	}
	if checkpoint := node.checkpoint; checkpoint != nil && checkpoint.Offset > partial.Size {
		// A failed network write may have acknowledged more bytes than the server
		// retained. None of that prefix is trusted; rewrite this owned part fully.
		node.staged, node.partial, node.digest = nil, nil, ""
		node.checkpoint = &transferCheckpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
		return resume, nil
	}
	if node.staged != nil && !sameTransferEntry(partial, *node.staged) {
		return resume, locales.Errorf(locales.MessageTheVerifiedTemporaryFileHasChanged, node.temporary)
	}
	if node.partial != nil && !sameTransferEntry(partial, *node.partial) {
		return resume, locales.Errorf(locales.MessageTheRecordedPartialFileHasChanged, node.temporary)
	}
	checkpoint := node.checkpoint
	if checkpoint == nil {
		if partial.Size != 0 {
			return resume, locales.NewError(locales.MessageTheStagedFileHasNoVerifiableCheckpoint)
		}
		checkpoint = &transferCheckpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
	}

	if checkpoint.Offset > 0 {
		resume.hash, err = verifiedTransferPrefix(ctx, target, node.temporary, partial, checkpoint)
		if err != nil {
			return resume, err
		}
	} else if checkpoint.Digest != hex.EncodeToString(resume.hash.Sum(nil)) {
		return resume, locales.NewError(locales.MessageInvalidTransferPrefixCheckpoint)
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
		return nil, locales.Errorf(locales.MessageUnableToVerifyTheTransferredPortionItWasNotReused, name, err)
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
		return nil, locales.Errorf(locales.MessageTheTransferredContentHasChangedItWasNotReused, name)
	}
	fresh, err := target.Stat(ctx, name)
	if err != nil {
		return nil, err
	}
	if !sameTransferEntry(fresh, partial) {
		return nil, locales.Errorf(locales.MessageThePartialFileChangedDuringVerification, name)
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
	if preserver, ok := target.(fileclient.PermissionPreserver); ok {
		if err := preserver.PreservePermissions(ctx, node.destination, node.temporary); err != nil {
			return locales.Errorf(locales.MessageUnableToPreserveDestinationPermissionsFileNotCommitted, node.destination, err)
		}
		staged, err := target.Stat(ctx, node.temporary)
		if err != nil {
			return err
		}
		if staged.Symlink || staged.Directory || staged.Size != node.entry.Size {
			return locales.Errorf(locales.MessageTheStagedFileHasChangedFileNotCommitted, node.temporary)
		}
		node.staged, node.partial = &staged, &staged
		return nil
	}
	if !current.ModeKnown {
		return locales.Errorf(
			locales.MessageUnableToVerifyTheExistingDestinationSPermissionsOverwritingBlocked,
			node.destination,
		)
	}
	if !current.Mode.IsRegular() {
		return locales.Errorf(locales.MessageTheDestinationIsNotARegularFileOverwritingBlocked, node.destination)
	}
	mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
	mode := current.Mode & mask
	setter, ok := target.(fileclient.PermissionSetter)
	if !ok {
		return locales.Errorf(
			locales.MessageTheDestinationDoesNotSupportPreservingFilePermissionsFileNotCommitted,
			node.destination,
		)
	}
	if err := setter.Chmod(ctx, node.temporary, mode); err != nil {
		return locales.Errorf(locales.MessageUnableToPreserveDestinationPermissionsFileNotCommitted, node.destination, err)
	}
	staged, err := target.Stat(ctx, node.temporary)
	if err != nil {
		return err
	}
	if staged.Symlink || staged.Directory || staged.Size != node.entry.Size ||
		!staged.ModeKnown || staged.Mode&mask != mode {
		return locales.Errorf(locales.MessageUnableToVerifyStagedFilePermissionsFileNotCommitted, node.temporary)
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
		return "", locales.Errorf(locales.MessageTheVerificationFileSizeHasChanged, name)
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
	node.temporary, node.done, node.committing = "", true, false
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
	if node.stageDirectory != "" {
		if err := target.Remove(ctx, node.stageDirectory, true); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		delete(job.temporary, node.stageDirectory)
		node.stageDirectory = ""
	}
	node.temporary, node.digest, node.staged = "", "", nil
	node.partial, node.checkpoint = nil, nil
	node.committing = false
	return nil
}

func verifyCompletedTransfers(ctx context.Context, target fileclient.Client, node *transferNode) error {
	if node.skipped {
		return nil
	}
	if node.done {
		entry, err := target.Stat(ctx, node.destination)
		if err != nil {
			return locales.Errorf(locales.MessageTheCompletedDestinationHasChangedQueueItAgain, node.destination, err)
		}
		if entry.Symlink || entry.Directory != node.entry.Directory ||
			!entry.Directory && (entry.Size != node.entry.Size || entry.ModeKnown && !entry.Mode.IsRegular()) {
			return locales.Errorf(locales.MessageTheCompletedDestinationTypeOrSizeHasChangedQueueItAgain, node.destination)
		}
	}
	for _, child := range node.children {
		if err := verifyCompletedTransfers(ctx, target, child); err != nil {
			return err
		}
	}
	return nil
}

func reconcileTransferCommits(ctx context.Context, target fileclient.Client, job *transferJob, node *transferNode, run *transferRun) error {
	if node.committing {
		if err := recoverTransferCommit(ctx, target, job, node, run); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := reconcileTransferCommits(ctx, target, job, child, run); err != nil {
			return err
		}
	}
	return nil
}

func recoverTransferCommit(ctx context.Context, target fileclient.Client, job *transferJob, node *transferNode, run *transferRun) error {
	uncertain := func(err error) error {
		return locales.Errorf(
			locales.MessageTheCommitResultIsStillUncertainVerifyItThenRetryOrCancelAnd,
			node.destination,
			err,
		)
	}
	partial, partialErr := target.Stat(ctx, node.temporary)
	if partialErr != nil && !errors.Is(partialErr, fs.ErrNotExist) {
		return uncertain(partialErr)
	}
	partialExists := partialErr == nil
	if partialExists && (partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.entry.Size) {
		return uncertain(locales.NewError(locales.MessageTheStagedFileTypeOrSizeHasChanged))
	}
	if partialExists && node.staged != nil && !sameTransferEntry(partial, *node.staged) {
		return uncertain(locales.NewError(locales.MessageTheVerifiedStagedFileHasChanged))
	}
	entry, targetErr := target.Stat(ctx, node.destination)
	if errors.Is(targetErr, fs.ErrNotExist) {
		if partialExists {
			node.committing = false
			return nil // The owned bytes remain staged; no final target exists.
		}
		return uncertain(locales.NewError(locales.MessageNeitherTheStagedFileNorTheDestinationExists))
	}
	if targetErr != nil {
		return uncertain(targetErr)
	}
	if entry.Symlink || entry.Directory || entry.Size != node.entry.Size ||
		entry.ModeKnown && !entry.Mode.IsRegular() {
		if partialExists {
			node.committing = false
			return nil
		}
		return uncertain(locales.NewError(locales.MessageTheDestinationTypeOrSizeDoesNotMatch))
	}
	if node.digest == "" {
		return uncertain(locales.NewError(locales.MessageTheCommitCheckpointIsMissingItsDigest))
	}
	digest, err := hashTransferFile(ctx, target, node.destination, node.entry.Size)
	if err != nil {
		return uncertain(locales.Errorf(locales.MessageUnableToReadBackTheDestinationForVerification, err))
	}
	fresh, err := target.Stat(ctx, node.destination)
	if err != nil {
		return uncertain(err)
	}
	if !sameTransferEntry(fresh, entry) {
		return uncertain(locales.NewError(locales.MessageTheDestinationChangedDuringVerification))
	}
	if digest != node.digest {
		if partialExists {
			node.committing = false
			return nil
		}
		return uncertain(locales.NewError(locales.MessageTheDestinationContentsDoNotMatch))
	}
	if partialExists {
		if err := target.Remove(ctx, node.temporary, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return uncertain(locales.Errorf(locales.MessageDestinationContentsVerifiedButTheStagedCopyCouldNotBeCleanedUp, err))
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
	journal       func() error
	lastJournal   time.Time
	journalOffset int64
	writer        io.Writer
	bytes         *atomic.Int64
	written       *atomic.Int64
	hash          hash.Hash
	offset        int64
	node          *transferNode
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
	if n > 0 && err == nil && w.journal != nil && (time.Since(w.lastJournal) >= time.Second || w.offset-w.journalOffset >= 4<<20) {
		if saveErr := w.journal(); saveErr != nil {
			return n, saveErr
		}
		w.lastJournal, w.journalOffset = time.Now(), w.offset
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
		if choice == 0 && job.mcpFailConflict {
			return "", nil, false, fs.ErrExist
		}
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
			if existing.Symlink || existing.Directory != source.Directory ||
				!existing.Directory && existing.ModeKnown && !existing.Mode.IsRegular() {
				return "", nil, false, locales.Errorf(
					locales.MessageTheDestinationHasADifferentTypeOrIsASymbolicLinkChooseKeep,
					candidate,
				)
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
	modeMatches := !left.ModeKnown || !right.ModeKnown ||
		left.Mode.Perm() == right.Mode.Perm() && left.Mode.Type() == right.Mode.Type()
	return left.Size == right.Size && left.Directory == right.Directory && left.Symlink == right.Symlink &&
		left.Modified.Equal(right.Modified) && modeMatches
}

func transferSafeName(name string) bool {
	return localfs.ValidName(name)
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
	names := make([]string, 0, len(job.temporary))
	for name := range job.temporary {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int { return len(b) - len(a) })
	for _, name := range names {
		entry, err := target.Stat(ctx, name)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.temporary, name)
			clearTransferTemporary(job.root, name)
			continue
		}
		privateDirectory := transferPrivateDirectory(job.root, name)
		if err == nil && (entry.Symlink || entry.Directory != privateDirectory) {
			err = locales.NewError(locales.MessageTheTemporaryFileWasReplacedDeletionBlocked)
		}
		if err == nil {
			err = target.Remove(ctx, name, privateDirectory)
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
	if node.stageDirectory == name {
		node.stageDirectory = ""
	}
	if node.temporary == name {
		node.temporary = ""
		node.staged, node.digest = nil, ""
		node.partial, node.checkpoint = nil, nil
		node.committing = false
	}
	for _, child := range node.children {
		clearTransferTemporary(child, name)
	}
}

func transferPrivateDirectory(node *transferNode, name string) bool {
	if node == nil {
		return false
	}
	if node.stageDirectory == name {
		return true
	}
	for _, child := range node.children {
		if transferPrivateDirectory(child, name) {
			return true
		}
	}
	return false
}

func cleanupTransferDirectories(ctx context.Context, target fileclient.Client, job *transferJob) error {
	for name := range job.temporary {
		if !transferPrivateDirectory(job.root, name) {
			continue
		}
		entry, err := target.Stat(ctx, name)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.temporary, name)
			clearTransferTemporary(job.root, name)
			continue
		}
		if err != nil {
			return err
		}
		if !entry.Directory || entry.Symlink {
			return locales.NewError(locales.MessageThePrivateStagingDirectoryWasReplacedCleanupBlocked)
		}
		if err := target.Remove(ctx, name, true); err != nil {
			return err
		}
		delete(job.temporary, name)
		clearTransferTemporary(job.root, name)
	}
	return nil
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
