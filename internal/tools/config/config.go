// Package config exposes runtime configuration reads and writes as a tool.
package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

var knownSettings = []string{
	"model",
	"theme",
	"verbose",
	"max_turns",
	"allowed_tools",
	"permission_mode",
}

// ConfigInput is the complete Config tool input contract. The presence of
// value selects a write; omitting value selects a read.
type ConfigInput struct {
	Setting string `json:"setting"`
	Value   any    `json:"value,omitempty"`
}

// Store is the persistence contract required by ConfigTool.
type Store interface {
	Get(key string) (string, bool)
	Set(key, value string) error
}

// ConfigTool reads and writes runtime configuration settings.
type ConfigTool struct {
	store Store
}

// NewConfigTool creates a Config tool backed by store.
func NewConfigTool(store Store) *ConfigTool {
	return &ConfigTool{store: store}
}

func (t *ConfigTool) Name() string { return "Config" }

func (t *ConfigTool) Description() string {
	return promptText(i18n.KeyToolConfigDescription)
}

func (t *ConfigTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ConcurrencySafe: true}
}

func (t *ConfigTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"setting": map[string]any{
				"type": "string",
				"description": promptFormat(
					i18n.KeyToolConfigSettingDescription,
					strings.Join(knownSettings, ", "),
				),
			},
			"value": map[string]any{
				"description": promptText(i18n.KeyToolConfigValueDescription),
			},
		},
		Required: []string{"setting"},
	}
}

func (t *ConfigTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := toolbase.ParseStrictInputOrError[ConfigInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	setting := strings.TrimSpace(in.Setting)
	if setting == "" {
		return errorResult(i18n.KeyToolConfigSettingRequired), nil
	}

	_, write := input["value"]
	if !write {
		value, ok := t.store.Get(setting)
		if !ok {
			return types.ToolResult{Content: runtimeFormat(i18n.KeyToolConfigNotSet, setting)}, nil
		}
		return types.ToolResult{Content: runtimeFormat(i18n.KeyToolConfigValue, setting, value)}, nil
	}
	if in.Value == nil {
		return errorResult(i18n.KeyToolConfigNullValue), nil
	}

	value := fmt.Sprint(in.Value)
	if err := t.store.Set(setting, value); err != nil {
		return errorResult(i18n.KeyToolConfigSaveFailed, err), nil
	}
	return types.ToolResult{
		Content: runtimeFormat(i18n.KeyToolConfigUpdated, setting, value),
	}, nil
}

func errorResult(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{
		Content: runtimeFormat(key, args...),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}

func promptText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func promptFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func runtimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}
