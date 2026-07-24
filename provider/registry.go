package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// ProviderInfo describes a registered provider's metadata and capabilities.
type ProviderInfo struct {
	// Name is the registered provider identifier (e.g. "anthropic", "openai").
	Name string

	// DisplayName is the human-readable name (e.g. "Anthropic", "OpenAI").
	DisplayName string

	// EnvKey is the primary environment variable for authentication (e.g. "ANTHROPIC_API_KEY").
	// Empty for providers that don't use an API key (e.g. "ollama").
	EnvKey string

	// Models lists the canonical model IDs available for this provider.
	Models []string

	// DefaultModel is the model used when none is specified.
	DefaultModel string

	// AuthMethods describes supported authentication methods.
	// Values: "api_key", "oauth_pkce", "device_code", "aws_credentials", "gcp_adc"
	AuthMethods []string

	// Hidden marks compatibility/internal aliases that remain resolvable but
	// are not shown in user-facing provider pickers.
	Hidden bool

	// Popularity is a sorting weight (higher = more popular, shown first).
	Popularity int

	// RequiresContext indicates the factory needs a context.Context (e.g. Bedrock, Vertex).
	RequiresContext bool

	// DefaultBaseURL is the default API base URL for this provider.
	// Empty means the provider does not have a well-known default or uses
	// the provider-specific SDK default.
	DefaultBaseURL string
}

// CanonicalProviderName maps legacy/internal provider aliases to the provider
// identity users should see in settings, commands, and picker labels.
func CanonicalProviderName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "openai-responses":
		return "openai"
	case "oauth":
		return "anthropic"
	default:
		return normalized
	}
}

// CredentialLookupNames returns credential-store keys that should be consulted
// for a provider, with the canonical key first and legacy aliases afterward.
func CredentialLookupNames(providerName string) []string {
	canonical := CanonicalProviderName(providerName)
	switch canonical {
	case "":
		return nil
	case "openai":
		return []string{"openai", "openai-responses"}
	case "anthropic":
		return []string{"anthropic", "oauth"}
	default:
		return []string{canonical}
	}
}

// ProviderFactory creates a Provider from a Config and optional model override.
// The context is provided for factories that need it (e.g. AWS credential chain).
type ProviderFactory func(cfg Config, modelOverride string) (Provider, error)

// OAuthHook is an optional interface for integrating OAuth credential
// management without creating an import cycle between provider and auth.
// The hook is set from outside the provider package (e.g., main.go or
// a bootstrap function) and used by the OAuth provider factory.
type OAuthHook interface {
	// LoadAccessToken returns a valid access token, refreshing if needed.
	// Returns ("", nil) if no credentials are stored.
	LoadAccessToken(ctx context.Context) (string, error)

	// OnAuthError is called when a 401 is received. Returns true if
	// credentials were successfully refreshed and a retry is warranted.
	OnAuthError() bool
}

// ProviderRegistry is a central registry of available providers and their factories.
// It maintains a catalog of all known models across all providers.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]ProviderInfo
	factories map[string]ProviderFactory
	catalog   *ModelCatalog
	credStore *CredentialStore // optional; checked by Available()/hasCredentials()
	oauthHook OAuthHook        // optional; used by OAuth provider factory
}

// NewProviderRegistry creates an empty ProviderRegistry with a new ModelCatalog.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ProviderInfo),
		factories: make(map[string]ProviderFactory),
		catalog:   NewModelCatalog(),
	}
}

// Register adds a provider with its factory to the registry.
func (r *ProviderRegistry) Register(info ProviderInfo, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.catalog != nil {
		if modelIDs := r.catalog.ModelIDsByProvider(info.Name); len(modelIDs) > 0 {
			info.Models = modelIDs
		}
		if def := r.catalog.DefaultForProvider(info.Name); def != "" {
			info.DefaultModel = def
		}
	}
	r.providers[info.Name] = info
	r.factories[info.Name] = factory
}

// Get returns the ProviderInfo for a given name.
func (r *ProviderRegistry) Get(name string) (ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.providers[name]
	return info, ok
}

