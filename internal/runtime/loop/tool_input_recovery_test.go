package loop

import (
	"context"
	"sync"
	"testing"

	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type toolInputRecoveryProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent
	params    []provider.Params
}

type toolInputRecoveryTool struct{}

func (*toolInputRecoveryTool) Name() string        { return "Inspect" }
func (*toolInputRecoveryTool) Description() string { return "test inspect" }
func (*toolInputRecoveryTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (*toolInputRecoveryTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "inspected"}, nil
}

func (p *toolInputRecoveryProvider) Name() string    { return "recovery-test" }
func (p *toolInputRecoveryProvider) ModelID() string { return "recovery-test-model" }
func (p *toolInputRecoveryProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.params)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.params = append(p.params, params)
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, 16)
	go func() {
		defer close(stream)
		if index >= len(p.responses) {
			return
		}
		for _, event := range p.responses[index] {
			select {
			case stream <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return stream, nil
}

func malformedToolResponse(responseID string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call_bad", Name: "Inspect", ProviderItemID: "fc_bad",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"path":`}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop, ResponseID: responseID, ProviderContinuation: &types.ProviderContinuation{
			Protocol: "responses/test/standard", RequestedModel: "recovery-test-model",
		}},
	}
}

func finalTextResponse(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop, ResponseID: "response-corrected"},
	}
}

func correctedToolResponse() []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call_corrected", Name: "Inspect", ProviderItemID: "fc_corrected",
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "tool_state_final", ID: "call_corrected", Name: "Inspect", PartialJSON: `{"path":"."}`,
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop, ResponseID: "response-valid-tool"},
	}
}

func TestMalformedToolInputRetriesFromSanitizedFullHistory(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid-parent"),
		correctedToolResponse(),
		finalTextResponse("corrected final"),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	var retryWarnings int
	err := query.Run(context.Background(), "inspect", func(event streamevent.Event) {
		if event.Type == streamevent.EventSystemWarning && event.RuntimeEvent != nil && event.RuntimeEvent.PrivateMetadata["reason"] == "invalid_tool_input" {
			retryWarnings++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.params) != 3 {
		t.Fatalf("provider calls = %d, want correction, tool result, and final", len(p.params))
	}
	if p.params[1].PreviousResponseID != "" {
		t.Fatalf("invalid response reused as parent: %q", p.params[1].PreviousResponseID)
	}
	foundRecovery := false
	for _, message := range p.params[1].Messages {
		if len(message.GetInvalidToolUses()) != 0 {
			t.Fatalf("invalid tool audit leaked into model view: %#v", p.params[1].Messages)
		}
		if message.InternalKind == types.InternalMessageKindToolInputRecovery && message.IsInternalRuntimeMessage() {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("trusted correction message missing: %#v", p.params[1].Messages)
	}
	if retryWarnings != 1 {
		t.Fatalf("structured recovery warnings = %d, want 1", retryWarnings)
	}
	invalidCount := 0
	for _, message := range query.Messages() {
		invalidCount += len(message.GetInvalidToolUses())
		if len(message.GetInvalidToolUses()) > 0 && message.HasProviderContinuation() {
			t.Fatal("malformed provider continuation remained attached to durable history")
		}
	}
	if invalidCount != 1 {
		t.Fatalf("durable invalid tool audits = %d, want 1", invalidCount)
	}
}

func TestMalformedToolInputRejectsTextOnlyCorrection(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid"),
		finalTextResponse("claimed completion without a tool"),
	}}
	query := New(p, registry.New(), Config{MaxTurns: 4, MaxTokens: 256})
	err := query.Run(context.Background(), "inspect", func(streamevent.Event) {})
	if err == nil {
		t.Fatal("text-only correction was accepted as successful completion")
	}
	if len(p.params) != 2 {
		t.Fatalf("provider calls = %d, want one bounded correction", len(p.params))
	}
}

func TestMalformedToolInputFailsClosedAfterOneCorrection(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid-1"),
		malformedToolResponse("response-invalid-2"),
	}}
	query := New(p, registry.New(), Config{MaxTurns: 4, MaxTokens: 256})
	err := query.Run(context.Background(), "inspect", func(streamevent.Event) {})
	if err == nil {
		t.Fatal("repeated malformed tool input completed successfully")
	}
	if len(p.params) != 2 {
		t.Fatalf("provider calls = %d, want bounded correction", len(p.params))
	}
	invalidCount := 0
	for _, message := range query.Messages() {
		invalidCount += len(message.GetInvalidToolUses())
	}
	if invalidCount != 2 {
		t.Fatalf("durable invalid tool audits = %d, want 2", invalidCount)
	}
}
