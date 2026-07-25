package shell

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/internal/contracts/filemutation"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

// BashTool executes shell commands with dangerous-command detection.
type BashTool struct {
	// scopeMu publishes the execution authority fields as one snapshot. The
	// permission check and Execute each capture that snapshot once; execution
	// never re-reads mutable CWD/AllowedDirs/PermissionRules after consuming an
	// approval receipt.
	scopeMu sync.RWMutex

	// CWD sets the working directory for command execution.
	// If empty, the process working directory is inherited.
	CWD string

	// Sandbox is the OS-level sandbox backend to use.
	// If nil (or Available() returns false), commands run unsandboxed.
	Sandbox sandbox.Backend
	// ForceSandbox prevents read-only fast paths and fallback-to-unsandboxed
	// execution. Isolated child agents enable it after verifying a real backend.
	ForceSandbox bool

	// PlanState, when non-nil, blocks execution while plan mode is active.
	PlanState PlanGate

	// Background tracks background shell tasks for TaskOutput/TaskStop parity.
	Background BackgroundRunner

	// AllowedDirs, when non-empty, restricts referenced filesystem paths to the
	// listed directories (and their descendants).
	AllowedDirs []string

	// PermissionRules carries Bash(...) rules consulted before execution.
	// An empty slice adds no local rule restriction; a valid dispatch receipt is
	// still mandatory.
	PermissionRules []permissions.Rule

	// FileMutations coordinates sed -i with the file tools' read evidence.
	FileMutations filemutation.Coordinator

	// OutputPersister stores oversized raw command output outside the runtime
	// compactor dependency graph.
	OutputPersister OutputPersister
}

type bashExecutionScope struct {
	cwd               string
	allowedDirs       []string
	permissionRules   []permissions.Rule
	sandbox           sandbox.Backend
	sandboxAvailable  bool
	sandboxName       string
	sandboxCapability string
	forceSandbox      bool
	planState         PlanGate
	background        BackgroundRunner
	fileMutations     filemutation.Coordinator
	outputPersister   OutputPersister
}

func (t *BashTool) executionScopeSnapshot() bashExecutionScope {
	if t == nil {
		return bashExecutionScope{}
	}
	t.scopeMu.RLock()
	defer t.scopeMu.RUnlock()
	capability, sandboxAvailable := sandbox.Snapshot(t.Sandbox)
	sandboxName := ""
	sandboxCapability := ""
	if sandboxAvailable {
		sandboxName = capability.Backend
		sandboxCapability = capability.ID()
	}
	return bashExecutionScope{
		cwd:             t.CWD,
		allowedDirs:     append([]string(nil), t.AllowedDirs...),
		permissionRules: append([]permissions.Rule(nil), t.PermissionRules...),
		sandbox:         t.Sandbox, sandboxAvailable: sandboxAvailable, sandboxName: sandboxName,
		sandboxCapability: sandboxCapability,
		forceSandbox:      t.ForceSandbox,
		planState:         t.PlanState,
		background:        t.Background,
		fileMutations:     t.FileMutations,
		outputPersister:   t.OutputPersister,
	}
}

// SetExecutionScope publishes CWD and AllowedDirs atomically. Session switches
// must use this combined setter so permission analysis cannot observe a mixed
// workspace generation.
func (t *BashTool) SetExecutionScope(cwd string, dirs []string) {
	if t == nil {
		return
	}
	t.scopeMu.Lock()
	defer t.scopeMu.Unlock()
	t.CWD = strings.TrimSpace(cwd)
	t.AllowedDirs = append([]string(nil), dirs...)
}

// Clone avoids copying scopeMu while preserving the intentionally
// shared service pointers used by agent-scoped registry clones.
func (t *BashTool) Clone() *BashTool {
	if t == nil {
		return nil
	}
	t.scopeMu.RLock()
	defer t.scopeMu.RUnlock()
	return &BashTool{
		CWD:     t.CWD,
		Sandbox: t.Sandbox, ForceSandbox: t.ForceSandbox,
		PlanState: t.PlanState, Background: t.Background,
		AllowedDirs:     append([]string(nil), t.AllowedDirs...),
		PermissionRules: append([]permissions.Rule(nil), t.PermissionRules...),
		FileMutations:   t.FileMutations,
		OutputPersister: t.OutputPersister,
	}
}

