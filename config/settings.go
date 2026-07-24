package config

// PromptSettings contains the original settings that affect prompt assembly.
// This is intentionally not a full settings subsystem; callers that already
// have runtime settings can map only these fields into prompt.Config.
type PromptSettings struct {
	DisableClaudeMds                    bool
	BareMode                            bool
	ClaudeMdExcludes                    []string
	AdditionalDirectories               []string
	AdditionalDirectoriesClaudeMd       bool
	Language                            string
	OutputStyle                         string
	UnsupportedOriginalPromptSettingIDs []string
}

// UnsupportedOriginalPromptSettings documents original settings that are not
// implemented by this compatibility layer. They are fail-closed/no-op here so
// prompt behavior does not silently depend on unsupported global settings.
func UnsupportedOriginalPromptSettings() []string {
	return []string{
		"remoteManagedSettings",
		"growthbookFeatureFlags",
		"settingsSync",
		"hooksMutation",
		"permissionRules",
		"sandbox",
		"statusLine",
		"mcpServers",
		"pluginSettings",
	}
}
