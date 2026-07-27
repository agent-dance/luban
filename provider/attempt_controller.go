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

const defaultMaxAttempts = 3

// AttemptController owns the raw-attempt budget for one logical generation.
// The provider's pre-stream retries and the runtime's uncommitted stream
// replays share this controller, so composing the two layers cannot multiply
// the configured limit.
type AttemptController struct {
	config RetryConfig

	mu              sync.Mutex
	attempts        int
	authRefreshUsed bool
	lastErr         error
}

type attemptControllerContextKey struct{}

// NewAttemptController creates a generation-scoped controller. A zero-value
// configuration receives the same production defaults as RetryProvider.
func NewAttemptController(config RetryConfig) *AttemptController {
	return &AttemptController{config: normalizeRetryConfig(config)}
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
	return NewAttemptController(config)
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

// beginNestedTransportAttempt charges a second or later HTTP request performed
// inside one Provider.CreateStream call (for example, a protocol or optional-
// field compatibility fallback). The outer provider decorator already charged
// the first request. Providers without a bound generation controller retain
// their standalone behavior.
func beginNestedTransportAttempt(ctx context.Context, cause error) error {
	controller := attemptControllerFromContext(ctx)
	if controller == nil {
		return nil
	}
	controller.recordError(cause)
	_, err := controller.beginAttempt()
	return err
}

// CreateStreamAttempt performs one controller-aware provider call. A
// RetryProvider consumes the budget around each call to its raw inner provider;
// raw providers are charged here. ProviderRef is handled without double-counting.
func CreateStreamAttempt(ctx context.Context, controller *AttemptController, p Provider, params Params) (<-chan types.StreamEvent, error) {
	if controller == nil {
		controller = AttemptControllerForProvider(p)
	}
	ctx = WithAttemptController(ctx, controller)
	if providerManagesAttempts(p) {
		return p.CreateStream(ctx, params)
	}
	if _, err := controller.beginAttempt(); err != nil {
		return nil, err
	}
	stream, err := p.CreateStream(ctx, params)
	controller.recordError(err)
	return stream, err
}

// MaxAttempts is the total number of raw calls allowed, including the initial
// call. The production default is three.
func (c *AttemptController) MaxAttempts() int {
	if c == nil {
		return defaultMaxAttempts
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

// CanRetry is the sole generation-scoped retry admission check. It combines
// the typed provider disposition with the shared raw-call budget; runtime and
// provider decorators must not maintain an independent retry policy.
func (c *AttemptController) CanRetry(err error) bool {
	if c == nil || !ClassifyAttemptError(err).Retryable() {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts < c.config.MaxAttempts
}

// RetryDelay returns the bounded delay before another raw call. It applies
// exponential full jitter and treats Retry-After as a server-provided minimum,
// while still enforcing MaxDelay. false means the failure is permanent or the
// generation budget is exhausted.
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
	if delay > capDelay {
		delay = capDelay
	}
	if retryAfter := parseRetryAfter(err, c.config.now()); retryAfter > delay {
		delay = retryAfter
	}
	if delay > c.config.MaxDelay {
		delay = c.config.MaxDelay
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

func (e *AttemptLimitError) Error() string {
	if e == nil {
		return ""
	}
	retries := e.MaxAttempts - 1
	if retries < 0 {
		retries = 0
	}
	message := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderRetryExceededWithoutCause, retries)
	if e.Cause != nil {
		return message + ": " + e.Cause.Error()
	}
	return message
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

func providerManagesAttempts(p Provider) bool {
	switch current := p.(type) {
	case *RetryProvider:
		return true
	case *ProviderRef:
		return providerManagesAttempts(current.Get())
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
		config.jitter = fullJitter
	}
	if config.now == nil {
		config.now = time.Now
	}
	return config
}

func fullJitter(upper time.Duration) time.Duration {
	if upper <= 0 {
		return 0
	}
	// Int64N is half-open; adding one allows the configured cap itself while
	// guarding the practically unreachable duration overflow case.
	if upper == time.Duration(1<<63-1) {
		return time.Duration(rand.Int64N(int64(upper)))
	}
	return time.Duration(rand.Int64N(int64(upper) + 1))
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
