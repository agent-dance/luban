package types

import (
	"encoding/json"
	"testing"
)

func TestUsageServerToolUseWebSearchRequestsRoundTrip(t *testing.T) {
	want := Usage{
		InputTokens: 12,
		ServerToolUse: ServerToolUsage{
			WebSearchRequests: 3,
			WebFetchRequests:  1,
		},
	}

	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Usage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ServerToolUse.WebSearchRequests != 3 || got.ServerToolUse.WebFetchRequests != 1 {
		t.Fatalf("server tool usage = %+v, want web_search=3 web_fetch=1", got.ServerToolUse)
	}
}

func TestServerToolDefinitionKeepsAnthropicWebSearchSeparateFromClientTools(t *testing.T) {
	schema := ServerToolDefinition{
		Type:           "web_search_20250305",
		Name:           "web_search",
		AllowedDomains: []string{"go.dev"},
		MaxUses:        8,
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"web_search_20250305"`, `"name":"web_search"`, `"allowed_domains":["go.dev"]`, `"max_uses":8`} {
		if !json.Valid(raw) || !containsJSONFragment(string(raw), want) {
			t.Fatalf("server schema %s does not contain %s", raw, want)
		}
	}
	client := ToolDefinition{Name: "web_search", InputSchema: StrictObjectSchema(map[string]any{})}
	if client.Name == schema.Name && client.InputSchema.Type == "" {
		t.Fatal("ordinary client tool unexpectedly shares server-tool representation")
	}
}

func TestContentDeltaPreservesServerToolRawJSON(t *testing.T) {
	raw := json.RawMessage(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[]}`)
	delta := ContentDelta{
		Type:      ContentTypeWebSearchToolResult,
		ToolUseID: "srv_1",
		RawJSON:   raw,
	}
	encoded, err := json.Marshal(delta)
	if err != nil {
		t.Fatal(err)
	}
	var got ContentDelta
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != ContentTypeWebSearchToolResult || got.ToolUseID != "srv_1" || string(got.RawJSON) != string(raw) {
		t.Fatalf("round trip = %+v raw=%s", got, got.RawJSON)
	}
}

func containsJSONFragment(value, fragment string) bool {
	return len(fragment) == 0 || len(value) >= len(fragment) && jsonFragmentIndex(value, fragment) >= 0
}

func jsonFragmentIndex(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}
