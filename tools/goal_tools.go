package tools

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const goalToolMaxResultSizeChars = 100_000

// GoalRuntime is the session-scoped persistence port shared with slash-command
// and query-loop goal consumers.
type GoalRuntime interface {
	LoadGoal() (*goal.Goal, error)
	SaveGoal(goal.Goal) error
}

// ContextGoalRuntime optionally routes goal persistence using the immutable
// identity attached to a tool execution instead of the currently focused UI.
type ContextGoalRuntime interface {
	GoalRuntime
	LoadGoalForContext(context.Context) (*goal.Goal, error)
	SaveGoalForContext(context.Context, goal.Goal) error
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

func NewGetGoalTool(runtime GoalRuntime) *GetGoalTool {
	return &GetGoalTool{runtime: newGoalToolRuntimeRef(runtime)}
}

func NewCreateGoalTool(runtime GoalRuntime) *CreateGoalTool {
	return &CreateGoalTool{runtime: newGoalToolRuntimeRef(runtime)}
}

func NewUpdateGoalTool(runtime GoalRuntime) *UpdateGoalTool {
	return &UpdateGoalTool{runtime: newGoalToolRuntimeRef(runtime)}
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

func (t *GetGoalTool) ToolContract() types.ToolContract {
	output := goalToolOutputSchema()
	return types.ToolContract{
		OutputSchema:       &output,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: goalToolMaxResultSizeChars,
	}
}

func (t *CreateGoalTool) ToolContract() types.ToolContract {
	output := goalToolOutputSchema()
	return types.ToolContract{OutputSchema: &output, Strict: true, MaxResultSizeChars: goalToolMaxResultSizeChars}
}

func (t *UpdateGoalTool) ToolContract() types.ToolContract {
	output := goalToolOutputSchema()
	return types.ToolContract{OutputSchema: &output, Strict: true, MaxResultSizeChars: goalToolMaxResultSizeChars}
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

func (t *GetGoalTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{AlwaysLoad: true, SearchHint: "inspect the current persisted session goal"}
}

func (t *CreateGoalTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{AlwaysLoad: true, SearchHint: "create a persisted session goal when explicitly requested"}
}

func (t *UpdateGoalTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{AlwaysLoad: true, SearchHint: "revise goal acceptance criteria or mark the goal complete or blocked"}
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

func (t *CreateGoalTool) ToAutoClassifierInput(input map[string]any) string {
	objective, _ := input["objective"].(string)
	criteria := goalToolStringSlice(input["acceptance_criteria"])
	return strings.TrimSpace(objective + " " + strings.Join(criteria, " "))
}

func (t *UpdateGoalTool) ToAutoClassifierInput(input map[string]any) string {
	status, _ := input["status"].(string)
	criteria := goalToolStringSlice(input["acceptance_criteria"])
	return strings.TrimSpace(status + " " + strings.Join(criteria, " "))
}

func goalToolStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
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
	if _, toolErr := parseStrictInputOrError[getGoalInput](input); toolErr != nil {
		return *toolErr, nil
	}
	runtime := t.runtime.get()
	if runtime == nil {
		return toolRuntimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	current, err := loadGoalForToolContext(ctx, runtime)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGoalLoadFailed, err), nil
	}
	data := GoalToolResult{Goal: cloneGoal(current)}
	return types.ToolResult{Content: getGoalModelText(data), Data: data}, nil
}

func (t *CreateGoalTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	in, toolErr := parseStrictInputOrError[createGoalInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if in.TokenBudget != nil && *in.TokenBudget <= 0 {
		return toolRuntimeError(i18n.KeyToolGoalTokenBudgetPositive), nil
	}
	if t == nil || t.runtime == nil || t.runtime.get() == nil {
		return toolRuntimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}

	t.runtime.mutateMu.Lock()
	defer t.runtime.mutateMu.Unlock()
	runtime := t.runtime.get()
	if runtime == nil {
		return toolRuntimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	tokenBudget := 0
	if in.TokenBudget != nil {
		tokenBudget = *in.TokenBudget
	}
	create := func(current *goal.Goal) (goal.Goal, error) {
		if current != nil && current.Status != goal.StatusAchieved && current.Status != goal.StatusCleared {
			return goal.Goal{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolGoalReplaceUnfinished, goalStatusText(current.Status)))
		}
		next, err := goal.CreateWithCriteria(in.Objective, in.AcceptanceCriteria, tokenBudget, time.Now().UTC())
		return next, localizeGoalToolError(err)
	}
	if next, atomic, err := updateGoalAtomicallyForToolContext(ctx, runtime, create); atomic {
		if err != nil {
			return ErrorResponse(err), nil
		}
		data := GoalToolResult{Goal: cloneGoal(&next)}
		return types.ToolResult{Content: createGoalModelText(data), Data: data}, nil
	}

	current, err := loadGoalForToolContext(ctx, runtime)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGoalLoadFailed, err), nil
	}
	next, err := create(current)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if err := saveGoalForToolContext(ctx, runtime, next); err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGoalSaveFailed, err), nil
	}
	data := GoalToolResult{Goal: cloneGoal(&next)}
	return types.ToolResult{Content: createGoalModelText(data), Data: data}, nil
}

