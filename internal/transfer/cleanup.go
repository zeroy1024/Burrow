package transfer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
)

func Cleanup(ctx context.Context, target fileclient.Client, job *Job) error {
	var failures []error
	names := make([]string, 0, len(job.Temporary))
	for name := range job.Temporary {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int { return len(b) - len(a) })
	for _, name := range names {
		entry, err := target.Stat(ctx, name)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.Temporary, name)
			clearTransferTemporary(job.Root, name)
			continue
		}
		privateDirectory := transferPrivateDirectory(job.Root, name)
		if err == nil && (entry.Symlink || entry.Directory != privateDirectory) {
			err = locales.NewError(locales.MessageTheTemporaryFileWasReplacedDeletionBlocked)
		}
		if err == nil {
			err = target.Remove(ctx, name, privateDirectory)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		} else {
			delete(job.Temporary, name)
			clearTransferTemporary(job.Root, name)
		}
	}
	return errors.Join(failures...)
}

func clearTransferTemporary(node *Node, name string) {
	if node == nil {
		return
	}
	if node.StageDirectory == name {
		node.StageDirectory = ""
	}
	if node.Temporary == name {
		node.Temporary = ""
		node.Staged, node.Digest = nil, ""
		node.Partial, node.Checkpoint = nil, nil
		node.Committing = false
	}
	for _, child := range node.Children {
		clearTransferTemporary(child, name)
	}
}

func transferPrivateDirectory(node *Node, name string) bool {
	if node == nil {
		return false
	}
	if node.StageDirectory == name {
		return true
	}
	for _, child := range node.Children {
		if transferPrivateDirectory(child, name) {
			return true
		}
	}
	return false
}

func cleanupTransferDirectories(ctx context.Context, target fileclient.Client, job *Job) error {
	for name := range job.Temporary {
		if !transferPrivateDirectory(job.Root, name) {
			continue
		}
		entry, err := target.Stat(ctx, name)
		if errors.Is(err, fs.ErrNotExist) {
			delete(job.Temporary, name)
			clearTransferTemporary(job.Root, name)
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
		delete(job.Temporary, name)
		clearTransferTemporary(job.Root, name)
	}
	return nil
}
