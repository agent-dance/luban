package loop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type lifecycleCompactor struct {
	err error
}

type resultContractCompactor struct {
	build func(messages []types.Message, trigger string) *compact.CompactionResult
}

func (c resultContractCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*compact.CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "manual")
}

func (c resultContractCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*compact.CompactionResult, error) {
	if c.build == nil {
		return nil, nil
	}
	return c.build(messages, trigger), nil
}

func (c lifecycleCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*compact.CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "manual")
}

func (c lifecycleCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*compact.CompactionResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger:              trigger,
		PreCompactTokenCount: len(messages),
	})
	return &compact.CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{types.UserMessage("summary")},
		MessagesToKeep:       tailMessage(messages),
		PreCompactTokenCount: len(messages),
	}, nil
}

func tailMessage(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return nil
	}
	return []types.Message{messages[len(messages)-1]}
}

func TestForceCompactRunsSharedCleanupAndClearsStatus(t *testing.T) {
	cleanupCalls := 0
	q := New(nil, nil, Config{
		MaxContextTokens: 100,
		PostCompactCleanup: func(context.Context) error {
			cleanupCalls++
			return nil
		},
	})
	q.compactor = lifecycleCompactor{}
	q.SetMessages([]types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("old"),
		types.UserMessage("tail"),
	})

	if err := q.ForceCompact(context.Background()); err != nil {
		t.Fatalf("ForceCompact: %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	if q.CompactStatus() != "" {
		t.Fatalf("compact status = %q, want cleared", q.CompactStatus())
	}
}

