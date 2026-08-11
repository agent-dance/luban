package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/tokenizer"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestContextWindowShouldCompact(t *testing.T) {
	cw := NewContextWindow(100000)
	cw.UsedInput = 50000
	if cw.ShouldCompact() {
		t.Error("50% should not trigger compact at 80% threshold")
	}
	cw.UsedInput = 85000
	if !cw.ShouldCompact() {
		t.Error("85% should trigger compact at 80% threshold")
	}
}

func TestToolResultBudget(t *testing.T) {
	budget := &ToolResultBudget{MaxCharsPerResult: 50}
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   strings.Repeat("x", 100),
				},
			},
		},
	}

	result := budget.Apply(msgs)
	tr, ok := result[0].Content[0].(types.ToolResultBlock)
	if !ok {
		t.Fatal("expected ToolResultBlock")
	}
	if len(tr.Content) >= 100 {
		t.Errorf("expected truncated content, got len %d", len(tr.Content))
	}
	if !strings.Contains(tr.Content, "truncated") {
		t.Error("expected truncation notice")
	}
}

func TestToolResultBudgetNoTruncation(t *testing.T) {
	budget := &ToolResultBudget{MaxCharsPerResult: 200}
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:    types.ContentTypeToolResult,
					Content: "short",
				},
			},
		},
	}
	result := budget.Apply(msgs)
	tr := result[0].Content[0].(types.ToolResultBlock)
	if tr.Content != "short" {
		t.Errorf("expected 'short', got '%s'", tr.Content)
	}
}

func TestCalibratedCounterDefault(t *testing.T) {
	c := NewCalibratedCounter(4.0)
	// 100 chars / 4.0 ratio = 25 tokens
	if count := c.Count(strings.Repeat("a", 100)); count != 25 {
		t.Errorf("expected 25, got %d", count)
	}
}

func TestCalibratedCounterZeroRatio(t *testing.T) {
	c := NewCalibratedCounter(0)
	// Should default to 4.0
	if c.Ratio() != 4.0 {
		t.Errorf("expected default ratio 4.0, got %f", c.Ratio())
	}
}

func TestCalibratedCounterCalibration(t *testing.T) {
	c := NewCalibratedCounter(4.0)

	// Initial: 100 chars → 25 tokens (ratio 4.0)
	if count := c.Count(strings.Repeat("a", 100)); count != 25 {
		t.Errorf("before calibration: expected 25, got %d", count)
	}

	// Calibrate: API says 100 chars = 50 tokens → ratio becomes 2.0
	c.Calibrate(100, 50)
	if ratio := c.Ratio(); ratio != 2.0 {
		t.Errorf("expected ratio 2.0 after calibration, got %f", ratio)
	}

	// Now 100 chars → 50 tokens
	if count := c.Count(strings.Repeat("a", 100)); count != 50 {
		t.Errorf("after calibration: expected 50, got %d", count)
	}
}

func TestCalibratedCounterConverges(t *testing.T) {
	c := NewCalibratedCounter(4.0)

	// Simulate multiple API calls with consistent ratio of ~2.5
	for i := 0; i < 10; i++ {
		c.Calibrate(250, 100) // 250 chars = 100 tokens → ratio 2.5
	}

	ratio := c.Ratio()
	if ratio < 2.4 || ratio > 2.6 {
		t.Errorf("expected ratio ~2.5, got %f", ratio)
	}

	// 100 chars at ratio 2.5 = 40 tokens
	count := c.Count(strings.Repeat("a", 100))
	if count != 40 {
		t.Errorf("expected 40 tokens, got %d", count)
	}
}

func TestCalibratedCounterIgnoresInvalidInput(t *testing.T) {
	c := NewCalibratedCounter(4.0)
	c.Calibrate(0, 100)  // zero chars
	c.Calibrate(100, 0)  // zero tokens
	c.Calibrate(-1, 100) // negative chars
	c.Calibrate(100, -1) // negative tokens

	// Ratio should remain at default
	if c.Ratio() != 4.0 {
		t.Errorf("expected ratio to remain 4.0, got %f", c.Ratio())
	}
}

