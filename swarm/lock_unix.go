//go:build unix

package swarm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFile acquires an exclusive flock on the given path and returns an
// unlock function. The lock file is created if it doesn't exist.
//
// Unlike a plain LOCK_EX (which blocks indefinitely), this implementation
// uses LOCK_NB and retries every 50 ms, honouring context cancellation and
// applying a 30-second hard deadline so the caller can never hang forever.
//
// Lock files are intentionally NOT deleted on unlock to avoid the classic
// flock+unlink TOCTOU race (where a concurrent process acquires a lock on
// the old inode, then the unlink deletes it, and a third process creates a
// new inode — resulting in two processes thinking they hold the lock).
// The .lock files are zero-byte and harmless to accumulate.
func lockFile(ctx context.Context, path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	const retryInterval = 5 * time.Millisecond
	const maxWait = 30 * time.Second
	deadline := time.Now().Add(maxWait)

	for {
		flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr == nil {
			// Lock acquired successfully.
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
				// Intentionally do NOT os.Remove(path) — see comment above.
			}, nil
		}
		if flockErr != syscall.EWOULDBLOCK {
			// Unexpected syscall error — do not retry.
			_ = f.Close()
			return nil, flockErr
		}

		// Lock is held by another process. Check context before sleeping.
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("lockFile %s: %w", path, ctx.Err())
		default:
		}

		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("lockFile %s: timed out after %s waiting for exclusive lock", path, maxWait)
		}

		// Sleep for one retry interval, but wake early on context cancellation.
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("lockFile %s: %w", path, ctx.Err())
		case <-time.After(retryInterval):
		}
	}
}
