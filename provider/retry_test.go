package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// --- Mock provider for retry tests ---

// mockProvider records calls and returns pre-configured results.
type mockProvider struct {
	name    string
	results []mockResult // responses in order; last one repeated
	calls   int
}

type mockResult struct {
	events []types.StreamEvent
	err    error
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) ModelID() string { return "mock-model" }

func (m *mockProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	idx := m.calls
	if idx >= len(m.results) {
		idx = len(m.results) - 1
	}
	m.calls++
	res := m.results[idx]
	if res.err != nil {
		return nil, res.err
	}
	ch := make(chan types.StreamEvent, len(res.events)+1)
	for _, e := range res.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// successEvents returns a minimal valid stream that completes cleanly.
func successEvents() []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

// errorEvent wraps an *types.APIError in a stream error event.
func errorEvent(ae *types.APIError) types.StreamEvent {
	return types.StreamEvent{Type: types.EventError, Error: ae}
}

// fastConfig returns a RetryConfig with minimal delays for unit tests.
func fastConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    5,
		BaseDelay:     time.Millisecond,
		MaxDelay:      10 * time.Millisecond,
		Max529Retries: 3,
	}
}

// --- Tests ---

// TestRetry_429_EventualSuccess verifies that a 429 error causes a retry and
// the second call succeeds.
func TestRetry_429_EventualSuccess(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 429, Type: "rate_limit_error", Message: "rate limited"}},
			{events: successEvents()},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	ch, err := rp.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drainAndExpectSuccess(t, ch)

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}
}

// TestRetry_529_LimitRespected verifies that consecutive 529 errors stop
// retrying once Max529Retries is exceeded.
func TestRetry_529_LimitRespected(t *testing.T) {
	cfg := fastConfig()
	cfg.Max529Retries = 3

	overloaded := &types.APIError{Status: 529, Type: "overloaded_error", Message: "overloaded"}
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: overloaded},
			{err: overloaded},
			{err: overloaded},
			{err: overloaded}, // 4th — should exceed limit
		},
	}
	rp := NewRetryProvider(mock, cfg)

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error after exceeding 529 retry limit")
	}
	// Should have tried: initial + 3 retries = 4 calls (limit is 3 529 retries)
	if mock.calls != 4 {
		t.Errorf("expected 4 calls, got %d", mock.calls)
	}
}

// TestRetry_ExponentialBackoff verifies the computed delay grows with attempt.
func TestRetry_ExponentialBackoff(t *testing.T) {
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  10 * time.Second,
	}}

	d0 := rp.computeDelay(0, nil)
	d1 := rp.computeDelay(1, nil)
	d2 := rp.computeDelay(2, nil)

	if d0 != 100*time.Millisecond || d1 != 200*time.Millisecond || d2 != 400*time.Millisecond {
		t.Errorf("delays = [%v %v %v], want [100ms 200ms 400ms]", d0, d1, d2)
	}
}

func TestDefaultRetryConfigUsesTenRetriesFromOneSecond(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 10 || cfg.BaseDelay != time.Second || cfg.MaxDelay != 32*time.Second || cfg.Max529Retries != 10 {
		t.Fatalf("default retry config = %+v", cfg)
	}
}

func TestRetryObserverReceivesProblemAndBackoff(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 429, Type: "rate_limit_error", Message: "rate limited"}},
			{events: successEvents()},
		},
	}
	cfg := fastConfig()
	cfg.BaseDelay = 2 * time.Millisecond
	rp := NewRetryProvider(mock, cfg)
	var observed []RetryEvent
	ctx := WithRetryObserver(context.Background(), func(event RetryEvent) {
		observed = append(observed, event)
	})
	ch, err := rp.CreateStream(ctx, Params{})
	if err != nil {
		t.Fatal(err)
	}
	drainAndExpectSuccess(t, ch)
	if len(observed) != 1 || observed[0].Attempt != 1 || observed[0].MaxRetries != 5 || observed[0].Delay != 2*time.Millisecond || observed[0].Err == nil {
		t.Fatalf("retry observer events = %+v", observed)
	}
}

// TestRetry_MaxDelayRespected verifies that delay is capped at MaxDelay.
func TestRetry_MaxDelayRespected(t *testing.T) {
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: 500 * time.Millisecond,
		MaxDelay:  1 * time.Second,
	}}
	// Attempt 10 would give 500ms * 2^10 = 512 s without the cap.
	d := rp.computeDelay(10, nil)
	if d > 2*time.Second { // generous upper bound including jitter
		t.Errorf("delay %v exceeds MaxDelay cap", d)
	}
}

// TestRetry_RetryAfterHeader verifies that the Retry-After header value
// overrides the computed delay when it is larger.
func TestRetry_RetryAfterHeader(t *testing.T) {
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: 1 * time.Millisecond,
		MaxDelay:  10 * time.Millisecond,
	}}
	ae := &types.APIError{
		Status:     429,
		RetryAfter: "2", // 2 seconds
	}
	d := rp.computeDelay(0, ae)
	if d < 2*time.Second {
		t.Errorf("expected Retry-After of 2s to be respected, got %v", d)
	}
}

// TestRetry_ContextCancellation verifies the retry loop aborts immediately on
// context cancellation.
func TestRetry_ContextCancellation(t *testing.T) {
	cfg := fastConfig()
	cfg.BaseDelay = 500 * time.Millisecond // long enough for cancel to win

	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 429, Message: "rate limited"}},
			{err: &types.APIError{Status: 429, Message: "rate limited"}},
		},
	}
	rp := NewRetryProvider(mock, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := rp.CreateStream(ctx, Params{})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestRetry_401_FastFail verifies that a 401 error is NOT retried by default.
func TestRetry_401_FastFail(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 401, Message: "unauthorized"}},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 call (no retry for 401), got %d", mock.calls)
	}
}