func TestCalibratedCounterMixedCalibration(t *testing.T) {
	c := NewCalibratedCounter(4.0)

	// First call: English text (ratio ~4)
	c.Calibrate(400, 100) // 4.0
	// Second call: Chinese text (ratio ~2)
	c.Calibrate(200, 100) // 2.0
	// Accumulated: 600 chars / 200 tokens = 3.0
	if ratio := c.Ratio(); ratio != 3.0 {
		t.Errorf("expected blended ratio 3.0, got %f", ratio)
	}
}

func TestNewContextWindowUsesTiktokenCounter(t *testing.T) {
	cw := NewContextWindow(200000)
	_, ok := cw.Counter.(*tokenizer.TiktokenCounter)
	if !ok {
		t.Error("expected NewContextWindow to use TiktokenCounter")
	}
}

// ── PTL retry tests ──────────────────────────────────────────────────────────

func TestIsPromptTooLongError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "plain prompt text", err: fmt.Errorf("prompt is too long for this model"), want: true},
		{name: "plain prompt type", err: fmt.Errorf("error: prompt_too_long"), want: true},
		{name: "plain uppercase", err: fmt.Errorf("PROMPT IS TOO LONG"), want: true},
		{name: "api prompt type", err: &types.APIError{Type: "prompt_too_long", Message: "request rejected"}, want: true},
		{name: "api context type", err: &types.APIError{Type: "context_window_full", Message: "request rejected"}, want: true},
		{name: "api context message", err: &types.APIError{Type: "invalid_request_error", Message: "context_window_full"}, want: true},
		{name: "untyped context wording", err: fmt.Errorf("context window exceeded"), want: false},
		{name: "empty", err: fmt.Errorf(""), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPromptTooLongError(tc.err); got != tc.want {
				t.Errorf("isPromptTooLongError(%T: %v) = %v, want %v", tc.err, tc.err, got, tc.want)
			}
		})
	}
}

func TestSummaryCompactorPTLFailsClosedWithoutDroppingHistory(t *testing.T) {
	providerErr := errors.New("prompt is too long")
	var received []types.Message
	s := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			received = append([]types.Message(nil), messages...)
			return "", providerErr
		},
	}

	msgs := []types.Message{
		types.UserMessage("earliest user round"), types.AssistantMessage("earliest assistant round"),
		types.UserMessage("middle user round"), types.AssistantMessage("middle assistant round"),
		types.UserMessage("latest old user round"), types.AssistantMessage("latest old assistant round"),
		types.UserMessage("preserved user tail"), types.AssistantMessage("preserved assistant tail"),
	}
	originalText := make([]string, len(msgs))
	for index := range msgs {
		originalText[index] = msgs[index].GetText()
	}

	result, err := s.Compact(context.Background(), msgs, 2)
	if !IsCompactPromptTooLongError(err) || !errors.Is(err, providerErr) {
		t.Fatalf("error = %T %v, want typed PTL preserving provider cause", err, err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want no publishable compact projection", result)
	}
	if len(received) != 6 || received[0].GetText() != "earliest user round" || received[5].GetText() != "latest old assistant round" {
		t.Fatalf("summarizer did not receive the complete old segment: %#v", received)
	}
	for index := range msgs {
		if got := msgs[index].GetText(); got != originalText[index] {
			t.Fatalf("input history mutated at %d: got %q want %q", index, got, originalText[index])
		}
	}
}

