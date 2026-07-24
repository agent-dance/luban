package tools

// Alignment red tests for the BashTool, derived from alignment_audit.md.
//
// These tests pin the *desired* contract against the TS reference
// implementation (BashTool.tsx). Each test corresponds to a specific gap
// listed in the audit and is expected to FAIL against the current Go
// implementation. They compile cleanly so they can run via:
//
//	go test -run BashAlignment -count=1 ./gosrc/tools/...
//
// Do NOT modify production code to make these pass without first reviewing
// the corresponding audit row.

import (
	"context"
	"strings"
	"testing"
)

// -------- Schema-shape gaps (bash.go:68-95) --------

// Gap: run_in_background field must be omitted from the published schema
// when isBackgroundTasksDisabled() (TS BashTool.tsx:254 uses omit). The Go
// schema currently always exposes the field.
func TestBashAlignment_Schema_RunInBackgroundOmittedWhenDisabled(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "1")
	tool := &BashTool{}
	schema := tool.Schema()
	if _, ok := schema.Properties["run_in_background"]; ok {
		t.Errorf("run_in_background should be omitted from schema when CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1; got %#v", schema.Properties["run_in_background"])
	}
}

// Gap: timeout upper bound is hard-coded to 600000 in the description;
// it should track getMaxTimeoutMs() so callers may extend it via env / config.
func TestBashAlignment_Schema_TimeoutMaxIsConfigurable(t *testing.T) {
	t.Setenv("CLAUDE_CODE_BASH_MAX_TIMEOUT_MS", "900000")
	tool := &BashTool{}
	schema := tool.Schema()
	prop, ok := schema.Properties["timeout"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing 'timeout' property: %#v", schema.Properties)
	}
	desc, _ := prop["description"].(string)
	if !strings.Contains(desc, "900000") {
		t.Errorf("timeout description should reflect configured max 900000, got %q", desc)
	}
	if strings.Contains(desc, "600000") {
		t.Errorf("timeout description should not hard-code 600000, got %q", desc)
	}
}

// Sanity lock: the schema declares the expected core property set even after
// the background-task / timeout updates land. This passes today but guards
// the contract once isBackgroundTasksDisabled() is wired.
func TestBashAlignment_Schema_HasAllRequiredProperties(t *testing.T) {
	tool := &BashTool{}
	schema := tool.Schema()
	for _, name := range []string{"command", "timeout", "description", "dangerouslyDisableSandbox"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Errorf("schema missing %q property", name)
		}
	}
}

// Sanity lock: only `command` is required.
func TestBashAlignment_Schema_RequiredOnlyCommand(t *testing.T) {
	tool := &BashTool{}
	schema := tool.Schema()
	if got := strings.Join(schema.Required, ","); got != "command" {
		t.Errorf("schema.Required should be exactly [\"command\"], got %v", schema.Required)
	}
}

// -------- Rich-output field gaps (bash.go:formatBashResult) --------
//
// TS BashTool.tsx:279 emits an output object with the fields:
//   { stdout, stderr, interrupted, isImage, exitCode, returnCodeInterpretation,
//     backgroundTaskId? }
//
// The Go runtime must surface them on the ToolResult so downstream consumers
// (registry/UI/analytics) can read them. They are expected as Metadata keys.

func TestBashAlignment_Output_HasInterruptedField(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": "echo ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Metadata["interrupted"]; !ok {
		t.Errorf("ToolResult.Metadata should include 'interrupted' (matches TS BashTool.tsx:279); got %#v", result.Metadata)
	}
}

func TestBashAlignment_Output_HasIsImageField(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": "echo ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Metadata["isImage"]; !ok {
		t.Errorf("ToolResult.Metadata should include 'isImage' (matches TS BashTool.tsx:279); got %#v", result.Metadata)
	}
}

