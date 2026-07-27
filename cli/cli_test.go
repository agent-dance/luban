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

func TestParseArgsReasoningEffort(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--reasoning-effort", "xhigh", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.ReasoningEffort != "xhigh" {
		t.Fatalf("ReasoningEffort = %q, want xhigh", opts.ReasoningEffort)
	}
}

func TestParseArgsPinnedModelAliases(t *testing.T) {
	for _, flag := range []string{"--pinned-model", "--no-model-fallback"} {
		opts, err := cli.ParseArgs([]string{flag, "-p", "hi"})
		if err != nil {
			t.Fatalf("ParseArgs(%q): %v", flag, err)
		}
		if !opts.PinnedModel {
			t.Fatalf("PinnedModel = false for %s", flag)
		}
	}
}

func TestParseArgsServiceTierPin(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--service-tier", "default", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if opts.ServiceTier != "default" {
		t.Fatalf("ServiceTier = %q, want default", opts.ServiceTier)
	}
	omitted, err := cli.ParseArgs([]string{"-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs omitted service tier: %v", err)
	}
	if omitted.ServiceTier != "" {
		t.Fatalf("omitted ServiceTier = %q, want empty", omitted.ServiceTier)
	}
	for _, value := range []string{"auto", "DEFAULT", " default "} {
		if _, err := cli.ParseArgs([]string{"--service-tier", value, "-p", "hi"}); err == nil {
			t.Fatalf("non-canonical service tier %q unexpectedly accepted", value)
		}
	}
}

func TestParseArgsResponsesWebSocketIsExplicitAndRequiresResponses(t *testing.T) {
	enabled, err := cli.ParseArgs([]string{"--responses-websocket", "--api", "responses", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs enabled: %v", err)
	}
	if !enabled.ResponsesWebSocket {
		t.Fatal("ResponsesWebSocket = false, want explicit opt-in")
	}

	omitted, err := cli.ParseArgs([]string{"--api", "responses", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs omitted: %v", err)
	}
	if omitted.ResponsesWebSocket {
		t.Fatal("omitted ResponsesWebSocket unexpectedly enabled")
	}

	for _, api := range []string{"chat-completions", "messages"} {
		if _, err := cli.ParseArgs([]string{"--responses-websocket", "--api", api, "-p", "hi"}); err == nil {
			t.Fatalf("Responses WebSocket accepted conflicting API format %q", api)
		}
	}
}

func TestForceSandboxToolsImpliesSandbox(t *testing.T) {
	opts, err := cli.ParseArgs([]string{"--force-sandbox-tools", "-p", "hi"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !opts.ForceSandboxTools || !opts.Sandbox {
		t.Fatalf("force sandbox options = %#v", opts)
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