func TestForceCompactCleanupFailureRestoresExactPreimageAndFailureLifecycle(t *testing.T) {
	cleanupErr := errors.New("private cleanup sentinel")
	q := New(nil, nil, Config{
		MaxContextTokens: 100,
		PostCompactCleanup: func(context.Context) error {
			return cleanupErr
		},
	})
	q.compactor = lifecycleCompactor{}
	original := []types.Message{
		types.UserMessage("original question"),
		types.AssistantMessage("original answer"),
		types.UserMessage("original tail"),
	}
	q.SetMessages(original)
	q.loadedToolNames["DeferredTool"] = struct{}{}
	q.seenToolUseIDs["tool-before-cleanup"] = struct{}{}
	q.lastResponseID = "response-before-cleanup"
	q.disableResponseChain = true
	q.lastEnvelopeFingerprint = "last-envelope-before-cleanup"
	q.currentEnvelopeFingerprint = "current-envelope-before-cleanup"
	replacementState := &compact.ContentReplacementState{
		SeenIDs:      map[string]struct{}{"tool-before-cleanup": {}},
		Replacements: map[string]string{"tool-before-cleanup": "stored replacement"},
	}
	q.contentReplacementState = replacementState
	cachedState := &compact.CachedMicrocompactState{
		RegisteredTools: map[string]struct{}{"Read": {}},
		DeletedRefs:     map[string]struct{}{"tool-deleted": {}},
		ToolOrder:       []string{"tool-before-cleanup"},
		ToolGroups:      [][]string{{"tool-before-cleanup"}},
		PinnedEdits: []compact.PinnedCacheEdits{{
			UserMessageIndex: 1,
			Block:            compact.CacheEditsBlock{Edits: []compact.CacheEdit{{Type: "clear_tool_uses_20250919", CacheReference: "tool-before-cleanup"}}},
		}},
	}
	q.cachedMicrocompactState = cachedState
	q.microcompactCfg.LastActivity = time.Unix(1_700_000_000, 123).UTC()
	budget := &compact.ToolResultBudget{MaxCharsPerResult: 4321}
	q.toolBudget = budget
	q.cacheBreakDetector = &CacheBreakDetector{
		prevCacheRead: 8000, prevCacheCreate: 1200, prevTime: time.Unix(1_699_999_000, 0).UTC(), callCount: 7, hasBaseline: true,
	}
	q.ctxWindow.RecordCompactFailure()
	q.ctxWindow.RecordCompactFailure()

	beforeSkillState := q.SkillCatalogState()
	beforeCachedState := cloneManualCompactCachedState(cachedState)
	beforeMicrocompact := q.microcompactCfg
	beforeCacheBreak := manualCompactCacheBreakPreimage{
		prevCacheRead:   q.cacheBreakDetector.prevCacheRead,
		prevCacheCreate: q.cacheBreakDetector.prevCacheCreate,
		prevTime:        q.cacheBreakDetector.prevTime,
		callCount:       q.cacheBreakDetector.callCount,
		hasBaseline:     q.cacheBreakDetector.hasBaseline,
	}
	var events []Event
	err := q.ForceCompactWithInstructionsAndEvents(context.Background(), "", func(event Event) {
		events = append(events, event)
	})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("ForceCompact error = %v, want cleanup sentinel", err)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, original) {
		t.Fatalf("messages after cleanup rollback = %#v, want exact pre-image %#v", got, original)
	}
	if got := q.LoadedToolNames(); !reflect.DeepEqual(got, []string{"DeferredTool"}) {
		t.Fatalf("loaded tools after cleanup rollback = %v", got)
	}
	if got := q.SeenToolUseIDs(); !reflect.DeepEqual(got, []string{"tool-before-cleanup"}) {
		t.Fatalf("tool-use ledger after cleanup rollback = %v", got)
	}
	if q.lastResponseID != "response-before-cleanup" || !q.disableResponseChain ||
		q.lastEnvelopeFingerprint != "last-envelope-before-cleanup" || q.currentEnvelopeFingerprint != "current-envelope-before-cleanup" {
		t.Fatalf("response-chain state was not restored: id=%q disabled=%t last=%q current=%q",
			q.lastResponseID, q.disableResponseChain, q.lastEnvelopeFingerprint, q.currentEnvelopeFingerprint)
	}
	if q.contentReplacementState != replacementState || !reflect.DeepEqual(q.contentReplacementState, replacementState) {
		t.Fatalf("content replacement state was not restored: pointer=%p want=%p state=%#v", q.contentReplacementState, replacementState, q.contentReplacementState)
	}
	if q.cachedMicrocompactState != cachedState || !reflect.DeepEqual(q.cachedMicrocompactState, beforeCachedState) {
		t.Fatalf("cached microcompact state was not restored: pointer=%p want=%p state=%#v", q.cachedMicrocompactState, cachedState, q.cachedMicrocompactState)
	}
	if q.microcompactCfg != beforeMicrocompact || q.toolBudget != budget {
		t.Fatalf("cleanup runtime state was not restored: microcompact=%+v budget=%p", q.microcompactCfg, q.toolBudget)
	}
	if got := q.SkillCatalogState(); !reflect.DeepEqual(got, beforeSkillState) {
		t.Fatalf("skill state after cleanup rollback = %#v, want %#v", got, beforeSkillState)
	}
	q.cacheBreakDetector.mu.Lock()
	afterCacheBreak := manualCompactCacheBreakPreimage{
		prevCacheRead:   q.cacheBreakDetector.prevCacheRead,
		prevCacheCreate: q.cacheBreakDetector.prevCacheCreate,
		prevTime:        q.cacheBreakDetector.prevTime,
		callCount:       q.cacheBreakDetector.callCount,
		hasBaseline:     q.cacheBreakDetector.hasBaseline,
	}
	q.cacheBreakDetector.mu.Unlock()
	if afterCacheBreak.prevCacheRead != beforeCacheBreak.prevCacheRead ||
		afterCacheBreak.prevCacheCreate != beforeCacheBreak.prevCacheCreate ||
		!afterCacheBreak.prevTime.Equal(beforeCacheBreak.prevTime) ||
		afterCacheBreak.callCount != beforeCacheBreak.callCount || afterCacheBreak.hasBaseline != beforeCacheBreak.hasBaseline {
		t.Fatalf("cache-break state after cleanup rollback = %+v, want %+v", afterCacheBreak, beforeCacheBreak)
	}
	if got := q.ctxWindow.ConsecutiveFailures(); got != 2 {
		t.Fatalf("context-window failures after cleanup rollback = %d, want 2", got)
	}

	boundaries, successfulEnds, failedEnds := 0, 0, 0
	for _, event := range events {
		switch {
		case event.Type == EventCompactBoundary:
			boundaries++
		case event.Type == EventProgress && event.Progress != nil && event.Progress.Stage == "compact_end":
			successfulEnds++
		case event.Type == EventProgress && event.Progress != nil && event.Progress.Stage == "compact_failed":
			failedEnds++
			if diagnostic, _ := event.Progress.Metadata["error"].(string); strings.Contains(diagnostic, cleanupErr.Error()) {
				t.Fatalf("failure lifecycle leaked private cleanup cause: %+v", event)
			}
		}
	}
	if boundaries != 0 || successfulEnds != 0 || failedEnds != 1 {
		t.Fatalf("cleanup failure lifecycle boundaries/success/failure = %d/%d/%d, want 0/0/1; events=%+v", boundaries, successfulEnds, failedEnds, events)
	}
}

