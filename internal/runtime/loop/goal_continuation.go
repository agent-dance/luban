package loop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

type goalContinuationDecision struct {
	Handled  bool
	Continue bool
}

func (q *QueryLoop) evaluateGoalContinuation(
	ctx context.Context,
	state *QueryState,
	snapshot QueryConfigSnapshot,
	transcript []types.Message,
	budgetTracker *BudgetTracker,
	turnOutputTokens int,
	turnCount int,
	onEvent func(stream.Event),
) goalContinuationDecision {
	if snapshot.GoalRuntime == nil {
		return goalContinuationDecision{}
	}

	current, err := snapshot.GoalRuntime.LoadGoal()
	if err != nil {
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalLoadFailed, nil, err)
		return goalContinuationDecision{Handled: true}
	}
	if current == nil || current.Status != goal.StatusActive {
		return goalContinuationDecision{}
	}
	if snapshot.GoalEvaluator == nil {
		reason := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRootGoalReasonEvaluatorUnavailable)
		if err := saveGoalEvaluatorReason(snapshot.GoalRuntime, *current, reason, goal.EvaluatorReasonUnavailable, "", "", time.Now()); err != nil {
			emitGoalProgressSaveWarning(onEvent, turnCount, i18n.KeyRootGoalProgressSaveAfterEvaluatorFailure, nil, err)
		}
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalEvaluatorUnavailable, nil, nil)
		return goalContinuationDecision{Handled: true}
	}

	result, err := snapshot.GoalEvaluator.Evaluate(ctx, newGoalEvaluationRequest(*current, transcript))
	if result.Usage != nil {
		onEvent(newGoalEvaluationEvent(result.Usage, turnCount, snapshot.Model))
	}
	if err == nil {
		result.Reason, err = validateGoalEvaluatorReason(result.Reason)
	}
	var criterionResults []goal.AcceptanceCriterionEvaluation
	allCriteriaMet := false
	if err == nil {
		criterionResults, allCriteriaMet, err = goalCriterionResults(*current, result)
	}
	if err != nil {
		reason, semanticKey, detail := goalEvaluatorFailureReason(err)
		if saveErr := saveGoalEvaluatorReason(snapshot.GoalRuntime, *current, reason, goal.EvaluatorReasonFailed, semanticKey, detail, time.Now()); saveErr != nil {
			emitGoalProgressSaveWarning(onEvent, turnCount, i18n.KeyRootGoalProgressSaveAfterEvaluatorFailure, nil, saveErr)
		}
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalEvaluatorFailed, nil, err)
		return goalContinuationDecision{Handled: true}
	}

	now := time.Now()
	reason := result.Reason
	evaluationRevision := goal.Normalize(*current).Revision
	next, err := updateGoalFromSnapshot(snapshot.GoalRuntime, *current, func(latest *goal.Goal) (goal.Goal, error) {
		updated, recordErr := goal.RecordAcceptanceEvaluation(*latest, evaluationRevision, criterionResults, reason, now)
		if recordErr != nil {
			if errors.Is(recordErr, goal.ErrAcceptanceEvaluationStale) {
				return goal.Goal{}, i18n.NewError(i18n.KeyRootGoalAcceptanceEvaluationStale)
			}
			if errors.Is(recordErr, goal.ErrAcceptanceEvaluationInvalid) {
				return goal.Goal{}, i18n.NewError(i18n.KeyRootGoalAcceptanceEvaluationInvalid)
			}
			return goal.Goal{}, recordErr
		}
		if allCriteriaMet {
			return goal.Achieve(updated, reason, now)
		}
		return updated, nil
	})
	if err != nil {
		emitGoalProgressSaveWarning(onEvent, turnCount, i18n.KeyRootGoalProgressSaveFailed, nil, err)
		return goalContinuationDecision{Handled: true}
	}
	if allCriteriaMet {
		return goalContinuationDecision{Handled: true}
	}

	if next.TokenBudget > 0 && next.Usage >= next.TokenBudget {
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalBudgetReached, []any{next.Usage, next.TokenBudget}, nil)
		return goalContinuationDecision{Handled: true}
	}
	if snapshot.TokenBudget > 0 && snapshot.AgentID == "" {
		decision := CheckTokenBudget(budgetTracker, snapshot.AgentID, snapshot.TokenBudget, turnOutputTokens)
		if !decision.Continue {
			if decision.CompletionEvent != nil && decision.CompletionEvent.DiminishingReturns {
				onEvent(NewSystemWarningEvent(i18n.KeyRuntimeTokenBudgetDiminishing, []any{decision.CompletionEvent.Percent}, nil, nil, turnCount))
			}
			return goalContinuationDecision{Handled: true}
		}
	}

	state.Messages = append(state.Messages, q.goalContinuationMessage(reason))
	state.MaxOutputTokensRecoveryCount = 0
	state.MaxOutputTokensOverride = 0
	state.Transition = QueryTransitionGoalContinuation
	return goalContinuationDecision{Handled: true, Continue: true}
}

