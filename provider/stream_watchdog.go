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

// Responses streams are intentionally not governed by a request-wide timeout.
// Match Codex CLI's stream_idle_timeout_ms policy: a complete SSE event or
// WebSocket frame is stream activity, and five minutes without such activity
// means the connection is lost. Raw socket bytes and SSE comments do not reset
// the deadline because they are not complete stream events.
const (
	defaultResponsesInitialIdleTimeout = 5 * time.Minute
	defaultResponsesActiveIdleTimeout  = 5 * time.Minute
	maxResponsesInitialIdleTimeout     = 60 * time.Minute
	maxResponsesActiveIdleTimeout      = 60 * time.Minute

	responsesStreamIdleTimeoutEnv  = "LUBAN_CODE_RESPONSES_STREAM_IDLE_TIMEOUT_MS"
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
	if strings.TrimSpace(os.Getenv(responsesStreamIdleTimeoutEnv)) != "" {
		idle := boundedPositiveDurationFromEnv(
			responsesStreamIdleTimeoutEnv,
			defaultResponsesInitialIdleTimeout,
			maxResponsesInitialIdleTimeout,
		)
		return streamWatchdogConfig{initialIdle: idle, activeIdle: idle}
	}
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
// produced no complete stream event for its deadline. It is distinct from
// context cancellation and from a request-wide deadline, and is safe for the
// query loop to replay from its last committed response boundary.
type StreamIdleTimeoutError struct {
	Phase   streamWatchdogPhase
	IdleFor time.Duration
}

func (e *StreamIdleTimeoutError) Error() string {
	if e == nil {
		return ""
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderStreamIdleTimeout, e.IdleFor)
}

func (e *StreamIdleTimeoutError) Timeout() bool   { return true }
func (e *StreamIdleTimeoutError) Temporary() bool { return true }

// streamWatchdogBody closes the underlying response body when its stream-
// activity deadline expires. Closing Response.Body is what unblocks a pending
// network Read; the wrapper then replaces the transport's incidental close
// error with a stable typed disposition. Read deliberately does not renew the
// deadline: only the Responses event parser can prove a complete event.
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

// markActivity switches to the active phase on the first complete stream event
// and renews the deadline for every later event.
func (w *streamWatchdogBody) markActivity() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.timedOut != nil {
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
		// A stream event may have reset the deadline while this callback was
		// already scheduled. Re-check the monotonic deadline before declaring a
		// stall.
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
