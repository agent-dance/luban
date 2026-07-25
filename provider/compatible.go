package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/i18n"
)

// APIStyle identifies the compatibility protocol exposed by an aggregate
// provider or gateway.
type APIStyle string

const (
	APIStyleOpenAI    APIStyle = "openai"
	APIStyleAnthropic APIStyle = "anthropic"
)

// ParseAPIStyle normalizes persisted and user-supplied protocol names. Unknown
// values intentionally fall back to OpenAI, the default in the setup flow.
func ParseAPIStyle(value string) APIStyle {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(APIStyleAnthropic):
		return APIStyleAnthropic
	default:
		return APIStyleOpenAI
	}
}

// DisplayName returns the protocol's stable product name. Protocol and brand
// names intentionally are not translated.
func (style APIStyle) DisplayName() string {
	if ParseAPIStyle(string(style)) == APIStyleAnthropic {
		return "Anthropic"
	}
	return "OpenAI"
}

// CompatibleProviderDefinition describes a multi-model endpoint implemented
// through one of the shared compatibility protocols.
type CompatibleProviderDefinition struct {
	Name        string
	DisplayName string
	Popularity  int
	BaseURLs    map[APIStyle]string
	UserDefined bool
}

// BaseURLForStyle returns the protocol-specific default URL, falling back to
// the legacy single default for older provider definitions.
func (info ProviderInfo) BaseURLForStyle(style APIStyle) string {
	style = ParseAPIStyle(string(style))
	if value := strings.TrimSpace(info.DefaultBaseURLs[style]); value != "" {
		return value
	}
	return strings.TrimSpace(info.DefaultBaseURL)
}

// RegisterCompatibleProvider installs a shared OpenAI/Anthropic-compatible
// factory. Named cloud plans and user-created gateways use this same path.
func (r *ProviderRegistry) RegisterCompatibleProvider(def CompatibleProviderDefinition) {
	name := CanonicalProviderName(def.Name)
	if name == "" {
		return
	}
	displayName := strings.TrimSpace(def.DisplayName)
	if displayName == "" {
		displayName = name
	}
	baseURLs := cloneCompatibleBaseURLs(def.BaseURLs)
	info := ProviderInfo{
		Name:            name,
		DisplayName:     displayName,
		AuthMethods:     []string{"api_key"},
		Popularity:      def.Popularity,
		DefaultBaseURL:  baseURLs[APIStyleOpenAI],
		APIStyles:       []APIStyle{APIStyleOpenAI, APIStyleAnthropic},
		DefaultBaseURLs: baseURLs,
		DynamicModels:   true,
		UserDefined:     def.UserDefined,
	}
	r.Register(info, func(cfg Config, modelOverride string) (Provider, error) {
		style := ParseAPIStyle(string(cfg.APIStyle))
		apiKey := strings.TrimSpace(cfg.APIKey)
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" {
			baseURL = baseURLs[style]
		}
		model := strings.TrimSpace(modelOverride)
		if model == "" {
			model = strings.TrimSpace(cfg.Model)
		}
		if model == "" {
			models := r.catalog.ListByProvider(name)
			if len(models) > 0 {
				model = models[0].ID
			}
		}
		if apiKey == "" {
			return NewUnconfiguredProvider(name, model, "API_KEY", ""), nil
		}
		if baseURL == "" {
			return nil, i18n.NewError(i18n.KeyProviderCompatibleBaseURLRequired)
		}

		if style == APIStyleAnthropic {
			// Compatible gateways vary between standard X-Api-Key and Claude
			// Code's bearer-token convention. Supplying both preserves the
			// Anthropic header while interoperating with the named coding plans.
			raw := NewAnthropic(Config{
				ProviderName: name,
				APIKey:       apiKey,
				BaseURL:      baseURL,
				Model:        model,
				Headers:      mergeHeaders(cfg.Headers, map[string]string{"Authorization": "Bearer " + apiKey}),
			})
			return NewRetryProvider(raw, DefaultRetryConfig()), nil
		}

		raw := NewOpenAI(Config{
			ProviderName:           name,
			APIKey:                 apiKey,
			BaseURL:                normalizeOpenAIChatBaseURL(baseURL),
			Model:                  model,
			Headers:                cloneHeaders(cfg.Headers),
			DisableStrictTools:     true,
			CacheRoutingPreference: cfg.CacheRoutingPreference,
		})
		return NewRetryProvider(raw, DefaultRetryConfig()), nil
	})
}

