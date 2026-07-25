package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"gopkg.in/yaml.v3"
)

type pluginAgentRoot struct {
	Name   string
	Source string
	Path   string
	CWD    string
}

type pluginManifest struct {
	Name        string                            `json:"name"`
	Description string                            `json:"description"`
	Agents      any                               `json:"agents"`
	UserConfig  map[string]pluginUserConfigOption `json:"userConfig"`
}

type pluginUserConfigOption struct {
	Sensitive bool `json:"sensitive"`
}

func loadPluginAgentProfile(agentType, cwd string) (agentProfile, bool, error) {
	roots, err := enabledPluginAgentRoots(cwd)
	if err != nil {
		return agentProfile{}, false, err
	}
	for _, root := range roots {
		profile, ok, err := loadPluginAgentProfileFromRoot(root, agentType)
		if err != nil {
			return agentProfile{}, false, err
		}
		if ok {
			return profile, true, nil
		}
	}
	return agentProfile{}, false, nil
}

func loadPluginAgentProfiles(cwd string) ([]agentProfile, error) {
	roots, err := enabledPluginAgentRoots(cwd)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var profiles []agentProfile
	for _, root := range roots {
		manifest := loadPluginManifest(root.Path, root.Name)
		paths, err := pluginAgentFiles(root.Path, manifest)
		if err != nil {
			continue
		}
		for _, candidate := range paths {
			profile, ok, err := parsePluginAgentProfileFile(candidate.path, root, candidate.namespace)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(profile.Name))
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}

func enabledPluginAgentRoots(cwd string) ([]pluginAgentRoot, error) {
	enabled := enabledPluginIDs(cwd)
	if len(enabled) == 0 {
		return nil, nil
	}
	pluginsDir, err := pluginsDir()
	if err != nil {
		return nil, err
	}
	installedPaths := installedPluginPaths(pluginsDir)
	roots := make([]pluginAgentRoot, 0, len(enabled))
	for pluginID := range enabled {
		pluginName, marketplace, ok := parsePluginIdentifier(pluginID)
		if !ok || marketplace == "" {
			continue
		}
		root := resolveInstalledPluginRoot(installedPaths[pluginID])
		if root == "" {
			root = resolveCachedPluginRoot(pluginsDir, pluginName, marketplace)
		}
		if root == "" {
			continue
		}
		manifest := loadPluginManifest(root, pluginName)
		name := strings.TrimSpace(manifest.Name)
		if name == "" {
			name = pluginName
		}
		roots = append(roots, pluginAgentRoot{
			Name:   name,
			Source: pluginID,
			Path:   root,
			CWD:    cwd,
		})
	}
	sort.SliceStable(roots, func(i, j int) bool {
		return roots[i].Source < roots[j].Source
	})
	return roots, nil
}

type installedPluginRecord struct {
	Version     string `json:"version"`
	InstallPath string `json:"installPath"`
}

func installedPluginPaths(pluginsDir string) map[string][]string {
	out := map[string][]string{}
	data, err := os.ReadFile(filepath.Join(pluginsDir, "installed_plugins.json"))
	if err != nil {
		return out
	}
	var raw struct {
		Version int                        `json:"version"`
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw.Plugins) == 0 {
		return out
	}
	for pluginID, payload := range raw.Plugins {
		pluginID = strings.TrimSpace(pluginID)
		if pluginID == "" {
			continue
		}
		if raw.Version == 2 {
			var entries []installedPluginRecord
			if err := json.Unmarshal(payload, &entries); err != nil {
				continue
			}
			for _, entry := range entries {
				addInstalledPluginPath(out, pluginID, entry.InstallPath)
			}
			continue
		}
		var entry installedPluginRecord
		if err := json.Unmarshal(payload, &entry); err != nil {
			continue
		}
		addInstalledPluginPath(out, pluginID, entry.InstallPath)
		if entry.Version != "" {
			addInstalledPluginPath(out, pluginID, versionedPluginCachePath(pluginsDir, pluginID, entry.Version))
		}
	}
	return out
}

func addInstalledPluginPath(paths map[string][]string, pluginID, path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	paths[pluginID] = append(paths[pluginID], filepath.Clean(path))
}

func resolveInstalledPluginRoot(paths []string) string {
	for _, path := range paths {
		if hasPluginAgentSurface(path) {
			return path
		}
	}
	return ""
}

func enabledPluginIDs(cwd string) map[string]bool {
	paths := []string{}
	if home := agentConfigHomeDir(); home != "" {
		paths = append(paths, filepath.Join(home, "settings.json"))
	}
	if strings.TrimSpace(cwd) != "" {
		paths = append(paths, filepath.Join(cwd, brand.ConfigDirName, "settings.json"))
	}
	enabled := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw struct {
			EnabledPlugins map[string]any `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		for id, value := range raw.EnabledPlugins {
			if pluginEnabledValue(value) {
				enabled[id] = true
			}
		}
	}
	return enabled
}

func pluginEnabledValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	default:
		return false
	}
}

func agentConfigHomeDir() string {
	if dir := strings.TrimSpace(os.Getenv("LUBAN_CODE_CONFIG_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, brand.ConfigDirName)
}

func pluginsDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("LUBAN_CODE_PLUGIN_CACHE_DIR")); dir != "" {
		return filepath.Clean(dir), nil
	}
	configDir := agentConfigHomeDir()
	if configDir == "" {
		return "", i18n.NewError(i18n.KeyToolAgentPluginConfigDirectoryUnavailable)
	}
	dirName := "plugins"
	if isTruthyAgentEnv(os.Getenv("LUBAN_CODE_USE_COWORK_PLUGINS")) {
		dirName = "cowork_plugins"
	}
	return filepath.Join(configDir, dirName), nil
}

func isTruthyAgentEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parsePluginIdentifier(pluginID string) (pluginName, marketplace string, ok bool) {
	pluginID = strings.TrimSpace(pluginID)
	at := strings.LastIndex(pluginID, "@")
	if at <= 0 || at == len(pluginID)-1 {
		return "", "", false
	}
	return pluginID[:at], pluginID[at+1:], true
}

func resolveCachedPluginRoot(pluginsDir, pluginName, marketplace string) string {
	marketplaceDir := filepath.Join(
		pluginsDir,
		"cache",
		sanitizePluginPathPart(marketplace),
		sanitizePluginPathPart(pluginName),
	)
	if entries, err := os.ReadDir(marketplaceDir); err == nil {
		var candidates []string
		for _, entry := range entries {
			if entry.IsDir() {
				candidates = append(candidates, filepath.Join(marketplaceDir, entry.Name()))
			}
		}
		sort.Strings(candidates)
		for i := len(candidates) - 1; i >= 0; i-- {
			if hasPluginAgentSurface(candidates[i]) {
				return candidates[i]
			}
		}
	}

	return ""
}

func versionedPluginCachePath(pluginsDir, pluginID, version string) string {
	pluginName, marketplace, ok := parsePluginIdentifier(pluginID)
	if !ok {
		return ""
	}
	return filepath.Join(
		pluginsDir,
		"cache",
		sanitizePluginPathPart(marketplace),
		sanitizePluginPathPart(pluginName),
		sanitizePluginVersionPathPart(version),
	)
}

func sanitizePluginPathPart(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func sanitizePluginVersionPathPart(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func hasPluginAgentSurface(root string) bool {
	if root == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(root, "agents")); err == nil && info.IsDir() {
		return true
	}
	manifest := loadPluginManifest(root, "")
	return manifest.Agents != nil
}

func loadPluginManifest(root, fallbackName string) pluginManifest {
	path := filepath.Join(root, ".luban-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err == nil {
		var manifest pluginManifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			if strings.TrimSpace(manifest.Name) == "" {
				manifest.Name = fallbackName
			}
			return manifest
		}
	}
	return pluginManifest{Name: fallbackName}
}

func loadPluginAgentProfileFromRoot(root pluginAgentRoot, agentType string) (agentProfile, bool, error) {
	manifest := loadPluginManifest(root.Path, root.Name)
	paths, err := pluginAgentFiles(root.Path, manifest)
	if err != nil {
		return agentProfile{}, false, err
	}
	for _, candidate := range paths {
		profile, ok, err := parsePluginAgentProfileFile(candidate.path, root, candidate.namespace)
		if err != nil {
			return agentProfile{}, false, err
		}
		if !ok {
			continue
		}
		if strings.EqualFold(profile.Name, agentType) {
			return profile, true, nil
		}
	}
	return agentProfile{}, false, nil
}

type pluginAgentFile struct {
	path      string
	namespace []string
}

func pluginAgentFiles(root string, manifest pluginManifest) ([]pluginAgentFile, error) {
	var files []pluginAgentFile
	seen := map[string]struct{}{}
	addFile := func(file pluginAgentFile) {
		cleaned := filepath.Clean(file.path)
		key := strings.ToLower(cleaned)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		file.path = cleaned
		files = append(files, file)
	}
	agentsDir := filepath.Join(root, "agents")
	err := filepath.WalkDir(agentsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(agentsDir, filepath.Dir(path))
		if err != nil {
			return nil
		}
		var namespace []string
		if rel != "." {
			namespace = strings.Split(filepath.ToSlash(rel), "/")
		}
		addFile(pluginAgentFile{path: path, namespace: namespace})
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if manifest.Agents != nil {
		for _, rel := range stringsFromYAML(manifest.Agents) {
			cleaned := filepath.Clean(strings.TrimPrefix(rel, "./"))
			if strings.TrimSpace(cleaned) == "" || strings.HasPrefix(cleaned, "..") || !strings.EqualFold(filepath.Ext(cleaned), ".md") {
				continue
			}
			full := filepath.Join(root, cleaned)
			if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() {
				addFile(pluginAgentFile{path: full})
			}
		}
	}
	return files, nil
}

type pluginAgentFrontmatterSchema struct {
	Name            any `yaml:"name"`
	Description     any `yaml:"description"`
	Tools           any `yaml:"tools"`
	DisallowedTools any `yaml:"disallowedTools"`
	Skills          any `yaml:"skills"`
	Model           any `yaml:"model"`
	Effort          any `yaml:"effort"`
	MaxTurns        any `yaml:"maxTurns"`
	Background      any `yaml:"background"`
	Memory          any `yaml:"memory"`
	Color           any `yaml:"color"`
	Isolation       any `yaml:"isolation"`
}

func parsePluginAgentProfileFile(path string, root pluginAgentRoot, namespace []string) (agentProfile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return agentProfile{}, false, err
	}
	text := string(data)
	frontmatter, body, ok, err := splitMarkdownFrontmatter(text)
	if err != nil {
		return agentProfile{}, false, i18n.WrapError(i18n.KeyToolAgentDeepFrontmatterParseFailed, err, path)
	}
	if strings.TrimSpace(body) == "" {
		return agentProfile{}, false, nil
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if ok {
		if fmName, _ := frontmatter["name"].(string); strings.TrimSpace(fmName) != "" {
			name = strings.TrimSpace(fmName)
		}
	}
	nameParts := append([]string{root.Name}, namespace...)
	nameParts = append(nameParts, name)
	agentType := strings.Join(nameParts, ":")

	description, _ := frontmatter["description"].(string)
	allowedValue, allowedPresent := yamlValueWithPresence(frontmatter, "tools")
	disallowedValue := yamlValue(frontmatter, "disallowedTools")
	allowedTools, allowedRules, allowedSpecs, allowedSpecified := allowedToolProfileFieldsFromYAML(allowedValue, allowedPresent)
	disallowedTools, disallowedRules, disallowedSpecs := disallowedToolProfileFieldsFromYAML(disallowedValue)
	profile := agentProfile{
		Name:                  agentType,
		WhenToUse:             normalizeAgentWhenToUse(description),
		SystemPrefix:          substitutePluginAgentVariables(strings.TrimSpace(body), root),
		AllowedTools:          allowedTools,
		DisallowedTools:       disallowedTools,
		AllowedToolRules:      allowedRules,
		DisallowedToolRules:   disallowedRules,
		AllowedToolSpecs:      allowedSpecs,
		DisallowedToolSpecs:   disallowedSpecs,
		AllowedToolsSpecified: allowedSpecified,
	}
	if ok {
		if skills := stringsFromYAML(yamlValue(frontmatter, "skills")); len(skills) > 0 {
			profile.Skills = skills
		}
		if model, ok := stringFromYAML(frontmatter, "model"); ok {
			if normalized, valid := agentModelFromString(model); valid {
				profile.Model = normalized
			}
		}
		if effort := reasoningEffortFromValue(yamlValue(frontmatter, "effort")); effort != "" {
			profile.ReasoningEffort = effort
		}
		if maxTurns, ok := positiveIntFromYAML(yamlValue(frontmatter, "maxTurns")); ok {
			profile.MaxTurns = maxTurns
		}
		if agentMarkdownBackgroundFromYAML(yamlValue(frontmatter, "background")) {
			profile.Background = true
		}
		if memory, ok := stringFromYAML(frontmatter, "memory"); ok {
			if isValidAgentMemoryScope(memory) {
				profile.Memory = memory
			}
		}
		if color, ok := stringFromYAML(frontmatter, "color"); ok {
			if isValidAgentColor(color) {
				profile.Color = color
			}
		}
		if isolation, ok := stringFromYAML(frontmatter, "isolation"); ok && isolation == "worktree" {
			profile.Isolation = isolation
		}
	}
	applyAgentMemoryToolAccess(&profile, root.CWD)
	return profile, true, nil
}

func splitMarkdownFrontmatter(text string) (map[string]any, string, bool, error) {
	if !strings.HasPrefix(text, "---") {
		return map[string]any{}, text, false, nil
	}
	rest := strings.TrimPrefix(text, "---")
	parts := strings.SplitN(rest, "---", 2)
	if len(parts) != 2 {
		return nil, "", false, fmt.Errorf("missing closing frontmatter delimiter")
	}
	if err := validateAgentFrontmatter[pluginAgentFrontmatterSchema](parts[0]); err != nil {
		return nil, "", false, err
	}
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(parts[0]), &frontmatter); err != nil {
		return nil, "", false, err
	}
	return frontmatter, parts[1], true, nil
}

func substitutePluginAgentVariables(content string, root pluginAgentRoot) string {
	content = strings.ReplaceAll(content, "${LUBAN_PLUGIN_ROOT}", normalizePluginSubstitutionPath(root.Path))
	if strings.Contains(content, "${LUBAN_PLUGIN_DATA}") {
		content = strings.ReplaceAll(content, "${LUBAN_PLUGIN_DATA}", normalizePluginSubstitutionPath(pluginDataDir(root.Source)))
	}
	manifest := loadPluginManifest(root.Path, root.Name)
	if len(manifest.UserConfig) == 0 || !strings.Contains(content, "${user_config.") {
		return content
	}
	return substitutePluginUserConfigInContent(content, loadPluginOptions(root.Source, root.CWD), manifest.UserConfig)
}

func normalizePluginSubstitutionPath(path string) string {
	return filepath.ToSlash(path)
}

func pluginDataDir(pluginID string) string {
	pluginsDir, err := pluginsDir()
	if err != nil || strings.TrimSpace(pluginID) == "" {
		return ""
	}
	dir := filepath.Join(pluginsDir, "data", sanitizePluginPathPart(pluginID))
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func loadPluginOptions(pluginID, cwd string) map[string]any {
	out := map[string]any{}
	for _, path := range pluginSettingsPaths(cwd) {
		mergePluginOptionsFromSettings(out, path, pluginID)
	}
	return out
}

func mergePluginOptionsFromSettings(out map[string]any, path, pluginID string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw struct {
		PluginConfigs map[string]struct {
			Options map[string]any `json:"options"`
		} `json:"pluginConfigs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	for key, value := range raw.PluginConfigs[pluginID].Options {
		out[key] = value
	}
}

func pluginSettingsPaths(cwd string) []string {
	var paths []string
	if home := agentConfigHomeDir(); home != "" {
		paths = append(paths, filepath.Join(home, "settings.json"))
	}
	if strings.TrimSpace(cwd) != "" {
		paths = append(paths, filepath.Join(cwd, brand.ConfigDirName, "settings.json"))
	}
	return paths
}

func substitutePluginUserConfigInContent(content string, options map[string]any, schema map[string]pluginUserConfigOption) string {
	re := regexp.MustCompile(`\$\{user_config\.([^}]+)\}`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		key := parts[1]
		if schema[key].Sensitive {
			return fmt.Sprintf("[sensitive option '%s' not available in skill content]", key)
		}
		value, ok := options[key]
		if !ok {
			return match
		}
		return fmt.Sprint(value)
	})
}