func TestSummaryCompactorTelemetrySuccessIncludesCountsAndUsage(t *testing.T) {
	var events []CompactionTelemetryEvent
	s := &SummaryCompactor{
		KeepRecent: 1,
		SummarizeMessages: func(ctx context.Context, _ []types.Message, _ string) (string, error) {
			recordCompactUsage(ctx, &types.Usage{
				InputTokens:              100,
				OutputTokens:             20,
				CacheCreationInputTokens: 30,
				CacheReadInputTokens:     70,
				ServerToolUse: types.ServerToolUsage{
					WebSearchRequests: 2,
				},
			})
			return "summary", nil
		},
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}

	result, err := s.CompactWithTrigger(context.Background(), []types.Message{
		types.UserMessage("one"),
		types.AssistantMessage("two"),
		types.UserMessage("three"),
	}, 1, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if result.CompactionUsage == nil {
		t.Fatal("result.CompactionUsage = nil, want captured compact call usage")
	}
	var success *CompactionTelemetryEvent
	for i := range events {
		if events[i].Kind == CompactionTelemetrySuccess {
			success = &events[i]
		}
	}
	if success == nil {
		t.Fatalf("missing success telemetry in %+v", events)
	}
	if success.Trigger != "auto" || success.PreCompactTokenCount <= 0 || success.PostCompactTokenCount <= 0 || success.TruePostCompactTokenCount <= 0 {
		t.Fatalf("incomplete success telemetry: %+v", *success)
	}
	if success.OriginalMessageCount != 3 || success.CompactedMessageCount != len(BuildPostCompactMessages(result)) {
		t.Fatalf("message counts not recorded: %+v", *success)
	}
	if success.CompactionUsage == nil || success.CompactionUsage.CacheReadInputTokens != 70 || success.CompactionUsage.CacheCreationInputTokens != 30 || success.CompactionUsage.ServerToolUse.WebSearchRequests != 2 {
		t.Fatalf("usage/cache metrics not recorded: %+v", success.CompactionUsage)
	}
}

func TestSummaryCompactorRecordsFailedPTLUsageWithoutRetry(t *testing.T) {
	var calls int
	var events []CompactionTelemetryEvent
	s := &SummaryCompactor{
		KeepRecent: 1,
		SummarizeMessages: func(ctx context.Context, _ []types.Message, _ string) (string, error) {
			calls++
			recordCompactUsage(ctx, &types.Usage{
				InputTokens:              100,
				OutputTokens:             2,
				CacheCreationInputTokens: 25,
			})
			return "", fmt.Errorf("prompt_too_long")
		},
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}

	result, err := s.CompactWithTrigger(context.Background(), []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("tail"),
	}, 1, "auto")
	if !IsCompactPromptTooLongError(err) || result != nil || calls != 1 {
		t.Fatalf("fail-closed PTL result=%#v error=%v calls=%d", result, err, calls)
	}
	want := types.Usage{
		InputTokens:              100,
		OutputTokens:             2,
		CacheCreationInputTokens: 25,
	}
	failure := findCompactTelemetry(events, CompactionTelemetryFailure)
	if failure == nil || failure.CompactionUsage == nil || *failure.CompactionUsage != want {
		t.Fatalf("PTL failure telemetry = %+v, want usage %+v", failure, want)
	}
}

func TestSummaryCompactorMergesStreamUsageWithoutDoubleCounting(t *testing.T) {
	reason := types.StopReasonEndTurn
	prov := newSummaryProviderFake(summaryProviderTurn{Events: []types.StreamEvent{
		{
			Type: types.EventMessageStart,
			Usage: &types.Usage{
				InputTokens:          100,
				CacheReadInputTokens: 70,
			},
		},
		{Type: types.EventContentBlockDelta, Delta: &types.ContentDelta{Type: "text_delta", Text: `{"schema":"compact-summary/v2","summary":"summary"}`}},
		{
			Type:       types.EventMessageDelta,
			StopReason: &reason,
			Usage: &types.Usage{
				InputTokens:          105,
				OutputTokens:         9,
				CacheReadInputTokens: 75,
			},
		},
	}})
	s := &SummaryCompactor{
		KeepRecent:        1,
		SummarizeMessages: NewLLMStructuredSummarizeFunc(prov),
	}

	result, err := s.Compact(context.Background(), []types.Message{
		types.UserMessage("old"), types.AssistantMessage("old"), types.UserMessage("tail"),
	}, 1)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	want := types.Usage{InputTokens: 105, OutputTokens: 9, CacheReadInputTokens: 75}
	if result.CompactionUsage == nil || *result.CompactionUsage != want {
		t.Fatalf("merged stream usage = %+v, want %+v", result.CompactionUsage, want)
	}
}

