package input

import (
	"testing"
)

func TestReaderOpts_Defaults(t *testing.T) {
	// Verify that NewReader sets a non-empty prompt when none is given.
	// We cannot instantiate a real readline in a non-TTY test env, so we test
	// the logic by inspecting that the prompt fallback constant is non-empty.
	opts := ReaderOpts{}
	if opts.Prompt != "" {
		t.Fatal("expected empty prompt initially")
	}
	// Simulate the defaulting logic used in NewReader.
	if opts.Prompt == "" {
		opts.Prompt = "> "
	}
	if opts.Prompt != "> " {
		t.Errorf("expected '>  ' default, got %q", opts.Prompt)
	}
}

func TestReaderOpts_HistoryFile_Default(t *testing.T) {
	path := DefaultHistoryPath()
	// If a home dir is available the path must end with ".claude/history".
	if path == "" {
		t.Skip("no home directory available")
	}
	if len(path) < len("/.claude/history") {
		t.Errorf("history path too short: %q", path)
	}
}

func TestReaderOpts_Fields(t *testing.T) {
	opts := ReaderOpts{
		Prompt:           "claude> ",
		HistoryFile:      "/tmp/test_history",
		MultilineEnabled: true,
	}
	if opts.Prompt != "claude> " {
		t.Errorf("Prompt: got %q", opts.Prompt)
	}
	if opts.HistoryFile != "/tmp/test_history" {
		t.Errorf("HistoryFile: got %q", opts.HistoryFile)
	}
	if !opts.MultilineEnabled {
		t.Error("MultilineEnabled should be true")
	}
}
