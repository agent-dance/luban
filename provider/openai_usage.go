package provider

import (
	"encoding/json"

	openai "github.com/sashabaranov/go-openai"

	"github.com/agent-dance/luban/types"
)

type extendedOpenAIUsageEnvelope struct {
	Usage *struct {
		PromptTokens          int `json:"prompt_tokens"`
		CompletionTokens      int `json:"completion_tokens"`
		InputTokens           int `json:"input_tokens"`
		OutputTokens          int `json:"output_tokens"`
		CachedTokens          int `json:"cached_tokens"`
		PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
		PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
		CacheReadInputTokens  int `json:"cache_read_input_tokens"`
		CacheWriteInputTokens int `json:"cache_creation_input_tokens"`
		PromptTokensDetails   struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
		InputTokensDetails struct {
			CachedTokens     int `json:"cached_tokens"`
			CacheWriteTokens int `json:"cache_write_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

func decodeOpenAIStreamChunk(raw []byte, dialect OpenAIDialect) (openai.ChatCompletionStreamResponse, *types.Usage, error) {
	var response openai.ChatCompletionStreamResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return response, nil, err
	}

	var envelope extendedOpenAIUsageEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return response, nil, err
	}
	if envelope.Usage == nil {
		return response, nil, nil
	}

	u := envelope.Usage
	totalInput := firstPositiveOpenAIUsageValue(u.PromptTokens, u.InputTokens)
	outputTokens := firstPositiveOpenAIUsageValue(u.CompletionTokens, u.OutputTokens)
	cacheRead := firstPositiveOpenAIUsageValue(
		u.PromptTokensDetails.CachedTokens,
		u.InputTokensDetails.CachedTokens,
		u.CachedTokens,
		u.PromptCacheHitTokens,
		u.CacheReadInputTokens,
	)
	cacheWrite := firstPositiveOpenAIUsageValue(
		u.PromptTokensDetails.CacheWriteTokens,
		u.InputTokensDetails.CacheWriteTokens,
		u.CacheWriteInputTokens,
	)
	if dialect == DialectDeepSeek {
		if u.PromptCacheHitTokens > 0 {
			cacheRead = u.PromptCacheHitTokens
		}
	}
	if totalInput <= 0 && u.PromptCacheHitTokens+u.PromptCacheMissTokens > 0 {
		totalInput = u.PromptCacheHitTokens + u.PromptCacheMissTokens
	}
	return response, normalizeOpenAIUsage(
		totalInput,
		outputTokens,
		cacheRead,
		cacheWrite,
	), nil
}

func firstPositiveOpenAIUsageValue(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// normalizeOpenAIUsage keeps OpenAI's total input count and normalizes its
// cache details so every bucket is non-negative and bounded by that total.
func normalizeOpenAIUsage(inputTokens, outputTokens, cachedTokens, cacheWriteTokens int) *types.Usage {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}
	if cachedTokens > inputTokens {
		cachedTokens = inputTokens
	}
	if cacheWriteTokens > inputTokens-cachedTokens {
		cacheWriteTokens = inputTokens - cachedTokens
	}
	usage := &types.Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	if cachedTokens > 0 {
		usage.CacheReadInputTokens = cachedTokens
	}
	if cacheWriteTokens > 0 {
		usage.CacheCreationInputTokens = cacheWriteTokens
	}
	return usage
}