func TestSummaryCompactorTelemetryPTLFailureIsNilUsageSafeAndDoesNotRetry(t *testing.T) {
	var events []CompactionTelemetryEvent
	s := &SummaryCompactor{
		KeepRecent: 1,
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			return "", fmt.Errorf("prompt is too long")
		},
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}
	msgs := []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("u3"), types.AssistantMessage("a3"),
	}

	_, err := s.Compact(context.Background(), msgs, 1)
	if !IsCompactPromptTooLongError(err) {
		t.Fatalf("error = %T %v, want compact PTL exhaustion", err, err)
	}
	var failureSeen bool
	for _, event := range events {
		switch event.Kind {
		case CompactionTelemetryFailure:
			failureSeen = true
			if event.CompactionUsage != nil {
				t.Fatalf("failure telemetry should be nil-safe when provider usage is unavailable: %+v", event)
			}
			if event.ErrorType == "" || event.PreCompactTokenCount <= 0 {
				t.Fatalf("incomplete failure telemetry: %+v", event)
			}
		}
	}
	if !failureSeen {
		t.Fatalf("failureSeen=%v events=%+v", failureSeen, events)
	}
}

func TestSummaryCompactorPTLReturnsTypedErrorWithoutLossyFallback(t *testing.T) {
	calls := 0
	s := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			calls++
			return "", fmt.Errorf("prompt is too long every time")
		},
	}

	msgs := []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("u3"), types.AssistantMessage("a3"),
		types.UserMessage("u4"), types.AssistantMessage("a4"),
	}

	result, err := s.Compact(context.Background(), msgs, 2)
	if err == nil {
		t.Fatal("expected typed compact PTL error")
	}
	if !IsCompactPromptTooLongError(err) {
		t.Fatalf("error = %T %v, want CompactPromptTooLongError", err, err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil so no lossy fallback was used", result)
	}
	if calls != 1 {
		t.Fatalf("summarize calls = %d, want exactly one complete-input attempt", calls)
	}
}

func TestSummaryCompactorAPIContextWindowFullFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		err  *types.APIError
	}{
		{
			name: "type",
			err:  &types.APIError{Type: "context_window_full", Message: "request rejected", Status: 400},
		},
		{
			name: "message",
			err:  &types.APIError{Type: "invalid_request_error", Message: "context_window_full", Status: 400},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			s := &SummaryCompactor{
				KeepRecent: 2,
				SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
					calls++
					return "", tt.err
				},
			}
			msgs := []types.Message{
				types.UserMessage("u1"), types.AssistantMessage("a1"),
				types.UserMessage("u2"), types.AssistantMessage("a2"),
				types.UserMessage("u3"), types.AssistantMessage("a3"),
			}
			originalText := make([]string, len(msgs))
			for index := range msgs {
				originalText[index] = msgs[index].GetText()
			}

			result, err := s.Compact(context.Background(), msgs, 2)
			if !IsCompactPromptTooLongError(err) || !errors.Is(err, tt.err) {
				t.Fatalf("error = %T %v, want typed PTL preserving API error", err, err)
			}
			if result != nil {
				t.Fatalf("result = %#v, want nil", result)
			}
			if calls != 1 {
				t.Fatalf("summarizer calls = %d, want 1", calls)
			}
			for index := range msgs {
				if got := msgs[index].GetText(); got != originalText[index] {
					t.Fatalf("input history mutated at %d: got %q want %q", index, got, originalText[index])
				}
			}
		})
	}
}

