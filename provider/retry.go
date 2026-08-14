package provider

import (
	"context"
	"time"

	"github.com/agent-dance/luban/types"
)

// RetryConfig controls retry behaviour.
type RetryConfig struct {
	// MaxAttempts is the total HTTP request-attempt budget used before a
	// response stream is established. The default is five (the initial request
	// plus four retries), matching Codex CLI's request_max_retries default.
	MaxAttempts int
	// StreamMaxAttempts is the independent response-stream budget. The default
	// is six (the initial stream plus five reconnects), matching Codex CLI's
	// stream_max_retries default. Keeping the budgets separate prevents a few
	// connection failures from consuming all stream-recovery capacity.
	StreamMaxAttempts int
	BaseDelay         time.Duration // default 200 ms
	MaxDelay          time.Duration // default 3.2 s for the bounded default budgets
	// OnRetry is called before each retry attempt. nil = no logging.
	// attempt is the one-based failed raw attempt that triggered the retry.
	OnRetry func(attempt, maxRetries int, delay time.Duration, err error)
	// OnAuthError is called when a 401 Unauthorized error is encountered.
	// If it returns true, credentials were refreshed and one retry is allowed.
	// If nil or it returns false, the 401 is surfaced immediately (fail fast).
	OnAuthError func() bool

	// Test seams kept private so callers cannot accidentally disable jitter or
	// replace the monotonic wall-clock policy in production.
	jitter func(time.Duration) time.Duration
	now    func() time.Time
}

// DefaultRetryConfig returns the interactive LLM retry policy.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       defaultRequestMaxAttempts,
		StreamMaxAttempts: defaultStreamMaxAttempts,
		BaseDelay:         200 * time.Millisecond,
		MaxDelay:          3200 * time.Millisecond,
	}
}

// RetryProvider is a decorator that wraps any Provider and adds retry logic.
// It implements the Provider interface and is transparent to callers.
type RetryProvider struct {
	inner  Provider
	config RetryConfig
}

// Close releases a persistent transport owned by the wrapped provider.
func (r *RetryProvider) Close() error {
	if closer, ok := r.inner.(CloseProvider); ok {
		return closer.Close()
	}
	return nil
}

// NewRetryProvider creates a RetryProvider wrapping inner with the given config.
// Use DefaultRetryConfig() to get sensible defaults.
func NewRetryProvider(inner Provider, cfg RetryConfig) *RetryProvider {
	return &RetryProvider{inner: inner, config: normalizeRetryConfig(cfg)}
}

// Name delegates to the wrapped provider.
func (r *RetryProvider) Name() string { return r.inner.Name() }

func (r *RetryProvider) UnsupportedRequestFields(model string) []string {
	if observable, ok := r.inner.(interface{ UnsupportedRequestFields(string) []string }); ok {
		return observable.UnsupportedRequestFields(model)
	}
	return nil
}

// ModelID delegates to the wrapped provider.
func (r *RetryProvider) ModelID() string { return r.inner.ModelID() }

// APIFormat exposes the wrapped OpenAI-family wire protocol for diagnostics.
func (r *RetryProvider) APIFormat() string {
	if formatted, ok := r.inner.(interface{ APIFormat() string }); ok {
		return formatted.APIFormat()
	}
	return ""
}

// Capabilities implements CapabilityProvider by delegating to the inner provider
// if it also implements CapabilityProvider. Returns zero-value if not supported.
func (r *RetryProvider) Capabilities() ProviderCapabilities {
	if cp, ok := r.inner.(CapabilityProvider); ok {
		return cp.Capabilities()
	}
	return ProviderCapabilities{}
}

// CreateStream calls the inner provider's CreateStream, retrying on transient
// errors according to the RetryConfig.
//
// Retry strategy:
//   - At most MaxAttempts HTTP calls while establishing this stream.
//   - Exponential backoff with ±10% jitter and Retry-After support.
//   - Permanent 4xx, context, quota, billing, and model errors fail fast.
//   - A 401 can refresh credentials at most once, then retries without delay.
//   - Context cancellation aborts the retry loop immediately.
//
// If CreateStream succeeds (returns a channel), the channel is returned directly
// without wrapping — stream-level EventErrors are forwarded as-is to the caller.
func (r *RetryProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	// Pre-flight capability validation: fail fast on unsupported features
	if err := ValidateParams(r.inner, params); err != nil {
		return nil, err
	}

	// Request establishment has an independent budget from the query loop's
	// stream-reconnect controller. This mirrors Codex CLI: request retries are
	// handled by the HTTP client, while dropped streams are replayed by the turn
	// loop. A fresh request controller is required for every stream attempt.
	controller := NewAttemptController(r.config)
	ctx = withRequestAttemptController(ctx, controller)

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		attempt, beginErr := controller.beginAttempt()
		if beginErr != nil {
			return nil, beginErr
		}

		ch, err := r.inner.CreateStream(ctx, params)
		if err == nil {
			// Success — return the channel directly; no stream-level retry wrapper.
			return ch, nil
		}
		controller.recordError(err)

		if is401Error(err) {
			// Refresh is both single-use and budget-aware. A second 401 is never
			// replayed, even if a buggy callback keeps returning true.
			if r.config.OnAuthError != nil && controller.reserveAuthRefresh() && r.config.OnAuthError() {
				r.notifyRetry(ctx, controller, attempt, 0, err)
				continue
			}
			return nil, err
		}
		// RetryDelay returns false for both permanent failures and an exhausted
		// attempt budget. Preserve permanent errors exactly as returned by the
		// provider; only retryable failures that cannot reserve another attempt
		// are attempt-limit failures.
		if !ClassifyAttemptError(err).Retryable() {
			return nil, err
		}
		delay, retry := controller.RetryDelay(err)
		if !retry {
			return nil, controller.exhausted(err)
		}
		r.notifyRetry(ctx, controller, attempt, delay, err)
		if waitErr := controller.Wait(ctx, delay); waitErr != nil {
			return nil, waitErr
		}
	}
}

// computeDelay remains a focused policy test seam. Production calls use the
// generation-scoped controller directly.
func (r *RetryProvider) computeDelay(attempt int, err error) time.Duration {
	config := normalizeRetryConfig(r.config)
	capDelay := exponentialDelay(config.BaseDelay, config.MaxDelay, attempt)
	delay := config.jitter(capDelay)
	if retryAfter := parseRetryAfter(err, config.now()); retryAfter > delay {
		delay = retryAfter
	}
	return delay
}

func (r *RetryProvider) notifyRetry(ctx context.Context, controller *AttemptController, attempt int, delay time.Duration, err error) {
	maxRetries := controller.MaxAttempts() - 1
	if r.config.OnRetry != nil {
		r.config.OnRetry(attempt, maxRetries, delay, err)
	}
	notifyRetryObserver(ctx, RetryEvent{
		Attempt:    attempt,
		MaxRetries: maxRetries,
		Delay:      delay,
		Err:        err,
		Kind:       "request",
	})
}
