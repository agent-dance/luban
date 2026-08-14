package provider

import "testing"

func TestLookupMaxContext_ExactMatch(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-5.6", 353400},
		{"gpt-5.6-sol", 353400},
		{"gpt-5.6-terra", 353400},
		{"gpt-5.6-luna", 353400},
		{"gpt-5.5", 258400},
		{"gpt-5.4", 258400},
		{"gpt-5.4-mini", 258400},
		{"kimi-k3", 1000000},
		{"claude-fable-5", 1000000},
		{"claude-opus-4-8", 1000000},
		{"claude-sonnet-5", 1000000},
		{"claude-opus-4-7", 1000000},
		{"claude-sonnet-4-6", 1000000},
		{"gemini-3.5-flash", 1048576},
		{"gemini-2.5-pro", 1048576},
		{"deepseek-v4-flash", 1048576},
		{"mistral-large-2512", 256000},
		{"mistral-large-latest", 256000},
		{"kimi-k2.6", 262144},
		{"kimi-k2.5", 262144},
		{"MiniMax-M3", 1000000},
		{"MiniMax-M2.7", 204800},
		{"glm-5.2", 1000000},
		{"glm-5.1", 200000},
		{"llama3.1", 131072},
		{"codellama", 16384},
	}
	for _, tt := range tests {
		got := LookupMaxContext(tt.model)
		if got != tt.want {
			t.Errorf("LookupMaxContext(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestLookupMaxContext_PrefixMatch(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-4o-2024-05-13", 128000},
		{"gpt-4o-mini-2024-07-18", 128000},
		{"gpt-4-turbo-preview", 128000},
		{"gemini-2.5-pro-latest", 1048576},
	}
	for _, tt := range tests {
		got := LookupMaxContext(tt.model)
		if got != tt.want {
			t.Errorf("LookupMaxContext(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestLookupMaxContext_PrefixLongestMatch(t *testing.T) {
	// "gpt-5.4-mini-xxx" should match "gpt-5.4-mini", whose API
	// window is replaced by the effective context Codex exposes.
	got := LookupMaxContext("gpt-5.4-mini-2026-03-17")
	if got != 258400 {
		t.Errorf("LookupMaxContext(\"gpt-5.4-mini-2026-03-17\") = %d, want 258400", got)
	}
}

func TestLookupMaxContext_BedrockRegionalAnthropicModel(t *testing.T) {
	got := LookupMaxContext("us.anthropic.claude-opus-4-8")
	if got != 1000000 {
		t.Errorf("LookupMaxContext(\"us.anthropic.claude-opus-4-8\") = %d, want 1000000", got)
	}
}

func TestLookupMaxOutput(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-5.6", 128000},
		{"gpt-5.6-terra", 128000},
		{"claude-sonnet-5", 128000},
		{"gemini-3.5-flash", 65536},
		{"MiniMax-M3", 524288},
		{"kimi-k2.6", 0},
		{"mistral-large-latest", 0},
		{"totally-unknown-model", 0},
	}
	for _, tt := range tests {
		if got := LookupMaxOutput(tt.model); got != tt.want {
			t.Errorf("LookupMaxOutput(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestDefaultMaxOutputTokens(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		want     int
	}{
		{provider: "deepseek", model: "deepseek-v4-flash", want: 256_000},
		{provider: "DeepSeek", model: "deepseek-v4-pro", want: 256_000},
		{provider: "", model: "deepseek-v4-flash", want: 256_000},
		{provider: "openai", model: "gpt-5.6-sol", want: 16 * 1024},
		{provider: "anthropic", model: "claude-sonnet-5", want: 16 * 1024},
	}
	for _, tt := range tests {
		if got := DefaultMaxOutputTokens(tt.provider, tt.model); got != tt.want {
			t.Errorf("DefaultMaxOutputTokens(%q, %q) = %d, want %d", tt.provider, tt.model, got, tt.want)
		}
	}
}

func TestLookupMaxContext_Unknown(t *testing.T) {
	got := LookupMaxContext("totally-unknown-model")
	if got != defaultMaxContext {
		t.Errorf("LookupMaxContext(\"totally-unknown-model\") = %d, want %d", got, defaultMaxContext)
	}
}

func TestLookupMaxContext_EmptyString(t *testing.T) {
	got := LookupMaxContext("")
	if got != defaultMaxContext {
		t.Errorf("LookupMaxContext(\"\") = %d, want %d", got, defaultMaxContext)
	}
}
