package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type EnvGetTool struct{}

func (t *EnvGetTool) Name() string {
	return "EnvGet"
}

func (t *EnvGetTool) Description() string {
	return "Get the value of an environment variable"
}

func (t *EnvGetTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the environment variable",
			},
		},
		Required: []string{"name"},
	}
}

func (t *EnvGetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	name, err := MustGetStringField(input, "name")
	if err != nil {
		return ErrorResponse(err), nil
	}

	value, exists := os.LookupEnv(name)
	if !exists {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCEnvNotSet, name)), nil
	}

	return ResponseJSON(map[string]string{
		"name":  name,
		"value": value,
	})
}

type EnvSetTool struct{}

func (t *EnvSetTool) Name() string {
	return "EnvSet"
}

func (t *EnvSetTool) Description() string {
	return "Set an environment variable"
}

func (t *EnvSetTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the environment variable",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Value to set",
			},
		},
		Required: []string{"name", "value"},
	}
}

func (t *EnvSetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	name, err := MustGetStringField(input, "name")
	if err != nil {
		return ErrorResponse(err), nil
	}

	value, err := MustGetStringField(input, "value")
	if err != nil {
		return ErrorResponse(err), nil
	}

	if err := os.Setenv(name, value); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCSetEnvFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"status": "success",
		"name":   name,
		"value":  value,
	})
}

type EnvListTool struct{}

func (t *EnvListTool) Name() string {
	return "EnvList"
}

func (t *EnvListTool) Description() string {
	return "List all environment variables"
}

func (t *EnvListTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"filter": map[string]any{
				"type":        "string",
				"description": "Optional filter pattern",
			},
		},
		Required: []string{},
	}
}

func (t *EnvListTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	filter := GetStringField(input, "filter", "")

	vars := os.Environ()
	var results []map[string]string

	for _, envVar := range vars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[0]
		value := parts[1]

		if filter != "" && !strings.Contains(name, filter) {
			continue
		}

		results = append(results, map[string]string{
			"name":  name,
			"value": value,
		})
	}

	return ResponseJSON(map[string]any{
		"count": len(results),
		"vars":  results,
	})
}

type PwdTool struct{}

func (t *PwdTool) Name() string {
	return "Pwd"
}

func (t *PwdTool) Description() string {
	return "Print the current working directory"
}

func (t *PwdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *PwdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCGetCWDFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"cwd": cwd,
	})
}

type CwdTool struct{}

func (t *CwdTool) Name() string {
	return "Cwd"
}

func (t *CwdTool) Description() string {
	return "Get the current working directory"
}

func (t *CwdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *CwdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCGetCWDFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"cwd": cwd,
	})
}

type WdTool struct{}

func (t *WdTool) Name() string {
	return "Wd"
}

func (t *WdTool) Description() string {
	return "Get the working directory"
}

func (t *WdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *WdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCGetWDFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"wd": cwd,
	})
}

type CdTool struct {
	AllowedDirs []string
}

func (t *CdTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *CdTool) Name() string {
	return "Cd"
}

func (t *CdTool) Description() string {
	return "Change the current working directory"
}

func (t *CdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to change to",
			},
		},
		Required: []string{"path"},
	}
}

func (t *CdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	path, err := MustGetStringField(input, "path")
	if err != nil {
		return ErrorResponse(err), nil
	}

	if err := checkAllowedPath(path, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCResolvePathFailed, err)), nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCAccessDirectoryFailed, err)), nil
	}

	if !info.IsDir() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathIsNotDirectory, absPath)), nil
	}

	if err := os.Chdir(absPath); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCChangeDirectoryFailed, err)), nil
	}

	newCwd, _ := os.Getwd()
	return ResponseJSON(map[string]string{
		"status": "success",
		"path":   newCwd,
	})
}