func TestSummaryCompactorCancellationDoesNotUseLossyFallback(t *testing.T) {
	calls := 0
	s := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			calls++
			return "", context.Canceled
		},
	}

	msgs := []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("u3"), types.AssistantMessage("a3"),
	}

	result, err := s.Compact(context.Background(), msgs, 2)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !IsCompactUserAbortError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %T %v, want compact user abort preserving context.Canceled", err, err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil so no lossy fallback was used", result)
	}
	if calls != 1 {
		t.Fatalf("summarize calls = %d, want 1", calls)
	}
}

func TestSummaryCompactorPTLSingleHumanTurnManyToolLoopsPreservesWholeRound(t *testing.T) {
	var attemptedMessages []types.Message
	calls := 0
	s := &SummaryCompactor{
		KeepRecent: 1,
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			calls++
			attemptedMessages = append([]types.Message(nil), messages...)
			return "", fmt.Errorf("prompt_too_long")
		},
	}

	msgs := []types.Message{
		types.UserMessage("single human task"),
		{ID: "a1", Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Read"}}},
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tu_1", Content: "file 1"}),
		{ID: "a2", Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_2", Name: "Read"}}},
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tu_2", Content: "file 2"}),
		{ID: "a3", Role: types.RoleAssistant, Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "done"}}},
		types.UserMessage("recent tail"),
	}

	result, err := s.Compact(context.Background(), msgs, 1)
	if !IsCompactPromptTooLongError(err) || result != nil || calls != 1 {
		t.Fatalf("fail-closed PTL result=%#v error=%v calls=%d", result, err, calls)
	}
	if len(attemptedMessages) != len(msgs)-1 {
		t.Fatalf("complete old round length = %d, want %d", len(attemptedMessages), len(msgs)-1)
	}
	if attemptedMessages[0].GetText() != "single human task" || attemptedMessages[len(attemptedMessages)-1].ID != "a3" {
		t.Fatalf("complete tool-loop round was not preserved: %#v", attemptedMessages)
	}
}

func TestSummaryCompactorClassifiesNoSummaryIncompleteAndAPIErrorSummary(t *testing.T) {
	tests := []struct {
		name string
		text string
		err  func(error) bool
	}{
		{
			name: "no summary response",
			text: "",
			err:  IsCompactNoSummaryError,
		},
		{
			name: "api error summary text",
			text: "API Error: 400 upstream rejected compact request",
			err:  IsCompactAPIError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			s := &SummaryCompactor{
				KeepRecent: 2,
				SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
					return tt.text, nil
				},
			}
			msgs := []types.Message{
				types.UserMessage("u1"), types.AssistantMessage("a1"),
				types.UserMessage("u2"), types.AssistantMessage("a2"),
				types.UserMessage("u3"), types.AssistantMessage("a3"),
			}

			result, err := s.Compact(context.Background(), msgs, 2)
			if err == nil || !tt.err(err) {
				t.Fatalf("error = %T %v, want classified compact error", err, err)
			}
			if result != nil {
				t.Fatalf("result = %#v, want nil", result)
			}
		})
	}

	reason := types.StopReasonMaxTokens
	summarize := NewLLMStructuredSummarizeFunc(newSummaryProviderFake(summaryProviderTurn{
		Events: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "<summary>partial"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, StopReason: &reason},
			{Type: types.EventMessageStop},
		},
	}))
	if _, err := summarize(context.Background(), []types.Message{types.UserMessage("conversation")}, ""); err == nil || !IsCompactIncompleteResponseError(err) {
		t.Fatalf("incomplete summary error = %T %v, want CompactIncompleteResponse", err, err)
	}
}

