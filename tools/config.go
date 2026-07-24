package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// knownKeys is the set of recognised config keys. Unknown keys are still
// accepted for extensibility — this list is used only for documentation.
var knownKeys = []string{
	"model",
	"theme",
	"verbose",
	"max_turns",
	"allowed_tools",
	"permission_mode",
}

// configDir returns the directory that holds LUBAN Code configuration.
func configDir() string {
	return brand.UserConfigDir()
}

// ─── ConfigStore ─────────────────────────────────────────────────────────────

// ConfigStore is a concurrency-safe key/value store backed by
// ~/.luban-code/config.json.
type ConfigStore struct {
	mu       sync.Mutex
	path     string
	settings map[string]string
}

// NewConfigStore creates a ConfigStore loaded from ~/.luban-code/config.json.
// If the file does not exist the store starts empty and the file will be
// created on the first Set call.
func NewConfigStore() *ConfigStore {
	path := filepath.Join(configDir(), "config.json")
	cs := &ConfigStore{
		path:     path,
		settings: make(map[string]string),
	}
	_ = cs.load() // ignore error; missing file is fine
	return cs
}

// load reads settings from disk. Acquires the mutex for the entire
// read-unmarshal sequence to prevent H24 race with concurrent Set().
func (c *ConfigStore) load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &c.settings)
}

// save persists settings to disk. Caller MUST hold the mutex.
func (c *ConfigStore) save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0700); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigCreateDirectory, err)
	}
	data, err := json.MarshalIndent(c.settings, "", "  ")
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigMarshal, err)
	}
	if err := os.WriteFile(c.path, data, 0600); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkConfigWrite, err)
	}
	return nil
}

// Get returns the value for key and whether it was present.
func (c *ConfigStore) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.settings[key]
	return v, ok
}

// Set stores value for key and immediately persists the config to disk.
func (c *ConfigStore) Set(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings[key] = value
	return c.save()
}

// All returns a shallow copy of all current settings.
func (c *ConfigStore) All() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.settings))
	for k, v := range c.settings {
		out[k] = v
	}
	return out
}

// ─── ConfigTool ──────────────────────────────────────────────────────────────

// ConfigTool exposes ConfigStore operations as a tool available to the model.
type ConfigTool struct {
	Store *ConfigStore
}

// NewConfigTool returns a ConfigTool backed by the provided store.
func NewConfigTool(store *ConfigStore) *ConfigTool {
	return &ConfigTool{Store: store}
}

func (t *ConfigTool) Name() string      { return "Config" }
func (t *ConfigTool) Aliases() []string { return []string{"ConfigTool"} }

func (t *ConfigTool) Description() string {
	return "Read or write LUBAN Code configuration settings"
}

func (t *ConfigTool) IsConcurrentSafe() bool { return true }

func (t *ConfigTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"setting": map[string]any{
				"type":        "string",
				"description": fmt.Sprintf("Setting key. Known keys: %s", strings.Join(knownKeys, ", ")),
			},
			"value": map[string]any{
				"description": `Value to write. Omit it to read the current setting.`,
			},
		},
		Required: []string{"setting"},
	}
}

func (t *ConfigTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseInputOrError[ConfigInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	key := strings.TrimSpace(in.Setting)
	if key == "" {
		key = strings.TrimSpace(in.Key)
	}
	action := strings.TrimSpace(in.Action)
	if action == "" {
		if in.Value == nil {
			action = "get"
		} else {
			action = "set"
		}
	}

	switch action {
	case "get":
		if key == "" {
			all := t.Store.All()
			if len(all) == 0 {
				return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyAConfigEmpty)}, nil
			}
			var sb strings.Builder
			for _, k := range knownKeys {
				if v, ok := all[k]; ok {
					fmt.Fprintf(&sb, "%s = %s\n", k, v)
					delete(all, k)
				}
			}
			// Any extra (unknown) keys come after the known ones.
			for k, v := range all {
				fmt.Fprintf(&sb, "%s = %s\n", k, v)
			}
			return types.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
		}
		v, ok := t.Store.Get(key)
		if !ok {
			return types.ToolResult{
				Content: toolRuntimeFormat(i18n.KeyToolLegacyAConfigNotSet, key),
			}, nil
		}
		return types.ToolResult{Content: fmt.Sprintf("%s = %s", key, v)}, nil

	case "set":
		if key == "" {
			return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolLegacyAConfigSetRequired), IsError: true}, nil
		}
		value := fmt.Sprint(in.Value)
		if value == "<nil>" {
			value = ""
		}
		if err := t.Store.Set(key, value); err != nil {
			return types.ToolResult{
				Content: toolRuntimeFormat(i18n.KeyToolLegacyAConfigSaveFailed, err),
				IsError: true,
			}, nil
		}
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolLegacyAConfigUpdated, key, value),
		}, nil

	default:
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolLegacyAConfigUnknownAction, action),
			IsError: true,
		}, nil
	}
}
