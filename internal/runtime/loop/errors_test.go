package loop

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// ---- helpers ----------------------------------------------------------------

func init() {
	// Speed up retry delays in all tests in this package.
	retryBaseDelay = time.Millisecond
}

// failNProvider returns an error for the first n calls then succeeds.
type failNProvider struct {
	calls     atomic.Int32
	failUntil int32
	response  []types.StreamEvent
	failErr   error
}

func (p *failNProvider) Name() string    { return "failN" }
func (p *failNProvider) ModelID() string { return "mock" }

func (p *failNProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	n := p.calls.Add(1)
	if n <= p.failUntil {
		return nil, p.failErr
	}
	ch := make(chan types.StreamEvent, len(p.response))
	for _, ev := range p.response {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// alwaysFailProvider always returns an error from CreateStream.
type alwaysFailProvider struct{ err error }

func (p *alwaysFailProvider) Name() string    { return "alwaysFail" }
func (p *alwaysFailProvider) ModelID() string { return "mock" }
func (p *alwaysFailProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	return nil, p.err
}

// emptyThenFullProvider returns an empty stream on the first call and a real
// response on the second call.
type emptyThenFullProvider struct {
	calls    atomic.Int32
	response []types.StreamEvent
}

func (p *emptyThenFullProvider) Name() string    { return "emptyThenFull" }
func (p *emptyThenFullProvider) ModelID() string { return "mock" }

func (p *emptyThenFullProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	n := p.calls.Add(1)
	if n == 1 {
		ch := make(chan types.StreamEvent)
		close(ch) // empty stream
		return ch, nil
	}
	ch := make(chan types.StreamEvent, len(p.response))
	for _, ev := range p.response {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// textEvents returns a minimal stream that produces a single text response.
func textEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{
			Type:         types.EventContentBlockStart,
			Index:        0,
			ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
		},
		{
			Type:  types.EventContentBlockDelta,
			Index: 0,
			Delta: &types.ContentDelta{Type: "text_delta", Text: text},
		},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

// ---- IsTransient ------------------------------------------------------------

func TestIsTransientNil(t *testing.T) {
	if IsTransient(nil) {
		t.Error("nil should not be transient")
	}
}

func TestIsTransientContextCancelled(t *testing.T) {
	if IsTransient(context.Canceled) {
		t.Error("context.Canceled should not be transient")
	}
}

func TestIsTransientContextDeadline(t *testing.T) {
	if IsTransient(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should not be transient")
	}
}

func TestIsTransientAPIErrorOverloaded(t *testing.T) {
	err := &types.APIError{Type: "overloaded_error", Message: "overloaded"}
	if !IsTransient(err) {
		t.Error("overloaded_error should be transient")
	}
}

func TestIsTransientAPIErrorRateLimit(t *testing.T) {
	err := &types.APIError{Type: "rate_limit_error", Message: "rate limit exceeded"}
	if !IsTransient(err) {
		t.Error("rate_limit_error should be transient")
	}
}

func TestIsTransientAPIErrorOther(t *testing.T) {
	err := &types.APIError{Type: "invalid_request_error", Message: "bad param"}
	if IsTransient(err) {
		t.Error("invalid_request_error should not be transient")
	}
}

func TestIsTransientNetworkErrors(t *testing.T) {
	cases := []string{
		"connection refused",
		"connection reset by peer",
		"dial tcp: timeout",
		"EOF",
		"temporary failure in name resolution",
		"request timeout",
	}
	for _, msg := range cases {
		if !IsTransient(errors.New(msg)) {
			t.Errorf("expected %q to be transient", msg)
		}
	}
}

func TestIsTransientPermanentErrors(t *testing.T) {
	cases := []string{
		"not found",
		"permission denied",
		"invalid JSON",
		"unknown tool",
	}
	for _, msg := range cases {
		if IsTransient(errors.New(msg)) {
			t.Errorf("expected %q NOT to be transient", msg)
		}
	}
}

// ---- retryDelay -------------------------------------------------------------

func TestRetryDelay(t *testing.T) {
	base := retryBaseDelay
	if retryDelay(0) != 1*base {
		t.Errorf("attempt 0: expected 1× base, got %v", retryDelay(0))
	}
	if retryDelay(1) != 2*base {
		t.Errorf("attempt 1: expected 2× base, got %v", retryDelay(1))
	}
	if retryDelay(2) != 4*base {
		t.Errorf("attempt 2: expected 4× base, got %v", retryDelay(2))
	}
	// Beyond the sixth delay: capped at 32× base.
	if retryDelay(10) != 32*base {
		t.Errorf("attempt 10: expected 32× base, got %v", retryDelay(10))
	}
}

// ---- PartialStreamError -----------------------------------------------------

func TestPartialStreamErrorMessage(t *testing.T) {
	cause := errors.New("upstream reset")
	pse := &PartialStreamError{Cause: cause, PartialBlocks: 3}
	if pse.Error() == "" {
		t.Error("expected non-empty error message")
	}
	if !errors.Is(pse, cause) {
		t.Error("Unwrap should expose the cause")
	}
}

// ---- Recovery path 1: transient retry ---------------------------------------

func TestTransientRetrySucceedsOnSecondAttempt(t *testing.T) {
	p := &failNProvider{
		failUntil: 1, // fail once, then succeed
		failErr:   &types.APIError{Type: "overloaded_error", Message: "overloaded"},
		response:  textEvents("hello"),
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var errorEvents []string
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		if e.Type == stream.EventSystemWarning {
			errorEvents = append(errorEvents, projectedSystemWarningText(e))
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.calls.Load() != 2 {
		t.Errorf("expected 2 provider calls (1 fail + 1 success), got %d", p.calls.Load())
	}
	// Should have emitted a warning about the transient error
	if len(errorEvents) == 0 {
		t.Error("expected at least one EventSystemWarning for the transient retry attempt")
	}
}

func TestLLMRequestLifecycleReportsRetryStartFirstTokenAndEnd(t *testing.T) {
	raw := &failNProvider{
		failUntil: 1,
		failErr:   &types.APIError{Type: "overloaded_error", Message: "overloaded"},
		response:  textEvents("hello"),
	}
	cfg := provider.RetryConfig{
		MaxRetries: 5, BaseDelay: time.Millisecond, MaxDelay: 16 * time.Millisecond, Max529Retries: 5,
	}
	ql := New(provider.NewRetryProvider(raw, cfg), registry.New(), Config{MaxTurns: 5, MaxTokens: 1024})

	var lifecycle []stream.Event
	err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		switch event.Type {
		case stream.EventRequestRetry, stream.EventRequestStart, stream.EventRequestFirstToken, stream.EventRequestEnd, stream.EventRequestFailed:
			lifecycle = append(lifecycle, event)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []stream.EventType{stream.EventRequestRetry, stream.EventRequestStart, stream.EventRequestFirstToken, stream.EventRequestEnd}
	if len(lifecycle) != len(want) {
		t.Fatalf("request lifecycle = %+v", lifecycle)
	}
	for index, eventType := range want {
		if lifecycle[index].Type != eventType || lifecycle[index].RequestStatus == nil {
			t.Fatalf("lifecycle[%d] = %+v, want %s with status", index, lifecycle[index], eventType)
		}
	}
	retry := lifecycle[0].RequestStatus
	if retry.Attempt != 1 || retry.MaxRetries != 5 || retry.RetryDelayMilliseconds != 1 || retry.Error == "" {
		t.Fatalf("retry status = %+v", retry)
	}
	requestID := lifecycle[1].RequestStatus.RequestID
	if requestID == "" || lifecycle[2].RequestStatus.RequestID != requestID || lifecycle[3].RequestStatus.RequestID != requestID {
		t.Fatalf("request lifecycle identity was not stable: %+v", lifecycle)
	}
}

func TestTransientProviderRetriesTenTimesWithExponentialDelays(t *testing.T) {
	p := &failNProvider{
		failUntil: 99,
		failErr:   &types.APIError{Type: "overloaded_error", Message: "overloaded"},
	}
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024})
	var retries []*stream.RequestStatusEvent
	err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventRequestRetry && event.RequestStatus != nil {
			copyStatus := *event.RequestStatus
			retries = append(retries, &copyStatus)
		}
	})
	if err == nil {
		t.Fatal("expected exhausted provider error")
	}
	if calls := p.calls.Load(); calls != 11 {
		t.Fatalf("provider calls = %d, want initial call plus ten retries", calls)
	}
	wantDelays := []int64{1, 2, 4, 8, 16, 32, 32, 32, 32, 32}
	if len(retries) != len(wantDelays) {
		t.Fatalf("retry events = %+v", retries)
	}
	for index, wantDelay := range wantDelays {
		if retries[index].Attempt != index+1 || retries[index].MaxRetries != 10 || retries[index].RetryDelayMilliseconds != wantDelay {
			t.Fatalf("retry[%d] = %+v, want delay %dms", index, retries[index], wantDelay)
		}
	}
}

func TestTransientRetryExhausted(t *testing.T) {
	// Always fails with a transient error and should give up after ten retries.
	p := &alwaysFailProvider{
		err: &types.APIError{Type: "overloaded_error", Message: "overloaded"},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	err := ql.Run(context.Background(), "hi", func(e stream.Event) {})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !errors.Is(err, p.err) {
		t.Errorf("expected wrapped API error, got: %v", err)
	}
}

func TestNonTransientErrorNotRetried(t *testing.T) {
	permanentErr := &types.APIError{Type: "invalid_request_error", Message: "bad request"}

	p2 := &failNProvider{
		failUntil: 99, // always fail
		failErr:   permanentErr,
		response:  nil,
	}
	reg := registry.New()
	ql2 := New(p2, reg, Config{MaxTurns: 5, MaxTokens: 1024})
	err := ql2.Run(context.Background(), "hi", func(e stream.Event) {})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should fail on first attempt — non-transient errors must not be retried
	calls := p2.calls.Load()
	if calls != 1 {
		t.Errorf("expected exactly 1 provider call for non-transient error, got %d", calls)
	}
}

func TestTransientRetryRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	p := &alwaysFailProvider{
		err: &types.APIError{Type: "overloaded_error", Message: "overloaded"},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	err := ql.Run(ctx, "hi", func(e stream.Event) {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// ---- Recovery path 2: stop reason -------------------------------------------

func TestStopReasonMaxTokensWarning(t *testing.T) {
	maxTokensReason := types.StopReasonMaxTokens
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "partial"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageDelta, StopReason: &maxTokensReason},
				{Type: types.EventMessageStop},
			},
			textEvents("continued"),
		},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var warnings []string
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		if e.Type == stream.EventSystemWarning {
			warnings = append(warnings, projectedSystemWarningText(e))
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected max_tokens warning event")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "max_tokens") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning containing 'max_tokens', got: %v", warnings)
	}
}

// ---- Recovery path 3: empty response retry ----------------------------------

func TestEmptyResponseRetrySucceeds(t *testing.T) {
	p := &emptyThenFullProvider{response: textEvents("recovered")}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var texts []string
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		if e.Type == stream.EventText {
			texts = append(texts, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error after empty-response retry: %v", err)
	}
	if len(texts) == 0 || texts[0] != "recovered" {
		t.Errorf("expected text 'recovered', got %v", texts)
	}
	if p.calls.Load() != 2 {
		t.Errorf("expected 2 provider calls (1 empty + 1 success), got %d", p.calls.Load())
	}
}

func TestEmptyResponseBothEmptyFails(t *testing.T) {
	// Always returns empty stream → should fail after retry
	p := &emptyThenFullProvider{response: []types.StreamEvent{}} // second call also empty
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	err := ql.Run(context.Background(), "hi", func(e stream.Event) {})
	if err == nil {
		t.Fatal("expected error when both attempts return empty response")
	}
}

// ---- Recovery path 4: partial stream error ----------------------------------

func TestPartialStreamContinuesWithBlocks(t *testing.T) {
	// Stream delivers one text block then an error event.
	interruptErr := &types.APIError{Type: "server_error", Message: "upstream reset"}
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				// First block arrives normally
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "partial text"}},
				{Type: types.EventContentBlockStop, Index: 0},
				// Stream then errors
				{Type: types.EventError, Error: interruptErr},
			},
			// Second turn: model responds normally (turn ends)
			textEvents("continued"),
		},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var warnings, texts []string
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		switch e.Type {
		case stream.EventSystemWarning:
			warnings = append(warnings, projectedSystemWarningText(e))
		case stream.EventText:
			texts = append(texts, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have emitted a warning about the interruption
	if len(warnings) == 0 {
		t.Error("expected stream-interrupted warning event")
	}
	// Both partial and continued text should have been collected
	if len(texts) == 0 {
		t.Error("expected text events from partial + continued response")
	}
}

func TestPartialStreamNoBlocksReturnsError(t *testing.T) {
	// Stream errors immediately before any content block → should return error.
	interruptErr := &types.APIError{Type: "server_error", Message: "instant failure"}
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventError, Error: interruptErr},
			},
		},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	err := ql.Run(context.Background(), "hi", func(e stream.Event) {})
	if err == nil {
		t.Fatal("expected error when stream errors with no partial content")
	}
}

