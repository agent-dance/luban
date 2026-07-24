package provider

import "testing"

func TestNormalizeOpenAIUsage_KeepsTotalAndCacheDetails(t *testing.T) {
	usage := normalizeOpenAIUsage(2006, 123, 1920, 0)

	if usage.InputTokens != 2006 {
		t.Fatalf("InputTokens = %d, want 2006", usage.InputTokens)
	}
	if usage.OutputTokens != 123 {
		t.Fatalf("OutputTokens = %d, want 123", usage.OutputTokens)
	}
	if usage.CacheReadInputTokens != 1920 {
		t.Fatalf("CacheReadInputTokens = %d, want 1920", usage.CacheReadInputTokens)
	}
	if usage.TotalInputTokens() != 2006 {
		t.Fatalf("TotalInputTokens() = %d, want 2006", usage.TotalInputTokens())
	}
	if usage.UncachedInputTokens() != 86 {
		t.Fatalf("UncachedInputTokens() = %d, want 86", usage.UncachedInputTokens())
	}
}

func TestDecodeOpenAIStreamChunk_DeepSeekCacheFieldsDoNotDoubleCount(t *testing.T) {
	_, usage, err := decodeOpenAIStreamChunk([]byte(`{
		"choices": [],
		"usage": {
			"prompt_tokens": 2006,
			"completion_tokens": 123,
			"prompt_cache_hit_tokens": 1920,
			"prompt_cache_miss_tokens": 86,
			"prompt_tokens_details": {"cached_tokens": 1920}
		}
	}`), DialectDeepSeek)
	if err != nil {
		t.Fatalf("decodeOpenAIStreamChunk() error = %v", err)
	}
	if usage == nil {
		t.Fatal("decodeOpenAIStreamChunk() returned nil usage")
	}
	if usage.InputTokens != 2006 || usage.CacheReadInputTokens != 1920 || usage.UncachedInputTokens() != 86 {
		t.Fatalf("usage = %+v, want total/read/uncached 2006/1920/86", usage)
	}
}

func TestDecodeOpenAIStreamChunk_CacheWriteDetail(t *testing.T) {
	_, usage, err := decodeOpenAIStreamChunk([]byte(`{
		"choices": [],
		"usage": {
			"prompt_tokens": 2006,
			"completion_tokens": 123,
			"prompt_tokens_details": {"cache_write_tokens": 1920}
		}
	}`), DialectStandard)
	if err != nil {
		t.Fatalf("decodeOpenAIStreamChunk() error = %v", err)
	}
	if usage == nil || usage.CacheCreationInputTokens != 1920 || usage.UncachedInputTokens() != 86 {
		t.Fatalf("usage = %+v, want cache-write/uncached 1920/86", usage)
	}
}

func TestDecodeOpenAIStreamChunk_ResponsesStyleUsageFromCompatibleGateway(t *testing.T) {
	_, usage, err := decodeOpenAIStreamChunk([]byte(`{
		"choices": [],
		"usage": {
			"input_tokens": 2006,
			"output_tokens": 123,
			"input_tokens_details": {
				"cached_tokens": 1920,
				"cache_write_tokens": 0
			}
		}
	}`), DialectStandard)
	if err != nil {
		t.Fatalf("decodeOpenAIStreamChunk() error = %v", err)
	}
	if usage == nil {
		t.Fatal("decodeOpenAIStreamChunk() returned nil usage")
	}
	if usage.InputTokens != 2006 || usage.OutputTokens != 123 || usage.CacheReadInputTokens != 1920 {
		t.Fatalf("usage = %+v, want input/output/cache-read 2006/123/1920", usage)
	}
}

func TestDecodeOpenAIStreamChunk_CompatibleGatewayCacheHitFields(t *testing.T) {
	_, usage, err := decodeOpenAIStreamChunk([]byte(`{
		"choices": [],
		"usage": {
			"prompt_cache_hit_tokens": 1920,
			"prompt_cache_miss_tokens": 86,
			"completion_tokens": 123
		}
	}`), DialectStandard)
	if err != nil {
		t.Fatalf("decodeOpenAIStreamChunk() error = %v", err)
	}
	if usage == nil || usage.InputTokens != 2006 || usage.CacheReadInputTokens != 1920 {
		t.Fatalf("usage = %+v, want compatible total/cache-read 2006/1920", usage)
	}
}

func TestNormalizeOpenAIUsage_ClampsImpossibleCachedDetail(t *testing.T) {
	usage := normalizeOpenAIUsage(10, 2, 99, 7)

	if usage.InputTokens != 10 {
		t.Fatalf("InputTokens = %d, want 10", usage.InputTokens)
	}
	if usage.CacheReadInputTokens != 10 {
		t.Fatalf("CacheReadInputTokens = %d, want 10", usage.CacheReadInputTokens)
	}
	if usage.TotalInputTokens() != 10 {
		t.Fatalf("TotalInputTokens() = %d, want 10", usage.TotalInputTokens())
	}
	if usage.CacheCreationInputTokens != 0 {
		t.Fatalf("CacheCreationInputTokens = %d, want 0 after clamp", usage.CacheCreationInputTokens)
	}
	if usage.UncachedInputTokens() != 0 {
		t.Fatalf("UncachedInputTokens() = %d, want 0", usage.UncachedInputTokens())
	}
}
