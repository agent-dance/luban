package loop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

func TestQueryLoopWebSearchNestedUsageIsIncludedInTurn(t *testing.T) {
	outer := &types.Usage{InputTokens: 100, OutputTokens: 20}
	results := []types.ToolResultBlock{{
		Usage: &types.Usage{
			InputTokens: 5, OutputTokens: 2,
			ServerToolUse: types.ServerToolUsage{WebSearchRequests: 3},
		},
	}}
	got := mergeToolResultUsage(outer, results)
	if got.InputTokens != 105 || got.OutputTokens != 22 || got.ServerToolUse.WebSearchRequests != 3 {
		t.Fatalf("merged usage = %+v", got)
	}
}

func TestQueryLoopIdentityBearingNestedUsageIsEmittedSeparately(t *testing.T) {
	outer := &types.Usage{InputTokens: 100, OutputTokens: 20}
	nested := &types.Usage{InputTokens: 30, OutputTokens: 5, CacheReadInputTokens: 22}
	results := []types.ToolResultBlock{{
		Usage: nested,
		Metadata: map[string]string{
			"usage.provider": "child-provider",
			"usage.model":    "child-model",
		},
	}}
	var events []stream.Event
	got := accountToolResultUsage(outer, results, 4, func(event stream.Event) { events = append(events, event) })

	if got.InputTokens != 100 || got.OutputTokens != 20 {
		t.Fatalf("child-model usage was merged into parent-model turn: %+v", got)
	}
	if len(events) != 1 || events[0].Type != stream.EventProviderUsage || events[0].Usage == nil || *events[0].Usage != *nested {
		t.Fatalf("nested provider usage events = %+v", events)
	}
	if events[0].Metadata["provider"] != "child-provider" || events[0].Metadata["model"] != "child-model" || events[0].Metadata["kind"] != "nested_tool" {
		t.Fatalf("nested usage metadata = %+v", events[0].Metadata)
	}
}

func TestQueryLoopWebSearchStreamPreservesServerBlocks(t *testing.T) {
	serverRaw := json.RawMessage(`{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{}}`)
	resultRaw := json.RawMessage(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[]}`)
	providerStream := make(chan types.StreamEvent, 7)
	providerStream <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeServerToolUse, ID: "srv_1", Name: "web_search", RawJSON: serverRaw}}
	providerStream <- types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"query":"go"}`}}
	providerStream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	providerStream <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 1, ContentBlock: &types.ContentDelta{Type: types.ContentTypeWebSearchToolResult, ToolUseID: "srv_1", RawJSON: resultRaw}}
	providerStream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 1}
	providerStream <- types.StreamEvent{Type: types.EventMessageStop}
	close(providerStream)

	message, _, _, err := (&QueryLoop{}).processStream(context.Background(), providerStream, 1, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if message == nil || len(message.Content) != 2 {
		t.Fatalf("message content = %#v", message)
	}
	for i, want := range []types.ContentType{types.ContentTypeServerToolUse, types.ContentTypeWebSearchToolResult} {
		block, ok := message.Content[i].(types.UnknownBlock)
		if !ok || block.Type != want || len(block.Raw) == 0 {
			t.Fatalf("block %d = %#v, want raw %s", i, message.Content[i], want)
		}
	}
}
