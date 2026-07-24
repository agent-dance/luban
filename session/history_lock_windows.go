//go:build windows

package session

import (
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func (s *FileStore) lockSessionHistory(sessionID string, exclusive bool) (func(), error) {
	if _, err := s.storageRoot(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, "."+sessionID+".history.lock")
	file, err := s.openOrCreatePrivateRegularFile(path)
	if err != nil {
		return nil, err
	}
	var overlapped windows.Overlapped
	flags := uint32(0)
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, flags, 0, ^uint32(0), ^uint32(0), &overlapped); err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := validateAndTightenPrivateRegularFile(file, path); err != nil {
		_ = windows.UnlockFileEx(handle, 0, ^uint32(0), ^uint32(0), &overlapped)
		_ = file.Close()
		return nil, err
	}
	current, err := s.openPrivateRegularFile(path)
	if err != nil {
		_ = windows.UnlockFileEx(handle, 0, ^uint32(0), ^uint32(0), &overlapped)
		_ = file.Close()
		return nil, err
	}
	heldInfo, heldErr := file.Stat()
	currentInfo, currentErr := current.Stat()
	closeCurrentErr := current.Close()
	if heldErr != nil || currentErr != nil || closeCurrentErr != nil || !os.SameFile(heldInfo, currentInfo) {
		_ = windows.UnlockFileEx(handle, 0, ^uint32(0), ^uint32(0), &overlapped)
		_ = file.Close()
		if heldErr != nil {
			return nil, heldErr
		}
		if currentErr != nil {
			return nil, currentErr
		}
		if closeCurrentErr != nil {
			return nil, closeCurrentErr
		}
		return nil, fs.ErrInvalid
	}
	s.registerHistoryLock(sessionID, file)
	return func() {
		s.unregisterHistoryLock(sessionID, file)
		_ = windows.UnlockFileEx(handle, 0, ^uint32(0), ^uint32(0), &overlapped)
		_ = file.Close()
	}, nil
}
