package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
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
		MaxAttempts:       3,
		StreamMaxAttempts: 3,
		BaseDelay:         time.Millisecond,
		MaxDelay:          10 * time.Millisecond,
		jitter:            func(upper time.Duration) time.Duration { return upper },
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

// TestRetry_529_LimitRespected verifies that overload errors cannot exceed the
// request-attempt budget.
func TestRetry_529_LimitRespected(t *testing.T) {
	cfg := fastConfig()

	overloaded := &types.APIError{Status: 529, Type: "overloaded_error", Message: "overloaded"}
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: overloaded},
			{err: overloaded},
			{err: overloaded},
		},
	}
	rp := NewRetryProvider(mock, cfg)

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error after exceeding 529 retry limit")
	}
	if mock.calls != 3 {
		t.Errorf("expected total budget of 3 calls, got %d", mock.calls)
	}
	var limit *AttemptLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %T %v, want AttemptLimitError", err, err)
	}
	if limit.Attempts != 3 || limit.MaxAttempts != 3 {
		t.Fatalf("attempt limit = %+v, want 3 actual attempts from a budget of 3", limit)
	}
	if !errors.Is(err, overloaded) {
		t.Fatal("attempt-limit error did not preserve its final provider cause")
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) || apiErr != overloaded {
		t.Fatalf("errors.As APIError = %#v, want final cause %#v", apiErr, overloaded)
	}
	wantMessage := i18n.WrapError(i18n.KeyProviderRetryExceededWithCause, overloaded, 2).Error()
	if err.Error() != wantMessage {
		t.Fatalf("attempt-limit message = %q, want %q", err, wantMessage)
	}
}

func TestAttemptLimitErrorMessageUsesActualAttempts(t *testing.T) {
	err := (&AttemptLimitError{MaxAttempts: 9, Attempts: 2}).Error()
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderRetryExceededWithoutCause, 1)
	if err != want {
		t.Fatalf("message = %q, want actual retry count message %q", err, want)
	}
}

// TestRetry_ExponentialBackoff verifies the computed delay grows with attempt.
func TestRetry_ExponentialBackoff(t *testing.T) {
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  10 * time.Second,
		jitter:    func(upper time.Duration) time.Duration { return upper },
	}}

	d0 := rp.computeDelay(0, nil)
	d1 := rp.computeDelay(1, nil)
	d2 := rp.computeDelay(2, nil)

	if d0 != 100*time.Millisecond || d1 != 200*time.Millisecond || d2 != 400*time.Millisecond {
		t.Errorf("delays = [%v %v %v], want [100ms 200ms 400ms]", d0, d1, d2)
	}
}

func TestDefaultRetryConfigMatchesCodexRequestAndStreamBudgets(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 5 || cfg.StreamMaxAttempts != 6 || cfg.BaseDelay != 200*time.Millisecond || cfg.MaxDelay != 3200*time.Millisecond {
		t.Fatalf("default retry config = %+v", cfg)
	}
}

