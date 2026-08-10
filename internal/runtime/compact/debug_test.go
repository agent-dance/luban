package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestStructuredSummarizerEmitsCompactionDebugExchange(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"compressed history"}`)})
	ref := provider.NewProviderRef(fake)
	debugEvents := make(chan provider.DebugEvent, 2)
	ref.SetDebugObserver(func(event provider.DebugEvent) { debugEvents <- event })

	summarize := NewLLMStructuredSummarizeFunc(ref)
	ctx := provider.WithDebugCall(context.Background(), provider.DebugCallCompaction, map[string]any{"trigger": "auto"})
	result, err := summarize(ctx, []types.Message{
		types.UserMessage("original user message"),
		types.AssistantMessage("original assistant message"),
	}, "focus on tests")
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(result, "compressed history") {
		t.Fatalf("summary = %q", result)
	}

	request := <-debugEvents
	response := <-debugEvents
	if request.Kind != provider.DebugCallCompaction || request.Request == nil {
		t.Fatalf("debug request = %#v", request)
	}
	if request.Metadata["trigger"] != "auto" {
		t.Fatalf("debug metadata = %#v", request.Metadata)
	}
	if !strings.HasPrefix(request.Request.System, CompactSystemPrompt) {
		t.Fatalf("system prompt = %q", request.Request.System)
	}
	if len(request.Request.Messages) != 3 {
		t.Fatalf("request message count = %d, want 2 conversation messages plus runtime request", len(request.Request.Messages))
	}
	if !strings.Contains(request.Request.Messages[2].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime summary request missing: %#v", request.Request.Messages[2])
	}
	if !strings.Contains(request.Request.System, "focus on tests") || !strings.Contains(request.Request.System, "Your task is to create a detailed summary") {
		t.Fatalf("compact prompt missing instructions:\n%s", request.Request.System)
	}
	if response.Response == nil || response.Response.Message == nil || !strings.Contains(response.Response.Message.GetText(), "compressed history") {
		t.Fatalf("debug response = %#v", response.Response)
	}
}
