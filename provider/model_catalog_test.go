package provider

import (
	"slices"
	"testing"
)

func TestNewModelCatalog_Empty(t *testing.T) {
	c := NewModelCatalog()
	if c.Count() != 0 {
		t.Fatalf("expected empty catalog, got %d models", c.Count())
	}
}

func TestModelCatalog_RegisterAndGet(t *testing.T) {
	c := NewModelCatalog()
	m := ModelInfo{
		ID:            "test-model",
		Name:          "Test Model",
		Provider:      "test",
		CostCurrency:  "USD",
		ContextWindow: 128000,
		IsDefault:     true,
	}
	c.Register(m)

	got, ok := c.Get("test-model")
	if !ok {
		t.Fatal("expected to find test-model")
	}
	if got.Name != "Test Model" {
		t.Errorf("expected name %q, got %q", "Test Model", got.Name)
	}
	if got.ContextWindow != 128000 {
		t.Errorf("expected context window 128000, got %d", got.ContextWindow)
	}
}

func TestModelCatalog_GetMissing(t *testing.T) {
	c := NewModelCatalog()
	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent model")
	}
}

func TestModelCatalog_Resolve_ExactMatch(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-4o", Provider: "openai"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-4o-mini", Provider: "openai"})

	m, ok := c.Resolve("gpt-4o")
	if !ok || m.ID != "gpt-4o" {
		t.Fatalf("expected exact match for gpt-4o, got %v (found=%v)", m.ID, ok)
	}
}

func TestModelCatalog_Resolve_PrefixMatch(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-4o", Provider: "openai"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-4o-mini", Provider: "openai"})

	// "gpt-4o-2024-05-13" should match "gpt-4o" (longest prefix with word boundary)
	m, ok := c.Resolve("gpt-4o-2024-05-13")
	if !ok || m.ID != "gpt-4o" {
		t.Fatalf("expected prefix match for gpt-4o, got %v (found=%v)", m.ID, ok)
	}

	// "gpt-4o-mini-2024-07-18" should match "gpt-4o-mini" not "gpt-4o"
	m, ok = c.Resolve("gpt-4o-mini-2024-07-18")
	if !ok || m.ID != "gpt-4o-mini" {
		t.Fatalf("expected prefix match for gpt-4o-mini, got %v (found=%v)", m.ID, ok)
	}
}

func TestModelCatalog_Resolve_CurrentProviderIdentifier(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-5.6-sol", Aliases: []string{"gpt-5.6"}, Provider: "openai"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-5.6-terra", Provider: "openai"})

	m, ok := c.ResolveForProvider("openai", "gpt-5.6")
	if !ok || m.ID != "gpt-5.6-sol" {
		t.Fatalf("provider identifier resolved to %+v, found=%v; want gpt-5.6-sol", m, ok)
	}

	m, ok = c.Resolve("gpt-5.6-terra-2026-06-01")
	if !ok || m.ID != "gpt-5.6-terra" {
		t.Fatalf("versioned canonical ID resolved to %+v, found=%v; want gpt-5.6-terra", m, ok)
	}
}

func TestModelCatalog_Resolve_NoMatch(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "gpt-4o", Provider: "openai"})

	_, ok := c.Resolve("completely-unknown")
	if ok {
		t.Fatal("expected no match for completely-unknown")
	}
}

func TestModelCatalog_ListByProvider(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "model-a", Provider: "p1"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "model-b", Provider: "p1", IsDefault: true})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "model-c", Provider: "p2"})

	list := c.ListByProvider("p1")
	if len(list) != 2 {
		t.Fatalf("expected 2 models for p1, got %d", len(list))
	}
	// Default model should sort first
	if list[0].ID != "model-b" {
		t.Errorf("expected default model first, got %q", list[0].ID)
	}
}

