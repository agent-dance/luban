package mcp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
)

func TestMCPOutputStorageUsesExecutionSessionOutsideProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	ctxA := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		ProjectRoot: project, SessionID: "session-a",
	})
	ctxB := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		ProjectRoot: project, SessionID: "session-b",
	})
	dirA := mcpToolResultsDirForContext(ctxA)
	dirB := mcpToolResultsDirForContext(ctxB)
	if dirA == dirB {
		t.Fatalf("MCP sessions share output directory %q", dirA)
	}
	result := persistMCPTextOutputAt("private", "session-result", false, dirA)
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if _, err := os.Lstat(filepath.Join(project, ".luban-code")); !os.IsNotExist(err) {
		t.Fatalf("MCP persistence dirtied project: %v", err)
	}
	for path, want := range map[string]fs.FileMode{dirA: 0o700, result.Filepath: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode(%s) = %04o, want %04o", path, got, want)
		}
	}
}
