package provider

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	defaultRequestMaxAttempts = 5
	defaultStreamMaxAttempts  = 6
	maxRequestMaxAttempts     = 101
	maxStreamMaxAttempts      = 101
)

// AttemptController owns one retry budget. The runtime creates a generation-
// scoped controller for stream reconnects, while RetryProvider creates a fresh
// request-scoped controller for each attempt to establish a stream.
type AttemptController struct {
	config        RetryConfig
	requestConfig RetryConfig

	mu              sync.Mutex
	attempts        int
	authRefreshUsed bool
	lastErr         error
}

type attemptControllerContextKey struct{}
type requestAttemptControllerContextKey struct{}

// NewAttemptController creates a generation-scoped controller. A zero-value
// configuration receives the same production defaults as RetryProvider.
func NewAttemptController(config RetryConfig) *AttemptController {
	config = normalizeRetryConfig(config)
	return &AttemptController{config: config, requestConfig: config}
}

// AttemptControllerForProvider creates a controller from the retry policy of
// the provider being sampled. ProviderRef wrappers are resolved recursively.
// Raw providers use fallback when supplied, otherwise the production default.
func AttemptControllerForProvider(p Provider, fallback ...RetryConfig) *AttemptController {
	config, ok := retryConfigForProvider(p)
	if !ok {
		if len(fallback) > 0 {
			config = fallback[0]
		} else {
			config = DefaultRetryConfig()
		}
	}
	config = normalizeRetryConfig(config)
	requestConfig := config
	config.MaxAttempts = config.StreamMaxAttempts
	controller := NewAttemptController(config)
	controller.requestConfig = requestConfig
	return controller
}

// WithAttemptController binds a generation budget to all nested provider
// decorators participating in the request.
func WithAttemptController(ctx context.Context, controller *AttemptController) context.Context {
	if ctx == nil || controller == nil {
		return ctx
	}
	return context.WithValue(ctx, attemptControllerContextKey{}, controller)
}

func attemptControllerFromContext(ctx context.Context) *AttemptController {
	if ctx == nil {
		return nil
	}
	controller, _ := ctx.Value(attemptControllerContextKey{}).(*AttemptController)
	return controller
}

func withRequestAttemptController(ctx context.Context, controller *AttemptController) context.Context {
	if ctx == nil || controller == nil {
		return ctx
	}
	return context.WithValue(ctx, requestAttemptControllerContextKey{}, controller)
}

func requestAttemptControllerFromContext(ctx context.Context) *AttemptController {
	if ctx == nil {
		return nil
	}
	controller, _ := ctx.Value(requestAttemptControllerContextKey{}).(*AttemptController)
	return controller
}

// beginNestedTransportAttempt charges a second or later HTTP request performed
// inside one Provider.CreateStream call (for example, a protocol or optional-
// field compatibility fallback). The outer RetryProvider already charged the
// first request. Providers without a bound request controller retain their
// standalone behavior.
func beginNestedTransportAttempt(ctx context.Context, cause error) error {
	controller := requestAttemptControllerFromContext(ctx)
	if controller == nil {
		return nil
	}
	controller.recordError(cause)
	nextAttempt, err := controller.beginAttempt()
	if err != nil {
		return err
	}
	if cause != nil {
		// Nested compatibility and transport fallbacks previously consumed the
		// request budget invisibly. Project the failed request before the
		// replacement begins so interactive clients can show the same
		// reconnecting + additional-details state as Codex StreamErrorEvent.
		notifyRetryObserver(ctx, RetryEvent{
			Attempt:    nextAttempt - 1,
			MaxRetries: controller.MaxAttempts() - 1,
			Delay:      0,
			Err:        cause,
			Kind:       "request",
			DroppedField: func() string {
				if apiErr, ok := AsAPIError(cause); ok && apiErr.FailureDiagnostic != nil {
					return apiErr.FailureDiagnostic.DroppedField
				}
				return ""
			}(),
		})
	}
	return nil
}

// CreateStreamAttempt charges exactly one response-stream attempt. Any
// RetryProvider nested inside the call owns a separate HTTP request budget.
func CreateStreamAttempt(ctx context.Context, controller *AttemptController, p Provider, params Params) (<-chan types.StreamEvent, error) {
	if controller == nil {
		controller = AttemptControllerForProvider(p)
	}
	ctx = WithAttemptController(ctx, controller)
	if _, err := controller.beginAttempt(); err != nil {
		return nil, err
	}
	if !providerManagesRequestAttempts(p) {
		requestController := NewAttemptController(controller.requestConfig)
		if _, err := requestController.beginAttempt(); err != nil {
			return nil, err
		}
		ctx = withRequestAttemptController(ctx, requestController)
	}
	stream, err := p.CreateStream(ctx, params)
	controller.recordError(err)
	return stream, err
}

// MaxAttempts is the total number of attempts in this controller's scope.
func (c *AttemptController) MaxAttempts() int {
	if c == nil {
		return defaultStreamMaxAttempts
	}
	return c.config.MaxAttempts
}

// Attempts reports how many raw provider calls have begun.
func (c *AttemptController) Attempts() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts
}

// CanRetry combines the typed provider disposition with this controller's
// scope-local budget.
func (c *AttemptController) CanRetry(err error) bool {
	if c == nil || !ClassifyAttemptError(err).Retryable() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts < c.config.MaxAttempts
}