func TestModelCatalog_All_SortOrder(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "z-model", Provider: "beta"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "a-model", Provider: "alpha", IsDefault: true})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "b-model", Provider: "alpha"})

	all := c.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 models, got %d", len(all))
	}
	// Should be sorted by provider first, then default first, then ID
	if all[0].Provider != "alpha" || all[0].ID != "a-model" {
		t.Errorf("expected alpha/a-model first (default), got %s/%s", all[0].Provider, all[0].ID)
	}
	if all[1].Provider != "alpha" || all[1].ID != "b-model" {
		t.Errorf("expected alpha/b-model second, got %s/%s", all[1].Provider, all[1].ID)
	}
	if all[2].Provider != "beta" {
		t.Errorf("expected beta last, got %s", all[2].Provider)
	}
}

func TestModelCatalog_DefaultForProvider(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "model-a", Provider: "test"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "model-b", Provider: "test", IsDefault: true})

	def := c.DefaultForProvider("test")
	if def != "model-b" {
		t.Errorf("expected model-b as default, got %q", def)
	}

	// Provider with no default
	def = c.DefaultForProvider("nonexistent")
	if def != "" {
		t.Errorf("expected empty default for nonexistent provider, got %q", def)
	}
}

func TestModelCatalog_ResolveForProvider(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "shared-model", Provider: "anthropic", ContextWindow: 200000})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "shared-model", Provider: "vertex", ContextWindow: 200000})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "shared-model-lite", Provider: "vertex", ContextWindow: 128000})

	m, ok := c.ResolveForProvider("vertex", "shared-model")
	if !ok || m.Provider != "vertex" || m.ID != "shared-model" {
		t.Fatalf("ResolveForProvider exact = %+v, found=%v", m, ok)
	}

	m, ok = c.ResolveForProvider("vertex", "shared-model-lite-2025-01")
	if !ok || m.Provider != "vertex" || m.ID != "shared-model-lite" {
		t.Fatalf("ResolveForProvider prefix = %+v, found=%v", m, ok)
	}
}

func TestModelCatalog_ModelIDsByProvider(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "b", Provider: "test"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "a", Provider: "test", IsDefault: true})

	ids := c.ModelIDsByProvider("test")
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids = %#v, want [a b]", ids)
	}
}

func TestDefaultCatalog_ModelCount(t *testing.T) {
	c := DefaultCatalog()
	count := c.Count()
	// Generated remote models plus local-only Ollama aliases.
	if count < 69 {
		t.Errorf("expected at least 69 models in default catalog, got %d", count)
	}
}

func TestDefaultCatalog_AnthropicDefault(t *testing.T) {
	c := DefaultCatalog()
	def := c.DefaultForProvider("anthropic")
	if def != "claude-sonnet-5" {
		t.Errorf("expected claude-sonnet-5 as anthropic default, got %q", def)
	}
}

func TestDefaultCatalog_OpenAIDefault(t *testing.T) {
	c := DefaultCatalog()
	def := c.DefaultForProvider("openai")
	if def != "gpt-5.6-sol" {
		t.Errorf("expected gpt-5.6-sol as openai default, got %q", def)
	}
}

func TestDefaultCatalog_GeminiDefault(t *testing.T) {
	c := DefaultCatalog()
	def := c.DefaultForProvider("gemini")
	if def != "gemini-3.5-flash" {
		t.Errorf("expected gemini-3.5-flash as gemini default, got %q", def)
	}
}

func TestDefaultCatalog_UpdatedProviderDefaults(t *testing.T) {
	c := DefaultCatalog()
	want := map[string]string{
		"bedrock": "anthropic.claude-sonnet-5",
		"kimi":    "kimi-k3",
		"minimax": "MiniMax-M3",
		"mistral": "mistral-large-2512",
		"vertex":  "claude-sonnet-5",
		"xai":     "grok-4.5",
		"zhipu":   "glm-5.2",
	}
	for provider, model := range want {
		if got := c.DefaultForProvider(provider); got != model {
			t.Errorf("%s default = %q, want %q", provider, got, model)
		}
	}
}

func TestDefaultCatalog_AllProvidersHaveDefault(t *testing.T) {
	c := DefaultCatalog()
	providers := map[string]bool{}
	for _, m := range c.All() {
		providers[m.Provider] = true
	}
	for p := range providers {
		def := c.DefaultForProvider(p)
		if def == "" {
			t.Errorf("provider %q has no default model", p)
		}
	}
}

