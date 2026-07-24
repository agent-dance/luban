//go:build windows

package tools

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-dance/luban/i18n"
	"golang.org/x/sys/windows"
)

func lockRuntimeFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	handle := windows.Handle(f.Fd())
	var ol windows.Overlapped

	const retryInterval = 50 * time.Millisecond
	const maxWait = 30 * time.Second
	deadline := time.Now().Add(maxWait)

	for {
		err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(handle, 0, 1, 0, &ol)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) && !errors.Is(err, windows.ERROR_IO_PENDING) {
			_ = f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, i18n.NewError(i18n.KeyToolFileLockTimedOut, path)
		}
		time.Sleep(retryInterval)
	}
}
