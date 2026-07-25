package schedule

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agent-dance/luban/internal/store/secureio"
)

const (
	leaderLeaseSchemaVersion       = 1
	leaderOwnerTokenBytes          = 32
	maxLeaderLeaseBytes            = 4 << 10
	defaultLeaderProbeInterval     = 5 * time.Second
	defaultLeaderHeartbeatInterval = 5 * time.Second
	defaultLeaderStaleAfter        = 10 * time.Second
)

type leaderLease struct {
	SchemaVersion int    `json:"schema_version"`
	OwnerToken    string `json:"owner_token"`
	Generation    uint64 `json:"generation"`
	PID           int    `json:"pid"`
	AcquiredAt    int64  `json:"acquired_at"`
	HeartbeatAt   int64  `json:"heartbeat_at"`
}

type leaderLock struct {
	path      string
	guardPath string
	owner     string
	pid       int

	operation chan struct{}

	mu               sync.Mutex
	holder           bool
	generation       uint64
	activeGeneration uint64
	releaseEpoch     uint64
	heartbeatCancel  context.CancelFunc
	heartbeatDone    chan struct{}

	clock          func() time.Time
	probeEvery     time.Duration
	heartbeatEvery time.Duration
	staleAfter     time.Duration
}

type leaderGuardResult struct {
	value any
	err   error
}

type leaderAcquireResult struct {
	acquired       bool
	writeAttempted bool
}

func newLeaderLock(path string) (*leaderLock, error) {
	owner, err := newLeaderOwnerToken(rand.Reader)
	if err != nil {
		return nil, err
	}
	cleaned := filepath.Clean(path)
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(cleaned)); err != nil {
		return nil, err
	}
	if _, err := secureio.TightenPrivateRuntimeRegularFile(cleaned, true); err != nil {
		return nil, err
	}
	guardPath := cleaned + ".guard"
	if err := secureio.PreparePrivateRuntimeLock(guardPath); err != nil {
		return nil, err
	}
	lock := &leaderLock{
		path:           cleaned,
		guardPath:      guardPath,
		owner:          owner,
		pid:            os.Getpid(),
		operation:      make(chan struct{}, 1),
		clock:          time.Now,
		probeEvery:     defaultLeaderProbeInterval,
		heartbeatEvery: defaultLeaderHeartbeatInterval,
		staleAfter:     defaultLeaderStaleAfter,
	}
	lock.operation <- struct{}{}
	return lock, nil
}

func newLeaderOwnerToken(reader io.Reader) (string, error) {
	buffer := make([]byte, leaderOwnerTokenBytes)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func (l *leaderLock) probeInterval() time.Duration {
	if l == nil || l.probeEvery <= 0 {
		return defaultLeaderProbeInterval
	}
	return l.probeEvery
}

func (l *leaderLock) isHolder() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.holder
}

