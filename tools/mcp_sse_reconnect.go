package tools

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// mcp_sse_reconnect.go — MCP-01 SSE reconnect with exponential backoff +
// jitter.
//
// When an SSE stream closes non-cleanly the agent must re-attempt the
// connection with progressively longer cooldowns so a transient blip
// doesn't permanently kill long-lived MCP server sessions. We surface a
// `mcp.connection_lost` event (via the registered listener) after N
// consecutive failures so the model can be told a server is unreachable.
//
// Backoff sequence: 1s, 2s, 4s, 8s, 16s, capped at 30s. Each delay has
// ±20% jitter applied to avoid thundering-herd.

// SSEBackoffSchedule returns the cooldown for the Nth attempt
// (1-indexed). The exponential ramp doubles each attempt, capped at 30s.
// Jitter is applied independently per call so two concurrent reconnects
// with the same attempt count don't collide.
var sseBackoffMaxCooldown = 30 * time.Second

func sseBackoffBase(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := time.Second
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= sseBackoffMaxCooldown {
			return sseBackoffMaxCooldown
		}
	}
	if d > sseBackoffMaxCooldown {
		d = sseBackoffMaxCooldown
	}
	return d
}

// SSEBackoffWithJitter returns the base cooldown for `attempt` plus a
// pseudo-random jitter of ±20%.
func SSEBackoffWithJitter(attempt int) time.Duration {
	base := sseBackoffBase(attempt)
	if base == 0 {
		return 0
	}
	// ±20% jitter. rand.Float64 returns [0.0, 1.0); shift to [-0.2, 0.2].
	jitter := (rand.Float64() - 0.5) * 0.4
	delta := time.Duration(float64(base) * jitter)
	return base + delta
}

// SSEConnectionLossListener is invoked when the reconnect loop has
// exhausted its retry budget. The serverName argument identifies which
// MCP server is unreachable.
type SSEConnectionLossListener func(serverName string)

var (
	sseLossMu       sync.RWMutex
	sseLossListener SSEConnectionLossListener
	// connectionLostThreshold is the number of consecutive reconnect
	// failures we tolerate before raising mcp.connection_lost.
	sseConnectionLostThreshold = 5
)

// SetSSEConnectionLossListener installs the global callback for
// mcp.connection_lost. Pass nil to clear.
func SetSSEConnectionLossListener(fn SSEConnectionLossListener) {
	sseLossMu.Lock()
	sseLossListener = fn
	sseLossMu.Unlock()
}

// SetSSEConnectionLostThreshold overrides the consecutive-failure count
// before mcp.connection_lost is raised. Defaults to 5.
func SetSSEConnectionLostThreshold(n int) {
	if n <= 0 {
		return
	}
	sseLossMu.Lock()
	sseConnectionLostThreshold = n
	sseLossMu.Unlock()
}

func sseLossThreshold() int {
	sseLossMu.RLock()
	defer sseLossMu.RUnlock()
	return sseConnectionLostThreshold
}

func emitSSEConnectionLost(serverName string) {
	sseLossMu.RLock()
	fn := sseLossListener
	sseLossMu.RUnlock()
	if fn == nil {
		return
	}
	defer func() { _ = recover() }()
	fn(serverName)
}

// SSEReconnectAttempts tracks per-server reconnect failure counts. The
// counter resets on a successful reconnect; once it reaches the
// connection-lost threshold the listener fires and the entry transitions
// to "failed" state.
type SSEReconnectAttempts struct {
	mu    sync.Mutex
	state map[string]*sseReconnectEntry
}

type sseReconnectEntry struct {
	consecutive atomic.Int32
	failed      atomic.Bool
	lastAttempt atomic.Int64
}

// SSEReconnectSnapshot is a copy-returning view of one server's SSE reconnect
// state. It is intentionally small so diagnostics can expose retry visibility
// without sharing mutable counters.
type SSEReconnectSnapshot struct {
	ServerName          string
	ConsecutiveFailures int
	Failed              bool
	LastAttempt         time.Time
}

