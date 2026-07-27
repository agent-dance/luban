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

	// Hidden marks internal providers omitted from user-facing pickers.
	Hidden bool

	// Popularity is a sorting weight (higher = more popular, shown first).
	Popularity int

	// RequiresContext indicates the factory needs a context.Context (e.g. Bedrock, Vertex).
	RequiresContext bool

	// DefaultBaseURL is the default API base URL for this provider.
	// Empty means the provider does not have a well-known default or uses
	// the provider-specific SDK default.
	DefaultBaseURL string

	// APIStyles lists the wire protocols offered by a compatible aggregate
	// provider. Empty means the provider uses its native, fixed protocol.
	APIStyles []APIStyle

	// DefaultBaseURLs maps each compatible protocol to its default endpoint.
	// It is populated for named aggregate providers and empty for user-defined
	// gateways, which must supply their own Base URL.
	DefaultBaseURLs map[APIStyle]string

	// DynamicModels marks providers whose model catalog is discovered from the
	// configured endpoint after authentication.
	DynamicModels bool

	// UserDefined marks providers created through the "other gateway" flow.
	UserDefined bool
}

// CanonicalProviderName normalizes a provider identifier.
func CanonicalProviderName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// CredentialLookupNames returns the canonical credential-store key.
func CredentialLookupNames(providerName string) []string {
	canonical := CanonicalProviderName(providerName)
	if canonical == "" {
		return nil
	}
	return []string{canonical}
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
	credStore *CredentialStore // optional; checked by Available()
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
	if normalizeCapabilitySupport(cfg.ResponsesWebSocket) == CapabilitySupported && CanonicalProviderName(name) != "openai" {
		return nil, responsesWebSocketProfileUnsupportedError()
	}
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderUnknown, name, strings.Join(r.VisibleNames(), ", ")))
	}

	created, err := factory(cfg, modelOverride)
	if err != nil {
		return nil, err
	}
	if normalizeCapabilitySupport(cfg.ResponsesWebSocket) == CapabilitySupported {
		capable, ok := created.(CapabilityProvider)
		if !ok || capable.Capabilities().ResponsesWebSocket != CapabilitySupported {
			if closer, closeOK := created.(CloseProvider); closeOK {
				_ = closer.Close()
			}
			return nil, responsesWebSocketProfileUnsupportedError()
		}
	}
	return created, nil
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
		if r.isModelSelectable(info) {
			result = append(result, info)
		}
	}
	return result
}

// SetCredentialStore attaches a CredentialStore to the registry.
// When set, Available() also checks the store.
func (r *ProviderRegistry) SetCredentialStore(cs *CredentialStore) {
	r.mu.Lock()
	r.credStore = cs
	r.mu.Unlock()

	if cs == nil {
		return
	}
	for _, entry := range cs.All() {
		if entry.UserDefined {
			r.RegisterCompatibleProvider(CompatibleProviderDefinition{
				Name:        entry.Provider,
				DisplayName: CompatibleProviderDisplayName(entry.DisplayName, entry.BaseURL),
				BaseURLs:    map[APIStyle]string{},
				UserDefined: true,
			})
		}
		if len(entry.Models) > 0 {
			r.ReplaceProviderModels(entry.Provider, entry.Models)
		}
	}
}

// ReplaceProviderModels refreshes the shared catalog and the corresponding
// ProviderInfo snapshot as one registry-level operation.
func (r *ProviderRegistry) ReplaceProviderModels(providerName string, models []ModelInfo) {
	providerName = CanonicalProviderName(providerName)
	r.catalog.ReplaceProviderModels(providerName, models)
	r.mu.Lock()
	defer r.mu.Unlock()
	info, ok := r.providers[providerName]
	if !ok {
		return
	}
	info.Models = r.catalog.ModelIDsByProvider(providerName)
	info.DefaultModel = r.catalog.DefaultForProvider(providerName)
	if info.DefaultModel == "" && len(info.Models) > 0 {
		info.DefaultModel = info.Models[0]
	}
	r.providers[providerName] = info
}

// UnregisterUserProvider removes a user-created provider and its discovered
// models. Built-in providers cannot be removed through this method.
func (r *ProviderRegistry) UnregisterUserProvider(name string) bool {
	name = CanonicalProviderName(name)
	r.mu.Lock()
	info, ok := r.providers[name]
	if !ok || !info.UserDefined {
		r.mu.Unlock()
		return false
	}
	delete(r.providers, name)
	delete(r.factories, name)
	r.mu.Unlock()
	r.catalog.RemoveProvider(name)
	return true
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

func (r *ProviderRegistry) isModelSelectable(info ProviderInfo) bool {
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
