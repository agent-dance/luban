package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/prompt"
)

func TestPromptCacheUserNamespaceIsOpaqueAndStable(t *testing.T) {
	config := Config{ProviderName: "openai", APIKey: "secret-account-key"}
	first := promptCacheUserNamespace(config)
	second := promptCacheUserNamespace(config)
	if first == "" || first != second {
		t.Fatalf("credential namespace is not stable: %q != %q", first, second)
	}
	if strings.Contains(first, config.APIKey) {
		t.Fatalf("credential namespace leaked the raw API key: %q", first)
	}
	if other := promptCacheUserNamespace(Config{ProviderName: "openai", APIKey: "other-account-key"}); other == first {
		t.Fatalf("different credentials shared namespace %q", first)
	}
}

func TestScopedPromptCacheKeySharesIndependentSessionsByDefault(t *testing.T) {
	t.Setenv("LUBAN_CODE_PROMPT_CACHE_SHARDS", "")
	namespace := promptCacheUserNamespace(Config{ProviderName: "openai", APIKey: "account-key"})
	first := scopedPromptCacheKey(namespace, "session-a", "gpt-5.6-sol", promptCacheRoutingShardCount("openai"))
	second := scopedPromptCacheKey(namespace, "session-b", "gpt-5.6-sol", promptCacheRoutingShardCount("openai"))
	if first == "" || first != second {
		t.Fatalf("default routing did not share independent sessions: %q != %q", first, second)
	}
	if otherModel := scopedPromptCacheKey(namespace, "session-a", "gpt-5.6-terra", 1); otherModel == first {
		t.Fatalf("different models shared routing key %q", first)
	}
}

func TestPromptCacheRoutingOverrideOnlyShardsOpenAI(t *testing.T) {
	t.Setenv("LUBAN_CODE_PROMPT_CACHE_SHARDS", "16")
	if got := promptCacheRoutingShardCount("openai"); got != 16 {
		t.Fatalf("OpenAI cache routing override = %d shards, want 16", got)
	}
	for _, providerName := range []string{"deepseek", "mistral", "kimi"} {
		if got := promptCacheRoutingShardCount(providerName); got != 1 {
			t.Fatalf("%s cache user scope was split into %d shards", providerName, got)
		}
	}
}

func TestPromptCacheUserNamespacePrefersStableOAuthAccount(t *testing.T) {
	base := Config{
		ProviderName: "openai",
		AuthToken:    "rotating-token-a",
		Headers:      map[string]string{"chatgpt-account-id": "account-123"},
	}
	first := promptCacheUserNamespace(base)
	base.AuthToken = "rotating-token-b"
	if second := promptCacheUserNamespace(base); second != first {
		t.Fatalf("OAuth refresh changed account cache identity: %q != %q", first, second)
	}
}

func TestOpenAIPromptCachePoliciesUseLongestDocumentedRetention(t *testing.T) {
	for _, model := range []string{"gpt-5.6-sol", "gpt-5.7"} {
		policy := promptCachePolicyForOpenAIModel(model)
		if !policy.Options || policy.Retention != "" {
			t.Fatalf("%s policy = %#v, want GPT-5.6+ options", model, policy)
		}
	}
	for _, model := range []string{"gpt-5.5", "gpt-5.4-2026-03-05", "gpt-5.1-codex-max", "gpt-4.1"} {
		policy := promptCachePolicyForOpenAIModel(model)
		if policy.Options || policy.Retention != "24h" {
			t.Fatalf("%s policy = %#v, want 24h retention", model, policy)
		}
	}
	for _, model := range []string{"gpt-5.4-mini", "gpt-4o", "o3"} {
		if policy := promptCachePolicyForOpenAIModel(model); policy != (openAIPromptCachePolicy{}) {
			t.Fatalf("%s received undocumented policy %#v", model, policy)
		}
	}
}

func TestOpenAIStaticSystemBreakpointPreservesJoinedText(t *testing.T) {
	blocks := []prompt.SystemPromptBlock{
		{Text: "stable one", Cache: true},
		{Text: "stable two", Cache: true},
		{Text: "dynamic"},
	}
	content, ok := openAIStaticSystemContent(blocks, "input_text")
	if !ok || len(content) != 3 {
		t.Fatalf("content = %#v, ok=%v", content, ok)
	}
	var joined strings.Builder
	for _, block := range content {
		joined.WriteString(block["text"].(string))
	}
	if joined.String() != "stable one\n\nstable two\n\ndynamic" {
		t.Fatalf("serialized system text = %q", joined.String())
	}
	if _, found := content[0]["prompt_cache_breakpoint"]; found {
		t.Fatal("breakpoint attached before the longest stable system prefix")
	}
	if _, found := content[1]["prompt_cache_breakpoint"]; !found {
		t.Fatal("longest stable system prefix missing explicit breakpoint")
	}
	if _, found := content[2]["prompt_cache_breakpoint"]; found {
		t.Fatal("dynamic system content was marked cacheable")
	}
}

func TestOpenAIChatRawCachePolicyAndBreakpoint(t *testing.T) {
	body := map[string]json.RawMessage{}
	body["messages"] = json.RawMessage(`[{"role":"system","content":"stable\n\ndynamic"},{"role":"user","content":"question"}]`)
	policy := promptCachePolicyForOpenAIModel("gpt-5.6-sol")
	if err := applyOpenAIPromptCachePolicyRaw(body, policy); err != nil {
		t.Fatal(err)
	}
	if err := applyOpenAIChatSystemCacheBreakpoint(body, []prompt.SystemPromptBlock{
		{Text: "stable", Cache: true},
		{Text: "dynamic"},
	}); err != nil {
		t.Fatal(err)
	}
	var options map[string]any
	if err := json.Unmarshal(body["prompt_cache_options"], &options); err != nil {
		t.Fatal(err)
	}
	if options["mode"] != "implicit" || options["ttl"] != "30m" {
		t.Fatalf("prompt cache options = %#v", options)
	}
	var messages []map[string]any
	if err := json.Unmarshal(body["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	content := messages[0]["content"].([]any)
	stable := content[0].(map[string]any)
	if stable["prompt_cache_breakpoint"] == nil {
		t.Fatalf("system content missing explicit breakpoint: %#v", messages[0])
	}
}

func TestAnthropicPromptCacheTTLIsProviderAndModelGated(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		baseURL  string
		want     string
	}{
		{"anthropic", "claude-sonnet-5", "", "1h"},
		{"anthropic", "claude-sonnet-5", "https://proxy.example/v1", ""},
		{"vertex", "claude-sonnet-4-6", "", "1h"},
		{"bedrock", "anthropic.claude-haiku-4-5-20251001-v1:0", "", "1h"},
		{"bedrock", "anthropic.claude-sonnet-4-6", "", ""},
	}
	for _, test := range tests {
		if got := anthropicPromptCacheTTL(test.provider, test.model, test.baseURL); got != test.want {
			t.Errorf("%s/%s TTL = %q, want %q", test.provider, test.model, got, test.want)
		}
	}
}

func TestAnthropicOneHourCacheControlSerializesTTL(t *testing.T) {
	_ = convertToAnthropicMessages(nil, "1h")
	encoded, err := json.Marshal(anthropicCacheControl("1h"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"ttl":"1h"`) {
		t.Fatalf("cache control = %s, want 1h TTL", encoded)
	}
}