func TestDefaultCatalog_KnownModelLookup(t *testing.T) {
	c := DefaultCatalog()
	tests := []struct {
		id       string
		provider string
	}{
		{"claude-fable-5", "anthropic"},
		{"claude-opus-4-8", "anthropic"},
		{"claude-sonnet-5", "anthropic"},
		{"claude-opus-4-7", "anthropic"},
		{"gpt-5.6-sol", "openai"},
		{"gpt-5.6-terra", "openai"},
		{"gpt-5.6-luna", "openai"},
		{"gpt-5.4", "openai"},
		{"gpt-5.4-mini", "openai"},
		{"gemini-3.5-flash", "gemini"},
		{"gemini-3.1-pro-preview", "gemini"},
		{"gemini-3.1-flash-lite", "gemini"},
		{"deepseek-v4-flash", "deepseek"},
		{"llama-3.3-70b-versatile", "groq"},
		{"mistral-medium-3-5", "mistral"},
		{"mistral-medium-2505", "mistral"},
		{"glm-5.2", "zhipu"},
		{"MiniMax-M3", "minimax"},
		{"kimi-k2.6", "kimi"},
		{"kimi-k2.7-code", "kimi"},
		{"llama3.1", "ollama"},
	}
	for _, tt := range tests {
		m, ok := c.Get(tt.id)
		if !ok {
			t.Errorf("expected to find model %q", tt.id)
			continue
		}
		if m.Provider != tt.provider {
			t.Errorf("model %q: expected provider %q, got %q", tt.id, tt.provider, m.Provider)
		}
	}
}

func TestDefaultCatalog_XAIOnlyIncludesLatestGrok(t *testing.T) {
	models := DefaultCatalog().ListByProvider("xai")
	if len(models) != 1 {
		t.Fatalf("xAI models = %d, want only grok-4.5", len(models))
	}
	m := models[0]
	if m.ID != "grok-4.5" || !m.IsDefault {
		t.Fatalf("xAI model = %+v, want default grok-4.5", m)
	}
	if m.ContextWindow != 500000 || m.MaxOutput != 0 {
		t.Fatalf("grok-4.5 limits = %d/%d, want 500000/unknown", m.ContextWindow, m.MaxOutput)
	}
	if !m.CanReason || !m.CanUseTools || !m.CanSeeImages {
		t.Fatalf("grok-4.5 capabilities = reasoning %v tools %v vision %v, want all true", m.CanReason, m.CanUseTools, m.CanSeeImages)
	}
	if m.CostPer1MIn != 0 || m.CacheReadPer1M != 0 || m.CostPer1MOut != 0 {
		t.Fatalf("grok-4.5 tiered pricing must remain unknown, got %.3f/%.3f/%.3f", m.CostPer1MIn, m.CacheReadPer1M, m.CostPer1MOut)
	}
	if m.APIFormat != "responses" {
		t.Fatalf("grok-4.5 API format = %q, want responses", m.APIFormat)
	}
	wantEfforts := []string{"low", "medium", "high", "xhigh"}
	if len(m.ReasoningEfforts) != len(wantEfforts) {
		t.Fatalf("grok-4.5 reasoning efforts = %#v, want %#v", m.ReasoningEfforts, wantEfforts)
	}
	for i := range wantEfforts {
		if m.ReasoningEfforts[i] != wantEfforts[i] {
			t.Fatalf("grok-4.5 reasoning efforts = %#v, want %#v", m.ReasoningEfforts, wantEfforts)
		}
	}
}

func TestDefaultCatalog_RemovedOpenAIModelsAreNotSelectable(t *testing.T) {
	c := DefaultCatalog()
	selectable := make(map[string]bool)
	for _, id := range c.ModelIDsByProvider("openai") {
		selectable[id] = true
	}
	for _, id := range []string{"gpt-5.2", "gpt-5.4-nano"} {
		if selectable[id] {
			t.Fatalf("removed OpenAI model %q remains selectable", id)
		}
		if _, ok := c.GetForProvider("openai", id); ok {
			t.Fatalf("removed OpenAI model %q remains an exact catalog entry", id)
		}
	}
}

