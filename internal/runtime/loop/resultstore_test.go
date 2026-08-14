package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type resultStoreRecordingProvider struct {
	mu       sync.Mutex
	requests []provider.Params
}

func (p *resultStoreRecordingProvider) Name() string    { return "recording" }
func (p *resultStoreRecordingProvider) ModelID() string { return "recording-model" }

func (p *resultStoreRecordingProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.requests = append(p.requests, params)
	idx := len(p.requests)
	p.mu.Unlock()

	events := []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "done"}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
	if idx == 1 {
		events = []types.StreamEvent{
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse,
				ID:   "toolu_large",
				Name: "LargeTool",
			}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
				Type:        "input_json_delta",
				PartialJSON: `{}`,
			}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageStop},
		}
	}
	events = attachTestProviderCommitReceipts(events)

	ch := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type largeResultTool struct{}

func (t *largeResultTool) Name() string        { return "LargeTool" }
func (t *largeResultTool) Description() string { return "large result tool" }
func (t *largeResultTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *largeResultTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content:  strings.Repeat("large output\n", 20),
		Metadata: map[string]string{"maxResultSizeChars": "10"},
	}, nil
}

func TestQueryLoopStoresLargeResultsBeforeAddingHistoryForNextRequest(t *testing.T) {
	p := &resultStoreRecordingProvider{}
	reg := registry.New()
	reg.Register(&largeResultTool{})
	ql := New(p, reg, Config{MaxTurns: 2})
	ql.SetResultStore(compact.NewResultStore(t.TempDir()))

	if err := ql.Run(context.Background(), "run large tool", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	p.mu.Lock()
	requests := append([]provider.Params(nil), p.requests...)
	p.mu.Unlock()
	if len(requests) < 2 {
		t.Fatalf("expected at least 2 provider requests, got %d", len(requests))
	}

	var sawPersistedToolResult bool
	for _, msg := range requests[1].Messages {
		for _, block := range msg.Content {
			tr, ok := block.(types.ToolResultBlock)
			if !ok || tr.ToolUseID != "toolu_large" {
				continue
			}
			sawPersistedToolResult = true
			if !strings.Contains(tr.Content, "<persisted-output>") {
				t.Fatalf("tool result was not persisted before next request history: %q", tr.Content)
			}
		}
	}
	if !sawPersistedToolResult {
		t.Fatalf("second request did not include tool result history: %#v", requests[1].Messages)
	}
}

func TestQueryLoopResultStoreFailureCarriesToolIdentity(t *testing.T) {
	p := &resultStoreRecordingProvider{}
	reg := registry.New()
	reg.Register(&largeResultTool{})
	ql := New(p, reg, Config{MaxTurns: 2, SessionID: "session-result-store"})

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("seed invalid result-store parent: %v", err)
	}
	ql.SetResultStore(compact.NewResultStore(filepath.Join(parentFile, "tool-results")))

	var persistenceError *stream.Event
	if err := ql.Run(context.Background(), "run large tool", func(event stream.Event) {
		if event.Type == stream.EventError && strings.Contains(event.Text, "persist tool result") {
			copy := event
			persistenceError = &copy
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if persistenceError == nil {
		t.Fatal("missing structured result-store failure event")
	}
	if persistenceError.ToolUseID != "toolu_large" {
		t.Fatalf("persistence error tool_use_id = %q, want toolu_large", persistenceError.ToolUseID)
	}
	if persistenceError.Error == nil || persistenceError.Error.Type != "tool_result_persistence_error" {
		t.Fatalf("persistence error payload = %#v", persistenceError.Error)
	}
	if persistenceError.Metadata["stage"] != "tool_result_persistence" || persistenceError.Metadata["outcome"] != string(types.ToolOutcomeFailed) {
		t.Fatalf("persistence error metadata = %#v", persistenceError.Metadata)
	}
}
