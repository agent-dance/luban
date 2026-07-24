package tools

import (
	"context"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// TestingPermissionTool is a testing-only tool that always routes through an
// interactive permission check before executing.
type TestingPermissionTool struct{}

func (t *TestingPermissionTool) Name() string { return "TestingPermission" }

func (t *TestingPermissionTool) Description() string {
	return "Test tool that always asks for permission"
}

func (t *TestingPermissionTool) IsConcurrentSafe() bool { return true }

func (t *TestingPermissionTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
	}
}

func (t *TestingPermissionTool) Execute(_ context.Context, _ map[string]any) (types.ToolResult, error) {
	return StringResponse(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDTestingPermissionSucceeded))
}
