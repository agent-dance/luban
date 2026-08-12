package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	loopapi "github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/prompt"
	providerapi "github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task26ProviderPlans struct {
	snapshotText string
	deltaText    string
	first        []types.Message
	noChange     []types.Message
	changed      []types.Message
}

func TestSkillCatalogIntegrationProviderRequestShapeParity(t *testing.T) {
	plans := task26BuildProviderPlans(t)
	tool := types.ToolDefinition{
		Name:        "Skill",
		Description: "stable task26 skill tool",
		InputSchema: types.StrictObjectSchema(map[string]any{"skill": map[string]any{"type": "string"}}, "skill"),
		Strict:      true,
	}
	base := providerapi.Params{
		Model:          "o3",
		System:         "stable task26 system",
		SystemBlocks:   []prompt.SystemPromptBlock{{Text: "stable task26 system", Cache: true}},
		Tools:          []types.ToolDefinition{tool},
		PromptCacheKey: "task26-cache-key",
		UsePromptCache: true,
		MaxTokens:      1024,
	}

	t.Run("anthropic full history", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task26CaptureAnthropic(t, firstParams)
		noChangeParams := base
		noChangeParams.Messages = plans.noChange
		noChange := task26CaptureAnthropic(t, noChangeParams)
		changedParams := base
		changedParams.Messages = plans.changed
		changed := task26CaptureAnthropic(t, changedParams)

		snapshotReminder := "<system-reminder>\n" + plans.snapshotText + "\n</system-reminder>"
		deltaReminder := "<system-reminder>\n" + plans.deltaText + "\n</system-reminder>"
		task26AssertProviderItems(t, task26AnthropicItems(t, first), []task26ProviderItem{
			{role: "user", content: snapshotReminder},
			{role: "user", content: "first user"},
		})
		task26AssertProviderItems(t, task26AnthropicItems(t, noChange), []task26ProviderItem{
			{role: "user", content: snapshotReminder},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "user", content: "no-change user"},
		})
		task26AssertProviderItems(t, task26AnthropicItems(t, changed), []task26ProviderItem{
			{role: "user", content: snapshotReminder},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "user", content: deltaReminder},
			{role: "user", content: "changed user"},
		})
		if !reflect.DeepEqual(first["system"], changed["system"]) || !reflect.DeepEqual(first["tools"], changed["tools"]) {
			t.Fatalf("catalog revision changed Anthropic stable envelope\nfirst=%#v\nchanged=%#v", first, changed)
		}
	})

	t.Run("openai chat full history", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task26CaptureOpenAIChat(t, firstParams)
		noChangeParams := base
		noChangeParams.Messages = plans.noChange
		noChange := task26CaptureOpenAIChat(t, noChangeParams)
		changedParams := base
		changedParams.Messages = plans.changed
		changed := task26CaptureOpenAIChat(t, changedParams)

		task26AssertProviderItems(t, task26RoleContentItems(t, first, "messages"), []task26ProviderItem{
			{role: "system", content: "stable task26 system"},
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
		})
		task26AssertProviderItems(t, task26RoleContentItems(t, noChange, "messages"), []task26ProviderItem{
			{role: "system", content: "stable task26 system"},
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "user", content: "no-change user"},
		})
		task26AssertProviderItems(t, task26RoleContentItems(t, changed, "messages"), []task26ProviderItem{
			{role: "system", content: "stable task26 system"},
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "developer", content: plans.deltaText},
			{role: "user", content: "changed user"},
		})
		if !reflect.DeepEqual(first["tools"], changed["tools"]) || first["model"] != changed["model"] {
			t.Fatalf("catalog revision changed OpenAI Chat stable envelope\nfirst=%#v\nchanged=%#v", first, changed)
		}
	})

	t.Run("responses compatible endpoint full input", func(t *testing.T) {
		firstParams := base
		firstParams.Messages = plans.first
		first := task26CaptureResponses(t, firstParams)

		noChangeParams := base
		noChangeParams.Messages = plans.noChange
		noChangeParams.PreviousResponseID = "resp_task26_previous"
		noChange := task26CaptureResponses(t, noChangeParams)

		changedParams := base
		changedParams.Messages = plans.changed
		changedParams.PreviousResponseID = "resp_task26_previous"
		changed := task26CaptureResponses(t, changedParams)

		task26AssertProviderItems(t, task26RoleContentItems(t, first, "input"), []task26ProviderItem{
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
		})
		// A custom HTTP-compatible endpoint receives the complete history because
		// response IDs returned by it do not imply HTTP chaining support.
		task26AssertProviderItems(t, task26RoleContentItems(t, noChange, "input"), []task26ProviderItem{
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "user", content: "no-change user"},
		})
		task26AssertProviderItems(t, task26RoleContentItems(t, changed, "input"), []task26ProviderItem{
			{role: "developer", content: plans.snapshotText},
			{role: "user", content: "first user"},
			{role: "assistant", content: "first assistant"},
			{role: "developer", content: plans.deltaText},
			{role: "user", content: "changed user"},
		})
		if _, ok := noChange["previous_response_id"]; ok {
			t.Fatalf("custom Responses request unexpectedly chained: %#v", noChange)
		}
		if _, ok := changed["previous_response_id"]; ok {
			t.Fatalf("custom Responses request unexpectedly chained: %#v", changed)
		}
		for _, key := range []string{"model", "instructions", "tools", "parallel_tool_calls", "prompt_cache_key"} {
			if !reflect.DeepEqual(first[key], changed[key]) {
				t.Fatalf("catalog revision changed Responses field %q\nfirst=%#v\nchanged=%#v", key, first[key], changed[key])
			}
		}
	})
}

