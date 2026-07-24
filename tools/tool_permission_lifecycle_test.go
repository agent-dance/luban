package tools

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestBashCheckPermissionsAllowsReadAndAsksForWrite(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	runtime := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{ProjectRoot: tool.CWD, AllowedDirs: []string{tool.CWD}}}

	read, err := tool.CheckPermissions(context.Background(), map[string]any{"command": "cat README.md"}, runtime)
	if err != nil || read.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("read-only Bash decision = %+v, err=%v", read, err)
	}
	write, err := tool.CheckPermissions(context.Background(), map[string]any{"command": "mkdir build"}, runtime)
	if err != nil || write.Behavior != types.PermissionBehaviorPassthrough || len(write.Suggestions) == 0 {
		t.Fatalf("write Bash decision = %+v, err=%v", write, err)
	}
}

func TestWebSearchCheckPermissionsSuggestsLocalSettingsRule(t *testing.T) {
	tool := NewWebSearchTool(nil)
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{"query": "golang"}, types.ToolPermissionRequest{})
	if err != nil || decision.Behavior != types.PermissionBehaviorPassthrough {
		t.Fatalf("decision = %+v, err=%v", decision, err)
	}
	if len(decision.Suggestions) != 1 || decision.Suggestions[0].Destination != types.PermissionDestinationLocalSettings {
		t.Fatalf("missing local-settings suggestion: %+v", decision.Suggestions)
	}
}

func TestFilePermissionChecksUseRuntimeAllowedDirectories(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	req := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}}}

	read := &FileReadTool{AllowedDirs: []string{root}}
	allowed, err := read.CheckPermissions(context.Background(), map[string]any{"file_path": inside}, req)
	if err != nil || allowed.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("inside read decision = %+v, err=%v", allowed, err)
	}
	asked, err := read.CheckPermissions(context.Background(), map[string]any{"file_path": outside}, req)
	if err != nil || asked.Behavior != types.PermissionBehaviorAsk || asked.BlockedPath == "" {
		t.Fatalf("outside read decision = %+v, err=%v", asked, err)
	}

	write := &FileWriteTool{AllowedDirs: []string{root}}
	writeDecision, err := write.CheckPermissions(context.Background(), map[string]any{"file_path": inside}, req)
	if err != nil || writeDecision.Behavior != types.PermissionBehaviorPassthrough || len(writeDecision.Suggestions) == 0 {
		t.Fatalf("inside write decision = %+v, err=%v", writeDecision, err)
	}
}

func TestRuntimeScopeEmptyAllowedToolsClearsWhitelist(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetAllowedTools([]string{"Read"})
	if allowed := scope.ToolRuntimeContext().AllowedTools; !allowed["Read"] {
		t.Fatalf("explicit whitelist = %v, want Read", allowed)
	}

	scope.SetAllowedTools([]string{"   "})
	if allowed := scope.ToolRuntimeContext().AllowedTools; allowed != nil {
		t.Fatalf("empty normalized whitelist = %v, want nil (unrestricted)", allowed)
	}
}
