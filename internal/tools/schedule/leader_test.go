package schedule

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/store/secureio"
)

func TestLeaderLockSamePIDInstancesAreMutuallyExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leader.json")
	first := mustNewLeaderLock(t, path)
	second := mustNewLeaderLock(t, path)
	if first.pid != second.pid {
		t.Fatalf("test requires the same process: %d != %d", first.pid, second.pid)
	}
	if first.owner == second.owner {
		t.Fatal("distinct instances received the same owner token")
	}

	acquired, err := first.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("first.tryAcquire() = %v, %v", acquired, err)
	}
	acquired, err = second.tryAcquire(context.Background())
	if err != nil {
		t.Fatalf("second.tryAcquire() error = %v", err)
	}
	if acquired {
		t.Fatal("second instance acquired a fresh lease held by the same PID")
	}
	if err := first.release(context.Background()); err != nil {
		t.Fatalf("first.release() error = %v", err)
	}
	acquired, err = second.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("second.tryAcquire() after release = %v, %v", acquired, err)
	}
	if err := second.release(context.Background()); err != nil {
		t.Fatalf("second.release() error = %v", err)
	}
}

func TestLeaderLockSamePIDInstancesRaceForOneLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leader.json")
	locks := []*leaderLock{
		mustNewLeaderLock(t, path),
		mustNewLeaderLock(t, path),
	}
	start := make(chan struct{})
	type acquisition struct {
		acquired bool
		err      error
	}
	results := make(chan acquisition, len(locks))
	for _, lock := range locks {
		go func(candidate *leaderLock) {
			<-start
			acquired, err := candidate.tryAcquire(context.Background())
			results <- acquisition{acquired: acquired, err: err}
		}(lock)
	}
	close(start)

	winners := 0
	for range locks {
		result := <-results
		if result.err != nil {
			t.Fatalf("tryAcquire() error = %v", result.err)
		}
		if result.acquired {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("simultaneous acquisitions = %d, want 1", winners)
	}
	for _, lock := range locks {
		if err := lock.release(context.Background()); err != nil {
			t.Fatalf("release() error = %v", err)
		}
	}
}

func TestLeaderLockConcurrentOperationsAreRaceSafe(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const callers = 24
	var acquireWait sync.WaitGroup
	acquireErrors := make(chan error, callers)
	for range callers {
		acquireWait.Add(1)
		go func() {
			defer acquireWait.Done()
			acquired, err := lock.tryAcquire(ctx)
			if err != nil {
				acquireErrors <- err
				return
			}
			if !acquired {
				acquireErrors <- fs.ErrInvalid
			}
		}()
	}
	acquireWait.Wait()
	close(acquireErrors)
	for err := range acquireErrors {
		t.Fatalf("concurrent tryAcquire() error = %v", err)
	}
	if !lock.isHolder() {
		t.Fatal("lock is not held after concurrent acquisition")
	}

	var releaseWait sync.WaitGroup
	releaseErrors := make(chan error, callers)
	for range callers {
		releaseWait.Add(1)
		go func() {
			defer releaseWait.Done()
			if err := lock.release(ctx); err != nil {
				releaseErrors <- err
			}
		}()
	}
	releaseWait.Wait()
	close(releaseErrors)
	for err := range releaseErrors {
		t.Fatalf("concurrent release() error = %v", err)
	}
	if lock.isHolder() {
		t.Fatal("lock remains held after concurrent release")
	}
}

func TestLeaderLockOverlappingAcquireAndReleaseConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leader.json")
	lock := mustNewLeaderLock(t, path)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const callers = 16
	const iterations = 12
	var wait sync.WaitGroup
	errorsSeen := make(chan error, callers)
	for caller := range callers {
		wait.Add(1)
		go func(acquireFirst bool) {
			defer wait.Done()
			for iteration := range iterations {
				acquire := (iteration%2 == 0) == acquireFirst
				if acquire {
					if _, err := lock.tryAcquire(ctx); err != nil {
						errorsSeen <- err
						return
					}
					continue
				}
				if err := lock.release(ctx); err != nil {
					errorsSeen <- err
					return
				}
			}
		}(caller%2 == 0)
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("overlapping operation error = %v", err)
	}
	if err := lock.release(ctx); err != nil {
		t.Fatalf("final release() error = %v", err)
	}
	if lock.isHolder() {
		t.Fatal("final release left holder=true")
	}

	peer := mustNewLeaderLock(t, path)
	acquired, err := peer.tryAcquire(ctx)
	if err != nil || !acquired {
		t.Fatalf("peer.tryAcquire() after convergence = %v, %v", acquired, err)
	}
	if err := peer.release(ctx); err != nil {
		t.Fatalf("peer.release() error = %v", err)
	}
}

