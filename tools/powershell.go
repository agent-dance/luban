package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// PowerShellTool executes commands through PowerShell on Windows-capable runtimes.
type PowerShellTool struct {
	// CWD sets the working directory for command execution.
	// If empty, the process working directory is inherited.
	CWD string

	// AllowedDirs restricts literal filesystem paths referenced by commands as
	// defense in depth. Strictly isolated child registries omit PowerShell
	// because Windows has no filesystem sandbox backend.
	AllowedDirs []string

	// PlanState, when non-nil, blocks execution while plan mode is active.
	PlanState *PlanState

	// Background tracks background shell tasks for TaskOutput/TaskStop parity.
	Background *BackgroundTaskManager
}

// PowerShellOutput gives presentation and SDK consumers the same stable
// process receipt that Bash exposes. Model-facing Content remains unchanged.
type PowerShellOutput struct {
	Stdout           string `json:"stdout"`
	Stderr           string `json:"stderr"`
	ExitCode         int    `json:"exitCode"`
	Interrupted      bool   `json:"interrupted"`
	DurationMs       int64  `json:"durationMs"`
	BackgroundTaskID string `json:"backgroundTaskId,omitempty"`
	OutputPath       string `json:"outputPath,omitempty"`
}

func (t *PowerShellTool) SetCWD(cwd string) {
	t.CWD = strings.TrimSpace(cwd)
}

func (t *PowerShellTool) Name() string { return "PowerShell" }

func (t *PowerShellTool) Description() string {
	return "Execute a PowerShell command"
}

func (t *PowerShellTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The PowerShell command to execute",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Timeout in milliseconds (max 600000)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Clear, concise description of what this command does in active voice.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to run this command in the background. Use TaskOutput to read the output later.",
			},
		},
		Required: []string{"command"},
	}
}

func (t *PowerShellTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	command, _ := input["command"].(string)
	semantics := classifyPowerShellCommand(command)
	readOnly := semantics == SemanticRead || semantics == SemanticProcess
	metadata := map[string]string{
		"semanticCategory": semantics.String(),
		"wasReadOnly":      strconv.FormatBool(readOnly),
	}
	if warning := powerShellDestructiveWarning(command, semantics); warning != "" {
		metadata["destructiveWarning"] = warning
	}
	if warning := powerShellSecurityWarning(command); warning != "" {
		metadata["securityWarn"] = warning
	}
	withMeta := func(result types.ToolResult) types.ToolResult {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string, len(metadata))
		}
		for key, value := range metadata {
			if _, exists := result.Metadata[key]; !exists {
				result.Metadata[key] = value
			}
		}
		return result
	}

	if t.PlanState != nil && t.PlanState.IsActive() {
		return withMeta(ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimePowerShellPlanModeBlocked))), nil
	}

	in, toolErr := parseInputOrError[PowerShellInput](input)
	if toolErr != nil {
		return withMeta(*toolErr), nil
	}

	command = in.Command
	var err error
	if command == "" {
		command, err = MustGetStringField(input, "command")
		if err != nil {
			return withMeta(ErrorResponse(err)), nil
		}
	}
	if len(t.AllowedDirs) > 0 {
		if dynamic := DynamicPowerShellPathReference(command); dynamic != "" {
			return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimePowerShellDynamicPath, dynamic))), nil
		}
		paths := ResolvePathsAgainstCWD(ExtractPathsFromPowerShellCommand(command), t.CWD)
		if err := ValidatePathsAgainstAllowedDirs(paths, t.AllowedDirs); err != nil {
			return withMeta(ErrorResponsef("%v", err)), nil
		}
	}

	timeoutMs := int(in.Timeout)
	if timeoutMs == 0 {
		timeoutMs = GetIntField(input, "timeout", 120000)
	}
	if timeoutMs > 600000 {
		timeoutMs = 600000
	}
	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	if in.RunInBackground {
		if t.Background == nil {
			return withMeta(ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBackgroundUnavailable))), nil
		}
		cmd, err := t.buildCommand(context.Background(), command)
		if err != nil {
			return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildBackgroundPowerShellFailed, err))), nil
		}
		snap, err := t.Background.StartShellTask(ctx, command, nonEmptyOrDefault(in.Description, command), cmd)
		if err != nil {
			return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeStartBackgroundPowerShellFailed, err))), nil
		}
		result, responseErr := StringResponse(formatBackgroundBashResult(snap.ID, snap.OutputPath))
		result.Data = PowerShellOutput{ExitCode: 0, BackgroundTaskID: snap.ID, OutputPath: snap.OutputPath}
		return withMeta(result), responseErr
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := t.buildCommand(cmdCtx, command)
	if err != nil {
		return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildPowerShellFailed, err))), nil
	}

	devNull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin = devNull
		defer devNull.Close()
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err = cmd.Run()
	durationMs := time.Since(started).Milliseconds()

	const maxOut = 500000
	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	if len(stdoutStr) > maxOut {
		stdoutStr = stdoutStr[:maxOut] + "\n" + toolRuntimeText(i18n.KeyToolRuntimeStdoutTruncated)
	}
	if len(stderrStr) > maxOut {
		stderrStr = stderrStr[:maxOut] + "\n" + toolRuntimeText(i18n.KeyToolRuntimeStderrTruncated)
	}

	if err != nil {
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		if cmdCtx.Err() == context.DeadlineExceeded {
			return withMeta(types.ToolResult{
				Content: formatFailedBashResult(stdoutStr, stderrStr, exitCode, true),
				IsError: true,
				Data:    PowerShellOutput{Stdout: stdoutStr, Stderr: stderrStr, ExitCode: exitCode, Interrupted: true, DurationMs: durationMs},
			}), nil
		}
		return withMeta(types.ToolResult{
			Content: formatFailedBashResult(stdoutStr, stderrStr, exitCode, false),
			IsError: true,
			Data:    PowerShellOutput{Stdout: stdoutStr, Stderr: stderrStr, ExitCode: exitCode, DurationMs: durationMs},
		}), nil
	}

	result, responseErr := StringResponse(formatSuccessfulBashResult(stdoutStr, stderrStr))
	result.Data = PowerShellOutput{Stdout: stdoutStr, Stderr: stderrStr, ExitCode: 0, DurationMs: durationMs}
	return withMeta(result), responseErr
}