func goalCriterionResults(current goal.Goal, result GoalEvaluationResult) ([]goal.AcceptanceCriterionEvaluation, bool, error) {
	criteria := current.Criteria()
	if len(result.Criteria) == 0 {
		return nil, false, i18n.NewError(i18n.KeyLoopGoalEvaluatorMissingCriteria)
	}
	if len(result.Criteria) != len(criteria) {
		return nil, false, i18n.NewError(i18n.KeyLoopGoalEvaluatorCriteriaIncomplete)
	}
	byID := make(map[string]GoalCriterionEvaluationResult, len(result.Criteria))
	for _, evaluated := range result.Criteria {
		id := strings.ToUpper(strings.TrimSpace(evaluated.ID))
		reason, err := validateGoalEvaluatorReason(evaluated.Reason)
		if id == "" || err != nil {
			return nil, false, i18n.NewError(i18n.KeyLoopGoalEvaluatorCriterionInvalid)
		}
		if _, exists := byID[id]; exists {
			return nil, false, i18n.NewError(i18n.KeyLoopGoalEvaluatorCriterionInvalid)
		}
		evaluated.ID = id
		evaluated.Reason = reason
		byID[id] = evaluated
	}
	ordered := make([]goal.AcceptanceCriterionEvaluation, 0, len(criteria))
	allMet := true
	for _, criterion := range criteria {
		evaluated, ok := byID[strings.ToUpper(criterion.ID)]
		if !ok {
			return nil, false, i18n.NewError(i18n.KeyLoopGoalEvaluatorCriteriaIncomplete)
		}
		ordered = append(ordered, goal.AcceptanceCriterionEvaluation{
			CriterionID: criterion.ID, Met: evaluated.Met, Reason: evaluated.Reason,
		})
		allMet = allMet && evaluated.Met
	}
	return ordered, allMet, nil
}

func saveGoalAssistantTurnUsage(runtime GoalRuntime, turnUsage *types.Usage, now time.Time) (bool, error) {
	if runtime == nil {
		return false, nil
	}
	current, err := runtime.LoadGoal()
	if err != nil {
		return true, err
	}
	if current == nil || current.Status != goal.StatusActive {
		return false, nil
	}
	_, err = updateGoalFromSnapshot(runtime, *current, func(latest *goal.Goal) (goal.Goal, error) {
		updated := *latest
		updated.TurnCount++
		if turnUsage != nil && turnUsage.OutputTokens > 0 {
			updated.Usage += turnUsage.OutputTokens
		}
		updated.UpdatedAt = now
		return updated, nil
	})
	return true, err
}

