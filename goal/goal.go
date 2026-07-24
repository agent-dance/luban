package goal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxObjectiveCharacters             = 4000
	MaxAcceptanceCriteria              = 20
	MaxAcceptanceCriterionCharacters   = 4000
	MaxAcceptanceEvaluationReasonRunes = 512
)

var (
	ErrObjectiveRequired            = errors.New("goal objective is required")
	ErrObjectiveTooLong             = errors.New("goal objective must not exceed 4000 characters")
	ErrAcceptanceCriteriaRequired   = errors.New("goal acceptance criteria are required")
	ErrAcceptanceCriteriaTooMany    = errors.New("goal has too many acceptance criteria")
	ErrAcceptanceCriterionRequired  = errors.New("goal acceptance criterion is required")
	ErrAcceptanceCriterionTooLong   = errors.New("goal acceptance criterion is too long")
	ErrAcceptanceCriterionDuplicate = errors.New("goal acceptance criterion is duplicated")
	ErrAcceptanceCriterionNotFound  = errors.New("goal acceptance criterion was not found")
	ErrCannotRemoveLastCriterion    = errors.New("goal must keep at least one acceptance criterion")
	ErrAcceptanceEvaluationInvalid  = errors.New("goal acceptance evaluation is invalid")
	ErrAcceptanceEvaluationStale    = errors.New("goal acceptance evaluation is stale")
	ErrAcceptanceCriteriaUnmet      = errors.New("goal acceptance criteria are not all met")
	ErrInvalidTransition            = errors.New("invalid transition")
)

type Status string

// EvaluatorReasonKind identifies first-party persisted reasons without
// freezing their rendered language. Empty means evaluator-authored/raw text.
type EvaluatorReasonKind string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusAchieved Status = "achieved"
	StatusBlocked  Status = "blocked"
	StatusCleared  Status = "cleared"
)

const (
	EvaluatorReasonUnavailable  EvaluatorReasonKind = "evaluator_unavailable"
	EvaluatorReasonFailed       EvaluatorReasonKind = "evaluator_failed"
	EvaluatorReasonModelDone    EvaluatorReasonKind = "model_marked_complete"
	EvaluatorReasonModelBlocked EvaluatorReasonKind = "model_marked_blocked"
)

// AcceptanceCriterion is one independently verifiable condition. Text is
// user-provided data and must be rendered without translation.
type AcceptanceCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// AcceptanceCriterionEvaluation records whether the transcript evidence met
// one condition in a specific Goal revision.
type AcceptanceCriterionEvaluation struct {
	CriterionID string `json:"criterion_id"`
	Met         bool   `json:"met"`
	Reason      string `json:"reason"`
}

// AcceptanceEvaluation is valid only for the exact Goal revision it names.
type AcceptanceEvaluation struct {
	Revision    int                             `json:"revision"`
	Criteria    []AcceptanceCriterionEvaluation `json:"criteria"`
	Summary     string                          `json:"summary"`
	EvaluatedAt time.Time                       `json:"evaluated_at"`
}

// Goal is the persisted state for one session goal.
type Goal struct {
	Objective                 string                `json:"objective"`
	AcceptanceCriteria        []AcceptanceCriterion `json:"acceptance_criteria,omitempty"`
	Revision                  int                   `json:"revision,omitempty"`
	NextCriterionID           int                   `json:"next_criterion_id,omitempty"`
	LastAcceptanceEvaluation  *AcceptanceEvaluation `json:"last_acceptance_evaluation,omitempty"`
	Status                    Status                `json:"status"`
	TokenBudget               int                   `json:"token_budget,omitempty"`
	Usage                     int                   `json:"usage,omitempty"`
	TurnCount                 int                   `json:"turn_count,omitempty"`
	LastEvaluatorReason       string                `json:"last_evaluator_reason,omitempty"`
	LastEvaluatorReasonKind   EvaluatorReasonKind   `json:"last_evaluator_reason_kind,omitempty"`
	LastEvaluatorReasonKey    string                `json:"last_evaluator_reason_key,omitempty"`
	LastEvaluatorReasonDetail string                `json:"last_evaluator_reason_detail,omitempty"`
	CreatedAt                 time.Time             `json:"created_at"`
	UpdatedAt                 time.Time             `json:"updated_at"`
	AchievedAt                *time.Time            `json:"achieved_at,omitempty"`
	BlockedAt                 *time.Time            `json:"blocked_at,omitempty"`
}

