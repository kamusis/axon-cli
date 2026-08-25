package vendor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// AcquireSyncLock takes an exclusive lock over the vendor cache, preventing
// concurrent `axon vendor sync` runs from racing on the same cache/Hub
// paths. git fetch/checkout and rsync --delete are not safe to run
// concurrently against the same directories: one process can delete or
// recreate a directory tree while another is mid-scan of it, producing
// confusing rsync/git failures.
//
// Unlike the update lock, this does not retry — a full vendor sync can take
// a while, so blocking on someone else's run isn't a useful default. Callers
// get an immediate, actionable error instead.
func AcquireSyncLock() (*flock.Flock, func(), error) {
	root, err := CacheRoot()
	if err != nil {
		return nil, func() {}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, func() {}, fmt.Errorf("cannot create cache root: %w", err)
	}
	lockPath := filepath.Join(root, "sync.lock")
	l := flock.New(lockPath)
	locked, err := l.TryLock()
	if err != nil {
		return nil, func() {}, fmt.Errorf("cannot acquire vendor sync lock: %w", err)
	}
	if !locked {
		return nil, func() {}, fmt.Errorf("another `axon vendor sync` is already running (lock: %s)", lockPath)
	}
	return l, func() { _ = l.Unlock() }, nil
}
