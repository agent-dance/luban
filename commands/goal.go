package commands

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/goal"
)

type goalCmd struct{}

var errGoalNotSet = errors.New("no goal is set")

type localizedGoalError struct {
	message string
	cause   error
}

func (e localizedGoalError) Error() string { return e.message }
func (e localizedGoalError) Unwrap() error { return e.cause }

func (*goalCmd) Name() string        { return "goal" }
func (*goalCmd) Aliases() []string   { return nil }
func (*goalCmd) Description() string { return builtinCommandDescription("goal") }

func (*goalCmd) Execute(ctx *Context, args string) error {
	if ctx == nil || ctx.GoalRuntime == nil {
		lang := i18n.DetectOrLoadLanguage()
		if ctx != nil {
			lang = ctx.Language
		}
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCommandGoalRuntimeMissing))
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return showGoal(ctx)
	}
	verb, rest := splitGoalArgs(args)
	switch verb {
	case "status", "view":
		if rest != "" {
			return goalUsageError(ctx)
		}
		return showGoal(ctx)
	case "set":
		return createGoal(ctx, rest)
	case "edit":
		return transitionGoal(ctx, "edit", func(current goal.Goal) (goal.Goal, error) {
			return goal.Edit(current, rest, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalUpdated))
	case "criteria":
		return editGoalCriteria(ctx, rest)
	case "pause":
		if rest != "" {
			return goalUsageError(ctx)
		}
		return transitionGoal(ctx, "pause", func(current goal.Goal) (goal.Goal, error) {
			return goal.Pause(current, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalPaused))
	case "resume":
		if rest != "" {
			return goalUsageError(ctx)
		}
		return transitionGoal(ctx, "resume", func(current goal.Goal) (goal.Goal, error) {
			return goal.Resume(current, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalActive))
	case "clear":
		if rest != "" {
			return goalUsageError(ctx)
		}
		return clearGoal(ctx)
	default:
		return createGoal(ctx, args)
	}
}

func createGoal(ctx *Context, input string) error {
	objective, criteria, err := parseGoalCreation(input)
	if err != nil {
		return localizeGoalDomainError(ctx.Language, err, "", "")
	}
	next, err := goal.CreateWithCriteria(objective, criteria, 0, time.Now())
	if err != nil {
		return localizeGoalDomainError(ctx.Language, err, "", "")
	}
	if updater, ok := ctx.GoalRuntime.(goal.Updater); ok {
		created := next
		next, err = updater.UpdateGoal(func(*goal.Goal) (goal.Goal, error) {
			return created, nil
		})
		if err != nil {
			return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
		}
		emitGoalTransition(ctx, i18n.Text(ctx.Language, i18n.KeyCommandGoalSet), next)
		return nil
	}
	if err := ctx.GoalRuntime.SaveGoal(next); err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
	}
	emitGoalTransition(ctx, i18n.Text(ctx.Language, i18n.KeyCommandGoalSet), next)
	return nil
}

func transitionGoal(ctx *Context, action string, transition func(goal.Goal) (goal.Goal, error), receipt string) error {
	if updater, ok := ctx.GoalRuntime.(goal.Updater); ok {
		from := goal.Status("")
		next, err := updater.UpdateGoal(func(current *goal.Goal) (goal.Goal, error) {
			if current == nil {
				return goal.Goal{}, errGoalNotSet
			}
			from = current.Status
			return transition(*current)
		})
		if errors.Is(err, errGoalNotSet) {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyCommandGoalNoActive))
		}
		if err != nil {
			if isGoalDomainError(err) {
				return localizeGoalDomainError(ctx.Language, err, action, from)
			}
			return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
		}
		emitGoalTransition(ctx, receipt, next)
		return nil
	}

	current, err := ctx.GoalRuntime.LoadGoal()
	if err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalLoadError), err)
	}
	if current == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyCommandGoalNoActive))
	}
	next, err := transition(*current)
	if err != nil {
		return localizeGoalDomainError(ctx.Language, err, action, current.Status)
	}
	if err := ctx.GoalRuntime.SaveGoal(next); err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
	}
	emitGoalTransition(ctx, receipt, next)
	return nil
}

func emitGoalTransition(ctx *Context, receipt string, next goal.Goal) {
	emitGoalEvent(ctx, i18n.Format(ctx.Language, i18n.KeyCommandGoalTransitionReport,
		receipt, formatGoalReport(ctx.Language, next)))
	if next.Status == goal.StatusActive && ctx.OnGoalActivated != nil {
		ctx.OnGoalActivated(next.Objective)
	}
}