// CurrentCWD returns the published execution directory.
func (t *BashTool) CurrentCWD() string {
	if t == nil {
		return ""
	}
	t.scopeMu.RLock()
	defer t.scopeMu.RUnlock()
	return t.CWD
}

// CurrentAllowedDirs returns a detached copy of the published path scope.
func (t *BashTool) CurrentAllowedDirs() []string {
	if t == nil {
		return nil
	}
	t.scopeMu.RLock()
	defer t.scopeMu.RUnlock()
	return append([]string(nil), t.AllowedDirs...)
}

// BashOutput is the typed programmatic result advertised by the Bash tool.
// Model-facing text stays in ToolResult.Content; ToolResult.Data carries this
// structure for SDK consumers and output-schema validation.
type BashOutput struct {
	Stdout                    string `json:"stdout"`
	Stderr                    string `json:"stderr"`
	RawOutputPath             string `json:"rawOutputPath,omitempty"`
	Interrupted               bool   `json:"interrupted"`
	IsImage                   bool   `json:"isImage,omitempty"`
	BackgroundTaskID          string `json:"backgroundTaskId,omitempty"`
	BackgroundedByUser        bool   `json:"backgroundedByUser,omitempty"`
	AssistantAutoBackgrounded bool   `json:"assistantAutoBackgrounded,omitempty"`
	DangerouslyDisableSandbox bool   `json:"dangerouslyDisableSandbox,omitempty"`
	ReturnCodeInterpretation  string `json:"returnCodeInterpretation,omitempty"`
	PersistedOutputPath       string `json:"persistedOutputPath,omitempty"`
	PersistedOutputSize       int64  `json:"persistedOutputSize,omitempty"`
	ExitCode                  int    `json:"exitCode"`

	modelText string
}

var blockedSleepPattern = regexp.MustCompile(`(?s)^\s*sleep\s+(\d+)\s*(?:(?:&&|;|\|\|)\s*(.+?)\s*)?$`)

func (t *BashTool) Name() string        { return "Bash" }
func (t *BashTool) Description() string { return toolPromptText(i18n.KeyToolPromptBashDescription) }

func (t *BashTool) Schema() types.JSONSchema {
	maxTimeout := getMaxBashTimeoutMs()
	properties := map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": toolPromptText(i18n.KeyToolPromptBashCommand),
		},
		"timeout": map[string]any{
			"type":        "number",
			"description": i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPromptBashTimeout, maxTimeout),
		},
		"description": map[string]any{
			"type":        "string",
			"description": toolPromptText(i18n.KeyToolPromptBashSummary),
		},
		"dangerouslyDisableSandbox": map[string]any{
			"type":        "boolean",
			"description": toolPromptText(i18n.KeyToolPromptBashDisableSandbox),
		},
	}
	if !isBackgroundTasksDisabled() {
		properties["run_in_background"] = map[string]any{
			"type":        "boolean",
			"description": toolPromptText(i18n.KeyToolPromptBashRunInBackground),
		}
	}
	return types.StrictObjectSchema(properties, "command")
}

// MapToolResultToToolResultBlock keeps the existing concise model text while
// exposing BashOutput as typed SDK data.
func (t *BashTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	out, ok := data.(BashOutput)
	if !ok {
		if ptr, ptrOK := data.(*BashOutput); ptrOK && ptr != nil {
			out = *ptr
			ok = true
		}
	}
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolRuntimeBashInvalidTypedResult),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   out.modelText,
	}
}

