package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

type fileWriteRegressionRuntime struct {
	snapshot types.ToolRuntimeContext
}

func (r fileWriteRegressionRuntime) ToolRuntimeContext() types.ToolRuntimeContext {
	return r.snapshot
}

func requireFileWriteResult(t *testing.T, result types.ToolResult, err error) FileWriteResult {
	t.Helper()
	if err != nil {
		t.Fatalf("Write returned infrastructure error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Write failed: %s", result.Content)
	}
	value, ok := result.Data.(FileWriteResult)
	if !ok {
		t.Fatalf("Write omitted typed result: %#v", result.Data)
	}
	return value
}

func TestFileWriteRelativePathUsesRuntimeProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	processCWD := t.TempDir()
	t.Chdir(processCWD)
	tool := &FileWriteTool{
		Runtime: fileWriteRegressionRuntime{snapshot: types.ToolRuntimeContext{
			ProjectRoot: projectRoot,
			AllowedDirs: []string{projectRoot},
		}},
		ReadState: NewReadFileState(),
	}

	toolResult, err := tool.Execute(context.Background(), map[string]any{
		"file_path": filepath.Join("nested", "game.js"),
		"content":   "export const ready = true;\n",
	})
	result := requireFileWriteResult(t, toolResult, err)
	want := filepath.Join(projectRoot, "nested", "game.js")
	if result.FilePath != want {
		t.Fatalf("Write path = %q, want runtime-relative %q", result.FilePath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("runtime-relative output missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(processCWD, "nested", "game.js")); !os.IsNotExist(err) {
		t.Fatalf("Write used process cwd instead of runtime project root: %v", err)
	}
}

func TestFileWriteExistingEmptyFileIsUpdate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	seedCanonicalFileReadState(t, state, path)
	tool := &FileWriteTool{AllowedDirs: []string{root}, ReadState: state}

	toolResult, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path,
		"content":   "first line\n",
	})
	result := requireFileWriteResult(t, toolResult, err)
	if result.Type != "update" {
		t.Fatalf("existing empty file type = %q, want update", result.Type)
	}
	if result.OriginalFile == nil || *result.OriginalFile != "" {
		t.Fatalf("existing empty file originalFile = %#v, want pointer to empty string", result.OriginalFile)
	}
	if len(result.StructuredPatch) == 0 {
		t.Fatal("existing empty file update must include a structured patch")
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != "first line\n" {
		t.Fatalf("written content = %q, want %q", written, "first line\\n")
	}
}

func TestFileWritePermissionUsesCanonicalWriteIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "game.js")
	tool := &FileWriteTool{AllowedDirs: []string{root}}
	input := map[string]any{"file_path": path, "content": ""}

	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot:  root,
		AllowedDirs:  []string{root},
		AllowedRules: []types.PermissionRuleValue{{ToolName: "Edit", RuleContent: path}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior == types.PermissionBehaviorAllow {
		t.Fatal("an Edit rule must not authorize Write")
	}
	if len(decision.Suggestions) != 1 || len(decision.Suggestions[0].Rules) != 1 || decision.Suggestions[0].Rules[0].ToolName != "Write" {
		t.Fatalf("Write permission suggestion must use canonical Write identity: %#v", decision.Suggestions)
	}

	allowed, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot:  root,
		AllowedDirs:  []string{root},
		AllowedRules: []types.PermissionRuleValue{{ToolName: "Write", RuleContent: path}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("canonical Write rule was not honored: %#v", allowed)
	}
}

func TestFilePermissionPathRejectsHistoricalAlias(t *testing.T) {
	if got := permissionFilePath(map[string]any{"path": "/tmp/legacy", "file_path": "  /tmp/current  "}); got != "/tmp/current" {
		t.Fatalf("file_path normalization = %q, want /tmp/current", got)
	}
	if got := permissionFilePath(map[string]any{"path": "/tmp/legacy"}); got != "" {
		t.Fatalf("historical path alias was accepted: %q", got)
	}
	const current = "/tmp/current"
	if _, ok := matchingFileWriteRule([]string{current}, []types.PermissionRuleValue{{ToolName: "Write", RuleContent: current}}); !ok {
		t.Fatal("canonical Write identity was rejected")
	}
	for _, name := range []string{"Edit", "edit", "write", "FileEdit", "FileWrite"} {
		if _, ok := matchingFileWriteRule([]string{current}, []types.PermissionRuleValue{{ToolName: name, RuleContent: current}}); ok {
			t.Fatalf("historical tool identity %q was accepted", name)
		}
	}
}
