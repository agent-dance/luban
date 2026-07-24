package provider

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/prompt"
)

const defaultPromptCacheRoutingShards = 1

// promptCacheUserNamespace derives an opaque, stable cache identity from the
// configured provider account. Session IDs must not be used here: DeepSeek
// treats user_id as a KV-cache isolation boundary, while OpenAI combines
// prompt_cache_key with the prompt prefix when routing cache lookups.
func promptCacheUserNamespace(cfg Config) string {
	identity := headerValue(cfg.Headers, "ChatGPT-Account-ID")
	if identity == "" {
		identity = strings.TrimSpace(cfg.APIKey)
	}
	if identity == "" {
		identity = strings.TrimSpace(cfg.AuthToken)
	}
	if identity == "" {
		return ""
	}

	providerName := CanonicalProviderName(cfg.ProviderName)
	if providerName == "" {
		providerName = "openai"
	}
	digest := sha256.Sum256([]byte(providerName + "\x00" + identity))
	return fmt.Sprintf("pcu_%x", digest[:12])
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// promptCacheRoutingShardCount defaults to one so independently-created
// sessions reuse the same warm route. High-volume OpenAI users may explicitly
// partition traffic; DeepSeek always keeps one credential-level user_id because
// that field is a privacy boundary rather than merely a routing hint.
func promptCacheRoutingShardCount(providerName string) int {
	if canonical := CanonicalProviderName(providerName); canonical != "" && canonical != "openai" {
		return 1
	}
	raw := strings.TrimSpace(os.Getenv("LUBAN_CODE_PROMPT_CACHE_SHARDS"))
	if raw == "" {
		return defaultPromptCacheRoutingShards
	}
	shards, err := strconv.Atoi(raw)
	if err != nil || shards <= 0 {
		return defaultPromptCacheRoutingShards
	}
	return min(shards, 1024)
}

// scopedPromptCacheKey lowers a conversation lineage into a credential- and
// model-scoped routing key. With the default single shard, independent sessions
// share the same key. An explicit multi-shard configuration keeps descendants
// with the same lineage on the same route.
func scopedPromptCacheKey(userNamespace, lineage, model string, shards int) string {
	lineage = strings.TrimSpace(lineage)
	if lineage == "" || userNamespace == "" {
		return lineage
	}
	if shards <= 0 {
		shards = defaultPromptCacheRoutingShards
	}
	lineageDigest := sha256.Sum256([]byte(lineage))
	shard := (int(lineageDigest[0])<<8 | int(lineageDigest[1])) % shards
	modelDigest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(model))))
	return fmt.Sprintf("%s_m%x_s%d", userNamespace, modelDigest[:4], shard)
}

type openAIPromptCachePolicy struct {
	Options   bool
	Retention string
}

func promptCachePolicyForOpenAIModel(model string) openAIPromptCachePolicy {
	if openAIModelAtLeast56(model) {
		return openAIPromptCachePolicy{Options: true}
	}
	if openAIModelSupports24hPromptCache(model) {
		return openAIPromptCachePolicy{Retention: "24h"}
	}
	return openAIPromptCachePolicy{}
}

func openAIModelAtLeast56(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(model, "gpt-") {
		return false
	}
	version := strings.TrimPrefix(model, "gpt-")
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minorText := parts[1]
	if index := strings.IndexByte(minorText, '-'); index >= 0 {
		minorText = minorText[:index]
	}
	minor, minorErr := strconv.Atoi(minorText)
	return majorErr == nil && minorErr == nil && (major > 5 || major == 5 && minor >= 6)
}

func openAIModelSupports24hPromptCache(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, exact := range []string{
		"gpt-5.5", "gpt-5.5-pro",
		"gpt-5.4", "gpt-5.2",
		"gpt-5.1", "gpt-5.1-codex-max", "gpt-5.1-codex", "gpt-5.1-codex-mini", "gpt-5.1-chat-latest",
		"gpt-5", "gpt-5-codex", "gpt-4.1",
	} {
		if model == exact || strings.HasPrefix(model, exact+"-20") {
			return true
		}
	}
	return false
}

