package tools

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

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/types"
)

type bashProgressEmitterContextKey struct{}

// WithBashProgressEmitter attaches an optional SDK progress sink to a Bash
// invocation. Callers that do not need progress keep the existing context.
func WithBashProgressEmitter(ctx context.Context, emitter sdk.ProgressEmitter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, bashProgressEmitterContextKey{}, emitter)
}

func emitBashProgress(ctx context.Context, status string, progress float64, message string) {
	if ctx == nil {
		return
	}
	emitter, _ := ctx.Value(bashProgressEmitterContextKey{}).(sdk.ProgressEmitter)
	if emitter == nil {
		return
	}
	emitter.Emit(sdk.ToolProgressEvent{
		ToolName: "Bash",
		Status:   status,
		Progress: progress,
		Message:  message,
	})
}

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

	// OriginalCWD captures the project root that subsequent runs should be
	// reset to when a command drifts outside the allowed working paths
	// (e.g. `cd /tmp`). Set on first call; honored when
	// shouldMaintainProjectWorkingDir returns true.
	OriginalCWD string

	// Sandbox is the OS-level sandbox backend to use.
	// If nil (or Available() returns false), commands run unsandboxed.
	Sandbox sandbox.Backend
	// ForceSandbox prevents read-only fast paths and fallback-to-unsandboxed
	// execution. Isolated child agents enable it after verifying a real backend.
	ForceSandbox bool

	// PlanState, when non-nil, blocks execution while plan mode is active.
	PlanState *PlanState

	// Background tracks background shell tasks for TaskOutput/TaskStop parity.
	Background *BackgroundTaskManager

	// Mode controls the execution envelope (plan/safe/yolo). Empty string is the
	// default (no mode-level checks).
	Mode BashExecutionMode

	// AllowedDirs, when non-empty, restricts referenced filesystem paths to the
	// listed directories (and their descendants).
	AllowedDirs []string

	// PermissionRules carries Bash(...) rules consulted before execution.
	// An empty slice means no rule is enforced and the call always proceeds.
	PermissionRules []permissions.Rule

	// ReadFileState tracks Read tool mtimes for sed -i validation.
	ReadFileState *ReadFileState

	// SedTracker enforces the "must Read first" gate on sed -i commands when
	// non-nil. See bash_sed_validation.go.
	SedTracker *SedReadStateTracker

	// SedValidationEnabled toggles bash-05 enforcement of "must Read first"
	// for sed -i commands. Defaults off so existing callers remain compatible.
	SedValidationEnabled bool

	// sedLockRegisteredForTest is a deterministic barrier immediately before a
	// recognized sed transaction waits on its first canonical path lock.
	sedLockRegisteredForTest func()

	// registryDispatchRequired is set monotonically by registry.Register. It
	// keeps standalone BashTool SDK use compatible while ensuring that a tool
	// obtained from Registry.Get/All cannot execute outside Registry's runtime
	// deny, permission, and one-time receipt boundary.
	registryDispatchRequired bool
}

type bashExecutionScope struct {
	cwd                      string
	originalCWD              string
	allowedDirs              []string
	permissionRules          []permissions.Rule
	sandbox                  sandbox.Backend
	sandboxAvailable         bool
	sandboxName              string
	sandboxCapability        string
	forceSandbox             bool
	mode                     BashExecutionMode
	registryDispatchRequired bool
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
		cwd: t.CWD, originalCWD: t.OriginalCWD,
		allowedDirs:     append([]string(nil), t.AllowedDirs...),
		permissionRules: append([]permissions.Rule(nil), t.PermissionRules...),
		sandbox:         t.Sandbox, sandboxAvailable: sandboxAvailable, sandboxName: sandboxName,
		sandboxCapability: sandboxCapability,
		forceSandbox:      t.ForceSandbox, mode: t.Mode,
		registryDispatchRequired: t.registryDispatchRequired,
	}
}

