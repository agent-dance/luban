package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

const (
	maxRunSteps         = 32
	maxRunStepInputSize = 128 * 1024
	defaultRunTimeoutMS = 120_000
	defaultRunHeadLines = 20
	defaultRunTailLines = 20
	defaultRunMaxChars  = 30_000
	maxRunOutputLines   = 1_000
	minRunMaxChars      = 256
	maxRunMaxChars      = 150_000
)

var runStepIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)

// RunTool executes a bounded immutable command DAG using the same authority
// snapshot, policy analyzer, permission rules, and sandbox backend as Bash.
// It deliberately holds a Bash pointer rather than copying its mutable scope;
// session publication remains atomic through BashTool.SetExecutionScope.
type RunTool struct {
	Bash *BashTool
}

// NewRunTool constructs the structured runner around the registry's Bash
// instance. Registration is intentionally owned by the registry integration.
func NewRunTool(bash *BashTool) *RunTool {
	if bash == nil {
		bash = &BashTool{}
	}
	return &RunTool{Bash: bash}
}

type runInput struct {
	Steps               []runStepInput `json:"steps"`
	FailFast            bool           `json:"fail_fast,omitempty"`
	RequiresPatchCommit bool           `json:"requires_patch_commit,omitempty"`
	Head                *int           `json:"head,omitempty"`
	Tail                *int           `json:"tail,omitempty"`
	MaxChars            *int           `json:"max_chars,omitempty"`
}

type runStepInput struct {
	ID          string    `json:"id"`
	Argv        *[]string `json:"argv,omitempty"`
	ShellScript *string   `json:"shell_script,omitempty"`
	CWD         string    `json:"cwd,omitempty"`
	TimeoutMS   *int      `json:"timeout_ms,omitempty"`
	DependsOn   []string  `json:"depends_on,omitempty"`
}

type compiledRunPlan struct {
	steps               []compiledRunStep
	failFast            bool
	headLines           int
	tailLines           int
	maxChars            int
	bindingCode         string
	readOnly            bool
	destructive         bool
	requiresPatchCommit bool
	revisionSafe        bool
	formatterCount      int
}

type compiledRunStep struct {
	index            int
	id               string
	argv             []string
	shellScript      string
	useShell         bool
	cwd              string
	timeout          time.Duration
	dependsOn        []int
	command          string
	policy           types.PolicyDecision
	sed              sedEditExecution
	semantics        CommandSemantic
	readOnly         bool
	effect           string
	resources        []string
	verificationKind string
	verificationSafe bool
	formatterWrites  []string
	managedRoot      string
}

// RunOutput is compact SDK data. Raw stdout/stderr is intentionally absent;
// bounded excerpts live only in the model-facing text held in modelText.
type RunOutput struct {
	Steps                     []RunStepOutput `json:"steps"`
	LogicalExecutionCommitted bool            `json:"logical_execution_committed"`
	RevisionSealDisposition   string          `json:"revision_seal_disposition,omitempty"`

	modelText string
	receipt   workspacerevision.Receipt
}

// CompactionProof retains execution and revision facts while excluding model
// step IDs, commands, paths, declared resources, and process output.
func (o *RunOutput) CompactionProof() compactproof.Proof {
	if o == nil {
		return compactproof.Proof{}
	}
	run := &compactproof.RunProof{
		LogicalExecutionCommitted: o.LogicalExecutionCommitted,
		RevisionSealDisposition:   o.RevisionSealDisposition,
		Steps:                     make([]compactproof.RunStepProof, 0, len(o.Steps)),
	}
	for ordinal, step := range o.Steps {
		duration := step.ProcessDurationMS
		if !step.Invoked {
			duration = step.DurationMS
		}
		run.TotalDurationMS += duration
		run.Steps = append(run.Steps, compactproof.RunStepProof{
			Ordinal: ordinal, Status: step.Status, ExitCode: step.ExitCode,
			DurationMS: duration, Invoked: step.Invoked, Truncated: step.Truncated,
		})
	}
	proof := compactproof.Proof{Run: run}
	if receipt, ok := o.WorkspaceRevisionReceipt(); ok {
		proof.Revision = &compactproof.RevisionProof{
			Status: "sealed", Epoch: uint64(receipt.Epoch()), Digest: string(receipt.Digest()),
		}
	}
	return proof
}