// ---- isResponseFailedRetryable ----------------------------------------------

func TestIsResponseFailedRetryableNil(t *testing.T) {
	if isResponseFailedRetryable(nil) {
		t.Error("nil should not be retryable")
	}
}

func TestIsResponseFailedRetryableTypes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			"api_error type",
			&types.APIError{Type: "api_error", Message: "Upstream websocket closed"},
			true,
		},
		{
			"server_error type",
			&types.APIError{Type: "server_error", Message: "internal error"},
			true,
		},
		{
			"overloaded_error type",
			&types.APIError{Type: "overloaded_error", Message: "service overloaded"},
			true,
		},
		{
			"rate_limit_error type",
			&types.APIError{Type: "rate_limit_error", Message: "rate limited"},
			true,
		},
		{
			"invalid_request_error should NOT retry",
			&types.APIError{Type: "invalid_request_error", Message: "bad param"},
			false,
		},
		{
			"previous_response_not_found should NOT retry (handled separately)",
			&types.APIError{Type: "previous_response_not_found", Message: "expired"},
			false,
		},
		{
			"authentication_error should NOT retry",
			&types.APIError{Type: "authentication_error", Message: "invalid key"},
			false,
		},
		{
			"plain error should NOT match",
			errors.New("random error"),
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isResponseFailedRetryable(tc.err)
			if got != tc.expect {
				t.Errorf("isResponseFailedRetryable(%v) = %v, want %v", tc.err, got, tc.expect)
			}
		})
	}
}