// isBackgroundTasksDisabled mirrors TS BashTool.tsx:254 — when the env flag
// is set, the schema must omit the run_in_background field entirely.
func isBackgroundTasksDisabled() bool {
	v := strings.TrimSpace(os.Getenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// getMaxBashTimeoutMs returns the configured upper bound for bash timeouts.
// Defaults to 600000 (10 minutes) but honours the LUBAN_CODE_BASH_MAX_TIMEOUT_MS
// override, mirroring TS getMaxTimeoutMs.
func getMaxBashTimeoutMs() int {
	const defaultMax = 600000
	v := strings.TrimSpace(os.Getenv("LUBAN_CODE_BASH_MAX_TIMEOUT_MS"))
	if v == "" {
		return defaultMax
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultMax
	}
	return n
}

func getMaxBashOutputLength() int {
	const (
		defaultLimit = 30_000
		upperLimit   = 150_000
	)
	raw := strings.TrimSpace(os.Getenv("BASH_MAX_OUTPUT_LENGTH"))
	if raw == "" {
		return defaultLimit
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return defaultLimit
	}
	if limit > upperLimit {
		return upperLimit
	}
	return limit
}

func (t *BashTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	executionScope := t.executionScopeSnapshot()
	mutationCoordinator := executionScope.fileMutations
	if executionScope.planState != nil && executionScope.planState.IsActive() {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBashPlanModeBlocked)), nil
	}

	in, decodeErr := types.DecodeStrictToolInput[bashInput](input)
	if decodeErr != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, decodeErr)), nil
	}
	if isBackgroundTasksDisabled() {
		if _, supplied := input["run_in_background"]; supplied {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundFieldDisabled, "run_in_background")), nil
		}
	}

	command := in.Command
	var err error
	if command == "" {
		command, err = requiredString(input, "command")
		if err != nil {
			return errorResponse(err), nil
		}
	}
	if !in.RunInBackground {
		if blocked := detectBlockedSleepPattern(command); blocked != "" {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBlockingSleep, blocked)), nil
		}
	}

	policyContext := executionScope.shellPolicyContext(types.ToolRuntimeContext{}, false)
	policy, sedExecution := analyzeBashCommandWithSedEvidencePolicy(command, policyContext)
	commitStatus := approvalcommit.Consume(ctx, t.Name(), input, executionScope.executionPolicyCode(policy.ExecutionBindingCode()))
	if commitStatus != approvalcommit.PermissionCommitValid {
		return errorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleApproval)), nil
	}

	// Pre-compute classification once and reuse for security/sandbox/metadata.
	semantics := ClassifyCommand(command)
	readOnly := IsReadOnlyCommand(command, semantics)

	// Build metadata up-front so even error returns carry analytics.
	metadata := map[string]string{
		"semanticCategory": semantics.String(),
		"wasReadOnly":      strconv.FormatBool(readOnly),
		"shellPolicyCode":  policy.Code,
	}
	withMeta := func(res types.ToolResult) types.ToolResult {
		if res.Metadata == nil {
			res.Metadata = map[string]string{}
		}
		for k, v := range metadata {
			if _, exists := res.Metadata[k]; !exists {
				res.Metadata[k] = v
			}
		}
		res.Data = bashOutputFromResult(res, in.DangerouslyDisableSandbox)
		return res
	}
	if policy.Disposition == types.PolicyBlock {
		result := withMeta(errorResponsef("%s", toolRuntimeFormat(policy.PublicKey, policy.PublicArgs...)))
		result.Data = policy
		return result, nil
	}
	// A receipt may satisfy an Ask, but it never suppresses an immutable Deny.
	// Re-evaluate the frozen local rules for defense in depth; the receipt's
	// authority digest separately proves these are the same rules preflight saw.
	if len(executionScope.permissionRules) > 0 {
		decision, matched, _ := matchBashRuleDetailed(command, executionScope.permissionRules)
		if matched != nil {
			switch decision {
			case permissions.DecisionDeny:
				return withMeta(errorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleDenied))), nil
			}
		}
	}

	// Path validation against allowed_dirs.
	if len(executionScope.allowedDirs) > 0 {
		paths := FilterBashPathScopeExemptions(ExtractPathsFromCommand(command))
		resolved := resolvePathsAgainstCWD(paths, executionScope.cwd)
		if err := ValidatePathsAgainstAllowedDirs(resolved, executionScope.allowedDirs); err != nil {
			return withMeta(errorResponsef("%v", err)), nil
		}
	}

	// sed -i must-read-first gate. Recognized targets share Edit's canonical
	// path locks for the entire validate -> execute -> evidence-refresh
	// transaction, closing the cooperative verify/commit lost-update window.
	releaseSedLocks := func() {}
	sedLocksOwnedByBackground := false
	defer func() {
		if !sedLocksOwnedByBackground {
			releaseSedLocks()
		}
	}()
	mutationTargets := sedEditExecutionMutationTargets(sedExecution)
	if mutationCoordinator != nil && len(mutationTargets) > 0 {
		rawUnlock := mutationCoordinator.Lock(mutationTargets)
		var releaseOnce sync.Once
		releaseSedLocks = func() { releaseOnce.Do(rawUnlock) }
	}
	if mutationCoordinator != nil && sedExecution.EvidenceSafe {
		if err := mutationCoordinator.ValidateFullRead(ctx, mutationTargets); err != nil {
			return withMeta(errorResponsef("%v", err)), nil
		}
	}

	// Parse timeout (default 120s, max from env config)
	timeoutMs := int(in.Timeout)
	if timeoutMs == 0 {
		timeoutMs = inputInt(input, "timeout", 120000)
	}
	maxTimeoutMs := getMaxBashTimeoutMs()
	if timeoutMs > maxTimeoutMs {
		timeoutMs = maxTimeoutMs
	}
	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	if in.RunInBackground {
		if executionScope.background == nil {
			return withMeta(errorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBackgroundUnavailable))), nil
		}
		cmd, err := t.buildCommandWithSemanticsAtScope(context.Background(), in, command, semantics, readOnly, executionScope)
		if err != nil {
			return withMeta(errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildBackgroundFailed, err))), nil
		}
		bindSedExecutionEnvironment(cmd, sedExecution)
		completion := func(runErr error, exitCode int) {
			defer releaseSedLocks()
			if mutationCoordinator == nil || !sedExecution.EvidenceSafe {
				return
			}
			if runErr != nil || exitCode != 0 {
				mutationCoordinator.Invalidate(ctx, mutationTargets)
				return
			}
			if err := mutationCoordinator.Commit(ctx, mutationTargets, t.Name()); err != nil {
				mutationCoordinator.Invalidate(ctx, mutationTargets)
			}
		}
		// Set ownership before process start: a very short command may complete
		// on the waiter goroutine before Start returns to this goroutine.
		sedLocksOwnedByBackground = true
		taskID, outputPath, err := executionScope.background.StartShellCommand(ctx, command, nonEmptyOrDefault(in.Description, command), cmd, timeout, completion)
		if err != nil {
			sedLocksOwnedByBackground = false
			if mutationCoordinator != nil && sedExecution.EvidenceSafe {
				mutationCoordinator.Invalidate(ctx, mutationTargets)
			}
			return withMeta(errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeStartBackgroundFailed, err))), nil
		}
		res, _ := stringResponse(formatBackgroundBashResult(taskID, outputPath))
		// Background-task parity: surface the task id, mark not interrupted /
		// not an image.
		metadata["backgroundTaskId"] = taskID
		metadata["rawOutputPath"] = outputPath
		metadata["interrupted"] = "false"
		metadata["isImage"] = "false"
		return withMeta(res), nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := t.buildCommandWithSemanticsAtScope(cmdCtx, in, command, semantics, readOnly, executionScope)
	if err != nil {
		return withMeta(errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildCommandFailed, err))), nil
	}
	bindSedExecutionEnvironment(cmd, sedExecution)

	// Close stdin to prevent commands from hanging waiting for input
	cmd.Stdin = os.Stdin // allow interactive if needed, but DevNull for safety
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		cmd.Stdin = devNull
		defer devNull.Close()
	}

	// Separate stdout and stderr for LLM decision-making
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if mutationCoordinator != nil && sedExecution.EvidenceSafe && err != nil {
		mutationCoordinator.Invalidate(ctx, mutationTargets)
	}

	fullStdout := stdout.String()
	fullStderr := stderr.String()
	stdoutStr := fullStdout
	stderrStr := fullStderr
	stdoutTotalLines := countLines(stdoutStr)
	stderrTotalLines := countLines(stderrStr)
	modelStdout := stdoutStr
	inlineLimit := getMaxBashOutputLength()
	if !isImageOutput(fullStdout) && len(fullStdout)+len(fullStderr) > inlineLimit {
		persistedContent := fullStdout
		if fullStderr != "" {
			if persistedContent != "" && !strings.HasSuffix(persistedContent, "\n") {
				persistedContent += "\n"
			}
			persistedContent += fullStderr
		}
		root := executionScope.cwd
		if root == "" {
			root, _ = os.Getwd()
		}
		const maxPersistedBashOutput = 64 * 1024 * 1024
		if executionScope.outputPersister != nil {
			persisted, persistErr := executionScope.outputPersister.PersistShellOutput(root, []byte(persistedContent), maxPersistedBashOutput, stdoutStr)
			if persistErr == nil {
				metadata["rawOutputPath"] = persisted.Path
				metadata["persistedOutputPath"] = persisted.Path
				metadata["persistedOutputSize"] = strconv.FormatInt(persisted.OriginalSize, 10)
				stdoutStr = truncateWithMarker(fullStdout, inlineLimit)
				stderrStr = truncateWithMarker(fullStderr, inlineLimit)
				modelStdout = persisted.ModelText
			} else {
				metadata["outputPersistenceFailed"] = "true"
				stdoutStr = truncateWithMarker(fullStdout, inlineLimit)
				stderrStr = truncateWithMarker(fullStderr, inlineLimit)
				modelStdout = stdoutStr
			}
		} else {
			metadata["outputPersistenceUnavailable"] = "true"
			stdoutStr = truncateWithMarker(fullStdout, inlineLimit)
			stderrStr = truncateWithMarker(fullStderr, inlineLimit)
			modelStdout = stdoutStr
		}
	}

	// Common output metadata shared by success and failure paths.
	metadata["stdout"] = stdoutStr
	metadata["stderr"] = stderrStr
	metadata["isImage"] = "false"
	metadata["stdoutTotalLines"] = strconv.Itoa(stdoutTotalLines)
	metadata["stderrTotalLines"] = strconv.Itoa(stderrTotalLines)

	if err != nil {
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		interrupted := cmdCtx.Err() != nil
		metadata["exitCode"] = strconv.Itoa(exitCode)
		metadata["interrupted"] = strconv.FormatBool(interrupted)
		metadata["returnCodeInterpretation"] = interpretReturnCodeWithCommand(command, exitCode, interrupted)
		if interp, ok := interpretCommandResult(firstSegmentCommand(command), exitCode); ok && interp.TreatAsSuccess && !interrupted {
			// grep / find / diff / test exit codes that mean "no match", "files
			// differ", "condition false", etc. should NOT be surfaced as errors
			// to the model — they are normal results.
			metadata["exitCodeMeaning"] = interp.Severity
			res, _ := stringResponse(formatSuccessfulBashResult(modelStdout, stderrStr))
			return withMeta(res), nil
		}
		if interrupted {
			failure := formatFailedBashResult(modelStdout, stderrStr, exitCode, true)
			if errors.Is(cmdCtx.Err(), context.Canceled) {
				failure = formatAbortedBashResult(modelStdout, stderrStr)
			}
			return withMeta(types.ToolResult{
				Content: failure,
				IsError: true,
			}), nil
		}
		return withMeta(types.ToolResult{
			Content: formatFailedBashResult(modelStdout, stderrStr, exitCode, false),
			IsError: true,
		}), nil
	}

	// On success, publish the post-edit snapshot to the shared read state.
	if mutationCoordinator != nil && sedExecution.EvidenceSafe {
		if commitErr := mutationCoordinator.Commit(ctx, mutationTargets, t.Name()); commitErr != nil {
			mutationCoordinator.Invalidate(ctx, mutationTargets)
			metadata["readEvidenceRefreshFailed"] = "true"
		}
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	metadata["exitCode"] = strconv.Itoa(exitCode)
	metadata["interrupted"] = "false"
	metadata["returnCodeInterpretation"] = interpretReturnCodeWithCommand(command, exitCode, false)
	res, _ := stringResponse(formatSuccessfulBashResult(modelStdout, stderrStr))
	// If stdout is a data:image/...;base64,... URI, prefer returning a
	// tool_result with an image block over a giant text blob — enables shell
	// scripts that emit screenshots/charts.
	if isImageOutput(stdoutStr) {
		caption := strings.TrimSpace(stderrStr)
		if imgRes, ok := buildImageToolResult(stdoutStr, caption); ok {
			imgRes = toolbase.ResizeImageToolResult(imgRes)
			if imgRes.Metadata == nil {
				imgRes.Metadata = map[string]string{}
			}
			for k, v := range metadata {
				if _, exists := imgRes.Metadata[k]; !exists {
					imgRes.Metadata[k] = v
				}
			}
			imgRes.Metadata["isImage"] = "true"
			return imgRes, nil
		}
	}
	return withMeta(res), nil
}