func TestRetryConfigCapsCustomRetryCountsLikeCodex(t *testing.T) {
	cfg := normalizeRetryConfig(RetryConfig{MaxAttempts: 1000, StreamMaxAttempts: 2000})
	if cfg.MaxAttempts != 101 || cfg.StreamMaxAttempts != 101 {
		t.Fatalf("bounded retry config = %+v, want at most 100 retries plus the initial attempt", cfg)
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
	if len(observed) != 1 || observed[0].Attempt != 1 || observed[0].MaxRetries != 2 || observed[0].Delay != 2*time.Millisecond || observed[0].Err == nil || observed[0].Kind != "request" {
		t.Fatalf("retry observer events = %+v", observed)
	}
}

// TestRetry_MaxDelayRespected verifies that delay is capped at MaxDelay.
func TestRetry_MaxDelayRespected(t *testing.T) {
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: 500 * time.Millisecond,
		MaxDelay:  1 * time.Second,
		jitter:    func(upper time.Duration) time.Duration { return upper },
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
		MaxDelay:  3 * time.Second,
		jitter:    func(upper time.Duration) time.Duration { return upper },
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

func TestRetry_RetryAfterHTTPDateIsAuthoritative(t *testing.T) {
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	rp := &RetryProvider{config: RetryConfig{
		BaseDelay: time.Millisecond,
		MaxDelay:  1500 * time.Millisecond,
		jitter:    func(time.Duration) time.Duration { return 0 },
		now:       func() time.Time { return now },
	}}
	ae := &types.APIError{Status: 429, RetryAfter: now.Add(5 * time.Second).Format(http.TimeFormat)}
	if delay := rp.computeDelay(0, ae); delay != 5*time.Second {
		t.Fatalf("request-layer HTTP-date Retry-After delay = %v, want authoritative server delay 5s", delay)
	}
}

func TestRetry_InjectedJitterStaysWithinExponentialCap(t *testing.T) {
	controller := NewAttemptController(RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    time.Second,
		jitter: func(upper time.Duration) time.Duration {
			return upper / 3
		},
	})
	if _, err := controller.beginAttempt(); err != nil {
		t.Fatal(err)
	}
	delay, ok := controller.RetryDelay(&types.APIError{Status: 429})
	if !ok || delay != 100*time.Millisecond/3 {
		t.Fatalf("injected jitter delay = %v, retry=%v", delay, ok)
	}
}

func TestAttemptControllerAllowsCodexPositiveJitter(t *testing.T) {
	controller := NewAttemptController(RetryConfig{
		MaxAttempts: 2,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    time.Second,
		jitter: func(delay time.Duration) time.Duration {
			return delay * 11 / 10
		},
	})
	if _, err := controller.beginAttempt(); err != nil {
		t.Fatal(err)
	}
	delay, ok := controller.RetryDelay(&types.APIError{Status: 503})
	if !ok || delay != 110*time.Millisecond {
		t.Fatalf("positive jitter delay = %v, retry=%v, want 110ms", delay, ok)
	}
}

func TestCodexJitterStaysWithinTenPercent(t *testing.T) {
	base := time.Second
	for index := 0; index < 1000; index++ {
		delay := codexJitter(base)
		if delay < 900*time.Millisecond || delay >= 1100*time.Millisecond {
			t.Fatalf("codex jitter = %s, want [900ms, 1.1s)", delay)
		}
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

func TestRetry_401_RefreshIsAttemptedAtMostOnce(t *testing.T) {
	unauthorized := &types.APIError{Status: 401, Message: "unauthorized"}
	mock := &mockProvider{name: "mock", results: []mockResult{{err: unauthorized}, {err: unauthorized}, {events: successEvents()}}}
	cfg := fastConfig()
	refreshes := 0
	cfg.OnAuthError = func() bool {
		refreshes++
		return true
	}
	rp := NewRetryProvider(mock, cfg)
	if _, err := rp.CreateStream(context.Background(), Params{}); err == nil {
		t.Fatal("expected second unauthorized response to fail")
	}
	if refreshes != 1 || mock.calls != 2 {
		t.Fatalf("refreshes=%d calls=%d, want exactly one refresh and two calls", refreshes, mock.calls)
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
	badRequest := &types.APIError{Status: 400, Message: "bad request"}
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: badRequest},
		},
	}
	rp := NewRetryProvider(mock, fastConfig())

	_, err := rp.CreateStream(context.Background(), Params{})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if err != badRequest || !errors.Is(err, badRequest) {
		t.Fatalf("error = %#v, want original 400 error %#v", err, badRequest)
	}
	if IsAttemptLimit(err) {
		t.Fatalf("permanent 400 was wrapped as attempt exhaustion: %v", err)
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) || apiErr != badRequest {
		t.Fatalf("errors.As APIError = %#v, want %#v", apiErr, badRequest)
	}
	if mock.calls != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", mock.calls)
	}
}

