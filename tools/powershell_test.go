package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPowerShellToolSchema(t *testing.T) {
	tool := &PowerShellTool{}
	schema := tool.Schema()
	if got := strings.Join(schema.Required, ","); got != "command" {
		t.Fatalf("PowerShell schema required = %q, want command", got)
	}
	if _, ok := schema.Properties["run_in_background"]; !ok {
		t.Fatalf("expected run_in_background property")
	}
}

func TestExtractPathsFromPowerShellCommand(t *testing.T) {
	command := `Get-Content -Path 'C:\parent checkout\secret.txt'; $target=C:\parent\out.txt; Set-Content -Path $target -Value x`
	paths := ExtractPathsFromPowerShellCommand(command)
	for _, want := range []string{`C:\parent checkout\secret.txt`, `C:\parent\out.txt`} {
		found := false
		for _, path := range paths {
			if path == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("PowerShell paths %v missing %q", paths, want)
		}
	}
}

func TestPowerShellToolRejectsAbsolutePathOutsideAllowedDirsBeforeLaunch(t *testing.T) {
	child := t.TempDir()
	parent := t.TempDir()
	tool := &PowerShellTool{CWD: child, AllowedDirs: []string{child}}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "Get-Content -Path " + filepath.Join(parent, "secret.txt"),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "outside allowed directories") {
		t.Fatalf("outside path result = %#v", result)
	}
}

func TestPowerShellToolRejectsUnverifiableDynamicPathInScopedAgent(t *testing.T) {
	child := t.TempDir()
	tool := &PowerShellTool{CWD: child, AllowedDirs: []string{child}}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": `Set-Content -Path $target -Value x`,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "cannot be verified") {
		t.Fatalf("dynamic path result = %#v", result)
	}
}

func TestPowerShellToolPlanModeBlocksExecution(t *testing.T) {
	ps := NewPlanState()
	ps.enter("plan.md")
	tool := &PowerShellTool{PlanState: ps}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "Write-Output hello",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "cannot use PowerShell in plan mode") {
		t.Fatalf("expected plan mode block, got %#v", result)
	}
	if result.Metadata["semanticCategory"] != "read" || result.Metadata["wasReadOnly"] != "true" {
		t.Fatalf("plan-mode terminal result lost semantic metadata: %#v", result.Metadata)
	}
}

func TestPowerShellSemanticMetadataUsesCommandInputNotResultProse(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		category   string
		readOnly   string
		warningKey string
	}{
		{name: "read", command: "Get-Content README.md", category: "read", readOnly: "true"},
		{name: "process", command: "Get-Process", category: "process", readOnly: "true"},
		{name: "write", command: "Set-Content out.txt value", category: "write", readOnly: "false"},
		{name: "network", command: "Invoke-WebRequest https://example.com", category: "network", readOnly: "false"},
		{name: "destructive", command: "Remove-Item -Recurse target", category: "destructive", readOnly: "false", warningKey: "destructiveWarning"},
		{name: "security warning", command: "Invoke-Expression $payload", category: "unknown", readOnly: "false", warningKey: "securityWarn"},
		{name: "portable fallback", command: "git rev-parse --show-toplevel", category: "read", readOnly: "true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewPlanState()
			state.enter("plan.md")
			result, err := (&PowerShellTool{PlanState: state}).Execute(context.Background(), map[string]any{"command": test.command})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Metadata["semanticCategory"]; got != test.category {
				t.Fatalf("semanticCategory=%q, want %q; metadata=%#v", got, test.category, result.Metadata)
			}
			if got := result.Metadata["wasReadOnly"]; got != test.readOnly {
				t.Fatalf("wasReadOnly=%q, want %q; metadata=%#v", got, test.readOnly, result.Metadata)
			}
			if test.warningKey != "" && strings.TrimSpace(result.Metadata[test.warningKey]) == "" {
				t.Fatalf("missing %s warning: %#v", test.warningKey, result.Metadata)
			}
			if strings.Contains(result.Content, test.category) {
				t.Fatalf("test precondition failed: result prose unexpectedly contains category %q: %q", test.category, result.Content)
			}
		})
	}
}

func TestPowerShellRejectedTerminalResultStillCarriesWriteMetadata(t *testing.T) {
	child := t.TempDir()
	result, err := (&PowerShellTool{CWD: child, AllowedDirs: []string{child}}).Execute(context.Background(), map[string]any{
		"command": `Set-Content -Path $target -Value x`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Metadata["semanticCategory"] != "write" || result.Metadata["wasReadOnly"] != "false" {
		t.Fatalf("rejected write metadata = %#v, result=%q", result.Metadata, result.Content)
	}
}

func TestPowerShellToolExecutesWhenAvailable(t *testing.T) {
	if _, _, err := resolvePowerShellExecutable(); err != nil {
		t.Skipf("PowerShell executable unavailable: %v", err)
	}
	tool := &PowerShellTool{CWD: t.TempDir()}

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "Write-Output 'hello-from-powershell'",
		"timeout": float64(30000),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected PowerShell error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello-from-powershell") {
		t.Fatalf("expected command output, got %q", result.Content)
	}
	if result.Metadata["semanticCategory"] != "read" || result.Metadata["wasReadOnly"] != "true" {
		t.Fatalf("successful result metadata = %#v", result.Metadata)
	}
	output, ok := result.Data.(PowerShellOutput)
	if !ok || output.ExitCode != 0 || !strings.Contains(output.Stdout, "hello-from-powershell") || output.DurationMs < 0 {
		t.Fatalf("structured PowerShell receipt = %#v", result.Data)
	}
}
