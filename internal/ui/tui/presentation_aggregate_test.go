package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestRoutineSuccessAggregationIsReversibleAndKeepsMemberIndex(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research"}
	for index, path := range []string{"/tmp/a", "/tmp/b"} {
		id := "read-" + string(rune('a'+index))
		if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": path}}); err != nil {
			t.Fatal(err)
		}
		ctx.Outcome = OutcomeSucceeded
		if err := store.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: id, Data: map[string]any{"line_count": index + 1}, Outcome: types.ToolOutcomeSucceeded}); err != nil {
			t.Fatal(err)
		}
	}

	first := observationByToolUseID(t, store.Snapshot(), "read-a")
	second := observationByToolUseID(t, store.Snapshot(), "read-b")
	if !first.Aggregation.Hidden || first.Decision.EffectiveLevel != PresentationHiddenMember {
		t.Fatalf("first member was not hidden by a proven group: %+v", first.Aggregation)
	}
	if !second.Aggregation.Representative || second.Aggregation.Count != 2 || second.Decision.EffectiveLevel != PresentationFolded {
		t.Fatalf("latest member was not the visible representative: %+v", second.Aggregation)
	}
	if first.Aggregation.GroupID == "" || first.Aggregation.GroupID != second.Aggregation.GroupID {
		t.Fatalf("group IDs differ: first=%q second=%q", first.Aggregation.GroupID, second.Aggregation.GroupID)
	}
	group, ok := store.Aggregate(second.Aggregation.GroupID)
	if !ok || len(group.MemberIDs) != 2 || group.MemberIDs[0] != first.ID || group.MemberIDs[1] != second.ID {
		t.Fatalf("aggregate member index = %+v", group)
	}

	if err := store.SetDisclosure(first.ID, DisclosureState{Level: DisclosureEvidence, HasMore: true, UserPinned: true}); err != nil {
		t.Fatal(err)
	}
	first, _ = store.Get(first.ID)
	second, _ = store.Get(second.ID)
	if first.Aggregation.Hidden || second.Aggregation.Hidden || first.Decision.EffectiveLevel != PresentationEvidence {
		t.Fatalf("pinning did not reverse aggregation: first=%+v second=%+v", first.Aggregation, second.Aggregation)
	}
	if len(store.AggregateSnapshot()) != 0 {
		t.Fatalf("single-member group remained visible: %+v", store.AggregateSnapshot())
	}
}

func TestAggregateFreezeKeepsLateMemberVisibleAndIndexesEvidence(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research", Outcome: OutcomeSucceeded}
	applyRead := func(id, path string) {
		t.Helper()
		if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": path}}); err != nil {
			t.Fatal(err)
		}
		if err := store.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: id, Content: "evidence-" + id, Outcome: types.ToolOutcomeSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	applyRead("a", "/tmp/a")
	applyRead("b", "/tmp/b")
	if frozen := store.FreezeAggregates("session", "turn"); frozen != 1 {
		t.Fatalf("frozen groups=%d, want 1", frozen)
	}
	group := store.AggregateSnapshot()[0]
	if !group.Frozen || group.Live || group.ObjectCount != 2 || group.EvidenceCount < 2 || len(group.EvidenceRefs) < 2 {
		t.Fatalf("frozen group index=%+v", group)
	}

	applyRead("late", "/tmp/late")
	late := observationByToolUseID(t, store.Snapshot(), "late")
	if late.Aggregation.GroupID != "" || late.Decision.EffectiveLevel != PresentationFolded {
		t.Fatalf("late member mutated frozen group: %+v", late)
	}
	group = store.AggregateSnapshot()[0]
	if len(group.MemberIDs) != 2 || group.Summary != "Read · 2 operations" {
		t.Fatalf("frozen summary changed after late output: %+v", group)
	}
}