// RequireRegistryDispatch binds this Bash tool to Registry's authorization
// boundary. The requirement is intentionally irreversible: unregistering or
// retaining a pointer returned by Get must not restore direct execution.
func (t *BashTool) RequireRegistryDispatch() {
	if t == nil {
		return
	}
	t.scopeMu.Lock()
	t.registryDispatchRequired = true
	t.scopeMu.Unlock()
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
	if t.OriginalCWD == "" {
		t.OriginalCWD = t.CWD
	}
	t.AllowedDirs = append([]string(nil), dirs...)
}

// SetPermissionRules publishes a detached local permission-rule snapshot.
func (t *BashTool) SetPermissionRules(rules []permissions.Rule) {
	if t == nil {
		return
	}
	t.scopeMu.Lock()
	t.PermissionRules = append([]permissions.Rule(nil), rules...)
	t.scopeMu.Unlock()
}

// cloneBashTool avoids copying scopeMu while preserving the intentionally
// shared service pointers used by agent-scoped registry clones.
func (t *BashTool) cloneBashTool() *BashTool {
	if t == nil {
		return nil
	}
	t.scopeMu.RLock()
	defer t.scopeMu.RUnlock()
	return &BashTool{
		CWD: t.CWD, OriginalCWD: t.OriginalCWD,
		Sandbox: t.Sandbox, ForceSandbox: t.ForceSandbox,
		PlanState: t.PlanState, Background: t.Background, Mode: t.Mode,
		AllowedDirs:     append([]string(nil), t.AllowedDirs...),
		PermissionRules: append([]permissions.Rule(nil), t.PermissionRules...),
		ReadFileState:   t.ReadFileState, SedTracker: t.SedTracker,
		SedValidationEnabled:     t.SedValidationEnabled,
		registryDispatchRequired: t.registryDispatchRequired,
	}
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

func (t *BashTool) SetCWD(cwd string) {
	t.scopeMu.Lock()
	defer t.scopeMu.Unlock()
	t.CWD = strings.TrimSpace(cwd)
	if t.OriginalCWD == "" {
		t.OriginalCWD = t.CWD
	}
}

// shouldMaintainProjectWorkingDir mirrors TS getShouldMaintainProjectWorkingDir.
// When set to "0"/"false"/"off" the auto-reset is disabled; the default is on.
func shouldMaintainProjectWorkingDir() bool {
	v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_MAINTAIN_PROJECT_WORKING_DIR"))
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// resetCwdIfOutsideProject resets t.CWD to t.OriginalCWD when the executed
// `command` would have moved the cwd outside the allowed working paths. We
// detect this conservatively by inspecting the leading `cd <target>` segment;
// if the target is absolute and not a prefix of any allowed dir (or of the
// project root), we reset. The caller logs `tengu_bash_tool_reset_to_original_dir`
// via the returned bool so analytics callers can fire their own metric.
func (t *BashTool) resetCwdIfOutsideProject(command string) bool {
	return t.resetCwdIfOutsideProjectAtScope(command, t.executionScopeSnapshot())
}

func (t *BashTool) resetCwdIfOutsideProjectAtScope(command string, scope bashExecutionScope) bool {
	if t == nil || scope.originalCWD == "" {
		return false
	}
	if !shouldMaintainProjectWorkingDir() {
		return false
	}
	// Walk segments and find the LAST cd target — that is the cwd left behind.
	target := lastCdTarget(command)
	if target == "" {
		return false
	}
	// Only reset on absolute targets; relative `cd dir` stays under project.
	if !isAbsolutePath(target) {
		return false
	}
	allowed := scope.allowedDirs
	if len(allowed) == 0 {
		allowed = []string{scope.originalCWD}
	}
	if pathIsUnderAny(target, allowed) {
		return false
	}
	t.scopeMu.Lock()
	defer t.scopeMu.Unlock()
	// A completed command from an older scope must not overwrite a newer
	// session switch.
	if t.CWD != scope.cwd || t.OriginalCWD != scope.originalCWD {
		return false
	}
	t.CWD = scope.originalCWD
	return true
}

func isAbsolutePath(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '/' {
		return true
	}
	// Windows drive letter (D:\ or D:/) or UNC.
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	if strings.HasPrefix(p, "\\\\") {
		return true
	}
	return false
}

func pathIsUnderAny(target string, roots []string) bool {
	clean := strings.ReplaceAll(target, "\\", "/")
	for _, r := range roots {
		root := strings.TrimRight(strings.ReplaceAll(r, "\\", "/"), "/")
		if root == "" {
			continue
		}
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

// lastCdTarget returns the argument of the last top-level `cd` in `command`.
func lastCdTarget(command string) string {
	segments := SplitBashSegments(command)
	target := ""
	for _, seg := range segments {
		s := strings.TrimSpace(seg.Stripped)
		if !strings.HasPrefix(s, "cd ") && s != "cd" {
			continue
		}
		fields := strings.Fields(s)
		if len(fields) >= 2 {
			arg := strings.Trim(fields[1], `"'`)
			target = arg
		}
	}
	return target
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

// ToolContract mirrors the TS Bash tool's strict input, typed output, and
// 30K model-result budget. Larger mapped results are persisted by ResultStore.
func (t *BashTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"stdout":                    map[string]any{"type": "string"},
			"stderr":                    map[string]any{"type": "string"},
			"rawOutputPath":             map[string]any{"type": "string"},
			"interrupted":               map[string]any{"type": "boolean"},
			"isImage":                   map[string]any{"type": "boolean"},
			"backgroundTaskId":          map[string]any{"type": "string"},
			"backgroundedByUser":        map[string]any{"type": "boolean"},
			"assistantAutoBackgrounded": map[string]any{"type": "boolean"},
			"dangerouslyDisableSandbox": map[string]any{"type": "boolean"},
			"returnCodeInterpretation":  map[string]any{"type": "string"},
			"persistedOutputPath":       map[string]any{"type": "string"},
			"persistedOutputSize":       map[string]any{"type": "number"},
			"exitCode":                  map[string]any{"type": "number"},
		},
		Required: []string{"stdout", "stderr", "interrupted"},
	}
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		MaxResultSizeChars: 30_000,
	}
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
	v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// getMaxBashTimeoutMs returns the configured upper bound for bash timeouts.
// Defaults to 600000 (10 minutes) but honours the CLAUDE_CODE_BASH_MAX_TIMEOUT_MS
// override, mirroring TS getMaxTimeoutMs.
func getMaxBashTimeoutMs() int {
	const defaultMax = 600000
	v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_BASH_MAX_TIMEOUT_MS"))
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
	sedValidationEnabled := t.SedValidationEnabled
	sedReadState := t.ReadFileState
	sedTracker := t.SedTracker
	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBashPlanModeBlocked)), nil
	}

	in, decodeErr := types.DecodeStrictToolInput[BashInput](input)
	if decodeErr != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, decodeErr)), nil
	}
	if isBackgroundTasksDisabled() {
		if _, supplied := input["run_in_background"]; supplied {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBackgroundFieldDisabled, "run_in_background")), nil
		}
	}

	command := in.Command
	var err error
	if command == "" {
		command, err = MustGetStringField(input, "command")
		if err != nil {
			return ErrorResponse(err), nil
		}
	}
	if !in.RunInBackground {
		if blocked := detectBlockedSleepPattern(command); blocked != "" {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBlockingSleep, blocked)), nil
		}
	}

	policyContext := executionScope.shellPolicyContext(types.ToolRuntimeContext{}, false)
	policy, sedExecution := analyzeBashCommandWithSedEvidencePolicy(command, policyContext)
	commitStatus := registry.ConsumePermissionCommit(ctx, t.Name(), input, executionScope.executionPolicyCode(policy.ExecutionBindingCode()))
	if commitStatus == registry.PermissionCommitInvalid ||
		(commitStatus == registry.PermissionCommitAbsent && executionScope.registryDispatchRequired) {
		return ErrorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleApproval)), nil
	}
	permissionCommitted := commitStatus == registry.PermissionCommitValid
	if commitStatus == registry.PermissionCommitAbsent {
		observability.RecordShellPolicy(string(policy.Disposition), policy.Code)
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
		result := withMeta(ErrorResponsef("%s", toolRuntimeFormat(policy.PublicKey, policy.PublicArgs...)))
		result.Data = policy
		return result, nil
	}
	if policy.Disposition == types.PolicyRequiredAsk {
		// The normal QueryLoop always runs CheckPermissions before Execute. A
		// direct/non-interactive Execute has no approval proof and fails closed
		// with the same structured decision and remediation.
		if !permissionCommitted {
			message := toolRuntimeFormat(policy.PublicKey, policy.PublicArgs...)
			if policy.Remediation != nil && policy.Remediation.PublicKey != "" {
				message += "\n" + toolRuntimeFormat(policy.Remediation.PublicKey, policy.Remediation.PublicArgs...)
			}
			result := withMeta(ErrorResponsef("%s", message))
			result.Data = policy
			return result, nil
		}
	}
	// A receipt may satisfy an Ask, but it never suppresses an immutable Deny.
	// Re-evaluate the frozen local rules for defense in depth; the receipt's
	// authority digest separately proves these are the same rules preflight saw.
	if len(executionScope.permissionRules) > 0 {
		decision, matched, partial := matchBashRuleDetailed(command, executionScope.permissionRules)
		if partial {
			if !permissionCommitted {
				return withMeta(ErrorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleApproval))), nil
			}
		}
		if matched != nil {
			switch decision {
			case permissions.DecisionDeny:
				return withMeta(ErrorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleDenied))), nil
			case permissions.DecisionAsk:
				if !permissionCommitted {
					return withMeta(ErrorResponsef("%s", toolPermissionText(i18n.KeyToolPermissionBashRuleApproval))), nil
				}
			}
		}
	}

	// Mode validation (plan/safe/yolo). Skipped when Mode is empty.
	if executionScope.mode != BashModeDefault {
		if err := ValidateCommandForMode(command, semantics, executionScope.mode); err != nil {
			return withMeta(ErrorResponsef("%v", err)), nil
		}
	}

	// Path validation against allowed_dirs.
	if len(executionScope.allowedDirs) > 0 {
		paths := FilterBashPathScopeExemptions(ExtractPathsFromCommand(command))
		resolved := ResolvePathsAgainstCWD(paths, executionScope.cwd)
		if err := ValidatePathsAgainstAllowedDirs(resolved, executionScope.allowedDirs); err != nil {
			return withMeta(ErrorResponsef("%v", err)), nil
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
	if targets := sedEditExecutionMutationTargets(sedExecution); len(targets) > 0 {
		rawUnlock := lockFileEditsWithRegisteredHook(t.sedLockRegisteredForTest, targets...)
		var releaseOnce sync.Once
		releaseSedLocks = func() { releaseOnce.Do(rawUnlock) }
	}
	if sedValidationEnabled && sedExecution.EvidenceSafe {
		if err := validateSedEditExecutionReadState(ctx, sedExecution, sedReadState); err != nil {
			return withMeta(ErrorResponsef("%v", err)), nil
		}
		if sedReadState == nil {
			if err := validateSedEditExecution(sedExecution, sedTracker); err != nil {
				return withMeta(ErrorResponsef("%v", err)), nil
			}
		}
	}

	// Parse timeout (default 120s, max from env config)
	timeoutMs := int(in.Timeout)
	if timeoutMs == 0 {
		timeoutMs = GetIntField(input, "timeout", 120000)
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
		if t.Background == nil {
			return withMeta(ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBackgroundUnavailable))), nil
		}
		emitBashProgress(ctx, "started", -1, toolRuntimeText(i18n.KeyToolRuntimeProgressStartingBackground))
		cmd, err := t.buildCommandWithSemanticsAtScope(context.Background(), in, command, semantics, readOnly, executionScope)
		if err != nil {
			emitBashProgress(ctx, "error", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressBuildBackgroundFailed))
			return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildBackgroundFailed, err))), nil
		}
		bindSedExecutionEnvironment(cmd, sedExecution)
		completion := func(runErr error, exitCode int) {
			defer releaseSedLocks()
			if runErr != nil || exitCode != 0 || !sedValidationEnabled || !sedExecution.EvidenceSafe {
				return
			}
			markSedEditExecutionReadState(ctx, sedExecution, sedReadState)
			if sedReadState == nil {
				markSedEditExecutionComplete(sedExecution, sedTracker)
			}
		}
		// Set ownership before process start: a very short command may complete
		// on the waiter goroutine before Start returns to this goroutine.
		sedLocksOwnedByBackground = true
		snap, err := t.Background.startShellTaskWithCompletion(ctx, command, nonEmptyOrDefault(in.Description, command), cmd, timeout, completion)
		if err != nil {
			sedLocksOwnedByBackground = false
			emitBashProgress(ctx, "error", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressStartBackgroundFailed))
			return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeStartBackgroundFailed, err))), nil
		}
		res, _ := StringResponse(formatBackgroundBashResult(snap.ID, snap.OutputPath))
		// Background-task parity: surface the task id, mark not interrupted /
		// not an image.
		metadata["backgroundTaskId"] = snap.ID
		metadata["rawOutputPath"] = snap.OutputPath
		metadata["interrupted"] = "false"
		metadata["isImage"] = "false"
		emitBashProgress(ctx, "completed", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressBackgroundStarted))
		return withMeta(res), nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := t.buildCommandWithSemanticsAtScope(cmdCtx, in, command, semantics, readOnly, executionScope)
	if err != nil {
		return withMeta(ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeBuildCommandFailed, err))), nil
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

	emitBashProgress(ctx, "started", -1, toolRuntimeText(i18n.KeyToolRuntimeProgressRunning))
	err = cmd.Run()

	fullStdout := stdout.String()
	fullStderr := stderr.String()
	stdoutStr := fullStdout
	stderrStr := fullStderr
	stdoutTotalLines := countLines(stdoutStr)
	stderrTotalLines := countLines(stderrStr)
	modelStdout := stdoutStr
	inlineLimit := getMaxBashOutputLength()
	if !IsImageOutput(fullStdout) && len(fullStdout)+len(fullStderr) > inlineLimit {
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
		store := compact.NewResultStore(root)
		if path, originalSize, persistErr := store.PersistRawOutput("bash", []byte(persistedContent), maxPersistedBashOutput); persistErr == nil {
			metadata["rawOutputPath"] = path
			metadata["persistedOutputPath"] = path
			metadata["persistedOutputSize"] = strconv.FormatInt(originalSize, 10)
			stdoutStr = truncateWithMarker(fullStdout, inlineLimit)
			stderrStr = truncateWithMarker(fullStderr, inlineLimit)
			modelStdout = compact.BuildPersistedOutputMessage(path, originalSize, stdoutStr)
		} else {
			metadata["outputPersistenceError"] = persistErr.Error()
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
		metadata["returnCodeInterpretation"] = InterpretReturnCodeWithCommand(command, exitCode, interrupted)
		if interp, ok := InterpretCommandResult(firstSegmentCommand(command), exitCode); ok && interp.TreatAsSuccess && !interrupted {
			// grep / find / diff / test exit codes that mean "no match", "files
			// differ", "condition false", etc. should NOT be surfaced as errors
			// to the model — they are normal results.
			metadata["exitCodeMeaning"] = interp.Severity
			res, _ := StringResponse(formatSuccessfulBashResult(modelStdout, stderrStr))
			return withMeta(res), nil
		}
		if interrupted {
			failure := formatFailedBashResult(modelStdout, stderrStr, exitCode, true)
			if errors.Is(cmdCtx.Err(), context.Canceled) {
				failure = formatAbortedBashResult(modelStdout, stderrStr)
			}
			emitBashProgress(ctx, "error", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressInterrupted))
			return withMeta(types.ToolResult{
				Content: failure,
				IsError: true,
			}), nil
		}
		emitBashProgress(ctx, "error", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressFailed))
		return withMeta(types.ToolResult{
			Content: formatFailedBashResult(modelStdout, stderrStr, exitCode, false),
			IsError: true,
		}), nil
	}

	// On success, mark sed edit complete to update tracker.
	if sedValidationEnabled && sedExecution.EvidenceSafe {
		markSedEditExecutionReadState(ctx, sedExecution, sedReadState)
		if sedReadState == nil {
			markSedEditExecutionComplete(sedExecution, sedTracker)
		}
	}

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	metadata["exitCode"] = strconv.Itoa(exitCode)
	metadata["interrupted"] = "false"
	metadata["returnCodeInterpretation"] = InterpretReturnCodeWithCommand(command, exitCode, false)
	emitBashProgress(ctx, "completed", 1, toolRuntimeText(i18n.KeyToolRuntimeProgressCompleted))

	res, _ := StringResponse(formatSuccessfulBashResult(modelStdout, stderrStr))
	if reset := t.resetCwdIfOutsideProjectAtScope(command, executionScope); reset {
		metadata["cwdResetToOriginal"] = "true"
	}
	// If stdout is a data:image/...;base64,... URI, prefer returning a
	// tool_result with an image block over a giant text blob — enables shell
	// scripts that emit screenshots/charts.
	if IsImageOutput(stdoutStr) {
		caption := strings.TrimSpace(stderrStr)
		if imgRes, ok := BuildImageToolResult(stdoutStr, caption); ok {
			imgRes = ResizeShellImageOutputResult(imgRes)
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

// buildCommandWithSemantics constructs the *exec.Cmd, gating sandbox usage on
// the command's semantic classification. Read-only commands skip sandboxing
// for performance per bash-07; network/write/destructive commands sandbox by
// default unless the caller opts out via dangerouslyDisableSandbox.
func (t *BashTool) buildCommandWithSemantics(ctx context.Context, in BashInput, command string, semantics CommandSemantic, readOnly bool) (*exec.Cmd, error) {
	return t.buildCommandWithSemanticsAtScope(ctx, in, command, semantics, readOnly, t.executionScopeSnapshot())
}

func (t *BashTool) buildCommandWithSemanticsAtScope(ctx context.Context, in BashInput, command string, semantics CommandSemantic, readOnly bool, scope bashExecutionScope) (*exec.Cmd, error) {
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
		// A command selected for sandboxing must never silently fall back to an
		// unsandboxed process. Besides weakening the execution boundary, the old
		// fallback wrote a warning directly to stderr and corrupted an active
		// alternate-screen TUI. Return a semantic internal error through the tool
		// result pipeline instead.
		return nil, i18n.WrapInternalError(i18n.KeyBashSandboxBuildError, err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if scope.cwd != "" {
		cmd.Dir = scope.cwd
	}
	configureCommandCancellation(cmd)
	return cmd, nil
}

// buildCommand preserves the legacy entry-point for callers that don't have a
// pre-computed semantic classification. It re-classifies on the fly and
// delegates to buildCommandWithSemantics.
func (t *BashTool) buildCommand(ctx context.Context, in BashInput, command string) (*exec.Cmd, error) {
	semantics := ClassifyCommand(command)
	readOnly := IsReadOnlyCommand(command, semantics)
	return t.buildCommandWithSemantics(ctx, in, command, semantics, readOnly)
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