func TestAutoCompactRunsSharedCleanupAndProgress(t *testing.T) {
	cleanupCalls := 0
	q := New(nil, nil, Config{
		MaxContextTokens: 100,
		PostCompactCleanup: func(context.Context) error {
			cleanupCalls++
			return nil
		},
	})
	q.compactor = lifecycleCompactor{}
	state := newQueryState([]types.Message{
		types.UserMessage(strings.Repeat("old ", 20)),
		types.AssistantMessage("old"),
		types.UserMessage("tail"),
	})
	var stages []string

	if _, err := q.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(event Event) {
		if event.Type == EventProgress && event.Progress != nil {
			stages = append(stages, event.Progress.Stage)
		}
	}); err != nil {
		t.Fatalf("prepareMessagesForQuery: %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	if q.CompactStatus() != "" {
		t.Fatalf("compact status = %q, want cleared", q.CompactStatus())
	}
	if !hasString(stages, "compact_start") || !hasString(stages, "compact_end") {
		t.Fatalf("progress stages = %v, want compact_start and compact_end", stages)
	}
}

func TestAutoCompactCleanupFailureRollsBackBeforeQueryPersistence(t *testing.T) {
	cleanupErr := errors.New("private auto cleanup sentinel")
	q := New(nil, nil, Config{
		MaxContextTokens: 100,
		PostCompactCleanup: func(context.Context) error {
			return cleanupErr
		},
	})
	q.compactor = lifecycleCompactor{}
	original := manyUserMessages(30)
	q.SetMessages(cloneMessages(original))
	q.lastResponseID = "response-before-auto"
	q.ctxWindow.RecordCompactFailure()
	q.ctxWindow.RecordCompactFailure()
	state := newQueryState(cloneMessages(original))
	var events []Event

	_, err := q.prepareMessagesForQuery(context.Background(), state, 4, 0, false, func(event Event) {
		events = append(events, event)
	})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("auto cleanup error = %v, want sentinel", err)
	}
	if got := state.Messages; !reflect.DeepEqual(got, original) {
		t.Fatalf("query state after auto rollback = %#v, want exact preimage %#v", got, original)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, original) || q.lastResponseID != "response-before-auto" {
		t.Fatalf("loop state after auto rollback = %#v response=%q", got, q.lastResponseID)
	}
	if got := q.ctxWindow.ConsecutiveFailures(); got != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("auto commit failure count = %d, want %d", got, compact.MaxConsecutiveAutocompactFailures)
	}
	if state.AutoCompactTracking.Compacted || state.AutoCompactTracking.ConsecutiveFailures != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("auto tracking after rollback = %#v", state.AutoCompactTracking)
	}
	requireTransactionalCompactionFailure(t, events, cleanupErr)
}

func TestAutoCompactInstallFailureRestoresExactPreimage(t *testing.T) {
	manager := skills.NewManager()
	q := New(nil, nil, Config{MaxContextTokens: 100, SessionID: "auto-install-rollback", SkillManager: manager})
	q.compactor = lifecycleCompactor{}
	original := manyUserMessages(30)
	q.SetMessages(cloneMessages(original))
	q.lastResponseID = "response-before-auto-install"
	q.ctxWindow.RecordCompactFailure()
	q.ctxWindow.RecordCompactFailure()
	q.skillRunGenerationMu.Lock()
	q.skillRunGeneration = manager.ProjectGeneration() + 1
	q.skillRunGenerationMu.Unlock()
	state := newQueryState(cloneMessages(original))
	var events []Event

	_, err := q.prepareMessagesForQuery(context.Background(), state, 5, 0, false, func(event Event) {
		events = append(events, event)
	})
	if err == nil {
		t.Fatal("auto install unexpectedly succeeded with stale catalog generation")
	}
	if got := state.Messages; !reflect.DeepEqual(got, original) {
		t.Fatalf("query state after auto install rollback = %#v, want %#v", got, original)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, original) || q.lastResponseID != "response-before-auto-install" {
		t.Fatalf("loop state after auto install rollback = %#v response=%q", got, q.lastResponseID)
	}
	if got := q.ctxWindow.ConsecutiveFailures(); got != compact.MaxConsecutiveAutocompactFailures {
		t.Fatalf("auto install failure count = %d, want %d", got, compact.MaxConsecutiveAutocompactFailures)
	}
	requireTransactionalCompactionFailure(t, events, nil)
}