// Create instantiates a Provider by name using the registered factory.
func (r *ProviderRegistry) Create(name string, cfg Config, modelOverride string) (Provider, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderUnknown, name, strings.Join(r.VisibleNames(), ", ")))
	}

	return factory(cfg, modelOverride)
}

// All returns every registered ProviderInfo, sorted by popularity (descending) then name.
func (r *ProviderRegistry) All() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ProviderInfo, 0, len(r.providers))
	for _, info := range r.providers {
		result = append(result, info)
	}
	sortProviderInfos(result)
	return result
}

// Visible returns registered ProviderInfo entries intended for user-facing
// provider selection, sorted by popularity (descending) then name.
func (r *ProviderRegistry) Visible() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ProviderInfo, 0, len(r.providers))
	for _, info := range r.providers {
		if !info.Hidden {
			result = append(result, info)
		}
	}
	sortProviderInfos(result)
	return result
}

// Available returns providers that can be selected for model use.
// Prefer ConnectionState when UI or commands need to distinguish configured,
// local, and setup-required providers.
func (r *ProviderRegistry) Available() []ProviderInfo {
	all := r.Visible()
	var result []ProviderInfo
	for _, info := range all {
		if r.hasCredentials(info) {
			result = append(result, info)
		}
	}
	return result
}

// SetCredentialStore attaches a CredentialStore to the registry.
// When set, Available() and hasCredentials() also check the store.
func (r *ProviderRegistry) SetCredentialStore(cs *CredentialStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.credStore = cs
}

// CredentialStore returns the attached CredentialStore, or nil.
func (r *ProviderRegistry) CredentialStoreRef() *CredentialStore {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.credStore
}

// SetOAuthHook attaches an OAuthHook to the registry.
// The hook is used by the OAuth provider factory for token management.
func (r *ProviderRegistry) SetOAuthHook(hook OAuthHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.oauthHook = hook
}

// OAuthHookRef returns the attached OAuthHook, or nil.
func (r *ProviderRegistry) OAuthHookRef() OAuthHook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.oauthHook
}

// hasCredentials is the compatibility gate behind Available().
// It now means "model-selectable", not "has an API key".
func (r *ProviderRegistry) hasCredentials(info ProviderInfo) bool {
	return r.connectionStateForInfo(info).CanSelectModels
}

// Names returns all registered provider names, sorted alphabetically.
func (r *ProviderRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// VisibleNames returns non-hidden registered provider names, sorted alphabetically.
func (r *ProviderRegistry) VisibleNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name, info := range r.providers {
		if !info.Hidden {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func sortProviderInfos(result []ProviderInfo) {
	sort.Slice(result, func(i, j int) bool {
		if result[i].Popularity != result[j].Popularity {
			return result[i].Popularity > result[j].Popularity
		}
		return result[i].Name < result[j].Name
	})
}

// Catalog returns the ModelCatalog associated with this registry.
func (r *ProviderRegistry) Catalog() *ModelCatalog {
	return r.catalog
}

func (r *ProviderRegistry) ApplyModelOverrides(overrides ModelOverrides) {
	if r == nil || r.catalog == nil {
		return
	}
	r.catalog.ApplyOverrides(overrides)
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, info := range r.providers {
		if modelIDs := r.catalog.ModelIDsByProvider(name); len(modelIDs) > 0 {
			info.Models = modelIDs
		}
		if def := r.catalog.DefaultForProvider(name); def != "" {
			info.DefaultModel = def
		}
		r.providers[name] = info
	}
}

// ── Singleton ─────────────────────────────────────────────────────────────────

var (
	defaultRegistryOnce sync.Once
	defaultRegistryInst *ProviderRegistry
)

// DefaultRegistry returns the singleton ProviderRegistry pre-populated with
// all built-in providers. The registry is lazily initialized on first call.
func DefaultRegistry() *ProviderRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistryInst = NewProviderRegistry()
		defaultRegistryInst.catalog = DefaultCatalog()
		registerBuiltinProviders(defaultRegistryInst)
	})
	return defaultRegistryInst
}