func editGoalCriteria(ctx *Context, args string) error {
	verb, rest := splitGoalArgs(strings.TrimSpace(args))
	switch verb {
	case "add":
		return transitionGoal(ctx, "edit", func(current goal.Goal) (goal.Goal, error) {
			return goal.AddAcceptanceCriterion(current, rest, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalCriterionAdded))
	case "edit":
		id, text := splitGoalArgs(rest)
		if id == "" || text == "" {
			return goalUsageError(ctx)
		}
		return transitionGoal(ctx, "edit", func(current goal.Goal) (goal.Goal, error) {
			return goal.EditAcceptanceCriterion(current, id, text, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalCriterionUpdated))
	case "remove":
		id, extra := splitGoalArgs(rest)
		if id == "" || extra != "" {
			return goalUsageError(ctx)
		}
		return transitionGoal(ctx, "edit", func(current goal.Goal) (goal.Goal, error) {
			return goal.RemoveAcceptanceCriterion(current, id, time.Now())
		}, i18n.Text(ctx.Language, i18n.KeyCommandGoalCriterionRemoved))
	default:
		return goalUsageError(ctx)
	}
}

func clearGoal(ctx *Context) error {
	if updater, ok := ctx.GoalRuntime.(goal.Updater); ok {
		from := goal.Status("")
		next, err := updater.UpdateGoal(func(current *goal.Goal) (goal.Goal, error) {
			if current == nil {
				return goal.Goal{}, errGoalNotSet
			}
			from = current.Status
			return goal.Clear(*current, time.Now())
		})
		if errors.Is(err, errGoalNotSet) {
			emitGoalEvent(ctx, i18n.Text(ctx.Language, i18n.KeyCommandGoalNoActive))
			return nil
		}
		if err != nil {
			if errors.Is(err, goal.ErrInvalidTransition) {
				return localizeGoalDomainError(ctx.Language, err, "clear", from)
			}
			return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
		}
		emitGoalEvent(ctx, i18n.Format(ctx.Language, i18n.KeyCommandGoalCleared, next.Objective))
		return nil
	}

	current, err := ctx.GoalRuntime.LoadGoal()
	if err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalLoadError), err)
	}
	if current == nil {
		emitGoalEvent(ctx, i18n.Text(ctx.Language, i18n.KeyCommandGoalNoActive))
		return nil
	}
	next, err := goal.Clear(*current, time.Now())
	if err != nil {
		return localizeGoalDomainError(ctx.Language, err, "clear", current.Status)
	}
	if err := ctx.GoalRuntime.SaveGoal(next); err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalSaveError), err)
	}
	emitGoalEvent(ctx, i18n.Format(ctx.Language, i18n.KeyCommandGoalCleared, next.Objective))
	return nil
}

func showGoal(ctx *Context) error {
	current, err := ctx.GoalRuntime.LoadGoal()
	if err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyCommandGoalLoadError), err)
	}
	if current == nil {
		emitGoalEvent(ctx, i18n.Text(ctx.Language, i18n.KeyCommandGoalNoActiveCreate))
		return nil
	}

	emitGoalEvent(ctx, formatGoalReport(ctx.Language, *current))
	return nil
}

func formatGoalReport(lang i18n.Language, current goal.Goal) string {
	current = goal.Normalize(current)
	var output strings.Builder
	output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalReport,
		i18n.RootGoalStatusLabel(lang, string(current.Status)), current.Objective))
	output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalCriteriaHeader, current.Revision))
	evaluated := make(map[string]goal.AcceptanceCriterionEvaluation)
	if current.LastAcceptanceEvaluation != nil && current.LastAcceptanceEvaluation.Revision == current.Revision {
		for _, result := range current.LastAcceptanceEvaluation.Criteria {
			evaluated[strings.ToUpper(result.CriterionID)] = result
		}
	}
	for _, criterion := range current.AcceptanceCriteria {
		result, ok := evaluated[strings.ToUpper(criterion.ID)]
		key := i18n.KeyCommandGoalCriterionPending
		if ok && result.Met {
			key = i18n.KeyCommandGoalCriterionMet
		} else if ok {
			key = i18n.KeyCommandGoalCriterionUnmet
		}
		output.WriteString(i18n.Format(lang, key, criterion.ID, criterion.Text))
		if ok && strings.TrimSpace(result.Reason) != "" {
			output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalCriterionReason, result.Reason))
		}
	}
	if current.TokenBudget > 0 {
		output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalBudget, current.TokenBudget))
	}
	if current.Usage > 0 || current.TokenBudget > 0 {
		output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalUsage, current.Usage))
	}
	output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalTurns, current.TurnCount))
	if current.LastEvaluatorReason != "" {
		output.WriteString(i18n.Format(lang, i18n.KeyCommandGoalLastEvaluation,
			i18n.RootGoalEvaluatorReasonStateLabel(lang, current.LastEvaluatorReason,
				string(current.LastEvaluatorReasonKind), current.LastEvaluatorReasonKey, current.LastEvaluatorReasonDetail)))
	}
	return output.String()
}