func TestAutoCompactBoundarylessNoopDoesNotPublishSuccessOrClearBreaker(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	q.compactor = resultContractCompactor{build: func(messages []types.Message, _ string) *compact.CompactionResult {
		return &compact.CompactionResult{MessagesToKeep: cloneMessages(messages)}
	}}
	original := manyUserMessages(30)
	q.SetMessages(cloneMessages(original))
	q.ctxWindow.RecordCompactFailure()
	q.ctxWindow.RecordCompactFailure()
	state := newQueryState(cloneMessages(original))
	var events []Event

	prepared, err := q.prepareMessagesForQuery(context.Background(), state, 6, 0, false, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("auto no-op: %v", err)
	}
	if state.AutoCompactTracking.Compacted || q.ctxWindow.ConsecutiveFailures() != 2 {
		t.Fatalf("auto no-op tracking=%#v breaker=%d, want unchanged breaker 2", state.AutoCompactTracking, q.ctxWindow.ConsecutiveFailures())
	}
	if !reflect.DeepEqual(state.Messages, original) || !reflect.DeepEqual(prepared.Messages, original) {
		t.Fatalf("auto no-op changed history: state=%#v prepared=%#v", state.Messages, prepared.Messages)
	}
	for _, event := range events {
		if event.Type == EventCompactBoundary || event.Type == EventProgress && event.Progress != nil && event.Progress.Stage == "auto_compact_success" {
			t.Fatalf("auto no-op published semantic success: %+v", event)
		}
	}
	requireCompactionTerminalLifecycle(t, events, "compact_end")
}

func TestCompactStatusClearsOnFailure(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	q.compactor = lifecycleCompactor{err: errors.New("compact failed")}
	q.SetMessages([]types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("old"),
		types.UserMessage("tail"),
	})

	err := q.ForceCompact(context.Background())
	if err == nil || !strings.Contains(err.Error(), "compact failed") {
		t.Fatalf("ForceCompact error = %v, want compact failed", err)
	}
	if q.CompactStatus() != "" {
		t.Fatalf("compact status = %q, want cleared", q.CompactStatus())
	}
}

func TestRunCompactionEmitsDistinctTerminalProgress(t *testing.T) {
	tests := []struct {
		name       string
		runErr     error
		wantStages []string
	}{
		{name: "success", wantStages: []string{"compact_start", "compact_end"}},
		{name: "failure", runErr: errors.New("summary provider failed"), wantStages: []string{"compact_start", "compact_failed"}},
		{name: "cancelled", runErr: context.Canceled, wantStages: []string{"compact_start", "compact_cancelled"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := New(nil, nil, Config{MaxContextTokens: 100})
			var stages []string
			var terminalMetadata map[string]any
			result, err := q.runCompaction(context.Background(), "auto", 3, func(event Event) {
				if event.Type == EventProgress && event.Progress != nil {
					stages = append(stages, event.Progress.Stage)
					if event.Progress.Stage == "compact_failed" || event.Progress.Stage == "compact_cancelled" {
						terminalMetadata = event.Progress.Metadata
					}
				}
			}, func() (*compact.CompactionResult, error) {
				return nil, tt.runErr
			})
			if !errors.Is(err, tt.runErr) {
				t.Fatalf("runCompaction error = %v, want %v", err, tt.runErr)
			}
			if tt.runErr != nil && result != nil {
				t.Fatalf("runCompaction result = %+v on error, want nil", result)
			}
			if strings.Join(stages, ",") != strings.Join(tt.wantStages, ",") {
				t.Fatalf("progress stages = %v, want %v", stages, tt.wantStages)
			}
			if q.CompactStatus() != "" {
				t.Fatalf("compact status = %q, want cleared", q.CompactStatus())
			}
			if tt.runErr != nil && terminalMetadata["error"] != tt.runErr.Error() {
				t.Fatalf("terminal compaction evidence metadata = %+v, want error %q", terminalMetadata, tt.runErr)
			}
		})
	}
}