func (t *BashTool) buildCommandWithSemanticsAtScope(ctx context.Context, in bashInput, command string, semantics CommandSemantic, readOnly bool, scope bashExecutionScope) (*exec.Cmd, error) {
	if scope.forceSandbox && (!scope.sandboxAvailable || scope.sandboxName == "none") {
		return nil, errors.New(toolRuntimeText(i18n.KeyToolRuntimeSandboxUnavailable))
	}
	useSandbox := scope.forceSandbox || (scope.sandboxAvailable && scope.sandboxName != "none" && !in.DangerouslyDisableSandbox && ShouldUseSandbox(command, semantics))
	if useSandbox {
		currentCapability, capabilityOK := sandbox.Snapshot(scope.sandbox)
		if !capabilityOK || currentCapability.ID() != scope.sandboxCapability {
			return nil, errors.New(toolRuntimeText(i18n.KeyToolRuntimeSandboxUnavailable))
		}
		readWritePaths := make([]string, 0, len(scope.allowedDirs)+1)
		seenPaths := make(map[string]struct{}, len(scope.allowedDirs)+1)
		candidates := append(append([]string(nil), scope.allowedDirs...), scope.cwd)
		if !scope.forceSandbox && len(scope.allowedDirs) == 0 {
			root := string(filepath.Separator)
			if volume := filepath.VolumeName(scope.cwd); volume != "" {
				root = volume + string(filepath.Separator)
			}
			candidates = []string{root}
		}
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate) == "" {
				continue
			}
			clean := filepath.Clean(candidate)
			if _, exists := seenPaths[clean]; exists {
				continue
			}
			seenPaths[clean] = struct{}{}
			readWritePaths = append(readWritePaths, clean)
		}
		sbCfg := sandbox.Config{
			ReadWritePaths: readWritePaths,
			WorkDir:        scope.cwd,
		}
		cmd, err := scope.sandbox.Command(ctx, sbCfg, "bash", "-c", command)
		if err == nil {
			if scope.cwd != "" {
				cmd.Dir = scope.cwd
			}
			configureCommandCancellation(cmd)
			return cmd, nil
		}
		// A command selected for sandboxing must never run as an unsandboxed
		// process. Return a semantic internal error through the tool result
		// pipeline instead.
		return nil, i18n.WrapInternalError(i18n.KeyBashSandboxBuildError, err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if scope.cwd != "" {
		cmd.Dir = scope.cwd
	}
	configureCommandCancellation(cmd)
	return cmd, nil
}

