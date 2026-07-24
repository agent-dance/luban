package provider

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ModelInfo describes a model's capabilities, pricing, and metadata.
// It consolidates data previously spread across modelContextWindows and cost/pricing.
type ModelInfo struct {
	// ID is the canonical model identifier (e.g. "claude-sonnet-4-20250514").
	ID string

	// Aliases lists accepted provider aliases that resolve to ID without being
	// shown as duplicate entries in model pickers.
	Aliases []string

	// Name is the human-readable display name (e.g. "Claude Sonnet 4").
	Name string

	// Provider is the provider that owns this model (e.g. "anthropic").
	Provider string

	// ContextWindow is the maximum input context window in tokens.
	ContextWindow int

	// MaxOutput is the maximum output tokens the model can generate.
	// 0 means unknown / provider-default.
	MaxOutput int

	// CostPer1MIn is the cost per 1M input tokens in the model's native billing currency.
	CostPer1MIn float64

	// CostPer1MOut is the cost per 1M output tokens in the model's native billing currency.
	CostPer1MOut float64

	// CacheReadPer1M is the cost per 1M cache-read tokens in the model's native billing currency. 0 = no caching.
	CacheReadPer1M float64

	// CacheCreatePer1M is the cost per 1M cache-creation tokens in the model's native billing currency. 0 = no caching.
	CacheCreatePer1M float64

	// CostCurrency is the ISO 4217 billing currency for all Cost* fields.
	// Empty means USD for backward compatibility with older catalog payloads.
	CostCurrency string

	// CanReason indicates the model supports extended thinking / reasoning.
	CanReason bool

	// ReasoningEfforts lists selectable reasoning effort tiers for providers
	// whose API supports a discrete effort parameter. Empty means no secondary
	// effort picker should be shown for this model.
	ReasoningEfforts []string

	// CanUseTools indicates the model supports function/tool calling.
	CanUseTools bool

	// CanSeeImages indicates the model supports vision / image inputs.
	CanSeeImages bool

	// CacheControl indicates the model supports prompt caching.
	CacheControl bool

	// APIFormat is the API protocol: "messages", "chat-completions", "responses".
	APIFormat string

	// IsDefault is true if this is the default model for its provider.
	IsDefault bool

	// ContextOverridden is true when ContextWindow was changed by user settings.
	ContextOverridden bool
}

// ModelCatalog is a registry of known models and their metadata.
// It is thread-safe for concurrent reads after initialization.
type ModelCatalog struct {
	mu     sync.RWMutex
	models map[string]ModelInfo
}

// NewModelCatalog creates an empty ModelCatalog.
func NewModelCatalog() *ModelCatalog {
	return &ModelCatalog{
		models: make(map[string]ModelInfo),
	}
}

func modelCatalogKey(provider, id string) string {
	return provider + "/" + id
}

func modelIdentifierMatches(m ModelInfo, id string) bool {
	if m.ID == id {
		return true
	}
	for _, alias := range m.Aliases {
		if alias == id {
			return true
		}
	}
	return false
}

func modelIdentifierPrefixMatchLength(m ModelInfo, id string) int {
	best := identifierPrefixMatchLength(m.ID, id)
	for _, alias := range m.Aliases {
		if matchLen := identifierPrefixMatchLength(alias, id); matchLen > best {
			best = matchLen
		}
	}
	return best
}

func identifierPrefixMatchLength(identifier, id string) int {
	if identifier == "" || !strings.HasPrefix(id, identifier) {
		return 0
	}
	if len(id) == len(identifier) || id[len(identifier)] == '-' || id[len(identifier)] == '.' {
		return len(identifier)
	}
	return 0
}

// Register adds or updates a model in the catalog.
func (c *ModelCatalog) Register(m ModelInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.models[modelCatalogKey(m.Provider, m.ID)] = m
}

// Get returns a model by its exact ID.
func (c *ModelCatalog) Get(id string) (ModelInfo, bool) {
	return c.Resolve(id)
}

// GetForProvider returns a model by provider + exact model ID.
func (c *ModelCatalog) GetForProvider(provider, id string) (ModelInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.models[modelCatalogKey(provider, id)]
	return m, ok
}

