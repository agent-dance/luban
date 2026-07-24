package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

func TestAnthropicSkillCatalogDeveloperMessageOrder(t *testing.T) {
	payload := `{"kind":"snapshot","summary":"escaped \\u003c/system-reminder\\u003e"}`
	developer := trustedDeveloperMessageForTest(payload, types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 7,
	})

	messages := convertToAnthropicMessagesForParams(Params{Messages: []types.Message{
		developer,
		types.UserMessage("real user input"),
	}}.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope, false))
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want one merged user turn", len(messages))
	}

	merged := marshalAnthropicMessageTask09(t, messages[0])
	if merged["role"] != "user" {
		t.Fatalf("role = %#v, want user", merged["role"])
	}

	wantReminder := "<system-reminder>\n" + payload + "\n</system-reminder>"
	content := merged["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("merged content blocks = %d, want reminder plus user text", len(content))
	}
	if got := content[0].(map[string]any)["text"]; got != wantReminder {
		t.Fatalf("reminder text = %#v, want %q", got, wantReminder)
	}
	if got := content[1].(map[string]any)["text"]; got != "real user input" {
		t.Fatalf("user text = %#v, want real user input", got)
	}
	if strings.Count(wantReminder, "<system-reminder>") != 1 || strings.Count(wantReminder, "</system-reminder>") != 1 {
		t.Fatalf("escaped payload broke reminder framing: %q", wantReminder)
	}

	reminderContent := content[0].(map[string]any)
	userContent := content[1].(map[string]any)
	if _, ok := reminderContent["cache_control"]; ok {
		t.Fatalf("catalog reminder unexpectedly moved the cache breakpoint: %#v", reminderContent)
	}
	if _, ok := userContent["cache_control"]; !ok {
		t.Fatalf("current user tail missing cache breakpoint: %#v", userContent)
	}
}

func TestAnthropicDeveloperMessageDeltaPreservesHistoryOrder(t *testing.T) {
	delta := trustedDeveloperMessageForTest("catalog delta", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogDelta,
		Revision: 9,
	})
	messages := convertToAnthropicMessagesForParams(Params{Messages: []types.Message{
		types.UserMessage("old user"),
		types.AssistantMessage("old assistant"),
		delta,
		types.UserMessage("current user"),
	}}.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope, false))
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want delta merged into current user turn", len(messages))
	}

	want := []string{"old user", "old assistant"}
	for i := range want {
		got := anthropicMessageTextTask09(t, marshalAnthropicMessageTask09(t, messages[i]))
		if got != want[i] {
			t.Fatalf("message %d text = %q, want %q", i, got, want[i])
		}
	}
	tail := marshalAnthropicMessageTask09(t, messages[2])
	content := tail["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("current user content blocks = %d, want reminder plus user text", len(content))
	}
	if got := content[0].(map[string]any)["text"]; got != "<system-reminder>\ncatalog delta\n</system-reminder>" {
		t.Fatalf("delta reminder = %#v", got)
	}
	if got := content[1].(map[string]any)["text"]; got != "current user" {
		t.Fatalf("current user text = %#v", got)
	}
}

