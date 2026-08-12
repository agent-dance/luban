package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	loopapi "github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/prompt"
	providerapi "github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task27ProviderPlans struct {
	snapshotText string
	deltaText    string
	first        []types.Message
	noChange     []types.Message
	changed      []types.Message
}

// TestSkillCacheProviderRequestShape treats request serialization as a local
// cacheability proxy. It does not measure or claim vendor cache hits or billing.
func TestSkillCacheProviderRequestShape(t *testing.T) {
	plans := task27BuildProviderPlans(t)
	base := providerapi.Params{
		Model:                   "o3",
		System:                  "task27 stable instructions",
		SystemBlocks:            []prompt.SystemPromptBlock{{Text: "task27 stable instructions", Cache: true, CacheScope: "ephemeral"}},
		Tools:                   task27StableSkillTools(),
		PromptCacheKey:          "task27-stable-cache-key",
		UsePromptCache:          true,
		MaxTokens:               2048,
		ReasoningEffort:         "high",
		MaxOutputTokensOverride: 1024,
	}

	t.Run("OpenAI-compatible Chat exact serialized prompt prefix", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task27CaptureOpenAIChat(t, firstParams)
		unchangedParams := base
		unchangedParams.Messages = plans.noChange
		unchanged := task27CaptureOpenAIChat(t, unchangedParams)
		changedParams := base
		changedParams.Messages = plans.changed
		changed := task27CaptureOpenAIChat(t, changedParams)

		firstMessages := task27AnyItems(t, first, "messages")
		unchangedMessages := task27AnyItems(t, unchanged, "messages")
		changedMessages := task27AnyItems(t, changed, "messages")
		task27AssertSerializedPrefix(t, firstMessages, unchangedMessages)
		task27AssertSerializedPrefix(t, firstMessages, changedMessages)
		task27AssertStableFields(t, first, unchanged, "model", "tools", "max_tokens", "stream")
		task27AssertStableFields(t, first, changed, "model", "tools", "max_tokens", "stream")
		if task27CountRoleContent(unchangedMessages, "developer", plans.deltaText) != 0 {
			t.Fatal("unchanged OpenAI Chat request appended a catalog delta")
		}
		if task27CountRoleContent(changedMessages, "developer", plans.deltaText) != 1 {
			t.Fatalf("changed OpenAI Chat request did not append exactly one delta: %#v", changedMessages)
		}
	})

	t.Run("Anthropic serialized prompt prefix excluding movable cache marker", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task27CaptureAnthropic(t, firstParams)
		unchangedParams := base
		unchangedParams.Messages = plans.noChange
		unchanged := task27CaptureAnthropic(t, unchangedParams)
		changedParams := base
		changedParams.Messages = plans.changed
		changed := task27CaptureAnthropic(t, changedParams)

		firstMessages := task27AnyItems(t, first, "messages")
		unchangedMessages := task27AnyItems(t, unchanged, "messages")
		changedMessages := task27AnyItems(t, changed, "messages")
		if got := task27CountKey(firstMessages, "cache_control"); got != 1 {
			t.Fatalf("first Anthropic request cache markers = %d, want 1", got)
		}
		if got := task27CountKey(unchangedMessages, "cache_control"); got != 1 {
			t.Fatalf("unchanged Anthropic request cache markers = %d, want 1", got)
		}
		if got := task27CountKey(changedMessages, "cache_control"); got != 1 {
			t.Fatalf("changed Anthropic request cache markers = %d, want 1", got)
		}

		// Anthropic moves the transport-only cache breakpoint to the current
		// request tail. Strip only that metadata before byte-comparing prompt
		// content; system blocks and tools are compared including cache_control.
		task27AssertSerializedPrefix(t, task27WithoutCacheControl(t, firstMessages), task27WithoutCacheControl(t, unchangedMessages))
		task27AssertSerializedPrefix(t, task27WithoutCacheControl(t, firstMessages), task27WithoutCacheControl(t, changedMessages))
		task27AssertStableFields(t, first, unchanged, "model", "system", "tools", "max_tokens")
		task27AssertStableFields(t, first, changed, "model", "system", "tools", "max_tokens")
		if task27CountText(unchangedMessages, plans.deltaText) != 0 {
			t.Fatal("unchanged Anthropic request appended a catalog delta")
		}
		if task27CountText(changedMessages, plans.deltaText) != 1 {
			t.Fatalf("changed Anthropic request did not append exactly one delta: %#v", changedMessages)
		}
	})

	t.Run("Responses compatible endpoint sends full history", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task27CaptureResponses(t, firstParams)
		unchangedParams := base
		unchangedParams.Messages = plans.noChange
		unchangedParams.PreviousResponseID = "task27-previous-response"
		unchanged := task27CaptureResponses(t, unchangedParams)
		changedParams := base
		changedParams.Messages = plans.changed
		changedParams.PreviousResponseID = "task27-previous-response"
		changed := task27CaptureResponses(t, changedParams)

		task27AssertRoleContent(t, task27AnyItems(t, first, "input"), [][2]string{
			{"developer", plans.snapshotText},
			{"user", "first user"},
		})
		task27AssertRoleContent(t, task27AnyItems(t, unchanged, "input"), [][2]string{
			{"developer", plans.snapshotText},
			{"user", "first user"},
			{"assistant", "first assistant"},
			{"user", "no-change user"},
		})
		task27AssertRoleContent(t, task27AnyItems(t, changed, "input"), [][2]string{
			{"developer", plans.snapshotText},
			{"user", "first user"},
			{"assistant", "first assistant"},
			{"developer", plans.deltaText},
			{"user", "changed user"},
		})
		for _, request := range []map[string]any{unchanged, changed} {
			if _, ok := request["previous_response_id"]; ok {
				t.Fatalf("custom Responses request unexpectedly chained: %#v", request)
			}
			if got, _ := request["prompt_cache_key"].(string); !strings.HasPrefix(got, "pcu_") || got == "task27-stable-cache-key" {
				t.Fatalf("Responses prompt_cache_key = %#v, want opaque credential-scoped route", request["prompt_cache_key"])
			}
		}
		task27AssertStableFields(t, first, unchanged, "model", "instructions", "tools", "parallel_tool_calls", "prompt_cache_key", "reasoning", "max_output_tokens")
		task27AssertStableFields(t, first, changed, "model", "instructions", "tools", "parallel_tool_calls", "prompt_cache_key", "reasoning", "max_output_tokens")
	})

	t.Log("local cacheability/request-shape evidence only; not provider cache-hit or billing evidence")
}