func (t *UpdateGoalTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	in, toolErr := parseStrictInputOrError[updateGoalInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if in.Status != "complete" && in.Status != "blocked" && in.Status != "revise" {
		return toolRuntimeError(i18n.KeyToolGoalStatusRequired), nil
	}
	if in.Status == "revise" && (in.ExpectedRevision == nil || *in.ExpectedRevision <= 0) {
		return toolRuntimeError(i18n.KeyToolGoalRevisionRequired), nil
	}
	if in.Status != "revise" && (in.ExpectedRevision != nil || len(in.AcceptanceCriteria) > 0) {
		return toolRuntimeError(i18n.KeyToolGoalRevisionFieldsUnexpected), nil
	}
	if t == nil || t.runtime == nil || t.runtime.get() == nil {
		return toolRuntimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}

	t.runtime.mutateMu.Lock()
	defer t.runtime.mutateMu.Unlock()
	runtime := t.runtime.get()
	if runtime == nil {
		return toolRuntimeError(i18n.KeyToolGoalRuntimeUnavailable), nil
	}
	update := func(current *goal.Goal) (goal.Goal, error) {
		if current == nil {
			return goal.Goal{}, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolGoalNoActive))
		}

		now := time.Now().UTC()
		switch in.Status {
		case "complete":
			if current.Status != goal.StatusActive {
				return goal.Goal{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolGoalCannotAchieve, goalStatusText(current.Status)))
			}
			if !goal.AcceptanceCriteriaMet(*current) {
				return goal.Goal{}, fmt.Errorf("%s", toolRuntimeText(i18n.KeyRootGoalAcceptanceCriteriaUnmet))
			}
			next, err := goal.Achieve(*current, current.LastAcceptanceEvaluation.Summary, now)
			if err == nil {
				next = goal.SetEvaluatorReason(next, next.LastEvaluatorReason, goal.EvaluatorReasonModelDone, "", "", now)
			}
			return next, err
		case "blocked":
			if current.Status != goal.StatusActive {
				return goal.Goal{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolGoalCannotBlock, goalStatusText(current.Status)))
			}
			next, err := goal.Block(*current, toolRuntimeText(i18n.KeyToolGoalReasonBlocked), now)
			if err == nil {
				next = goal.SetEvaluatorReason(next, next.LastEvaluatorReason, goal.EvaluatorReasonModelBlocked, "", "", now)
			}
			return next, err
		case "revise":
			normalized := goal.Normalize(*current)
			if normalized.Revision != *in.ExpectedRevision {
				return goal.Goal{}, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolGoalRevisionStale))
			}
			next, err := goal.ReplaceAcceptanceCriteria(*current, in.AcceptanceCriteria, now)
			return next, localizeGoalToolError(err)
		default:
			return goal.Goal{}, fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolGoalStatusRequired))
		}
	}
	if next, atomic, err := updateGoalAtomicallyForToolContext(ctx, runtime, update); atomic {
		if err != nil {
			return ErrorResponse(err), nil
		}
		data := GoalToolResult{Goal: cloneGoal(&next)}
		return types.ToolResult{Content: updateGoalModelText(goalToolUpdateResultStatus(in.Status), data), Data: data}, nil
	}

	current, err := loadGoalForToolContext(ctx, runtime)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGoalLoadFailed, err), nil
	}
	next, err := update(current)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if err := saveGoalForToolContext(ctx, runtime, next); err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGoalSaveFailed, err), nil
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

