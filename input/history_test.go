package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultHistoryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p := DefaultHistoryPath()
	if p == "" {
		t.Skip("no home directory available")
	}
	if filepath.Base(p) != "history" {
		t.Errorf("expected base name 'history', got %q", filepath.Base(p))
	}
	if filepath.Base(filepath.Dir(p)) != ".luban-code" {
		t.Errorf("expected parent dir '.luban-code', got %q", filepath.Base(filepath.Dir(p)))
	}
}

func TestDefaultHistoryPathMigratesLegacyDeepSeekHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	legacy := filepath.Join(home, ".deepseek-code", "history")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old command\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	current := DefaultHistoryPath()
	if current != filepath.Join(home, ".luban-code", "history") {
		t.Fatalf("history path = %q", current)
	}
	data, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read migrated history: %v", err)
	}
	if string(data) != "old command\n" {
		t.Fatalf("migrated history = %q", data)
	}
}

func TestDeduplicateConsecutive(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "a"}, []string{"a"}},
		{[]string{"a", "a", "b"}, []string{"a", "b"}},
		{[]string{"a", "b", "a"}, []string{"a", "b", "a"}},
		{[]string{"a", "a", "a"}, []string{"a"}},
	}
	for _, tc := range tests {
		got := deduplicateConsecutive(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("input %v: got %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("input %v: got %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestAppendAndLoadHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "history")

	// Append a few entries.
	entries := []string{"ls -la", "git status", "git status", "echo hello"}
	for _, e := range entries {
		if err := AppendHistory(path, e); err != nil {
			t.Fatalf("AppendHistory(%q): %v", e, err)
		}
	}

	got := LoadHistory(path)
	// Consecutive duplicates should be removed: "git status" appears twice.
	want := []string{"ls -la", "git status", "echo hello"}
	if len(got) != len(want) {
		t.Fatalf("LoadHistory: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entry[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAppendHistoryMaxEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	// Write maxHistoryEntries+10 distinct entries.
	total := maxHistoryEntries + 10
	for i := 0; i < total; i++ {
		entry := filepath.Join("cmd", string(rune('a'+i%26)), string(rune('A'+i/26%26)))
		if err := AppendHistory(path, entry); err != nil {
			t.Fatalf("AppendHistory: %v", err)
		}
	}

	got := LoadHistory(path)
	if len(got) > maxHistoryEntries {
		t.Errorf("expected at most %d entries, got %d", maxHistoryEntries, len(got))
	}
}

func TestLoadHistoryMissingFile(t *testing.T) {
	lines := LoadHistory("/nonexistent/path/to/history")
	if lines != nil {
		t.Errorf("expected nil for missing file, got %v", lines)
	}
}

func TestWriteLinesAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	lines := []string{"one", "two", "three"}
	if err := writeLines(path, lines); err != nil {
		t.Fatalf("writeLines: %v", err)
	}

	got := LoadHistory(path)
	if len(got) != len(lines) {
		t.Fatalf("got %v, want %v", got, lines)
	}

	// Ensure no temp file is left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file should have been renamed away")
	}
}
