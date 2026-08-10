package loop

import (
	"context"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestProcessStreamEmitsContentFreeToolInputActivityAtBlockStart(t *testing.T) {
	ql := &QueryLoop{}
	var progress []stream.Event
	events := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call-1", Name: "ApplyPatch",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "input_json_delta", PartialJSON: `{"patch":"sensitive project content"}`,
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop},
	)

	_, _, _, err := ql.processStream(context.Background(), events, 3, func(event stream.Event) {
		if event.Type == stream.EventProgress {
			progress = append(progress, event)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 2 || progress[0].Progress == nil || progress[1].Progress == nil {
		t.Fatalf("tool-input activity events = %+v", progress)
	}
	got := progress[0]
	if got.TurnCount != 3 || got.Progress.Stage != stream.ProgressStageLLMToolInput || got.Progress.Metadata["tool_name"] != "ApplyPatch" {
		t.Fatalf("tool-input activity = %+v", got)
	}
	if progress[1].Progress.Metadata["tool_input_bytes"] != len(`{"patch":"sensitive project content"}`) {
		t.Fatalf("tool-input byte progress = %+v", progress[1])
	}
	for _, event := range progress {
		if event.Text != "" || event.Progress.Message != "" {
			t.Fatalf("tool-input activity leaked display or input content: %+v", event)
		}
		if _, leaked := event.Progress.Metadata["partial_json"]; leaked {
			t.Fatalf("tool-input activity leaked partial input metadata: %+v", event)
		}
	}
}

func TestToolContinuationActivityFollowsCommittedToolRoundAndPrecedesNextRequest(t *testing.T) {
	p := &mockProvider{responses: [][]types.StreamEvent{
		{
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "call-1", Name: "Echo",
			}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
				Type: "input_json_delta", PartialJSON: `{"text":"hello"}`,
			}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		},
		{
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "done"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		},
	}}
	reg := registry.New()
	reg.Register(&mockEchoTool{})
	ql := New(p, reg, Config{MaxTurns: 4, MaxTokens: 1024})

	var sequence []string
	err := ql.Run(context.Background(), "run echo", func(event stream.Event) {
		switch {
		case event.Type == stream.EventRequestStart:
			sequence = append(sequence, "request")
		case event.Type == stream.EventToolRoundMetrics:
			sequence = append(sequence, "tools-committed")
		case event.Type == stream.EventProgress && event.Progress != nil && event.Progress.Stage == stream.ProgressStageLLMWaitingAfterTools:
			sequence = append(sequence, "waiting-after-tools")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"request", "tools-committed", "waiting-after-tools", "request"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("model activity sequence = %v, want %v", sequence, want)
	}
}
