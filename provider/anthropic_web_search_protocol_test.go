package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/agent-dance/luban/types"
)

func TestAnthropicWebSearchServerToolRequestShape(t *testing.T) {
	ordinary, err := convertToAnthropicTools([]types.ToolDefinition{{
		Name:        "Read",
		Description: "read",
		InputSchema: types.StrictObjectSchema(map[string]any{}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	server, err := convertToAnthropicServerTools([]types.ServerToolDefinition{{
		Type:           "web_search_20250305",
		Name:           "web_search",
		AllowedDomains: []string{"go.dev"},
		MaxUses:        8,
	}})
	if err != nil {
		t.Fatal(err)
	}
	all := append(ordinary, server...)
	if len(all) != 2 || all[0].OfTool == nil || all[0].OfWebSearchTool20250305 != nil {
		t.Fatalf("ordinary tool was reclassified as a server tool: %+v", all[0])
	}
	if all[1].OfWebSearchTool20250305 == nil {
		t.Fatalf("web search server tool missing: %+v", all[1])
	}
	raw, err := json.Marshal(all[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"web_search_20250305"`, `"name":"web_search"`, `"allowed_domains":["go.dev"]`, `"max_uses":8`} {
		if !jsonContains(string(raw), want) {
			t.Fatalf("request tool %s missing %s", raw, want)
		}
	}
}

func TestAnthropicWebSearchRequestHeaderAndLiveStream(t *testing.T) {
	var header string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("anthropic-beta")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode request: %v\n%s", err, raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_web\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":4,\"output_tokens\":0,\"server_tool_use\":{\"web_search_requests\":0,\"web_fetch_requests\":0}}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"server_tool_use\",\"id\":\"srv_1\",\"name\":\"web_search\",\"input\":{}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\\\"golang\\\"}\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		_, _ = io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"web_search_tool_result\",\"tool_use_id\":\"srv_1\",\"content\":[{\"type\":\"web_search_result\",\"title\":\"Go\",\"url\":\"https://go.dev\"}]}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":4,\"output_tokens\":2,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0,\"server_tool_use\":{\"web_search_requests\":1,\"web_fetch_requests\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	p := NewAnthropic(Config{AuthToken: "test", BaseURL: server.URL, Model: "claude-sonnet-4-6"})
	stream, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("search")},
		ExtraToolSchemas: []types.ServerToolDefinition{{
			Type: "web_search_20250305", Name: "web_search", AllowedDomains: []string{"go.dev"}, MaxUses: 8,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawServer, sawResult, sawUsage bool
	for event := range stream {
		if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeServerToolUse && len(event.ContentBlock.RawJSON) > 0 {
			sawServer = true
		}
		if event.ContentBlock != nil && event.ContentBlock.Type == types.ContentTypeWebSearchToolResult && event.ContentBlock.ToolUseID == "srv_1" {
			sawResult = true
		}
		if event.Usage != nil && event.Usage.ServerToolUse.WebSearchRequests == 1 {
			sawUsage = true
		}
	}
	if header != anthropicWebSearchBetaHeader {
		t.Fatalf("anthropic-beta = %q, want %q", header, anthropicWebSearchBetaHeader)
	}
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", body["tools"])
	}
	encoded, _ := json.Marshal(tools[0])
	for _, want := range []string{"web_search_20250305", "go.dev", `"max_uses":8`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("server schema %s missing %q", encoded, want)
		}
	}
	if !sawServer || !sawResult || !sawUsage {
		t.Fatalf("stream preservation server/result/usage = %v/%v/%v", sawServer, sawResult, sawUsage)
	}
}

func TestAnthropicStreamServerToolBlocksAndUsage(t *testing.T) {
	var serverStart anthropic.ContentBlockStartEventContentBlockUnion
	if err := json.Unmarshal([]byte(`{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{}}`), &serverStart); err != nil {
		t.Fatal(err)
	}
	got := anthropicStreamContentBlock(serverStart)
	if got.Type != types.ContentTypeServerToolUse || got.ID != "srv_1" || got.Name != "web_search" || len(got.RawJSON) == 0 {
		t.Fatalf("server tool start = %+v", got)
	}

	var resultStart anthropic.ContentBlockStartEventContentBlockUnion
	if err := json.Unmarshal([]byte(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[{"type":"web_search_result","title":"Go","url":"https://go.dev"}]}`), &resultStart); err != nil {
		t.Fatal(err)
	}
	got = anthropicStreamContentBlock(resultStart)
	if got.Type != types.ContentTypeWebSearchToolResult || got.ToolUseID != "srv_1" || len(got.RawJSON) == 0 {
		t.Fatalf("web search result start = %+v", got)
	}

	usage := anthropicUsageToTypes(anthropic.Usage{ServerToolUse: anthropic.ServerToolUsage{WebSearchRequests: 2}})
	if usage.ServerToolUse.WebSearchRequests != 2 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestAnthropicNonStreamingWebSearchBlocksAreNotDropped(t *testing.T) {
	fixtures := []struct {
		raw      string
		wantType types.ContentType
	}{
		{`{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"go"}}`, types.ContentTypeServerToolUse},
		{`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[]}`, types.ContentTypeWebSearchToolResult},
	}
	for _, fixture := range fixtures {
		var block anthropic.ContentBlockUnion
		if err := json.Unmarshal([]byte(fixture.raw), &block); err != nil {
			t.Fatal(err)
		}
		start, _, ok := anthropicBlockToEvents(block)
		if !ok || start.Type != fixture.wantType || len(start.RawJSON) == 0 {
			t.Fatalf("block %s => %+v ok=%v", fixture.raw, start, ok)
		}
	}
}

func TestAnthropicWebSearchServerBlocksRoundTripInAssistantMessage(t *testing.T) {
	messages := convertToAnthropicMessages([]types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.UnknownBlock{Type: types.ContentTypeServerToolUse, Raw: json.RawMessage(`{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"go"}}`)},
			types.UnknownBlock{Type: types.ContentTypeWebSearchToolResult, Raw: json.RawMessage(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[]}`)},
		},
	}})
	if len(messages) != 1 || len(messages[0].Content) != 2 {
		t.Fatalf("round-tripped messages = %+v", messages)
	}
	raw, err := json.Marshal(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"server_tool_use", "web_search_tool_result", "srv_1"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("assistant message %s missing %q", raw, want)
		}
	}
}

func jsonContains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