// Normalize upgrades legacy objective-only goals in memory. Callers that
// persist the returned value make the migration durable without a separate
// metadata format migration.
func Normalize(current Goal) Goal {
	current.AcceptanceCriteria = append([]AcceptanceCriterion(nil), current.AcceptanceCriteria...)
	if current.Revision <= 0 {
		current.Revision = 1
	}
	if len(current.AcceptanceCriteria) == 0 && strings.TrimSpace(current.Objective) != "" {
		current.AcceptanceCriteria = []AcceptanceCriterion{{ID: criterionID(1), Text: strings.TrimSpace(current.Objective)}}
	}
	maxID := 0
	for index := range current.AcceptanceCriteria {
		current.AcceptanceCriteria[index].ID = strings.TrimSpace(current.AcceptanceCriteria[index].ID)
		current.AcceptanceCriteria[index].Text = strings.TrimSpace(current.AcceptanceCriteria[index].Text)
		if current.AcceptanceCriteria[index].ID == "" {
			current.AcceptanceCriteria[index].ID = criterionID(index + 1)
		}
		maxID = max(maxID, criterionNumber(current.AcceptanceCriteria[index].ID))
	}
	if current.NextCriterionID <= maxID {
		current.NextCriterionID = maxID + 1
	}
	if current.NextCriterionID <= 0 {
		current.NextCriterionID = len(current.AcceptanceCriteria) + 1
	}
	if current.LastAcceptanceEvaluation != nil {
		evaluation := *current.LastAcceptanceEvaluation
		evaluation.Criteria = append([]AcceptanceCriterionEvaluation(nil), evaluation.Criteria...)
		current.LastAcceptanceEvaluation = &evaluation
	}
	return current
}

// Criteria returns a detached, legacy-normalized condition list.
func (current Goal) Criteria() []AcceptanceCriterion {
	current = Normalize(current)
	return append([]AcceptanceCriterion(nil), current.AcceptanceCriteria...)
}

// SetEvaluatorReason stores compatibility text together with stable semantic
// metadata. Callers pass an empty kind for evaluator-authored raw reasons.
func SetEvaluatorReason(current Goal, reason string, kind EvaluatorReasonKind, key, detail string, now time.Time) Goal {
	current.LastEvaluatorReason = strings.TrimSpace(reason)
	current.LastEvaluatorReasonKind = kind
	current.LastEvaluatorReasonKey = strings.TrimSpace(key)
	current.LastEvaluatorReasonDetail = strings.TrimSpace(detail)
	current.UpdatedAt = now
	return current
}

// Create preserves the original package API for internal callers by treating
// the objective as one explicit acceptance condition. User-facing creation
// paths use CreateWithCriteria so the condition is supplied separately.
func Create(objective string, tokenBudget int, now time.Time) (Goal, error) {
	return CreateWithCriteria(objective, []string{objective}, tokenBudget, now)
}