func TestAggregateInsertionScalesLinearlyAtHundredThousandMembers(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	const memberCount = 100000
	store.observations = make([]Observation, 0, memberCount)
	store.byID = make(map[string]int, memberCount)
	store.aggregateKeyByObservation = make(map[string]string, memberCount)
	template := Observation{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "bulk", Presentation: FormattedPresentation{Family: FamilyFileRead, AggregationIntent: "read"}}
	aggregateKey := observationAggregateKey(template)
	started := time.Now()
	store.mu.Lock()
	for index := 0; index < memberCount; index++ {
		observation := Observation{
			ID: fmt.Sprintf("observation-%06d", index), SessionID: "session", TurnID: "turn",
			ActorID: "agent", WorkUnitID: "bulk", Outcome: OutcomeSucceeded,
			Disclosure:   DisclosureState{Level: DisclosureSummary},
			Presentation: FormattedPresentation{Family: FamilyFileRead, Object: fmt.Sprintf("/tmp/%d", index), AggregationIntent: "read"},
			Decision:     PresentationDecision{DefaultLevel: PresentationFolded, EffectiveLevel: PresentationFolded, AggregationEligible: true},
		}
		store.appendLocked(observation)
		store.addObservationToAggregateLocked(len(store.observations)-1, aggregateKey)
	}
	store.mu.Unlock()
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("100k aggregate insertion took %s; hot path is no longer linear", elapsed)
	}
	groups := store.AggregateSnapshot()
	if len(groups) != 1 || len(groups[0].MemberIDs) != memberCount || groups[0].EvidenceCount != 0 || len(groups[0].ObjectSamples) != 20 {
		t.Fatalf("100k aggregate index=%+v", groups)
	}
	first, _ := store.Get("observation-000000")
	last, _ := store.Get("observation-099999")
	if !first.Aggregation.Hidden || !last.Aggregation.Representative || last.Aggregation.Count != memberCount {
		t.Fatalf("100k projection first=%+v last=%+v", first.Aggregation, last.Aggregation)
	}
}

func TestWarningNeverJoinsRoutineSuccessAggregate(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research", Outcome: OutcomeSucceeded}
	for index := 0; index < 3; index++ {
		id := string(rune('a' + index))
		if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: "Grep", Input: map[string]any{"pattern": "TODO", "path": "/workspace"}}); err != nil {
			t.Fatal(err)
		}
		result := types.ToolResultBlock{ToolUseID: id, Data: map[string]any{"match_count": index + 1}, Outcome: types.ToolOutcomeSucceeded}
		if index == 2 {
			result.Completeness.Source = types.ToolResultCompletenessSourceTruncated
		}
		if err := store.ApplyToolResult(ctx, result); err != nil {
			t.Fatal(err)
		}
	}

	warning := observationByToolUseID(t, store.Snapshot(), "c")
	if warning.Aggregation.GroupID != "" || warning.Decision.EffectiveLevel < PresentationStructured || !containsPresentationReason(warning.Decision.Reasons, ReasonWarning) {
		t.Fatalf("warning was swallowed by aggregate: %+v", warning)
	}
	groups := store.AggregateSnapshot()
	if len(groups) != 1 || len(groups[0].MemberIDs) != 2 {
		t.Fatalf("routine group = %+v, want exactly two safe members", groups)
	}
}

func TestAggregateExceptionRotatesSuccessSuccessWarningSuccess(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research", Outcome: OutcomeSucceeded}
	apply := func(id string, warning bool) {
		t.Helper()
		if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: "Grep", Input: map[string]any{"pattern": "TODO", "path": "/workspace"}}); err != nil {
			t.Fatal(err)
		}
		result := types.ToolResultBlock{ToolUseID: id, Data: map[string]any{"match_count": 1}, Outcome: types.ToolOutcomeSucceeded}
		if warning {
			result.Completeness.Source = types.ToolResultCompletenessSourceTruncated
		}
		if err := store.ApplyToolResult(ctx, result); err != nil {
			t.Fatal(err)
		}
	}

	apply("success-a", false)
	apply("success-b", false)
	apply("warning", true)
	apply("success-c", false)

	groups := store.AggregateSnapshot()
	if len(groups) != 1 || len(groups[0].MemberIDs) != 2 || !groups[0].Frozen || groups[0].Live {
		t.Fatalf("pre-warning group was not frozen at the exception: %+v", groups)
	}
	warning := observationByToolUseID(t, store.Snapshot(), "warning")
	after := observationByToolUseID(t, store.Snapshot(), "success-c")
	if warning.Aggregation.GroupID != "" || after.Aggregation.GroupID != "" {
		t.Fatalf("warning/later success crossed exception boundary: warning=%+v after=%+v", warning.Aggregation, after.Aggregation)
	}

	apply("success-d", false)
	groups = store.AggregateSnapshot()
	if len(groups) != 2 {
		t.Fatalf("later successes did not form a rotated group: %+v", groups)
	}
	if groups[0].ID == groups[1].ID || len(groups[0].MemberIDs) != 2 || len(groups[1].MemberIDs) != 2 {
		t.Fatalf("rotated group identity/members = %+v", groups)
	}
}

