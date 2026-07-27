package loop

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type customStreamTool struct{}

func (*customStreamTool) Name() string        { return "ApplyPatch" }
func (*customStreamTool) Description() string { return "test custom patch" }
func (*customStreamTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{"patch": map[string]any{"type": "string"}}, "patch")
}
func (*customStreamTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (*customStreamTool) CustomToolInputFormat() (types.ToolInputFormat, bool) {
	return types.ToolInputFormat{Type: "grammar", Syntax: "lark", Definition: "start: PATCH"}, true
}
func (*customStreamTool) DecodeCustomToolInput(raw string) (map[string]any, error) {
	return map[string]any{"patch": raw}, nil
}

func TestProcessStreamCustomToolPreservesRawAndCanonicalExecutionInput(t *testing.T) {
	reg := registry.New()
	reg.Register(&customStreamTool{})
	query := &QueryLoop{registry: reg}
	patch := "*** Begin Patch\n*** Add File: note.txt\n+hello \\ world\n*** End Patch"
	streamEvents := makeStreamChan(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call_patch", Name: "ApplyPatch", ToolType: types.ToolDefinitionTypeCustom,
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "input_text_delta", ToolType: types.ToolDefinitionTypeCustom, PartialText: "discarded fragment",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "tool_state_final", ID: "call_patch", Name: "ApplyPatch",
			ToolType: types.ToolDefinitionTypeCustom, PartialText: patch,
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: stopReasonPointer(types.StopReasonToolUse)},
		types.StreamEvent{Type: types.EventMessageStop},
	)
	message, _, stopReason, err := query.processStream(context.Background(), streamEvents, 1, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	uses := message.GetToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool uses = %#v", uses)
	}
	use := uses[0]
	if use.ToolType != types.ToolDefinitionTypeCustom || use.RawInput != patch || use.Input["patch"] != patch {
		t.Fatalf("custom tool use = %#v", use)
	}
	if stopReason == nil || *stopReason != types.StopReasonToolUse {
		t.Fatalf("stop reason = %#v", stopReason)
	}
}

func TestCustomToolResultKindSurvivesMessagePersistence(t *testing.T) {
	original := types.Message{Role: types.RoleUser, Content: []types.ContentBlock{types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "call_patch", Content: "applied",
		ToolType: types.ToolDefinitionTypeCustom,
	}}}
	encoded, err := original.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded types.Message
	if err := decoded.UnmarshalJSON(encoded); err != nil {
		t.Fatal(err)
	}
	result := decoded.Content[0].(types.ToolResultBlock)
	if result.ToolType != types.ToolDefinitionTypeCustom {
		t.Fatalf("persisted tool type = %q", result.ToolType)
	}
}

func stopReasonPointer(reason types.StopReason) *types.StopReason { return &reason }

var (
	_ types.CustomToolDefinitionProvider = (*customStreamTool)(nil)
	_ types.CustomToolInputDecoder       = (*customStreamTool)(nil)
)