func TestSummaryCompactorNonPTLErrorFailsClosed(t *testing.T) {
	calls := 0
	var events []CompactionTelemetryEvent
	s := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			calls++
			return "", fmt.Errorf("network timeout")
		},
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}

	msgs := []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("u3"), types.AssistantMessage("a3"),
	}
	original := append([]types.Message(nil), msgs...)

	result, err := s.Compact(context.Background(), msgs, 2)
	if err == nil {
		t.Fatal("expected summarizer error")
	}
	if calls != 1 {
		t.Errorf("summarizer calls = %d, want 1", calls)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	for index := range msgs {
		if msgs[index].GetText() != original[index].GetText() {
			t.Fatalf("input history mutated at %d", index)
		}
	}
	if failure := findCompactTelemetry(events, CompactionTelemetryFailure); failure == nil {
		t.Fatalf("missing failure telemetry: %+v", events)
	}
}

func TestSummaryCompactorPreservesFailedProviderUsageTelemetry(t *testing.T) {
	wantUsage := types.Usage{
		InputTokens:              75,
		OutputTokens:             3,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     50,
	}
	var events []CompactionTelemetryEvent
	s := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(ctx context.Context, _ []types.Message, _ string) (string, error) {
			recordCompactUsage(ctx, &wantUsage)
			return "", fmt.Errorf("network timeout after usage")
		},
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}

	msgs := []types.Message{
		types.UserMessage("u1"), types.AssistantMessage("a1"),
		types.UserMessage("u2"), types.AssistantMessage("a2"),
		types.UserMessage("u3"), types.AssistantMessage("a3"),
	}
	original := append([]types.Message(nil), msgs...)
	result, err := s.Compact(context.Background(), msgs, 2)
	if err == nil {
		t.Fatal("expected summarizer error")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	for index := range msgs {
		if msgs[index].GetText() != original[index].GetText() {
			t.Fatalf("input history mutated at %d", index)
		}
	}
	failure := findCompactTelemetry(events, CompactionTelemetryFailure)
	if failure == nil || failure.CompactionUsage == nil || *failure.CompactionUsage != wantUsage {
		t.Fatalf("failure telemetry = %+v, want usage %+v", failure, wantUsage)
	}
}

type recordingSummaryProvider struct {
	calls []provider.Params
}

func (p *recordingSummaryProvider) Name() string    { return "recording" }
func (p *recordingSummaryProvider) ModelID() string { return "recording-model" }
func (p *recordingSummaryProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.calls = append(p.calls, params)
	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{
		Type:  types.EventContentBlockDelta,
		Delta: &types.ContentDelta{Type: types.ContentTypeText, Text: `{"schema":"compact-summary/v2","summary":"structured summary"}`},
	}
	close(ch)
	return ch, nil
}