func (o *RunOutput) WorkspaceRevisionReceipt() (workspacerevision.Receipt, bool) {
	if o == nil {
		return workspacerevision.Receipt{}, false
	}
	return o.receipt, o.receipt.Valid()
}

// ToolExecutionEvidence projects only protocol facts. Model-authored step IDs,
// commands, paths, resources, and process output never cross this boundary.
func (o *RunOutput) ToolExecutionEvidence() runtimeevent.ToolExecutionEvidence {
	if o == nil {
		return runtimeevent.ToolExecutionEvidence{}
	}
	evidence := runtimeevent.ToolExecutionEvidence{
		LogicalExecutionCommitted: o.LogicalExecutionCommitted,
		RevisionSealDisposition:   o.RevisionSealDisposition,
	}
	for ordinal, step := range o.Steps {
		if !step.Invoked {
			continue
		}
		evidence.PhysicalSteps = append(evidence.PhysicalSteps, runtimeevent.PhysicalToolStepEvidence{
			Ordinal: ordinal, StartedOffsetMS: step.StartedOffsetMS, EndedOffsetMS: step.EndedOffsetMS,
			DurationMS: step.ProcessDurationMS, Outcome: step.Status,
			StdoutBytes: step.StdoutBytes, StderrBytes: step.StderrBytes,
		})
	}
	return evidence
}

// ReportsPhysicalChildOperations tells the scheduler that Run's dispatcher is
// not itself a physical operation. Only child processes that cross exec.Start
// contribute to operational counts.
func (t *RunTool) ReportsPhysicalChildOperations() bool { return true }

// RunStepOutput exposes deterministic execution facts and the preflight's
// resource/effect declaration without copying process output into Data.
type RunStepOutput struct {
	ID                string   `json:"id"`
	Status            string   `json:"status"`
	ExitCode          int      `json:"exit_code"`
	DurationMS        int64    `json:"duration_ms"`
	Invoked           bool     `json:"invoked"`
	StartedOffsetMS   int64    `json:"started_offset_ms,omitempty"`
	EndedOffsetMS     int64    `json:"ended_offset_ms,omitempty"`
	ProcessDurationMS int64    `json:"process_duration_ms,omitempty"`
	Truncated         bool     `json:"truncated"`
	Effect            string   `json:"effect"`
	Resources         []string `json:"resources"`
	StdoutBytes       int64    `json:"stdout_bytes"`
	StderrBytes       int64    `json:"stderr_bytes"`
	// Deprecated compatibility aliases. Capture accounting has always counted
	// bytes, not Unicode characters.
	StdoutChars int64 `json:"stdout_chars"`
	StderrChars int64 `json:"stderr_chars"`
}

func (t *RunTool) bash() *BashTool {
	if t == nil || t.Bash == nil {
		return &BashTool{}
	}
	return t.Bash
}

func (t *RunTool) Name() string { return "Run" }

func (t *RunTool) ConsumesWorkspaceRevisionBarrier() bool {
	return t != nil && t.bash().executionScopeSnapshot().workspaceRevisions != nil
}

func (t *RunTool) RequiresPatchCommit(input map[string]any) bool {
	required, _ := input["requires_patch_commit"].(bool)
	return required
}

func (t *RunTool) Description() string {
	return toolPromptText(i18n.KeyToolRunDescription)
}

