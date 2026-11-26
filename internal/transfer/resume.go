package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/fs"
	"math"
	"net"
	"net/textproto"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
)

type transferResume struct {
	offset int64
	hash   hash.Hash
	exists bool
}

func prepareTransferResume(
	ctx context.Context,
	target fileclient.Client,
	node *Node,
) (resume transferResume, err error) {
	resume.hash = sha256.New()
	partial, err := target.Stat(ctx, node.Temporary)
	if errors.Is(err, fs.ErrNotExist) {
		node.Staged, node.Partial, node.Checkpoint, node.Digest = nil, nil, nil, ""
		node.Checkpoint = &Checkpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
		return resume, nil
	}
	if err != nil {
		return resume, err
	}
	resume.exists = true
	if partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.Entry.Size ||
		partial.ModeKnown && !partial.Mode.IsRegular() {
		return resume, locales.Errorf(locales.MessageInvalidTemporaryFileState, node.Temporary)
	}
	if checkpoint := node.Checkpoint; checkpoint != nil && checkpoint.Offset > partial.Size {
		// A failed network write may have acknowledged more bytes than the server
		// retained. None of that prefix is trusted; rewrite this owned part fully.
		node.Staged, node.Partial, node.Digest = nil, nil, ""
		node.Checkpoint = &Checkpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
		return resume, nil
	}
	if node.Staged != nil && !sameTransferEntry(partial, *node.Staged) {
		return resume, locales.Errorf(locales.MessageTheVerifiedTemporaryFileHasChanged, node.Temporary)
	}
	if node.Partial != nil && !sameTransferEntry(partial, *node.Partial) {
		return resume, locales.Errorf(locales.MessageTheRecordedPartialFileHasChanged, node.Temporary)
	}
	checkpoint := node.Checkpoint
	if checkpoint == nil {
		if partial.Size != 0 {
			return resume, locales.NewError(locales.MessageTheStagedFileHasNoVerifiableCheckpoint)
		}
		checkpoint = &Checkpoint{Digest: hex.EncodeToString(resume.hash.Sum(nil))}
	}

	if checkpoint.Offset > 0 {
		resume.hash, err = verifiedTransferPrefix(ctx, target, node.Temporary, partial, checkpoint)
		if err != nil {
			return resume, err
		}
	} else if checkpoint.Digest != hex.EncodeToString(resume.hash.Sum(nil)) {
		return resume, locales.NewError(locales.MessageInvalidTransferPrefixCheckpoint)
	}
	resume.offset = checkpoint.Offset
	node.Checkpoint, node.Partial = checkpoint, &partial
	return resume, nil
}