func TestRetry_PermanentErrorAfterTransientRetryIsReturnedDirectly(t *testing.T) {
	transient := &types.APIError{Status: 503, Type: "server_error", Message: "unavailable"}
	badRequest := &types.APIError{Status: 400, Type: "invalid_request_error", Message: "bad request"}
	mock := &mockProvider{
		name: "mock",
		results: []mockResult{
			{err: transient},
			{err: badRequest},
			{events: successEvents()},
		},
	}

	_, err := NewRetryProvider(mock, fastConfig()).CreateStream(context.Background(), Params{})
	if err != badRequest || !errors.Is(err, badRequest) {
		t.Fatalf("error = %#v, want original post-retry permanent error %#v", err, badRequest)
	}
	if IsAttemptLimit(err) {
		t.Fatalf("post-retry permanent error was wrapped as attempt exhaustion: %v", err)
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) || apiErr != badRequest {
		t.Fatalf("errors.As APIError = %#v, want %#v", apiErr, badRequest)
	}
	if mock.calls != 2 {
		t.Fatalf("raw calls = %d, want one transient call and one permanent call", mock.calls)
	}
}

func TestAttemptControllerExhaustedDoesNotWrapAvailableBudget(t *testing.T) {
	controller := NewAttemptController(fastConfig())
	if _, err := controller.beginAttempt(); err != nil {
		t.Fatal(err)
	}
	cause := &types.APIError{Status: 503, Message: "unavailable"}
	err := controller.exhausted(cause)
	if err != cause || IsAttemptLimit(err) {
		t.Fatalf("exhausted before budget consumption = %#v, want original cause", err)
	}
}

func TestRetry_PermanentProblemsUseExactlyOneRawCall(t *testing.T) {
	cases := []*types.APIError{
		{Status: 400, Type: "context_length_exceeded", Message: "context window exceeded"},
		{Status: 402, Type: "billing_error", Message: "payment required"},
		{Status: 403, Type: "model_not_found", Message: "model not found"},
		{Status: 404, Type: "server_error", Message: "endpoint not found"},
	}
	for _, apiErr := range cases {
		mock := &mockProvider{name: "mock", results: []mockResult{{err: apiErr}, {events: successEvents()}}}
		if _, err := NewRetryProvider(mock, fastConfig()).CreateStream(context.Background(), Params{}); err == nil {
			t.Fatalf("expected permanent problem to fail: %+v", apiErr)
		}
		if mock.calls != 1 {
			t.Fatalf("permanent problem %+v made %d raw calls, want 1", apiErr, mock.calls)
		}
	}
}

func TestRetry_CustomAttemptBudgetIsHonored(t *testing.T) {
	serverErr := &types.APIError{Status: 503, Type: "server_error", Message: "unavailable"}
	mock := &mockProvider{name: "mock", results: []mockResult{{err: serverErr}}}
	cfg := fastConfig()
	cfg.MaxAttempts = 4
	if _, err := NewRetryProvider(mock, cfg).CreateStream(context.Background(), Params{}); err == nil {
		t.Fatal("expected configured attempt budget to exhaust")
	}
	if mock.calls != 4 {
		t.Fatalf("raw calls = %d, want configured total of 4", mock.calls)
	}
}

func TestAnthropicSDKAndStreamingFallbackShareControllerBudget(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
	}))
	defer srv.Close()

	raw := NewAnthropic(Config{APIKey: "test-key", BaseURL: srv.URL, Model: "test-model"})
	controller := NewAttemptController(RetryConfig{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	stream, err := CreateStreamAttempt(context.Background(), controller, raw, Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if requests != 1 {
		t.Fatalf("Anthropic HTTP requests = %d, want controller cap of 1", requests)
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

// Context-shape errors require request transformation, not blind replay.
func TestRetry_MaxTokensOverflow_IsPermanent(t *testing.T) {
	ae := &types.APIError{Status: 400, Message: "max_tokens exceed context length"}
	if IsRetryable(ae) {
		t.Error("expected max_tokens overflow to fail fast")
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
	if !IsPromptTooLong(&types.APIError{Status: 400, Code: "context_length_exceeded", Message: "prompt is too long for this model"}) {
		t.Error("expected true for structured context code")
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
