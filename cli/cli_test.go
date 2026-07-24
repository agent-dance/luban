package cli_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/cli"
)

func TestParseArgsAgentsFlag(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--agents", `{"reviewer":{"description":"d","prompt":"p"}}`, "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Agents != `{"reviewer":{"description":"d","prompt":"p"}}` {
		t.Fatalf("expected agents JSON to round-trip, got %q", opts.Agents)
	}
	if !opts.Print || len(opts.Args) != 1 || opts.Args[0] != "hi" {
		t.Fatalf("unexpected parsed options: %#v", opts)
	}
}

func TestParseArgsPromptDumpFlags(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--prompt-dump-json", "--system-prompt", "debug prompt"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.PromptDumpJSON {
		t.Fatalf("PromptDumpJSON = false, want true")
	}
	if opts.SystemPrompt != "debug prompt" {
		t.Fatalf("SystemPrompt = %q", opts.SystemPrompt)
	}
}

func TestParseArgsDebugFileFlag(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--debug-file", "/tmp/deepseek-debug.log", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.DebugFile != "/tmp/deepseek-debug.log" {
		t.Fatalf("DebugFile = %q", opts.DebugFile)
	}
	if _, err := cli.ParseArgs([]string{"--debug", "-p", "hi"}); err == nil {
		t.Fatal("legacy --debug unexpectedly accepted")
	}
}

func TestParseArgsRuntimePromptSettings(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--language", "japanese", "--output-style", "concise", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.Language != "japanese" || opts.OutputStyle != "concise" {
		t.Fatalf("runtime prompt settings mismatch: %#v", opts)
	}
}

func TestParseArgsScreenReaderMode(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--screen-reader"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.ScreenReader || opts.Print {
		t.Fatalf("screen reader options = %#v", opts)
	}
}

func TestScreenReaderRejectsCompetingStdinOwners(t *testing.T) {
	for _, args := range [][]string{
		{"--screen-reader", "--sdk"},
		{"--screen-reader", "--print", "hello"},
	} {
		_, err := cli.ParseArgs(args)
		if err == nil || !strings.Contains(err.Error(), "screen-reader") {
			t.Fatalf("ParseArgs(%v) error = %v, want screen-reader mode conflict", args, err)
		}
	}
}

func TestScreenReaderRequiresTerminalStdin(t *testing.T) {
	opts := cli.Options{ScreenReader: true}
	if err := cli.ValidateInputMode(opts, false); err == nil || !strings.Contains(err.Error(), "terminal stdin") {
		t.Fatalf("ValidateInputMode() error = %v, want terminal stdin requirement", err)
	}
	if err := cli.ValidateInputMode(opts, true); err != nil {
		t.Fatalf("terminal screen-reader mode rejected: %v", err)
	}
}

func TestScreenReaderRejectsMachineOutputRenderer(t *testing.T) {
	_, err := cli.ParseArgs([]string{"--screen-reader", "--output-format", "stream-json"})
	if err == nil || !strings.Contains(err.Error(), "output-format") {
		t.Fatalf("ParseArgs error = %v, want screen-reader output-format conflict", err)
	}
}
