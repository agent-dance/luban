package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// cron_lock.go — multi-session scheduler lock with periodic probe.
//
// Mirrors the TS cronScheduler.ts tryAcquireSchedulerLock /
// releaseSchedulerLock / LOCK_PROBE_INTERVAL_MS contract:
//
//   * One scheduler in the process group holds the leader lock and drives
//     recurring cron firing.
//   * Loser sessions probe periodically (every probeInterval — default 5s)
//     and take over within `staleTimeout` of the leader exiting (default 10s).
//   * The lock file records the holder's pid + a fresh heartbeat timestamp
//     so a crashed leader is reaped automatically.

// schedulerLockProbeInterval is how often non-leader sessions retry to take
// over the leader lock. Mirrors LOCK_PROBE_INTERVAL_MS=5000.
const schedulerLockProbeInterval = 5 * time.Second

// schedulerLockStaleAfter is how stale a heartbeat must be before another
// session is allowed to steal the lock. The TS scheduler uses 2x the probe
// interval to absorb a missed heartbeat tick.
const schedulerLockStaleAfter = 2 * schedulerLockProbeInterval

// schedulerLockHeartbeatInterval is how often the leader refreshes the
// heartbeat in the lock file.
const schedulerLockHeartbeatInterval = schedulerLockProbeInterval

type schedulerLockFile struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at"`
	Heartbeat int64 `json:"heartbeat"`
}

// SchedulerLock is the multi-session leader lock.
type SchedulerLock struct {
	path string

	mu     sync.Mutex
	holder bool // we currently hold the lock
	stopHB chan struct{}
	hbDone chan struct{}
	clock  func() time.Time
	pid    int
}

// NewSchedulerLock returns a lock keyed at `path` (typically
// scheduled_tasks.json.scheduler.lock).
func NewSchedulerLock(path string) *SchedulerLock {
	return &SchedulerLock{
		path:  path,
		clock: time.Now,
		pid:   os.Getpid(),
	}
}

// SetClock overrides time.Now (test-only).
func (l *SchedulerLock) SetClock(fn func() time.Time) {
	if fn == nil {
		return
	}
	l.mu.Lock()
	l.clock = fn
	l.mu.Unlock()
}

// SetPID overrides os.Getpid() (test-only).
func (l *SchedulerLock) SetPID(pid int) {
	l.mu.Lock()
	l.pid = pid
	l.mu.Unlock()
}

// IsHolder returns whether this lock instance currently holds the leader role.
func (l *SchedulerLock) IsHolder() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.holder
}

// readLockFile parses the on-disk lock state. A missing file yields a zero
// struct and no error.
func (l *SchedulerLock) readLockFile() (schedulerLockFile, bool, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return schedulerLockFile{}, false, nil
		}
		return schedulerLockFile{}, false, err
	}
	var lf schedulerLockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		// Corrupt lock — treat as absent so we can recover.
		return schedulerLockFile{}, false, nil
	}
	return lf, true, nil
}

// writeLockFile atomically writes the lock state.
func (l *SchedulerLock) writeLockFile(lf schedulerLockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	return atomicWriteFile(l.path, append(data, '\n'), 0o644)
}

// TryAcquire attempts to take the leader lock. Returns true if we now hold
// it. If a stale leader is detected, the lock is stolen.
func (l *SchedulerLock) TryAcquire() (bool, error) {
	l.mu.Lock()
	if l.holder {
		l.mu.Unlock()
		return true, nil
	}
	clock := l.clock
	pid := l.pid
	l.mu.Unlock()

	// Use a file lock around read+write to prevent two sessions racing on
	// the same file. The withRuntimeFileLockResult helper already exists.
	res, err := withRuntimeFileLockResult(l.path+".lk", func() (any, error) {
		lf, exists, readErr := l.readLockFile()
		if readErr != nil {
			return false, readErr
		}
		now := clock()
		if exists {
			heartbeat := time.UnixMilli(lf.Heartbeat)
			fresh := now.Sub(heartbeat) < schedulerLockStaleAfter
			if fresh && lf.PID != pid {
				return false, nil
			}
		}
		newLF := schedulerLockFile{
			PID:       pid,
			StartedAt: now.UnixMilli(),
			Heartbeat: now.UnixMilli(),
		}
		if err := l.writeLockFile(newLF); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return false, err
	}
	took, _ := res.(bool)
	if !took {
		return false, nil
	}

	l.mu.Lock()
	l.holder = true
	l.stopHB = make(chan struct{})
	l.hbDone = make(chan struct{})
	stop := l.stopHB
	done := l.hbDone
	l.mu.Unlock()

	go l.heartbeatLoop(stop, done)
	return true, nil
}

// heartbeatLoop refreshes the heartbeat field while we hold the lock.
func (l *SchedulerLock) heartbeatLoop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	tick := time.NewTicker(schedulerLockHeartbeatInterval)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			l.mu.Lock()
			clock := l.clock
			pid := l.pid
			l.mu.Unlock()
			now := clock()
			_, _ = withRuntimeFileLockResult(l.path+".lk", func() (any, error) {
				lf, exists, _ := l.readLockFile()
				if !exists || lf.PID != pid {
					return nil, errors.New("lock taken over")
				}
				lf.Heartbeat = now.UnixMilli()
				return nil, l.writeLockFile(lf)
			})
		}
	}
}

// Release surrenders the lock if we hold it. Safe to call repeatedly.
func (l *SchedulerLock) Release() {
	l.mu.Lock()
	if !l.holder {
		l.mu.Unlock()
		return
	}
	l.holder = false
	stop := l.stopHB
	done := l.hbDone
	pid := l.pid
	l.stopHB = nil
	l.hbDone = nil
	l.mu.Unlock()

	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}

	_, _ = withRuntimeFileLockResult(l.path+".lk", func() (any, error) {
		lf, exists, _ := l.readLockFile()
		if exists && lf.PID == pid {
			_ = os.Remove(l.path)
		}
		return nil, nil
	})
}

// ProbeInterval returns the probe interval (test-friendly).
func (l *SchedulerLock) ProbeInterval() time.Duration {
	return schedulerLockProbeInterval
}

// schedulerProcessAlive reports whether the given pid identifies a running
// process. On Windows we conservatively return true so we don't aggressively
// steal locks (the heartbeat staleness check is the safety net).
func schedulerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Windows we can't reliably interrogate arbitrary pids; return
	// true so the heartbeat-staleness check is the only kill signal.
	if runtimeIsWindows() {
		return true
	}
	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		return false
	}
	if err := proc.Signal(syscallZeroSignal()); err == nil {
		return true
	}
	return false
}

// schedulerLockHeartbeatString returns a debug-friendly representation of the
// holder's pid + heartbeat. Used by tests and probes.
func (l *SchedulerLock) schedulerLockHeartbeatString() string {
	lf, exists, _ := l.readLockFile()
	if !exists {
		return "no-leader"
	}
	return "pid=" + strconv.Itoa(lf.PID) + " heartbeat=" + time.UnixMilli(lf.Heartbeat).UTC().Format(time.RFC3339Nano)
}

// schedulerLockFileForTest exposes the on-disk lock contents to tests.
func (l *SchedulerLock) schedulerLockFileForTest() (schedulerLockFile, bool) {
	lf, exists, _ := l.readLockFile()
	return lf, exists
}

// fmtUnused keeps fmt-import alive when the file shrinks during refactors.
var _ = fmt.Sprintf
