package permissions

import "context"

type PromptKind string

const (
	PromptKindPermission PromptKind = "permission"
	PromptKindPlan       PromptKind = "plan"
	PromptKindAskUser    PromptKind = "ask_user"
)

type AskUserOption struct {
	Label       string
	Description string
	Preview     string
}

type AskUserQuestion struct {
	Question    string
	Header      string
	Options     []AskUserOption
	MultiSelect bool
}

type AskUserQuestionnaire struct {
	Questions []AskUserQuestion
}

type AskUserAnswer struct {
	Selection []string
	OtherText string
	Notes     string
}

type AskUserQuestionnaireResponse struct {
	Answers map[string]AskUserAnswer
}

type PromptOutcome string

const (
	PromptOutcomeApproved  PromptOutcome = "approved"
	PromptOutcomeRejected  PromptOutcome = "rejected"
	PromptOutcomeEscaped   PromptOutcome = "escaped"
	PromptOutcomeCancelled PromptOutcome = "cancelled"
	PromptOutcomeTimedOut  PromptOutcome = "timed_out"
	PromptOutcomeShutdown  PromptOutcome = "shutdown"
)

type PromptRequest struct {
	DecisionID         string
	SessionID          string
	ExecutionSessionID string
	TurnID             string
	ToolUseID          string
	ToolName           string
	Input              map[string]any
	ActorID            string
	ActorType          string
	WorkUnitID         string
	Kind               PromptKind
	Action             string
	Target             string
	Impact             string
	RiskLevel          int
	RiskReason         string
	RuleSource         string
	ApprovalScope      string
	Choices            []string
	Body               string
	ReviewDetails      []string
	PostMode           string
	Message            string
	Questionnaire      *AskUserQuestionnaire
}

type PromptResponse struct {
	DecisionID    string
	Decision      Decision
	Outcome       PromptOutcome
	Choice        string
	Questionnaire *AskUserQuestionnaireResponse
}

type StructuredPromptFunc func(context.Context, PromptRequest) PromptResponse

func responseForDecision(decision Decision) PromptResponse {
	response := PromptResponse{Decision: decision}
	switch decision {
	case DecisionAllow, DecisionAllowOnce:
		response.Outcome = PromptOutcomeApproved
	default:
		response.Outcome = PromptOutcomeRejected
	}
	return response
}
