package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestReactiveCompactRetriesContextWindowFull(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}},
		{Events: parityTextEvents("recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3})
	fake := &prepareCountingCompactor{summaryMessage: "reactive summary"}
	q.compactor = fake

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if fake.calls != 1 {
		t.Fatalf("reactive compactor calls = %d, want 1", fake.calls)
	}
	if got := joinedMessageText(prov.Calls[1].Messages); !strings.Contains(got, "reactive summary") {
		t.Fatalf("retry did not use compacted messages: %q", got)
	}
}

type reactiveEvidenceCompactor struct {
	err error
}

func (c reactiveEvidenceCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*compact.CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "reactive")
}

func (c reactiveEvidenceCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*compact.CompactionResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger:                   trigger,
		PreCompactTokenCount:      1200,
		PreviousTailIdentifier:    "assistant:tail",
		PreCompactDiscoveredTools: []string{"Read", "Bash"},
		PreservedSegment: &compact.PreservedSegmentMetadata{
			StartIndex: 1,
			Count:      1,
			Anchor:     "assistant:tail",
			Direction:  "tail",
		},
	})
	return &compact.CompactionResult{
		BoundaryMarker:            &boundary,
		SummaryMessages:           []types.Message{types.UserMessage("complete reactive summary evidence")},
		MessagesToKeep:            messages[len(messages)-1:],
		PostCompactTokenCount:     300,
		TruePostCompactTokenCount: 280,
		UserDisplayMessage:        "reactive display evidence",
	}, nil
}

func TestReactiveCompactEmitsCompleteBoundaryBeforeTerminalProgress(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}},
		{Events: parityTextEvents("recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3})
	q.compactor = reactiveEvidenceCompactor{}

	var lifecycle []string
	var boundary *stream.CompactBoundaryEvent
	if err := q.Run(context.Background(), "hello", func(event stream.Event) {
		switch {
		case event.Type == stream.EventProgress && event.Progress != nil && event.Progress.Stage == "compact_start":
			lifecycle = append(lifecycle, event.Progress.Stage)
		case event.Type == stream.EventCompactBoundary:
			lifecycle = append(lifecycle, string(event.Type))
			boundary = event.Compact
		case event.Type == stream.EventProgress && event.Progress != nil && event.Progress.Stage == "compact_end":
			lifecycle = append(lifecycle, event.Progress.Stage)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantLifecycle := []string{"compact_start", string(stream.EventCompactBoundary), "compact_end"}
	if strings.Join(lifecycle, ",") != strings.Join(wantLifecycle, ",") {
		t.Fatalf("reactive compaction lifecycle = %v, want %v", lifecycle, wantLifecycle)
	}
	if boundary == nil || boundary.Trigger != "reactive" || boundary.PreCompactTokenCount != 1200 || boundary.PostCompactTokenCount != 300 || boundary.TruePostCompactTokenCount != 280 {
		t.Fatalf("reactive compaction boundary = %+v", boundary)
	}
	if boundary.PreviousTailIdentifier != "assistant:tail" || len(boundary.PreCompactDiscoveredTools) != 2 || boundary.PreservedSegment == nil || boundary.PreservedSegment.Anchor != "assistant:tail" {
		t.Fatalf("reactive compaction boundary lost retained-range evidence: %+v", boundary)
	}
	if boundary.Summary != "complete reactive summary evidence" || boundary.UserDisplayMessage != "reactive display evidence" {
		t.Fatalf("reactive compaction boundary lost summary/display evidence: %+v", boundary)
	}
}

func TestReactiveCompactFailureAndCancellationDoNotEmitBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, terminalStage string
		err                 error
	}{
		{name: "failure", terminalStage: "compact_failed", err: errors.New("reactive summary failed")},
		{name: "cancellation", terminalStage: "compact_cancelled", err: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov := newParityFakeProvider([]parityProviderTurn{{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}}})
			q := New(prov, registry.New(), Config{MaxTurns: 3})
			q.compactor = reactiveEvidenceCompactor{err: tc.err}
			var stages []string
			boundaryCount := 0
			err := q.Run(context.Background(), "hello", func(event stream.Event) {
				if event.Type == stream.EventCompactBoundary {
					boundaryCount++
				}
				if event.Type == stream.EventProgress && event.Progress != nil && strings.HasPrefix(event.Progress.Stage, "compact_") {
					stages = append(stages, event.Progress.Stage)
				}
			})
			if err == nil {
				t.Fatal("Run succeeded, want original provider error")
			}
			if boundaryCount != 0 {
				t.Fatalf("reactive %s emitted %d false boundary event(s)", tc.name, boundaryCount)
			}
			wantStages := []string{"compact_start", tc.terminalStage}
			if strings.Join(stages, ",") != strings.Join(wantStages, ",") {
				t.Fatalf("reactive %s stages = %v, want %v", tc.name, stages, wantStages)
			}
		})
	}
}