func CreateWithCriteria(objective string, acceptanceCriteria []string, tokenBudget int, now time.Time) (Goal, error) {
	objective, err := validateObjective(objective)
	if err != nil {
		return Goal{}, err
	}
	criteria, err := newAcceptanceCriteria(acceptanceCriteria)
	if err != nil {
		return Goal{}, err
	}
	return Goal{
		Objective:          objective,
		AcceptanceCriteria: criteria,
		Revision:           1,
		NextCriterionID:    len(criteria) + 1,
		Status:             StatusActive,
		TokenBudget:        tokenBudget,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func Edit(current Goal, objective string, now time.Time) (Goal, error) {
	if !canEdit(current.Status) {
		return current, transitionError("edit", current.Status)
	}
	objective, err := validateObjective(objective)
	if err != nil {
		return current, err
	}
	current = Normalize(current)
	current.Objective = objective
	return beginRevision(current, now), nil
}

func AddAcceptanceCriterion(current Goal, text string, now time.Time) (Goal, error) {
	if !canEdit(current.Status) {
		return current, transitionError("edit", current.Status)
	}
	current = Normalize(current)
	if len(current.AcceptanceCriteria) >= MaxAcceptanceCriteria {
		return current, ErrAcceptanceCriteriaTooMany
	}
	text, err := validateAcceptanceCriterion(text)
	if err != nil {
		return current, err
	}
	if criterionTextExists(current.AcceptanceCriteria, text, "") {
		return current, ErrAcceptanceCriterionDuplicate
	}
	current.AcceptanceCriteria = append(current.AcceptanceCriteria, AcceptanceCriterion{
		ID: criterionID(current.NextCriterionID), Text: text,
	})
	current.NextCriterionID++
	return beginRevision(current, now), nil
}

func EditAcceptanceCriterion(current Goal, id, text string, now time.Time) (Goal, error) {
	if !canEdit(current.Status) {
		return current, transitionError("edit", current.Status)
	}
	current = Normalize(current)
	text, err := validateAcceptanceCriterion(text)
	if err != nil {
		return current, err
	}
	index := criterionIndex(current.AcceptanceCriteria, id)
	if index < 0 {
		return current, ErrAcceptanceCriterionNotFound
	}
	if criterionTextExists(current.AcceptanceCriteria, text, current.AcceptanceCriteria[index].ID) {
		return current, ErrAcceptanceCriterionDuplicate
	}
	current.AcceptanceCriteria[index].Text = text
	return beginRevision(current, now), nil
}

// ReplaceAcceptanceCriteria applies an Agent-authored revision while
// preserving stable IDs by position where possible. The revision invalidates
// every result from the previous acceptance contract.
func ReplaceAcceptanceCriteria(current Goal, values []string, now time.Time) (Goal, error) {
	if !canEdit(current.Status) {
		return current, transitionError("edit", current.Status)
	}
	validated, err := newAcceptanceCriteria(values)
	if err != nil {
		return current, err
	}
	current = Normalize(current)
	for index := range validated {
		if index < len(current.AcceptanceCriteria) {
			validated[index].ID = current.AcceptanceCriteria[index].ID
			continue
		}
		validated[index].ID = criterionID(current.NextCriterionID)
		current.NextCriterionID++
	}
	current.AcceptanceCriteria = validated
	return beginRevision(current, now), nil
}

func RemoveAcceptanceCriterion(current Goal, id string, now time.Time) (Goal, error) {
	if !canEdit(current.Status) {
		return current, transitionError("edit", current.Status)
	}
	current = Normalize(current)
	index := criterionIndex(current.AcceptanceCriteria, id)
	if index < 0 {
		return current, ErrAcceptanceCriterionNotFound
	}
	if len(current.AcceptanceCriteria) <= 1 {
		return current, ErrCannotRemoveLastCriterion
	}
	current.AcceptanceCriteria = append(current.AcceptanceCriteria[:index:index], current.AcceptanceCriteria[index+1:]...)
	return beginRevision(current, now), nil
}

// RecordAcceptanceEvaluation persists a complete result for every condition in
// the current revision. Partial, duplicate, or stale results are rejected.
func RecordAcceptanceEvaluation(current Goal, revision int, results []AcceptanceCriterionEvaluation, summary string, now time.Time) (Goal, error) {
	if current.Status != StatusActive {
		return current, transitionError("evaluate", current.Status)
	}
	current = Normalize(current)
	if revision != current.Revision {
		return current, ErrAcceptanceEvaluationStale
	}
	if len(results) != len(current.AcceptanceCriteria) {
		return current, ErrAcceptanceEvaluationInvalid
	}
	byID := make(map[string]AcceptanceCriterionEvaluation, len(results))
	for _, result := range results {
		id := strings.TrimSpace(result.CriterionID)
		reason := strings.TrimSpace(result.Reason)
		if id == "" || reason == "" || utf8.RuneCountInString(reason) > MaxAcceptanceEvaluationReasonRunes {
			return current, ErrAcceptanceEvaluationInvalid
		}
		if _, exists := byID[strings.ToUpper(id)]; exists {
			return current, ErrAcceptanceEvaluationInvalid
		}
		result.CriterionID = id
		result.Reason = reason
		byID[strings.ToUpper(id)] = result
	}
	ordered := make([]AcceptanceCriterionEvaluation, 0, len(current.AcceptanceCriteria))
	for _, criterion := range current.AcceptanceCriteria {
		result, ok := byID[strings.ToUpper(criterion.ID)]
		if !ok {
			return current, ErrAcceptanceEvaluationInvalid
		}
		result.CriterionID = criterion.ID
		ordered = append(ordered, result)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" || utf8.RuneCountInString(summary) > MaxAcceptanceEvaluationReasonRunes {
		return current, ErrAcceptanceEvaluationInvalid
	}
	current.LastAcceptanceEvaluation = &AcceptanceEvaluation{
		Revision: revision, Criteria: ordered, Summary: summary, EvaluatedAt: now,
	}
	current.LastEvaluatorReason = summary
	current.LastEvaluatorReasonKind = ""
	current.LastEvaluatorReasonKey = ""
	current.LastEvaluatorReasonDetail = ""
	current.UpdatedAt = now
	return current, nil
}

// AcceptanceCriteriaMet reports whether the current revision has a complete
// evaluation in which every condition is met.
func AcceptanceCriteriaMet(current Goal) bool {
	current = Normalize(current)
	evaluation := current.LastAcceptanceEvaluation
	if evaluation == nil || evaluation.Revision != current.Revision || len(evaluation.Criteria) != len(current.AcceptanceCriteria) {
		return false
	}
	byID := make(map[string]bool, len(evaluation.Criteria))
	for _, result := range evaluation.Criteria {
		key := strings.ToUpper(strings.TrimSpace(result.CriterionID))
		if key == "" || !result.Met {
			return false
		}
		if _, exists := byID[key]; exists {
			return false
		}
		byID[key] = true
	}
	for _, criterion := range current.AcceptanceCriteria {
		if !byID[strings.ToUpper(criterion.ID)] {
			return false
		}
	}
	return len(current.AcceptanceCriteria) > 0
}

func Pause(current Goal, now time.Time) (Goal, error) {
	if current.Status != StatusActive {
		return current, transitionError("pause", current.Status)
	}
	current.Status = StatusPaused
	current.UpdatedAt = now
	return current, nil
}

func Resume(current Goal, now time.Time) (Goal, error) {
	if current.Status != StatusPaused && current.Status != StatusBlocked {
		return current, transitionError("resume", current.Status)
	}
	current.Status = StatusActive
	current.UpdatedAt = now
	current.BlockedAt = nil
	return current, nil
}

func Achieve(current Goal, reason string, now time.Time) (Goal, error) {
	if current.Status != StatusActive {
		return current, transitionError("achieve", current.Status)
	}
	if !AcceptanceCriteriaMet(current) {
		return current, ErrAcceptanceCriteriaUnmet
	}
	current.Status = StatusAchieved
	current.LastEvaluatorReason = strings.TrimSpace(reason)
	current.LastEvaluatorReasonKind = ""
	current.LastEvaluatorReasonKey = ""
	current.LastEvaluatorReasonDetail = ""
	current.UpdatedAt = now
	current.AchievedAt = timePointer(now)
	return current, nil
}

func Block(current Goal, reason string, now time.Time) (Goal, error) {
	if current.Status != StatusActive {
		return current, transitionError("block", current.Status)
	}
	current.Status = StatusBlocked
	current.LastEvaluatorReason = strings.TrimSpace(reason)
	current.LastEvaluatorReasonKind = ""
	current.LastEvaluatorReasonKey = ""
	current.LastEvaluatorReasonDetail = ""
	current.UpdatedAt = now
	current.BlockedAt = timePointer(now)
	return current, nil
}

func Clear(current Goal, now time.Time) (Goal, error) {
	if current.Status == StatusCleared {
		return current, transitionError("clear", current.Status)
	}
	current.Status = StatusCleared
	current.UpdatedAt = now
	return current, nil
}

func validateObjective(objective string) (string, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return "", ErrObjectiveRequired
	}
	if utf8.RuneCountInString(objective) > MaxObjectiveCharacters {
		return "", ErrObjectiveTooLong
	}
	return objective, nil
}

func newAcceptanceCriteria(values []string) ([]AcceptanceCriterion, error) {
	if len(values) == 0 {
		return nil, ErrAcceptanceCriteriaRequired
	}
	if len(values) > MaxAcceptanceCriteria {
		return nil, ErrAcceptanceCriteriaTooMany
	}
	criteria := make([]AcceptanceCriterion, 0, len(values))
	for index, value := range values {
		text, err := validateAcceptanceCriterion(value)
		if err != nil {
			return nil, err
		}
		if criterionTextExists(criteria, text, "") {
			return nil, ErrAcceptanceCriterionDuplicate
		}
		criteria = append(criteria, AcceptanceCriterion{ID: criterionID(index + 1), Text: text})
	}
	return criteria, nil
}

func validateAcceptanceCriterion(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrAcceptanceCriterionRequired
	}
	if utf8.RuneCountInString(text) > MaxAcceptanceCriterionCharacters {
		return "", ErrAcceptanceCriterionTooLong
	}
	return text, nil
}