func bashOutputFromResult(result types.ToolResult, dangerouslyDisableSandbox bool) *BashOutput {
	metadata := result.Metadata
	exitCode, _ := strconv.Atoi(metadata["exitCode"])
	persistedSize, _ := strconv.ParseInt(metadata["persistedOutputSize"], 10, 64)
	interrupted, _ := strconv.ParseBool(metadata["interrupted"])
	isImage, _ := strconv.ParseBool(metadata["isImage"])
	backgroundedByUser, _ := strconv.ParseBool(metadata["backgroundedByUser"])
	assistantAutoBackgrounded, _ := strconv.ParseBool(metadata["assistantAutoBackgrounded"])
	return &BashOutput{
		Stdout:                    metadata["stdout"],
		Stderr:                    metadata["stderr"],
		RawOutputPath:             metadata["rawOutputPath"],
		Interrupted:               interrupted,
		IsImage:                   isImage,
		BackgroundTaskID:          metadata["backgroundTaskId"],
		BackgroundedByUser:        backgroundedByUser,
		AssistantAutoBackgrounded: assistantAutoBackgrounded,
		DangerouslyDisableSandbox: dangerouslyDisableSandbox,
		ReturnCodeInterpretation:  metadata["returnCodeInterpretation"],
		PersistedOutputPath:       metadata["persistedOutputPath"],
		PersistedOutputSize:       persistedSize,
		ExitCode:                  exitCode,
		modelText:                 result.Content,
	}
}

