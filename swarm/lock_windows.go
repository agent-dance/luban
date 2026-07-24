//go:build windows

package swarm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// lockFile acquires an exclusive byte-range lock on Windows. Lock files are
// kept after unlock for the same TOCTOU reason as the Unix implementation.
func lockFile(ctx context.Context, path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	const retryInterval = 50 * time.Millisecond
	const maxWait = 30 * time.Second
	deadline := time.Now().Add(maxWait)
	overlapped := windows.Overlapped{}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)

	for {
		lockErr := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, ^uint32(0), ^uint32(0), &overlapped)
		if lockErr == nil {
			return func() {
				_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, ^uint32(0), ^uint32(0), &overlapped)
				_ = f.Close()
			}, nil
		}

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

		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("lockFile %s: %w", path, ctx.Err())
		case <-time.After(retryInterval):
		}
	}
}