func TestAggregateKeySeparatesSearchQueriesAndMCPRouting(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		firstInput map[string]any
		otherInput map[string]any
	}{
		{
			name: "search query", tool: "Grep",
			firstInput: map[string]any{"pattern": "TODO", "path": "/workspace"},
			otherInput: map[string]any{"pattern": "FIXME", "path": "/workspace"},
		},
		{
			name: "web query", tool: "WebSearch",
			firstInput: map[string]any{"query": "claude display policy"},
			otherInput: map[string]any{"query": "codex display policy"},
		},
		{
			name: "web url", tool: "WebFetch",
			firstInput: map[string]any{"url": "https://example.com/alpha"},
			otherInput: map[string]any{"url": "https://example.com/beta"},
		},
		{
			name: "mcp server", tool: "ReadMcpResourceTool",
			firstInput: map[string]any{"server": "alpha", "uri": "memo://shared"},
			otherInput: map[string]any{"server": "beta", "uri": "memo://shared"},
		},
		{
			name: "mcp capability", tool: "mcp__github__issues",
			firstInput: map[string]any{"query": "bug"},
			otherInput: map[string]any{"tool_name": "pull_requests", "query": "bug"},
		},
		{
			name: "mcp uri", tool: "ReadMcpResourceTool",
			firstInput: map[string]any{"server": "alpha", "uri": "memo://one"},
			otherInput: map[string]any{"server": "alpha", "uri": "memo://two"},
		},
		{
			name: "lsp operation", tool: "LSP",
			firstInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 4},
			otherInput: map[string]any{"operation": "findReferences", "filePath": "/workspace/main.go", "line": 12, "character": 4},
		},
		{
			name: "lsp file", tool: "LSP",
			firstInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 4},
			otherInput: map[string]any{"operation": "hover", "filePath": "/workspace/other.go", "line": 12, "character": 4},
		},
		{
			name: "lsp line", tool: "LSP",
			firstInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 4},
			otherInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 13, "character": 4},
		},
		{
			name: "lsp character", tool: "LSP",
			firstInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 4},
			otherInput: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 5},
		},
	}
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			family := CommandFamilyForTool(test.tool)
			first := Observation{SessionID: ctx.SessionID, TurnID: ctx.TurnID, ActorID: ctx.ActorID, WorkUnitID: ctx.WorkUnitID, ToolName: test.tool, ToolInput: test.firstInput,
				Presentation: FormattedPresentation{Family: family, AggregationIntent: string(family) + ":" + test.tool}}
			other := first
			other.ToolInput = test.otherInput
			if firstKey, otherKey := observationAggregateKey(first), observationAggregateKey(other); firstKey == otherKey {
				t.Fatalf("different domain intents share aggregate key %q", firstKey)
			}
		})
	}
}

func TestSearchAndMCPGroupsDoNotCrossQueryOrServer(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "research", Outcome: OutcomeSucceeded}
	apply := func(id, tool string, input map[string]any) {
		t.Helper()
		if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: tool, Input: input}); err != nil {
			t.Fatal(err)
		}
		if err := store.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: id, Outcome: types.ToolOutcomeSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"TODO", "FIXME"} {
		for index := 0; index < 2; index++ {
			apply(fmt.Sprintf("search-%s-%d", query, index), "Grep", map[string]any{"pattern": query, "path": "/workspace"})
		}
	}
	for _, server := range []string{"alpha", "beta"} {
		for index := 0; index < 2; index++ {
			apply(fmt.Sprintf("mcp-%s-%d", server, index), "ReadMcpResourceTool", map[string]any{"server": server, "uri": "memo://shared"})
		}
	}

	groups := store.AggregateSnapshot()
	if len(groups) != 4 {
		t.Fatalf("cross-query/server aggregation produced %d groups: %+v", len(groups), groups)
	}
	for _, group := range groups {
		if len(group.MemberIDs) != 2 {
			t.Fatalf("domain group crossed intent boundary: %+v", group)
		}
	}
}