// countLines returns the number of lines in `s` (matches strings.Count for
// `\n` plus 1 when the trailing line has no newline). Empty input yields 0.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// truncateWithMarker truncates `s` to `maxBytes` and appends a marker that
// tells the model how many lines were dropped, so the model can re-run with
// head/tail/grep instead of reasoning on partial data. Mirrors the TS
// formatOutput behaviour.
func truncateWithMarker(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	marker := "\n\n" + toolRuntimeFormat(i18n.KeyToolRuntimeLinesTruncated, countLines(s))
	if len(marker) >= maxBytes {
		return marker[:maxBytes]
	}
	cutLimit := maxBytes - len(marker)
	cut := s[:cutLimit]
	// Round to last newline boundary so we don't slice through a line.
	if idx := strings.LastIndex(cut, "\n"); idx > 0 {
		cut = s[:idx]
	}
	rest := s[len(cut):]
	dropped := countLines(rest)
	marker = "\n\n" + toolRuntimeFormat(i18n.KeyToolRuntimeLinesTruncated, dropped)
	if len(cut)+len(marker) > maxBytes {
		cut = cut[:maxBytes-len(marker)]
	}
	return cut + marker
}

func nonEmptyOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func detectBlockedSleepPattern(command string) string {
	matches := blockedSleepPattern.FindStringSubmatch(command)
	if len(matches) == 0 {
		return ""
	}
	seconds, err := strconv.Atoi(matches[1])
	if err != nil || seconds < 2 {
		return ""
	}
	rest := strings.TrimSpace(matches[2])
	if rest != "" {
		return toolRuntimeFormat(i18n.KeyToolRuntimeSleepFollowedBy, seconds, rest)
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeStandaloneSleep, seconds)
}

