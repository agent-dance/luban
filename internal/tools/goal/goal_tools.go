package goaltool

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

const goalToolMaxResultSizeChars = 100_000

func runtimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func runtimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func runtimeError(key i18n.Key) types.ToolResult {
	return types.ToolResult{Content: runtimeText(key), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func runtimeErrorf(key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{Content: runtimeFormat(key, args...), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func errorResponse(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}

// GoalRuntime is the session-scoped persistence port shared with slash-command
// and query-loop goal consumers.
type GoalRuntime interface {
	LoadGoal() (*goal.Goal, error)
	UpdateGoal(goal.UpdateFunc) (goal.Goal, error)
}

// ContextGoalRuntime routes goal persistence using the immutable
// identity attached to a tool execution instead of the currently focused UI.
type ContextGoalRuntime interface {
	GoalRuntime
	LoadGoalForContext(context.Context) (*goal.Goal, error)
	UpdateGoalForContext(context.Context, goal.UpdateFunc) (goal.Goal, error)
}

type goalToolRuntimeRef struct {
	mu       sync.RWMutex
	mutateMu sync.Mutex
	runtime  GoalRuntime
}

func newGoalToolRuntimeRef(runtime GoalRuntime) *goalToolRuntimeRef {
	return &goalToolRuntimeRef{runtime: runtime}
}

func (r *goalToolRuntimeRef) set(runtime GoalRuntime) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.runtime = runtime
	r.mu.Unlock()
}

func (r *goalToolRuntimeRef) get() GoalRuntime {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	runtime := r.runtime
	r.mu.RUnlock()
	if goalRuntimeIsNil(runtime) {
		return nil
	}
	return runtime
}

func goalRuntimeIsNil(runtime GoalRuntime) bool {
	if runtime == nil {
		return true
	}
	value := reflect.ValueOf(runtime)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type GetGoalTool struct {
	runtime *goalToolRuntimeRef
}

type CreateGoalTool struct {
	runtime *goalToolRuntimeRef
}

type UpdateGoalTool struct {
	runtime *goalToolRuntimeRef
}

// NewGoalTools constructs the three model-facing tools over one synchronized
// runtime reference so create and terminal updates cannot race each other.
func NewGoalTools(runtime GoalRuntime) (*GetGoalTool, *CreateGoalTool, *UpdateGoalTool) {
	ref := newGoalToolRuntimeRef(runtime)
	return &GetGoalTool{runtime: ref}, &CreateGoalTool{runtime: ref}, &UpdateGoalTool{runtime: ref}
}

func (t *GetGoalTool) SetRuntime(runtime GoalRuntime) {
	if t != nil {
		t.runtime.set(runtime)
	}
}

func (t *CreateGoalTool) SetRuntime(runtime GoalRuntime) {
	if t != nil {
		t.runtime.set(runtime)
	}
}

func (t *UpdateGoalTool) SetRuntime(runtime GoalRuntime) {
	if t != nil {
		t.runtime.set(runtime)
	}
}

func (t *GetGoalTool) Name() string    { return "GetGoal" }
func (t *CreateGoalTool) Name() string { return "CreateGoal" }
func (t *UpdateGoalTool) Name() string { return "UpdateGoal" }

func (t *GetGoalTool) Description() string {
	return "Get the current persisted session goal, revision, acceptance criteria, evaluation, and status."
}

func (t *CreateGoalTool) Description() string {
	return "Create a persisted session goal with explicit, independently verifiable acceptance criteria only when the user requests one. An unfinished goal must not be replaced."
}

func (t *UpdateGoalTool) Description() string {
	return "Revise Agent-authored acceptance criteria, or mark the active session goal complete or blocked. Revising requires the expected current revision. Complete is accepted only when every criterion in that revision has passed."
}

func (t *GetGoalTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *CreateGoalTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"objective": map[string]any{
			"type":        "string",
			"description": "The outcome to achieve; acceptance_criteria separately decide completion",
			"maxLength":   goal.MaxObjectiveCharacters,
		},
		"acceptance_criteria": map[string]any{
			"type":        "array",
			"description": "One to twenty explicit, independently verifiable conditions that must all be met",
			"items": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": goal.MaxAcceptanceCriterionCharacters,
			},
			"minItems": 1,
			"maxItems": goal.MaxAcceptanceCriteria,
		},
		"token_budget": map[string]any{
			"type":        "integer",
			"description": "Optional positive token budget for the goal",
			"minimum":     1,
		},
	}, "objective", "acceptance_criteria")
}

func (t *UpdateGoalTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"status": map[string]any{
			"type":        "string",
			"description": "Action to apply to the goal",
			"enum":        []string{"complete", "blocked", "revise"},
		},
		"acceptance_criteria": map[string]any{
			"type":        "array",
			"description": "Complete replacement acceptance contract when status is revise",
			"items": map[string]any{
				"type": "string", "minLength": 1, "maxLength": goal.MaxAcceptanceCriterionCharacters,
			},
			"minItems": 1, "maxItems": goal.MaxAcceptanceCriteria,
		},
		"expected_revision": map[string]any{
			"type": "integer", "description": "Revision returned by GetGoal", "minimum": 1,
		},
	}, "status")
}

