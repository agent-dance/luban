package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestContextWindowRemaining(t *testing.T) {
	cw := NewContextWindow(100000)
	cw.UsedInput = 60000
	// Remaining = effectiveContextWindowSize - UsedInput
	// effectiveContextWindowSize = MaxTokens(100000) - WarningThresholdBufferTokens(20000) = 80000
	// Remaining = 80000 - 60000 = 20000
	if r := cw.Remaining(); r != 20000 {
		t.Errorf("expected 20000, got %d", r)
	}
}

func TestContextWindowUpdateUsageNil(t *testing.T) {
	cw := NewContextWindow(100000)
	cw.UsedInput = 500
	cw.UpdateUsage(nil) // should not panic
	if cw.UsedInput != 500 {
		t.Error("UsedInput changed after nil update")
	}
}

func TestContextWindowUpdateUsageIncludesCachedPromptBuckets(t *testing.T) {
	cw := NewContextWindow(100000)
	cw.UpdateUsage(&types.Usage{
		InputTokens:              2134,
		CacheReadInputTokens:     1920,
		CacheCreationInputTokens: 128,
	})

	if cw.UsedInput != 2134 {
		t.Fatalf("UsedInput = %d, want 2134", cw.UsedInput)
	}
}

func TestContextWindowPostCompactEstimateIsLocalUntilProviderUsage(t *testing.T) {
	cw := NewContextWindow(200_000)
	cw.UpdateUsage(&types.Usage{InputTokens: 160_000})
	cw.UpdatePostCompactUsage(40_000)
	current := cw.CurrentInputUsage()
	if current.UsedTokens != 40_000 || current.Measurement != ContextUsageLocalEstimate || !current.EstimateComplete {
		t.Fatalf("post-compact context = %+v", current)
	}

	cw.UpdateUsage(&types.Usage{InputTokens: 92_000})
	current = cw.CurrentInputUsage()
	if current.UsedTokens != 92_000 || current.Measurement != ContextUsageProviderReported || !current.EstimateComplete {
		t.Fatalf("provider context did not replace estimate: %+v", current)
	}
}

func TestContextWindowSnapshotRestoresContextAfterFailedCompaction(t *testing.T) {
	cw := NewContextWindow(200_000)
	cw.UpdateUsage(&types.Usage{InputTokens: 160_000})
	snapshot := cw.CaptureCompactionTracker()
	cw.UpdatePostCompactUsage(40_000)
	cw.RestoreCompactionTracker(snapshot)
	current := cw.CurrentInputUsage()
	if current.UsedTokens != 160_000 || current.Measurement != ContextUsageProviderReported {
		t.Fatalf("failed compaction changed context: %+v", current)
	}
}

func TestContextWindowThresholdEdge(t *testing.T) {
	// MaxTokens=200000, MaxOutputTokens=0 → uses WarningThresholdBufferTokens(20000)
	// effectiveContextWindow = 200000 - 20000 = 180000
	// autoCompactThreshold = 180000 - 13000 = 167000
	cw := NewContextWindow(200000)

	cw.UsedInput = 167000 // exactly at threshold
	if cw.ShouldCompact() {
		t.Error("exactly at threshold should NOT trigger compact (> not >=)")
	}
	cw.UsedInput = 167001
	if !cw.ShouldCompact() {
		t.Error("above threshold should trigger compact")
	}

	// With custom MaxOutputTokens
	cw2 := NewContextWindow(200000)
	cw2.MaxOutputTokens = 16384
	// effectiveContextWindow = 200000 - 16384 = 183616
	// autoCompactThreshold = 183616 - 13000 = 170616
	cw2.UsedInput = 170616
	if cw2.ShouldCompact() {
		t.Error("with MaxOutputTokens=16384: exactly at threshold should NOT trigger compact")
	}
	cw2.UsedInput = 170617
	if !cw2.ShouldCompact() {
		t.Error("with MaxOutputTokens=16384: above threshold should trigger compact")
	}
}

func TestEstimateMessagesWithToolResults(t *testing.T) {
	cw := NewContextWindow(100000)
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "hello"},
				types.ToolResultBlock{Type: types.ContentTypeToolResult, Content: strings.Repeat("x", 400)},
			},
		},
	}
	est := cw.EstimateMessages(msgs)
	// With tiktoken, 400 chars of 'x' ≈ 50-100 tokens. Just verify it's non-trivial.
	if est < 10 {
		t.Errorf("expected at least 10 estimated tokens, got %d", est)
	}
}

func TestEstimateMessagesEmpty(t *testing.T) {
	cw := NewContextWindow(100000)
	if est := cw.EstimateMessages(nil); est != 0 {
		t.Errorf("expected 0 for nil messages, got %d", est)
	}
}