func goalTokenBudgetReached(runtime GoalRuntime) (bool, goal.Goal, error) {
	if runtime == nil {
		return false, goal.Goal{}, nil
	}
	current, err := runtime.LoadGoal()
	if err != nil {
		return false, goal.Goal{}, err
	}
	if current == nil || current.Status != goal.StatusActive || current.TokenBudget <= 0 {
		return false, goal.Goal{}, nil
	}
	return current.Usage >= current.TokenBudget, *current, nil
}

func saveGoalEvaluatorReason(runtime GoalRuntime, current goal.Goal, reason string, kind goal.EvaluatorReasonKind, key, detail string, now time.Time) error {
	_, err := updateGoalFromSnapshot(runtime, current, func(latest *goal.Goal) (goal.Goal, error) {
		return goal.SetEvaluatorReason(*latest, reason, kind, key, detail, now), nil
	})
	return err
}

func updateGoalFromSnapshot(runtime GoalRuntime, expected goal.Goal, update goal.UpdateFunc) (goal.Goal, error) {
	guarded := func(current *goal.Goal) (goal.Goal, error) {
		if current == nil || !reflect.DeepEqual(*current, expected) {
			return goal.Goal{}, goal.ErrStaleUpdate
		}
		return update(current)
	}
	return runtime.UpdateGoal(guarded)
}

func goalEvaluatorFailureReason(err error) (reason, semanticKey, detail string) {
	reason = boundedGoalEvaluatorFailureReason(err)
	info, ok := i18n.DescribeSemanticError(err)
	if !ok {
		if err != nil {
			detail = boundedGoalEvaluatorDetail(err.Error())
		}
		return reason, "", detail
	}
	semanticKey = string(info.Key)
	if info.IncludeCause && info.Cause != nil {
		detail = boundedGoalEvaluatorDetail(info.Cause.Error())
	}
	return reason, semanticKey, detail
}

func boundedGoalEvaluatorFailureReason(err error) string {
	lang := i18n.DetectOrLoadLanguage()
	reason := i18n.Format(lang, i18n.KeyRootGoalReasonEvaluatorFailed, i18n.Text(lang, i18n.KeyPresentationReasonUnavailable))
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		reason = i18n.Format(lang, i18n.KeyRootGoalReasonEvaluatorFailed, strings.TrimSpace(err.Error()))
	}
	runes := []rune(reason)
	if len(runes) <= goalEvaluatorMaxReasonRunes {
		return reason
	}
	return string(runes[:goalEvaluatorMaxReasonRunes-3]) + "..."
}

func boundedGoalEvaluatorDetail(detail string) string {
	runes := []rune(strings.TrimSpace(detail))
	if len(runes) <= goalEvaluatorMaxReasonRunes {
		return string(runes)
	}
	return string(runes[:goalEvaluatorMaxReasonRunes-3]) + "..."
}

func (q *QueryLoop) goalContinuationMessage(reason string) types.Message {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleGoalReasonDefault)
	}
	message := types.UserMessage(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleGoalContinuation, quoteGoalEvaluatorReason(reason)))
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindGoalContinuation
	return q.sealRuntimeControlMessage(message)
}

func emitGoalContinuationWarning(onEvent func(stream.Event), turnCount int, key i18n.Key, args []any, err error) {
	onEvent(NewSystemWarningEvent(key, args, err, nil, turnCount))
}

func emitGoalProgressSaveWarning(onEvent func(stream.Event), turnCount int, key i18n.Key, args []any, err error) {
	if errors.Is(err, goal.ErrStaleUpdate) {
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalChangedStale, nil, nil)
		return
	}
	emitGoalContinuationWarning(onEvent, turnCount, key, args, err)
}

func emitGoalTurnSaveWarning(onEvent func(stream.Event), turnCount int, err error) {
	if errors.Is(err, goal.ErrStaleUpdate) {
		emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalChangedDuringSave, nil, nil)
		return
	}
	emitGoalContinuationWarning(onEvent, turnCount, i18n.KeyRuntimeGoalUsageSaveFailed, nil, err)
}
