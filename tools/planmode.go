package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ─── EnterPlanModeTool ─────────────────────────────────────────────────────

// EnterPlanModeTool signals entry into plan mode. The TS baseline does not
// create or expose a plan file during entry; Go keeps only an internal future
// plan path so ExitPlanMode and legacy callers still have a stable location if
// a plan is later materialized.
type EnterPlanModeTool struct {
	State   *PlanState
	Runtime planModePermissionRuntime
	mu      sync.Mutex
}

func NewEnterPlanModeTool(state *PlanState, runtime ...planModePermissionRuntime) *EnterPlanModeTool {
	tool := &EnterPlanModeTool{State: state}
	if len(runtime) > 0 {
		tool.Runtime = runtime[0]
	}
	if state != nil && tool.Runtime != nil {
		state.bindPermissionRuntime(tool.Runtime)
	}
	return tool
}

func (t *EnterPlanModeTool) Name() string { return "EnterPlanMode" }

func (t *EnterPlanModeTool) Description() string {
	return enterPlanModePrompt()
}

func (t *EnterPlanModeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *EnterPlanModeTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Confirmation that plan mode was entered",
				},
			},
			Required:             []string{"message"},
			AdditionalProperties: false,
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

type enterPlanModeResult struct {
	Message string `json:"message"`
}

const enterPlanModeMessage = "Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach."

func (t *EnterPlanModeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.State == nil {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolPlanModeStateMissing)
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(input) > 0 {
		if err := types.ValidateToolInput(t, input); err != nil {
			// i18n:allow display-literal protocol -- Tool-use error tags and InputValidationError are model protocol grammar.
			return types.ToolResult{Content: "<tool_use_error>InputValidationError: " + err.Error() + "</tool_use_error>", IsError: true}, nil
		}
	}
	runtime, err := t.validateRuntimeContext()
	if err != nil {
		return types.ToolResult{}, err
	}
	if runtime.ChannelsActive || AskUserChannelsActive() {
		return enterPlanModeUnavailableResult(), nil
	}
	if t.State.IsActive() {
		return toolRuntimeError(i18n.KeyToolPlanAlreadyActive), nil
	}
	// PM-04: refuse plan-mode entry while an interview phase is active. The
	// pre-plan permission snapshot would be corrupted by a nested transition,
	// leaving permissions over-restricted on exit. Surface a structured
	// IsError so the model can defer until the interview completes.
	if phase := strings.TrimSpace(t.State.InterviewPhase()); phase != "" {
		return toolRuntimeErrorf(i18n.KeyToolPlanInterviewActive, phase), nil
	}
	planFile, err := reservePlanFilePath()
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolPlanPrepareFailed, err), nil
	}

	snapshot, err := t.transitionToPlanMode(ctx, runtime)
	if err != nil {
		return types.ToolResult{}, err
	}
	if err := t.State.enterWithSnapshot(planFile, snapshot); err != nil {
		if t.Runtime != nil {
			if previous, _ := snapshot["permission_mode"].(string); previous != "" {
				_ = t.Runtime.TransitionPermissionMode(previous)
			}
		}
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolPlanModePersistState, err)
	}
	data := enterPlanModeResult{Message: toolRuntimeText(i18n.KeyToolPlanEntered)}
	return types.ToolResult{Content: data.Message, Data: data}, nil
}

func (t *EnterPlanModeTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(enterPlanModeResult)
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID}
	}
	if enterPlanModeInterviewPhaseEnabled() {
		instructions := toolRuntimeFormat(i18n.KeyToolPlanModeInterviewInstructions, result.Message)
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: instructions}
	}
	instructions := toolRuntimeFormat(i18n.KeyToolPlanModeInstructions, result.Message)
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: instructions}
}

// reservePlanFilePath returns a future plan path without creating the plan
// artifact. The directory is prepared so existing ExitPlanMode round-trips can
// materialize the plan later with a simple write.
func reservePlanFilePath() (string, error) {
	plansDir := filepath.Join(brand.ConfigDirName, "plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		return "", i18n.WrapError(i18n.KeyToolPlanModeCreateDirectory, err, plansDir)
	}
	sessionID := newPlanSessionID()
	planFile := filepath.Join(plansDir, "plan-"+sessionID+".md")
	return planFile, nil
}

// newPlanSessionID returns a unique random session id used as the plan
// filename suffix. Falls back to a constant string only if crypto/rand
// is unavailable (extremely rare on supported platforms).
func newPlanSessionID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "session"
	}
	return hex.EncodeToString(b[:])
}

// persistEditedPlan atomically writes plan content to a deterministic path.
// PM-03: when the user curates the plan in the UI we replace the in-memory
// plan with the edited version so downstream readers (worktree resume, plan
// audit) see the same artifact. Atomicity guarantees a partially-written
// file never overwrites the prior valid plan.
func persistEditedPlan(originalPath, plan string) (string, error) {
	target := strings.TrimSpace(originalPath)
	if target == "" {
		// No prior plan path — synthesise one under the plans dir so the
		// curated content lives alongside other plan artifacts.
		plansDir := filepath.Join(brand.ConfigDirName, "plans")
		if err := os.MkdirAll(plansDir, 0755); err != nil {
			return "", i18n.WrapError(i18n.KeyToolPlanModeCreatePlans, err)
		}
		target = filepath.Join(plansDir, "plan-"+newPlanSessionID()+".md")
	} else if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", i18n.WrapError(i18n.KeyToolPlanModeCreatePlanDirectory, err)
	}
	if err := atomicWriteFile(target, []byte(plan), 0600); err != nil {
		return "", err
	}
	return target, nil
}

// AutoModeGateFn is consulted by ExitPlanMode when the captured pre-plan
// permission mode is auto-like. PM-02: returns (allow, reason). When allow=false the caller appends a
// structured fallback notice so the model knows why no auto-mode agent ran.
// Defaulting to nil makes ExitPlanMode behave identically to the prior
// release for runtimes that haven't wired up auto-mode.
var AutoModeGateFn func(ctx context.Context) (bool, string)

// SetAutoModeGateFn registers the resolver above; pass nil to clear.
func SetAutoModeGateFn(fn func(ctx context.Context) (bool, string)) {
	AutoModeGateFn = fn
}

func autoModeGateAllow(ctx context.Context) (bool, string) {
	if AutoModeGateFn == nil {
		return false, "auto-mode is not enabled in this runtime"
	}
	allow, reason := AutoModeGateFn(ctx)
	if reason == "" {
		if allow {
			reason = "ok"
		} else {
			reason = "auto-mode gate refused"
		}
	}
	return allow, reason
}

func filterAllowedPrompts(prompts []PlanAllowedPrompt) []PlanAllowedPrompt {
	filtered := make([]PlanAllowedPrompt, 0, len(prompts))
	seen := make(map[string]struct{}, len(prompts))
	for _, prompt := range prompts {
		tool := strings.TrimSpace(prompt.Tool)
		value := strings.TrimSpace(prompt.Prompt)
		if tool == "" || value == "" {
			continue
		}
		if tool != "Bash" {
			continue
		}
		key := tool + "\x00" + value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, PlanAllowedPrompt{Tool: tool, Prompt: value})
	}
	return filtered
}

func formatAllowedPromptsSummary(prompts []PlanAllowedPrompt) string {
	if len(prompts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(prompts)+1)
	lines = append(lines, "\n\nAllowed prompts:")
	for _, prompt := range prompts {
		lines = append(lines, fmt.Sprintf("- %s: %s", prompt.Tool, prompt.Prompt))
	}
	return strings.Join(lines, "\n")
}