func (l *leaderLock) tryAcquire(ctx context.Context) (bool, error) {
	if l == nil {
		return false, fs.ErrInvalid
	}
	if err := l.beginOperation(ctx); err != nil {
		return false, err
	}
	defer l.endOperation()

	l.mu.Lock()
	if l.holder {
		l.mu.Unlock()
		return true, nil
	}
	l.generation++
	generation := l.generation
	releaseEpoch := l.releaseEpoch
	now := l.clock()
	l.mu.Unlock()

	resultChannel, err := l.startGuarded(ctx, func() (any, error) {
		if err := ctx.Err(); err != nil {
			return leaderAcquireResult{}, err
		}
		lease, exists, err := l.readLease()
		if err != nil {
			return leaderAcquireResult{}, err
		}
		if exists && l.leaseIsFresh(lease, now) && lease.OwnerToken != l.owner {
			return leaderAcquireResult{}, nil
		}
		if err := ctx.Err(); err != nil {
			return leaderAcquireResult{}, err
		}
		candidate := leaderLease{
			SchemaVersion: leaderLeaseSchemaVersion,
			OwnerToken:    l.owner,
			Generation:    generation,
			PID:           l.pid,
			AcquiredAt:    now.UnixMilli(),
			HeartbeatAt:   now.UnixMilli(),
		}
		outcome := leaderAcquireResult{writeAttempted: true}
		if err := l.writeLease(candidate); err != nil {
			return outcome, err
		}
		outcome.acquired = true
		return outcome, nil
	})
	if err != nil {
		return false, err
	}

	select {
	case <-ctx.Done():
		go l.cleanupInterruptedAcquire(resultChannel, generation)
		return false, ctx.Err()
	case result := <-resultChannel:
		outcome, ok := result.value.(leaderAcquireResult)
		if !ok {
			if result.err != nil {
				return false, result.err
			}
			return false, fs.ErrInvalid
		}
		if result.err != nil {
			if outcome.writeAttempted {
				cleanupErr := l.removeOwnedLease(context.Background(), generation)
				return false, errors.Join(result.err, cleanupErr)
			}
			return false, result.err
		}
		if !outcome.acquired {
			return false, nil
		}

		l.mu.Lock()
		if l.releaseEpoch != releaseEpoch {
			l.mu.Unlock()
			cleanupErr := l.removeOwnedLease(context.Background(), generation)
			return false, cleanupErr
		}
		heartbeatContext, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		l.holder = true
		l.activeGeneration = generation
		l.heartbeatCancel = cancel
		l.heartbeatDone = done
		interval := l.heartbeatEvery
		if interval <= 0 {
			interval = defaultLeaderHeartbeatInterval
		}
		l.mu.Unlock()

		go l.heartbeatLoop(heartbeatContext, generation, interval, done)
		return true, nil
	}
}

func (l *leaderLock) cleanupInterruptedAcquire(resultChannel <-chan leaderGuardResult, generation uint64) {
	result := <-resultChannel
	outcome, ok := result.value.(leaderAcquireResult)
	if ok && outcome.writeAttempted {
		_ = l.removeOwnedLease(context.Background(), generation)
	}
}

func (l *leaderLock) release(ctx context.Context) error {
	if l == nil {
		return nil
	}

	// Demote synchronously. Even when the caller's context is already done,
	// this instance must never continue reporting itself as the leader.
	l.mu.Lock()
	l.releaseEpoch++
	generation := l.activeGeneration
	done := l.heartbeatDone
	cancel := l.heartbeatCancel
	l.holder = false
	l.activeGeneration = 0
	l.heartbeatCancel = nil
	l.heartbeatDone = nil
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := l.beginOperation(ctx); err != nil {
		return err
	}
	defer l.endOperation()
	if generation == 0 {
		return nil
	}
	return l.removeOwnedLease(ctx, generation)
}

func (l *leaderLock) heartbeatLoop(ctx context.Context, generation uint64, interval time.Duration, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			owned, err := l.refreshLease(ctx, generation)
			if err != nil || !owned {
				l.loseLeadership(generation)
				return
			}
		}
	}
}

