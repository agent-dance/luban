package agent

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

type agentCWDLocalAllowTool struct {
	name     string
	metadata types.ToolMetadata
}

func (t *agentCWDLocalAllowTool) Name() string        { return t.name }
func (t *agentCWDLocalAllowTool) Description() string { return "test local allow tool" }
func (t *agentCWDLocalAllowTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}
func (t *agentCWDLocalAllowTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return t.metadata
}
func (t *agentCWDLocalAllowTool) CheckPermissions(_ context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
}
func (t *agentCWDLocalAllowTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}
func (t *agentCWDLocalAllowTool) MapToolResultToToolResultBlock(any, string) types.ToolResultBlock {
	return types.ToolResultBlock{}
}

func TestAgentCWDReadWrapperPreservesToolLocalReadOnlyAllow(t *testing.T) {
	root := t.TempDir()
	tool := &agentCWDLocalAllowTool{name: "Read", metadata: types.ToolMetadata{ReadOnly: true}}
	wrapper := &agentCWDReadToolWrapper{agentCWDToolWrapper: &agentCWDToolWrapper{base: root, tool: tool}}

	result, err := wrapper.CheckPermissions(context.Background(), map[string]any{"file_path": "note.txt"}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Behavior != types.PermissionBehaviorPassthrough || !result.ToolLocalReadOnlyAllow {
		t.Fatalf("Read wrapper permission=%+v, want delegated local read-only allow", result)
	}
}

func TestAgentCWDLifecycleWrapperOnlyMarksReadOnlyLocalAllows(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata types.ToolMetadata
		want     bool
	}{
		{name: "Glob", metadata: types.ToolMetadata{ReadOnly: true, Search: true}, want: true},
		{name: "Grep", metadata: types.ToolMetadata{ReadOnly: true, Search: true}, want: true},
		{name: "Write", metadata: types.ToolMetadata{Write: true}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := &agentCWDLocalAllowTool{name: test.name, metadata: test.metadata}
			wrapper := &agentCWDLifecycleToolWrapper{agentCWDToolWrapper: &agentCWDToolWrapper{base: t.TempDir(), tool: tool}}

			result, err := wrapper.CheckPermissions(context.Background(), map[string]any{"path": "."}, types.ToolPermissionRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if result.Behavior != types.PermissionBehaviorPassthrough || result.ToolLocalReadOnlyAllow != test.want {
				t.Fatalf("%s wrapper permission=%+v, want local read-only allow=%v", test.name, result, test.want)
			}
		})
	}
}
