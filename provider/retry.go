package provider

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// RetryConfig controls retry behaviour.
type RetryConfig struct {
	MaxRetries    int           // default 10
	BaseDelay     time.Duration // default 1 s
	MaxDelay      time.Duration // default 32 s
	Max529Retries int           // default 10
	// OnRetry is called before each retry attempt. nil = no logging.
	// attempt is 1-based; delay is 0 when giving up (e.g. 529 limit reached).
	OnRetry func(attempt, maxRetries int, delay time.Duration, err error)
	// OnAuthError is called when a 401 Unauthorized error is encountered.
	// If it returns true, credentials were refreshed and one retry is allowed.
	// If nil or it returns false, the 401 is surfaced immediately (fail fast).
	OnAuthError func() bool
}

// DefaultRetryConfig returns the interactive LLM retry policy.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    10,
		BaseDelay:     time.Second,
		MaxDelay:      32 * time.Second,
		Max529Retries: 10,
	}
}

// RetryProvider is a decorator that wraps any Provider and adds retry logic.
// It implements the Provider interface and is transparent to callers.
type RetryProvider struct {
	inner  Provider
	config RetryConfig
}

// NewRetryProvider creates a RetryProvider wrapping inner with the given config.
// Use DefaultRetryConfig() to get sensible defaults.
func NewRetryProvider(inner Provider, cfg RetryConfig) *RetryProvider {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = DefaultRetryConfig().MaxRetries
	}
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = DefaultRetryConfig().BaseDelay
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = DefaultRetryConfig().MaxDelay
	}
	if cfg.Max529Retries == 0 {
		cfg.Max529Retries = DefaultRetryConfig().Max529Retries
	}
	return &RetryProvider{inner: inner, config: cfg}
}

// Name delegates to the wrapped provider.
func (r *RetryProvider) Name() string { return r.inner.Name() }

// ModelID delegates to the wrapped provider.
func (r *RetryProvider) ModelID() string { return r.inner.ModelID() }

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
//   - Exponential back-off: delay = min(base*2^attempt, maxDelay)
//   - Respects Retry-After header when present in the error.
//   - 529 "overloaded" retries are limited by Max529Retries.
//   - Context cancellation aborts the retry loop immediately.
//
// If CreateStream succeeds (returns a channel), the channel is returned directly
// without wrapping — stream-level EventErrors are forwarded as-is to the caller.
func (r *RetryProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	// Pre-flight capability validation: fail fast on unsupported features
	if err := ValidateParams(r.inner, params); err != nil {
		return nil, err
	}

	cfg := r.config
	retries529 := 0

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		ch, err := r.inner.CreateStream(ctx, params)
		if err == nil {
			// Success — return the channel directly; no stream-level retry wrapper.
			return ch, nil
		}

		// Classify the error.
		if Is529Error(err) {
			retries529++
			if retries529 > cfg.Max529Retries {
				r.notifyRetry(ctx, attempt+1, 0, err)
				return nil, err
			}
		} else if is401Error(err) {
			// 401 Unauthorized: no built-in auth refresh; fail fast unless
			// OnAuthError callback signals that credentials were refreshed.
			if cfg.OnAuthError != nil && cfg.OnAuthError() {
				// Credentials refreshed — allow one retry without backoff.
				continue
			}
			return nil, err
		} else if IsPromptTooLong(err) {
			// Not retryable — surface to caller for PTL handling.
			return nil, err
		} else if !IsRetryable(err) {
			return nil, err
		}

		if attempt == cfg.MaxRetries {
			return nil, fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderRetryExceededWithoutCause, cfg.MaxRetries), err)
		}

		delay := r.computeDelay(attempt, err)
		r.notifyRetry(ctx, attempt+1, delay, err)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderRetryExceededWithoutCause, cfg.MaxRetries))
}

// computeDelay returns the back-off delay for a given attempt number.
//
// Formula:
//
//	baseDelay = min(BaseDelay * 2^attempt, MaxDelay)
//	delay     = baseDelay
//
// If the error carries a Retry-After header, that value is used instead when
// it is larger than the computed delay.
func (r *RetryProvider) computeDelay(attempt int, err error) time.Duration {
	base := r.config.BaseDelay
	maxD := r.config.MaxDelay

	factor := math.Pow(2, float64(attempt))
	scaled := time.Duration(float64(base) * factor)
	if scaled > maxD {
		scaled = maxD
	}
	delay := scaled

	// Honor Retry-After header if present.
	if ae, ok := AsAPIError(err); ok && ae.RetryAfter != "" {
		if secs, parseErr := strconv.ParseFloat(ae.RetryAfter, 64); parseErr == nil {
			ra := time.Duration(secs * float64(time.Second))
			if ra > delay {
				delay = ra
			}
		}
	}

	return delay
}

func (r *RetryProvider) notifyRetry(ctx context.Context, attempt int, delay time.Duration, err error) {
	if r.config.OnRetry != nil {
		r.config.OnRetry(attempt, r.config.MaxRetries, delay, err)
	}
	notifyRetryObserver(ctx, RetryEvent{
		Attempt:    attempt,
		MaxRetries: r.config.MaxRetries,
		Delay:      delay,
		Err:        err,
	})
}