// ---- Recovery path 5: tool error isolation ----------------------------------

// ---- isStreamInterrupted ----------------------------------------------------

func TestIsStreamInterruptedNil(t *testing.T) {
	if isStreamInterrupted(nil) {
		t.Error("nil should not be stream interrupted")
	}
}

func TestIsStreamInterruptedTypes(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			"stream_interrupted APIError type",
			&types.APIError{Type: "stream_interrupted", Message: "SSE stream ended"},
			true,
		},
		{
			"upstream websocket closed",
			&types.APIError{Type: "api_error", Message: "Upstream websocket closed before response.completed (close_code=1000)"},
			true,
		},
		{
			"upstream disconnected",
			&types.APIError{Type: "api_error", Message: "Upstream disconnected unexpectedly"},
			true,
		},
		{
			"connection reset in message",
			&types.APIError{Type: "api_error", Message: "connection reset during streaming"},
			true,
		},
		{
			"plain error with stream interrupted",
			errors.New("stream interrupted after 2 block(s)"),
			true,
		},
		{
			"plain error with upstream closed",
			errors.New("upstream websocket closed"),
			true,
		},
		{
			"unrelated api_error",
			&types.APIError{Type: "api_error", Message: "invalid model specified"},
			false,
		},
		{
			"invalid_request_error",
			&types.APIError{Type: "invalid_request_error", Message: "bad param"},
			false,
		},
		{
			"unrelated plain error",
			errors.New("file not found"),
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isStreamInterrupted(tc.err)
			if got != tc.expect {
				t.Errorf("isStreamInterrupted(%v) = %v, want %v", tc.err, got, tc.expect)
			}
		})
	}
}