func verifiedTransferPrefix(
	ctx context.Context,
	target fileclient.Client,
	name string,
	partial fileclient.Entry,
	checkpoint *Checkpoint,
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
	node *Node,
	current fileclient.Entry,
	exists bool,
) error {
	if !exists {
		// A new target keeps its client's creation policy. Requiring chmod here
		// would reject otherwise valid uploads to FTP servers without SITE CHMOD.
		return nil
	}
	if preserver, ok := target.(fileclient.PermissionPreserver); ok {
		if err := preserver.PreservePermissions(ctx, node.Destination, node.Temporary); err != nil {
			return locales.Errorf(locales.MessageUnableToPreserveDestinationPermissionsFileNotCommitted, node.Destination, err)
		}
		staged, err := target.Stat(ctx, node.Temporary)
		if err != nil {
			return err
		}
		if staged.Symlink || staged.Directory || staged.Size != node.Entry.Size {
			return locales.Errorf(locales.MessageTheStagedFileHasChangedFileNotCommitted, node.Temporary)
		}
		node.Staged, node.Partial = &staged, &staged
		return nil
	}
	if !current.ModeKnown {
		return locales.Errorf(
			locales.MessageUnableToVerifyTheExistingDestinationSPermissionsOverwritingBlocked,
			node.Destination,
		)
	}
	if !current.Mode.IsRegular() {
		return locales.Errorf(locales.MessageTheDestinationIsNotARegularFileOverwritingBlocked, node.Destination)
	}
	mask := fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky
	mode := current.Mode & mask
	setter, ok := target.(fileclient.PermissionSetter)
	if !ok {
		return locales.Errorf(
			locales.MessageTheDestinationDoesNotSupportPreservingFilePermissionsFileNotCommitted,
			node.Destination,
		)
	}
	if err := setter.Chmod(ctx, node.Temporary, mode); err != nil {
		return locales.Errorf(locales.MessageUnableToPreserveDestinationPermissionsFileNotCommitted, node.Destination, err)
	}
	staged, err := target.Stat(ctx, node.Temporary)
	if err != nil {
		return err
	}
	if staged.Symlink || staged.Directory || staged.Size != node.Entry.Size ||
		!staged.ModeKnown || staged.Mode&mask != mode {
		return locales.Errorf(locales.MessageUnableToVerifyStagedFilePermissionsFileNotCommitted, node.Temporary)
	}
	node.Staged, node.Partial = &staged, &staged
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

func markTransferCommitted(job *Job, node *Node, run *Run) {
	delete(job.Temporary, node.Temporary)
	node.Temporary, node.Done, node.Committing = "", true, false
	node.Partial, node.Checkpoint = nil, nil
	job.DoneBytes += node.Entry.Size
	run.Bytes.Store(job.DoneBytes)
}

func discardTransferStage(ctx context.Context, target fileclient.Client, job *Job, node *Node) error {
	if node.Temporary != "" {
		if err := target.Remove(ctx, node.Temporary, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		delete(job.Temporary, node.Temporary)
	}
	if node.StageDirectory != "" {
		if err := target.Remove(ctx, node.StageDirectory, true); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		delete(job.Temporary, node.StageDirectory)
		node.StageDirectory = ""
	}
	node.Temporary, node.Digest, node.Staged = "", "", nil
	node.Partial, node.Checkpoint = nil, nil
	node.Committing = false
	return nil
}

func verifyCompletedTransfers(ctx context.Context, target fileclient.Client, node *Node) error {
	if node.Skipped {
		return nil
	}
	if node.Done {
		entry, err := target.Stat(ctx, node.Destination)
		if err != nil {
			return locales.Errorf(locales.MessageTheCompletedDestinationHasChangedQueueItAgain, node.Destination, err)
		}
		if entry.Symlink || entry.Directory != node.Entry.Directory ||
			!entry.Directory && (entry.Size != node.Entry.Size || entry.ModeKnown && !entry.Mode.IsRegular()) {
			return locales.Errorf(locales.MessageTheCompletedDestinationTypeOrSizeHasChangedQueueItAgain, node.Destination)
		}
	}
	for _, child := range node.Children {
		if err := verifyCompletedTransfers(ctx, target, child); err != nil {
			return err
		}
	}
	return nil
}

func reconcileTransferCommits(ctx context.Context, target fileclient.Client, job *Job, node *Node, run *Run) error {
	if node.Committing {
		if err := recoverTransferCommit(ctx, target, job, node, run); err != nil {
			return err
		}
	}
	for _, child := range node.Children {
		if err := reconcileTransferCommits(ctx, target, job, child, run); err != nil {
			return err
		}
	}
	return nil
}

func recoverTransferCommit(ctx context.Context, target fileclient.Client, job *Job, node *Node, run *Run) error {
	uncertain := func(err error) error {
		return locales.Errorf(
			locales.MessageTheCommitResultIsStillUncertainVerifyItThenRetryOrCancelAnd,
			node.Destination,
			err,
		)
	}
	partial, partialErr := target.Stat(ctx, node.Temporary)
	if partialErr != nil && !errors.Is(partialErr, fs.ErrNotExist) {
		return uncertain(partialErr)
	}
	partialExists := partialErr == nil
	if partialExists && (partial.Symlink || partial.Directory || partial.Size < 0 || partial.Size > node.Entry.Size) {
		return uncertain(locales.NewError(locales.MessageTheStagedFileTypeOrSizeHasChanged))
	}
	if partialExists && node.Staged != nil && !sameTransferEntry(partial, *node.Staged) {
		return uncertain(locales.NewError(locales.MessageTheVerifiedStagedFileHasChanged))
	}
	entry, targetErr := target.Stat(ctx, node.Destination)
	if errors.Is(targetErr, fs.ErrNotExist) {
		if partialExists {
			node.Committing = false
			return nil // The owned bytes remain staged; no final target exists.
		}
		return uncertain(locales.NewError(locales.MessageNeitherTheStagedFileNorTheDestinationExists))
	}
	if targetErr != nil {
		return uncertain(targetErr)
	}
	if entry.Symlink || entry.Directory || entry.Size != node.Entry.Size ||
		entry.ModeKnown && !entry.Mode.IsRegular() {
		if partialExists {
			node.Committing = false
			return nil
		}
		return uncertain(locales.NewError(locales.MessageTheDestinationTypeOrSizeDoesNotMatch))
	}
	if node.Digest == "" {
		return uncertain(locales.NewError(locales.MessageTheCommitCheckpointIsMissingItsDigest))
	}
	digest, err := hashTransferFile(ctx, target, node.Destination, node.Entry.Size)
	if err != nil {
		return uncertain(locales.Errorf(locales.MessageUnableToReadBackTheDestinationForVerification, err))
	}
	fresh, err := target.Stat(ctx, node.Destination)
	if err != nil {
		return uncertain(err)
	}
	if !sameTransferEntry(fresh, entry) {
		return uncertain(locales.NewError(locales.MessageTheDestinationChangedDuringVerification))
	}
	if digest != node.Digest {
		if partialExists {
			node.Committing = false
			return nil
		}
		return uncertain(locales.NewError(locales.MessageTheDestinationContentsDoNotMatch))
	}
	if partialExists {
		if err := target.Remove(ctx, node.Temporary, false); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return uncertain(locales.Errorf(locales.MessageDestinationContentsVerifiedButTheStagedCopyCouldNotBeCleanedUp, err))
		}
	}
	markTransferCommitted(job, node, run)
	return nil
}