// classifyPowerShellCommand assigns the same stable semantic labels used by
// Bash without interpreting command output. Native cmdlets are classified by
// their verb/noun contract; portable commands fall back to the existing Bash
// classifier so git, rg, curl, and similar invocations stay aligned.
func classifyPowerShellCommand(command string) CommandSemantic {
	if strings.TrimSpace(command) == "" {
		return SemanticUnknown
	}
	if warning, destructive := DestructiveCommandWarning(command); destructive && warning != "" {
		return SemanticDestructive
	}
	semantic := SemanticUnknown
	for _, segment := range splitPowerShellCommand(command) {
		candidate := classifyPowerShellSegment(segment)
		if candidate > semantic {
			semantic = candidate
		}
	}
	return semantic
}

func splitPowerShellCommand(command string) []string {
	segments := make([]string, 0, 4)
	start := 0
	var quote rune
	runes := []rune(command)
	for index, current := range runes {
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case ';', '|', '\n', '\r':
			if segment := strings.TrimSpace(string(runes[start:index])); segment != "" {
				segments = append(segments, segment)
			}
			start = index + 1
		}
	}
	if segment := strings.TrimSpace(string(runes[start:])); segment != "" {
		segments = append(segments, segment)
	}
	return segments
}

func classifyPowerShellSegment(segment string) CommandSemantic {
	if powerShellHasWriteRedirect(segment) {
		return SemanticWrite
	}
	fields := strings.Fields(strings.TrimSpace(segment))
	if len(fields) == 0 {
		return SemanticUnknown
	}
	commandIndex := 0
	for commandIndex < len(fields) && (fields[commandIndex] == "&" || fields[commandIndex] == ".") {
		commandIndex++
	}
	if commandIndex+2 < len(fields) && strings.HasPrefix(fields[commandIndex], "$") && fields[commandIndex+1] == "=" {
		commandIndex += 2
	}
	if commandIndex >= len(fields) {
		return SemanticUnknown
	}
	name := strings.ToLower(strings.Trim(fields[commandIndex], "'\"(){}"))
	name = strings.TrimSuffix(name, ".exe")

	switch name {
	case "get-process", "gps", "wait-process":
		return SemanticProcess
	case "remove-item", "ri", "rm", "del", "erase", "rmdir", "clear-content", "clc",
		"stop-process", "kill", "stop-computer", "restart-computer", "clear-disk", "format-volume", "initialize-disk":
		return SemanticDestructive
	case "invoke-webrequest", "iwr", "invoke-restmethod", "irm", "test-connection", "new-pssession",
		"enter-pssession", "invoke-command", "ssh", "scp", "sftp", "curl", "wget":
		return SemanticNetwork
	case "set-content", "sc", "add-content", "ac", "new-item", "ni", "copy-item", "copy", "cp",
		"move-item", "move", "mv", "rename-item", "ren", "out-file", "tee-object", "start-process":
		return SemanticWrite
	case "write-output", "write-host", "out-string", "get-content", "gc", "type", "cat", "get-childitem", "gci",
		"dir", "ls", "select-string", "sls", "test-path", "resolve-path", "measure-object", "compare-object":
		return SemanticRead
	}

	switch {
	case strings.HasPrefix(name, "remove-"), strings.HasPrefix(name, "clear-"), strings.HasPrefix(name, "reset-"):
		return SemanticDestructive
	case strings.HasPrefix(name, "get-"), strings.HasPrefix(name, "find-"), strings.HasPrefix(name, "select-"),
		strings.HasPrefix(name, "test-"), strings.HasPrefix(name, "measure-"), strings.HasPrefix(name, "compare-"),
		strings.HasPrefix(name, "convertfrom-"), strings.HasPrefix(name, "convertto-"):
		return SemanticRead
	case strings.HasPrefix(name, "invoke-web"), strings.HasPrefix(name, "connect-"), strings.HasPrefix(name, "disconnect-"):
		return SemanticNetwork
	case strings.HasPrefix(name, "new-"), strings.HasPrefix(name, "set-"), strings.HasPrefix(name, "add-"),
		strings.HasPrefix(name, "copy-"), strings.HasPrefix(name, "move-"), strings.HasPrefix(name, "rename-"),
		strings.HasPrefix(name, "start-"), strings.HasPrefix(name, "stop-"), strings.HasPrefix(name, "restart-"),
		strings.HasPrefix(name, "enable-"), strings.HasPrefix(name, "disable-"), strings.HasPrefix(name, "install-"),
		strings.HasPrefix(name, "update-"), strings.HasPrefix(name, "import-"), strings.HasPrefix(name, "export-"),
		strings.HasPrefix(name, "register-"), strings.HasPrefix(name, "unregister-"):
		return SemanticWrite
	}

	return ClassifyCommand(segment)
}

