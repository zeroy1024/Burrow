package transfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/fs"
	"math"
	"sync/atomic"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
)

func (job *Job) Execute(ctx context.Context, run *Run,
	source, target fileclient.Client, locks *Locks,
) error {
	if job.Root == nil {
		sourcePath, err := transferAbsolute(ctx, source, job.SourceRemote, job.SourcePath)
		if err != nil {
			return err
		}
		targetPath, err := transferAbsolute(ctx, target, !job.SourceRemote, job.TargetPath)
		if err != nil {
			return err
		}
		if !SafeName(job.Name) {
			return locales.NewError(locales.MessageTheFilenameCannotBeSafelyUsedInTheDestinationPath)
		}
		root, err := buildTransferManifest(ctx, source, job.SourceRemote, sourcePath)
		if err != nil {
			return err
		}
		job.Root, job.Total, job.SourcePath, job.TargetPath = root, root.Total, sourcePath, targetPath
	}
	run.Total.Store(job.Total)
	run.Bytes.Store(job.DoneBytes)
	if run.Journal != nil {
		if err := run.Journal(); err != nil {
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
		release, err := locks.acquire(ctx, job.DestinationKey, name, directory, job)
		if err == nil {
			releases = append(releases, release)
		}
		return err
	}
	if job.Root.Destination != "" {
		if err := reserve(job.Root.Destination, job.Root.Entry.Directory); err != nil {
			return err
		}
	}
	if err := reconcileTransferCommits(ctx, target, job, job.Root, run); err != nil {
		return err
	}
	if err := verifyCompletedTransfers(ctx, target, job.Root); err != nil {
		return err
	}
	if err := job.copyNode(ctx, run, source, target, job.Root,
		Join(!job.SourceRemote, job.TargetPath, job.Name), reserve); err != nil {
		return err
	}
	return cleanupTransferDirectories(ctx, target, job)
}

func buildTransferManifest(ctx context.Context, client fileclient.Client, remote bool, source string) (*Node, error) {
	return buildTransferManifestAt(ctx, client, remote, source, 0)
}

func buildTransferManifestAt(ctx context.Context, client fileclient.Client, remote bool, source string, depth int) (*Node, error) {
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
	node := &Node{Source: source, Entry: entry}
	if !entry.Directory {
		node.Total = entry.Size
		return node, nil
	}
	entries, err := client.ReadDir(ctx, source)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, child := range entries {
		if !SafeName(child.Name) || seen[child.Name] {
			return nil, locales.NewError(locales.MessageTheDirectoryResponseContainsInvalidOrDuplicateFilenames)
		}
		seen[child.Name] = true
		nested, err := buildTransferManifestAt(ctx, client, remote, Join(remote, source, child.Name), depth+1)
		if err != nil {
			return nil, err
		}
		// The checked directory component determines the destination name; a
		// protocol Stat response must not replace it with another server path.
		nested.Entry.Name = child.Name
		if nested.Total > math.MaxInt64-node.Total {
			return nil, locales.NewError(locales.MessageTheTotalTransferSizeExceedsTheSupportedRange)
		}
		node.Total += nested.Total
		node.Children = append(node.Children, nested)
	}
	return node, nil
}

func (job *Job) copyNode(ctx context.Context, run *Run,
	source, target fileclient.Client, node *Node, proposed string, reserve func(string, bool) error,
) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.Done || node.Skipped {
		return nil
	}
	if job.MCPFinish != nil {
		defer func() { result = errors.Join(result, job.MCPFinish(context.WithoutCancel(ctx), node, result)) }()
	}
	if job.MCPCheck != nil {
		if err := job.MCPCheck(ctx, source, target, node, proposed); err != nil {
			return err
		}
	}
	fresh, err := source.Stat(ctx, node.Source)
	if err != nil {
		return err
	}
	if fresh.ModeKnown && !fresh.Directory && !fresh.Mode.IsRegular() && !fresh.Symlink {
		return locales.Errorf(locales.MessageTransferringSpecialFilesIsNotSupported, node.Source)
	}
	if fresh.Symlink || fresh.Directory != node.Entry.Directory || (!fresh.Directory && !sameTransferEntry(fresh, node.Entry)) {
		return locales.Errorf(locales.MessageTheSourceFileChangedDuringTransfer, node.Source)
	}
	if node.Destination == "" {
		destination, approved, skip, err := job.chooseDestination(ctx, run, target,
			proposed, node.Entry, reserve)
		if err != nil {
			return err
		}
		if skip {
			node.Skipped = true
			job.Total -= node.Total
			run.Total.Store(job.Total)
			return nil
		}
		node.Destination, node.Approved = destination, approved
		if node == job.Root && run.DestinationChanged != nil {
			run.DestinationChanged(job.SourceRemote, destination)
		}
	} else if err := reserve(node.Destination, node.Entry.Directory); err != nil {
		return err
	}
	if job.MCPCheck != nil && node.Destination != proposed {
		if err := job.MCPCheck(ctx, source, target, node, node.Destination); err != nil {
			return err
		}
	}
	if node.Entry.Directory {
		existing, err := target.Stat(ctx, node.Destination)
		if errors.Is(err, fs.ErrNotExist) {
			err = target.Mkdir(ctx, node.Destination)
		} else if err == nil && (!existing.Directory || existing.Symlink) {
			err = locales.Errorf(locales.MessageTheDestinationIsNotARegularDirectory, node.Destination)
		}
		if err != nil {
			return err
		}
		for _, child := range node.Children {
			if err := job.copyNode(ctx, run, source, target, child,
				Join(!job.SourceRemote, node.Destination, child.Entry.Name), reserve); err != nil {
				return err
			}
		}
		node.Done = true
		return nil
	}
	var privateWriter fileclient.PrivateStageWriter
	privateDirectory := false
	if !job.SourceRemote && node.Approved != nil {
		if !node.Approved.ModeKnown {
			return locales.Errorf(
				locales.MessageUnableToVerifyTheExistingDestinationSPermissionsOverwritingBlocked,
				node.Destination,
			)
		}
		if node.Approved.Mode.Perm()&0044 != 0044 {
			var supported bool
			privateWriter, supported = target.(fileclient.PrivateStageWriter)
			if !supported {
				_, privateDirectory = target.(fileclient.PrivateDirectoryMaker)
				if !privateDirectory {
					return locales.Errorf(
						locales.MessageThisConnectionCannotSafelyCreateAPrivateTemporaryFileBeforeUploadingOverwritingThe,
						node.Destination,
					)
				}
			}
		}
	}
	if node.StageDirectory != "" {
		entry, err := target.Stat(ctx, node.StageDirectory)
		if err != nil {
			return err
		}
		if !entry.Directory || entry.Symlink || !entry.ModeKnown || entry.Mode.Perm() != 0700 {
			return locales.NewError(locales.MessagePrivateStagingDirectoryPermissionsChangedUploadWillNotContinue)
		}
		if node.Temporary == "" {
			node.Temporary = Join(!job.SourceRemote, node.StageDirectory, "content")
			job.Temporary[node.Temporary] = struct{}{}
		}
	}
	if privateDirectory && node.Temporary != "" && node.StageDirectory == "" {
		return locales.NewError(
			locales.MessageThePreviousTemporaryFileIsOutsideThePrivateDirectoryTheRestrictedDestinationCannot,
		)
	}
	if privateWriter != nil && node.Temporary != "" {
		// Old parts may still have the server's public creation permissions.
		// Restrict them before prefix verification or another source read.
		output, _, err := privateWriter.OpenPrivateWrite(ctx, node.Temporary, 0, false, false)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.Temporary, node.Temporary)
			clearTransferTemporary(node, node.Temporary)
		} else {
			if err != nil {
				return err
			}
			mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
			for _, baseline := range []*fileclient.Entry{node.Staged, node.Partial} {
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
	if node.Temporary == "" {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return err
		}
		name := ".burrow-" + hex.EncodeToString(suffix[:]) + ".part"
		temporary := Join(!job.SourceRemote, Dir(!job.SourceRemote, node.Destination), name)
		if temporary == node.Destination {
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
				node.Temporary = temporary
				job.Temporary[temporary] = struct{}{}
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
				node.StageDirectory = temporary
				job.Temporary[temporary] = struct{}{}
			}
			if err != nil {
				return err
			}
			node.Temporary = Join(!job.SourceRemote, temporary, "content")
			job.Temporary[node.Temporary] = struct{}{}
		} else {
			node.Temporary = temporary
			job.Temporary[temporary] = struct{}{}
		}
	}
	if run.Journal != nil {
		if err := run.Journal(); err != nil {
			return err
		}
	}
	resume, err := prepareTransferResume(ctx, target, node)
	if err != nil {
		return err
	}
	offset := resume.offset
	run.Bytes.Store(job.DoneBytes + offset)
	if offset < node.Entry.Size || !resume.exists {
		input, err := source.OpenRead(ctx, node.Source, offset)
		if err != nil {
			return err
		}
		node.Partial = nil
		var output io.WriteCloser
		if privateWriter != nil {
			output, _, err = privateWriter.OpenPrivateWrite(ctx, node.Temporary, offset, offset == 0, false)
		} else {
			output, err = target.OpenWrite(ctx, node.Temporary, offset, offset == 0)
		}
		if err != nil {
			return errors.Join(err, input.Close())
		}
		writer := transferCountingWriter{
			writer: output, bytes: &run.Bytes, written: &run.Written,
			hash: resume.hash, offset: offset, node: node,
			journal: run.Journal, lastJournal: time.Now(), journalOffset: offset,
		}
		reader := transferSizedReader(input, node.Entry.Size-offset)
		copied, copyErr := io.CopyBuffer(&writer, reader, make([]byte, 64<<10))
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if copied != node.Entry.Size-offset {
			return locales.Errorf(locales.MessageTheSourceFileSizeChangedDuringTransfer, node.Source)
		}
	}
	digest := hex.EncodeToString(resume.hash.Sum(nil))
	fresh, err = source.Stat(ctx, node.Source)
	if err != nil {
		return err
	}
	if !sameTransferEntry(fresh, node.Entry) || node.Digest != "" && node.Digest != digest {
		return locales.Errorf(locales.MessageTheSourceFileChangedDuringTransfer, node.Source)
	}
	written, err := target.Stat(ctx, node.Temporary)
	if err != nil {
		return err
	}
	if written.Symlink || written.Directory || written.Size != node.Entry.Size ||
		written.ModeKnown && !written.Mode.IsRegular() {
		return locales.Errorf(locales.MessageTheDestinationTemporaryFileSizeDoesNotMatch, node.Temporary)
	}
	node.Staged, node.Partial, node.Digest = &written, &written, digest
	for {
		current, err := target.Stat(ctx, node.Destination)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if errors.Is(err, fs.ErrNotExist) {
			// Approval belonged to the previous inode. A later arrival must win
			// an exclusive commit and get its own conflict/permission decision.
			node.Approved = nil
		}
		if err == nil && (node.Approved == nil || !sameTransferEntry(current, *node.Approved)) {
			destination, approved, skip, err := job.chooseDestination(ctx, run, target,
				node.Destination, node.Entry, reserve)
			if err != nil {
				return err
			}
			if skip {
				if err := discardTransferStage(ctx, target, job, node); err != nil {
					return err
				}
				node.Skipped = true
				job.Total -= node.Total
				run.Total.Store(job.Total)
				run.Bytes.Store(job.DoneBytes)
				return nil
			}
			node.Destination, node.Approved = destination, approved
			if node == job.Root && run.DestinationChanged != nil {
				run.DestinationChanged(job.SourceRemote, destination)
			}
			continue // Refresh metadata if conflict handling selected another name.
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if job.MCPCheck != nil {
			if err := job.MCPCheck(ctx, source, target, node, node.Destination); err != nil {
				return err
			}
		}
		if err := preserveTransferPermissions(ctx, target, node, current, err == nil); err != nil {
			return err
		}
		node.Committing = true
		if run.Journal != nil {
			if err := run.Journal(); err != nil {
				return err
			}
		}
		noReplace := node.Approved == nil
		err = target.Rename(ctx, node.Temporary, node.Destination, fileclient.RenameOptions{NoReplace: noReplace})
		if err == nil {
			markTransferCommitted(job, node, run)
			if run.Journal != nil {
				return run.Journal()
			}
			return nil
		}
		if noReplace && errors.Is(err, fs.ErrExist) {
			node.Committing = false
			continue // The exclusive commit lost a race; ask about the real target.
		}
		if !uncertainTransferCommit(ctx, target, err) {
			node.Committing = false
			return locales.Errorf(locales.MessageUnableToCommitTransferredFile, err)
		}
		return locales.Errorf(locales.MessageTheCommitResultIsUncertainRetryingWillVerifyTheStagedFileAndDestination, err)
	}
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
	node          *Node
}

func (w *transferCountingWriter) Write(buffer []byte) (int, error) {
	n, err := w.writer.Write(buffer)
	w.bytes.Add(int64(n))
	w.written.Add(int64(n))
	if n > 0 {
		w.hash.Write(buffer[:n])
		w.offset += int64(n)
		w.node.Partial = nil
		if w.offset <= w.node.Entry.Size {
			w.node.Checkpoint = &Checkpoint{Offset: w.offset, Digest: hex.EncodeToString(w.hash.Sum(nil))}
		} else {
			// The extra EOF-check byte detects a growing source. Do not persist
			// an acknowledged offset outside the immutable manifest's bounds.
			w.node.Checkpoint = nil
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