func TestForceCompactResultContractRejectsUnsafeResultsWithoutMutation(t *testing.T) {
	original := []types.Message{
		types.UserMessage("contract original question"),
		types.AssistantMessage("contract original answer"),
	}
	tests := []struct {
		name  string
		build func(target *QueryLoop) func([]types.Message, string) *compact.CompactionResult
	}{
		{
			name: "boundaryless changed result",
			build: func(*QueryLoop) func([]types.Message, string) *compact.CompactionResult {
				return func([]types.Message, string) *compact.CompactionResult {
					return &compact.CompactionResult{MessagesToKeep: []types.Message{types.UserMessage("unauthorized replacement")}}
				}
			},
		},
		{
			name: "foreign scoped boundary",
			build: func(*QueryLoop) func([]types.Message, string) *compact.CompactionResult {
				foreign := New(nil, nil, Config{MaxContextTokens: 100})
				boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"})
				boundary = foreign.sealRuntimeControlMessage(boundary)
				return func([]types.Message, string) *compact.CompactionResult {
					return &compact.CompactionResult{
						BoundaryMarker:  &boundary,
						SummaryMessages: []types.Message{types.UserMessage("foreign replacement")},
					}
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanupCalls := 0
			q := New(nil, nil, Config{
				MaxContextTokens: 100,
				PostCompactCleanup: func(context.Context) error {
					cleanupCalls++
					return nil
				},
			})
			q.SetMessages(append([]types.Message(nil), original...))
			q.lastResponseID = "contract-response-chain"
			q.disableResponseChain = true
			q.loadedToolNames["ContractTool"] = struct{}{}
			q.seenToolUseIDs["contract-tool-use"] = struct{}{}
			q.compactor = resultContractCompactor{build: test.build(q)}
			before, err := q.captureManualCompactInstallPreimage()
			if err != nil {
				t.Fatal(err)
			}

			var events []Event
			err = q.ForceCompactWithInstructionsAndEvents(context.Background(), "", func(event Event) {
				events = append(events, event)
			})
			if err == nil {
				t.Fatal("unsafe compactor result was accepted")
			}
			if cleanupCalls != 0 {
				t.Fatalf("post-compact cleanup calls = %d, want 0", cleanupCalls)
			}
			if got := q.Messages(); !reflect.DeepEqual(got, original) {
				t.Fatalf("unsafe result changed history: got=%#v want=%#v", got, original)
			}
			if q.lastResponseID != before.lastResponseID || q.disableResponseChain != before.disableResponseChain ||
				!reflect.DeepEqual(q.LoadedToolNames(), before.visible.loadedToolNames) ||
				!reflect.DeepEqual(q.SeenToolUseIDs(), before.visible.seenToolUseIDs) {
				t.Fatalf("unsafe result changed runtime state: response=%q disabled=%t loaded=%v seen=%v",
					q.lastResponseID, q.disableResponseChain, q.LoadedToolNames(), q.SeenToolUseIDs())
			}
			requireCompactionTerminalLifecycle(t, events, "compact_failed")
		})
	}
}

func TestForceCompactResultContractNormalizesNilAndBoundarylessNoop(t *testing.T) {
	original := []types.Message{
		types.UserMessage("short history question"),
		types.AssistantMessage("short history answer"),
	}
	tests := []struct {
		name         string
		build        func([]types.Message, string) *compact.CompactionResult
		wantCleanup  int
		wantResponse string
	}{
		{name: "nil result", wantResponse: "noop-response-chain"},
		{
			name:         "deep equal boundaryless result",
			wantCleanup:  1,
			wantResponse: "noop-response-chain",
			build: func(messages []types.Message, _ string) *compact.CompactionResult {
				return &compact.CompactionResult{MessagesToKeep: append([]types.Message(nil), messages...)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanupCalls := 0
			q := New(nil, nil, Config{
				MaxContextTokens: 100,
				PostCompactCleanup: func(context.Context) error {
					cleanupCalls++
					return nil
				},
			})
			q.SetMessages(append([]types.Message(nil), original...))
			q.lastResponseID = "noop-response-chain"
			q.compactor = resultContractCompactor{build: test.build}
			var events []Event
			if err := q.ForceCompactWithInstructionsAndEvents(context.Background(), "", func(event Event) {
				events = append(events, event)
			}); err != nil {
				t.Fatalf("legal compactor no-op failed: %v", err)
			}
			if cleanupCalls != test.wantCleanup {
				t.Fatalf("no-op cleanup calls = %d, want %d", cleanupCalls, test.wantCleanup)
			}
			if got := q.Messages(); !reflect.DeepEqual(got, original) || q.lastResponseID != test.wantResponse {
				t.Fatalf("no-op changed live state: messages=%#v response=%q", got, q.lastResponseID)
			}
			requireCompactionTerminalLifecycle(t, events, "compact_end")
		})
	}
}

func TestRunCompactionResultContractUsesExactProjectedInput(t *testing.T) {
	for _, trigger := range []string{"auto", "reactive"} {
		t.Run(trigger, func(t *testing.T) {
			durable := []types.Message{types.UserMessage("durable source"), types.AssistantMessage("durable tail")}
			projection := []types.Message{types.UserMessage("lossy provider projection")}
			q := New(nil, nil, Config{MaxContextTokens: 100})
			q.SetMessages(durable)
			var events []Event
			result, semanticNoop, err := q.runCompactionAgainst(context.Background(), trigger, 2, func(event Event) {
				events = append(events, event)
			}, projection, func() (*compact.CompactionResult, error) {
				return &compact.CompactionResult{MessagesToKeep: append([]types.Message(nil), projection...)}, nil
			})
			if err != nil || result != nil || !semanticNoop {
				t.Fatalf("projected no-op result=%#v semantic=%t err=%v, want nil/true/nil", result, semanticNoop, err)
			}
			if got := q.Messages(); !reflect.DeepEqual(got, durable) {
				t.Fatalf("projected no-op changed durable history: %#v", got)
			}
			requireCompactionTerminalLifecycle(t, events, "compact_end")

			events = nil
			result, _, err = q.runCompactionAgainst(context.Background(), trigger, 3, func(event Event) {
				events = append(events, event)
			}, projection, func() (*compact.CompactionResult, error) {
				return &compact.CompactionResult{MessagesToKeep: []types.Message{types.UserMessage("changed without boundary")}}, nil
			})
			if err == nil || result != nil {
				t.Fatalf("boundaryless changed result=%#v err=%v, want nil/error", result, err)
			}
			if got := q.Messages(); !reflect.DeepEqual(got, durable) {
				t.Fatalf("rejected projected result changed durable history: %#v", got)
			}
			requireCompactionTerminalLifecycle(t, events, "compact_failed")
		})
	}
}

func TestRunCompactionResultContractFreezesNestedInputBeforeCompactorMutation(t *testing.T) {
	mutableInput := map[string]any{"path": "before", "nested": map[string]any{"value": "before"}}
	input := []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: "mutating-tool", Name: "Read", Input: mutableInput,
	}}}}
	durable := cloneMessages(input)
	q := New(nil, nil, Config{MaxContextTokens: 100})
	q.SetMessages(durable)
	var events []Event

	result, _, err := q.runCompactionAgainst(context.Background(), "reactive", 1, func(event Event) {
		events = append(events, event)
	}, input, func() (*compact.CompactionResult, error) {
		mutableInput["path"] = "after"
		mutableInput["nested"].(map[string]any)["value"] = "after"
		return &compact.CompactionResult{MessagesToKeep: cloneMessages(input)}, nil
	})
	if err == nil || result != nil {
		t.Fatalf("in-place mutation result=%#v err=%v, want nil/error", result, err)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, durable) {
		t.Fatalf("in-place compactor mutation changed durable history: got=%#v want=%#v", got, durable)
	}
	requireCompactionTerminalLifecycle(t, events, "compact_failed")
}