func TestStructuredLLMSummarizePreservesToolUseInputAndDisablesTools(t *testing.T) {
	prov := &recordingSummaryProvider{}
	summarize := NewLLMStructuredSummarizeFunc(prov)
	messages := []types.Message{
		{
			ID:   "assistant-1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "Reading config"},
				types.ToolUseBlock{
					Type: types.ContentTypeToolUse,
					ID:   "toolu_1",
					Name: "Read",
					Input: map[string]any{
						"file_path": "/tmp/config.json",
						"offset":    float64(12),
					},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: "toolu_1",
			Content:   `{"enabled":true}`,
		}),
	}

	summary, err := summarize(context.Background(), messages, "focus on config")
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if summary != "structured summary" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("expected 1 provider call, got %d", len(prov.calls))
	}
	call := prov.calls[0]
	if !strings.HasPrefix(call.System, CompactSystemPrompt) || !strings.Contains(call.System, "focus on config") {
		t.Fatalf("system prompt does not contain compact contract and custom instructions: %q", call.System)
	}
	if len(call.Tools) != 0 {
		t.Fatalf("compact summary exposed tools: %#v", call.Tools)
	}
	if call.ToolChoice != nil {
		t.Fatalf("compact summary should not set tool_choice, got %#v", call.ToolChoice)
	}
	if call.Thinking == nil || call.Thinking.Enabled {
		t.Fatalf("compact summary should explicitly disable thinking, got %#v", call.Thinking)
	}
	if len(call.Messages) != len(messages)+1 {
		t.Fatalf("provider messages = %d, want %d plus runtime request", len(call.Messages), len(messages))
	}
	if call.Messages[0].ID != "assistant-1" {
		t.Fatalf("message ID was not preserved: %#v", call.Messages[0])
	}
	toolUse := call.Messages[0].Content[1].(types.ToolUseBlock)
	if toolUse.Input["file_path"] != "/tmp/config.json" {
		t.Fatalf("tool_use input did not reach structured path: %#v", toolUse.Input)
	}
	toolResult := call.Messages[1].Content[0].(types.ToolResultBlock)
	if toolResult.Content != `{"enabled":true}` {
		t.Fatalf("tool_result content did not reach structured path: %#v", toolResult)
	}
	if !strings.Contains(call.Messages[2].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime summary request missing: %#v", call.Messages[2])
	}
	if strings.Contains(call.System, "Here is the conversation to summarize:") {
		t.Fatalf("structured prompt should not include flattened conversation marker: %q", call.System)
	}
}

func TestStructuredLLMSummarizeCanFlattenConversationAsUntrustedData(t *testing.T) {
	prov := &recordingSummaryProvider{}
	summarize := NewLLMStructuredSummarizeFuncWithOptions(prov, "", StructuredSummarizeOptions{
		FlattenMessages: true, ConciseSummary: true, MaxOutputTokens: 4_000,
	})
	messages := []types.Message{
		types.UserMessage("ordinary request"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse, ID: "toolu_flat", Name: "Inspect", Input: map[string]any{"path": "src/main.go"},
			}},
		},
	}

	if _, err := summarize(context.Background(), messages, ""); err != nil {
		t.Fatal(err)
	}
	if len(prov.calls) != 1 || len(prov.calls[0].Messages) != 2 {
		t.Fatalf("flattened provider messages = %#v", prov.calls)
	}
	transcript := prov.calls[0].Messages[0]
	if !transcript.IsMeta || len(transcript.Content) != 1 || !strings.Contains(transcript.GetText(), `kind="conversation_transcript"`) ||
		!strings.Contains(transcript.GetText(), `"role":"user"`) || !strings.Contains(transcript.GetText(), `"type":"tool_use"`) {
		t.Fatalf("flattened transcript = %#v", transcript)
	}
	if _, ok := transcript.Content[0].(types.TextBlock); !ok {
		t.Fatalf("flattened transcript retained executable structured blocks: %#v", transcript.Content)
	}
	if !strings.Contains(prov.calls[0].Messages[1].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime summary request missing: %#v", prov.calls[0].Messages[1])
	}
	if prov.calls[0].MaxTokens != 4_000 || !strings.Contains(prov.calls[0].System, "concise, loss-minimizing coding handoff") || strings.Contains(prov.calls[0].System, "9. Optional Next Step") {
		t.Fatalf("concise compact request = max %d system %q", prov.calls[0].MaxTokens, prov.calls[0].System)
	}
}