func TestReactiveCompactCleanupFailureRestoresExactPreimageAndFailureLifecycle(t *testing.T) {
	cleanupErr := errors.New("private reactive cleanup sentinel")
	q := New(newParityFakeProvider(nil), registry.New(), Config{
		MaxTurns: 3, MaxContextTokens: 100,
		PostCompactCleanup: func(context.Context) error {
			return cleanupErr
		},
	})
	q.compactor = reactiveEvidenceCompactor{}
	original := []types.Message{types.UserMessage("reactive original"), types.AssistantMessage("reactive tail")}
	q.SetMessages(executioncontract.CloneMessages(original))
	q.lastResponseID = "response-before-reactive"
	q.ctxWindow.RecordCompactFailure()
	q.ctxWindow.RecordCompactFailure()
	state := newQueryState(executioncontract.CloneMessages(original))
	state.MaxOutputTokensOverride = 777
	var events []stream.Event

	retry, err := q.recoverFromTerminalProviderFailure(
		context.Background(), state, executioncontract.CloneMessages(original),
		&types.APIError{Type: "context_window_full", Message: "context_window_full"}, 2,
		func(event stream.Event) { events = append(events, event) },
	)
	if retry || !errors.Is(err, cleanupErr) {
		t.Fatalf("reactive cleanup retry/error = %t/%v, want false/sentinel", retry, err)
	}
	if got := state.Messages; !reflect.DeepEqual(got, original) || state.HasAttemptedReactiveCompact || state.MaxOutputTokensOverride != 777 || state.Transition != QueryTransitionNextTurn {
		t.Fatalf("reactive state after rollback = %#v", state)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, original) || q.lastResponseID != "response-before-reactive" {
		t.Fatalf("reactive loop after rollback = %#v response=%q", got, q.lastResponseID)
	}
	if got := q.ctxWindow.ConsecutiveFailures(); got != 2 {
		t.Fatalf("reactive rollback changed auto breaker = %d, want 2", got)
	}
	requireTransactionalCompactionFailure(t, events, cleanupErr)
}

func TestReactiveCompactInstallFailureRestoresExactPreimage(t *testing.T) {
	manager := newLoopTestSkillManager(t)
	q := New(newParityFakeProvider(nil), registry.New(), Config{
		MaxTurns: 3, MaxContextTokens: 100, SessionID: "reactive-install-rollback", SkillManager: manager,
	})
	q.compactor = reactiveEvidenceCompactor{}
	original := []types.Message{types.UserMessage("reactive install original"), types.AssistantMessage("reactive install tail")}
	q.SetMessages(executioncontract.CloneMessages(original))
	q.lastResponseID = "response-before-reactive-install"
	q.skillRunGenerationMu.Lock()
	q.skillRunGeneration = manager.ProjectGeneration() + 1
	q.skillRunGenerationMu.Unlock()
	state := newQueryState(executioncontract.CloneMessages(original))
	state.MaxOutputTokensOverride = 999
	var events []stream.Event

	retry, err := q.recoverFromTerminalProviderFailure(
		context.Background(), state, executioncontract.CloneMessages(original),
		&types.APIError{Type: "context_window_full", Message: "context_window_full"}, 2,
		func(event stream.Event) { events = append(events, event) },
	)
	if retry || err == nil {
		t.Fatalf("reactive install retry/error = %t/%v, want false/error", retry, err)
	}
	if got := state.Messages; !reflect.DeepEqual(got, original) || state.MaxOutputTokensOverride != 999 || state.HasAttemptedReactiveCompact {
		t.Fatalf("reactive install state after rollback = %#v", state)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, original) || q.lastResponseID != "response-before-reactive-install" {
		t.Fatalf("reactive install loop after rollback = %#v response=%q", got, q.lastResponseID)
	}
	requireTransactionalCompactionFailure(t, events, nil)
}