func TestRunCompactionRejectsForeignControlAnywhereInResult(t *testing.T) {
	owner := New(nil, nil, Config{MaxContextTokens: 100})
	consumer := New(nil, nil, Config{MaxContextTokens: 100})
	control := types.UserMessage("foreign receipt")
	control.ID = "compact:reminder:v1:foreign"
	control.IsMeta = true
	control.InternalKind = types.InternalMessageKindCompactReminder
	foreign := control.WithInternalControlProvenance(messagecontrol.Runtime(), owner.internalControlScope)
	control.Content = []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "unbound receipt"}}
	control.ID = "compact:reminder:v1:unbound"
	unbound := control.WithInternalControlProvenance(messagecontrol.Runtime())
	foreignSummary := types.UserMessage("foreign summary")
	foreignSummary.ID = "compact:summary:v1"
	foreignSummary.IsMeta = true
	foreignSummary.InternalKind = types.InternalMessageKindCompactSummary
	foreignSummary = foreignSummary.WithInternalControlProvenance(messagecontrol.Runtime(), owner.internalControlScope)
	foreignReplacementMessages := compact.AppendContentReplacementRecordsForScope(
		[]types.Message{types.UserMessage("replacement carrier")},
		[]compact.ContentReplacementRecord{{Kind: "tool-result", ToolUseID: "foreign-tool", Replacement: "foreign replacement"}},
		messagecontrol.Runtime(), owner.internalControlScope,
	)
	foreignReplacement := foreignReplacementMessages[0].Content[len(foreignReplacementMessages[0].Content)-1]

	tests := []struct {
		name   string
		mutate func(*compact.CompactionResult)
	}{
		{name: "summary", mutate: func(result *compact.CompactionResult) { result.SummaryMessages = []types.Message{foreignSummary} }},
		{name: "attachment", mutate: func(result *compact.CompactionResult) { result.Attachments = []types.Message{foreign} }},
		{name: "unbound attachment", mutate: func(result *compact.CompactionResult) { result.Attachments = []types.Message{unbound} }},
		{name: "retained replacement", mutate: func(result *compact.CompactionResult) { result.MessagesToKeep = foreignReplacementMessages }},
		{name: "nested new message", mutate: func(result *compact.CompactionResult) {
			result.HookResults = []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: "nested", NewMessages: []types.Message{foreign},
			}}}}
		}},
		{name: "nested replacement", mutate: func(result *compact.CompactionResult) {
			result.HookResults = []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: "nested", ContentBlocks: []types.ContentBlock{foreignReplacement},
			}}}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []types.Message{types.UserMessage("original")}
			boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "reactive"})
			candidate := &compact.CompactionResult{BoundaryMarker: &boundary, SummaryMessages: []types.Message{types.UserMessage("safe summary")}}
			test.mutate(candidate)
			var events []Event
			result, _, err := consumer.runCompactionAgainst(context.Background(), "reactive", 1, func(event Event) {
				events = append(events, event)
			}, input, func() (*compact.CompactionResult, error) { return candidate, nil })
			if err == nil || result != nil {
				t.Fatalf("foreign result=%#v err=%v, want nil/error", result, err)
			}
			requireCompactionTerminalLifecycle(t, events, "compact_failed")
		})
	}
}

