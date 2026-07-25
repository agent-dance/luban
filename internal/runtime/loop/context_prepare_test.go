package loop

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type prepareCountingCompactor struct {
	calls          int
	receivedTexts  []string
	receivedMsgs   [][]types.Message
	receivedMedia  []bool
	fail           bool
	summaryMessage string
}

type charBasedCounterForTest struct{}

func (*charBasedCounterForTest) Count(text string) int { return len(text) / 4 }

func (c *prepareCountingCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*compact.CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "auto")
}

func (c *prepareCountingCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*compact.CompactionResult, error) {
	c.calls++
	c.receivedTexts = append(c.receivedTexts, joinedText(messages))
	c.receivedMsgs = append(c.receivedMsgs, cloneMessagesForPrepare(messages))
	c.receivedMedia = append(c.receivedMedia, containsMedia(messages))
	if c.fail {
		return nil, fmt.Errorf("compact failed")
	}
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: trigger})
	summary := c.summaryMessage
	if summary == "" {
		summary = "pre-call compact summary"
	}
	return &compact.CompactionResult{
		BoundaryMarker:            &boundary,
		SummaryMessages:           []types.Message{types.UserMessage(summary)},
		MessagesToKeep:            messages[len(messages)-1:],
		PreCompactTokenCount:      123,
		PostCompactTokenCount:     20000,
		TruePostCompactTokenCount: 20000,
		CompactionUsage: &types.Usage{
			InputTokens:              100,
			OutputTokens:             20,
			CacheCreationInputTokens: 30,
			CacheReadInputTokens:     70,
		},
	}, nil
}

func TestPrepareMessagesForQueryRunsAutoCompactBeforeProviderRequest(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100})
	fake := &prepareCountingCompactor{summaryMessage: "summary visible to provider"}
	ql.compactor = fake
	ql.messages = manyUserMessages(30)

	if err := ql.runLoop(context.Background(), func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("compactor calls = %d, want 1 before provider request", fake.calls)
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	if got := joinedText(prov.requests[0].Messages); !strings.Contains(got, "summary visible to provider") {
		t.Fatalf("provider request did not use post-compact messages: %q", got)
	}
	if ql.ctxWindow.ConsecutiveFailures() != 0 {
		t.Fatalf("consecutive failures = %d, want reset to 0", ql.ctxWindow.ConsecutiveFailures())
	}
}

func TestPrepareMessagesForQueryEmitsAutoCompactTelemetry(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 50000, SessionID: "session-123"})
	ql.ctxWindow.UsedInput = 40000
	ql.compactor = &prepareCountingCompactor{summaryMessage: "summary visible to provider"}
	state := newQueryState(manyUserMessages(30))
	var events []stream.Event

	if _, err := ql.prepareMessagesForQuery(context.Background(), state, 7, 0, true, func(e stream.Event) {
		events = append(events, e)
	}); err != nil {
		t.Fatal(err)
	}

	telemetry := findProgressEvent(events, "auto_compact_success")
	if telemetry == nil {
		t.Fatalf("missing auto compact telemetry in %+v", events)
	}
	metadata := telemetry.Progress.Metadata
	if metadata["pre_compact_token_count"] == nil || metadata["true_post_compact_token_count"] == nil {
		t.Fatalf("missing token counts in telemetry metadata: %+v", metadata)
	}
	if metadata["auto_compact_threshold"] == nil || metadata["post_compact_would_retrigger"] != true {
		t.Fatalf("missing threshold/retrigger metadata: %+v", metadata)
	}
	if metadata["compact_input_tokens"] != 100 || metadata["compact_output_tokens"] != 20 {
		t.Fatalf("missing compaction usage metadata: %+v", metadata)
	}
	if metadata["cache_read_input_tokens"] != 70 || metadata["cache_creation_input_tokens"] != 30 {
		t.Fatalf("missing prompt cache usage metadata: %+v", metadata)
	}
	if metadata["turn_id"] != "turn_7" || metadata["query_id"] != "session-123" {
		t.Fatalf("missing identifiers in metadata: %+v", metadata)
	}
}

func TestPrepareMessagesForQueryRunsAutoCompactBeforeNoToolAssistantResponse(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("terminal answer")}})
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100})
	fake := &prepareCountingCompactor{summaryMessage: "summary before no-tool response"}
	ql.compactor = fake
	ql.messages = manyUserMessages(30)

	if err := ql.runLoop(context.Background(), func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("compactor calls = %d, want 1 before no-tool response", fake.calls)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(prov.Calls))
	}
	got := joinedText(prov.Calls[0].Messages)
	if !strings.Contains(got, "summary before no-tool response") {
		t.Fatalf("provider request did not include compact summary: %q", got)
	}
	if !strings.Contains(joinedText(ql.Messages()), "terminal answer") {
		t.Fatalf("final messages missing no-tool assistant response: %q", joinedText(ql.Messages()))
	}
}

