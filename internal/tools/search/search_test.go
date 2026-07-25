package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestGlobToolDeepPattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "deep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "top.go"), []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if !strings.Contains(result.Content, "deep.go") {
		t.Error("expected deep.go in ** results")
	}
	if !strings.Contains(result.Content, "top.go") {
		t.Error("expected top.go in ** results")
	}
}

func TestGlobToolNativeFallbackDeepPattern(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "deep.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "top.go"), []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected fallback glob error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deep.go") || !strings.Contains(result.Content, "top.go") {
		t.Fatalf("expected native fallback to match top and deep files, got %q", result.Content)
	}
}

func TestGrepToolNoMatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "nonexistent_pattern",
		"path":    dir,
	})
	if result.IsError {
		t.Error("no match should not be an error")
	}
	if result.Content != toolRuntimeText(i18n.KeyToolSearchNoFiles) {
		t.Errorf("expected no files message, got '%s'", result.Content)
	}
}

func TestGrepToolContentMode(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "code.go")
	os.WriteFile(fp, []byte("func main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "Println",
		"path":        fp,
		"output_mode": "content",
	})
	if !strings.Contains(result.Content, "Println") {
		t.Errorf("expected content output with match, got '%s'", result.Content)
	}
}

func TestGrepToolCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("Hello World\nhello world\nHELLO WORLD\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "hello",
		"-i":          true,
		"path":        fp,
		"output_mode": "count",
	})
	if !strings.Contains(result.Content, "3") {
		t.Errorf("expected 3 case-insensitive matches, got '%s'", result.Content)
	}
}

func TestGrepToolWithContextLines(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("line1\nline2\ntarget\nline4\nline5\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "target",
		"path":        fp,
		"output_mode": "content",
		"-C":          float64(1),
	})
	if !strings.Contains(result.Content, "line2") {
		t.Error("expected context line before match")
	}
	if !strings.Contains(result.Content, "line4") {
		t.Error("expected context line after match")
	}
}

func TestGrepToolNativeFallbackContentAndContext(t *testing.T) {
	withUnavailableRipgrep(t)
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("line1\nline2\ntarget\nline4\nline5\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "target",
		"path":        fp,
		"output_mode": "content",
		"-C":          float64(1),
	})
	if result.IsError {
		t.Fatalf("unexpected fallback grep error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "line2") || !strings.Contains(result.Content, "line4") {
		t.Fatalf("expected native fallback context lines, got %q", result.Content)
	}
}

func TestGrepToolHeadLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, strings.Repeat("a", i+1)+".txt")
		os.WriteFile(name, []byte("findme\n"), 0644)
	}

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":    "findme",
		"path":       dir,
		"head_limit": float64(2),
	})
	lines := grepResultPayloadLines(result)
	if len(lines) > 2 {
		t.Errorf("expected at most 2 results with head_limit=2, got %d", len(lines))
	}
}

func TestGlobToolNoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.xyz",
		"path":    dir,
	})
	if result.IsError {
		t.Error("no matches should not be an error")
	}
	if result.Content != toolRuntimeText(i18n.KeyToolSearchNoFiles) {
		t.Errorf("expected exact no files message, got '%s'", result.Content)
	}
}

func TestGrepToolDefaultHeadLimitIsApplied(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 255; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file-%03d.txt", i))
		os.WriteFile(name, []byte("findme\n"), 0644)
	}

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := grepResultPayloadLines(result)
	if got := len(lines); got != defaultGrepHeadLimit {
		t.Fatalf("expected default head limit %d, got %d", defaultGrepHeadLimit, got)
	}
}

func grepResultPayloadLines(result types.ToolResult) []string {
	if output, ok := result.Data.(grepOutput); ok && output.Mode == "files_with_matches" {
		return append([]string(nil), output.Filenames...)
	}
	return strings.Split(strings.TrimSpace(result.Content), "\n")
}

func TestGrepToolIncludesHiddenFilesButExcludesVCS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden.txt"), []byte("findme\n"), 0644)
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)
	os.WriteFile(filepath.Join(gitDir, "config"), []byte("findme\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".hidden.txt") {
		t.Fatalf("expected hidden file to be included, got %q", result.Content)
	}
	if strings.Contains(result.Content, ".git") {
		t.Fatalf("expected VCS directory to be excluded, got %q", result.Content)
	}
}

func TestGrepToolPatternStartingWithDash(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "flags.txt")
	os.WriteFile(fp, []byte("-flag\nother\n"), 0644)

	tool := &grepTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern":     "-flag",
		"path":        fp,
		"output_mode": "content",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "-flag") {
		t.Fatalf("expected dashed pattern to match, got %q", result.Content)
	}
}

func TestGlobToolAbsolutePatternUsesBaseDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0755)
	target := filepath.Join(sub, "deep.go")
	os.WriteFile(target, []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": filepath.Join(dir, "**", "*.go"),
		"path":    t.TempDir(),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deep.go") {
		t.Fatalf("expected absolute pattern to find deep.go, got %q", result.Content)
	}
}

func TestGlobToolIncludesHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden.txt"), []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, ".hidden.txt") {
		t.Fatalf("expected hidden file in results, got %q", result.Content)
	}
}

func TestGlobToolRejectsNonDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "single.txt")
	os.WriteFile(fp, []byte("x"), 0644)

	tool := &globTool{}
	result, _ := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    fp,
	})
	if !result.IsError {
		t.Fatalf("expected non-directory path to error")
	}
	if !strings.Contains(result.Content, fp) {
		t.Fatalf("expected directory validation error, got %q", result.Content)
	}
}

func TestGrepToolMissingPathSuggestsNearbyFile(t *testing.T) {
	tool := &grepTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	expected := filepath.Join(tmpDir, "config.txt")
	if err := os.WriteFile(expected, []byte("findme\n"), 0644); err != nil {
		t.Fatalf("write suggested file: %v", err)
	}

	result, _ := tool.Execute(ctx, map[string]any{
		"pattern": "findme",
		"path":    filepath.Join(tmpDir, "cnfig.txt"),
	})
	if !result.IsError {
		t.Fatalf("expected missing-path error")
	}
	if !strings.Contains(result.Content, expected) {
		t.Fatalf("expected nearby-path suggestion, got %q", result.Content)
	}
}