func TestLeaderLockUsesStrictCurrentSchema(t *testing.T) {
	cases := map[string]string{
		"legacy":    `{"pid":1,"started_at":1,"heartbeat":1}`,
		"version":   `{"schema_version":2,"owner_token":"` + strings.Repeat("a", 64) + `","generation":1,"pid":1,"acquired_at":1,"heartbeat_at":1}`,
		"unknown":   `{"schema_version":1,"owner_token":"` + strings.Repeat("a", 64) + `","generation":1,"pid":1,"acquired_at":1,"heartbeat_at":1,"legacy":true}`,
		"duplicate": `{"schema_version":1,"owner_token":"` + strings.Repeat("a", 64) + `","generation":1,"generation":2,"pid":1,"acquired_at":1,"heartbeat_at":1}`,
		"trailing":  `{"schema_version":1,"owner_token":"` + strings.Repeat("a", 64) + `","generation":1,"pid":1,"acquired_at":1,"heartbeat_at":1} {}`,
		"oversized": `{"schema_version":1,"owner_token":"` + strings.Repeat("a", 64) + `","generation":1,"pid":1,"acquired_at":1,"heartbeat_at":1,"padding":"` + strings.Repeat("x", maxLeaderLeaseBytes) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "leader.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			lock := mustNewLeaderLock(t, path)
			acquired, err := lock.tryAcquire(context.Background())
			if err == nil || acquired {
				t.Fatalf("tryAcquire() = %v, %v; want strict decode error", acquired, err)
			}
			preserved, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(preserved) != body {
				t.Fatal("invalid lease was overwritten")
			}
		})
	}
}

func TestLeaderLockPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "leader.json")
	lock := mustNewLeaderLock(t, path)
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}
	assertPermissions(t, dir, 0o700)
	assertPermissions(t, path, 0o600)
	assertPermissions(t, path+".guard", 0o600)
	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release() error = %v", err)
	}
}

func TestLeaderLockRejectsLinkedRuntimeFiles(t *testing.T) {
	for _, target := range []string{"lease", "guard"} {
		for _, linkKind := range []string{"symlink", "hardlink"} {
			t.Run(target+"_"+linkKind, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "leader.json")
				linkPath := path
				if target == "guard" {
					linkPath += ".guard"
				}
				source := filepath.Join(dir, "source")
				if err := os.WriteFile(source, []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				if linkKind == "symlink" {
					err = os.Symlink(source, linkPath)
				} else {
					err = os.Link(source, linkPath)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := newLeaderLock(path); err == nil {
					t.Fatal("newLeaderLock() accepted a linked private runtime file")
				}
			})
		}
	}
}

func TestLeaderLockHeartbeatOwnerMismatchLosesLeadership(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = 5 * time.Millisecond
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}

	now := time.Now()
	replacement := leaderLease{
		SchemaVersion: leaderLeaseSchemaVersion,
		OwnerToken:    strings.Repeat("b", 64),
		Generation:    99,
		PID:           os.Getpid(),
		AcquiredAt:    now.UnixMilli(),
		HeartbeatAt:   now.UnixMilli(),
	}
	writeLeaseUnderGuard(t, lock, replacement)
	waitForCondition(t, time.Second, func() bool { return !lock.isHolder() })

	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release() error = %v", err)
	}
	lease, exists, err := lock.readLease()
	if err != nil || !exists {
		t.Fatalf("replacement lease read = %v, %v", exists, err)
	}
	if lease.OwnerToken != replacement.OwnerToken || lease.Generation != replacement.Generation {
		t.Fatal("release removed or changed another instance's lease")
	}
}

func TestLeaderLockHeartbeatReadFailureLosesLeadership(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = 5 * time.Millisecond
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}

	if err := secureio.WithRuntimeFileLock(lock.guardPath, func() error {
		if err := os.Remove(lock.path); err != nil {
			return err
		}
		source := lock.path + ".linked"
		if err := os.WriteFile(source, []byte("invalid"), 0o600); err != nil {
			return err
		}
		return os.Link(source, lock.path)
	}); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, time.Second, func() bool { return !lock.isHolder() })
}