// TestRetry_401_WithOnAuthError_Refreshed verifies that when OnAuthError returns
// true (credentials refreshed), one retry is allowed and succeeds.
func TestRetry_401_WithOnAuthError_Refreshed(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 401, Message: "unauthorized"}},
			{events: successEvents()},
		},
	}
	cfg := fastConfig()
	called := false
	cfg.OnAuthError = func() bool {
		called = true
		return true // credentials refreshed
	}
	rp := NewRetryProvider(mock, cfg)

	ch, err := rp.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("unexpected error after auth refresh: %v", err)
	}
	drainAndExpectSuccess(t, ch)

	if !called {
		t.Error("expected OnAuthError to be called")
	}
	if mock.calls != 2 {
		t.Errorf("expected 2 calls (1 initial + 1 retry), got %d", mock.calls)
	}
}

// TestRetry_401_WithOnAuthError_NotRefreshed verifies that when OnAuthError
// returns false, the 401 is surfaced immediately without retrying.
func TestRetry_401_WithOnAuthError_NotRefreshed(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 401, Message: "unauthorized"}},
		},
	}
	cfg := fastConfig()
	cfg.OnAuthError = func() bool {
		return false // could not refresh
	}
	rp := NewRetryProvider(mock, cfg)

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error when OnAuthError returns false")
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", mock.calls)
	}
}

// TestRetry_400_NoRetry verifies that 400 Bad Request is NOT retried.
func TestRetry_400_NoRetry(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 400, Message: "bad request"}},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", mock.calls)
	}
}

// TestRetry_5xx_Retried verifies that 500 server errors are retried.
func TestRetry_5xx_Retried(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 500, Message: "internal server error"}},
			{events: successEvents()},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	ch, err := rp.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drainAndExpectSuccess(t, ch)

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}
}

// TestRetry_StreamEventError_Forwarded verifies that an error delivered as an
// EventError inside the stream is forwarded to the caller as-is.
// Since Issue 2 fix: RetryProvider only retries at the CreateStream call level;
// stream-level EventErrors are not retried to prevent event interleaving.
func TestRetry_StreamEventError_Forwarded(t *testing.T) {
	streamErr := errorEvent(&types.APIError{Status: 529, Type: "overloaded_error", Message: "overloaded"})
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{events: []types.StreamEvent{streamErr}}, // error arrives mid-stream
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	ch, err := rp.CreateStream(context.Background(), Params{})
	if err != nil {
		t.Fatalf("unexpected error from CreateStream: %v", err)
	}

	// The EventError must be forwarded unchanged to the consumer.
	var sawError bool
	for e := range ch {
		if e.Type == types.EventError {
			sawError = true
		}
	}

	if !sawError {
		t.Error("expected EventError to be forwarded to consumer (no in-stream retry)")
	}
	// Only 1 CreateStream call was made — no retry happened.
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 CreateStream call, got %d", mock.calls)
	}
}

// TestRetry_PromptTooLong_NoRetry verifies that PTL errors are not retried.
func TestRetry_PromptTooLong_NoRetry(t *testing.T) {
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: &types.APIError{Status: 400, Type: "prompt_too_long", Message: "prompt is too long"}},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error for PTL")
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 call (no retry for PTL), got %d", mock.calls)
	}
}

// TestRetry_MaxTokensOverflow_IsRetryable verifies that a 400 caused by
// max_tokens overflow is classified as retryable.
func TestRetry_MaxTokensOverflow_IsRetryable(t *testing.T) {
	ae := &types.APIError{Status: 400, Message: "max_tokens exceed context length"}
	if !IsRetryable(ae) {
		t.Error("expected max_tokens overflow to be retryable")
	}
}

func TestIs529Error(t *testing.T) {
	if !Is529Error(&types.APIError{Status: 529}) {
		t.Error("expected true for 529 status")
	}
	if !Is529Error(&types.APIError{Type: "overloaded_error"}) {
		t.Error("expected true for overloaded_error type")
	}
	if Is529Error(&types.APIError{Status: 500}) {
		t.Error("expected false for 500")
	}
}

func TestIsPromptTooLong(t *testing.T) {
	if !IsPromptTooLong(&types.APIError{Status: 400, Message: "prompt is too long for this model"}) {
		t.Error("expected true for PTL message")
	}
	if !IsPromptTooLong(&types.APIError{Type: "prompt_too_long"}) {
		t.Error("expected true for prompt_too_long type")
	}
	if IsPromptTooLong(&types.APIError{Status: 400, Message: "bad request"}) {
		t.Error("expected false for generic 400")
	}
}

// TestRetryProvider_ImplementsProvider verifies compile-time interface compliance.
func TestRetryProvider_ImplementsProvider(t *testing.T) {
	mock := &mockProvider{name: "mock", results: []mockResult{{events: successEvents()}}}
	var _ Provider = NewRetryProvider(mock, DefaultRetryConfig())
}

// --- helpers ---

// drainAndExpectSuccess drains ch and fails if it contains an EventError.
func drainAndExpectSuccess(t *testing.T, ch <-chan types.StreamEvent) {
	t.Helper()
	for e := range ch {
		if e.Type == types.EventError {
			t.Errorf("unexpected error event: %v", e.Error)
		}
	}
}
