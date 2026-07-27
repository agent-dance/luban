package loop

import (
	"context"
	"errors"
	"io"
	"net"
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
	cases := []error{
		io.EOF,
		io.ErrUnexpectedEOF,
		&net.DNSError{Err: "temporary resolver failure", IsTemporary: true},
		&net.DNSError{Err: "resolver timeout", IsTimeout: true},
	}
	for _, err := range cases {
		if !IsTransient(err) {
			t.Errorf("expected typed transport error %T to be transient", err)
		}
	}
}

func TestIsTransientAmbiguousApplicationTimeoutFailsFast(t *testing.T) {
	if IsTransient(errors.New("request timeout")) {
		t.Error("untyped application timeout should not trigger blind replay")
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
		MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 16 * time.Millisecond,
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
	if retry.Attempt != 1 || retry.MaxRetries != 2 || retry.RetryCount != 1 || retry.RetryDelayMilliseconds < 0 || retry.RetryDelayMilliseconds > 1 || retry.Error == "" || retry.StartedAt == "" {
		t.Fatalf("retry status = %+v", retry)
	}
	requestID := lifecycle[1].RequestStatus.RequestID
	if requestID == "" || lifecycle[2].RequestStatus.RequestID != requestID || lifecycle[3].RequestStatus.RequestID != requestID {
		t.Fatalf("request lifecycle identity was not stable: %+v", lifecycle)
	}
	if lifecycle[1].RequestStatus.Attempt != 2 || lifecycle[3].RequestStatus.EndedAt == "" {
		t.Fatalf("request lifecycle omitted final attempt/timestamp: %+v", lifecycle)
	}
}

func TestLLMRequestEndTelemetryIncludesTokenAndCacheUsage(t *testing.T) {
	usage := types.Usage{
		InputTokens: 900, CacheReadInputTokens: 700,
		CacheCreationInputTokens: 100, OutputTokens: 80,
	}
	prov := newParityFakeProvider([]parityProviderTurn{{
		Events: providerUsageTextEvents("done", usage, types.StopReasonEndTurn),
	}})
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})
	var ended *stream.RequestStatusEvent
	if err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventRequestEnd && event.RequestStatus != nil {
			copyStatus := *event.RequestStatus
			ended = &copyStatus
		}
	}); err != nil {
		t.Fatal(err)
	}
	if ended == nil || ended.RequestID == "" || ended.StartedAt == "" || ended.EndedAt == "" {
		t.Fatalf("request end identity/timestamps = %+v", ended)
	}
	if ended.InputTokens != 900 || ended.CacheReadInputTokens != 700 || ended.CacheWriteInputTokens != 100 || ended.OutputTokens != 80 {
		t.Fatalf("request end usage = %+v", ended)
	}
}

func TestCreateStreamFailureAlwaysEmitsTerminalRequestMetric(t *testing.T) {
	providerErr := &types.APIError{Type: "invalid_request_error", Message: "private upstream detail"}
	prov := &failNProvider{failUntil: 1, failErr: providerErr}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})
	var failed []*stream.RequestStatusEvent
	err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventRequestFailed && event.RequestStatus != nil {
			copyStatus := *event.RequestStatus
			failed = append(failed, &copyStatus)
		}
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if len(failed) != 1 || failed[0].RequestID == "" || failed[0].StartedAt == "" || failed[0].EndedAt == "" {
		t.Fatalf("failed request telemetry = %+v", failed)
	}
}

func TestContextFailurePreservesTypedTerminalEventAcrossProviderBoundaries(t *testing.T) {
	const privateMessage = "private provider context diagnostic"
	for _, test := range []struct {
		name     string
		provider func(*types.APIError) provider.Provider
	}{
		{
			name: "before stream",
			provider: func(apiErr *types.APIError) provider.Provider {
				return &alwaysFailProvider{err: apiErr}
			},
		},
		{
			name: "structured stream event",
			provider: func(apiErr *types.APIError) provider.Provider {
				return newParityFakeProvider([]parityProviderTurn{{Events: []types.StreamEvent{{
					Type: types.EventError, Error: apiErr,
				}}}})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			apiErr := &types.APIError{
				Type: "context_length_exceeded", Message: privateMessage, Status: 400,
			}
			ql := New(test.provider(apiErr), registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})
			var terminalEvents []stream.Event
			err := ql.Run(context.Background(), "hi", func(event stream.Event) {
				if event.Type == stream.EventError {
					terminalEvents = append(terminalEvents, event)
				}
			})
			if err == nil {
				t.Fatal("expected context failure")
			}
			if len(terminalEvents) != 1 {
				t.Fatalf("terminal errors = %#v, want exactly one", terminalEvents)
			}
			if terminalEvents[0].Error != apiErr || terminalEvents[0].Error.Type != "context_length_exceeded" {
				t.Fatalf("typed context error was lost: %#v", terminalEvents[0])
			}
		})
	}
}