func (t *GetGoalTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: goalToolMaxResultSizeChars}
}

func (t *CreateGoalTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: goalToolMaxResultSizeChars}
}

func (t *UpdateGoalTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: goalToolMaxResultSizeChars}
}

func (t *GetGoalTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{AlwaysLoad: true, SearchHint: "inspect the current persisted session goal"}
}

func (t *CreateGoalTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{AlwaysLoad: true, SearchHint: "create a persisted session goal when explicitly requested"}
}

func (t *UpdateGoalTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{AlwaysLoad: true, SearchHint: "revise goal acceptance criteria or mark the goal complete or blocked"}
}

func (t *GetGoalTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return strings.TrimSpace(runtime.AgentID) == ""
}

func (t *CreateGoalTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return strings.TrimSpace(runtime.AgentID) == ""
}

func (t *UpdateGoalTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return strings.TrimSpace(runtime.AgentID) == ""
}

type getGoalInput struct{}

type createGoalInput struct {
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	TokenBudget        *int     `json:"token_budget,omitempty"`
}

type updateGoalInput struct {
	Status             string   `json:"status"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	ExpectedRevision   *int     `json:"expected_revision,omitempty"`
}

type GoalToolResult struct {
	Goal *goal.Goal `json:"goal"`
}

func (t *GetGoalTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	if _, toolErr := toolbase.ParseStrictInputOrError[getGoalInput](input); toolErr != nil {
		return *toolErr, nil
	}
	runtime := t.runtime.get()
	if runtime == nil {
		return runtimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	current, err := loadGoalForToolContext(ctx, runtime)
	if err != nil {
		return runtimeErrorf(i18n.KeyToolGoalLoadFailed, err), nil
	}
	data := GoalToolResult{Goal: cloneGoal(current)}
	return types.ToolResult{Content: getGoalModelText(data), Data: data}, nil
}

func (t *CreateGoalTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	in, toolErr := toolbase.ParseStrictInputOrError[createGoalInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if in.TokenBudget != nil && *in.TokenBudget <= 0 {
		return runtimeError(i18n.KeyToolGoalTokenBudgetPositive), nil
	}
	if t == nil || t.runtime == nil || t.runtime.get() == nil {
		return runtimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}

	t.runtime.mutateMu.Lock()
	defer t.runtime.mutateMu.Unlock()
	runtime := t.runtime.get()
	if runtime == nil {
		return runtimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	tokenBudget := 0
	if in.TokenBudget != nil {
		tokenBudget = *in.TokenBudget
	}
	create := func(current *goal.Goal) (goal.Goal, error) {
		if current != nil && current.Status != goal.StatusAchieved && current.Status != goal.StatusCleared {
			return goal.Goal{}, fmt.Errorf("%s", runtimeFormat(i18n.KeyToolGoalReplaceUnfinished, goalStatusText(current.Status)))
		}
		next, err := goal.CreateWithCriteria(in.Objective, in.AcceptanceCriteria, tokenBudget, time.Now().UTC())
		return next, localizeGoalToolError(err)
	}
	next, err := updateGoalAtomicallyForToolContext(ctx, runtime, create)
	if err != nil {
		return errorResponse(err), nil
	}
	data := GoalToolResult{Goal: cloneGoal(&next)}
	return types.ToolResult{Content: createGoalModelText(data), Data: data}, nil
}

func (t *UpdateGoalTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	in, toolErr := toolbase.ParseStrictInputOrError[updateGoalInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if in.Status != "complete" && in.Status != "blocked" && in.Status != "revise" {
		return runtimeError(i18n.KeyToolGoalStatusRequired), nil
	}
	if in.Status == "revise" && (in.ExpectedRevision == nil || *in.ExpectedRevision <= 0) {
		return runtimeError(i18n.KeyToolGoalRevisionRequired), nil
	}
	if in.Status != "revise" && (in.ExpectedRevision != nil || len(in.AcceptanceCriteria) > 0) {
		return runtimeError(i18n.KeyToolGoalRevisionFieldsUnexpected), nil
	}
	if t == nil || t.runtime == nil || t.runtime.get() == nil {
		return runtimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}

	t.runtime.mutateMu.Lock()
	defer t.runtime.mutateMu.Unlock()
	runtime := t.runtime.get()
	if runtime == nil {
		return runtimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	update := func(current *goal.Goal) (goal.Goal, error) {
		if current == nil {
			return goal.Goal{}, fmt.Errorf("%s", runtimeText(i18n.KeyToolGoalNoActive))
		}

		now := time.Now().UTC()
		switch in.Status {
		case "complete":
			if current.Status != goal.StatusActive {
				return goal.Goal{}, fmt.Errorf("%s", runtimeFormat(i18n.KeyToolGoalCannotAchieve, goalStatusText(current.Status)))
			}
			if !goal.AcceptanceCriteriaMet(*current) {
				return goal.Goal{}, fmt.Errorf("%s", runtimeText(i18n.KeyRootGoalAcceptanceCriteriaUnmet))
			}
			next, err := goal.Achieve(*current, current.LastAcceptanceEvaluation.Summary, now)
			if err == nil {
				next = goal.SetEvaluatorReason(next, next.LastEvaluatorReason, goal.EvaluatorReasonModelDone, "", "", now)
			}
			return next, err
		case "blocked":
			if current.Status != goal.StatusActive {
				return goal.Goal{}, fmt.Errorf("%s", runtimeFormat(i18n.KeyToolGoalCannotBlock, goalStatusText(current.Status)))
			}
			next, err := goal.Block(*current, runtimeText(i18n.KeyToolGoalReasonBlocked), now)
			if err == nil {
				next = goal.SetEvaluatorReason(next, next.LastEvaluatorReason, goal.EvaluatorReasonModelBlocked, "", "", now)
			}
			return next, err
		case "revise":
			normalized := goal.Normalize(*current)
			if normalized.Revision != *in.ExpectedRevision {
				return goal.Goal{}, fmt.Errorf("%s", runtimeText(i18n.KeyToolGoalRevisionStale))
			}
			next, err := goal.ReplaceAcceptanceCriteria(*current, in.AcceptanceCriteria, now)
			return next, localizeGoalToolError(err)
		default:
			return goal.Goal{}, fmt.Errorf("%s", runtimeText(i18n.KeyToolGoalStatusRequired))
		}
	}
	next, err := updateGoalAtomicallyForToolContext(ctx, runtime, update)
	if err != nil {
		return errorResponse(err), nil
	}
	data := GoalToolResult{Goal: cloneGoal(&next)}
	return types.ToolResult{Content: updateGoalModelText(goalToolUpdateResultStatus(in.Status), data), Data: data}, nil
}

func goalToolUpdateResultStatus(status string) string {
	if status == "revise" {
		return "updated"
	}
	return status
}

func loadGoalForToolContext(ctx context.Context, runtime GoalRuntime) (*goal.Goal, error) {
	if contextual, ok := contextualGoalRuntime(ctx, runtime); ok {
		return contextual.LoadGoalForContext(ctx)
	}
	return runtime.LoadGoal()
}

func updateGoalAtomicallyForToolContext(ctx context.Context, runtime GoalRuntime, update goal.UpdateFunc) (goal.Goal, error) {
	if hasGoalToolExecutionContext(ctx) {
		contextual, ok := runtime.(ContextGoalRuntime)
		if !ok {
			return goal.Goal{}, i18n.NewError(i18n.KeyToolGoalRuntimeUnavailable)
		}
		return contextual.UpdateGoalForContext(ctx, update)
	}
	return runtime.UpdateGoal(update)
}

func hasGoalToolExecutionContext(ctx context.Context) bool {
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok || strings.TrimSpace(exec.SessionID) == "" {
		return false
	}
	return strings.TrimSpace(exec.SessionProjectDir) != "" || strings.TrimSpace(exec.ProjectRoot) != ""
}

func contextualGoalRuntime(ctx context.Context, runtime GoalRuntime) (ContextGoalRuntime, bool) {
	contextual, ok := runtime.(ContextGoalRuntime)
	if !ok {
		return nil, false
	}
	if !hasGoalToolExecutionContext(ctx) {
		return nil, false
	}
	return contextual, true
}

func (t *GetGoalTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := goalToolResult(data)
	if !ok {
		return invalidGoalToolResult("GetGoal", toolUseID)
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: getGoalModelText(result)}
}

func (t *CreateGoalTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := goalToolResult(data)
	if !ok {
		return invalidGoalToolResult("CreateGoal", toolUseID)
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: createGoalModelText(result)}
}

func (t *UpdateGoalTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := goalToolResult(data)
	if !ok {
		return invalidGoalToolResult("UpdateGoal", toolUseID)
	}
	status := "updated"
	if result.Goal != nil {
		switch result.Goal.Status {
		case goal.StatusAchieved:
			status = "complete"
		case goal.StatusBlocked:
			status = "blocked"
		}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: updateGoalModelText(status, result)}
}

func goalToolResult(data any) (GoalToolResult, bool) {
	switch value := data.(type) {
	case GoalToolResult:
		return value, true
	case *GoalToolResult:
		if value != nil {
			return *value, true
		}
	}
	return GoalToolResult{}, false
}

func invalidGoalToolResult(name, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   runtimeFormat(i18n.KeyToolGoalInvalidTypedResult, name),
		IsError:   true,
	}
}

func getGoalModelText(result GoalToolResult) string {
	if result.Goal == nil {
		return runtimeText(i18n.KeyToolGoalNone)
	}
	currentValue := goal.Normalize(*result.Goal)
	current := &currentValue
	lines := []string{
		runtimeFormat(i18n.KeyToolGoalLabelGoal, current.Objective),
		runtimeFormat(i18n.KeyToolGoalLabelStatus, goalStatusText(current.Status)),
	}
	lines = append(lines, goalAcceptanceCriteriaModelLines(current)...)
	if current.TokenBudget > 0 {
		lines = append(lines, runtimeFormat(i18n.KeyToolGoalTokenUsageBudget, current.Usage, current.TokenBudget))
	} else if current.Usage > 0 {
		lines = append(lines, runtimeFormat(i18n.KeyToolGoalTokenUsage, current.Usage))
	}
	if current.TurnCount > 0 {
		lines = append(lines, runtimeFormat(i18n.KeyToolGoalEvaluatedTurns, current.TurnCount))
	}
	if reason := strings.TrimSpace(current.LastEvaluatorReason); reason != "" {
		lines = append(lines, runtimeFormat(i18n.KeyToolGoalLastEvaluation, localizedGoalReason(current)))
	}
	return strings.Join(lines, "\n")
}

func localizedGoalReason(current *goal.Goal) string {
	if current == nil {
		return ""
	}
	return i18n.RootGoalEvaluatorReasonStateLabel(
		i18n.DetectOrLoadLanguage(), current.LastEvaluatorReason,
		string(current.LastEvaluatorReasonKind), current.LastEvaluatorReasonKey,
		current.LastEvaluatorReasonDetail,
	)
}

func createGoalModelText(result GoalToolResult) string {
	if result.Goal == nil {
		return runtimeText(i18n.KeyToolGoalCreationEmpty)
	}
	return runtimeFormat(i18n.KeyToolGoalCreated, result.Goal.Objective) + "\n" + strings.Join(goalAcceptanceCriteriaModelLines(result.Goal), "\n")
}

func updateGoalModelText(status string, result GoalToolResult) string {
	if result.Goal == nil {
		return runtimeText(i18n.KeyToolGoalUpdateEmpty)
	}
	return runtimeFormat(i18n.KeyToolGoalMarked, goalUpdateStatusText(status), result.Goal.Objective) + "\n" +
		strings.Join(goalAcceptanceCriteriaModelLines(result.Goal), "\n")
}

func goalAcceptanceCriteriaModelLines(current *goal.Goal) []string {
	if current == nil {
		return nil
	}
	normalized := goal.Normalize(*current)
	lines := []string{runtimeFormat(i18n.KeyToolGoalAcceptanceCriteriaHeader, normalized.Revision)}
	evaluated := make(map[string]goal.AcceptanceCriterionEvaluation)
	if normalized.LastAcceptanceEvaluation != nil && normalized.LastAcceptanceEvaluation.Revision == normalized.Revision {
		for _, result := range normalized.LastAcceptanceEvaluation.Criteria {
			evaluated[strings.ToUpper(result.CriterionID)] = result
		}
	}
	for _, criterion := range normalized.AcceptanceCriteria {
		result, ok := evaluated[strings.ToUpper(criterion.ID)]
		key := i18n.KeyToolGoalAcceptanceCriterionItem
		if ok && result.Met {
			key = i18n.KeyToolGoalAcceptanceCriterionMet
		} else if ok {
			key = i18n.KeyToolGoalAcceptanceCriterionUnmet
		}
		lines = append(lines, runtimeFormat(key, criterion.ID, criterion.Text))
		if ok && strings.TrimSpace(result.Reason) != "" {
			lines = append(lines, runtimeFormat(i18n.KeyToolGoalAcceptanceReason, result.Reason))
		}
	}
	return lines
}

func localizeGoalToolError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, goal.ErrObjectiveRequired):
		return fmt.Errorf("%s", runtimeText(i18n.KeyToolGoalObjectiveRequired))
	case errors.Is(err, goal.ErrObjectiveTooLong):
		return fmt.Errorf("%s", runtimeText(i18n.KeyToolGoalObjectiveTooLong))
	case errors.Is(err, goal.ErrAcceptanceCriteriaRequired):
		return fmt.Errorf("%s", runtimeText(i18n.KeyRootGoalAcceptanceCriteriaRequired))
	case errors.Is(err, goal.ErrAcceptanceCriteriaTooMany):
		return fmt.Errorf("%s", runtimeFormat(i18n.KeyRootGoalAcceptanceCriteriaTooMany, goal.MaxAcceptanceCriteria))
	case errors.Is(err, goal.ErrAcceptanceCriterionRequired):
		return fmt.Errorf("%s", runtimeText(i18n.KeyRootGoalAcceptanceCriterionRequired))
	case errors.Is(err, goal.ErrAcceptanceCriterionTooLong):
		return fmt.Errorf("%s", runtimeFormat(i18n.KeyRootGoalAcceptanceCriterionTooLong, goal.MaxAcceptanceCriterionCharacters))
	case errors.Is(err, goal.ErrAcceptanceCriterionDuplicate):
		return fmt.Errorf("%s", runtimeText(i18n.KeyRootGoalAcceptanceCriterionDuplicate))
	case errors.Is(err, goal.ErrAcceptanceCriteriaUnmet):
		return fmt.Errorf("%s", runtimeText(i18n.KeyRootGoalAcceptanceCriteriaUnmet))
	default:
		return err
	}
}

func goalStatusText(status goal.Status) string {
	var key i18n.Key
	switch status {
	case goal.StatusActive:
		key = i18n.KeyToolGoalStatusActive
	case goal.StatusPaused:
		key = i18n.KeyToolGoalStatusPaused
	case goal.StatusAchieved:
		key = i18n.KeyToolGoalStatusAchieved
	case goal.StatusBlocked:
		key = i18n.KeyToolGoalStatusBlocked
	case goal.StatusCleared:
		key = i18n.KeyToolGoalStatusCleared
	default:
		return string(status)
	}
	return runtimeText(key)
}

func goalUpdateStatusText(status string) string {
	switch status {
	case "complete":
		return runtimeText(i18n.KeyToolGoalStatusComplete)
	case "blocked":
		return runtimeText(i18n.KeyToolGoalStatusBlocked)
	case "updated":
		return runtimeText(i18n.KeyToolGoalStatusUpdated)
	default:
		return status
	}
}

func cloneGoal(current *goal.Goal) *goal.Goal {
	if current == nil {
		return nil
	}
	cloned := goal.Normalize(*current)
	if current.AchievedAt != nil {
		value := *current.AchievedAt
		cloned.AchievedAt = &value
	}
	if current.BlockedAt != nil {
		value := *current.BlockedAt
		cloned.BlockedAt = &value
	}
	return &cloned
}