func cloneCompatibleBaseURLs(values map[APIStyle]string) map[APIStyle]string {
	result := make(map[APIStyle]string, len(values))
	for style, value := range values {
		result[ParseAPIStyle(string(style))] = strings.TrimSpace(value)
	}
	return result
}

// NextUserProviderName creates a collision-free canonical identifier. The
// display name remains independent and can contain the user's original text.
func (r *ProviderRegistry) NextUserProviderName(displayName, baseURL string) string {
	base := customProviderIdentifier(displayName, baseURL)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, exists := r.providers[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := r.providers[candidate]; !exists {
			return candidate
		}
	}
}

// CompatibleProviderDisplayName applies the requested hostname fallback for a
// user-created gateway.
func CompatibleProviderDisplayName(displayName, baseURL string) string {
	if displayName = strings.TrimSpace(displayName); displayName != "" {
		return displayName
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return strings.TrimSpace(baseURL)
}

func customProviderIdentifier(displayName, baseURL string) string {
	source := CompatibleProviderDisplayName(displayName, baseURL)
	var slug strings.Builder
	separator := false
	for _, char := range strings.ToLower(source) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '.' {
			if separator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(char)
			separator = false
		} else {
			separator = true
		}
	}
	value := strings.Trim(slug.String(), "-.")
	if value == "" {
		value = "gateway"
	}
	return "custom-" + value
}

// CompatibleModelRequest contains the endpoint inputs needed for authenticated
// model discovery.
type CompatibleModelRequest struct {
	Provider string
	APIStyle APIStyle
	BaseURL  string
	APIKey   string
}

// DiscoverCompatibleModels lists models from a compatible endpoint and fills
// missing metadata from normalized matches in the built-in catalog.
func (r *ProviderRegistry) DiscoverCompatibleModels(ctx context.Context, input CompatibleModelRequest) ([]ModelInfo, error) {
	if r == nil || r.catalog == nil {
		return nil, i18n.NewError(i18n.KeyProviderCompatibleCatalogUnavailable)
	}
	excludedProviders := make(map[string]struct{})
	r.mu.RLock()
	for name, info := range r.providers {
		if info.DynamicModels {
			excludedProviders[CanonicalProviderName(name)] = struct{}{}
		}
	}
	r.mu.RUnlock()
	excludedProviders[CanonicalProviderName(input.Provider)] = struct{}{}
	return discoverCompatibleModels(ctx, r.catalog, input, excludedProviders)
}

type discoveredModel struct {
	info ModelInfo

	nameSet          bool
	contextSet       bool
	maxOutputSet     bool
	costInSet        bool
	costOutSet       bool
	cacheReadSet     bool
	cacheCreateSet   bool
	currencySet      bool
	reasoningSet     bool
	reasoningListSet bool
	toolsSet         bool
	visionSet        bool
	cacheControlSet  bool
	defaultSet       bool
}

