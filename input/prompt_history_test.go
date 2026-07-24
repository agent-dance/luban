package input

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptHistoryLoadsCurrentSessionBeforeProjectHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	entries := []PromptHistoryEntry{
		{Display: "other session older", Project: "/project", SessionID: "session-b"},
		{Display: "current session older", Project: "/project", SessionID: "session-a"},
		{Display: "different project", Project: "/other", SessionID: "session-a"},
		{Display: "other session newer", Project: "/project", SessionID: "session-b"},
		{Display: "current session newer", Project: "/project", SessionID: "session-a"},
	}
	for _, entry := range entries {
		if err := AppendPromptHistory(path, entry); err != nil {
			t.Fatalf("append prompt history: %v", err)
		}
	}

	got := LoadPromptHistory(path, "/project", "session-a", 100)
	want := []string{
		"current session newer",
		"current session older",
		"other session newer",
		"other session older",
	}
	if len(got) != len(want) {
		t.Fatalf("loaded entries = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Display != want[i] {
			t.Fatalf("entry[%d] = %q, want %q", i, got[i].Display, want[i])
		}
	}
}

func TestPromptHistoryRoundTripsMultilineAndSkipsMalformedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "first line\nsecond line\n第三行"
	if err := AppendPromptHistory(path, PromptHistoryEntry{
		Display: want, Project: "/project", SessionID: "session-a",
	}); err != nil {
		t.Fatalf("append prompt history: %v", err)
	}

	got := LoadPromptHistory(path, "/project", "session-a", 100)
	if len(got) != 1 || got[0].Display != want {
		t.Fatalf("loaded multiline history = %+v, want %q", got, want)
	}
}

func TestPromptHistorySuppressesConsecutiveDuplicateInSameSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	entry := PromptHistoryEntry{Display: "same prompt", Project: "/project", SessionID: "session-a"}
	if err := AppendPromptHistory(path, entry); err != nil {
		t.Fatal(err)
	}
	if err := AppendPromptHistory(path, entry); err != nil {
		t.Fatal(err)
	}
	entry.SessionID = "session-b"
	if err := AppendPromptHistory(path, entry); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("history line count = %d, want 2: %q", len(lines), data)
	}
}

func TestPromptHistoryLoadHonorsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	for _, display := range []string{"one", "two", "three"} {
		if err := AppendPromptHistory(path, PromptHistoryEntry{
			Display: display, Project: "/project", SessionID: "session-a",
		}); err != nil {
			t.Fatal(err)
		}
	}

	got := LoadPromptHistory(path, "/project", "session-a", 2)
	if len(got) != 2 || got[0].Display != "three" || got[1].Display != "two" {
		t.Fatalf("limited history = %+v, want three then two", got)
	}
}

func TestDefaultPromptHistoryPathUsesJSONLFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := DefaultPromptHistoryPath()
	if filepath.Base(got) != "history.jsonl" {
		t.Fatalf("history path = %q, want history.jsonl", got)
	}
}

func TestPromptHistorySkipsOversizedRecordAndContinues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	oversized := strings.Repeat("x", maxPromptHistoryRecordBytes+1) + "\n"
	if err := os.WriteFile(path, []byte(oversized), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendPromptHistory(path, PromptHistoryEntry{
		Display: "valid after oversized", Project: "/project", SessionID: "session-a",
	}); err != nil {
		t.Fatal(err)
	}

	got := LoadPromptHistory(path, "/project", "session-a", 100)
	if len(got) != 1 || got[0].Display != "valid after oversized" {
		t.Fatalf("history after oversized record = %+v", got)
	}
}