func TestSummaryCompactorSuccess(t *testing.T) {
	sc := &SummaryCompactor{
		Summarize: func(ctx context.Context, text string, customInstructions string) (string, error) {
			return "Summary of conversation", nil
		},
		KeepRecent: 2,
	}
	msgs := make([]types.Message, 5)
	for i := range msgs {
		msgs[i] = types.UserMessage(fmt.Sprintf("msg %d", i))
	}

	result, err := sc.Compact(context.Background(), msgs, 0)
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	postCompact := BuildPostCompactMessages(result)
	// 1 boundary + 1 summary + 2 recent = 4
	if len(postCompact) != 4 {
		t.Errorf("expected 4 messages, got %d", len(postCompact))
	}
	if !IsCompactBoundaryMessage(postCompact[0]) {
		t.Error("expected compact boundary as first message")
	}
	if !strings.Contains(postCompact[1].GetText(), "This session is being continued") {
		t.Error("expected summary continuation message after boundary")
	}
	if !strings.Contains(postCompact[1].GetText(), "Summary of conversation") {
		t.Error("expected summary content after boundary")
	}
}

func TestSummaryCompactorLegacyFallbackFlagFailsClosed(t *testing.T) {
	var events []CompactionTelemetryEvent
	sc := &SummaryCompactor{
		Summarize: func(ctx context.Context, text string, customInstructions string) (string, error) {
			return "", errors.New("LLM unavailable")
		},
		KeepRecent:               2,
		AllowHistorySnipFallback: true,
		OnTelemetry: func(event CompactionTelemetryEvent) {
			events = append(events, event)
		},
	}
	msgs := make([]types.Message, 10)
	for i := range msgs {
		msgs[i] = types.UserMessage(fmt.Sprintf("msg %d", i))
	}
	original := append([]types.Message(nil), msgs...)

	result, err := sc.Compact(context.Background(), msgs, 0)
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
	if failure := findCompactTelemetry(events, CompactionTelemetryFailure); failure == nil {
		t.Fatalf("missing failure telemetry: %+v", events)
	}
}

func TestSummaryCompactorNoOpWhenSmall(t *testing.T) {
	sc := &SummaryCompactor{
		Summarize: func(ctx context.Context, text string, customInstructions string) (string, error) {
			t.Error("Summarize should not be called")
			return "", nil
		},
		KeepRecent: 10,
	}
	msgs := []types.Message{
		types.UserMessage("a"),
		types.AssistantMessage("b"),
	}
	result, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	postCompact := BuildPostCompactMessages(result)
	if len(postCompact) != 2 {
		t.Errorf("expected 2, got %d", len(postCompact))
	}
}

func TestToolResultBudgetPreservesTextBlocks(t *testing.T) {
	budget := &ToolResultBudget{MaxCharsPerResult: 10}
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "preserved"},
				types.ToolResultBlock{Type: types.ContentTypeToolResult, Content: strings.Repeat("x", 100)},
			},
		},
	}
	result := budget.Apply(msgs)
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result[0].Content))
	}
	tb, ok := result[0].Content[0].(types.TextBlock)
	if !ok {
		t.Fatal("first block should be TextBlock")
	}
	if tb.Text != "preserved" {
		t.Errorf("TextBlock modified: '%s'", tb.Text)
	}
}

func TestHistorySnipKeepRecentOverride(t *testing.T) {
	snip := &HistorySnip{KeepFirst: 1, KeepLast: 2}
	msgs := make([]types.Message, 10)
	for i := range msgs {
		msgs[i] = types.UserMessage(fmt.Sprintf("msg %d", i))
	}
	// Override keepRecent to 5
	result, _ := snip.Compact(context.Background(), msgs, 5)
	postCompact := BuildPostCompactMessages(result)
	// 1 boundary + 1 marker + 1 first + 5 last = 8
	if len(postCompact) != 8 {
		t.Errorf("expected 8 messages with keepRecent=5, got %d", len(postCompact))
	}
}

func TestContextWindowCircuitBreaker(t *testing.T) {
	cw := NewContextWindow(200000)
	cw.UsedInput = 200000 // way above threshold

	// Should compact before any failures
	if !cw.ShouldCompact() {
		t.Error("expected ShouldCompact() = true before failures")
	}

	// Record MaxConsecutiveAutocompactFailures failures
	for i := 0; i < MaxConsecutiveAutocompactFailures; i++ {
		cw.RecordCompactFailure()
	}

	// Circuit breaker should now prevent compaction
	if cw.ShouldCompact() {
		t.Errorf("expected ShouldCompact() = false after %d consecutive failures (circuit breaker)", MaxConsecutiveAutocompactFailures)
	}

	// Reset on success
	cw.RecordCompactSuccess()
	if !cw.ShouldCompact() {
		t.Error("expected ShouldCompact() = true after RecordCompactSuccess()")
	}
}