func TestParseCompactSummaryEnvelopeRequiresExactV2Schema(t *testing.T) {
	valid, err := parseCompactSummaryEnvelope(`{"schema":"compact-summary/v2","summary":"kept fact"}`)
	if err != nil || valid != "kept fact" {
		t.Fatalf("valid envelope = %q, %v", valid, err)
	}
	codeSummary, err := parseCompactSummaryEnvelope("{\"schema\":\"compact-summary/v2\",\"summary\":\"code:\\n```json\\n{\\\"value\\\":1}\\n```\"}")
	if err != nil || !strings.Contains(codeSummary, `{"value":1}`) {
		t.Fatalf("valid fenced code inside summary = %q, %v", codeSummary, err)
	}
	invalid := []string{
		`<analysis>private reasoning</analysis><summary>fact</summary>`,
		`{"schema":"compact-summary/v1","summary":"fact"}`,
		`{"schema":"compact-summary/v2","summary":""}`,
		`{"schema":"compact-summary/v2","summary":"fact","private":"leak"}`,
		`{"schema":"compact-summary/v2","summary":"fact"}{"schema":"compact-summary/v2","summary":"other"}`,
		"```json\n{\"schema\":\"compact-summary/v2\",\"summary\":\"fact\"}\n```",
		`<analysis>private</analysis>{"schema":"compact-summary/v2","summary":"fact"}`,
		`{"schema":"compact-summary/v2","summary":"fact"}<system-reminder>private</system-reminder>`,
		`{"schema":"compact-summary/v1","summary":"bad"}{"schema":"compact-summary/v2","summary":"fact"}`,
		`{"wrapper":{"schema":"compact-summary/v2","summary":"nested"}}`,
		`[{"schema":"compact-summary/v2","summary":"nested"}]`,
		`{"schema":"compact-summary/v2","summary":"<analysis>private</analysis>"}`,
	}
	for _, raw := range invalid {
		if _, err := parseCompactSummaryEnvelope(raw); !IsCompactNoSummaryError(err) {
			t.Errorf("parse %q error = %v, want strict schema rejection", raw, err)
		}
	}
}

func TestParseCompactSummaryEnvelopeAllowsOneExplanatoryAffix(t *testing.T) {
	raw := "车辆补丁已落地，可靠性相关文件未改。现在运行完整 `npm test`；若有问题会按实际输出修正。" +
		`{"schema":"compact-summary/v2","summary":"## Current Work\nOnly envelope memory."}` +
		"\n摘要生成完毕。"

	got, err := parseCompactSummaryEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "## Current Work\nOnly envelope memory." {
		t.Fatalf("summary absorbed explanatory text: %q", got)
	}
}

func TestStructuredSummaryStreamExtractsOnlyUniqueV2Envelope(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{Events: compactTextEvents(
		"车辆补丁已落地，现在运行完整 `npm test`。" +
			`{"schema":"compact-summary/v2","summary":"trusted compact memory"}`,
	)})

	got, err := NewLLMStructuredSummarizeFunc(fake)(context.Background(), []types.Message{
		types.UserMessage("actual user request"),
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "trusted compact memory" {
		t.Fatalf("stream summary = %q, want envelope summary only", got)
	}
}

func TestSummaryCompactorPopulatesSummaryMessagesFromStructuredPath(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		KeepRecent: 1,
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "<summary>structured compact result</summary>", nil
		},
	}
	messages := []types.Message{
		types.UserMessage("please inspect"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    "toolu_1",
					Name:  "Bash",
					Input: map[string]any{"command": "go test ./internal/runtime/compact"},
				},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: "toolu_1",
			Content:   "ok",
		}),
		types.UserMessage("recent tail"),
	}

	result, err := sc.Compact(context.Background(), messages, 1)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("structured summarizer saw %d old messages, want 3", len(seen))
	}
	toolUse := seen[1].Content[0].(types.ToolUseBlock)
	if toolUse.Input["command"] != "go test ./internal/runtime/compact" {
		t.Fatalf("tool_use input missing from structured compactor path: %#v", toolUse.Input)
	}
	if len(result.SummaryMessages) != 1 {
		t.Fatalf("summaryMessages len = %d, want 1", len(result.SummaryMessages))
	}
	if got := result.SummaryMessages[0].GetText(); !strings.Contains(got, "structured compact result") {
		t.Fatalf("summaryMessages not populated from structured output: %q", got)
	}
}
