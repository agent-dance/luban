package provider

import (
	"strings"
	"sync"
)

const (
	MinOverrideContextWindow = 8192
	MaxOverrideContextWindow = 10000000
)

type ModelOverride struct {
	ContextWindow *int `json:"context_window,omitempty"`
	MaxOutput     *int `json:"max_output,omitempty"`
}

type ModelOverrides map[string]ModelOverride

var (
	runtimeModelOverridesMu sync.RWMutex
	runtimeModelOverrides   ModelOverrides
)

func SetRuntimeModelOverrides(overrides ModelOverrides) {
	runtimeModelOverridesMu.Lock()
	defer runtimeModelOverridesMu.Unlock()
	runtimeModelOverrides = cloneModelOverrides(overrides)
}

func RuntimeModelOverrides() ModelOverrides {
	runtimeModelOverridesMu.RLock()
	defer runtimeModelOverridesMu.RUnlock()
	return cloneModelOverrides(runtimeModelOverrides)
}

func cloneModelOverrides(overrides ModelOverrides) ModelOverrides {
	if len(overrides) == 0 {
		return nil
	}
	cloned := make(ModelOverrides, len(overrides))
	for key, override := range overrides {
		var copied ModelOverride
		if override.ContextWindow != nil {
			value := *override.ContextWindow
			copied.ContextWindow = &value
		}
		if override.MaxOutput != nil {
			value := *override.MaxOutput
			copied.MaxOutput = &value
		}
		cloned[key] = copied
	}
	return cloned
}

func OverrideKey(providerName, modelID string) string {
	providerName = CanonicalProviderName(providerName)
	modelID = strings.TrimSpace(modelID)
	if providerName == "" || modelID == "" {
		return ""
	}
	return providerName + "/" + modelID
}

func ValidOverrideContextWindow(value int) bool {
	return value >= MinOverrideContextWindow && value <= MaxOverrideContextWindow
}

func (c *ModelCatalog) ApplyOverrides(overrides ModelOverrides) {
	if c == nil || len(overrides) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, override := range overrides {
		providerName, modelID, ok := strings.Cut(strings.TrimSpace(key), "/")
		if !ok {
			continue
		}
		catalogKey := modelCatalogKey(CanonicalProviderName(providerName), strings.TrimSpace(modelID))
		model, ok := c.models[catalogKey]
		if !ok {
			continue
		}
		if override.ContextWindow != nil && ValidOverrideContextWindow(*override.ContextWindow) {
			model.ContextWindow = *override.ContextWindow
			model.ContextOverridden = true
		}
		if override.MaxOutput != nil && *override.MaxOutput >= 0 && (model.ContextWindow <= 0 || *override.MaxOutput <= model.ContextWindow) {
			model.MaxOutput = *override.MaxOutput
		}
		c.models[catalogKey] = model
	}
}