// NewSSEReconnectAttempts constructs a fresh tracker.
func NewSSEReconnectAttempts() *SSEReconnectAttempts {
	return &SSEReconnectAttempts{state: make(map[string]*sseReconnectEntry)}
}

func (s *SSEReconnectAttempts) entry(name string) *sseReconnectEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.state[name]; ok {
		return e
	}
	e := &sseReconnectEntry{}
	s.state[name] = e
	return e
}

// Record bumps the failure counter for `serverName` and returns the new
// total. When the count crosses the configured threshold, the
// connection-lost listener is fired and the entry is marked as failed.
func (s *SSEReconnectAttempts) Record(serverName string) int {
	e := s.entry(serverName)
	e.lastAttempt.Store(time.Now().UnixNano())
	count := int(e.consecutive.Add(1))
	if count >= sseLossThreshold() && !e.failed.Swap(true) {
		emitSSEConnectionLost(serverName)
	}
	return count
}

// Reset zeroes the failure counter on a successful reconnect.
func (s *SSEReconnectAttempts) Reset(serverName string) {
	e := s.entry(serverName)
	e.consecutive.Store(0)
	e.failed.Store(false)
}

// Failed reports whether the given server has crossed the connection-lost
// threshold without a successful reconnect.
func (s *SSEReconnectAttempts) Failed(serverName string) bool {
	e := s.entry(serverName)
	return e.failed.Load()
}

// Snapshot returns the retry state for one server.
func (s *SSEReconnectAttempts) Snapshot(serverName string) SSEReconnectSnapshot {
	if s == nil {
		return SSEReconnectSnapshot{ServerName: serverName}
	}
	e := s.entry(serverName)
	var lastAttempt time.Time
	if nanos := e.lastAttempt.Load(); nanos > 0 {
		lastAttempt = time.Unix(0, nanos)
	}
	return SSEReconnectSnapshot{
		ServerName:          serverName,
		ConsecutiveFailures: int(e.consecutive.Load()),
		Failed:              e.failed.Load(),
		LastAttempt:         lastAttempt,
	}
}

// ReconnectWithBackoff repeatedly invokes `attempt` until it returns nil
// or the context is done. Each failure increments the per-server
// failure tracker; on the threshold the connection-lost listener fires
// and the loop returns the last error wrapped in errSSEConnectionLost.
//
// The maxAttempts parameter caps the loop; pass 0 for "loop until ctx
// expires or threshold trips."
func ReconnectWithBackoff(
	ctx context.Context,
	tracker *SSEReconnectAttempts,
	serverName string,
	maxAttempts int,
	attempt func(ctx context.Context) error,
) error {
	if attempt == nil {
		return errors.New("ReconnectWithBackoff: nil attempt fn")
	}
	if tracker == nil {
		tracker = NewSSEReconnectAttempts()
	}
	var lastErr error
	for try := 1; ; try++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := attempt(ctx)
		if err == nil {
			tracker.Reset(serverName)
			return nil
		}
		lastErr = err
		count := tracker.Record(serverName)
		if maxAttempts > 0 && try >= maxAttempts {
			return err
		}
		if count >= sseLossThreshold() {
			return errSSEConnectionLost(serverName, lastErr)
		}
		cooldown := SSEBackoffWithJitter(try)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cooldown):
		}
	}
}

// errSSEConnectionLost wraps the underlying error with a structured
// signal that the reconnect budget is exhausted.
type sseConnectionLost struct {
	server string
	cause  error
}

func (e *sseConnectionLost) Error() string {
	if e == nil {
		return "mcp.connection_lost"
	}
	if e.cause == nil {
		return "mcp.connection_lost: " + e.server
	}
	return "mcp.connection_lost: " + e.server + ": " + e.cause.Error()
}

func (e *sseConnectionLost) Unwrap() error { return e.cause }

func errSSEConnectionLost(server string, cause error) error {
	return &sseConnectionLost{server: server, cause: cause}
}
