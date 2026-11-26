package transfer

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
)

type State int

const (
	Queued State = iota
	Running
	Paused
	Done
	Failed
)

type Task[S any] struct {
	MCP       *MCPOwner
	ID        int
	Persisted bool
	Session   *S
	// View is the owning tab; destination reveal never transfers ownership.
	View                   *S
	Entry                  fileclient.Entry
	SourceRemote           bool
	SourcePath, TargetPath string
	State                  State
	Transferred, Total     int64
	Speed                  float64
	Started, Completed     time.Time
	ErrorText              locales.Text
	Job                    *Job
	Run                    *Run
	RemoveRequested        bool
	FailureNotice          bool
	CleanupPending         bool
}

type Summary struct {
	Running, Queued, Paused, Done int
	Bytes, Total                  int64
	Speed                         float64
}

// Queue is owned by one serial caller. Workers publish progress through Run.
type Queue[S any] struct {
	Tasks               []*Task[S]
	NextID, NextBatchID int
	Locks               Locks
	Quitting            bool
	StoreErr, WriteErr  error
}

func (q *Queue[S]) NewBatch(policy int) *Batch {
	q.NextBatchID++
	batch := &Batch{ID: q.NextBatchID}
	batch.Policy.Store(int32(policy))
	return batch
}

// Enqueue binds the source and target paths to this task before any asynchronous work starts.
func (q *Queue[S]) Enqueue(task *Task[S], batch *Batch) {
	q.NextID++
	task.ID, task.State = q.NextID, Queued
	task.Job = &Job{
		SourceRemote: task.SourceRemote, SourcePath: task.SourcePath, TargetPath: task.TargetPath,
		Name: task.Entry.Name, Batch: batch, Temporary: make(map[string]struct{}),
	}
	q.Tasks = append(q.Tasks, task)
}

// Schedule calls start synchronously. start must attach its run before returning.
func (q *Queue[S]) Schedule(limit int, durable bool, needsPassword func(*S) bool, start func(*Task[S], bool)) {
	if q.Quitting || q.StoreErr != nil || q.WriteErr != nil {
		return
	}
	active := 0
	for _, task := range q.Tasks {
		if task.Run != nil {
			active++
		}
	}
	for _, task := range q.Tasks {
		if active >= limit {
			break
		}
		if task.Run == nil && task.RemoveRequested && (!durable || task.Persisted) {
			start(task, true)
			active++
			continue
		}
		if task.State != Queued || task.Run != nil || task.RemoveRequested || durable && !task.Persisted {
			continue
		}
		if needsPassword(task.Session) {
			waiting := slices.ContainsFunc(q.Tasks, func(other *Task[S]) bool {
				return other.Session == task.Session && other.Run != nil && other.Run.AwaitingClient
			})
			if waiting {
				continue
			}
		}
		start(task, false)
		active++
	}
}

func Summarize[S any](tasks []*Task[S]) Summary {
	s := Summary{}
	for _, task := range tasks {
		s.Bytes += task.Transferred
		s.Total += task.Total
		switch task.State {
		case Running:
			s.Running++
			s.Speed += task.Speed
		case Queued:
			s.Queued++
		case Paused:
			s.Paused++
		case Done:
			s.Done++
		}
	}
	return s
}

func (task *Task[S]) Begin(base context.Context, cleanup bool) *Run {
	ctx, cancel := context.WithCancel(base)
	run := &Run{Context: ctx, Cancel: cancel, Ready: make(chan ClientResult, 1), AwaitingClient: true, MCP: task.MCP}
	run.Manifest.Store(SnapshotManifest(task.Job))
	run.Bytes.Store(task.Transferred)
	run.Total.Store(task.Total)
	task.Run, task.Speed = run, 0
	if !cleanup {
		task.State, task.ErrorText = Running, locales.Text{}
		if task.Started.IsZero() {
			task.Started = time.Now()
		}
	}
	return run
}

// Finish releases the active attempt and reports a newly completed copy.
func (q *Queue[S]) Finish(task *Task[S], run *Run, cleanup bool, err error) bool {
	task.Run = nil
	run.Cancel()
	run.Request.Cancel()
	task.Speed = 0
	if cleanup {
		if err == nil {
			q.Tasks = slices.DeleteFunc(q.Tasks, func(candidate *Task[S]) bool { return candidate == task })
		} else {
			task.RemoveRequested, task.CleanupPending = false, true
			task.State, task.ErrorText = Failed, locales.Concat(
				locales.Deferred(locales.MessageUnableToCleanUpTemporaryFilesAfterCancellation),
				locales.ErrorText(err),
			)
			task.FailureNotice = true
		}
	} else {
		task.Transferred, task.Total = run.Bytes.Load(), run.Total.Load()
		if task.RemoveRequested || errors.Is(err, ErrConflictCanceled) {
			task.RemoveRequested = true
			if len(task.Job.Temporary) == 0 {
				q.Tasks = slices.DeleteFunc(q.Tasks, func(candidate *Task[S]) bool { return candidate == task })
			} else if q.Quitting {
				task.RemoveRequested, task.CleanupPending = false, true
				task.State, task.ErrorText = Failed, locales.Deferred(
					locales.MessageTemporaryFilesAwaitingCleanupAfterCancellation,
				)
			} else {
				task.CleanupPending = true
				task.State = Failed
			}
		} else if task.State == Paused || task.State == Queued {
			// A pause may already have been followed by resume. The old worker
			// still has to finish before the new attempt can own its manifest.
		} else if err != nil {
			task.State, task.ErrorText, task.FailureNotice = Failed, locales.ErrorText(err), true
		} else {
			task.State, task.Completed = Done, time.Now()
			if destination := task.Job.Root.Destination; destination != "" {
				task.Entry.Name = Base(!task.SourceRemote, destination)
				task.TargetPath = Dir(!task.SourceRemote, destination)
			}
			return true
		}
	}
	return false
}

func (task *Task[S]) Pause() bool {
	if task.RemoveRequested || task.CleanupPending {
		return false
	}
	task.State, task.Speed = Paused, 0
	if task.Run != nil {
		task.Run.Cancel()
		task.Run.Request.Cancel()
	}
	return true
}

func (task *Task[S]) Resume() bool {
	if task.State == Running || task.State == Done || task.CleanupPending || task.RemoveRequested {
		return false
	}
	task.State, task.ErrorText, task.FailureNotice = Queued, locales.Text{}, false
	return true
}

func (q *Queue[S]) Cancel(task *Task[S]) bool {
	if q.Quitting {
		return false
	}
	if task.RemoveRequested {
		return false
	}
	task.RemoveRequested, task.FailureNotice = true, false
	if task.Run != nil {
		task.Run.Cancel()
		task.Run.Request.Cancel()
	} else if task.Job == nil || len(task.Job.Temporary) == 0 {
		q.Tasks = slices.DeleteFunc(q.Tasks, func(candidate *Task[S]) bool { return candidate == task })
	} else {
		task.CleanupPending = true
		task.State = Failed
	}
	return true
}