func discoverCompatibleModels(ctx context.Context, catalog *ModelCatalog, input CompatibleModelRequest, excludedProviders map[string]struct{}) ([]ModelInfo, error) {
	style := ParseAPIStyle(string(input.APIStyle))
	endpoint, err := compatibleModelsURL(input.BaseURL, style)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderCompatibleModelsRequestBuildFailed, err)
	}
	apiKey := strings.TrimSpace(input.APIKey)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		if style == APIStyleAnthropic {
			req.Header.Set("X-Api-Key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderCompatibleModelsRequestFailed, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderCompatibleModelsReadFailed, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, i18n.NewError(i18n.KeyProviderCompatibleModelsHTTPFailed, response.StatusCode, compatibleHTTPErrorDetail(body))
	}

	remote, err := decodeDiscoveredModels(body)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderCompatibleModelsDecodeFailed, err)
	}
	if len(remote) == 0 {
		return nil, i18n.NewError(i18n.KeyProviderCompatibleModelsEmpty)
	}

	providerName := CanonicalProviderName(input.Provider)
	models := make([]ModelInfo, 0, len(remote))
	seen := make(map[string]struct{}, len(remote))
	for _, item := range remote {
		id := strings.TrimSpace(item.info.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, enrichDiscoveredModel(catalog, providerName, style, item, excludedProviders))
	}
	if len(models) == 0 {
		return nil, i18n.NewError(i18n.KeyProviderCompatibleModelsEmpty)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func compatibleModelsURL(baseURL string, style APIStyle) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", i18n.NewError(i18n.KeyProviderCompatibleBaseURLInvalid)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if style == APIStyleAnthropic && !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	if !strings.HasSuffix(path, "/models") {
		path += "/models"
	}
	parsed.Path = path
	parsed.RawQuery = ""
	if style == APIStyleAnthropic {
		query := parsed.Query()
		query.Set("limit", "1000")
		parsed.RawQuery = query.Encode()
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func compatibleHTTPErrorDetail(body []byte) string {
	detail := []rune(strings.TrimSpace(string(body)))
	const maxRunes = 2048
	if len(detail) > maxRunes {
		detail = detail[:maxRunes]
	}
	return string(detail)
}

func decodeDiscoveredModels(body []byte) ([]discoveredModel, error) {
	var envelope struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		var direct []json.RawMessage
		if directErr := json.Unmarshal(body, &direct); directErr != nil {
			return nil, err
		}
		envelope.Data = direct
	}
	items := envelope.Data
	if len(items) == 0 {
		items = envelope.Models
	}
	result := make([]discoveredModel, 0, len(items))
	for _, raw := range items {
		var values map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			var id string
			if stringErr := json.Unmarshal(raw, &id); stringErr != nil || strings.TrimSpace(id) == "" {
				return nil, err
			}
			result = append(result, discoveredModel{info: ModelInfo{ID: strings.TrimSpace(id)}})
			continue
		}
		item := parseDiscoveredModel(values)
		if item.info.ID != "" {
			result = append(result, item)
		}
	}
	return result, nil
}