func formatBackgroundBashResult(taskID, outputPath string) string {
	return toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundResult, taskID, outputPath)
}

func formatSuccessfulBashResult(stdout, stderr string) string {
	return formatBashResult(stdout, stderr)
}

func formatFailedBashResult(stdout, stderr string, exitCode int, timedOut bool) string {
	parts := make([]string, 0, 3)
	if text := formatBashResult(stdout, stderr); text != "" {
		parts = append(parts, text)
	}
	if timedOut {
		parts = append(parts, toolRuntimeText(i18n.KeyToolRuntimeCommandTimedOut))
	} else if exitCode != 0 {
		parts = append(parts, toolRuntimeFormat(i18n.KeyToolRuntimeExitCodeLabel, exitCode))
	}
	return strings.Join(parts, "\n")
}

func formatAbortedBashResult(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if text := formatBashResult(stdout, stderr); text != "" {
		parts = append(parts, text)
	}
	parts = append(parts, toolRuntimeText(i18n.KeyToolRuntimeCommandAborted))
	return strings.Join(parts, "\n")
}

func formatBashResult(stdout, stderr string) string {
	processedStdout := strings.TrimRight(stdout, "\n")
	processedStdout = strings.TrimLeft(processedStdout, " \t\r\n")
	processedStderr := strings.TrimSpace(stderr)

	parts := make([]string, 0, 2)
	if processedStdout != "" {
		parts = append(parts, processedStdout)
	}
	if processedStderr != "" {
		parts = append(parts, processedStderr)
	}
	return strings.Join(parts, "\n")
}

// interpretReturnCode returns a short human-readable explanation of `exitCode`
// suitable for surfacing in ToolResult.Metadata['returnCodeInterpretation'].
// Mirrors the TS BashTool which annotates non-zero exits.
func interpretReturnCode(exitCode int, interrupted bool) string {
	if interrupted {
		return toolRuntimeText(i18n.KeyToolRuntimeReturnInterrupted)
	}
	switch {
	case exitCode == 0:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnSuccess)
	case exitCode == -1:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnNoStatus)
	case exitCode == 1:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnGeneralError)
	case exitCode == 2:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnBuiltinMisuse)
	case exitCode == 126:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnNotExecutable)
	case exitCode == 127:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnNotFound)
	case exitCode == 130:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnSIGINT)
	case exitCode == 137:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnSIGKILL)
	case exitCode == 139:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnSIGSEGV)
	case exitCode == 143:
		return toolRuntimeText(i18n.KeyToolRuntimeReturnSIGTERM)
	case exitCode > 128 && exitCode < 256:
		return toolRuntimeFormat(i18n.KeyToolRuntimeReturnSignal, exitCode, exitCode-128)
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeReturnFailed, exitCode)
}