func saveGoalForToolContext(ctx context.Context, runtime GoalRuntime, next goal.Goal) error {
	if contextual, ok := contextualGoalRuntime(ctx, runtime); ok {
		return contextual.SaveGoalForContext(ctx, next)
	}
	return runtime.SaveGoal(next)
}

func updateGoalAtomicallyForToolContext(ctx context.Context, runtime GoalRuntime, update goal.UpdateFunc) (goal.Goal, bool, error) {
	if hasGoalToolExecutionContext(ctx) {
		if contextual, ok := runtime.(goal.ContextUpdater); ok {
			next, err := contextual.UpdateGoalForContext(ctx, update)
			return next, true, err
		}
		// A legacy contextual runtime must keep routing through its context-aware
		// Load/Save pair instead of mutating the focused fallback session.
		if _, ok := runtime.(ContextGoalRuntime); ok {
			return goal.Goal{}, false, nil
		}
	}
	if updater, ok := runtime.(goal.Updater); ok {
		next, err := updater.UpdateGoal(update)
		return next, true, err
	}
	return goal.Goal{}, false, nil
}

func hasGoalToolExecutionContext(ctx context.Context) bool {
	exec, ok := loop.ToolExecutionContextFromContext(ctx)
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
		Content:   toolRuntimeFormat(i18n.KeyToolGoalInvalidTypedResult, name),
		IsError:   true,
	}
}

func getGoalModelText(result GoalToolResult) string {
	if result.Goal == nil {
		return toolRuntimeText(i18n.KeyToolGoalNone)
	}
	currentValue := goal.Normalize(*result.Goal)
	current := &currentValue
	lines := []string{
		toolRuntimeFormat(i18n.KeyToolGoalLabelGoal, current.Objective),
		toolRuntimeFormat(i18n.KeyToolGoalLabelStatus, goalStatusText(current.Status)),
	}
	lines = append(lines, goalAcceptanceCriteriaModelLines(current)...)
	if current.TokenBudget > 0 {
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolGoalTokenUsageBudget, current.Usage, current.TokenBudget))
	} else if current.Usage > 0 {
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolGoalTokenUsage, current.Usage))
	}
	if current.TurnCount > 0 {
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolGoalEvaluatedTurns, current.TurnCount))
	}
	if reason := strings.TrimSpace(current.LastEvaluatorReason); reason != "" {
		lines = append(lines, toolRuntimeFormat(i18n.KeyToolGoalLastEvaluation, localizedGoalReason(current)))
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
		return toolRuntimeText(i18n.KeyToolGoalCreationEmpty)
	}
	return toolRuntimeFormat(i18n.KeyToolGoalCreated, result.Goal.Objective) + "\n" + strings.Join(goalAcceptanceCriteriaModelLines(result.Goal), "\n")
}

func updateGoalModelText(status string, result GoalToolResult) string {
	if result.Goal == nil {
		return toolRuntimeText(i18n.KeyToolGoalUpdateEmpty)
	}
	return toolRuntimeFormat(i18n.KeyToolGoalMarked, goalUpdateStatusText(status), result.Goal.Objective) + "\n" +
		strings.Join(goalAcceptanceCriteriaModelLines(result.Goal), "\n")
}

