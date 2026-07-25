package compact_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/compact"
	toolsearch "github.com/agent-dance/luban/internal/tools/search"
	"github.com/agent-dance/luban/types"
)

func TestResultStoreGrepUses20KToolThreshold(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "large.txt")
	if err := os.WriteFile(file, []byte(strings.Repeat("needle payload\n", 2_000)), 0o644); err != nil {
		t.Fatalf("write Grep fixture: %v", err)
	}
	tool := toolsearch.NewGrep(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "needle", "path": file, "output_mode": "content", "head_limit": float64(0),
	})
	if err != nil || result.IsError {
		t.Fatalf("execute Grep: err=%v result=%#v", err, result)
	}
	block := types.MapToolResult(tool, result, "toolu_grep_large")
	if block.Metadata["maxResultSizeChars"] != "20000" {
		t.Fatalf("Grep threshold metadata mismatch: %#v", block.Metadata)
	}
	storeDir := t.TempDir()
	store := compact.NewResultStore(storeDir)
	processed, err := store.ProcessResultForTool(block, "Grep")
	if err != nil {
		t.Fatalf("persist Grep result: %v", err)
	}
	if !strings.Contains(processed.Content, "Full output saved to") {
		t.Fatalf("oversized Grep output was not persisted: %q", processed.Content)
	}
	if _, err := os.Stat(filepath.Join(storeDir, "tool-results", "toolu_grep_large.txt")); err != nil {
		t.Fatalf("expected persisted Grep text: %v", err)
	}
}
