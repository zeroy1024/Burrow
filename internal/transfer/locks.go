package transfer

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type Locks struct {
	mu      sync.Mutex
	held    map[*transferLock]struct{}
	changed chan struct{}
}

type transferLock struct {
	key, path string
	directory bool
	owner     *Job
}

func (l *Locks) acquire(ctx context.Context, key, name string, directory bool, owner *Job) (func(), error) {
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