func TestPrepareMessagesForQuerySkipsAutoCompactAfterCircuitBreaker(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100})
	fake := &prepareCountingCompactor{}
	ql.compactor = fake
	ql.messages = manyUserMessages(30)
	for i := 0; i < compact.MaxConsecutiveAutocompactFailures; i++ {
		ql.ctxWindow.RecordCompactFailure()
	}

	if err := ql.runLoop(context.Background(), func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 0 {
		t.Fatalf("compactor calls = %d, want 0 after circuit breaker trips", fake.calls)
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
}

func TestPrepareMessagesForQuerySkipsAutoCompactForCompactSources(t *testing.T) {
	for _, source := range []QuerySource{QuerySourceCompact} {
		t.Run(string(source), func(t *testing.T) {
			prov := &aggregateBudgetProvider{}
			ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100, QuerySource: source})
			fake := &prepareCountingCompactor{}
			ql.compactor = fake
			ql.messages = manyUserMessages(30)

			if err := ql.runLoop(context.Background(), func(stream.Event) {}); err != nil {
				t.Fatal(err)
			}
			if fake.calls != 0 {
				t.Fatalf("compactor calls = %d, want 0 for source %q", fake.calls, source)
			}
			if len(prov.requests) != 1 {
				t.Fatalf("provider requests = %d, want 1", len(prov.requests))
			}
			if got := joinedText(prov.requests[0].Messages); !strings.Contains(got, "message 05") {
				t.Fatalf("guarded source lost middle history before provider request: %q", got)
			}
		})
	}
}

func TestPrepareMessagesForQueryAutoCompactFailureDoesNotForceTruncateAndTripsBreaker(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100})
	fake := &prepareCountingCompactor{fail: true}
	ql.compactor = fake
	ql.messages = manyUserMessages(30)
	originalCount := len(ql.messages)
	var warnings []stream.Event

	for i := 0; i < compact.MaxConsecutiveAutocompactFailures; i++ {
		state := newQueryState(ql.messages)
		if _, err := ql.prepareMessagesForQuery(context.Background(), state, i+1, 0, false, func(e stream.Event) {
			if e.Type == stream.EventSystemWarning {
				warnings = append(warnings, e)
			}
		}); err != nil {
			t.Fatalf("prepareMessagesForQuery attempt %d: %v", i+1, err)
		}
		ql.messages = state.Messages
	}
	state := newQueryState(ql.messages)
	if _, err := ql.prepareMessagesForQuery(context.Background(), state, compact.MaxConsecutiveAutocompactFailures+1, 0, false, func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("compactor calls = %d, want %d", fake.calls, compact.MaxConsecutiveAutocompactFailures)
	}
	if len(state.Messages) != originalCount {
		t.Fatalf("messages were force-truncated after auto-compact failure: got %d want %d", len(state.Messages), originalCount)
	}
	if len(warnings) != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("warnings = %d, want %d", len(warnings), compact.MaxConsecutiveAutocompactFailures)
	}
	if state.AutoCompactTracking.ConsecutiveFailures != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("tracking consecutive failures = %d, want %d", state.AutoCompactTracking.ConsecutiveFailures, compact.MaxConsecutiveAutocompactFailures)
	}
}

func findProgressEvent(events []stream.Event, stage string) *stream.Event {
	for i := range events {
		if events[i].Type == stream.EventProgress && events[i].Progress != nil && events[i].Progress.Stage == stage {
			return &events[i]
		}
	}
	return nil
}

func TestPrepareMessagesForQueryMicrocompactDoesNotSplitToolPairOrSnipHistory(t *testing.T) {
	ql := &QueryLoop{
		ctxWindow:               compact.NewContextWindow(100),
		contentReplacementState: compact.NewContentReplacementState(),
		microcompactCfg: compact.MicrocompactConfig{
			KeepRecent:       1,
			TimeBasedEnabled: true,
			QuerySource:      compact.MicrocompactSourceMain,
			IdleThreshold:    time.Hour,
			LastActivity:     time.Now().Add(-2 * time.Hour),
		},
	}
	state := newQueryState([]types.Message{
		types.UserMessage("head"),
		assistantUseForPrepare("old_use"),
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "old_use", Content: strings.Repeat("old", 100)}),
	})
	state.Messages = append(state.Messages, manyUserMessages(30)...)
	state.Messages = append(state.Messages,
		assistantUseForPrepare("new_use"),
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "new_use", Content: strings.Repeat("new", 100)}),
	)

	prepared, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	assertToolPairPresent(t, prepared.Messages, "old_use")
	assertToolPairPresent(t, prepared.Messages, "new_use")
	if got := findToolResultContent(prepared.Messages, "old_use"); !strings.Contains(got, "cleared") {
		t.Fatalf("old tool result was not microcompacted after snip: %q", got)
	}
	if got := findToolResultContent(prepared.Messages, "new_use"); strings.Contains(got, "cleared") {
		t.Fatalf("new tool result should be preserved by microcompact keep budget: %q", got)
	}
	if got := joinedText(prepared.Messages); !strings.Contains(got, "message 05") {
		t.Fatalf("microcompact unexpectedly snipped ordinary middle history: %q", got)
	}
}