func applyOpenAIPromptCachePolicy(body map[string]any, model string) openAIPromptCachePolicy {
	policy := promptCachePolicyForOpenAIModel(model)
	if policy.Options {
		body["prompt_cache_options"] = map[string]any{
			"mode": "implicit",
			"ttl":  "30m",
		}
	}
	if policy.Retention != "" {
		body["prompt_cache_retention"] = policy.Retention
	}
	return policy
}

// openAIStaticSystemContent preserves the legacy joined prompt text while
// adding one explicit breakpoint after the leading cache-eligible blocks.
func openAIStaticSystemContent(blocks []prompt.SystemPromptBlock, contentType string) ([]map[string]any, bool) {
	filtered := make([]prompt.SystemPromptBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Text != "" {
			filtered = append(filtered, block)
		}
	}
	if len(filtered) == 0 {
		return nil, false
	}

	breakpoint := -1
	for index, block := range filtered {
		if !block.Cache {
			break
		}
		breakpoint = index
	}
	if breakpoint < 0 {
		return nil, false
	}

	content := make([]map[string]any, 0, len(filtered))
	for index, block := range filtered {
		text := block.Text
		if index+1 < len(filtered) {
			text += "\n\n"
		}
		item := map[string]any{"type": contentType, "text": text}
		if index == breakpoint {
			item["prompt_cache_breakpoint"] = map[string]string{"mode": "explicit"}
		}
		content = append(content, item)
	}
	return content, true
}

func applyOpenAIPromptCachePolicyRaw(body map[string]json.RawMessage, policy openAIPromptCachePolicy) error {
	if policy.Options {
		encoded, err := json.Marshal(map[string]any{
			"mode": "implicit",
			"ttl":  "30m",
		})
		if err != nil {
			return err
		}
		body["prompt_cache_options"] = encoded
	}
	if policy.Retention != "" {
		encoded, err := json.Marshal(policy.Retention)
		if err != nil {
			return err
		}
		body["prompt_cache_retention"] = encoded
	}
	return nil
}

func applyOpenAIChatSystemCacheBreakpoint(body map[string]json.RawMessage, blocks []prompt.SystemPromptBlock) error {
	content, ok := openAIStaticSystemContent(blocks, "text")
	if !ok {
		return nil
	}
	var messages []map[string]any
	if err := json.Unmarshal(body["messages"], &messages); err != nil {
		return err
	}
	for index := range messages {
		role, _ := messages[index]["role"].(string)
		if role != "system" {
			continue
		}
		messages[index]["content"] = content
		encoded, err := json.Marshal(messages)
		if err != nil {
			return err
		}
		body["messages"] = encoded
		return nil
	}
	return nil
}

func anthropicPromptCacheTTL(providerName, model, baseURL string) string {
	providerName = CanonicalProviderName(providerName)
	model = strings.ToLower(strings.TrimSpace(model))
	switch providerName {
	case "anthropic":
		if baseURL != "" && cacheEndpointHostname(baseURL) != "api.anthropic.com" {
			return ""
		}
		if knownAnthropicOneHourModel(model, true) {
			return "1h"
		}
	case "vertex":
		if knownAnthropicOneHourModel(model, true) {
			return "1h"
		}
	case "bedrock":
		// Bedrock currently excludes the 4.6 family from its documented 1h list.
		if strings.Contains(model, "claude-sonnet-4-6") || strings.Contains(model, "claude-opus-4-6") {
			return ""
		}
		if knownAnthropicOneHourModel(model, false) {
			return "1h"
		}
	}
	return ""
}

func knownAnthropicOneHourModel(model string, include46 bool) bool {
	for _, marker := range []string{
		"claude-sonnet-5", "claude-fable-5", "claude-mythos-5",
		"claude-opus-4-8", "claude-opus-4-7",
		"claude-sonnet-4-5", "claude-opus-4-5", "claude-haiku-4-5",
	} {
		if strings.Contains(model, marker) {
			return true
		}
	}
	return include46 && (strings.Contains(model, "claude-sonnet-4-6") || strings.Contains(model, "claude-opus-4-6"))
}