func TestBashAlignment_Output_HasBackgroundTaskIdField(t *testing.T) {
	requireBashAvailable(t)
	tmp := t.TempDir()
	tool := &BashTool{CWD: tmp, Background: NewBackgroundTaskManager(tmp)}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command":           "sleep 1",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.Metadata["backgroundTaskId"]; !ok {
		t.Errorf("background ToolResult.Metadata must include 'backgroundTaskId' for parity with TS BashTool.tsx:279; got %#v", result.Metadata)
	}
}

func TestBashAlignment_Output_HasReturnCodeInterpretation(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": "exit 1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := result.Metadata["returnCodeInterpretation"]
	if got == "" {
		t.Errorf("non-zero exit should surface 'returnCodeInterpretation' metadata; got %#v", result.Metadata)
	}
}

// Numeric exitCode in metadata, not buried in the Content string.
func TestBashAlignment_Output_HasNumericExitCode(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{"command": "exit 7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.Metadata["exitCode"]; got != "7" {
		t.Errorf("Metadata['exitCode'] should be \"7\" for `exit 7`, got %q (Content=%q)", got, result.Content)
	}
}

// Stdout and stderr should be addressable separately (TS bash output keeps
// them apart). Currently we concatenate into Content with a newline.
func TestBashAlignment_Output_StdoutStderrSeparated(t *testing.T) {
	requireBashAvailable(t)
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo out; echo err >&2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout := result.Metadata["stdout"]
	stderr := result.Metadata["stderr"]
	if !strings.Contains(stdout, "out") {
		t.Errorf("Metadata['stdout'] should contain 'out', got %q", stdout)
	}
	if !strings.Contains(stderr, "err") {
		t.Errorf("Metadata['stderr'] should contain 'err', got %q", stderr)
	}
}

// -------- COMMAND_ALLOWLIST coverage (bash_readonly.go:13-60) --------
//
// TS readOnlyValidation.ts exports an explicit COMMAND_ALLOWLIST of
// commands that bypass permission gates entirely. Go has no equivalent
// constant; the audit P1-4 calls this out explicitly. These tests pin the
// expected entries so a future explicit allowlist can be reviewed.

func TestBashAlignment_Readonly_CommandAllowlist_LS(t *testing.T) {
	if !IsReadOnlyCommand("ls", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `ls` (TS readOnlyValidation.ts)")
	}
}

func TestBashAlignment_Readonly_CommandAllowlist_CAT(t *testing.T) {
	if !IsReadOnlyCommand("cat README.md", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `cat` (TS readOnlyValidation.ts)")
	}
}

func TestBashAlignment_Readonly_CommandAllowlist_ECHO(t *testing.T) {
	if !IsReadOnlyCommand("echo hi", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `echo` (TS readOnlyValidation.ts)")
	}
}

func TestBashAlignment_Readonly_CommandAllowlist_PWD(t *testing.T) {
	if !IsReadOnlyCommand("pwd", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `pwd` (TS readOnlyValidation.ts)")
	}
}

func TestBashAlignment_Readonly_CommandAllowlist_WHICH(t *testing.T) {
	if !IsReadOnlyCommand("which gcc", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `which` (TS readOnlyValidation.ts)")
	}
}

func TestBashAlignment_Readonly_CommandAllowlist_FILE(t *testing.T) {
	if !IsReadOnlyCommand("file ./binary", SemanticUnknown) {
		t.Errorf("COMMAND_ALLOWLIST should include `file` (TS readOnlyValidation.ts)")
	}
}

// Extra: TS COMMAND_ALLOWLIST also includes nproc/getconf/locale which the
// Go readCommands map does not. These fail today and prove the gap.
func TestBashAlignment_Readonly_CommandAllowlist_MissingTSEntries(t *testing.T) {
	// TS ref: src/tools/BashTool/readOnlyValidation.ts COMMAND_ALLOWLIST
	want := []string{"nproc", "getconf", "locale"}
	for _, cmd := range want {
		if !IsReadOnlyCommand(cmd, SemanticUnknown) {
			t.Errorf("COMMAND_ALLOWLIST missing %q (TS readOnlyValidation.ts)", cmd)
		}
	}
}