// ---- Recovery path 5 (stream): stream interrupt retry -----------------------

// streamInterruptProvider returns a stream that delivers an error event (no content blocks)
// on the first call, then a normal response on the second call.
type streamInterruptProvider struct {
	calls    atomic.Int32
	response []types.StreamEvent
	errMsg   string
}

type alwaysStreamInterruptProvider struct {
	calls atomic.Int32
}

func (p *alwaysStreamInterruptProvider) Name() string    { return "alwaysStreamInterrupt" }
func (p *alwaysStreamInterruptProvider) ModelID() string { return "mock" }
func (p *alwaysStreamInterruptProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.EventError, Error: &types.APIError{
		Type: "api_error", Message: "upstream websocket closed",
	}}
	close(ch)
	return ch, nil
}

func (p *streamInterruptProvider) Name() string    { return "streamInterrupt" }
func (p *streamInterruptProvider) ModelID() string { return "mock" }

func (p *streamInterruptProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	n := p.calls.Add(1)
	if n == 1 {
		// First call: stream opens successfully but delivers an error event
		ch := make(chan types.StreamEvent, 2)
		ch <- types.StreamEvent{
			Type: types.EventError,
			Error: &types.APIError{
				Type:    "api_error",
				Message: p.errMsg,
			},
		}
		close(ch)
		return ch, nil
	}
	// Second call: normal response
	ch := make(chan types.StreamEvent, len(p.response))
	for _, ev := range p.response {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func TestStreamInterruptRetrySucceeds(t *testing.T) {
	p := &streamInterruptProvider{
		response: textEvents("recovered after interrupt"),
		errMsg:   "Upstream websocket closed before response.completed (close_code=1000)",
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var warnings, texts []string
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		switch e.Type {
		case stream.EventSystemWarning:
			warnings = append(warnings, projectedSystemWarningText(e))
		case stream.EventText:
			texts = append(texts, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.calls.Load() != 2 {
		t.Errorf("expected 2 provider calls (1 interrupt + 1 success), got %d", p.calls.Load())
	}
	// Should have emitted a warning about the stream interrupt
	foundWarning := false
	for _, w := range warnings {
		lower := strings.ToLower(w)
		if strings.Contains(lower, "stream failed") && strings.Contains(lower, "retrying") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected stream-failed retry warning, got: %v", warnings)
	}
	// Should have received the recovered text
	if len(texts) == 0 || texts[0] != "recovered after interrupt" {
		t.Errorf("expected text 'recovered after interrupt', got %v", texts)
	}
}

func TestStreamInterruptRetriesTenTimesWithExponentialDelays(t *testing.T) {
	p := &alwaysStreamInterruptProvider{}
	ql := New(p, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024})
	var retries []*stream.RequestStatusEvent
	err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventRequestRetry && event.RequestStatus != nil {
			copyStatus := *event.RequestStatus
			retries = append(retries, &copyStatus)
		}
	})
	if err == nil {
		t.Fatal("expected exhausted stream error")
	}
	if calls := p.calls.Load(); calls != 11 {
		t.Fatalf("stream calls = %d, want initial call plus ten retries", calls)
	}
	wantDelays := []int64{1, 2, 4, 8, 16, 32, 32, 32, 32, 32}
	if len(retries) != len(wantDelays) {
		t.Fatalf("stream retry events = %+v", retries)
	}
	for index, wantDelay := range wantDelays {
		if retries[index].Attempt != index+1 || retries[index].MaxRetries != 10 || retries[index].RetryDelayMilliseconds != wantDelay {
			t.Fatalf("stream retry[%d] = %+v, want delay %dms", index, retries[index], wantDelay)
		}
	}
}

func TestStreamInterruptClearsPreviousResponseID(t *testing.T) {
	// When a stream interrupt occurs and we had a previous_response_id active,
	// the retry should clear it and disable chaining.
	// To ensure previous_response_id is active on turn 2, we must match the
	// request fingerprint by using the same model/system/tools across turns.
	// The mockProvider's turnIndex tracks CreateStream calls (not Run calls).

	p := &mockProvider{
		responses: [][]types.StreamEvent{
			// Turn 1: normal response with ResponseID
			{
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "first turn"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventMessageStop, ResponseID: "resp_abc123"},
			},
			// Turn 2, attempt 1: upstream disconnect
			{
				{Type: types.EventError, Error: &types.APIError{
					Type:    "api_error",
					Message: "Upstream websocket closed before response.completed (close_code=1000)",
				}},
			},
			// Turn 2, attempt 2 (retry): normal response
			textEvents("recovered"),
		},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024, SessionID: "session-1"})

	// First turn: establishes lastResponseID
	err := ql.Run(context.Background(), "turn 1", func(e stream.Event) {})
	if err != nil {
		t.Fatalf("turn 1 failed: %v", err)
	}

	// Verify lastResponseID was captured
	if ql.lastResponseID != "resp_abc123" {
		t.Fatalf("expected lastResponseID='resp_abc123', got %q", ql.lastResponseID)
	}

	// Turn 2: stream interrupts then retry succeeds.
	// With the new envelopeFingerprint (model/system/tools only, no messages),
	// the fingerprint matches between turns, so previous_response_id IS used.
	// The stream interrupt handler will clear it and retry.
	var warnings []string
	err = ql.Run(context.Background(), "turn 2", func(e stream.Event) {
		if e.Type == stream.EventSystemWarning {
			warnings = append(warnings, projectedSystemWarningText(e))
		}
	})
	if err != nil {
		t.Fatalf("turn 2 failed: %v", err)
	}

	// The stream interrupt should have been handled with a warning + retry
	foundInterruptWarning := false
	for _, w := range warnings {
		lower := strings.ToLower(w)
		if strings.Contains(lower, "stream failed") && strings.Contains(lower, "retrying") {
			foundInterruptWarning = true
		}
	}
	if !foundInterruptWarning {
		t.Errorf("expected stream-interrupt retry warning, got: %v", warnings)
	}
}

