package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestFileEditIdenticalStrings(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("hello world\n"), 0644)

	editTool := &FileEditTool{}
	result, _ := editTool.Execute(context.Background(), map[string]any{
		"file_path":  fp,
		"old_string": "hello",
		"new_string": "hello",
	})
	if result.IsError || result.Outcome != types.ToolOutcomeSucceeded || result.Metadata["semanticCategory"] != "unchanged" {
		t.Errorf("expected successful unchanged result, got %+v", result)
	}
}

func TestFileEditReplaceAll(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	originalContent := "aaa bbb aaa ccc aaa\n"
	os.WriteFile(fp, []byte(originalContent), 0644)

	state := NewReadFileState()
	abs, _ := filepath.Abs(fp)
	recordStrongReadEvidenceForTest(t, state, abs)

	editTool := &FileEditTool{ReadState: state}
	result, _ := editTool.Execute(context.Background(), map[string]any{
		"file_path":   fp,
		"old_string":  "aaa",
		"new_string":  "zzz",
		"replace_all": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := os.ReadFile(fp)
	content := string(data)
	if strings.Contains(content, "aaa") {
		t.Error("expected all 'aaa' to be replaced")
	}
	if strings.Count(content, "zzz") != 3 {
		t.Errorf("expected 3 'zzz' replacements, got %d", strings.Count(content, "zzz"))
	}
}

func TestGrepToolWithGlobFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("findme\n"), 0644)
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("findme\n"), 0644)

	tool := &GrepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
		"glob":    "*.go",
	})
	if !strings.Contains(result.Content, "code.go") {
		t.Error("expected code.go in results")
	}
	if strings.Contains(result.Content, "data.txt") {
		t.Error("data.txt should be filtered out by glob")
	}
}

func TestGlobToolSimplePattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0644)

	tool := &GlobTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if !strings.Contains(result.Content, "a.go") {
		t.Error("expected a.go in results")
	}
	if strings.Contains(result.Content, "b.txt") {
		t.Error("b.txt should not match *.go pattern")
	}
}
