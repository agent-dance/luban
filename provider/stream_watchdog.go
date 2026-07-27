package provider

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// Responses streams are intentionally not governed by a request-wide timeout:
// an xhigh reasoning request can remain healthy for a long time as long as the
// server keeps the SSE transport alive. The watchdog below limits only periods
// with no bytes at all. SSE comments and future event types therefore count as
// transport progress even when they do not produce model-visible deltas.
const (
	defaultResponsesInitialIdleTimeout = 4 * time.Minute
	defaultResponsesActiveIdleTimeout  = 90 * time.Second
	maxResponsesInitialIdleTimeout     = 5 * time.Minute
	maxResponsesActiveIdleTimeout      = 2 * time.Minute

	responsesInitialIdleTimeoutEnv = "LUBAN_CODE_RESPONSES_INITIAL_IDLE_TIMEOUT_MS"
	responsesActiveIdleTimeoutEnv  = "LUBAN_CODE_RESPONSES_ACTIVE_IDLE_TIMEOUT_MS"
)

type streamWatchdogPhase string

const (
	streamWatchdogAwaitingOutput streamWatchdogPhase = "awaiting_output"
	streamWatchdogActive         streamWatchdogPhase = "active_output"
)

type streamWatchdogConfig struct {
	initialIdle time.Duration
	activeIdle  time.Duration
}

func responsesStreamWatchdogConfig() streamWatchdogConfig {
	return streamWatchdogConfig{
		initialIdle: boundedPositiveDurationFromEnv(
			responsesInitialIdleTimeoutEnv,
			defaultResponsesInitialIdleTimeout,
			maxResponsesInitialIdleTimeout,
		),
		activeIdle: boundedPositiveDurationFromEnv(
			responsesActiveIdleTimeoutEnv,
			defaultResponsesActiveIdleTimeout,
			maxResponsesActiveIdleTimeout,
		),
	}
}

func boundedPositiveDurationFromEnv(name string, fallback, upperBound time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	milliseconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || milliseconds <= 0 {
		return fallback
	}
	if milliseconds > upperBound.Milliseconds() {
		return upperBound
	}
	return time.Duration(milliseconds) * time.Millisecond
}

// StreamIdleTimeoutError is returned only when a live streaming response has
// made no byte-level progress for its phase-specific deadline. It is distinct
// from context cancellation and from a request-wide deadline, and is safe for
// the query loop to replay from its last committed response boundary.
type StreamIdleTimeoutError struct {
	Phase   streamWatchdogPhase
	IdleFor time.Duration
}

func (e *StreamIdleTimeoutError) Error() string {
	if e == nil {
		return ""
	}
	key := i18n.KeyProviderStreamInitialIdleTimeout
	if e.Phase == streamWatchdogActive {
		key = i18n.KeyProviderStreamActiveIdleTimeout
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, e.IdleFor)
}

func (e *StreamIdleTimeoutError) Timeout() bool   { return true }
func (e *StreamIdleTimeoutError) Temporary() bool { return true }

// streamWatchdogBody closes the underlying response body when its byte-progress
// deadline expires. Closing Response.Body is what unblocks a pending network
// Read; the wrapper then replaces the transport's incidental close error with a
// stable typed disposition.
type streamWatchdogBody struct {
	body   io.ReadCloser
	config streamWatchdogConfig

	mu       sync.Mutex
	phase    streamWatchdogPhase
	deadline time.Time
	timer    *time.Timer
	closed   bool
	timedOut *StreamIdleTimeoutError
}

func newStreamWatchdogBody(body io.ReadCloser, config streamWatchdogConfig) *streamWatchdogBody {
	config = normalizeStreamWatchdogConfig(config)
	w := &streamWatchdogBody{
		body:     body,
		config:   config,
		phase:    streamWatchdogAwaitingOutput,
		deadline: time.Now().Add(config.initialIdle),
	}
	w.timer = time.AfterFunc(config.initialIdle, w.fire)
	return w
}

func normalizeStreamWatchdogConfig(config streamWatchdogConfig) streamWatchdogConfig {
	if config.initialIdle <= 0 || config.initialIdle > maxResponsesInitialIdleTimeout {
		config.initialIdle = minPositiveDuration(config.initialIdle, defaultResponsesInitialIdleTimeout, maxResponsesInitialIdleTimeout)
	}
	if config.activeIdle <= 0 || config.activeIdle > maxResponsesActiveIdleTimeout {
		config.activeIdle = minPositiveDuration(config.activeIdle, defaultResponsesActiveIdleTimeout, maxResponsesActiveIdleTimeout)
	}
	return config
}

func minPositiveDuration(value, fallback, upperBound time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	if value > upperBound {
		return upperBound
	}
	return value
}

func (w *streamWatchdogBody) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)

	w.mu.Lock()
	if n > 0 && w.timedOut == nil && !w.closed {
		w.resetDeadlineLocked(time.Now())
	}
	timeoutErr := w.timedOut
	w.mu.Unlock()

	if timeoutErr != nil && (err != nil || n == 0) {
		return n, timeoutErr
	}
	return n, err
}

func (w *streamWatchdogBody) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	return w.body.Close()
}

// markOutputActive switches to the shorter post-output idle budget. It is
// called only for non-empty content/reasoning/tool-argument deltas, not for
// response.created or a reasoning item shell that may precede long xhigh work.
func (w *streamWatchdogBody) markOutputActive() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.timedOut != nil || w.phase == streamWatchdogActive {
		return
	}
	w.phase = streamWatchdogActive
	w.resetDeadlineLocked(time.Now())
}

func (w *streamWatchdogBody) currentTimeoutLocked() time.Duration {
	if w.phase == streamWatchdogActive {
		return w.config.activeIdle
	}
	return w.config.initialIdle
}

func (w *streamWatchdogBody) resetDeadlineLocked(now time.Time) {
	delay := w.currentTimeoutLocked()
	w.deadline = now.Add(delay)
	if w.timer != nil {
		w.timer.Stop()
		w.timer.Reset(delay)
	}
}

func (w *streamWatchdogBody) fire() {
	w.mu.Lock()
	if w.closed || w.timedOut != nil {
		w.mu.Unlock()
		return
	}
	now := time.Now()
	if remaining := w.deadline.Sub(now); remaining > 0 {
		// A Read may have reset the deadline while this callback was already
		// scheduled. Re-check the monotonic deadline before declaring a stall.
		w.timer.Reset(remaining)
		w.mu.Unlock()
		return
	}
	w.timedOut = &StreamIdleTimeoutError{
		Phase:   w.phase,
		IdleFor: w.currentTimeoutLocked(),
	}
	w.mu.Unlock()

	// net/http documents that closing Response.Body unblocks a concurrent Read.
	// The Read path above translates the resulting implementation-specific error
	// back into StreamIdleTimeoutError.
	_ = w.body.Close()
}

func streamIdleTimeoutFromError(err error) (*StreamIdleTimeoutError, bool) {
	var idle *StreamIdleTimeoutError
	ok := errors.As(err, &idle)
	return idle, ok
}
