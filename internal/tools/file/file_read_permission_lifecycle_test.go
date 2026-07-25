package file

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestFileReadPermissionUsesRuntimeAllowedDirectories(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	request := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root,
		AllowedDirs: []string{root},
	}}
	tool := &FileReadTool{AllowedDirs: []string{root}}

	allowed, err := tool.CheckPermissions(context.Background(), map[string]any{"file_path": inside}, request)
	if err != nil || allowed.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("inside read decision = %+v, err=%v", allowed, err)
	}

	asked, err := tool.CheckPermissions(context.Background(), map[string]any{"file_path": outside}, request)
	if err != nil || asked.Behavior != types.PermissionBehaviorAsk || asked.BlockedPath == "" {
		t.Fatalf("outside read decision = %+v, err=%v", asked, err)
	}
}
