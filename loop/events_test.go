package loop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestToolUseEventEmittedBeforeToolCompletes(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseTool := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseTool)

	reg := registry.New()
	reg.Register(&orderedBatchTool{
		name:       "SlowTool",
		concurrent: true,
		execute: func(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
			close(started)
			select {
			case <-release:
				return types.ToolResult{Content: "done"}, nil
			case <-ctx.Done():
				return types.ToolResult{}, ctx.Err()
			}
		},
	})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("call_slow", "SlowTool", `{}`, nil)},
		{Events: parityTextEvents("finished")},
	})
	ql := New(prov, reg, Config{MaxTurns: 3, MaxTokens: 1024})

	events := make(chan Event, 16)
	done := make(chan error, 1)
	go func() {
		done <- ql.Run(context.Background(), "run the slow tool", func(evt Event) {
			events <- evt
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		releaseTool()
		<-done
		t.Fatal("slow tool did not start")
	}

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case evt := <-events:
			if evt.Type == EventToolResult {
				releaseTool()
				<-done
				t.Fatal("tool_result was emitted before the blocked tool completed")
			}
			if evt.Type == EventToolUse && evt.ToolUse != nil && evt.ToolUse.ID == "call_slow" {
				goto toolUseObserved
			}
		case <-deadline:
			releaseTool()
			<-done
			t.Fatal("tool_use event was not emitted until after the tool completed")
		}
	}

toolUseObserved:

	select {
	case evt := <-events:
		if evt.Type == EventToolResult {
			releaseTool()
			<-done
			t.Fatal("tool_result was emitted before the blocked tool completed")
		}
	default:
	}

	releaseTool()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestEventExtendedFieldsAreOptionalAndStructured(t *testing.T) {
	evt := Event{
		Type:           EventTombstone,
		MessageID:      "msg_123",
		TerminalReason: "model_fallback",
		Tombstone: &TombstoneEvent{
			MessageID: "msg_123",
			Reason:    "model_fallback",
			Summary:   "assistant message replaced by fallback marker",
		},
		Metadata: map[string]any{"provider": "mock"},
	}

	if evt.Type != EventTombstone {
		t.Fatalf("Type = %q, want %q", evt.Type, EventTombstone)
	}
	if evt.Tombstone == nil || evt.Tombstone.Reason != "model_fallback" {
		t.Fatalf("Tombstone = %+v, want structured tombstone payload", evt.Tombstone)
	}
	if evt.Text != "" || evt.ToolUse != nil || evt.ToolResult != nil {
		t.Fatalf("legacy fields should remain optional zero values: %+v", evt)
	}
}