func requireCompactionTerminalLifecycle(t *testing.T, events []Event, wantTerminal string) {
	t.Helper()
	boundaries := 0
	var terminals []string
	for _, event := range events {
		if event.Type == EventCompactBoundary {
			boundaries++
		}
		if event.Type != EventProgress || event.Progress == nil {
			continue
		}
		switch event.Progress.Stage {
		case "compact_end", "compact_failed", "compact_cancelled":
			terminals = append(terminals, event.Progress.Stage)
		}
	}
	if boundaries != 0 || !reflect.DeepEqual(terminals, []string{wantTerminal}) {
		t.Fatalf("compaction boundaries/terminals = %d/%v, want 0/[%s]; events=%+v", boundaries, terminals, wantTerminal, events)
	}
}

func requireTransactionalCompactionFailure(t *testing.T, events []Event, privateCause error) {
	t.Helper()
	boundaries, failed := 0, 0
	for _, event := range events {
		if event.Type == EventCompactBoundary {
			boundaries++
		}
		if event.Type == EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_end", "compact_success", "auto_compact_success":
				t.Fatalf("transaction failure published success lifecycle: %+v", event)
			case "compact_failed":
				failed++
				if diagnostic, _ := event.Progress.Metadata["error"].(string); privateCause != nil && strings.Contains(diagnostic, privateCause.Error()) {
					t.Fatalf("transaction failure leaked private cause: %+v", event)
				}
			}
		}
		if event.Type == EventProviderUsage && event.Metadata["kind"] == "compaction" && event.Metadata["status"] != "failure" {
			t.Fatalf("transaction failure retained success usage: %+v", event)
		}
	}
	if boundaries != 0 || failed != 1 {
		t.Fatalf("transaction failure boundaries/failed = %d/%d, want 0/1; events=%+v", boundaries, failed, events)
	}
}

func TestRunCompactionAuthenticatesCompleteBoundaryBeforeEmission(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	marker := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger: "auto", PreCompactTokenCount: 1200, PreviousTailIdentifier: "assistant:tail",
		PreCompactDiscoveredTools: []string{"Read"},
		PreservedSegment:          &compact.PreservedSegmentMetadata{StartIndex: 4, Count: 2, Anchor: "assistant:tail", Direction: "tail"},
	})
	compactionResult := &compact.CompactionResult{
		BoundaryMarker:            &marker,
		SummaryMessages:           []types.Message{types.UserMessage("complete summary evidence")},
		UserDisplayMessage:        "hook display evidence",
		PostCompactTokenCount:     300,
		TruePostCompactTokenCount: 280,
	}
	var events []Event

	result, err := q.runCompaction(context.Background(), "auto", 4, func(event Event) {
		events = append(events, event)
	}, func() (*compact.CompactionResult, error) {
		return compactionResult, nil
	})
	if err != nil || result == nil {
		t.Fatalf("runCompaction result=%+v err=%v", result, err)
	}
	if _, ok := compact.ParseCompactBoundaryMessage(*result.BoundaryMarker); !ok {
		t.Fatal("result boundary was emitted without runtime provenance")
	}

	var lifecycle []string
	var boundary *CompactBoundaryEvent
	for i := range events {
		switch {
		case events[i].Type == EventProgress && events[i].Progress != nil && (events[i].Progress.Stage == "compact_start" || events[i].Progress.Stage == "compact_end"):
			lifecycle = append(lifecycle, events[i].Progress.Stage)
		case events[i].Type == EventCompactBoundary:
			lifecycle = append(lifecycle, string(events[i].Type))
			boundary = events[i].Compact
		}
	}
	wantLifecycle := []string{"compact_start", string(EventCompactBoundary), "compact_end"}
	if strings.Join(lifecycle, ",") != strings.Join(wantLifecycle, ",") {
		t.Fatalf("compaction lifecycle = %v, want %v", lifecycle, wantLifecycle)
	}
	if boundary == nil || boundary.PreCompactTokenCount != 1200 || boundary.PostCompactTokenCount != 300 || boundary.TruePostCompactTokenCount != 280 || boundary.PreviousTailIdentifier != "assistant:tail" || boundary.PreservedSegment == nil || boundary.PreservedSegment.Count != 2 {
		t.Fatalf("authorized boundary metadata = %+v", boundary)
	}
	if boundary.Summary != "complete summary evidence" || boundary.UserDisplayMessage != "hook display evidence" {
		t.Fatalf("authorized boundary lost summary/display evidence: %+v", boundary)
	}
}