// RetryDelay returns the delay before another attempt. It applies exponential
// backoff with ±10% jitter. Retry-After is authoritative when longer than the
// computed delay, as it is in Codex CLI. false means the failure is permanent
// or this scope's budget is exhausted.
func (c *AttemptController) RetryDelay(err error) (time.Duration, bool) {
	if c == nil || !ClassifyAttemptError(err).Retryable() {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempts >= c.config.MaxAttempts {
		return 0, false
	}
	retryIndex := c.attempts - 1
	if retryIndex < 0 {
		retryIndex = 0
	}
	capDelay := exponentialDelay(c.config.BaseDelay, c.config.MaxDelay, retryIndex)
	delay := c.config.jitter(capDelay)
	if delay < 0 {
		delay = 0
	}
	if retryAfter := parseRetryAfter(err, c.config.now()); retryAfter > delay {
		delay = retryAfter
	}
	return delay, true
}

// Wait blocks for a retry delay while preserving context cancellation.
func (c *AttemptController) Wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *AttemptController) beginAttempt() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempts >= c.config.MaxAttempts {
		return 0, c.exhaustedErrorLocked(c.lastErr)
	}
	c.attempts++
	return c.attempts, nil
}

func (c *AttemptController) recordError(err error) {
	if c == nil || err == nil {
		return
	}
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
}

func (c *AttemptController) exhausted(err error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		err = c.lastErr
	}
	if c.attempts < c.config.MaxAttempts {
		return err
	}
	return c.exhaustedErrorLocked(err)
}

func (c *AttemptController) exhaustedErrorLocked(err error) error {
	return &AttemptLimitError{
		MaxAttempts: c.config.MaxAttempts,
		Attempts:    c.attempts,
		Cause:       err,
	}
}

func (c *AttemptController) reserveAuthRefresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authRefreshUsed || c.attempts >= c.config.MaxAttempts {
		return false
	}
	c.authRefreshUsed = true
	return true
}

// AttemptLimitError retains the final transport cause while making exhaustion
// machine-detectable to callers, preventing a second retry layer from starting
// another loop.
type AttemptLimitError struct {
	MaxAttempts int
	Attempts    int
	Cause       error
}

// AttemptErrorStage/Class/ReplaySafety make a request-budget exhaustion a
// terminal boundary for the outer stream loop. The preserved cause remains
// available through errors.Is/As, but it must not start a second request retry
// loop and multiply the configured budget.
func (e *AttemptLimitError) AttemptErrorStage() types.ProviderErrorStage {
	return types.ProviderErrorStageConnect
}

func (e *AttemptLimitError) AttemptErrorClass() types.ProviderErrorClass {
	return types.ProviderErrorClassPermanent
}

func (e *AttemptLimitError) AttemptReplaySafety() types.ProviderReplaySafety {
	return types.ProviderReplayUnsafe
}

func (e *AttemptLimitError) Error() string {
	if e == nil {
		return ""
	}
	retries := e.Attempts - 1
	if retries < 0 {
		retries = 0
	}
	if e.Cause != nil {
		return i18n.WrapError(i18n.KeyProviderRetryExceededWithCause, e.Cause, retries).Error()
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderRetryExceededWithoutCause, retries)
}

func (e *AttemptLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// IsAttemptLimit reports whether a generation has consumed its complete raw
// provider-call budget.
func IsAttemptLimit(err error) bool {
	var limit *AttemptLimitError
	return errors.As(err, &limit)
}

func providerManagesRequestAttempts(p Provider) bool {
	switch current := p.(type) {
	case *RetryProvider:
		return true
	case *ProviderRef:
		return providerManagesRequestAttempts(current.Get())
	default:
		return false
	}
}

func retryConfigForProvider(p Provider) (RetryConfig, bool) {
	switch current := p.(type) {
	case *RetryProvider:
		return current.config, true
	case *ProviderRef:
		return retryConfigForProvider(current.Get())
	default:
		return RetryConfig{}, false
	}
}

func normalizeRetryConfig(config RetryConfig) RetryConfig {
	defaults := DefaultRetryConfig()
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = defaults.MaxAttempts
	}
	if config.StreamMaxAttempts <= 0 {
		config.StreamMaxAttempts = defaults.StreamMaxAttempts
	}
	if config.MaxAttempts > maxRequestMaxAttempts {
		config.MaxAttempts = maxRequestMaxAttempts
	}
	if config.StreamMaxAttempts > maxStreamMaxAttempts {
		config.StreamMaxAttempts = maxStreamMaxAttempts
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = defaults.BaseDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = defaults.MaxDelay
	}
	if config.MaxDelay < config.BaseDelay {
		config.BaseDelay = config.MaxDelay
	}
	if config.jitter == nil {
		config.jitter = codexJitter
	}
	if config.now == nil {
		config.now = time.Now
	}
	return config
}

func codexJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	factor := 0.9 + rand.Float64()*0.2
	return time.Duration(float64(delay) * factor)
}

func exponentialDelay(base, maximum time.Duration, retryIndex int) time.Duration {
	if retryIndex < 0 {
		retryIndex = 0
	}
	delay := base
	for index := 0; index < retryIndex; index++ {
		if delay >= maximum || delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func parseRetryAfter(err error, now time.Time) time.Duration {
	apiErr, ok := AsAPIError(err)
	if !ok {
		return 0
	}
	value := strings.TrimSpace(apiErr.RetryAfter)
	if value == "" {
		return 0
	}
	if seconds, parseErr := strconv.ParseFloat(value, 64); parseErr == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	}
	when, parseErr := http.ParseTime(value)
	if parseErr != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}
