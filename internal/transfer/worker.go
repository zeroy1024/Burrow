package transfer

import (
	"errors"
	"sync"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
)

// Work owns the protocol clients for one attempt. The caller supplies an authorized
// remote client on Ready; downloads open their local directory before waiting for it.
func (run *Run) Work(job *Job, locks *Locks, cleanup bool, wrapSource func(fileclient.Client) fileclient.Client) error {
	ctx := run.Context
	var local, remote fileclient.Client
	var err error
	if run.MCP != nil && !(cleanup && !job.SourceRemote) {
		local, err = fileclient.OpenLocalRoot(ctx, run.MCP.LocalRoot)
	} else if job.SourceRemote {
		// Anchor the selected destination before authentication or a conflict
		// can leave time for another process to replace its descendants.
		local, err = fileclient.OpenLocalRoot(ctx, job.TargetPath)
	} else if !cleanup {
		// Keep upload reads inside the selected source while authentication
		// and conflict dialogs run. Remote cleanup does not need the source.
		local, err = fileclient.OpenLocalSource(ctx, job.SourcePath)
	}
	if err == nil && !(cleanup && job.SourceRemote) {
		select {
		case result := <-run.Ready:
			remote, err = result.Client, result.Err
		case <-ctx.Done():
			err = ctx.Err()
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		source, target := local, remote
		if job.SourceRemote {
			source, target = remote, local
		}
		if wrapSource != nil {
			source = wrapSource(source)
		}
		if cleanup {
			err = Cleanup(ctx, target, job)
		} else {
			stopProgress := make(chan struct{})
			var progress sync.WaitGroup
			if run.ProgressChanged != nil {
				progress.Go(func() { run.trackProgress(stopProgress) })
			}
			err = job.Execute(ctx, run, source, target, locks)
			close(stopProgress)
			progress.Wait()
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
	return err
}

func (run *Run) trackProgress(stopped <-chan struct{}) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	previous, previousAt := int64(0), time.Now()
	for {
		select {
		case now := <-ticker.C:
			bytes, total, written := run.Bytes.Load(), run.Total.Load(), run.Written.Load()
			speed := float64(max(written-previous, 0)) / now.Sub(previousAt).Seconds()
			previous, previousAt = written, now
			run.ProgressChanged(bytes, total, speed)
		case <-stopped:
			return
		case <-run.Context.Done():
			return
		}
	}
}