func parseDiscoveredModel(values map[string]any) discoveredModel {
	var result discoveredModel
	result.info.ID, _ = stringValue(values, "id")
	if result.info.ID == "" {
		result.info.ID, _ = stringValue(values, "model")
	}
	if name, ok := firstStringValue(values, "display_name", "displayName", "name"); ok && name != result.info.ID {
		result.info.Name, result.nameSet = name, true
	}
	result.info.ContextWindow, result.contextSet = firstIntValue(values,
		"context_window", "context_length", "max_context_length", "max_input_tokens")
	result.info.MaxOutput, result.maxOutputSet = firstIntValue(values, "max_output_tokens", "max_tokens")

	result.info.CostPer1MIn, result.costInSet = firstFloatValue(values,
		"cost_per_1m_in", "input_cost_per_million", "input_price_per_million")
	result.info.CostPer1MOut, result.costOutSet = firstFloatValue(values,
		"cost_per_1m_out", "output_cost_per_million", "output_price_per_million")
	result.info.CacheReadPer1M, result.cacheReadSet = firstFloatValue(values,
		"cache_read_per_1m", "cache_read_price_per_million")
	result.info.CacheCreatePer1M, result.cacheCreateSet = firstFloatValue(values,
		"cache_create_per_1m", "cache_write_price_per_million")
	if pricing, ok := mapValue(values, "pricing"); ok {
		if !result.costInSet {
			if value, found := firstFloatValue(pricing, "input_per_million", "prompt_per_million"); found {
				result.info.CostPer1MIn, result.costInSet = value, true
			} else if value, found := firstFloatValue(pricing, "input", "prompt"); found {
				result.info.CostPer1MIn, result.costInSet = value*1_000_000, true
			}
		}
		if !result.costOutSet {
			if value, found := firstFloatValue(pricing, "output_per_million", "completion_per_million"); found {
				result.info.CostPer1MOut, result.costOutSet = value, true
			} else if value, found := firstFloatValue(pricing, "output", "completion"); found {
				result.info.CostPer1MOut, result.costOutSet = value*1_000_000, true
			}
		}
		if !result.cacheReadSet {
			if value, found := firstFloatValue(pricing, "cache_read_per_million", "input_cache_read_per_million"); found {
				result.info.CacheReadPer1M, result.cacheReadSet = value, true
			} else if value, found := firstFloatValue(pricing, "cache_read", "input_cache_read"); found {
				result.info.CacheReadPer1M, result.cacheReadSet = value*1_000_000, true
			}
		}
		if !result.cacheCreateSet {
			if value, found := firstFloatValue(pricing, "cache_write_per_million", "input_cache_write_per_million"); found {
				result.info.CacheCreatePer1M, result.cacheCreateSet = value, true
			} else if value, found := firstFloatValue(pricing, "cache_write", "input_cache_write"); found {
				result.info.CacheCreatePer1M, result.cacheCreateSet = value*1_000_000, true
			}
		}
		if !result.currencySet {
			result.info.CostCurrency, result.currencySet = firstStringValue(pricing, "currency")
		}
	}
	if currency, ok := firstStringValue(values, "cost_currency", "currency"); ok {
		result.info.CostCurrency, result.currencySet = currency, true
	}

	result.info.CanReason, result.reasoningSet = firstBoolValue(values,
		"can_reason", "supports_reasoning", "reasoning", "thinking")
	result.info.CanUseTools, result.toolsSet = firstBoolValue(values,
		"can_use_tools", "supports_tools", "tool_use", "function_calling")
	result.info.CanSeeImages, result.visionSet = firstBoolValue(values,
		"can_see_images", "supports_vision", "vision", "multimodal")
	result.info.CacheControl, result.cacheControlSet = firstBoolValue(values,
		"cache_control", "supports_prompt_cache", "supports_caching")
	result.info.IsDefault, result.defaultSet = firstBoolValue(values, "is_default", "default")
	result.info.ReasoningEfforts, result.reasoningListSet = firstStringSliceValue(values,
		"reasoning_efforts", "supported_reasoning_efforts")

	if capabilities, ok := mapValue(values, "capabilities"); ok {
		if !result.reasoningSet {
			result.info.CanReason, result.reasoningSet = firstBoolValue(capabilities, "reasoning", "thinking")
		}
		if !result.toolsSet {
			result.info.CanUseTools, result.toolsSet = firstBoolValue(capabilities, "tool_use", "tools")
		}
		if !result.visionSet {
			result.info.CanSeeImages, result.visionSet = firstBoolValue(capabilities, "vision", "image_input")
		}
	}
	if topProvider, ok := mapValue(values, "top_provider"); ok && !result.maxOutputSet {
		result.info.MaxOutput, result.maxOutputSet = firstIntValue(topProvider, "max_completion_tokens", "max_output_tokens")
	}
	if architecture, ok := mapValue(values, "architecture"); ok {
		if modalities, found := firstStringSliceValue(architecture, "input_modalities", "modalities"); found {
			applyInputModalities(&result, modalities)
		}
	}
	if modalities, ok := firstStringSliceValue(values, "input_modalities", "modalities"); ok {
		applyInputModalities(&result, modalities)
	}
	if parameters, ok := firstStringSliceValue(values, "supported_parameters"); ok {
		for _, parameter := range parameters {
			switch strings.ToLower(strings.TrimSpace(parameter)) {
			case "tools", "tool_choice", "function_calling":
				result.info.CanUseTools, result.toolsSet = true, true
			case "reasoning", "reasoning_effort", "include_reasoning", "thinking":
				result.info.CanReason, result.reasoningSet = true, true
			}
		}
	}
	return result
}

func applyInputModalities(result *discoveredModel, modalities []string) {
	for _, modality := range modalities {
		if strings.EqualFold(strings.TrimSpace(modality), "image") {
			result.info.CanSeeImages, result.visionSet = true, true
			return
		}
	}
}