func TestDefaultCatalog_DeepSeekV4FormalReleaseMetadata(t *testing.T) {
	c := DefaultCatalog()
	flash, ok := c.GetForProvider("deepseek", "deepseek-v4-flash")
	if !ok {
		t.Fatal("expected deepseek-v4-flash")
	}
	if flash.BillingCurrency() != "USD" {
		t.Fatalf("currency = %q, want USD", flash.BillingCurrency())
	}
	if flash.CostPer1MIn != 0.14 || flash.CacheReadPer1M != 0.0028 || flash.CostPer1MOut != 0.28 {
		t.Fatalf("flash pricing = in %.4f cache %.4f out %.4f, want 0.14/0.0028/0.28 USD", flash.CostPer1MIn, flash.CacheReadPer1M, flash.CostPer1MOut)
	}
	if flash.APIFormat != "responses" || flash.DefaultReasoningEffort != "high" {
		t.Fatalf("flash protocol/default effort = %q/%q, want responses/high", flash.APIFormat, flash.DefaultReasoningEffort)
	}
	wantEfforts := []string{"low", "high", "max"}
	if !slices.Equal(flash.ReasoningEfforts, wantEfforts) {
		t.Fatalf("flash reasoning efforts = %#v, want %#v", flash.ReasoningEfforts, wantEfforts)
	}

	pro, ok := c.GetForProvider("deepseek", "deepseek-v4-pro")
	if !ok {
		t.Fatal("expected deepseek-v4-pro")
	}
	if pro.BillingCurrency() != "USD" {
		t.Fatalf("currency = %q, want USD", pro.BillingCurrency())
	}
	if pro.CostPer1MIn != 0.435 || pro.CacheReadPer1M != 0.003625 || pro.CostPer1MOut != 0.87 {
		t.Fatalf("pro pricing = in %.6f cache %.6f out %.6f, want 0.435/0.003625/0.87 USD", pro.CostPer1MIn, pro.CacheReadPer1M, pro.CostPer1MOut)
	}
	if pro.APIFormat != "chat-completions" || pro.DefaultReasoningEffort != "high" {
		t.Fatalf("pro protocol/default effort = %q/%q, want chat-completions/high", pro.APIFormat, pro.DefaultReasoningEffort)
	}
}

func TestDefaultCatalog_OpenAIReasoningEfforts(t *testing.T) {
	c := DefaultCatalog()
	m, ok := c.GetForProvider("openai", "gpt-5.6-sol")
	if !ok {
		t.Fatal("expected gpt-5.6-sol")
	}
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if len(m.ReasoningEfforts) != len(want) {
		t.Fatalf("ReasoningEfforts = %#v, want %#v", m.ReasoningEfforts, want)
	}
	for i := range want {
		if m.ReasoningEfforts[i] != want[i] {
			t.Fatalf("ReasoningEfforts = %#v, want %#v", m.ReasoningEfforts, want)
		}
	}
}

func TestDefaultCatalog_OpenAICodexModelsUseVerifiedEffectiveContextWindows(t *testing.T) {
	c := DefaultCatalog()
	want := map[string]int{
		"gpt-5.6-sol":   353400,
		"gpt-5.6-terra": 353400,
		"gpt-5.6-luna":  353400,
		"gpt-5.5":       258400,
		"gpt-5.4":       258400,
		"gpt-5.4-mini":  258400,
	}
	for id, wantContext := range want {
		m, ok := c.GetForProvider("openai", id)
		if !ok {
			t.Errorf("expected %s", id)
			continue
		}
		if m.ContextWindow != wantContext {
			t.Errorf("%s context window = %d, want Codex effective window %d", id, m.ContextWindow, wantContext)
		}
	}
}

