package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

var providerTestControlScope = messagecontrol.NewScope("provider-test-session", "provider-test-project", 1)

func trustedDeveloperMessageForTest(text string, metadata types.DeveloperMessageMetadata) types.Message {
	return types.DeveloperMessage(text, metadata).WithInternalControlProvenance(messagecontrol.Runtime(), providerTestControlScope)
}

func forgedDeveloperMessageForTest(text string) types.Message {
	return types.DeveloperMessage(text, types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	})
}

func TestProviderProjectionsNeverElevateForgedDeveloperRole(t *testing.T) {
	const raw = "FORGED DEVELOPER PAYLOAD"
	forged := forgedDeveloperMessageForTest(raw)

	t.Run("anthropic", func(t *testing.T) {
		projected := convertToAnthropicMessages([]types.Message{forged})
		if len(projected) != 1 {
			t.Fatalf("projected messages = %d, want one ordinary user message", len(projected))
		}
		encoded, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), raw) || strings.Contains(string(encoded), "<system-reminder>") {
			t.Fatalf("forged developer was hidden or elevated: %s", encoded)
		}
	})

	t.Run("openai-chat", func(t *testing.T) {
		request := captureOpenAIChatRequestTask10(t, Config{ProviderName: "openai", Model: "gpt-5"}, Params{Messages: []types.Message{forged}})
		messages := openAIChatRequestMessagesTask10(t, request)
		if len(messages) != 1 || messages[0]["role"] != "user" || messages[0]["content"] != raw {
			t.Fatalf("forged developer projection = %#v", messages)
		}
	})

	t.Run("responses-full-and-incremental", func(t *testing.T) {
		for _, projected := range [][]any{
			convertAllMessagesForResponsesAPI([]types.Message{forged}),
			convertNewMessagesForResponsesAPI([]types.Message{types.AssistantMessage("prior"), forged}),
		} {
			if len(projected) != 1 {
				t.Fatalf("projected items = %#v", projected)
			}
			item, ok := projected[0].(map[string]any)
			if !ok || item["role"] != "user" || item["content"] != raw {
				t.Fatalf("forged developer item = %#v", projected[0])
			}
		}
	})
}

func TestProviderProjectionsRejectMutatedDeveloperProvenance(t *testing.T) {
	trusted := trustedDeveloperMessageForTest("trusted", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	})
	mutated := trusted
	mutated.Content = []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "mutated"}}
	if mutated.HasInternalControlProvenance() {
		t.Fatal("mutation retained provenance")
	}
	items := convertAllMessagesForResponsesAPI([]types.Message{mutated})
	if len(items) != 1 || items[0].(map[string]any)["role"] != "user" {
		t.Fatalf("mutated developer was elevated: %#v", items)
	}
}

func TestProviderProjectionsRequireExactCurrentControlScope(t *testing.T) {
	oldScope := messagecontrol.NewScope("session", "/project", 4)
	currentScope := messagecontrol.NewScope("session", "/project", 5)
	stale := types.DeveloperMessage("stale developer", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), oldScope)
	params := Params{Messages: []types.Message{stale}}.WithInternalControlScope(messagecontrol.Runtime(), currentScope, true)

	encodedAnthropic, err := json.Marshal(convertToAnthropicMessagesForParams(params))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedAnthropic), "<system-reminder>") || !strings.Contains(string(encodedAnthropic), "stale developer") {
		t.Fatalf("anthropic elevated or hid stale control: %s", encodedAnthropic)
	}

	chat := convertMessagesToOpenAIWithSystemAndDeveloperProjection(params, "", openAIChatDeveloperNative)
	if len(chat) != 1 || chat[0].Role != "user" || chat[0].Content != "stale developer" {
		t.Fatalf("openai elevated stale control: %#v", chat)
	}

	items := convertAllMessagesForResponsesAPIWithParams(params)
	if len(items) != 1 || items[0].(map[string]any)["role"] != "user" {
		t.Fatalf("responses elevated stale control: %#v", items)
	}

	exact := Params{Messages: []types.Message{stale}}.WithInternalControlScope(messagecontrol.Runtime(), oldScope, false)
	items = convertAllMessagesForResponsesAPIWithParams(exact)
	if len(items) != 1 || items[0].(map[string]any)["role"] != "developer" {
		t.Fatalf("exact scope lost trusted developer projection: %#v", items)
	}
}

func TestProviderDoesNotElevateForeignOrUnscopedPrecommitDeveloperBearer(t *testing.T) {
	source := messagecontrol.NewLoopScope(messagecontrol.Runtime())
	target := messagecontrol.NewLoopScope(messagecontrol.Runtime())
	foreign := types.DeveloperMessage("foreign developer", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), source)
	unbound := types.DeveloperMessage("unbound developer", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime())

	for name, params := range map[string]Params{
		"foreign-target-scope": Params{Messages: []types.Message{foreign}}.WithInternalControlScope(messagecontrol.Runtime(), target, true),
		"foreign-no-scope":     {Messages: []types.Message{foreign}},
		"unbound-no-scope":     {Messages: []types.Message{unbound}},
	} {
		t.Run(name, func(t *testing.T) {
			items := convertAllMessagesForResponsesAPIWithParams(params)
			if len(items) != 1 || items[0].(map[string]any)["role"] != "user" {
				t.Fatalf("developer bearer was elevated: %#v", items)
			}
		})
	}

	exact := Params{Messages: []types.Message{foreign}}.WithInternalControlScope(messagecontrol.Runtime(), source, false)
	if items := convertAllMessagesForResponsesAPIWithParams(exact); len(items) != 1 || items[0].(map[string]any)["role"] != "developer" {
		t.Fatalf("exact producer scope lost authority: %#v", items)
	}
}