func TestPrepareMessagesForQuerySemanticAutoCompactReceivesUnsnippedHistory(t *testing.T) {
	window := compact.NewContextWindow(50_000)
	window.Counter = &charBasedCounterForTest{}
	window.MaxOutputTokens = 20_000
	fake := &prepareCountingCompactor{summaryMessage: "semantic summary"}
	ql := &QueryLoop{
		ctxWindow:               window,
		contentReplacementState: compact.NewContentReplacementState(),
		compactor:               fake,
	}
	messages := make([]types.Message, 30)
	for index := range messages {
		messages[index] = types.UserMessage(fmt.Sprintf("message %02d %s", index, strings.Repeat("x", 3_000)))
	}
	state := newQueryState(messages)

	if _, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("semantic compactor calls = %d, want 1", fake.calls)
	}
	if got := fake.receivedTexts[0]; !strings.Contains(got, "message 05") || strings.Contains(got, "earlier messages were compressed") {
		t.Fatalf("semantic compactor received a pre-snipped history: %q", got)
	}
}

func TestPrepareMessagesForQueryUsesSystemEnvelopeInAutoCompactThreshold(t *testing.T) {
	window := compact.NewContextWindow(50_000)
	window.Counter = &charBasedCounterForTest{}
	window.MaxOutputTokens = 20_000
	fake := &prepareCountingCompactor{summaryMessage: "system-overhead summary"}
	ql := &QueryLoop{
		provider:                &aggregateBudgetProvider{},
		registry:                registry.New(),
		config:                  Config{System: strings.Repeat("s", 80_000), MaxContextTokens: 50_000, MaxOutputTokens: 20_000},
		ctxWindow:               window,
		contentReplacementState: compact.NewContentReplacementState(),
		compactor:               fake,
	}
	state := newQueryState([]types.Message{types.UserMessage("small request")})

	if _, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("system prompt overhead did not trigger compaction: calls=%d", fake.calls)
	}
}

func TestProviderRequestPublishesNoLocalContextEstimate(t *testing.T) {
	reported := types.Usage{InputTokens: 1234, OutputTokens: 7, CacheReadInputTokens: 1000}
	prov := &aggregateBudgetProvider{events: [][]types.StreamEvent{
		providerUsageTextEvents("done", reported, types.StopReasonEndTurn),
	}}
	ql := New(prov, registry.New(), Config{
		MaxTurns: 1, MaxContextTokens: 200_000, System: "system envelope",
	})
	localEstimateEvents := 0
	if err := ql.Run(context.Background(), "request", func(event stream.Event) {
		if string(event.Type) == "context_usage" {
			localEstimateEvents++
		}
	}); err != nil {
		t.Fatal(err)
	}
	if localEstimateEvents != 0 {
		t.Fatalf("request emitted %d local context estimates", localEstimateEvents)
	}
	_, current := ql.ContextUsageDetail()
	if current.Measurement != compact.ContextUsageProviderReported || current.UsedTokens != reported.TotalInputTokens() {
		t.Fatalf("provider usage did not supersede local estimate: %+v", current)
	}
}

func TestPrepareMessagesForQueryWithoutCompactorDoesNotImplicitlySnip(t *testing.T) {
	window := compact.NewContextWindow(50_000)
	window.Counter = &charBasedCounterForTest{}
	window.MaxOutputTokens = 20_000
	ql := &QueryLoop{
		ctxWindow:               window,
		contentReplacementState: compact.NewContentReplacementState(),
		lastResponseID:          "response-before-prepare",
		skillCatalogEpoch:       7,
	}
	messages := make([]types.Message, 30)
	for index := range messages {
		messages[index] = types.UserMessage(fmt.Sprintf("message %02d %s", index, strings.Repeat("x", 3_000)))
	}
	original := cloneMessagesForPrepare(messages)
	state := newQueryState(messages)

	prepared, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prepared.Messages, original) || !reflect.DeepEqual(state.Messages, original) {
		t.Fatal("nil compactor changed model-visible or durable history")
	}
	if ql.skillCatalogEpoch != 7 || ql.lastResponseID != "response-before-prepare" {
		t.Fatalf("nil compactor crossed context fence: epoch=%d response=%q", ql.skillCatalogEpoch, ql.lastResponseID)
	}
}

