package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepToolGlobFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "code.go"), []byte("findme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("findme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrep(nil).Execute(context.Background(), map[string]any{
		"pattern": "findme",
		"path":    dir,
		"glob":    "*.go",
	})
	if err != nil || result.IsError {
		t.Fatalf("Grep failed: err=%v result=%+v", err, result)
	}
	if !strings.Contains(result.Content, "code.go") {
		t.Fatalf("Grep omitted matching Go file: %q", result.Content)
	}
	if strings.Contains(result.Content, "data.txt") {
		t.Fatalf("Grep included file excluded by glob: %q", result.Content)
	}
}

func TestGlobToolSimplePatternFiltersExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGlob(nil).Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if err != nil || result.IsError {
		t.Fatalf("Glob failed: err=%v result=%+v", err, result)
	}
	if !strings.Contains(result.Content, "a.go") {
		t.Fatalf("Glob omitted matching Go file: %q", result.Content)
	}
	if strings.Contains(result.Content, "b.txt") {
		t.Fatalf("Glob included file with a different extension: %q", result.Content)
	}
}