func goalAcceptanceCriteriaModelLines(current *goal.Goal) []string {
	if current == nil {
		return nil
	}
	normalized := goal.Normalize(*current)
	lines := []string{toolRuntimeFormat(i18n.KeyToolGoalAcceptanceCriteriaHeader, normalized.Revision)}
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
		lines = append(lines, toolRuntimeFormat(key, criterion.ID, criterion.Text))
		if ok && strings.TrimSpace(result.Reason) != "" {
			lines = append(lines, toolRuntimeFormat(i18n.KeyToolGoalAcceptanceReason, result.Reason))
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
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolGoalObjectiveRequired))
	case errors.Is(err, goal.ErrObjectiveTooLong):
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolGoalObjectiveTooLong))
	case errors.Is(err, goal.ErrAcceptanceCriteriaRequired):
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyRootGoalAcceptanceCriteriaRequired))
	case errors.Is(err, goal.ErrAcceptanceCriteriaTooMany):
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyRootGoalAcceptanceCriteriaTooMany, goal.MaxAcceptanceCriteria))
	case errors.Is(err, goal.ErrAcceptanceCriterionRequired):
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyRootGoalAcceptanceCriterionRequired))
	case errors.Is(err, goal.ErrAcceptanceCriterionTooLong):
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyRootGoalAcceptanceCriterionTooLong, goal.MaxAcceptanceCriterionCharacters))
	case errors.Is(err, goal.ErrAcceptanceCriterionDuplicate):
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyRootGoalAcceptanceCriterionDuplicate))
	case errors.Is(err, goal.ErrAcceptanceCriteriaUnmet):
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyRootGoalAcceptanceCriteriaUnmet))
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
	return toolRuntimeText(key)
}

func goalUpdateStatusText(status string) string {
	switch status {
	case "complete":
		return toolRuntimeText(i18n.KeyToolGoalStatusComplete)
	case "blocked":
		return toolRuntimeText(i18n.KeyToolGoalStatusBlocked)
	case "updated":
		return toolRuntimeText(i18n.KeyToolGoalStatusUpdated)
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

func goalToolOutputSchema() types.JSONSchema {
	criterionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":   map[string]any{"type": "string"},
			"text": map[string]any{"type": "string"},
		},
		"required":             []string{"id", "text"},
		"additionalProperties": false,
	}
	criterionEvaluationSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"criterion_id": map[string]any{"type": "string"},
			"met":          map[string]any{"type": "boolean"},
			"reason":       map[string]any{"type": "string"},
		},
		"required":             []string{"criterion_id", "met", "reason"},
		"additionalProperties": false,
	}
	acceptanceEvaluationSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"revision":     map[string]any{"type": "integer"},
			"criteria":     map[string]any{"type": "array", "items": criterionEvaluationSchema},
			"summary":      map[string]any{"type": "string"},
			"evaluated_at": map[string]any{"type": "string", "format": "date-time"},
		},
		"required":             []string{"revision", "criteria", "summary", "evaluated_at"},
		"additionalProperties": false,
	}
	goalSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"objective":                  map[string]any{"type": "string"},
			"acceptance_criteria":        map[string]any{"type": "array", "items": criterionSchema},
			"revision":                   map[string]any{"type": "integer"},
			"next_criterion_id":          map[string]any{"type": "integer"},
			"last_acceptance_evaluation": acceptanceEvaluationSchema,
			"status":                     map[string]any{"type": "string", "enum": []string{"active", "paused", "achieved", "blocked", "cleared"}},
			"token_budget":               map[string]any{"type": "integer"},
			"usage":                      map[string]any{"type": "integer"},
			"turn_count":                 map[string]any{"type": "integer"},
			"last_evaluator_reason":      map[string]any{"type": "string"},
			"created_at":                 map[string]any{"type": "string", "format": "date-time"},
			"updated_at":                 map[string]any{"type": "string", "format": "date-time"},
			"achieved_at":                map[string]any{"type": "string", "format": "date-time"},
			"blocked_at":                 map[string]any{"type": "string", "format": "date-time"},
		},
		"required":             []string{"objective", "acceptance_criteria", "revision", "status", "created_at", "updated_at"},
		"additionalProperties": false,
	}
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"goal": map[string]any{"anyOf": []any{goalSchema, map[string]any{"type": "null"}}},
		},
		Required:             []string{"goal"},
		AdditionalProperties: false,
	}
}
