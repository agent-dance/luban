package pierbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"golang.org/x/sys/unix"
)

const registryGateSchemaVersion = "agentic-bench/registry-gate-v1"

type registryGateState struct {
	SchemaVersion        string    `json:"schema_version"`
	CooldownUntil        time.Time `json:"cooldown_until"`
	ConsecutiveThrottles int       `json:"consecutive_throttles"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type registryGateLease struct {
	file      *os.File
	statePath string
	state     registryGateState
	closed    bool
}

func sharedRegistryGatePath(config Config) string {
	if config.RegistryGatePath != "" {
		return config.RegistryGatePath
	}
	return filepath.Join(os.TempDir(), "agentic-bench-public-ecr")
}

func acquireRegistryGate(ctx context.Context, basePath string) (*registryGateLease, error) {
	if !filepath.IsAbs(basePath) {
		return nil, errors.New("registry coordination path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(basePath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			file.Close()
			return nil, err
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	lease := &registryGateLease{file: file, statePath: basePath + ".json"}
	lease.state, err = loadRegistryGateState(lease.statePath)
	if err != nil {
		_ = lease.unlock()
		return nil, err
	}
	for delay := time.Until(lease.state.CooldownUntil); delay > 0; delay = time.Until(lease.state.CooldownUntil) {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			_ = lease.unlock()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return lease, nil
}

func loadRegistryGateState(path string) (registryGateState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registryGateState{SchemaVersion: registryGateSchemaVersion}, nil
	}
	if err != nil {
		return registryGateState{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var state registryGateState
	if err := decoder.Decode(&state); err != nil {
		return registryGateState{}, fmt.Errorf("decode registry coordination state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return registryGateState{}, errors.New("registry coordination state has trailing JSON")
	}
	if state.SchemaVersion != registryGateSchemaVersion || state.ConsecutiveThrottles < 0 || state.ConsecutiveThrottles > 30 {
		return registryGateState{}, errors.New("registry coordination state is invalid")
	}
	return state, nil
}

func (lease *registryGateLease) finish(success, throttled bool, retryAfter string) error {
	if lease == nil || lease.closed {
		return errors.New("registry coordination lease is not active")
	}
	now := time.Now().UTC()
	if throttled {
		lease.state.ConsecutiveThrottles++
		exponent := min(lease.state.ConsecutiveThrottles-1, 5)
		delay := 30 * time.Second * time.Duration(1<<exponent)
		if delay > 15*time.Minute {
			delay = 15 * time.Minute
		}
		if advertised := parseRetryAfter(retryAfter, now); advertised > delay {
			delay = advertised
		}
		lease.state.CooldownUntil = now.Add(delay)
	} else if success {
		if lease.state.ConsecutiveThrottles > 0 {
			lease.state.ConsecutiveThrottles--
		}
		// Pace anonymous manifest requests even when the registry is healthy.
		lease.state.CooldownUntil = now.Add(time.Second)
	}
	lease.state.SchemaVersion = registryGateSchemaVersion
	lease.state.UpdatedAt = now
	writeErr := harness.WriteJSONAtomic(lease.statePath, lease.state, 0o600)
	unlockErr := lease.unlock()
	return errors.Join(writeErr, unlockErr)
}

func (lease *registryGateLease) unlock() error {
	if lease == nil || lease.closed {
		return nil
	}
	lease.closed = true
	unlockErr := unix.Flock(int(lease.file.Fd()), unix.LOCK_UN)
	closeErr := lease.file.Close()
	return errors.Join(unlockErr, closeErr)
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if instant, err := http.ParseTime(value); err == nil && instant.After(now) {
		return instant.Sub(now)
	}
	return 0
}

func registryThrottleEvidence(values ...[]byte) bool {
	joined := bytes.ToLower(bytes.Join(values, []byte{'\n'}))
	for _, marker := range [][]byte{[]byte("toomanyrequests"), []byte("too many requests"), []byte("status 429"), []byte("http 429"), []byte("throttl")} {
		if bytes.Contains(joined, marker) {
			return true
		}
	}
	return false
}