func TestReactiveCompactCleanupFailureLeavesRunPersistenceViewUnchanged(t *testing.T) {
	cleanupErr := errors.New("private run cleanup sentinel")
	prov := newParityFakeProvider([]parityProviderTurn{{
		Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"},
	}})
	q := New(prov, registry.New(), Config{
		MaxTurns: 3,
		PostCompactCleanup: func(context.Context) error {
			return cleanupErr
		},
	})
	q.compactor = reactiveEvidenceCompactor{}
	initial := []types.Message{types.UserMessage("persisted before failed recovery")}
	q.SetMessages(executioncontract.CloneMessages(initial))
	var events []stream.Event

	err := q.Run(context.Background(), "current user must survive", func(event stream.Event) {
		events = append(events, event)
	})
	if err == nil || strings.Contains(err.Error(), cleanupErr.Error()) {
		t.Fatalf("Run error = %v, want semantic failure without private sentinel", err)
	}
	want := append(executioncontract.CloneMessages(initial), types.UserMessage("current user must survive"))
	if got := q.Messages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Run persistence view after failed recovery = %#v, want %#v", got, want)
	}
	if got := joinedMessageText(q.Messages()); strings.Contains(got, "complete reactive summary evidence") {
		t.Fatalf("Run persistence view retained failed replacement: %q", got)
	}
	requireTransactionalCompactionFailure(t, events, cleanupErr)
}

func TestReactiveCompactRetriesPromptTooLongFromStreamEventWithoutAssistantError(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: []types.StreamEvent{{
			Type:  types.EventError,
			Error: &types.APIError{Status: 400, Type: "prompt_too_long", Message: "prompt is too long"},
		}}},
		{Events: parityTextEvents("recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}

	var errorEvents []stream.Event
	if err := q.Run(context.Background(), "hello", func(evt stream.Event) {
		if evt.Type == stream.EventError {
			errorEvents = append(errorEvents, evt)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if len(errorEvents) != 0 {
		t.Fatalf("stream-level PTL should be withheld during successful recovery, got %#v", errorEvents)
	}
	if got := joinedMessageText(q.Messages()); strings.Contains(got, "prompt is too long") {
		t.Fatalf("invalid assistant error was appended to transcript: %q", got)
	}
}

func TestPreCallBlockingLimitWhenAutoCompactDisabled(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_AUTO_COMPACT", "1")
	t.Setenv("LUBAN_CODE_BLOCKING_LIMIT_OVERRIDE", "1")
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("unexpected")}})
	q := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100_000})

	err := q.Run(context.Background(), "hello", func(stream.Event) {})
	if err == nil {
		t.Fatal("Run succeeded, want blocking-limit error")
	}
	if !strings.Contains(err.Error(), "Run /compact") {
		t.Fatalf("error = %v, want manual compact guidance", err)
	}
	if len(prov.Calls) != 0 {
		t.Fatalf("provider calls = %d, want 0 synthetic pre-call block", len(prov.Calls))
	}
}

