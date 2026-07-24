//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package session

import "sync"

var fallbackHistoryLocks sync.Map

func (s *FileStore) lockSessionHistory(sessionID string, _ bool) (func(), error) {
	key := s.dir + "\x00" + sessionID
	value, _ := fallbackHistoryLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock, nil
}
