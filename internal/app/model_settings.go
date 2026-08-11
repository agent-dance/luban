package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
)

type startupModelSettings struct {
	Provider              string
	Model                 string
	APIKey                string
	CacheRoutingMode      string
	ReasoningEffort       string
	ModelOverrides        provider.ModelOverrides
	ProgressiveContext    compact.ProgressiveConfig
	ProgressiveContextSet bool
	Source                string
}

func loadStartupModelSettings(cwd string) (startupModelSettings, error) {
	paths := make([]string, 0, 3)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, brand.ConfigDirName, "settings.json"))
	}
	if strings.TrimSpace(cwd) != "" {
		paths = append(paths,
			filepath.Join(cwd, brand.ConfigDirName, "settings.json"),
			filepath.Join(cwd, brand.ConfigDirName, "settings.local.json"),
		)
	}

	var merged startupModelSettings
	for _, path := range paths {
		settings, ok, err := readModelSettingsFile(path)
		if err != nil {
			return startupModelSettings{}, err
		}
		if ok {
			if settings.Provider != "" {
				if merged.Provider != "" && settings.Provider != merged.Provider {
					// API keys are provider-scoped. Never carry a lower-priority
					// provider's key across a provider override.
					merged.APIKey = ""
					merged.ReasoningEffort = ""
				}
				merged.Provider = settings.Provider
			}
			if settings.Model != "" {
				if merged.Model != "" && settings.Model != merged.Model {
					merged.ReasoningEffort = ""
				}
				merged.Model = settings.Model
			}
			if settings.APIKey != "" {
				merged.APIKey = settings.APIKey
			}
			if settings.CacheRoutingMode != "" {
				merged.CacheRoutingMode = settings.CacheRoutingMode
			}
			if settings.ReasoningEffort != "" {
				merged.ReasoningEffort = settings.ReasoningEffort
			}
			if len(settings.ModelOverrides) > 0 {
				if merged.ModelOverrides == nil {
					merged.ModelOverrides = provider.ModelOverrides{}
				}
				for key, override := range settings.ModelOverrides {
					merged.ModelOverrides[key] = override
				}
			}
			if settings.ProgressiveContextSet {
				merged.ProgressiveContext = settings.ProgressiveContext
				merged.ProgressiveContextSet = true
			}
			merged.Source = path
		}
	}
	if !merged.ProgressiveContextSet {
		merged.ProgressiveContext = compact.ProductionProgressiveConfig()
	}
	return merged, nil
}