func TestRunCompactionEmitsStructuredUsageOnceOnSuccess(t *testing.T) {
	usage := &types.Usage{
		InputTokens:              120,
		OutputTokens:             18,
		CacheCreationInputTokens: 25,
		CacheReadInputTokens:     80,
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: 2,
		},
	}
	sc := &compact.SummaryCompactor{}
	q := New(nil, nil, Config{MaxContextTokens: 100, Model: "compact-model"})
	q.compactor = sc
	var events []Event

	result, err := q.runCompaction(context.Background(), "auto", 4, func(event Event) {
		events = append(events, event)
	}, func() (*compact.CompactionResult, error) {
		// SummaryCompactor telemetry and the returned result both expose the
		// same provider call. Consumers must receive it exactly once.
		sc.OnTelemetry(compact.CompactionTelemetryEvent{
			Kind:            compact.CompactionTelemetrySuccess,
			Trigger:         "auto",
			CompactionUsage: usage,
		})
		boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "auto"})
		return &compact.CompactionResult{
			BoundaryMarker:  &boundary,
			SummaryMessages: []types.Message{types.UserMessage("usage summary")},
			CompactionUsage: usage,
		}, nil
	})
	if err != nil || result == nil {
		t.Fatalf("runCompaction result=%+v err=%v, want success", result, err)
	}

	usageEvents := providerUsageEventsOfKind(events, "compaction")
	if len(usageEvents) != 1 {
		t.Fatalf("compaction usage events = %d, want 1; events=%+v", len(usageEvents), events)
	}
	got := usageEvents[0]
	if got.Usage == nil || *got.Usage != *usage {
		t.Fatalf("structured compaction usage = %+v, want %+v", got.Usage, usage)
	}
	if got.TurnCount != 4 || got.Metadata["kind"] != "compaction" || got.Metadata["model"] != "compact-model" ||
		got.Metadata["trigger"] != "auto" || got.Metadata["status"] != "success" {
		t.Fatalf("compaction usage metadata = %+v, turn=%d", got.Metadata, got.TurnCount)
	}
}

func TestRunCompactionEmitsStructuredUsageOnceOnFailure(t *testing.T) {
	usage := &types.Usage{
		InputTokens:          90,
		OutputTokens:         7,
		CacheReadInputTokens: 60,
	}
	sc := &compact.SummaryCompactor{}
	q := New(nil, nil, Config{MaxContextTokens: 100, Model: "compact-model"})
	q.compactor = sc
	var events []Event
	wantErr := errors.New("summary response was rejected")

	result, err := q.runCompaction(context.Background(), "reactive", 6, func(event Event) {
		events = append(events, event)
	}, func() (*compact.CompactionResult, error) {
		sc.OnTelemetry(compact.CompactionTelemetryEvent{
			Kind:            compact.CompactionTelemetryFailure,
			Trigger:         "reactive",
			CompactionUsage: usage,
		})
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) || result != nil {
		t.Fatalf("runCompaction result=%+v err=%v, want nil/%v", result, err, wantErr)
	}

	usageEvents := providerUsageEventsOfKind(events, "compaction")
	if len(usageEvents) != 1 {
		t.Fatalf("compaction usage events = %d, want 1; events=%+v", len(usageEvents), events)
	}
	got := usageEvents[0]
	if got.Usage == nil || *got.Usage != *usage {
		t.Fatalf("failed compaction usage = %+v, want %+v", got.Usage, usage)
	}
	if got.Metadata["model"] != "compact-model" || got.Metadata["trigger"] != "reactive" || got.Metadata["status"] != "failure" {
		t.Fatalf("failed compaction usage metadata = %+v", got.Metadata)
	}
}

func providerUsageEventsOfKind(events []Event, kind string) []Event {
	var matches []Event
	for _, event := range events {
		if event.Type == EventProviderUsage && event.Metadata["kind"] == kind {
			matches = append(matches, event)
		}
	}
	return matches
}

func TestForceCompactCancellationPreservesHistoryAndCancellationSemantics(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	original := []types.Message{
		types.UserMessage("keep original"),
		types.AssistantMessage("assistant original"),
	}
	q.SetMessages(append([]types.Message(nil), original...))
	q.compactor = lifecycleCompactor{err: context.Canceled}

	err := q.ForceCompact(context.Background())
	if err == nil {
		t.Fatal("ForceCompact succeeded, want cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want errors.Is(context.Canceled)", err)
	}
	if got := joinedText(q.Messages()); got != joinedText(original) {
		t.Fatalf("messages mutated after cancellation:\ngot  %q\nwant %q", got, joinedText(original))
	}
}