func powerShellHasWriteRedirect(segment string) bool {
	var quote rune
	for _, current := range segment {
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '>':
			return true
		}
	}
	return false
}

func powerShellDestructiveWarning(command string, semantics CommandSemantic) string {
	if warning, destructive := DestructiveCommandWarning(command); destructive && warning != "" {
		return warning
	}
	if semantics == SemanticDestructive {
		return toolRuntimeText(i18n.KeyToolRuntimePowerShellDestructiveWarning)
	}
	return ""
}

func powerShellSecurityWarning(command string) string {
	lower := strings.ToLower(command)
	for _, signal := range []string{"invoke-expression", "iex ", "-encodedcommand", "-enc ", "frombase64string", "-verb runas"} {
		if strings.Contains(lower, signal) {
			return toolRuntimeText(i18n.KeyToolRuntimePowerShellSecurityWarning)
		}
	}
	if findings := EvaluateBashSecurity(command); len(findings) > 0 {
		return FindingReasons(findings)
	}
	return ""
}

func (t *PowerShellTool) buildCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	exe, args, err := powerShellCommandArgs(command)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	if t.CWD != "" {
		cmd.Dir = t.CWD
	}
	return cmd, nil
}

func powerShellCommandArgs(command string) (string, []string, error) {
	exe, isWindowsPowerShell, err := resolvePowerShellExecutable()
	if err != nil {
		return "", nil, err
	}
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive"}
	if isWindowsPowerShell {
		args = append(args, "-ExecutionPolicy", "Bypass")
	}
	args = append(args, "-Command", command)
	return exe, args, nil
}

func resolvePowerShellExecutable() (string, bool, error) {
	candidates := []struct {
		Name              string
		WindowsPowerShell bool
	}{
		{Name: "pwsh", WindowsPowerShell: false},
		{Name: "powershell", WindowsPowerShell: true},
	}
	if runtime.GOOS == "windows" {
		candidates = []struct {
			Name              string
			WindowsPowerShell bool
		}{
			{Name: "pwsh.exe", WindowsPowerShell: false},
			{Name: "powershell.exe", WindowsPowerShell: true},
			{Name: "pwsh", WindowsPowerShell: false},
			{Name: "powershell", WindowsPowerShell: true},
		}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate.Name); err == nil {
			return path, candidate.WindowsPowerShell, nil
		}
	}
	return "", false, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimePowerShellNotFound))
}