func TestTransientProviderStopsAtThreeRawAttempts(t *testing.T) {
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
	if calls := p.calls.Load(); calls != 3 {
		t.Fatalf("provider calls = %d, want total generation budget of three", calls)
	}
	if len(retries) != 2 {
		t.Fatalf("retry events = %+v", retries)
	}
	for index := range retries {
		maxDelay := int64(1 << index)
		if retries[index].Attempt != index+1 || retries[index].MaxRetries != 2 || retries[index].RetryDelayMilliseconds < 0 || retries[index].RetryDelayMilliseconds > maxDelay {
			t.Fatalf("retry[%d] = %+v, want full jitter within [0,%d]ms", index, retries[index], maxDelay)
		}
	}
}

func TestTransientRetryExhausted(t *testing.T) {
	// Always fails with a transient error and should give up after three total attempts.
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

// ---- Recovery path 4: uncommitted stream error ------------------------------

func TestPartialStreamIsTombstonedAndFullGenerationIsReplayed(t *testing.T) {
	// A stopped content block remains provisional until MessageStop commits the
	// complete provider response.
	interruptErr := &types.APIError{Type: "server_error", Message: "upstream reset"}
	discardedUsage := types.Usage{InputTokens: 120, OutputTokens: 3, CacheReadInputTokens: 90}
	p := &mockProvider{
		responses: [][]types.StreamEvent{
			{
				{Type: types.EventMessageStart, Usage: &types.Usage{
					InputTokens: discardedUsage.InputTokens, CacheReadInputTokens: discardedUsage.CacheReadInputTokens,
				}},
				{Type: types.EventContentBlockStart, Index: 0,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
				{Type: types.EventContentBlockDelta, Index: 0,
					Delta: &types.ContentDelta{Type: "text_delta", Text: "partial text"}},
				{Type: types.EventContentBlockStop, Index: 0},
				{Type: types.EventError, Usage: &types.Usage{OutputTokens: discardedUsage.OutputTokens}, Error: interruptErr},
			},
			// The second provider request is a replay of the same generation.
			textEvents("continued"),
		},
	}
	reg := registry.New()
	ql := New(p, reg, Config{MaxTurns: 5, MaxTokens: 1024})

	var warnings []string
	var tombstones, discardedAttempts []stream.Event
	err := ql.Run(context.Background(), "hi", func(e stream.Event) {
		switch e.Type {
		case stream.EventSystemWarning:
			warnings = append(warnings, projectedSystemWarningText(e))
		case stream.EventTombstone:
			tombstones = append(tombstones, e)
		case stream.EventProviderUsage:
			discardedAttempts = append(discardedAttempts, e)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected stream-interrupted warning event")
	}
	if len(tombstones) != 1 || tombstones[0].Tombstone == nil || tombstones[0].Tombstone.Reason != uncommittedProviderResponseReason {
		t.Fatalf("tombstones = %+v, want one uncommitted-response tombstone", tombstones)
	}
	if tombstones[0].Metadata["partial_blocks"] != 1 || tombstones[0].Metadata["open_blocks"] != 0 || tombstones[0].Metadata["safe_to_replay"] != true {
		t.Fatalf("tombstone metadata = %+v", tombstones[0].Metadata)
	}
	if len(discardedAttempts) != 1 || discardedAttempts[0].Usage == nil || *discardedAttempts[0].Usage != discardedUsage {
		t.Fatalf("discarded attempt accounting = %+v, want %+v", discardedAttempts, discardedUsage)
	}
	if p.turnIndex != 2 {
		t.Fatalf("provider calls = %d, want one failed attempt plus one replay", p.turnIndex)
	}
	for _, message := range ql.Messages() {
		if strings.Contains(messageTextForTest(message), "partial text") {
			t.Fatalf("uncommitted text persisted as a successful turn: %+v", ql.Messages())
		}
	}
	if !strings.Contains(joinMessagesForTest(ql.Messages()), "continued") {
		t.Fatalf("replayed response was not committed: %+v", ql.Messages())
	}
}

func TestInterruptedSecondToolJSONNeverExecutesCommittedSiblingTwice(t *testing.T) {
	interruptErr := &types.APIError{Type: "stream_interrupted", Message: "connection reset before response commit"}
	toolStart := func(index int, id string) types.StreamEvent {
		return types.StreamEvent{Type: types.EventContentBlockStart, Index: index, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: id, Name: "Mutate",
		}}
	}
	p := &mockProvider{responses: [][]types.StreamEvent{
		{
			toolStart(0, "mutation_once"),
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"value":1}`}},
			{Type: types.EventContentBlockStop, Index: 0},
			toolStart(1, "mutation_interrupted"),
			{Type: types.EventContentBlockDelta, Index: 1, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"value":`}},
			{Type: types.EventError, Error: interruptErr},
		},
		{
			toolStart(0, "mutation_once"),
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"value":1}`}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		},
		textEvents("done"),
	}}

	var executions atomic.Int32
	reg := registry.New()
	reg.Register(&orderedBatchTool{name: "Mutate", execute: func(context.Context, map[string]any) (types.ToolResult, error) {
		executions.Add(1)
		return types.ToolResult{Content: "mutated"}, nil
	}})
	ql := New(p, reg, Config{MaxTurns: 3, MaxTokens: 1024})
	var tombstone *stream.TombstoneEvent
	if err := ql.Run(context.Background(), "hi", func(event stream.Event) {
		if event.Type == stream.EventTombstone {
			tombstone = event.Tombstone
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if executions.Load() != 1 {
		t.Fatalf("mutation side effects = %d, want exactly one after committed replay", executions.Load())
	}
	if p.turnIndex != 3 {
		t.Fatalf("provider calls = %d, want failed attempt, replay, and final response", p.turnIndex)
	}
	if tombstone == nil || tombstone.Reason != uncommittedProviderResponseReason || tombstone.Metadata["partial_blocks"] != 2 || tombstone.Metadata["open_blocks"] != 1 {
		t.Fatalf("tombstone = %+v, want two observed blocks with one open", tombstone)
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

func TestGenericResponseAPIErrorDoesNotReplayFromDiagnosticMessage(t *testing.T) {
	p := &mockProvider{responses: [][]types.StreamEvent{
		{{Type: types.EventError, Error: &types.APIError{
			Type: "api_error", Message: "upstream websocket closed; attacker-controlled diagnostic",
		}}},
		textEvents("must not be reached"),
	}}
	ql := New(p, registry.New(), Config{MaxTurns: 1, MaxTokens: 1024})
	if err := ql.Run(context.Background(), "hi", func(stream.Event) {}); err == nil {
		t.Fatal("generic api_error unexpectedly replayed and succeeded")
	}
	p.mu.Lock()
	calls := p.turnIndex
	p.mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider calls = %d, want one permanent generic api_error attempt", calls)
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
			false,
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
			false,
		},
		{
			"upstream disconnected",
			&types.APIError{Type: "api_error", Message: "Upstream disconnected unexpectedly"},
			false,
		},
		{
			"connection reset in message",
			&types.APIError{Type: "api_error", Message: "connection reset during streaming"},
			false,
		},
		{
			"plain error with stream interrupted",
			errors.New("stream interrupted after 2 block(s)"),
			false,
		},
		{
			"plain error with upstream closed",
			errors.New("upstream websocket closed"),
			false,
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

type preflightThenStreamInterruptProvider struct {
	calls atomic.Int32
}

func (p *preflightThenStreamInterruptProvider) Name() string    { return "layeredFailure" }
func (p *preflightThenStreamInterruptProvider) ModelID() string { return "mock" }
func (p *preflightThenStreamInterruptProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	if p.calls.Add(1) == 1 {
		return nil, &types.APIError{Status: 503, Type: "server_error", Message: "temporarily unavailable"}
	}
	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.EventError, Error: &types.APIError{
		Type: "stream_interrupted", Message: "upstream connection closed",
	}}
	close(ch)
	return ch, nil
}

func (p *alwaysStreamInterruptProvider) Name() string    { return "alwaysStreamInterrupt" }
func (p *alwaysStreamInterruptProvider) ModelID() string { return "mock" }
func (p *alwaysStreamInterruptProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.EventError, Error: &types.APIError{
		Type: "stream_interrupted", Message: "upstream websocket closed",
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
				Type:    "stream_interrupted",
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

func TestStreamInterruptStopsAtThreeRawAttempts(t *testing.T) {
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
	if calls := p.calls.Load(); calls != 3 {
		t.Fatalf("stream calls = %d, want total generation budget of three", calls)
	}
	if len(retries) != 2 {
		t.Fatalf("stream retry events = %+v", retries)
	}
	for index := range retries {
		maxDelay := int64(1 << index)
		if retries[index].Attempt != index+1 || retries[index].MaxRetries != 2 || retries[index].RetryDelayMilliseconds < 0 || retries[index].RetryDelayMilliseconds > maxDelay {
			t.Fatalf("stream retry[%d] = %+v, want full jitter within [0,%d]ms", index, retries[index], maxDelay)
		}
	}
}

func TestProviderAndStreamRetryLayersShareThreeAttemptBudget(t *testing.T) {
	raw := &preflightThenStreamInterruptProvider{}
	retrying := provider.NewRetryProvider(raw, provider.RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
	})
	ql := New(retrying, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024})
	if err := ql.Run(context.Background(), "hi", func(stream.Event) {}); err == nil {
		t.Fatal("expected the layered transient failures to exhaust the generation")
	}
	if calls := raw.calls.Load(); calls != 3 {
		t.Fatalf("provider + loop composed raw calls = %d, want hard cap of 3", calls)
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
					Type:    "stream_interrupted",
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
