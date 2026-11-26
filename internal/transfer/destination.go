package transfer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/localfs"
)

func (job *Job) chooseDestination(ctx context.Context, run *Run,
	target fileclient.Client, proposed string, source fileclient.Entry, reserve func(string, bool) error,
) (string, *fileclient.Entry, bool, error) {
	remote := !job.SourceRemote
	parent, original := Dir(remote, proposed), Base(remote, proposed)
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
		choice := int(job.Batch.Policy.Load())
		if choice == 0 && job.MCPFailConflict {
			return "", nil, false, fs.ErrExist
		}
		if choice == 0 {
			answer, err := run.ResolveConflict(ctx, Dir(remote, candidate),
				fileclient.Entry{Name: Base(remote, candidate), Size: source.Size, Directory: source.Directory})
			if err != nil {
				return "", nil, false, err
			}
			choice = answer
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
				candidate = Join(remote, parent, name)
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
			return "", nil, false, ErrConflictCanceled
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
	if !SafeName(target) {
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

func SafeName(name string) bool {
	return localfs.ValidName(name)
}

func Join(remote bool, parent, name string) string {
	if remote {
		return path.Join(parent, name)
	}
	return filepath.Join(parent, name)
}

func Dir(remote bool, name string) string {
	if remote {
		return path.Dir(name)
	}
	return filepath.Dir(name)
}

func Base(remote bool, name string) string {
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