func enrichDiscoveredModel(catalog *ModelCatalog, providerName string, style APIStyle, remote discoveredModel, excludedProviders map[string]struct{}) ModelInfo {
	result := ModelInfo{CostCurrency: "USD"}
	if builtin, ok := catalog.resolveNormalizedBuiltin(remote.info.ID, excludedProviders); ok {
		result = builtin
	}
	result.ID = remote.info.ID
	result.Provider = providerName
	result.Aliases = nil
	result.ContextOverridden = false
	result.IsDefault = false

	if remote.nameSet {
		result.Name = remote.info.Name
	}
	if result.Name == "" {
		result.Name = result.ID
	}
	if remote.contextSet {
		result.ContextWindow = remote.info.ContextWindow
	}
	if remote.maxOutputSet {
		result.MaxOutput = remote.info.MaxOutput
	}
	if remote.costInSet {
		result.CostPer1MIn = remote.info.CostPer1MIn
	}
	if remote.costOutSet {
		result.CostPer1MOut = remote.info.CostPer1MOut
	}
	if remote.cacheReadSet {
		result.CacheReadPer1M = remote.info.CacheReadPer1M
	}
	if remote.cacheCreateSet {
		result.CacheCreatePer1M = remote.info.CacheCreatePer1M
	}
	if remote.currencySet {
		result.CostCurrency = remote.info.CostCurrency
	}
	if strings.TrimSpace(result.CostCurrency) == "" {
		result.CostCurrency = "USD"
	}
	if remote.reasoningSet {
		result.CanReason = remote.info.CanReason
		if !result.CanReason && !remote.reasoningListSet {
			result.ReasoningEfforts = nil
		}
	}
	if remote.reasoningListSet {
		result.ReasoningEfforts = append([]string(nil), remote.info.ReasoningEfforts...)
	}
	if remote.toolsSet {
		result.CanUseTools = remote.info.CanUseTools
	}
	if remote.visionSet {
		result.CanSeeImages = remote.info.CanSeeImages
	}
	if remote.cacheControlSet {
		result.CacheControl = remote.info.CacheControl
	}
	if remote.defaultSet {
		result.IsDefault = remote.info.IsDefault
	}
	if style == APIStyleAnthropic {
		result.APIFormat = "messages"
	} else {
		result.APIFormat = "chat-completions"
	}
	return result
}

// NormalizeModelIdentifier implements the gateway matching rule: comparisons
// are case-insensitive and ignore underscore and hyphen separators.
func NormalizeModelIdentifier(id string) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(id)) {
		if char == '_' || char == '-' {
			continue
		}
		normalized.WriteRune(char)
	}
	return normalized.String()
}

// resolveNormalizedBuiltin finds an exact normalized ID or alias match in the
// project's static catalog. Dynamic aggregate providers are excluded so one
// gateway cannot accidentally become the metadata source for another.
func (c *ModelCatalog) resolveNormalizedBuiltin(id string, excludedProviders map[string]struct{}) (ModelInfo, bool) {
	wanted := NormalizeModelIdentifier(id)
	if wanted == "" {
		return ModelInfo{}, false
	}
	models := c.All()
	for _, model := range models {
		if _, excluded := excludedProviders[CanonicalProviderName(model.Provider)]; excluded {
			continue
		}
		if NormalizeModelIdentifier(model.ID) == wanted {
			return model, true
		}
		for _, alias := range model.Aliases {
			if NormalizeModelIdentifier(alias) == wanted {
				return model, true
			}
		}
	}
	return ModelInfo{}, false
}

func stringValue(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func firstStringValue(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := stringValue(values, key); ok {
			return value, true
		}
	}
	return "", false
}

func mapValue(values map[string]any, key string) (map[string]any, bool) {
	value, ok := values[key]
	if !ok {
		return nil, false
	}
	result, ok := value.(map[string]any)
	return result, ok
}

func firstIntValue(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		parsed, ok := numericValue(value)
		if ok && parsed > 0 {
			return int(parsed), true
		}
	}
	return 0, false
}

func firstFloatValue(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		if parsed, ok := numericValue(value); ok {
			return parsed, true
		}
	}
	return 0, false
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func firstBoolValue(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
			if err == nil {
				return parsed, true
			}
		case json.Number:
			parsed, err := typed.Int64()
			if err == nil && (parsed == 0 || parsed == 1) {
				return parsed == 1, true
			}
		}
	}
	return false, false
}

func firstStringSliceValue(values map[string]any, keys ...string) ([]string, bool) {
	for _, key := range keys {
		value, exists := values[key]
		if !exists {
			continue
		}
		raw, ok := value.([]any)
		if !ok {
			continue
		}
		result := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result, true
	}
	return nil, false
}