// convertToAnthropicMessages is the shared projection used by the direct
// Anthropic, Bedrock, and Vertex providers.
func TestAnthropicCompatibleDeveloperMessagePreservesToolResultPairing(t *testing.T) {
	delta := trustedDeveloperMessageForTest("catalog delta after tools", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogDelta,
		Revision: 11,
	})
	messages := convertToAnthropicMessagesForParams(Params{Messages: []types.Message{
		types.UserMessage("run the tool"),
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool_1", Name: "Read", Input: map[string]any{}},
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool_2", Name: "Read", Input: map[string]any{}},
			},
		},
		delta,
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "tool_1", Content: "done one"},
			types.ToolResultBlock{ToolUseID: "tool_2", Content: "done two"},
		),
	}}.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope, false))
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3 so no reminder splits tool_use/tool_result", len(messages))
	}

	assistant := marshalAnthropicMessageTask09(t, messages[1])
	result := marshalAnthropicMessageTask09(t, messages[2])
	if assistant["role"] != "assistant" || result["role"] != "user" {
		t.Fatalf("tool pair roles = %#v/%#v, want assistant/user", assistant["role"], result["role"])
	}
	content := result["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("tool result content blocks = %d, want two results then reminder", len(content))
	}
	first := content[0].(map[string]any)
	second := content[1].(map[string]any)
	if first["type"] != "tool_result" || first["tool_use_id"] != "tool_1" {
		t.Fatalf("first content block = %#v, want tool_result for tool_1", first)
	}
	if second["type"] != "tool_result" || second["tool_use_id"] != "tool_2" {
		t.Fatalf("second content block = %#v, want tool_result for tool_2", second)
	}
	third := content[2].(map[string]any)
	if third["type"] != "text" || third["text"] != "<system-reminder>\ncatalog delta after tools\n</system-reminder>" {
		t.Fatalf("third content block = %#v, want trailing catalog reminder", third)
	}
}

func TestAnthropicSkillCatalogRequestKeepsStablePrefixAndHidesLocalMetadata(t *testing.T) {
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	provider := NewAnthropic(Config{AuthToken: "test", BaseURL: server.URL, Model: "claude-sonnet-4-6"})
	base := Params{
		SystemBlocks: []prompt.SystemPromptBlock{{Text: "stable system", Cache: true}},
		Tools: []types.ToolDefinition{{
			Name:        "Skill",
			Description: "stable tool",
			InputSchema: types.JSONSchema{Type: "object"},
		}},
		Messages: []types.Message{types.UserMessage("current user")},
	}
	run := func(params Params) {
		t.Helper()
		stream, err := provider.CreateStream(context.Background(), params)
		if err != nil {
			t.Fatalf("CreateStream: %v", err)
		}
		for range stream {
		}
	}

	run(base)
	withCatalog := base
	developer := types.DeveloperMessage("public catalog payload", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKind("private-local-kind"),
		Revision: 987654321,
	})
	developer.ID = "private-local-message-id"
	withCatalog.Messages = []types.Message{developer, types.UserMessage("current user")}
	run(withCatalog)

	baselineBody := <-requests
	catalogBody := <-requests
	var baseline, catalog map[string]any
	if err := json.Unmarshal(baselineBody, &baseline); err != nil {
		t.Fatalf("unmarshal baseline request: %v\n%s", err, baselineBody)
	}
	if err := json.Unmarshal(catalogBody, &catalog); err != nil {
		t.Fatalf("unmarshal catalog request: %v\n%s", err, catalogBody)
	}
	if !reflect.DeepEqual(baseline["system"], catalog["system"]) {
		t.Fatalf("catalog changed system blocks\nbaseline: %#v\ncatalog: %#v", baseline["system"], catalog["system"])
	}
	if !reflect.DeepEqual(baseline["tools"], catalog["tools"]) {
		t.Fatalf("catalog changed tool definitions\nbaseline: %#v\ncatalog: %#v", baseline["tools"], catalog["tools"])
	}
	for _, leaked := range []string{"developer_metadata", "private-local-kind", "987654321", "private-local-message-id", "is_meta"} {
		if strings.Contains(string(catalogBody), leaked) {
			t.Fatalf("local message metadata %q leaked into provider request: %s", leaked, catalogBody)
		}
	}
}

func marshalAnthropicMessageTask09(t *testing.T, message any) map[string]any {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal Anthropic message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal Anthropic message: %v", err)
	}
	return decoded
}

func anthropicMessageTextTask09(t *testing.T, message map[string]any) string {
	t.Helper()
	content, ok := message["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("message content = %#v, want one text block", message["content"])
	}
	block, ok := content[0].(map[string]any)
	if !ok || block["type"] != "text" {
		t.Fatalf("content block = %#v, want text", content[0])
	}
	text, ok := block["text"].(string)
	if !ok {
		t.Fatalf("text = %#v, want string", block["text"])
	}
	return text
}