func TestPreCallBlockingLimitDoesNotStealReactiveOverflow(t *testing.T) {
	t.Setenv("LUBAN_CODE_BLOCKING_LIMIT_OVERRIDE", "1")
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}},
		{Events: parityTextEvents("recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxContextTokens: 100_000})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("provider calls = %d, want real overflow plus reactive retry", len(prov.Calls))
	}
}

func TestReactiveCompactSecondContextWindowFullSurfacesError(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full first"}},
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full second"}},
		{Events: parityTextEvents("unexpected")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 5})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}

	err := q.Run(context.Background(), "hello", func(stream.Event) {})
	if err == nil {
		t.Fatal("Run succeeded, want second context_window_full error")
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2 (no infinite retry)", len(prov.Calls))
	}
	if !strings.Contains(err.Error(), "context_window_full second") {
		t.Fatalf("error = %v, want second provider error", err)
	}
}

func TestReactiveCompactSecondStreamPromptTooLongSurfacesErrorWithoutSpiral(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: []types.StreamEvent{{
			Type:  types.EventError,
			Error: &types.APIError{Status: 400, Type: "prompt_too_long", Message: "prompt is too long first"},
		}}},
		{Events: []types.StreamEvent{{
			Type:  types.EventError,
			Error: &types.APIError{Status: 400, Type: "prompt_too_long", Message: "prompt is too long second"},
		}}},
		{Events: parityTextEvents("unexpected")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 5})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}

	err := q.Run(context.Background(), "hello", func(stream.Event) {})
	if err == nil {
		t.Fatal("Run succeeded, want second prompt_too_long error")
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2 (single reactive retry only)", len(prov.Calls))
	}
	if !strings.Contains(err.Error(), "prompt is too long second") {
		t.Fatalf("error = %v, want second provider error", err)
	}
	if got := joinedMessageText(q.Messages()); strings.Contains(got, "prompt is too long") {
		t.Fatalf("invalid assistant error was appended to transcript: %q", got)
	}
}

func TestMarkerLikeUserTextUsesReactiveCompact(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}},
		{Events: parityTextEvents("reactive recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3})
	q.messages = []types.Message{
		types.UserMessage("old"),
		types.UserMessage("[context-collapse-staged] not-json"),
		types.UserMessage("tail"),
	}
	fake := &prepareCountingCompactor{summaryMessage: "reactive summary"}
	q.compactor = fake

	if err := q.runLoop(context.Background(), func(stream.Event) {}); err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if fake.calls != 1 {
		t.Fatalf("reactive compactor calls = %d, want 1 when invalid marker cannot drain", fake.calls)
	}
	if got := joinedMessageText(prov.Calls[1].Messages); !strings.Contains(got, "reactive summary") {
		t.Fatalf("retry did not use reactive compact summary: %q", got)
	}
}

func TestMediaSizeRecoveryStripsSummaryInput(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "invalid_request_error", Message: "media size exceeds provider limit"}},
		{Events: parityTextEvents("media recovered")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3})
	fake := &prepareCountingCompactor{summaryMessage: "media compact summary"}
	q.compactor = fake
	content := []types.ContentBlock{
		types.TextBlock{Type: types.ContentTypeText, Text: "inspect"},
		types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("x", 100)}},
	}

	if err := q.RunWithContent(context.Background(), content, func(stream.Event) {}); err != nil {
		t.Fatalf("RunWithContent: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if fake.calls != 1 {
		t.Fatalf("reactive compactor calls = %d, want 1", fake.calls)
	}
	if len(fake.receivedMedia) != 1 || fake.receivedMedia[0] {
		t.Fatalf("summary input still contained media: %#v", fake.receivedMedia)
	}
	if !containsMedia(prov.Calls[1].Messages) {
		t.Fatalf("retry should preserve tail media so second media overflow can surface: %#v", prov.Calls[1].Messages)
	}
}