func task26BuildProviderPlans(t *testing.T) task26ProviderPlans {
	t.Helper()
	initialSkill := skills.EffectiveSkill{
		ID:                 "skill:project:/repo/.luban-code/skills/task26/SKILL.md",
		Name:               "task26-provider",
		Summary:            "provider snapshot summary",
		Source:             skills.SourceProject,
		Locator:            "/repo/.luban-code/skills/task26/SKILL.md",
		Digest:             "sha256:2626262626262626262626262626262626262626262626262626262626262626",
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
		ContextEpoch:    "task26-provider-epoch",
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
		ContextEpoch:    "task26-provider-epoch",
		VisibleHistory:  visible,
		CharBudget:      10_000,
	})
	if err != nil || noChange.Message != nil || noChange.Kind != loopapi.SkillCatalogPlanNone {
		t.Fatalf("no-change catalog plan = %#v, %v", noChange, err)
	}

	updatedSkill := initialSkill
	updatedSkill.Summary = "provider delta summary"
	updatedSkill.Digest = "sha256:2727272727272727272727272727272727272727272727272727272727272727"
	updatedSkill.Revision = 2
	updatedSnapshot, err := skills.NewCatalogSnapshot(2, []skills.EffectiveSkill{updatedSkill})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := loopapi.PlanSkillCatalog(loopapi.SkillCatalogCoordinatorInput{
		CurrentSnapshot: updatedSnapshot,
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "task26-provider-epoch",
		VisibleHistory:  visible,
		CharBudget:      10_000,
	})
	if err != nil || changed.Message == nil || changed.Kind != loopapi.SkillCatalogPlanDelta {
		t.Fatalf("changed catalog plan = %#v, %v", changed, err)
	}
	trustedChanged := changed.Message.WithInternalControlProvenance(messagecontrol.Runtime(), providerTestControlScope)
	changed.Message = &trustedChanged

	return task26ProviderPlans{
		snapshotText: initial.Message.GetText(),
		deltaText:    changed.Message.GetText(),
		first:        []types.Message{*initial.Message, types.UserMessage("first user")},
		noChange: append(append([]types.Message(nil), visible...),
			types.UserMessage("no-change user")),
		changed: append(append([]types.Message(nil), visible...),
			*changed.Message, types.UserMessage("changed user")),
	}
}

type task26ProviderItem struct {
	role    string
	content string
}

func task26AssertProviderItems(t *testing.T, got, want []task26ProviderItem) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider logical order mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func task26RoleContentItems(t *testing.T, body map[string]any, field string) []task26ProviderItem {
	t.Helper()
	raw, ok := body[field].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", field, body[field])
	}
	items := make([]task26ProviderItem, 0, len(raw))
	for index, value := range raw {
		item, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s[%d] = %#v, want object", field, index, value)
		}
		role, roleOK := item["role"].(string)
		content, contentOK := item["content"].(string)
		if !roleOK || !contentOK {
			t.Fatalf("%s[%d] role/content = %#v", field, index, item)
		}
		items = append(items, task26ProviderItem{role: role, content: content})
	}
	return items
}

func task26AnthropicItems(t *testing.T, body map[string]any) []task26ProviderItem {
	t.Helper()
	raw, ok := body["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", body["messages"])
	}
	items := make([]task26ProviderItem, 0, len(raw))
	for index, value := range raw {
		message, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("messages[%d] = %#v", index, value)
		}
		role, _ := message["role"].(string)
		blocks, ok := message["content"].([]any)
		if !ok || len(blocks) == 0 {
			t.Fatalf("messages[%d].content = %#v, want text blocks", index, message["content"])
		}
		for blockIndex, value := range blocks {
			block, ok := value.(map[string]any)
			if !ok || block["type"] != "text" {
				t.Fatalf("messages[%d].content[%d] = %#v, want text block", index, blockIndex, value)
			}
			content, _ := block["text"].(string)
			items = append(items, task26ProviderItem{role: role, content: content})
		}
	}
	return items
}

func task26CaptureAnthropic(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task26CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewAnthropic(providerapi.Config{AuthToken: "task26-token", BaseURL: baseURL, Model: "claude-sonnet-4-6"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_task26\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	})
}

func task26CaptureOpenAIChat(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task26CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewOpenAI(providerapi.Config{ProviderName: "openai", APIKey: "task26-key", BaseURL: baseURL, Model: "o3"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_task26\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"o3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
}

func task26CaptureResponses(t *testing.T, params providerapi.Params) map[string]any {
	t.Helper()
	return task26CaptureProviderRequest(t, func(baseURL string) providerapi.Provider {
		return providerapi.NewResponses(providerapi.Config{APIKey: "task26-key", BaseURL: baseURL, Model: "o3"})
	}, params, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"id\":\"resp_task26\"}\n\nevent: response.completed\ndata: {\"response\":{\"id\":\"resp_task26\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	})
}

func task26CaptureProviderRequest(
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