func TestMaxTurnsReachedEventEmitted(t *testing.T) {
	toolTurn := []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse,
			ID:   "call_loop",
			Name: "Echo",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type:        "input_json_delta",
			PartialJSON: `{"text":"again"}`,
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: toolTurn},
		{Events: toolTurn},
	})
	reg := registry.New()
	reg.Register(&mockEchoTool{})
	ql := New(prov, reg, Config{MaxTurns: 1, MaxTokens: 1024})

	var events []Event
	err := ql.Run(context.Background(), "loop", func(evt Event) {
		events = append(events, evt)
	})
	if err == nil {
		t.Fatal("Run error = nil, want max turns error")
	}
	var maxTurnsErr *MaxTurnsError
	if !errors.As(err, &maxTurnsErr) {
		t.Fatalf("Run error = %T, want *MaxTurnsError", err)
	}

	var found *Event
	for i := range events {
		if events[i].Type == EventMaxTurnsReached {
			found = &events[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("missing %q event in %+v", EventMaxTurnsReached, events)
	}
	if found.TerminalReason != "max_turns_reached" {
		t.Fatalf("TerminalReason = %q, want max_turns_reached", found.TerminalReason)
	}
	if found.MaxTurns == nil || found.MaxTurns.MaxTurns != 1 || found.MaxTurns.TurnCount != 2 {
		t.Fatalf("MaxTurns payload = %+v, want MaxTurns=1 TurnCount=2", found.MaxTurns)
	}
}

func TestEventCompactBoundaryEmittedAfterAutoCompact(t *testing.T) {
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityToolUseEventsWithUsage("call_1", "Echo", `{"text":"large"}`, &types.Usage{InputTokens: 100000, OutputTokens: 10})},
		{Events: parityTextEvents("done")},
	})
	reg := registry.New()
	reg.Register(&mockEchoTool{})
	ql := New(prov, reg, Config{
		MaxTurns:         3,
		MaxContextTokens: 100,
		MaxTokens:        1024,
	})
	ql.compactor = &countingCompactor{}

	var events []Event
	if err := ql.Run(context.Background(), "run tool", func(evt Event) {
		events = append(events, evt)
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found *Event
	var lifecycle []string
	for i := range events {
		switch {
		case events[i].Type == EventProgress && events[i].Progress != nil && (events[i].Progress.Stage == "compact_start" || events[i].Progress.Stage == "compact_end"):
			lifecycle = append(lifecycle, events[i].Progress.Stage)
		case events[i].Type == EventCompactBoundary:
			lifecycle = append(lifecycle, string(EventCompactBoundary))
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("missing %q event in %+v", EventCompactBoundary, events)
	}
	if found.Compact == nil || found.Compact.Trigger != "auto" {
		t.Fatalf("Compact payload = %+v, want trigger auto", found.Compact)
	}
	if len(lifecycle) == 0 || len(lifecycle)%3 != 0 {
		t.Fatalf("auto compaction lifecycle = %v, want complete start/boundary/end triplets", lifecycle)
	}
	wantLifecycle := []string{"compact_start", string(EventCompactBoundary), "compact_end"}
	for i := 0; i < len(lifecycle); i += len(wantLifecycle) {
		if strings.Join(lifecycle[i:i+len(wantLifecycle)], ",") != strings.Join(wantLifecycle, ",") {
			t.Fatalf("auto compaction lifecycle[%d] = %v, want %v (all=%v)", i/len(wantLifecycle), lifecycle[i:i+len(wantLifecycle)], wantLifecycle, lifecycle)
		}
	}
}

func TestAutoCompactFailureAndCancellationDoNotEmitBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, terminalStage string
		err                 error
	}{
		{name: "failure", terminalStage: "compact_failed", err: errors.New("auto summary failed")},
		{name: "cancellation", terminalStage: "compact_cancelled", err: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov := newParityFakeProvider([]parityProviderTurn{
				{Events: parityToolUseEventsWithUsage("call_1", "Echo", `{"text":"large"}`, &types.Usage{InputTokens: 100000, OutputTokens: 10})},
				{Events: parityTextEvents("done")},
			})
			reg := registry.New()
			reg.Register(&mockEchoTool{})
			ql := New(prov, reg, Config{MaxTurns: 3, MaxContextTokens: 100, MaxTokens: 1024})
			ql.compactor = lifecycleCompactor{err: tc.err}

			boundaryCount := 0
			var terminalStages []string
			if err := ql.Run(context.Background(), "run tool", func(event Event) {
				if event.Type == EventCompactBoundary {
					boundaryCount++
				}
				if event.Type == EventProgress && event.Progress != nil && strings.HasPrefix(event.Progress.Stage, "compact_") && event.Progress.Stage != "compact_start" {
					terminalStages = append(terminalStages, event.Progress.Stage)
				}
			}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if boundaryCount != 0 {
				t.Fatalf("auto %s emitted %d false boundary event(s)", tc.name, boundaryCount)
			}
			if len(terminalStages) == 0 {
				t.Fatalf("auto %s emitted no terminal compaction progress", tc.name)
			}
			for _, stage := range terminalStages {
				if stage != tc.terminalStage {
					t.Fatalf("auto %s terminal stages = %v, want only %q", tc.name, terminalStages, tc.terminalStage)
				}
			}
		})
	}
}

func TestCompactBoundaryEventPreservesCompletePersistedMetadata(t *testing.T) {
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger: "reactive", PreCompactTokenCount: 1200, PreviousTailIdentifier: "assistant:tail",
		PreCompactDiscoveredTools: []string{"Read", "Bash"},
		PreservedSegment:          &compact.PreservedSegmentMetadata{StartIndex: 8, Count: 4, Anchor: "assistant:tail", Direction: "tail"},
	}, messagecontrol.Runtime())
	event := newCompactBoundaryEvent(&compact.CompactionResult{
		BoundaryMarker: &boundary, PostCompactTokenCount: 300, TruePostCompactTokenCount: 280,
		SummaryMessages: []types.Message{types.UserMessage("complete summary evidence")}, UserDisplayMessage: "hook display evidence",
	}, "reactive", 3)

	encoded, err := json.Marshal(event.Compact)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{"pre_compact_discovered_tools", "Read", "Bash", "preserved_segment", "start_index", "assistant:tail", "complete summary evidence", "hook display evidence"} {
		if !strings.Contains(text, want) {
			t.Fatalf("compact boundary event omitted %q: %s", want, text)
		}
	}
}