func (l *leaderLock) refreshLease(ctx context.Context, generation uint64) (bool, error) {
	resultChannel, err := l.startGuarded(ctx, func() (any, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		lease, exists, err := l.readLease()
		if err != nil {
			return false, err
		}
		if !exists || lease.OwnerToken != l.owner || lease.Generation != generation {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		l.mu.Lock()
		now := l.clock()
		active := l.holder && l.activeGeneration == generation
		l.mu.Unlock()
		if !active {
			return false, nil
		}
		lease.HeartbeatAt = now.UnixMilli()
		if err := l.writeLease(lease); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return false, err
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case result := <-resultChannel:
		if result.err != nil {
			return false, result.err
		}
		owned, ok := result.value.(bool)
		if !ok {
			return false, fs.ErrInvalid
		}
		return owned, nil
	}
}

func (l *leaderLock) loseLeadership(generation uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.activeGeneration != generation {
		return
	}
	l.holder = false
	if l.heartbeatCancel != nil {
		l.heartbeatCancel()
	}
}

func (l *leaderLock) removeOwnedLease(ctx context.Context, generation uint64) error {
	resultChannel, err := l.startGuarded(ctx, func() (any, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		lease, exists, err := l.readLease()
		if err != nil {
			return false, err
		}
		if !exists || lease.OwnerToken != l.owner || lease.Generation != generation {
			return false, nil
		}
		if _, err := secureio.TightenPrivateRuntimeRegularFile(l.path, false); err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := os.Remove(l.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		if err := secureio.SyncRuntimeDirectory(filepath.Dir(l.path)); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-resultChannel:
		return result.err
	}
}

func (l *leaderLock) startGuarded(ctx context.Context, fn func() (any, error)) (<-chan leaderGuardResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := secureio.PreparePrivateRuntimeLock(l.guardPath); err != nil {
		return nil, err
	}
	resultChannel := make(chan leaderGuardResult, 1)
	go func() {
		value, err := secureio.WithRuntimeFileLockResult(l.guardPath, func() (any, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return fn()
		})
		resultChannel <- leaderGuardResult{value: value, err: err}
	}()
	return resultChannel, nil
}

func (l *leaderLock) beginOperation(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.operation:
		return nil
	}
}

func (l *leaderLock) endOperation() {
	l.operation <- struct{}{}
}

func (l *leaderLock) leaseIsFresh(lease leaderLease, now time.Time) bool {
	staleAfter := l.staleAfter
	if staleAfter <= 0 {
		staleAfter = defaultLeaderStaleAfter
	}
	return now.Sub(time.UnixMilli(lease.HeartbeatAt)) < staleAfter
}

func (l *leaderLock) readLease() (leaderLease, bool, error) {
	file, err := secureio.OpenPrivateRuntimeRegularFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return leaderLease{}, false, nil
	}
	if err != nil {
		return leaderLease{}, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLeaderLeaseBytes+1))
	if err != nil {
		return leaderLease{}, false, err
	}
	if len(data) > maxLeaderLeaseBytes {
		return leaderLease{}, false, fs.ErrInvalid
	}
	if err := rejectDuplicateLeaderLeaseFields(data); err != nil {
		return leaderLease{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var lease leaderLease
	if err := decoder.Decode(&lease); err != nil {
		return leaderLease{}, false, err
	}
	if err := rejectTrailingLeaderJSON(decoder); err != nil {
		return leaderLease{}, false, err
	}
	if err := validateLeaderLease(lease); err != nil {
		return leaderLease{}, false, err
	}
	return lease, true, nil
}

func rejectDuplicateLeaderLeaseFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return fs.ErrInvalid
	}
	seen := make(map[string]struct{}, 6)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fs.ErrInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return fs.ErrInvalid
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return fs.ErrInvalid
	}
	return rejectTrailingLeaderJSON(decoder)
}

func rejectTrailingLeaderJSON(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fs.ErrInvalid
	}
	return err
}

func (l *leaderLock) writeLease(lease leaderLease) error {
	if err := validateLeaderLease(lease); err != nil {
		return err
	}
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	return secureio.AtomicWritePrivateRuntimeFile(l.path, append(data, '\n'))
}

func validateLeaderLease(lease leaderLease) error {
	if lease.SchemaVersion != leaderLeaseSchemaVersion ||
		lease.Generation == 0 ||
		lease.PID <= 0 ||
		lease.AcquiredAt <= 0 ||
		lease.HeartbeatAt < lease.AcquiredAt ||
		len(lease.OwnerToken) != leaderOwnerTokenBytes*2 {
		return fs.ErrInvalid
	}
	decoded, err := hex.DecodeString(lease.OwnerToken)
	if err != nil || len(decoded) != leaderOwnerTokenBytes {
		return fs.ErrInvalid
	}
	return nil
}