// Resolve looks up a model by exact ID first, then by longest prefix match.
// This handles versioned model names like "claude-sonnet-4-20250514" matching "claude-sonnet-4".
func (c *ModelCatalog) Resolve(id string) (ModelInfo, bool) {
	if provider, modelID, ok := strings.Cut(id, "/"); ok {
		return c.ResolveForProvider(provider, modelID)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	exact := make([]ModelInfo, 0, 1)
	for _, m := range c.models {
		if modelIdentifierMatches(m, id) {
			exact = append(exact, m)
		}
	}
	if len(exact) > 0 {
		sort.Slice(exact, func(i, j int) bool {
			if exact[i].Provider != exact[j].Provider {
				return exact[i].Provider < exact[j].Provider
			}
			return exact[i].ID < exact[j].ID
		})
		return exact[0], true
	}

	var best ModelInfo
	bestLen := 0
	for _, m := range c.models {
		if matchLen := modelIdentifierPrefixMatchLength(m, id); matchLen > bestLen {
			best = m
			bestLen = matchLen
		}
	}
	if bestLen > 0 {
		return best, true
	}

	return ModelInfo{}, false
}

// ResolveForProvider looks up a model within a specific provider, trying exact
// match first and then longest-prefix match.
func (c *ModelCatalog) ResolveForProvider(provider, id string) (ModelInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if m, ok := c.models[modelCatalogKey(provider, id)]; ok {
		return m, true
	}
	for _, m := range c.models {
		if m.Provider == provider && modelIdentifierMatches(m, id) {
			return m, true
		}
	}

	var best ModelInfo
	bestLen := 0
	for _, m := range c.models {
		if m.Provider != provider {
			continue
		}
		if matchLen := modelIdentifierPrefixMatchLength(m, id); matchLen > bestLen {
			best = m
			bestLen = matchLen
		}
	}
	if bestLen > 0 {
		return best, true
	}

	return ModelInfo{}, false
}

// ListByProvider returns all models for a given provider, sorted by ID.
func (c *ModelCatalog) ListByProvider(provider string) []ModelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []ModelInfo
	for _, m := range c.models {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// All returns every model in the catalog, sorted by provider then ID.
func (c *ModelCatalog) All() []ModelInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]ModelInfo, 0, len(c.models))
	for _, m := range c.models {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// DefaultForProvider returns the default model ID for a given provider.
func (c *ModelCatalog) DefaultForProvider(provider string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, m := range c.models {
		if m.Provider == provider && m.IsDefault {
			return m.ID
		}
	}
	return ""
}

// ModelIDsByProvider returns canonical model IDs for a provider.
func (c *ModelCatalog) ModelIDsByProvider(provider string) []string {
	models := c.ListByProvider(provider)
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids
}

// Count returns the total number of models in the catalog.
func (c *ModelCatalog) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.models)
}

// BillingCurrency returns the normalized billing currency for this model.
func (m ModelInfo) BillingCurrency() string {
	currency := strings.ToUpper(strings.TrimSpace(m.CostCurrency))
	if currency == "" {
		return "USD"
	}
	return currency
}

// CostCurrencySymbol returns a compact display symbol for a billing currency.
func CostCurrencySymbol(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "", "USD":
		return "$"
	case "CNY", "RMB":
		return "¥"
	default:
		return strings.ToUpper(strings.TrimSpace(currency)) + " "
	}
}

// FormatContextWindow formats a token window using common rounded labels.
// It intentionally maps binary/token-exact limits like 1,048,576 and 262,144
// to user-facing labels such as 1M and 256K, matching provider documentation.
func FormatContextWindow(tokens int) string {
	if tokens <= 0 {
		return ""
	}
	common := map[int]string{
		1_050_000: "1M",
		1_048_576: "1M",
		1_000_000: "1M",
		400_000:   "400K",
		393_216:   "384K",
		384_000:   "384K",
		262_144:   "256K",
		256_000:   "256K",
		204_800:   "200K",
		200_000:   "200K",
		131_072:   "128K",
		128_000:   "128K",
		65_536:    "64K",
		64_000:    "64K",
		32_768:    "32K",
		32_000:    "32K",
		16_384:    "16K",
		16_000:    "16K",
		8_192:     "8K",
		8_000:     "8K",
	}
	if label, ok := common[tokens]; ok {
		return label
	}
	if tokens >= 1_000_000 {
		tenths := (tokens + 99_999) / 100_000
		if tenths%10 == 0 {
			return fmt.Sprintf("%dM", tenths/10)
		}
		return fmt.Sprintf("%d.%dM", tenths/10, tenths%10)
	}
	return fmt.Sprintf("%dK", (tokens+999)/1000)
}

// CatalogDefaultModel returns the catalog default model for a provider, falling
// back to the supplied value when the provider has no registered default.
func CatalogDefaultModel(provider, fallback string) string {
	if def := DefaultCatalog().DefaultForProvider(provider); def != "" {
		return def
	}
	return fallback
}

//go:generate python3 scripts/catalog_sync.py

// DefaultCatalog returns a ModelCatalog pre-populated with generated provider
// metadata plus local-only model aliases.
func DefaultCatalog() *ModelCatalog {
	c := NewModelCatalog()
	registerGeneratedCatalog(c)
	registerLocalCatalog(c)
	c.ApplyOverrides(RuntimeModelOverrides())
	return c
}

func registerLocalCatalog(c *ModelCatalog) {
	c.Register(ModelInfo{
		ID: "llama3.1", Name: "Llama 3.1",
		Provider: "ollama", ContextWindow: 131072,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions", IsDefault: true,
	})
	c.Register(ModelInfo{
		ID: "llama3.2", Name: "Llama 3.2",
		Provider: "ollama", ContextWindow: 131072,
		CanReason: false, CanUseTools: true, CanSeeImages: true,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "llama3.3", Name: "Llama 3.3",
		Provider: "ollama", ContextWindow: 131072,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "qwen2.5-coder", Name: "Qwen 2.5 Coder",
		Provider: "ollama", ContextWindow: 32768,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "qwen2.5", Name: "Qwen 2.5",
		Provider: "ollama", ContextWindow: 32768,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "codellama", Name: "Code Llama",
		Provider: "ollama", ContextWindow: 16384,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "phi4", Name: "Phi-4",
		Provider: "ollama", ContextWindow: 16384,
		CanReason: false, CanUseTools: true, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
	c.Register(ModelInfo{
		ID: "deepseek-r1", Name: "DeepSeek R1 (local)",
		Provider: "ollama", ContextWindow: 64000,
		CanReason: true, CanUseTools: false, CanSeeImages: false,
		APIFormat: "chat-completions",
	})
}