func (t *RunTool) Schema() types.JSONSchema {
	stepProperties := map[string]any{
		"id": map[string]any{
			"type": "string", "description": toolPromptText(i18n.KeyToolRunSchemaStepID),
		},
		"argv": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1,
			"description": toolPromptText(i18n.KeyToolRunSchemaArgv),
		},
		"shell_script": map[string]any{
			"type": "string", "description": toolPromptText(i18n.KeyToolRunSchemaShellScript),
		},
		"cwd": map[string]any{
			"type": "string", "description": toolPromptText(i18n.KeyToolRunSchemaCWD),
		},
		"timeout_ms": map[string]any{
			"type": "integer", "minimum": 1, "maximum": getMaxBashTimeoutMs(),
			"description": toolPromptText(i18n.KeyToolRunSchemaTimeout),
		},
		"depends_on": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true,
			"description": toolPromptText(i18n.KeyToolRunSchemaDependsOn),
		},
	}
	stepSchema := map[string]any{
		"type": "object", "properties": stepProperties,
		"required": []string{"id"}, "additionalProperties": false,
		"oneOf": []any{
			map[string]any{"required": []string{"argv"}, "not": map[string]any{"required": []string{"shell_script"}}},
			map[string]any{"required": []string{"shell_script"}, "not": map[string]any{"required": []string{"argv"}}},
		},
	}
	return types.StrictObjectSchema(map[string]any{
		"steps": map[string]any{
			"type": "array", "items": stepSchema, "minItems": 1, "maxItems": maxRunSteps,
			"description": toolPromptText(i18n.KeyToolRunSchemaSteps),
		},
		"fail_fast": map[string]any{
			"type": "boolean", "description": toolPromptText(i18n.KeyToolRunSchemaFailFast),
		},
		"requires_patch_commit": map[string]any{
			"type": "boolean", "description": toolPromptText(i18n.KeyToolRunSchemaRequiresPatchCommit),
		},
		"head": map[string]any{
			"type": "integer", "minimum": 0, "maximum": maxRunOutputLines,
			"description": toolPromptText(i18n.KeyToolRunSchemaHead),
		},
		"tail": map[string]any{
			"type": "integer", "minimum": 0, "maximum": maxRunOutputLines,
			"description": toolPromptText(i18n.KeyToolRunSchemaTail),
		},
		"max_chars": map[string]any{
			"type": "integer", "minimum": minRunMaxChars, "maximum": maxRunMaxChars,
			"description": toolPromptText(i18n.KeyToolRunSchemaMaxChars),
		},
	}, "steps")
}

func (t *RunTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	scope := t.bash().executionScopeSnapshot()
	plan, err := compileRunPlan(input, scope, types.ToolRuntimeContext{}, true)
	if err != nil {
		return types.ToolMetadata{Write: true, Destructive: true, MaxResultSizeChars: types.UnlimitedToolResultSize}
	}
	return types.ToolMetadata{
		ReadOnly: plan.readOnly, Write: !plan.readOnly, Destructive: plan.destructive,
		ConcurrencySafe: plan.readOnly, MaxResultSizeChars: types.UnlimitedToolResultSize,
	}
}

// CheckPermissions compiles and normalizes the complete DAG before consulting
// policy or permissions. All node decisions are then reduced monotonically to
// one approval boundary; an allow can never hide a block, deny, or required ask.
func (t *RunTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	scope := t.bash().executionScopeSnapshot()
	plan, err := compileRunPlan(input, scope, request.Runtime, request.AvoidPrompts)
	if err != nil {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny, Message: runPublicError(err), Required: true,
			Sandboxed:         scope.sandboxAvailable && scope.sandboxName != "none",
			SandboxCapability: scope.sandboxCapability,
		}, nil
	}
	if scope.forceSandbox && (!scope.sandboxAvailable || scope.sandboxName == "none") {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolRunSandboxUnavailable), Required: true,
			ExecutionPolicyCode: scope.executionPolicyCode(plan.bindingCode),
		}, nil
	}
	if scope.planState != nil && scope.planState.IsActive() {
		return types.ToolPermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Message:  toolPermissionText(i18n.KeyToolRunPlanModeBlocked), Required: true,
			ExecutionPolicyCode: scope.executionPolicyCode(plan.bindingCode),
			Sandboxed:           scope.sandboxAvailable && scope.sandboxName != "none",
			SandboxCapability:   scope.sandboxCapability,
		}, nil
	}

	aggregate := types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}
	bestRank := runPermissionRank(types.PermissionBehaviorAllow, false)
	seenSuggestions := make(map[string]struct{})
	for _, step := range plan.steps {
		stepInput := map[string]any{"command": step.command}
		decision := checkBashCommandPermissionsAtScope(scope, step.command, step.policy, stepInput, request)
		rank := runPermissionRank(decision.Behavior, decision.Required)
		if rank > bestRank {
			bestRank = rank
			aggregate.Behavior = decision.Behavior
			aggregate.Required = decision.Required
			aggregate.Message = toolPermissionFormat(i18n.KeyToolRunPermissionStep, step.id, decision.Message)
			if decision.PolicyDecision != nil {
				copy := decision.PolicyDecision.Clone()
				aggregate.PolicyDecision = &copy
			} else {
				aggregate.PolicyDecision = nil
			}
		}
		for _, suggestion := range decision.Suggestions {
			encoded, _ := json.Marshal(suggestion)
			key := string(encoded)
			if _, exists := seenSuggestions[key]; exists {
				continue
			}
			seenSuggestions[key] = struct{}{}
			aggregate.Suggestions = append(aggregate.Suggestions, suggestion)
		}
	}
	aggregate.ExecutionPolicyCode = scope.executionPolicyCode(plan.bindingCode)
	aggregate.Sandboxed = scope.sandboxAvailable && scope.sandboxName != "none"
	aggregate.SandboxCapability = scope.sandboxCapability
	return aggregate, nil
}

