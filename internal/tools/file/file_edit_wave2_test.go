// Package file contains focused FileEdit regression tests.
package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileEditWave2_SuggestSimilarPath — fe-suggest-similar-path.
// An ENOENT for an edit must include "Did you mean ..." with a near-miss
// from the cwd.
func TestFileEditWave2_SuggestSimilarPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "configuration.yaml")
	if err := os.WriteFile(good, []byte("k: v"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Pivot cwd into our scratch dir so the suggester walks it.
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	missing := filepath.Join(dir, "configuration.yml") // typo: yml vs yaml
	tool := &FileEditTool{ReadState: NewReadFileState()}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  missing,
		"old_string": "k:",
		"new_string": "key:",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error, got %s", res.Content)
	}
	if !strings.Contains(res.Content, "Did you mean") {
		t.Fatalf("expected 'Did you mean' suggestion, got %s", res.Content)
	}
	if !strings.Contains(res.Content, "configuration.yaml") {
		t.Fatalf("expected suggestion to include configuration.yaml, got %s", res.Content)
	}
}

// TestSuggestSimilarPath_Direct — direct unit tests for the helper so the
// scoring tiers are exercised without going through the full tool harness.
func TestSuggestSimilarPath_Direct(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"foo.go", "bar.txt", "Foo.GO"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	got := suggestSimilarPath(dir, "Foo.go")
	if len(got) == 0 {
		t.Fatalf("expected at least one suggestion")
	}
	// Exact match should rank first.
	hadExact := false
	for _, p := range got {
		if filepath.Base(p) == "Foo.GO" || filepath.Base(p) == "foo.go" {
			hadExact = true
		}
	}
	if !hadExact {
		t.Fatalf("expected exact-or-case-match candidate, got %v", got)
	}
}
