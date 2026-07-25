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
	"github.com/agent-dance/luban/types"
)

func TestOpenAIChatDeveloperSkillCatalogSnapshotSerialized(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "openai",
		Model:        "gpt-4o",
	}, Params{
		Model:  "o3",
		System: "stable system",
		Messages: []types.Message{
			trustedDeveloperMessageForTest("catalog snapshot", types.DeveloperMessageMetadata{
				Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
				Revision: 7,
			}),
			types.UserMessage("current user"),
		},
	})

	if got := request["model"]; got != "o3" {
		t.Fatalf("model = %#v, want per-request override o3", got)
	}
	messages := openAIChatRequestMessagesTask10(t, request)
	assertOpenAIChatRolesTask10(t, messages, "system", "developer", "user")
	if got := messages[0]["content"]; got != "stable system" {
		t.Fatalf("stable system content = %#v", got)
	}
	if got := messages[1]["content"]; got != "catalog snapshot" {
		t.Fatalf("catalog content = %#v", got)
	}

	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"developer_metadata", "skill_catalog_snapshot", `"revision"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("provider request leaked local metadata %q: %s", forbidden, raw)
		}
	}
}

func TestOpenAIChatDialectDeveloperLowering(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "OpenAI chat model",
			config: Config{
				ProviderName: "openai",
				Model:        "gpt-4o",
			},
		},
		{
			name: "OpenAI-compatible provider even with an o-series model name",
			config: Config{
				ProviderName: "deepseek",
				Model:        "o3",
			},
		},
		{
			name: "custom endpoint configured under OpenAI",
			config: Config{
				ProviderName: "openai",
				BaseURL:      "https://gateway.example/v1",
				Model:        "o3",
			},
		},
		{
			name:   "Gemini compatibility API",
			config: Config{ProviderName: "gemini", Model: "gemini-3.5-flash"},
		},
		{
			name:   "Mistral compatibility API",
			config: Config{ProviderName: "mistral", Model: "mistral-large"},
		},
		{
			name:   "Groq compatibility API",
			config: Config{ProviderName: "groq", Model: "llama-3.3-70b-versatile"},
		},
		{
			name:   "Ollama compatibility API",
			config: Config{ProviderName: "ollama", Model: "llama3.1"},
		},
		{
			name:   "unknown compatibility API",
			config: Config{ProviderName: "custom", Model: "custom-model"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := captureOpenAIChatRequestTask10(t, test.config, Params{
				System: "stable system",
				Messages: []types.Message{
					trustedDeveloperMessageForTest("catalog delta", types.DeveloperMessageMetadata{
						Kind:     types.DeveloperMessageKindSkillCatalogDelta,
						Revision: 9,
					}),
					types.UserMessage("current user"),
				},
			})

			messages := openAIChatRequestMessagesTask10(t, request)
			assertOpenAIChatRolesTask10(t, messages, "system", "user")
			if got := messages[0]["content"]; got != "stable system" {
				t.Fatalf("stable system was rewritten: %#v", got)
			}
			want := "<system-reminder>\ncatalog delta\n</system-reminder>\n\ncurrent user"
			if got := messages[1]["content"]; got != want {
				t.Fatalf("fallback user reminder = %#v, want %q", got, want)
			}
		})
	}
}

func TestDeepSeekDeveloperDeltaPreservesSerializedPrefix(t *testing.T) {
	tool := types.ToolDefinition{
		Name:        "Skill",
		Description: "load one skill",
		InputSchema: types.StrictObjectSchema(map[string]any{
			"name": map[string]any{"type": "string"},
		}, "name"),
		Strict: true,
	}
	snapshot := trustedDeveloperMessageForTest("catalog snapshot", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 1,
	})
	baseParams := Params{
		System: "stable system",
		Tools:  []types.ToolDefinition{tool},
		Messages: []types.Message{
			snapshot,
			types.UserMessage("first user"),
			types.AssistantMessage("first assistant"),
		},
	}
	config := Config{ProviderName: "deepseek", Model: "deepseek-v4-flash"}
	first := captureOpenAIChatRequestTask10(t, config, baseParams)

	deltaParams := baseParams
	deltaParams.Messages = append(append([]types.Message(nil), baseParams.Messages...),
		trustedDeveloperMessageForTest("catalog delta", types.DeveloperMessageMetadata{
			Kind:     types.DeveloperMessageKindSkillCatalogDelta,
			Revision: 2,
		}),
		types.UserMessage("current user"),
	)
	second := captureOpenAIChatRequestTask10(t, config, deltaParams)

	firstMessages := openAIChatRequestMessagesTask10(t, first)
	secondMessages := openAIChatRequestMessagesTask10(t, second)
	assertOpenAIChatRolesTask10(t, firstMessages, "system", "user", "assistant")
	assertOpenAIChatRolesTask10(t, secondMessages, "system", "user", "assistant", "user")
	if !reflect.DeepEqual(firstMessages, secondMessages[:len(firstMessages)]) {
		t.Fatalf("DeepSeek catalog delta rewrote serialized prefix\nfirst:  %#v\nsecond: %#v", firstMessages, secondMessages)
	}
	if !reflect.DeepEqual(first["tools"], second["tools"]) {
		t.Fatalf("DeepSeek catalog delta changed tools\nfirst:  %#v\nsecond: %#v", first["tools"], second["tools"])
	}
	for i, message := range secondMessages {
		if i > 0 && message["role"] == "system" {
			t.Fatalf("late system message remained at index %d: %#v", i, secondMessages)
		}
		if message["role"] == "developer" {
			t.Fatalf("DeepSeek request contains unsupported developer role: %#v", secondMessages)
		}
	}
	wantTail := "<system-reminder>\ncatalog delta\n</system-reminder>\n\ncurrent user"
	if got := secondMessages[len(secondMessages)-1]["content"]; got != wantTail {
		t.Fatalf("DeepSeek delta tail = %#v, want %q", got, wantTail)
	}
}

func TestDeepSeekDeveloperReminderPreservesToolResultAdjacency(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "deepseek",
		Model:        "deepseek-v4-flash",
	}, Params{
		System: "stable system",
		Messages: []types.Message{
			types.UserMessage("run tool"),
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{types.ToolUseBlock{
					Type: types.ContentTypeToolUse,
					ID:   "tool_1",
					Name: "Skill",
				}},
			},
			trustedDeveloperMessageForTest("catalog delta", types.DeveloperMessageMetadata{
				Kind:     types.DeveloperMessageKindSkillCatalogDelta,
				Revision: 2,
			}),
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "tool_1", Content: "loaded"}),
		},
	})

	messages := openAIChatRequestMessagesTask10(t, request)
	assertOpenAIChatRolesTask10(t, messages, "system", "user", "assistant", "tool", "user")
	if got := messages[3]["tool_call_id"]; got != "tool_1" {
		t.Fatalf("tool result call ID = %#v, want tool_1", got)
	}
	if got := messages[4]["content"]; got != "<system-reminder>\ncatalog delta\n</system-reminder>" {
		t.Fatalf("tool-result reminder = %#v", got)
	}
}

func TestOpenAIChatDeveloperDeltaPreservesSerializedPrefixAndTools(t *testing.T) {
	tool := types.ToolDefinition{
		Name:        "Skill",
		Description: "load one skill",
		InputSchema: types.StrictObjectSchema(map[string]any{
			"name": map[string]any{"type": "string"},
		}, "name"),
		Strict: true,
	}
	snapshot := trustedDeveloperMessageForTest("catalog snapshot", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 1,
	})
	toolUse := types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "tool_1",
				Name:  "Skill",
				Input: map[string]any{"name": "alpha"},
			},
		},
	}
	toolResult := types.ToolResultMessage(types.ToolResultBlock{
		ToolUseID: "tool_1",
		Content:   "loaded",
	})
	baseParams := Params{
		System:         "stable system",
		PromptCacheKey: "session-stable",
		UsePromptCache: true,
		Tools:          []types.ToolDefinition{tool},
		ToolChoice:     &ToolChoice{Type: "auto"},
		Messages: []types.Message{
			snapshot,
			types.UserMessage("load alpha"),
			toolUse,
			toolResult,
		},
	}
	first := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "openai",
		Model:        "o3",
	}, baseParams)

	deltaParams := baseParams
	deltaParams.Messages = append(append([]types.Message(nil), baseParams.Messages...),
		trustedDeveloperMessageForTest("catalog delta", types.DeveloperMessageMetadata{
			Kind:     types.DeveloperMessageKindSkillCatalogDelta,
			Revision: 2,
		}),
		types.UserMessage("current user"),
	)
	second := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "openai",
		Model:        "o3",
	}, deltaParams)

	firstMessages := openAIChatRequestMessagesTask10(t, first)
	secondMessages := openAIChatRequestMessagesTask10(t, second)
	assertOpenAIChatRolesTask10(t, firstMessages, "system", "developer", "user", "assistant", "tool")
	assertOpenAIChatRolesTask10(t, secondMessages, "system", "developer", "user", "assistant", "tool", "developer", "user")
	if !reflect.DeepEqual(firstMessages, secondMessages[:len(firstMessages)]) {
		t.Fatalf("catalog delta rewrote serialized prefix\nfirst:  %#v\nsecond: %#v", firstMessages, secondMessages)
	}
	if !reflect.DeepEqual(first["tools"], second["tools"]) {
		t.Fatalf("catalog delta changed tools\nfirst:  %#v\nsecond: %#v", first["tools"], second["tools"])
	}
	var cacheKey any
	for _, request := range []map[string]any{first, second} {
		if request["model"] != "o3" || request["tool_choice"] != "auto" {
			t.Fatalf("stable request fields changed: %#v", request)
		}
		got := request["prompt_cache_key"]
		if got == nil || got == "session-stable" {
			t.Fatalf("Chat Completions prompt_cache_key = %#v, want opaque credential-scoped route", got)
		}
		if cacheKey != nil && got != cacheKey {
			t.Fatalf("Chat Completions cache route changed: %#v != %#v", got, cacheKey)
		}
		cacheKey = got
	}
}

func TestOpenAIChatDeveloperOrdinaryTurnUnchanged(t *testing.T) {
	request := captureOpenAIChatRequestTask10(t, Config{
		ProviderName: "openai",
		Model:        "o3",
	}, Params{
		System: "stable system",
		Messages: []types.Message{
			types.UserMessage("hello"),
			types.AssistantMessage("hi"),
			types.UserMessage("continue"),
		},
	})

	messages := openAIChatRequestMessagesTask10(t, request)
	assertOpenAIChatRolesTask10(t, messages, "system", "user", "assistant", "user")
}

func captureOpenAIChatRequestTask10(t *testing.T, config Config, params Params) map[string]any {
	t.Helper()
	params = params.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope)
	simulateOfficialOpenAI := CanonicalProviderName(config.ProviderName) == "openai" && !isCustomOpenAIBaseURL(config.BaseURL)
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decode request: %v\n%s", err, body)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_task10\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	config.APIKey = "test-key"
	config.BaseURL = server.URL
	provider := NewOpenAI(config)
	provider.officialOpenAIChatEndpoint = simulateOfficialOpenAI
	stream, err := provider.CreateStream(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if captured == nil {
		t.Fatal("request was not captured")
	}
	return captured
}

func openAIChatRequestMessagesTask10(t *testing.T, request map[string]any) []map[string]any {
	t.Helper()
	rawMessages, ok := request["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", request["messages"])
	}
	messages := make([]map[string]any, len(rawMessages))
	for i, raw := range rawMessages {
		message, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("message[%d] = %#v, want object", i, raw)
		}
		messages[i] = message
	}
	return messages
}

func assertOpenAIChatRolesTask10(t *testing.T, messages []map[string]any, roles ...string) {
	t.Helper()
	if len(messages) != len(roles) {
		t.Fatalf("message count = %d, want %d: %#v", len(messages), len(roles), messages)
	}
	for i, role := range roles {
		if got := messages[i]["role"]; got != role {
			t.Fatalf("message[%d].role = %#v, want %q: %#v", i, got, role, messages)
		}
	}
}
