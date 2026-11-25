//go:build integration

package sshclient_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/transfer"
)

func TestSSHIntegrationResumeQueuedUpload(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn := dialIntegrationConnection(t, sshFixture(t).config)
	release, err := conn.Retain()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	open := func() fileclient.Client {
		t.Helper()
		client, err := fileclient.NewSFTP(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}

	name := "queue-" + rand.Text() + ".bin"
	source, target := filepath.Join(t.TempDir(), name), "/home/burrow"
	payload := bytes.Repeat([]byte("Burrow"), 1<<20)
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	queue := &transfer.Queue[int]{}
	batch := queue.NewBatch(0)
	task := &transfer.Task[int]{Entry: fileclient.Entry{Name: name, Size: int64(len(payload))},
		SourcePath: source, TargetPath: target, Persisted: true}
	queue.Enqueue(task, batch)
	run := task.Begin(ctx, false)
	run.Ready <- transfer.ClientResult{Client: open()}
	checkpoint := make(chan int64)
	run.Journal = func() error {
		if cp := task.Job.Root.Checkpoint; cp != nil && cp.Offset < int64(len(payload)) {
			select {
			case checkpoint <- cp.Offset:
			case <-ctx.Done():
				return ctx.Err()
			}
			<-run.Context.Done()
			return run.Context.Err()
		}
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- run.Work(task.Job, &queue.Locks, false, nil) }()
	var offset int64
	select {
	case offset = <-checkpoint:
	case err := <-done:
		t.Fatalf("worker ended before a partial checkpoint: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !task.Pause() || !task.Resume() {
		t.Fatal("pause/resume rejected")
	}
	queue.Schedule(2, true, func(*int) bool { return false }, func(*transfer.Task[int], bool) {
		t.Fatal("resumed job acquired the manifest while its canceled worker still owned it")
	})
	err = <-done
	if !errors.Is(err, context.Canceled) || queue.Finish(task, run, false, err) || task.State != transfer.Queued {
		t.Fatalf("canceled worker changed resumed state to %v: %v", task.State, err)
	}

	recovered := transfer.RestoreTask[int](task.Snapshot(1), batch, nil, nil)
	if recovered.State != transfer.Paused || !recovered.Resume() {
		t.Fatal("restored job could not resume")
	}
	queue.Tasks = []*transfer.Task[int]{recovered}
	queue.Schedule(1, true, func(*int) bool { return false }, func(task *transfer.Task[int], cleanup bool) {
		run := task.Begin(ctx, cleanup)
		run.Ready <- transfer.ClientResult{Client: open()}
		err := run.Work(task.Job, &queue.Locks, cleanup, nil)
		if !queue.Finish(task, run, cleanup, err) || run.Written.Load() != int64(len(payload))-offset {
			t.Fatalf("resume wrote %d bytes from checkpoint %d: %v", run.Written.Load(), offset, err)
		}
	})
	if recovered.State != transfer.Done || len(recovered.Job.Temporary) != 0 {
		t.Fatal("restored job did not commit its staged file")
	}
	client := open()
	defer client.Close()
	stream, err := client.OpenRead(ctx, path.Join(target, name), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("committed contents differ: %v", err)
	}
}