func TestLeaderLockHeartbeatWriteFailureLosesLeadership(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = 5 * time.Millisecond
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}

	// Moving the clock behind AcquiredAt makes the strict writer reject the
	// heartbeat. The leader must demote itself on that write failure.
	lock.mu.Lock()
	lock.clock = func() time.Time { return time.UnixMilli(1) }
	lock.mu.Unlock()
	waitForCondition(t, time.Second, func() bool { return !lock.isHolder() })
}

func TestLeaderLockTakesOverStaleLease(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = time.Hour
	lock.staleAfter = 10 * time.Millisecond
	now := time.Now()
	stale := leaderLease{
		SchemaVersion: leaderLeaseSchemaVersion,
		OwnerToken:    strings.Repeat("c", 64),
		Generation:    7,
		PID:           os.Getpid(),
		AcquiredAt:    now.Add(-time.Minute).UnixMilli(),
		HeartbeatAt:   now.Add(-time.Minute).UnixMilli(),
	}
	writeLeaseUnderGuard(t, lock, stale)

	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() stale lease = %v, %v", acquired, err)
	}
	lease, exists, err := lock.readLease()
	if err != nil || !exists {
		t.Fatalf("current lease read = %v, %v", exists, err)
	}
	if lease.OwnerToken != lock.owner {
		t.Fatal("stale lease was not replaced by this instance")
	}
	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release() error = %v", err)
	}
}

func TestLeaderLockReleaseDeletesOnlyItsGeneration(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = time.Hour
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}

	now := time.Now()
	replacement := leaderLease{
		SchemaVersion: leaderLeaseSchemaVersion,
		OwnerToken:    lock.owner,
		Generation:    lock.activeGeneration + 1,
		PID:           lock.pid,
		AcquiredAt:    now.UnixMilli(),
		HeartbeatAt:   now.UnixMilli(),
	}
	writeLeaseUnderGuard(t, lock, replacement)
	if err := lock.release(context.Background()); err != nil {
		t.Fatalf("release() error = %v", err)
	}
	lease, exists, err := lock.readLease()
	if err != nil || !exists {
		t.Fatalf("newer lease read = %v, %v", exists, err)
	}
	if lease.Generation != replacement.Generation {
		t.Fatal("release deleted another generation belonging to the same instance")
	}
}

func TestLeaderLockReleaseHonorsContextWithoutLeavingHolder(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	lock.heartbeatEvery = time.Hour
	acquired, err := lock.tryAcquire(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryAcquire() = %v, %v", acquired, err)
	}

	guardHeld := make(chan struct{})
	releaseGuard := make(chan struct{})
	guardDone := make(chan error, 1)
	go func() {
		guardDone <- secureio.WithRuntimeFileLock(lock.guardPath, func() error {
			close(guardHeld)
			<-releaseGuard
			return nil
		})
	}()
	<-guardHeld

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = lock.release(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("release() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("release() blocked for %v", elapsed)
	}
	if lock.isHolder() {
		t.Fatal("release timeout left holder=true")
	}
	close(releaseGuard)
	if err := <-guardDone; err != nil {
		t.Fatal(err)
	}
}

func TestLeaderOwnerTokenRandomFailureIsReturned(t *testing.T) {
	want := errors.New("random source unavailable")
	if _, err := newLeaderOwnerToken(errorReader{err: want}); !errors.Is(err, want) {
		t.Fatalf("newLeaderOwnerToken() error = %v, want %v", err, want)
	}
}

func TestLeaderLockProbeInterval(t *testing.T) {
	lock := mustNewLeaderLock(t, filepath.Join(t.TempDir(), "leader.json"))
	if got := lock.probeInterval(); got != defaultLeaderProbeInterval {
		t.Fatalf("probeInterval() = %v", got)
	}
	lock.probeEvery = 17 * time.Millisecond
	if got := lock.probeInterval(); got != 17*time.Millisecond {
		t.Fatalf("custom probeInterval() = %v", got)
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func mustNewLeaderLock(t *testing.T, path string) *leaderLock {
	t.Helper()
	lock, err := newLeaderLock(path)
	if err != nil {
		t.Fatalf("newLeaderLock() error = %v", err)
	}
	return lock
}

func assertPermissions(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func writeLeaseUnderGuard(t *testing.T, lock *leaderLock, lease leaderLease) {
	t.Helper()
	if err := secureio.WithRuntimeFileLock(lock.guardPath, func() error {
		return lock.writeLease(lease)
	}); err != nil {
		t.Fatalf("write replacement lease: %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not satisfied before timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

var _ io.Reader = errorReader{}
