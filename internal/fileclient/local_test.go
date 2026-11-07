//go:build darwin || linux

package fileclient_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeroy1024/Burrow/internal/fileclient"
)

func TestLocalRootAnchorsDownloadsAndCleanup(t *testing.T) {
	t.Parallel()
	base := localTestDirectory(t)
	selected, moved, outside := filepath.Join(base, "selected"), filepath.Join(base, "moved"), filepath.Join(base, "outside")
	localTestWrite(t, filepath.Join(selected, ".cancel.part"), "owned part")
	localTestWrite(t, filepath.Join(selected, "nested", "keep.txt"), "owned tree")
	localTestWrite(t, filepath.Join(outside, ".cancel.part"), "outside part")
	localTestWrite(t, filepath.Join(outside, ".download.part"), "outside stage")
	localTestWrite(t, filepath.Join(outside, "nested", "keep.txt"), "outside tree")
	client, err := fileclient.OpenLocalRoot(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { localTestClose(t, client) })
	localTestReplaceDirectory(t, selected, moved, outside)

	stage, destination := filepath.Join(selected, ".download.part"), filepath.Join(selected, "result.txt")
	output, err := client.OpenWrite(t.Context(), stage, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := io.WriteString(output, "downloaded payload")
	if err := errors.Join(writeErr, output.Close()); err != nil {
		t.Fatal(err)
	}
	if err := client.Rename(t.Context(), stage, destination, fileclient.RenameOptions{NoReplace: true}); err != nil {
		t.Fatal(err)
	}
	localTestContent(t, filepath.Join(moved, "result.txt"), "downloaded payload")
	localTestContent(t, filepath.Join(outside, ".download.part"), "outside stage")
	if _, err := os.Lstat(filepath.Join(outside, "result.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("download appeared outside the selected directory: %v", err)
	}

	if err := client.Remove(t.Context(), filepath.Join(selected, ".cancel.part"), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(moved, ".cancel.part")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("owned part was not removed: %v", err)
	}
	if err := client.(fileclient.TreeRemover).RemoveTree(t.Context(), filepath.Join(selected, "nested")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "nested")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("owned tree was not removed: %v", err)
	}
	localTestContent(t, filepath.Join(outside, ".cancel.part"), "outside part")
	localTestContent(t, filepath.Join(outside, "nested", "keep.txt"), "outside tree")
}

func TestLocalRootRejectsEscapingDescendants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(context.Context, fileclient.Client, string) error
	}{
		{"download", func(ctx context.Context, client fileclient.Client, directory string) error {
			output, err := client.OpenWrite(ctx, filepath.Join(directory, "guard.txt"), 0, true)
			if err != nil {
				return err
			}
			_, writeErr := io.WriteString(output, "escaped write")
			return errors.Join(writeErr, output.Close())
		}},
		{"cancel_part", func(ctx context.Context, client fileclient.Client, directory string) error {
			return client.Remove(ctx, filepath.Join(directory, "guard.txt"), false)
		}},
		{"recursive_cleanup", func(ctx context.Context, client fileclient.Client, directory string) error {
			return client.(fileclient.TreeRemover).RemoveTree(ctx, filepath.Join(directory, "child"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			base := localTestDirectory(t)
			selected, outside := filepath.Join(base, "selected"), filepath.Join(base, "outside")
			nested := filepath.Join(selected, "nested")
			localTestWrite(t, filepath.Join(nested, "guard.txt"), "selected guard")
			localTestWrite(t, filepath.Join(nested, "child", "keep.txt"), "selected child")
			localTestWrite(t, filepath.Join(outside, "guard.txt"), "outside guard")
			localTestWrite(t, filepath.Join(outside, "child", "keep.txt"), "outside child")
			client, err := fileclient.OpenLocalRoot(t.Context(), selected)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { localTestClose(t, client) })
			localTestReplaceDirectory(t, nested, filepath.Join(selected, "held"), outside)
			err = test.run(t.Context(), client, nested)
			localTestContent(t, filepath.Join(outside, "guard.txt"), "outside guard")
			localTestContent(t, filepath.Join(outside, "child", "keep.txt"), "outside child")
			if err == nil {
				t.Fatal("operation followed a descendant link outside the selected root")
			}
		})
	}
}

