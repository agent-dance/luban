package interaction

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

// ─── EnterPlanModeTool ─────────────────────────────────────────────────────

// EnterPlanModeTool signals entry into plan mode. Entry reserves an internal
// path without creating or exposing the plan file; ExitPlanMode consumes that
// path after a plan is materialized.
type EnterPlanModeTool struct {
	State   *PlanState
	Runtime planModePermissionRuntime
	mu      sync.Mutex
}

func NewEnterPlanModeTool(state *PlanState, runtime planModePermissionRuntime) *EnterPlanModeTool {
	tool := &EnterPlanModeTool{State: state, Runtime: runtime}
	if state != nil && runtime != nil {
		state.bindPermissionRuntime(runtime)
	}
	return tool
}

func (t *EnterPlanModeTool) Name() string { return "EnterPlanMode" }

func (t *EnterPlanModeTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *EnterPlanModeTool) Description() string {
	return enterPlanModePrompt()
}

func (t *EnterPlanModeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

type enterPlanModeResult struct {
	Message string `json:"message"`
}

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
	if t.State.IsActive() {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanAlreadyActive), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	planFile, err := reservePlanFilePath(t.State.projectRootPath())
	if err != nil {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanPrepareFailed, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
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
	data := enterPlanModeResult{Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanEntered)}
	return types.ToolResult{Content: data.Message, Data: data}, nil
}

func (t *EnterPlanModeTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(enterPlanModeResult)
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID}
	}
	instructions := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPlanModeInstructions, result.Message)
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: instructions}
}

// reservePlanFilePath returns a future plan path without creating the plan
// artifact. The directory is prepared so existing ExitPlanMode round-trips can
// materialize the plan later with a simple write.
func reservePlanFilePath(projectRoot string) (string, error) {
	plansDir := filepath.Join(projectRoot, ".luban-code", "plans")
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
func persistEditedPlan(projectRoot, originalPath, plan string) (string, error) {
	target := strings.TrimSpace(originalPath)
	if target == "" {
		// No prior plan path — synthesise one under the plans dir so the
		// curated content lives alongside other plan artifacts.
		plansDir := filepath.Join(projectRoot, ".luban-code", "plans")
		if err := os.MkdirAll(plansDir, 0755); err != nil {
			return "", i18n.WrapError(i18n.KeyToolPlanModeCreatePlans, err)
		}
		target = filepath.Join(plansDir, "plan-"+newPlanSessionID()+".md")
	} else if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", i18n.WrapError(i18n.KeyToolPlanModeCreatePlanDirectory, err)
	}
	if err := secureio.AtomicWriteFile(target, []byte(plan), 0600); err != nil {
		return "", err
	}
	return target, nil
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
