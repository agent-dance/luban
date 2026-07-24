package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
)

const (
	remoteTriggerGrowthBookFeature = "tengu_surreal_dali"
	remoteTriggerPolicyName        = "allow_remote_sessions"
)

// RemoteTriggerAvailability is the cache-backed feature and policy surface
// used by the production RemoteTrigger instance. Missing policy data fails
// open, while a missing GrowthBook feature uses the caller-provided default.
type RemoteTriggerAvailability interface {
	FeatureEnabled(name string, defaultValue bool) bool
	PolicyAllowed(name string) bool
}

// CachedRemoteTriggerAvailability reads the same durable cache shapes used by
// the TS runtime. Reads are intentionally fresh for each decision so another
// process's GrowthBook or policy refresh is observed without a Go-only polling
// goroutine or process-global mutable state.
type CachedRemoteTriggerAvailability struct {
	featureConfigPaths []string
	policyCachePaths   []string
}

func NewCachedRemoteTriggerAvailability() *CachedRemoteTriggerAvailability {
	return &CachedRemoteTriggerAvailability{
		featureConfigPaths: remoteTriggerFeatureConfigPaths(),
		policyCachePaths:   remoteTriggerPolicyCachePaths(),
	}
}

func (s *CachedRemoteTriggerAvailability) FeatureEnabled(name string, defaultValue bool) bool {
	if s == nil || strings.TrimSpace(name) == "" {
		return defaultValue
	}
	if value, ok := remoteTriggerGrowthBookEnvOverride(name); ok {
		return value
	}
	for _, path := range s.featureConfigPaths {
		var config struct {
			GrowthBookOverrides      map[string]any `json:"growthBookOverrides"`
			CachedGrowthBookFeatures map[string]any `json:"cachedGrowthBookFeatures"`
		}
		if !readRemoteTriggerCache(path, &config) {
			continue
		}
		if value, ok := remoteTriggerBool(config.GrowthBookOverrides[name]); ok {
			return value
		}
		if value, ok := remoteTriggerBool(config.CachedGrowthBookFeatures[name]); ok {
			return value
		}
	}
	return defaultValue
}

func (s *CachedRemoteTriggerAvailability) PolicyAllowed(name string) bool {
	if s == nil || strings.TrimSpace(name) == "" {
		return true
	}
	for _, path := range s.policyCachePaths {
		var cache struct {
			Restrictions map[string]struct {
				Allowed bool `json:"allowed"`
			} `json:"restrictions"`
		}
		if !readRemoteTriggerCache(path, &cache) {
			continue
		}
		restriction, ok := cache.Restrictions[name]
		if !ok {
			return true
		}
		return restriction.Allowed
	}
	return true
}

func remoteTriggerGrowthBookEnvOverride(name string) (bool, bool) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("USER_TYPE")), "ant") {
		return false, false
	}
	raw := strings.TrimSpace(os.Getenv("CLAUDE_INTERNAL_FC_OVERRIDES"))
	if raw == "" {
		return false, false
	}
	var overrides map[string]any
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil {
		return false, false
	}
	return remoteTriggerBool(overrides[name])
}

func remoteTriggerBool(value any) (bool, bool) {
	result, ok := value.(bool)
	return result, ok
}

func readRemoteTriggerCache(path string, destination any) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4<<20 {
		return false
	}
	return json.Unmarshal(data, destination) == nil
}

func remoteTriggerFeatureConfigPaths() []string {
	suffix := remoteTriggerOAuthFileSuffix()
	filename := ".claude" + suffix + ".json"
	paths := make([]string, 0, 5)
	if configDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configDir != "" {
		paths = append(paths, filepath.Join(configDir, ".config.json"), filepath.Join(configDir, filename))
	}
	if home := brand.HomeDir(); home != "" {
		paths = append(paths, filepath.Join(home, filename))
	}
	paths = append(paths,
		filepath.Join(brand.UserConfigDir(), "global.json"),
		filepath.Join(brand.LegacyDeepSeekUserConfigDir(), ".config.json"),
		filepath.Join(brand.LegacyUserConfigDir(), ".config.json"),
	)
	return uniqueRemoteTriggerPaths(paths)
}

func remoteTriggerPolicyCachePaths() []string {
	paths := make([]string, 0, 3)
	if configDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configDir != "" {
		paths = append(paths, filepath.Join(configDir, "policy-limits.json"))
	}
	paths = append(paths,
		filepath.Join(brand.UserConfigDir(), "policy-limits.json"),
		filepath.Join(brand.LegacyDeepSeekUserConfigDir(), "policy-limits.json"),
		filepath.Join(brand.LegacyUserConfigDir(), "policy-limits.json"),
	)
	return uniqueRemoteTriggerPaths(paths)
}

func remoteTriggerOAuthFileSuffix() string {
	if strings.TrimSpace(os.Getenv("CLAUDE_CODE_CUSTOM_OAUTH_URL")) != "" {
		return "-custom-oauth"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("USER_TYPE")), "ant") {
		if isTruthyEnv(os.Getenv("USE_LOCAL_OAUTH")) {
			return "-local-oauth"
		}
		if isTruthyEnv(os.Getenv("USE_STAGING_OAUTH")) {
			return "-staging-oauth"
		}
	}
	return ""
}

func uniqueRemoteTriggerPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}