// ---- Recovery path 5 (tool): tool error isolation ----------------------------------

// errorTool always returns an error from Execute.
type errorTool struct{}

func (t *errorTool) Name() string        { return "ErrorTool" }
func (t *errorTool) Description() string { return "always errors" }
func (t *errorTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *errorTool) Execute(_ context.Context, _ map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, errors.New("tool business error")
}

func TestToolErrorIsolatedNotInfrastructure(t *testing.T) {
	import_json_delta := `{}`
	toolTurn := []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0,
			ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "call_1", Name: "ErrorTool",
			}},
		{Type: types.EventContentBlockDelta, Index: 0,
			Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: import_json_delta}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			toolTurn,
			textEvents("error handled"),
		},
	}
	reg := registry.New()
	reg.Register(&errorTool{})

	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var toolResults []types.ToolResultBlock
	err := ql.Run(context.Background(), "run error tool", func(e stream.Event) {
		if e.Type == stream.EventToolResult {
			toolResults = append(toolResults, *e.ToolResult)
		}
	})
	// Loop should not abort — tool error is isolated
	if err != nil {
		t.Fatalf("unexpected error: loop should not abort on tool business error: %v", err)
	}
	if len(toolResults) == 0 {
		t.Fatal("expected a tool result event")
	}
	if !toolResults[0].IsError {
		t.Error("expected tool result to have IsError=true")
	}
}