func TestReactiveCompactMediaTailSecondOverflowSurfacesError(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "invalid_request_error", Message: "media size exceeds provider limit first"}},
		{Error: &types.APIError{Type: "invalid_request_error", Message: "media size exceeds provider limit second"}},
		{Events: parityTextEvents("unexpected")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 5})
	q.compactor = &prepareCountingCompactor{summaryMessage: "media compact summary"}
	content := []types.ContentBlock{
		types.TextBlock{Type: types.ContentTypeText, Text: "inspect"},
		types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("x", 100)}},
	}

	err := q.RunWithContent(context.Background(), content, func(stream.Event) {})
	if err == nil {
		t.Fatal("RunWithContent succeeded, want second media-size error")
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2 (single media recovery retry only)", len(prov.Calls))
	}
	if !containsMedia(prov.Calls[1].Messages) {
		t.Fatalf("retry should preserve tail media and surface second overflow: %#v", prov.Calls[1].Messages)
	}
	if !strings.Contains(err.Error(), "media size exceeds provider limit second") {
		t.Fatalf("error = %v, want second media-size provider error", err)
	}
}

func TestRecoveryFailureDoesNotRunStopHook(t *testing.T) {
	dir := t.TempDir()
	touched := filepath.Join(dir, "ran")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Command: testHookTouchCommand(touched),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{{
		Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"},
	}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, HookRunner: runner})
	q.compactor = &prepareCountingCompactor{fail: true}

	err := q.Run(context.Background(), "hello", func(stream.Event) {})
	if err == nil {
		t.Fatal("Run succeeded, want recovery failure")
	}
	if !strings.Contains(err.Error(), "context_window_full") {
		t.Fatalf("error = %v, want original provider error", err)
	}
	if strings.Contains(err.Error(), "reactive compact failed") || strings.Contains(err.Error(), "compact failed") {
		t.Fatalf("error = %v, should not replace withheld provider error with compact failure", err)
	}
	if _, err := os.Stat(touched); !os.IsNotExist(err) {
		t.Fatalf("Stop hook ran after recovery failure; stat err=%v", err)
	}
}

func TestRecoveryAfterPreviousResponseFallbackPreservesPromptCache(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "previous_response_not_found", Message: "Previous response with id resp_old not found"}},
		{Error: &types.APIError{Type: "context_window_full", Message: "context_window_full"}},
		{Events: append(parityTextEvents("ok after compact"), types.StreamEvent{Type: types.EventMessageStop, ResponseID: "resp_new"})},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 5, MaxTokens: 1024, SessionID: "session-123", Model: "gpt-4o"})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}
	q.lastResponseID = "resp_old"
	q.lastEnvelopeFingerprint = envelopeFingerprint(q.providerParams(
		newQueryState(nil),
		newQueryConfigSnapshot(q.config, q.thinkingConfig),
		nil,
	))

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 3 {
		t.Fatalf("CreateStream calls = %d, want previous_response fallback + recovery retry", len(prov.Calls))
	}
	if prov.Calls[0].PreviousResponseID != "resp_old" {
		t.Fatalf("first PreviousResponseID = %q, want resp_old", prov.Calls[0].PreviousResponseID)
	}
	if prov.Calls[1].PreviousResponseID != "" || prov.Calls[2].PreviousResponseID != "" {
		t.Fatalf("fallback/recovery PreviousResponseID = %q/%q, want empty", prov.Calls[1].PreviousResponseID, prov.Calls[2].PreviousResponseID)
	}
	for i, call := range prov.Calls {
		if call.PromptCacheKey != "session-123" || !call.UsePromptCache {
			t.Fatalf("call %d prompt cache = key %q enabled %v, want session-123 true", i, call.PromptCacheKey, call.UsePromptCache)
		}
	}
}

func containsMedia(messages []types.Message) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch typed := block.(type) {
			case types.ImageBlock, types.DocumentBlock:
				return true
			case types.ToolResultBlock:
				for _, nested := range typed.ContentBlocks {
					if nested.GetType() == types.ContentTypeImage || nested.GetType() == types.ContentTypeDocument {
						return true
					}
				}
			}
		}
	}
	return false
}
