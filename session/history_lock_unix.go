//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
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
	operation := syscall.LOCK_SH
	if exclusive {
		operation = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), operation); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock session history: %w", err)
	}
	if _, err := validateAndTightenPrivateRegularFile(file, path); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	current, err := s.openPrivateRegularFile(path)
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	heldInfo, heldErr := file.Stat()
	currentInfo, currentErr := current.Stat()
	closeCurrentErr := current.Close()
	if heldErr != nil || currentErr != nil || closeCurrentErr != nil || !os.SameFile(heldInfo, currentInfo) {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
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
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