func runPermissionRank(behavior types.PermissionBehavior, required bool) int {
	switch behavior {
	case types.PermissionBehaviorDeny:
		return 5
	case types.PermissionBehaviorAsk:
		if required {
			return 4
		}
		return 3
	case types.PermissionBehaviorPassthrough:
		return 2
	default:
		return 1
	}
}

// Execute consumes the registry-issued receipt for the exact normalized plan
// and current execution authority before starting any process.
func (t *RunTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	scope := t.bash().executionScopeSnapshot()
	if scope.planState != nil && scope.planState.IsActive() {
		return types.ToolResult{Content: toolRuntimeText(i18n.KeyToolRunPlanModeBlocked), IsError: true, Outcome: types.ToolOutcomeDenied}, nil
	}
	runtime := types.ToolRuntimeContext{AllowedDirs: append([]string(nil), scope.allowedDirs...)}
	plan, err := compileRunPlan(input, scope, runtime, true)
	if err != nil {
		return types.ToolResult{Content: runPublicError(err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	receipt, revisionBound := workspacerevision.FromContext(ctx)
	if plan.requiresPatchCommit && !revisionBound {
		return runPatchCommitRequiredResult(), nil
	}
	policyCode := scope.executionPolicyCode(plan.bindingCode)
	if approvalcommit.Consume(ctx, t.Name(), input, policyCode) != approvalcommit.PermissionCommitValid {
		return types.ToolResult{Content: toolPermissionText(i18n.KeyToolRunApprovalRequired), IsError: true, Outcome: types.ToolOutcomeDenied}, nil
	}
	if revisionBound && (scope.workspaceRevisions == nil || scope.workspaceRevisions.Validate(receipt) != nil) {
		return runRevisionChangedResult(types.ToolResult{}), nil
	}
	if revisionBound && plan.revisionSafe {
		safe := executeRevisionSafeRunPlan(ctx, scope, plan, receipt)
		result := safe.result
		if !safe.certified {
			if result.Metadata == nil {
				result.Metadata = make(map[string]string)
			}
			if safe.reason != "" {
				result.Metadata["verification.safety_reason"] = safe.reason
			}
			return runCommittedUnverifiedResult(result), nil
		}
		if output, ok := result.Data.(*RunOutput); ok && output != nil {
			output.receipt = safe.receipt
			output.RevisionSealDisposition = "revision_bound"
		}
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["verification.status"] = "revision_bound"
		result.Metadata["verification.revision_epoch"] = strconv.FormatUint(uint64(safe.receipt.Epoch()), 10)
		result.Metadata["verification.revision_digest"] = string(safe.receipt.Digest())
		if safe.mutationCommitted {
			result.Metadata["mutation.status"] = "committed"
		}
		if safe.verificationRan {
			if verificationKind, verificationConfig := runVerificationAttestation(plan); verificationKind != "" {
				result.Metadata["verification.kind"] = verificationKind
				result.Metadata["verification.config_digest"] = verificationConfig
			}
		}
		return result, nil
	}
	result := executeRunPlan(ctx, scope, plan)
	if !plan.readOnly {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		if revisionBound {
			result.Metadata["verification.safety_reason"] = "plan_not_revision_safe"
		} else {
			result.Metadata["verification.safety_reason"] = "revision_receipt_unavailable"
		}
		return runCommittedUnverifiedResult(result), nil
	}
	if verificationKind, verificationConfig := runVerificationAttestation(plan); verificationKind != "" {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["verification.kind"] = verificationKind
		result.Metadata["verification.config_digest"] = verificationConfig
	}
	if revisionBound {
		if scope.workspaceRevisions.Validate(receipt) != nil {
			return runRevisionChangedResult(result), nil
		}
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["verification.status"] = "revision_bound"
		result.Metadata["verification.revision_epoch"] = strconv.FormatUint(uint64(receipt.Epoch()), 10)
		result.Metadata["verification.revision_digest"] = string(receipt.Digest())
		setRunRevisionSealDisposition(&result, "revision_bound")
	}
	return result, nil
}

func runPatchCommitRequiredResult() types.ToolResult {
	return types.ToolResult{
		Content: toolRuntimeText(i18n.KeyToolRunPatchCommitRequired),
		Metadata: map[string]string{
			"verification.status": "patch_commit_required",
		},
		IsError:      true,
		Outcome:      types.ToolOutcomeFailed,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}
}

func runCommittedUnverifiedResult(result types.ToolResult) types.ToolResult {
	warning := toolRuntimeText(i18n.KeyToolRunCommittedUnverified)
	switch reason := result.Metadata["verification.safety_reason"]; reason {
	case "revision_receipt_unavailable":
		warning += "\n" + toolRuntimeText(i18n.KeyToolRunSealReceiptMissing)
	case "plan_not_revision_safe":
		warning += "\n" + toolRuntimeText(i18n.KeyToolRunSealPlanUnsupported)
	case "":
	default:
		warning += "\n" + toolRuntimeFormat(i18n.KeyToolRunSealSafetyFailed, reason)
	}
	appendWarning := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return warning
		}
		return strings.TrimSpace(value) + "\n" + warning
	}
	result.Content = appendWarning(result.Content)
	if output, ok := result.Data.(*RunOutput); ok && output != nil {
		// Successful typed results are rendered from modelText by the mapper,
		// so keep the safety disposition visible on both result paths.
		output.modelText = appendWarning(output.modelText)
		result.Content = output.modelText
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["verification.status"] = "committed_unverified"
	result.Metadata["mutation.status"] = "possible"
	setRunRevisionSealDisposition(&result, "committed_unverified")
	delete(result.Metadata, "verification.kind")
	delete(result.Metadata, "verification.config_digest")
	return result
}

func runRevisionChangedResult(result types.ToolResult) types.ToolResult {
	message := toolRuntimeText(i18n.KeyToolRunRevisionChanged)
	if strings.TrimSpace(result.Content) == "" {
		result.Content = message
	} else {
		result.Content = strings.TrimSpace(result.Content) + "\n" + message
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["verification.status"] = "revision_mismatch"
	setRunRevisionSealDisposition(&result, "revision_mismatch")
	delete(result.Metadata, "verification.kind")
	delete(result.Metadata, "verification.config_digest")
	result.IsError = true
	result.Outcome = types.ToolOutcomeFailed
	result.Completeness = types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}
	return result
}

func setRunRevisionSealDisposition(result *types.ToolResult, disposition string) {
	if result == nil {
		return
	}
	if output, ok := result.Data.(*RunOutput); ok && output != nil {
		output.RevisionSealDisposition = disposition
	}
}

func (t *RunTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	out, ok := data.(RunOutput)
	if !ok {
		if pointer, pointerOK := data.(*RunOutput); pointerOK && pointer != nil {
			out, ok = *pointer, true
		}
	}
	if !ok {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
			Content: toolRuntimeText(i18n.KeyToolRunTypedResultInvalid), IsError: true,
		}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: out.modelText}
}

func runPlanError(key i18n.Key, args ...any) error {
	return i18n.NewError(key, args...)
}

func runPlanWrap(key i18n.Key, cause error, args ...any) error {
	return i18n.WrapInternalError(key, cause, args...)
}

func runPublicError(err error) string {
	if semantic, ok := i18n.DescribeSemanticError(err); ok {
		return i18n.Format(i18n.DetectOrLoadLanguage(), semantic.Key, semantic.Args...)
	}
	return toolRuntimeText(i18n.KeyToolRunInvalidInput)
}

func compileRunPlan(input map[string]any, scope bashExecutionScope, runtime types.ToolRuntimeContext, avoidPrompts bool) (*compiledRunPlan, error) {
	in, err := types.DecodeStrictToolInput[runInput](input)
	if err != nil {
		return nil, runPlanWrap(i18n.KeyToolRunInvalidInput, err)
	}
	if len(in.Steps) == 0 {
		return nil, runPlanError(i18n.KeyToolRunStepsRequired)
	}
	if len(in.Steps) > maxRunSteps {
		return nil, runPlanError(i18n.KeyToolRunTooManySteps, maxRunSteps)
	}
	headLines := defaultRunHeadLines
	if in.Head != nil {
		headLines = *in.Head
	}
	tailLines := defaultRunTailLines
	if in.Tail != nil {
		tailLines = *in.Tail
	}
	maxChars := defaultRunMaxChars
	if in.MaxChars != nil {
		maxChars = *in.MaxChars
	}
	if headLines < 0 || headLines > maxRunOutputLines || tailLines < 0 || tailLines > maxRunOutputLines || maxChars < minRunMaxChars || maxChars > maxRunMaxChars {
		return nil, runPlanError(i18n.KeyToolRunOutputBounds)
	}

	baseCWD := strings.TrimSpace(scope.cwd)
	if baseCWD == "" {
		baseCWD, err = os.Getwd()
		if err != nil {
			return nil, runPlanWrap(i18n.KeyToolRunInvalidInput, err)
		}
	}
	baseCWD, err = filepath.Abs(baseCWD)
	if err != nil {
		return nil, runPlanWrap(i18n.KeyToolRunInvalidInput, err)
	}
	baseCWD = resolveExistingPathPrefix(baseCWD)
	allowedDirs := append([]string(nil), scope.allowedDirs...)
	if len(runtime.AllowedDirs) > 0 {
		allowedDirs = append([]string(nil), runtime.AllowedDirs...)
	}

	plan := &compiledRunPlan{
		steps: make([]compiledRunStep, len(in.Steps)), failFast: in.FailFast,
		headLines: headLines, tailLines: tailLines, maxChars: maxChars, readOnly: true,
		requiresPatchCommit: in.RequiresPatchCommit,
	}
	ids := make(map[string]int, len(in.Steps))
	for index, raw := range in.Steps {
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			return nil, runPlanError(i18n.KeyToolRunStepIDRequired, index+1)
		}
		if !runStepIDPattern.MatchString(id) {
			return nil, runPlanError(i18n.KeyToolRunStepIDInvalid, id)
		}
		if _, exists := ids[id]; exists {
			return nil, runPlanError(i18n.KeyToolRunStepIDDuplicate, id)
		}
		ids[id] = index

		hasArgv, hasShell := raw.Argv != nil, raw.ShellScript != nil
		if hasArgv == hasShell {
			return nil, runPlanError(i18n.KeyToolRunCommandChoice, id)
		}
		step := compiledRunStep{index: index, id: id}
		if hasArgv {
			step.argv = append([]string(nil), (*raw.Argv)...)
			if len(step.argv) == 0 || strings.TrimSpace(step.argv[0]) == "" {
				return nil, runPlanError(i18n.KeyToolRunArgumentInvalid, id, 0)
			}
			total := 0
			for argumentIndex, argument := range step.argv {
				total += len(argument)
				if strings.IndexByte(argument, 0) >= 0 || total > maxRunStepInputSize {
					return nil, runPlanError(i18n.KeyToolRunArgumentInvalid, id, argumentIndex)
				}
			}
			step.command = shellJoinArgv(step.argv)
		} else {
			step.useShell = true
			step.shellScript = *raw.ShellScript
			if strings.TrimSpace(step.shellScript) == "" || strings.IndexByte(step.shellScript, 0) >= 0 || len(step.shellScript) > maxRunStepInputSize {
				return nil, runPlanError(i18n.KeyToolRunCommandChoice, id)
			}
			step.command = step.shellScript
		}

		step.cwd, err = normalizeRunCWD(baseCWD, raw.CWD)
		if err != nil {
			return nil, runPlanWrap(i18n.KeyToolRunCWDInvalid, err, id, raw.CWD)
		}
		info, statErr := os.Stat(step.cwd)
		if statErr != nil {
			return nil, runPlanWrap(i18n.KeyToolRunCWDInvalid, statErr, id, step.cwd)
		}
		if !info.IsDir() {
			return nil, runPlanError(i18n.KeyToolRunCWDNotDirectory, id, step.cwd)
		}
		if err := ValidatePathsAgainstAllowedDirs([]string{step.cwd}, allowedDirs); err != nil {
			return nil, runPlanWrap(i18n.KeyToolRunCWDInvalid, err, id, step.cwd)
		}

		timeoutMS := defaultRunTimeoutMS
		if raw.TimeoutMS != nil {
			timeoutMS = *raw.TimeoutMS
		}
		if timeoutMS <= 0 || timeoutMS > getMaxBashTimeoutMs() {
			return nil, runPlanError(i18n.KeyToolRunTimeoutInvalid, id, getMaxBashTimeoutMs())
		}
		step.timeout = time.Duration(timeoutMS) * time.Millisecond

		policyContext := scope.shellPolicyContext(runtime, !avoidPrompts)
		policyContext.CWD = step.cwd
		policyContext.AllowedDirs = append([]string(nil), allowedDirs...)
		step.policy, step.sed = analyzeBashCommandWithSedEvidencePolicy(step.command, policyContext)
		step.semantics = ClassifyCommand(step.command)
		step.verificationKind = classifyRunStepVerification(step)
		step.readOnly = step.semantics == SemanticRead && IsReadOnlyCommand(step.command, step.semantics) && step.policy.Disposition == types.PolicyAllow
		step.effect = step.semantics.String()
		step.resources = runStepResources(baseCWD, step.cwd, step.command, step.semantics)
		paths := resolvePathsAgainstCWD(FilterBashPathScopeExemptions(ExtractPathsFromCommand(step.command)), step.cwd)
		if err := ValidatePathsAgainstAllowedDirs(paths, allowedDirs); err != nil {
			return nil, runPlanWrap(i18n.KeyToolRunCWDInvalid, err, id, step.cwd)
		}
		if !step.readOnly {
			plan.readOnly = false
		}
		if step.semantics == SemanticDestructive {
			plan.destructive = true
		}
		plan.steps[index] = step
	}

	indegree := make([]int, len(plan.steps))
	dependents := make([][]int, len(plan.steps))
	for index, raw := range in.Steps {
		seen := make(map[string]struct{}, len(raw.DependsOn))
		for _, dependencyID := range raw.DependsOn {
			dependencyID = strings.TrimSpace(dependencyID)
			if dependencyID == plan.steps[index].id {
				return nil, runPlanError(i18n.KeyToolRunDependencySelf, plan.steps[index].id)
			}
			if _, exists := seen[dependencyID]; exists {
				return nil, runPlanError(i18n.KeyToolRunDependencyDuplicate, plan.steps[index].id, dependencyID)
			}
			seen[dependencyID] = struct{}{}
			dependencyIndex, exists := ids[dependencyID]
			if !exists {
				return nil, runPlanError(i18n.KeyToolRunDependencyUnknown, plan.steps[index].id, dependencyID)
			}
			plan.steps[index].dependsOn = append(plan.steps[index].dependsOn, dependencyIndex)
			indegree[index]++
			dependents[dependencyIndex] = append(dependents[dependencyIndex], index)
		}
		sort.Ints(plan.steps[index].dependsOn)
	}
	ready := make([]int, 0, len(plan.steps))
	for index, degree := range indegree {
		if degree == 0 {
			ready = append(ready, index)
		}
	}
	visited := 0
	for len(ready) > 0 {
		index := ready[0]
		ready = ready[1:]
		visited++
		for _, dependent := range dependents[index] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	if visited != len(plan.steps) {
		return nil, runPlanError(i18n.KeyToolRunDependencyCycle)
	}
	configureRunRevisionSafety(plan, baseCWD, scope.runVerificationRoot)

	plan.bindingCode = runPlanBindingCode(plan)
	return plan, nil
}

func normalizeRunCWD(base, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return base, nil
	}
	for _, value := range candidate {
		if value == 0 || value < 0x20 || value == 0x7f {
			return "", os.ErrInvalid
		}
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	return resolveExistingPathPrefix(absolute), nil
}

func shellJoinArgv(argv []string) string {
	quoted := make([]string, len(argv))
	for index, argument := range argv {
		if argument != "" && strings.IndexFunc(argument, func(value rune) bool {
			return !(value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || strings.ContainsRune("_@%+=:,./-", value))
		}) == -1 {
			quoted[index] = argument
			continue
		}
		quoted[index] = "'" + strings.ReplaceAll(argument, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func runStepResources(baseCWD, cwd, command string, semantics CommandSemantic) []string {
	seen := make(map[string]struct{})
	resources := make([]string, 0, 4)
	add := func(value string) {
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		resources = append(resources, value)
	}
	add(runPathResource(baseCWD, cwd))
	for _, path := range resolvePathsAgainstCWD(FilterBashPathScopeExemptions(ExtractPathsFromCommand(command)), cwd) {
		add(runPathResource(baseCWD, path))
	}
	switch semantics {
	case SemanticNetwork:
		add("network")
	case SemanticProcess:
		add("process")
	case SemanticUnknown:
		add("external")
	}
	sort.Strings(resources)
	return resources
}

func runPathResource(baseCWD, path string) string {
	path = filepath.Clean(path)
	if relative, err := filepath.Rel(baseCWD, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		if relative == "." {
			return "workspace:."
		}
		return "workspace:" + filepath.ToSlash(relative)
	}
	return "filesystem:" + filepath.ToSlash(path)
}

func runPlanBindingCode(plan *compiledRunPlan) string {
	type bindingStep struct {
		ID               string   `json:"id"`
		Argv             []string `json:"argv,omitempty"`
		Script           string   `json:"script,omitempty"`
		CWD              string   `json:"cwd"`
		Timeout          int64    `json:"timeout"`
		DependsOn        []int    `json:"depends_on,omitempty"`
		Policy           string   `json:"policy"`
		Effect           string   `json:"effect"`
		Resources        []string `json:"resources"`
		VerificationKind string   `json:"verification_kind,omitempty"`
		FormatterWrites  []string `json:"formatter_writes,omitempty"`
		ManagedRoot      string   `json:"managed_root,omitempty"`
	}
	bound := struct {
		Steps               []bindingStep `json:"steps"`
		FailFast            bool          `json:"fail_fast"`
		RequiresPatchCommit bool          `json:"requires_patch_commit"`
		Head                int           `json:"head"`
		Tail                int           `json:"tail"`
		MaxChars            int           `json:"max_chars"`
	}{FailFast: plan.failFast, RequiresPatchCommit: plan.requiresPatchCommit, Head: plan.headLines, Tail: plan.tailLines, MaxChars: plan.maxChars}
	for _, step := range plan.steps {
		bound.Steps = append(bound.Steps, bindingStep{
			ID: step.id, Argv: append([]string(nil), step.argv...), Script: step.shellScript,
			CWD: step.cwd, Timeout: step.timeout.Milliseconds(), DependsOn: append([]int(nil), step.dependsOn...),
			Policy: step.policy.ExecutionBindingCode(), Effect: step.effect, Resources: append([]string(nil), step.resources...),
			VerificationKind: step.verificationKind, FormatterWrites: append([]string(nil), step.formatterWrites...), ManagedRoot: step.managedRoot,
		})
	}
	encoded, err := json.Marshal(bound)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return "run.plan." + hex.EncodeToString(digest[:])
}