func readModelSettingsFile(path string) (startupModelSettings, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return startupModelSettings{}, false, nil
		}
		return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsRead, err, path)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
	}

	settings := startupModelSettings{}
	if providerValue, ok := raw["provider"].(string); ok {
		settings.Provider = normalizeSettingsProvider(providerValue)
	}
	if modelValue, ok := raw["model"].(string); ok {
		settings.Model = strings.TrimSpace(modelValue)
	}
	if apiKeyValue, ok := raw["apiKey"].(string); ok {
		settings.APIKey = strings.TrimSpace(apiKeyValue)
	}
	if cacheRoutingMode, ok := raw["cacheRoutingMode"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(cacheRoutingMode)) {
		case "auto", "on", "off":
			settings.CacheRoutingMode = strings.ToLower(strings.TrimSpace(cacheRoutingMode))
		}
	}
	if reasoningEffort, ok := raw["reasoningEffort"].(string); ok {
		settings.ReasoningEffort = strings.TrimSpace(reasoningEffort)
	}
	if rawOverrides, ok := raw["modelOverrides"]; ok {
		data, err := json.Marshal(rawOverrides)
		if err != nil {
			return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		var overrides provider.ModelOverrides
		if err := json.Unmarshal(data, &overrides); err != nil {
			return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		settings.ModelOverrides = sanitizedModelOverrides(overrides)
	}
	if rawProgressive, ok := raw["progressiveContext"]; ok {
		data, err := json.Marshal(rawProgressive)
		if err != nil {
			return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		var progressive compact.ProgressiveConfig
		if err := json.Unmarshal(data, &progressive); err != nil {
			return startupModelSettings{}, false, rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		settings.ProgressiveContext = compact.NormalizeProgressiveConfig(progressive)
		settings.ProgressiveContextSet = true
	}
	if settings.Provider == "" && settings.Model == "" && settings.APIKey == "" && settings.CacheRoutingMode == "" && settings.ReasoningEffort == "" && len(settings.ModelOverrides) == 0 && !settings.ProgressiveContextSet {
		return startupModelSettings{}, false, nil
	}
	return settings, true, nil
}

func resolveStartupReasoningEffort(explicit string, settings startupModelSettings, providerName, modelID string, catalog *provider.ModelCatalog) string {
	if effort := strings.TrimSpace(explicit); effort != "" {
		return effort
	}

	providerName = normalizeSettingsProvider(providerName)
	modelID = strings.TrimSpace(modelID)
	settingsProvider := normalizeSettingsProvider(settings.Provider)
	settingsModel := strings.TrimSpace(settings.Model)
	if effort := strings.TrimSpace(settings.ReasoningEffort); effort != "" &&
		(settingsProvider == "" || settingsProvider == providerName) &&
		(settingsModel == "" || settingsModel == modelID) {
		return effort
	}

	if catalog != nil {
		if model, ok := catalog.ResolveForProvider(providerName, modelID); ok {
			return provider.DefaultReasoningEffortForModel(model)
		}
	}
	return ""
}

func applyStartupModelSettings(opts *cli.Options, settings startupModelSettings) {
	if opts == nil {
		return
	}

	settingsProvider := normalizeSettingsProvider(settings.Provider)
	settingsModel := strings.TrimSpace(settings.Model)
	envProvider := explicitProviderFromEnv()

	if opts.Provider == "" && envProvider == "" && settingsProvider != "" {
		opts.Provider = settingsProvider
	}

	effectiveProvider := normalizeSettingsProvider(opts.Provider)
	if effectiveProvider == "" {
		effectiveProvider = envProvider
	}
	if effectiveProvider == "" && settingsProvider != "" {
		effectiveProvider = settingsProvider
	}
	if effectiveProvider == "" {
		effectiveProvider = detectedDefaultProvider()
	}

	if opts.Model == "" &&
		settingsModel != "" &&
		!modelEnvSetForProvider(effectiveProvider) &&
		(settingsProvider == "" || settingsProvider == effectiveProvider) {
		opts.Model = settingsModel
	}

	settingsAPIKey := strings.TrimSpace(settings.APIKey)
	if settingsAPIKey != "" {
		if envKey := apiKeyEnvForProvider(effectiveProvider); envKey != "" && os.Getenv(envKey) == "" {
			os.Setenv(envKey, settingsAPIKey)
		}
	}
	if mode := strings.TrimSpace(settings.CacheRoutingMode); mode != "" && os.Getenv("LUBAN_CODE_CACHE_ROUTING_MODE") == "" {
		os.Setenv("LUBAN_CODE_CACHE_ROUTING_MODE", mode)
	}
}

func sanitizedModelOverrides(overrides provider.ModelOverrides) provider.ModelOverrides {
	if len(overrides) == 0 {
		return nil
	}
	sanitized := provider.ModelOverrides{}
	for key, override := range overrides {
		providerName, modelID, ok := strings.Cut(strings.TrimSpace(key), "/")
		if !ok {
			continue
		}
		cleanKey := provider.OverrideKey(providerName, modelID)
		if cleanKey == "" {
			continue
		}
		cleanOverride := provider.ModelOverride{}
		if override.ContextWindow != nil && provider.ValidOverrideContextWindow(*override.ContextWindow) {
			value := *override.ContextWindow
			cleanOverride.ContextWindow = &value
		}
		if override.MaxOutput != nil && *override.MaxOutput >= 0 && (cleanOverride.ContextWindow == nil || *override.MaxOutput <= *cleanOverride.ContextWindow) {
			value := *override.MaxOutput
			cleanOverride.MaxOutput = &value
		}
		if cleanOverride.ContextWindow != nil || cleanOverride.MaxOutput != nil {
			sanitized[cleanKey] = cleanOverride
		}
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func saveUserModelOverride(providerName, modelID string, contextWindow *int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootModelSettingsHome, err)
	}
	return saveModelOverrideAt(filepath.Join(home, brand.ConfigDirName, "settings.json"), providerName, modelID, contextWindow)
}

func saveModelOverrideAt(path, providerName, modelID string, contextWindow *int) error {
	key := provider.OverrideKey(providerName, modelID)
	if key == "" {
		return nil
	}
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsRead, err, path)
		}
	} else if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
	}

	overrides := provider.ModelOverrides{}
	if raw, ok := settings["modelOverrides"]; ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		if err := json.Unmarshal(data, &overrides); err != nil {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
		overrides = sanitizedModelOverrides(overrides)
	}
	if overrides == nil {
		overrides = provider.ModelOverrides{}
	}
	if contextWindow == nil {
		delete(overrides, key)
	} else {
		overrides[key] = provider.ModelOverride{ContextWindow: contextWindow}
	}
	if len(overrides) == 0 {
		delete(settings, "modelOverrides")
	} else {
		settings["modelOverrides"] = overrides
	}
	return writeSettingsFile(path, settings)
}

func writeSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return rootRuntimeWrap(i18n.KeyRootModelSettingsDirectory, err, filepath.Dir(path))
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootModelSettingsMarshal, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootModelSettingsTempCreate, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return rootRuntimeWrap(i18n.KeyRootModelSettingsTempWrite, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return rootRuntimeWrap(i18n.KeyRootModelSettingsTempChmod, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return rootRuntimeWrap(i18n.KeyRootModelSettingsTempClose, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return rootRuntimeWrap(i18n.KeyRootModelSettingsReplace, err, path)
	}
	return nil
}

func saveUserModelSettings(providerName, modelID string, reasoningEffort ...string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootModelSettingsHome, err)
	}
	return saveModelSettingsAt(filepath.Join(home, brand.ConfigDirName, "settings.json"), providerName, modelID, reasoningEffort...)
}

func saveModelSettingsAt(path, providerName, modelID string, reasoningEffort ...string) error {
	providerName = normalizeSettingsProvider(providerName)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	settings := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsRead, err, path)
		}
	} else if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return rootRuntimeWrap(i18n.KeyRootModelSettingsParse, err, path)
		}
	}

	if providerName != "" {
		settings["provider"] = providerName
	}
	settings["model"] = modelID
	if len(reasoningEffort) > 0 {
		if effort := strings.TrimSpace(reasoningEffort[0]); effort != "" {
			settings["reasoningEffort"] = effort
		} else {
			delete(settings, "reasoningEffort")
		}
	}

	return writeSettingsFile(path, settings)
}

func normalizeSettingsProvider(providerName string) string {
	return strings.ToLower(strings.TrimSpace(providerName))
}

func explicitProviderFromEnv() string {
	if providerName := normalizeSettingsProvider(os.Getenv("PROVIDER")); providerName != "" {
		return providerName
	}
	if os.Getenv("LUBAN_CODE_USE_BEDROCK") == "1" {
		return "bedrock"
	}
	if os.Getenv("LUBAN_CODE_USE_VERTEX") == "1" {
		return "vertex"
	}
	return ""
}

func detectedDefaultProvider() string {
	if envProvider := explicitProviderFromEnv(); envProvider != "" {
		return envProvider
	}
	if os.Getenv("DEEPSEEK_API_KEY") != "" {
		return brand.DeepSeekProvider
	}
	if os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return "openai"
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "anthropic"
	}
	return brand.DefaultProvider
}

func apiKeyEnvForProvider(providerName string) string {
	switch normalizeSettingsProvider(providerName) {
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "groq":
		return "GROQ_API_KEY"
	case "xai":
		return "XAI_API_KEY"
	case "mistral":
		return "MISTRAL_API_KEY"
	case "zhipu":
		return "ZHIPU_API_KEY"
	case "minimax":
		return "MINIMAX_API_KEY"
	case "kimi":
		return "MOONSHOT_API_KEY"
	default:
		return ""
	}
}

func modelEnvSetForProvider(providerName string) bool {
	switch normalizeSettingsProvider(providerName) {
	case "anthropic":
		return os.Getenv("ANTHROPIC_MODEL") != ""
	case "openai":
		return os.Getenv("OPENAI_MODEL") != ""
	case "bedrock":
		return os.Getenv("BEDROCK_MODEL") != ""
	case "vertex":
		return os.Getenv("VERTEX_MODEL") != ""
	case "ollama":
		return os.Getenv("OLLAMA_MODEL") != ""
	case "deepseek":
		return os.Getenv("DEEPSEEK_MODEL") != ""
	case "gemini":
		return os.Getenv("GEMINI_MODEL") != ""
	case "groq":
		return os.Getenv("GROQ_MODEL") != ""
	case "xai":
		return os.Getenv("XAI_MODEL") != ""
	case "mistral":
		return os.Getenv("MISTRAL_MODEL") != ""
	case "zhipu":
		return os.Getenv("ZHIPU_MODEL") != ""
	case "minimax":
		return os.Getenv("MINIMAX_MODEL") != ""
	case "kimi":
		return os.Getenv("KIMI_MODEL") != ""
	default:
		return false
	}
}
