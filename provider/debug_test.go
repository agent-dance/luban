package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestOpenDebugFileAppendsAndRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("seed debug file: %v", err)
	}

	file, err := OpenDebugFile(path)
	if err != nil {
		t.Fatalf("OpenDebugFile: %v", err)
	}
	if _, err := file.WriteString("new\n"); err != nil {
		t.Fatalf("write debug file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close debug file: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debug file: %v", err)
	}
	if string(data) != "existing\nnew\n" {
		t.Fatalf("debug file content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat debug file: %v", err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("debug file permissions = %o, want 600", got)
	}
}

type debugProvider struct {
	events []types.StreamEvent
	err    error
}

func (p *debugProvider) Name() string    { return "debug-provider" }
func (p *debugProvider) ModelID() string { return "debug-model" }
func (p *debugProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	if p.err != nil {
		return nil, p.err
	}
	stream := make(chan types.StreamEvent, len(p.events))
	for _, event := range p.events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestProviderRefDebugObserverCapturesFullExchange(t *testing.T) {
	stopReason := types.StopReasonToolUse
	wantEvents := []types.StreamEvent{
		{Type: types.EventMessageStart, Usage: &types.Usage{InputTokens: 42}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "hello"}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventContentBlockStart, Index: 1, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Read"}},
		{Type: types.EventContentBlockDelta, Index: 1, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: `{"file_path":"README.md"}`}},
		{Type: types.EventContentBlockStop, Index: 1},
		{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: 7}, StopReason: &stopReason},
		{Type: types.EventMessageStop, ResponseID: "resp_123"},
	}
	ref := NewProviderRef(&debugProvider{events: wantEvents})
	debugEvents := make(chan DebugEvent, 2)
	ref.SetDebugObserver(func(event DebugEvent) { debugEvents <- event })

	ctx := WithDebugCall(context.Background(), DebugCallConversation, map[string]any{"turn": 3})
	stream, err := ref.CreateStream(ctx, Params{
		Model:     "requested-model",
		MaxTokens: 2048,
		System:    "full system prompt",
		Messages:  []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	var gotEvents []types.StreamEvent
	for event := range stream {
		gotEvents = append(gotEvents, event)
	}
	if len(gotEvents) != len(wantEvents) {
		t.Fatalf("forwarded events = %d, want %d", len(gotEvents), len(wantEvents))
	}

	request := <-debugEvents
	response := <-debugEvents
	if request.Phase != DebugPhaseRequest || request.Request == nil {
		t.Fatalf("first debug event = %#v, want request", request)
	}
	if request.Provider != "debug-provider" || request.Model != "requested-model" {
		t.Fatalf("request identity = %s/%s", request.Provider, request.Model)
	}
	if request.Request.System != "full system prompt" {
		t.Fatalf("request system = %q", request.Request.System)
	}
	if response.Phase != DebugPhaseResponse || response.Response == nil {
		t.Fatalf("second debug event = %#v, want response", response)
	}
	if response.ID != request.ID {
		t.Fatalf("response ID = %d, request ID = %d", response.ID, request.ID)
	}
	if response.Response.Message == nil || response.Response.Message.GetText() != "hello" {
		t.Fatalf("response message = %#v", response.Response.Message)
	}
	toolUses := response.Response.Message.GetToolUses()
	if len(toolUses) != 1 || toolUses[0].Name != "Read" || toolUses[0].Input["file_path"] != "README.md" {
		t.Fatalf("response tool uses = %#v", toolUses)
	}
	if response.Response.Usage == nil || response.Response.Usage.InputTokens != 42 || response.Response.Usage.OutputTokens != 7 {
		t.Fatalf("response usage = %#v", response.Response.Usage)
	}
	if response.Response.StopReason == nil || *response.Response.StopReason != types.StopReasonToolUse || response.Response.ResponseID != "resp_123" {
		t.Fatalf("response terminal fields = %#v", response.Response)
	}

	formatted, err := FormatDebugEvent(request)
	if err != nil {
		t.Fatalf("FormatDebugEvent: %v", err)
	}
	for _, want := range []string{"conversation request #", `"turn": 3`, `"system": "full system prompt"`, `"messages"`} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted request missing %q:\n%s", want, formatted)
		}
	}
}

func TestProviderRefDebugContextMarksCompaction(t *testing.T) {
	ref := NewProviderRef(&debugProvider{})
	debugEvents := make(chan DebugEvent, 2)
	ref.SetDebugObserver(func(event DebugEvent) { debugEvents <- event })
	ctx := WithDebugCall(context.Background(), DebugCallCompaction, map[string]any{"trigger": "auto"})

	stream, err := ref.CreateStream(ctx, Params{System: "compact system", Messages: []types.Message{types.UserMessage("history")}})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	request := <-debugEvents
	response := <-debugEvents
	if request.Kind != DebugCallCompaction || request.Metadata["trigger"] != "auto" {
		t.Fatalf("request context = %#v", request)
	}
	if response.Kind != DebugCallCompaction || response.Metadata["trigger"] != "auto" {
		t.Fatalf("response context = %#v", response)
	}
}

func TestProviderRefDebugObserverCapturesImmediateError(t *testing.T) {
	ref := NewProviderRef(&debugProvider{err: errors.New("connection failed")})
	debugEvents := make(chan DebugEvent, 2)
	ref.SetDebugObserver(func(event DebugEvent) { debugEvents <- event })

	if _, err := ref.CreateStream(context.Background(), Params{}); err == nil {
		t.Fatal("CreateStream error = nil, want failure")
	}
	request := <-debugEvents
	response := <-debugEvents
	if response.ID != request.ID || response.Response == nil || response.Response.Error != "connection failed" {
		t.Fatalf("debug response = %#v", response)
	}
}