func TestDefaultCatalog_LatestOpenAIMetadata(t *testing.T) {
	c := DefaultCatalog()
	tests := []struct {
		id                                   string
		context                              int
		input, cacheWrite, cacheRead, output float64
	}{
		{"gpt-5.6-sol", 353400, 5.0, 6.25, 0.5, 30.0},
		{"gpt-5.6-terra", 353400, 2.5, 3.125, 0.25, 15.0},
		{"gpt-5.6-luna", 353400, 1.0, 1.25, 0.1, 6.0},
		{"gpt-5.4", 258400, 2.5, 0, 0.25, 15.0},
	}
	for _, tt := range tests {
		m, ok := c.GetForProvider("openai", tt.id)
		if !ok {
			t.Fatalf("expected %s", tt.id)
		}
		if m.ContextWindow != tt.context || m.MaxOutput != 128000 {
			t.Fatalf("%s limits = %d/%d, want %d/128000", tt.id, m.ContextWindow, m.MaxOutput, tt.context)
		}
		if !m.CanSeeImages || !m.CanReason {
			t.Fatalf("%s capabilities = vision %v reasoning %v, want both true", tt.id, m.CanSeeImages, m.CanReason)
		}
		if m.CostPer1MIn != tt.input || m.CacheCreatePer1M != tt.cacheWrite || m.CacheReadPer1M != tt.cacheRead || m.CostPer1MOut != tt.output {
			t.Fatalf("%s pricing = %.3f/%.3f/%.3f/%.3f, want %.3f/%.3f/%.3f/%.3f", tt.id, m.CostPer1MIn, m.CacheCreatePer1M, m.CacheReadPer1M, m.CostPer1MOut, tt.input, tt.cacheWrite, tt.cacheRead, tt.output)
		}
	}
	alias, ok := c.ResolveForProvider("openai", "gpt-5.6")
	if !ok || alias.ID != "gpt-5.6-sol" {
		t.Fatalf("gpt-5.6 provider identifier = %+v, found=%v; want gpt-5.6-sol", alias, ok)
	}

}

func TestDefaultCatalog_LatestAnthropicMetadata(t *testing.T) {
	c := DefaultCatalog()
	opus, ok := c.GetForProvider("anthropic", "claude-opus-4-8")
	if !ok {
		t.Fatal("expected claude-opus-4-8")
	}
	if opus.ContextWindow != 1000000 || opus.MaxOutput != 128000 {
		t.Fatalf("opus limits = %d/%d, want 1000000/128000", opus.ContextWindow, opus.MaxOutput)
	}
	if opus.CostPer1MIn != 5.0 || opus.CacheReadPer1M != 0.5 || opus.CostPer1MOut != 25.0 {
		t.Fatalf("opus pricing = in %.3f cache %.3f out %.3f, want 5.0/0.5/25.0 USD", opus.CostPer1MIn, opus.CacheReadPer1M, opus.CostPer1MOut)
	}

	sonnet, ok := c.GetForProvider("anthropic", "claude-sonnet-5")
	if !ok {
		t.Fatal("expected claude-sonnet-5")
	}
	if sonnet.ContextWindow != 1000000 || sonnet.MaxOutput != 128000 {
		t.Fatalf("sonnet limits = %d/%d, want 1000000/128000", sonnet.ContextWindow, sonnet.MaxOutput)
	}
	if sonnet.CostPer1MIn != 2.0 || sonnet.CacheReadPer1M != 0.2 || sonnet.CostPer1MOut != 10.0 {
		t.Fatalf("sonnet introductory pricing = %.3f/%.3f/%.3f, want 2.0/0.2/10.0", sonnet.CostPer1MIn, sonnet.CacheReadPer1M, sonnet.CostPer1MOut)
	}

	fable, ok := c.GetForProvider("anthropic", "claude-fable-5")
	if !ok || fable.CostPer1MIn != 10.0 || fable.CostPer1MOut != 50.0 {
		t.Fatalf("fable metadata = %+v, found=%v", fable, ok)
	}
}