func beginRevision(current Goal, now time.Time) Goal {
	current.Revision++
	current.LastAcceptanceEvaluation = nil
	current.LastEvaluatorReason = ""
	current.LastEvaluatorReasonKind = ""
	current.LastEvaluatorReasonKey = ""
	current.LastEvaluatorReasonDetail = ""
	current.UpdatedAt = now
	return current
}

func canEdit(status Status) bool {
	return status == StatusActive || status == StatusPaused || status == StatusBlocked
}

func criterionID(number int) string {
	return "AC-" + strconv.Itoa(number)
}

func criterionNumber(id string) int {
	id = strings.ToUpper(strings.TrimSpace(id))
	if !strings.HasPrefix(id, "AC-") {
		return 0
	}
	number, _ := strconv.Atoi(strings.TrimPrefix(id, "AC-"))
	return number
}

func criterionIndex(criteria []AcceptanceCriterion, id string) int {
	id = strings.TrimSpace(id)
	for index, criterion := range criteria {
		if strings.EqualFold(criterion.ID, id) {
			return index
		}
	}
	return -1
}

func criterionTextExists(criteria []AcceptanceCriterion, text, exceptID string) bool {
	for _, criterion := range criteria {
		if strings.EqualFold(criterion.ID, exceptID) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(criterion.Text), strings.TrimSpace(text)) {
			return true
		}
	}
	return false
}

func transitionError(action string, from Status) error {
	return fmt.Errorf("goal: cannot %s from %s status: %w", action, from, ErrInvalidTransition)
}

func timePointer(value time.Time) *time.Time {
	return &value
}