func task27BuildProviderPlans(t *testing.T) task27ProviderPlans {
	t.Helper()
	initialSkill := skills.EffectiveSkill{
		ID:                 "skill:project:/repo/.luban-code/skills/task27/SKILL.md",
		Name:               "task27-provider",
		Summary:            "provider snapshot summary",
		Source:             skills.SourceProject,
		Locator:            "/repo/.luban-code/skills/task27/SKILL.md",
		Digest:             "sha256:2727272727272727272727272727272727272727272727272727272727272727",
		Revision:           1,
		Visibility:         skills.VisibilityAuto,
		VisibilitySource:   skills.SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
	initialSnapshot, err := skills.NewCatalogSnapshot(1, []skills.EffectiveSkill{initialSkill})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := loopapi.PlanSkillCatalog(loopapi.SkillCatalogCoordinatorInput{
		CurrentSnapshot: initialSnapshot,
		ContextEpoch:    "task27-provider-epoch",
		CharBudget:      10_000,
	})
	if err != nil || initial.Message == nil || initial.Kind != loopapi.SkillCatalogPlanSnapshot {
		t.Fatalf("initial catalog plan = %#v, %v", initial, err)
	}
	trustedInitial := initial.Message.WithInternalControlProvenance(messagecontrol.Runtime(), providerTestControlScope)
	initial.Message = &trustedInitial

	visible := []types.Message{*initial.Message, types.UserMessage("first user"), types.AssistantMessage("first assistant")}
	noChange, err := loopapi.PlanSkillCatalog(loopapi.SkillCatalogCoordinatorInput{
		CurrentSnapshot: initialSnapshot,
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "task27-provider-epoch",
		VisibleHistory:  visible,
		CharBudget:      10_000,
	})
	if err != nil || noChange.Message != nil || noChange.Kind != loopapi.SkillCatalogPlanNone {
		t.Fatalf("no-change catalog plan = %#v, %v", noChange, err)
	}

	updatedSkill := initialSkill
	updatedSkill.Summary = "provider delta summary"
	updatedSkill.Digest = "sha256:2828282828282828282828282828282828282828282828282828282828282828"
	updatedSkill.Revision = 2
	updatedSnapshot, err := skills.NewCatalogSnapshot(2, []skills.EffectiveSkill{updatedSkill})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := loopapi.PlanSkillCatalog(loopapi.SkillCatalogCoordinatorInput{
		CurrentSnapshot: updatedSnapshot,
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "task27-provider-epoch",
		VisibleHistory:  visible,
		CharBudget:      10_000,
	})
	if err != nil || changed.Message == nil || changed.Kind != loopapi.SkillCatalogPlanDelta {
		t.Fatalf("changed catalog plan = %#v, %v", changed, err)
	}
	trustedChanged := changed.Message.WithInternalControlProvenance(messagecontrol.Runtime(), providerTestControlScope)
	changed.Message = &trustedChanged

	return task27ProviderPlans{
		snapshotText: initial.Message.GetText(),
		deltaText:    changed.Message.GetText(),
		first:        []types.Message{*initial.Message, types.UserMessage("first user")},
		noChange: append(append([]types.Message(nil), visible...),
			types.UserMessage("no-change user")),
		changed: append(append([]types.Message(nil), visible...),
			*changed.Message, types.UserMessage("changed user")),
	}
}

func task27CaptureAnthropic(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task27CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewAnthropic(providerapi.Config{AuthToken: "task27-token", BaseURL: baseURL, Model: "claude-sonnet-4-6"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_task27\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	})
}

func task27CaptureOpenAIChat(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task27CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewOpenAI(providerapi.Config{ProviderName: "openai", APIKey: "task27-key", BaseURL: baseURL, Model: "o3"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_task27\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"o3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
}

func task27CaptureResponses(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task27CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewResponses(providerapi.Config{APIKey: "task27-key", BaseURL: baseURL, Model: "o3"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"id\":\"resp_task27\"}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_task27\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	})
}

func task27CaptureProviderRequest(
	t *testing.T,
	newProvider func(string) providerapi.Provider,
	params providerapi.Params,
	respond func(http.ResponseWriter),
) map[string]any {
	t.Helper()
	params = params.WithInternalControlScope(messagecontrol.Runtime(), providerTestControlScope)
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read provider request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decode provider request: %v\n%s", err, body)
			return
		}
		respond(w)
	}))
	defer server.Close()

	stream, err := newProvider(server.URL).CreateStream(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if captured == nil {
		t.Fatal("provider request was not captured")
	}
	return captured
}

func task27StableSkillTools() []types.ToolDefinition {
	return []types.ToolDefinition{{
		Name:        "Skill",
		Description: "task27 stable skill loader",
		InputSchema: types.StrictObjectSchema(map[string]any{
			"skill": map[string]any{"type": "string"},
		}, "skill"),
		Strict: true,
	}}
}

func task27AnyItems(t testing.TB, body map[string]any, key string) []any {
	t.Helper()
	items, ok := body[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", key, body[key])
	}
	return items
}

func task27AssertSerializedPrefix(t testing.TB, prefix, complete []any) {
	t.Helper()
	if len(complete) < len(prefix) {
		t.Fatalf("serialized sequence shortened: prefix=%d complete=%d", len(prefix), len(complete))
	}
	want := task27JSON(t, prefix)
	got := task27JSON(t, complete[:len(prefix)])
	if !bytes.Equal(got, want) {
		t.Fatalf("serialized request sequence rewrote its prior prefix\n got: %s\nwant: %s", got, want)
	}
}

func task27AssertStableFields(t testing.TB, before, after map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if !reflect.DeepEqual(before[key], after[key]) {
			t.Fatalf("catalog change rewrote stable field %q\nbefore: %#v\n after: %#v", key, before[key], after[key])
		}
	}
}

func task27WithoutCacheControl(t testing.TB, value []any) []any {
	t.Helper()
	var clone []any
	if err := json.Unmarshal(task27JSON(t, value), &clone); err != nil {
		t.Fatal(err)
	}
	var strip func(any)
	strip = func(candidate any) {
		switch typed := candidate.(type) {
		case map[string]any:
			delete(typed, "cache_control")
			for _, nested := range typed {
				strip(nested)
			}
		case []any:
			for _, nested := range typed {
				strip(nested)
			}
		}
	}
	strip(clone)
	return clone
}

func task27CountKey(value any, key string) int {
	count := 0
	var walk func(any)
	walk = func(candidate any) {
		switch typed := candidate.(type) {
		case map[string]any:
			if _, ok := typed[key]; ok {
				count++
			}
			for _, nested := range typed {
				walk(nested)
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
	return count
}

func task27CountRoleContent(items []any, role, content string) int {
	count := 0
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if ok && mapped["role"] == role && mapped["content"] == content {
			count++
		}
	}
	return count
}

func task27CountText(value any, needle string) int {
	count := 0
	var walk func(any)
	walk = func(candidate any) {
		switch typed := candidate.(type) {
		case string:
			if typed == needle || typed == "<system-reminder>\n"+needle+"\n</system-reminder>" {
				count++
			}
		case map[string]any:
			for _, nested := range typed {
				walk(nested)
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
	return count
}

func task27AssertRoleContent(t testing.TB, items []any, want [][2]string) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("incremental input length = %d, want %d: %#v", len(items), len(want), items)
	}
	for index, expected := range want {
		mapped, ok := items[index].(map[string]any)
		if !ok || mapped["role"] != expected[0] || mapped["content"] != expected[1] {
			t.Fatalf("input[%d] = %#v, want role=%q content=%q", index, items[index], expected[0], expected[1])
		}
	}
}

func task27JSON(t testing.TB, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