func splitGoalArgs(args string) (verb, rest string) {
	if index := strings.IndexFunc(args, unicode.IsSpace); index >= 0 {
		return strings.ToLower(args[:index]), strings.TrimSpace(args[index:])
	}
	return strings.ToLower(args), ""
}

func parseGoalCreation(input string) (string, []string, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "", nil, goal.ErrObjectiveRequired
	}
	firstCriterion := -1
	for index, field := range fields {
		if field == "--accept" {
			firstCriterion = index
			break
		}
	}
	if firstCriterion <= 0 {
		objective := strings.Join(fields, " ")
		return objective, []string{objective}, nil
	}
	objective := strings.Join(fields[:firstCriterion], " ")
	var criteria []string
	start := firstCriterion + 1
	for index := start; index <= len(fields); index++ {
		if index < len(fields) && fields[index] != "--accept" {
			continue
		}
		if index == start {
			return objective, nil, goal.ErrAcceptanceCriterionRequired
		}
		criteria = append(criteria, strings.Join(fields[start:index], " "))
		start = index + 1
	}
	return objective, criteria, nil
}

func emitGoalEvent(ctx *Context, message string) {
	if ctx.OnEvent != nil {
		ctx.OnEvent(message)
	}
}

func goalUsageError(ctx *Context) error {
	return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyCommandGoalUsageError))
}

func localizeGoalDomainError(lang i18n.Language, err error, action string, from goal.Status) error {
	if err == nil {
		return nil
	}
	message := ""
	switch {
	case errors.Is(err, goal.ErrObjectiveRequired):
		message = i18n.Text(lang, i18n.KeyRootGoalObjectiveRequired)
	case errors.Is(err, goal.ErrObjectiveTooLong):
		message = i18n.Format(lang, i18n.KeyRootGoalObjectiveTooLong, goal.MaxObjectiveCharacters)
	case errors.Is(err, goal.ErrAcceptanceCriteriaRequired):
		message = i18n.Text(lang, i18n.KeyRootGoalAcceptanceCriteriaRequired)
	case errors.Is(err, goal.ErrAcceptanceCriteriaTooMany):
		message = i18n.Format(lang, i18n.KeyRootGoalAcceptanceCriteriaTooMany, goal.MaxAcceptanceCriteria)
	case errors.Is(err, goal.ErrAcceptanceCriterionRequired):
		message = i18n.Text(lang, i18n.KeyRootGoalAcceptanceCriterionRequired)
	case errors.Is(err, goal.ErrAcceptanceCriterionTooLong):
		message = i18n.Format(lang, i18n.KeyRootGoalAcceptanceCriterionTooLong, goal.MaxAcceptanceCriterionCharacters)
	case errors.Is(err, goal.ErrAcceptanceCriterionDuplicate):
		message = i18n.Text(lang, i18n.KeyRootGoalAcceptanceCriterionDuplicate)
	case errors.Is(err, goal.ErrAcceptanceCriterionNotFound):
		message = i18n.Text(lang, i18n.KeyRootGoalAcceptanceCriterionNotFound)
	case errors.Is(err, goal.ErrCannotRemoveLastCriterion):
		message = i18n.Text(lang, i18n.KeyRootGoalCannotRemoveLastCriterion)
	case errors.Is(err, goal.ErrAcceptanceCriteriaUnmet):
		message = i18n.Text(lang, i18n.KeyRootGoalAcceptanceCriteriaUnmet)
	case errors.Is(err, goal.ErrInvalidTransition):
		message = i18n.Format(lang, i18n.KeyRootGoalTransitionInvalid,
			i18n.RootGoalActionLabel(lang, action), i18n.RootGoalStatusLabel(lang, string(from)))
	default:
		return err
	}
	return localizedGoalError{message: message, cause: err}
}

func isGoalDomainError(err error) bool {
	return errors.Is(err, goal.ErrInvalidTransition) ||
		errors.Is(err, goal.ErrObjectiveRequired) ||
		errors.Is(err, goal.ErrObjectiveTooLong) ||
		errors.Is(err, goal.ErrAcceptanceCriteriaRequired) ||
		errors.Is(err, goal.ErrAcceptanceCriteriaTooMany) ||
		errors.Is(err, goal.ErrAcceptanceCriterionRequired) ||
		errors.Is(err, goal.ErrAcceptanceCriterionTooLong) ||
		errors.Is(err, goal.ErrAcceptanceCriterionDuplicate) ||
		errors.Is(err, goal.ErrAcceptanceCriterionNotFound) ||
		errors.Is(err, goal.ErrCannotRemoveLastCriterion) ||
		errors.Is(err, goal.ErrAcceptanceCriteriaUnmet)
}