func TestDefaultCatalog_LatestCrossProviderMetadata(t *testing.T) {
	c := DefaultCatalog()
	tests := []struct {
		provider, id                 string
		context, output              int
		input, cacheRead, outputCost float64
		currency                     string
	}{
		{"gemini", "gemini-3.5-flash", 1048576, 65536, 1.5, 0.15, 9.0, "USD"},
		{"zhipu", "glm-5.2", 1000000, 128000, 8.0, 2.0, 28.0, "CNY"},
		{"minimax", "MiniMax-M3", 1000000, 524288, 0.3, 0.06, 1.2, "USD"},
		{"kimi", "kimi-k3", 1000000, 0, 0, 0, 0, "USD"},
	}
	for _, tt := range tests {
		m, ok := c.GetForProvider(tt.provider, tt.id)
		if !ok {
			t.Fatalf("expected %s/%s", tt.provider, tt.id)
		}
		if m.ContextWindow != tt.context || m.MaxOutput != tt.output {
			t.Errorf("%s limits = %d/%d, want %d/%d", tt.id, m.ContextWindow, m.MaxOutput, tt.context, tt.output)
		}
		if m.CostPer1MIn != tt.input || m.CacheReadPer1M != tt.cacheRead || m.CostPer1MOut != tt.outputCost || m.BillingCurrency() != tt.currency {
			t.Errorf("%s pricing = %.3f/%.3f/%.3f %s, want %.3f/%.3f/%.3f %s", tt.id, m.CostPer1MIn, m.CacheReadPer1M, m.CostPer1MOut, m.BillingCurrency(), tt.input, tt.cacheRead, tt.outputCost, tt.currency)
		}
	}
}

func TestDefaultCatalog_CurrentCachedInputPricing(t *testing.T) {
	c := DefaultCatalog()
	tests := []struct {
		provider, id string
		cacheRead    float64
	}{
		{"groq", "openai/gpt-oss-120b", 0.075},
		{"groq", "openai/gpt-oss-20b", 0.0375},
		{"mistral", "mistral-large-2512", 0.05},
		{"mistral", "mistral-medium-3-5", 0.15},
		{"mistral", "mistral-small-2603", 0.015},
		{"mistral", "codestral-2508", 0.03},
	}
	for _, tt := range tests {
		m, ok := c.GetForProvider(tt.provider, tt.id)
		if !ok {
			t.Fatalf("expected %s/%s", tt.provider, tt.id)
		}
		if m.CacheReadPer1M != tt.cacheRead || m.CacheCreatePer1M != 0 || !m.CacheControl {
			t.Errorf("%s/%s cache pricing = read %.4f create %.4f enabled=%v, want %.4f/0/true", tt.provider, tt.id, m.CacheReadPer1M, m.CacheCreatePer1M, m.CacheControl, tt.cacheRead)
		}
	}
}

func TestDefaultCatalog_RetiredKimiModelsAreNotSelectable(t *testing.T) {
	c := DefaultCatalog()
	for _, id := range []string{"kimi-k2-0905-preview", "kimi-k2-thinking", "kimi-k2-thinking-turbo"} {
		if _, ok := c.GetForProvider("kimi", id); ok {
			t.Errorf("retired Kimi model %q should not be selectable", id)
		}
	}
}

func TestFormatContextWindowCommonLabels(t *testing.T) {
	tests := []struct {
		tokens int
		want   string
	}{
		{1048576, "1M"},
		{1050000, "1M"},
		{262144, "256K"},
		{204800, "200K"},
		{200001, "201K"},
	}
	for _, tt := range tests {
		if got := FormatContextWindow(tt.tokens); got != tt.want {
			t.Fatalf("FormatContextWindow(%d) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestDefaultCatalog_VertexUsesShortModelID(t *testing.T) {
	c := DefaultCatalog()
	m, ok := c.GetForProvider("vertex", "claude-sonnet-5")
	if !ok {
		t.Fatal("expected to find vertex claude-sonnet-5")
	}
	if m.Provider != "vertex" {
		t.Fatalf("provider = %q, want vertex", m.Provider)
	}
}

func TestModelCatalog_RegisterOverwrite(t *testing.T) {
	c := NewModelCatalog()
	c.Register(ModelInfo{CostCurrency: "USD", ID: "m1", Name: "Original", Provider: "test"})
	c.Register(ModelInfo{CostCurrency: "USD", ID: "m1", Name: "Updated", Provider: "test"})

	m, ok := c.Get("m1")
	if !ok {
		t.Fatal("expected to find m1")
	}
	if m.Name != "Updated" {
		t.Errorf("expected name Updated, got %q", m.Name)
	}
	if c.Count() != 1 {
		t.Errorf("expected 1 model after overwrite, got %d", c.Count())
	}
}