func TestLocalRootRemoveTreeUnlinksDirectoryLink(t *testing.T) {
	t.Parallel()
	base := localTestDirectory(t)
	selected, outside := filepath.Join(base, "selected"), filepath.Join(base, "outside")
	localTestWrite(t, filepath.Join(selected, "nested", "keep.txt"), "selected tree")
	localTestWrite(t, filepath.Join(outside, "keep.txt"), "outside tree")
	client, err := fileclient.OpenLocalRoot(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { localTestClose(t, client) })
	link := filepath.Join(selected, "nested")
	localTestReplaceDirectory(t, link, filepath.Join(selected, "held"), outside)
	if err := client.(fileclient.TreeRemover).RemoveTree(t.Context(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("directory link was not unlinked: %v", err)
	}
	localTestContent(t, filepath.Join(outside, "keep.txt"), "outside tree")
	localTestContent(t, filepath.Join(selected, "held", "keep.txt"), "selected tree")
}

func TestLocalSourceAnchorsDirectoryAndFileParent(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			base := localTestDirectory(t)
			selected, moved, outside := filepath.Join(base, "source"), filepath.Join(base, "moved"), filepath.Join(base, "outside")
			localTestWrite(t, filepath.Join(selected, "payload.txt"), "selected source")
			localTestWrite(t, filepath.Join(outside, "payload.txt"), "outside private data")
			source := selected
			if kind == "file" {
				source = filepath.Join(selected, "payload.txt")
			}
			client, err := fileclient.OpenLocalSource(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { localTestClose(t, client) })
			localTestReplaceDirectory(t, selected, moved, outside)
			input, err := client.OpenRead(t.Context(), filepath.Join(selected, "payload.txt"), 0)
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(input)
			if err := errors.Join(readErr, input.Close()); err != nil {
				t.Fatal(err)
			}
			if string(data) != "selected source" {
				t.Fatal("upload source was redirected outside the selected directory")
			}
			localTestContent(t, filepath.Join(outside, "payload.txt"), "outside private data")
		})
	}
}

func TestLocalSourceDoesNotFollowReplacedFile(t *testing.T) {
	t.Parallel()
	base := localTestDirectory(t)
	selected, outside := filepath.Join(base, "source", "payload.txt"), filepath.Join(base, "outside", "payload.txt")
	localTestWrite(t, selected, "selected source")
	localTestWrite(t, outside, "outside private data")
	client, err := fileclient.OpenLocalSource(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { localTestClose(t, client) })
	input, err := client.OpenRead(t.Context(), selected, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(selected, selected+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, selected); err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(input)
	if err := errors.Join(readErr, input.Close()); err != nil {
		t.Fatal(err)
	}
	if string(data) != "selected source" {
		t.Fatal("an opened upload stream changed identity after replacing its source")
	}
	if redirected, err := client.OpenRead(t.Context(), selected, 0); err == nil {
		localTestClose(t, redirected)
		t.Fatal("a later upload read followed the replacement symbolic link")
	}
	if redirected, err := fileclient.OpenLocalSource(t.Context(), selected); err == nil {
		localTestClose(t, redirected)
		t.Fatal("a new upload accepted a source symbolic link")
	}
	localTestContent(t, outside, "outside private data")
}

func localTestDirectory(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func localTestWrite(t *testing.T, name, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func localTestContent(t *testing.T, name, want string) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("unexpected contents at %s", name)
	}
}

func localTestReplaceDirectory(t *testing.T, original, held, outside string) {
	t.Helper()
	if err := os.Rename(original, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, original); err != nil {
		t.Fatal(err)
	}
}

func localTestClose(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Error(err)
	}
}