func TestPrepareMessagesForQueryNormalNonIdleDoesNotMicrocompact(t *testing.T) {
	cfg := compact.DefaultMicrocompactConfig()
	cfg.QuerySource = compact.MicrocompactSourceMain
	cfg.LastActivity = time.Now()
	cfg.KeepRecent = 1
	ql := &QueryLoop{
		contentReplacementState: compact.NewContentReplacementState(),
		microcompactCfg:         cfg,
	}
	state := newQueryState(compactablePrepareMessages(3))
	original := cloneMessagesForPrepare(state.Messages)

	prepared, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Messages, original) {
		t.Fatal("prepareMessagesForQuery mutated input state messages")
	}
	if got := countClearedToolResults(prepared.Messages); got != 0 {
		t.Fatalf("normal non-idle request cleared %d tool results, want 0", got)
	}
}

func TestPrepareMessagesForQuerySubagentDoesNotTimeBasedMicrocompact(t *testing.T) {
	cfg := compact.DefaultMicrocompactConfig()
	cfg.QuerySource = compact.MicrocompactSourceNonMain
	cfg.LastActivity = time.Now().Add(-2 * time.Hour)
	cfg.KeepRecent = 1
	ql := &QueryLoop{
		contentReplacementState: compact.NewContentReplacementState(),
		microcompactCfg:         cfg,
	}
	state := newQueryState(compactablePrepareMessages(3))

	prepared, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := countClearedToolResults(prepared.Messages); got != 0 {
		t.Fatalf("subagent/non-main request cleared %d tool results, want 0", got)
	}
}

func TestPrepareMessagesForQueryIdleMainMicrocompactNotifiesCacheBreakDetector(t *testing.T) {
	cfg := compact.DefaultMicrocompactConfig()
	cfg.QuerySource = compact.MicrocompactSourceMain
	cfg.LastActivity = time.Now().Add(-2 * time.Hour)
	cfg.KeepRecent = 1
	detector := &CacheBreakDetector{}
	_ = detector.Check(&types.Usage{CacheReadInputTokens: 10_000})
	ql := &QueryLoop{
		contentReplacementState: compact.NewContentReplacementState(),
		microcompactCfg:         cfg,
		cacheBreakDetector:      detector,
	}
	state := newQueryState(compactablePrepareMessages(3))

	prepared, err := ql.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := countClearedToolResults(prepared.Messages); got != 2 {
		t.Fatalf("idle main request cleared %d tool results, want 2", got)
	}
	if detector.hasBaseline {
		t.Fatal("time-based microcompact should reset cache-break baseline")
	}
}

func manyUserMessages(n int) []types.Message {
	msgs := make([]types.Message, n)
	for i := range msgs {
		msgs[i] = types.UserMessage(fmt.Sprintf("message %02d %s", i, strings.Repeat("x", 20)))
	}
	return msgs
}

func assistantUseForPrepare(id string) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "Read", Input: map[string]any{"file_path": "/tmp/" + id}},
		},
	}
}

func compactablePrepareMessages(n int) []types.Message {
	msgs := []types.Message{types.UserMessage("head")}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("prep_%d", i)
		msgs = append(msgs,
			assistantUseForPrepare(id),
			types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: id, Content: "content " + id}),
		)
	}
	return msgs
}

func cloneMessagesForPrepare(messages []types.Message) []types.Message {
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	for i := range cloned {
		cloned[i].Content = append([]types.ContentBlock(nil), messages[i].Content...)
	}
	return cloned
}

func countClearedToolResults(messages []types.Message) int {
	count := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok && strings.Contains(result.Content, "cleared") {
				count++
			}
		}
	}
	return count
}

func assertToolPairPresent(t *testing.T, messages []types.Message, id string) {
	t.Helper()
	var sawUse, sawResult bool
	for _, msg := range messages {
		for _, use := range msg.GetToolUses() {
			if use.ID == id {
				sawUse = true
			}
		}
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == id {
				sawResult = true
			}
		}
	}
	if !sawUse || !sawResult {
		t.Fatalf("tool pair %s present use=%v result=%v in %#v", id, sawUse, sawResult, messages)
	}
}

func joinedText(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.GetText())
		b.WriteByte('\n')
	}
	return b.String()
}
