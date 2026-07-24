package tools

import "context"

// AskUserInteractionOutcome describes how a structured UI interaction ended.
// It is deliberately independent from permission approval: AskUserQuestion
// collects user input and must not mint an authorization decision.
type AskUserInteractionOutcome string

const (
	AskUserInteractionCompleted AskUserInteractionOutcome = "completed"
	AskUserInteractionCancelled AskUserInteractionOutcome = "cancelled"
	AskUserInteractionStale     AskUserInteractionOutcome = "stale"
	AskUserInteractionShutdown  AskUserInteractionOutcome = "shutdown"
)

// AskUserInteractionRequest is the surface-neutral input passed from the tool
// to the one interactive input owner selected by the runtime.
type AskUserInteractionRequest struct {
	RequestID  string
	SessionID  string
	TurnID     string
	ToolUseID  string
	ActorID    string
	ActorType  string
	WorkUnitID string
	Questions  []QuestionSpec
}

// AskUserInteractionResponse preserves the typed selection and annotation
// shapes used by AskUserQuestion's model-facing result.
type AskUserInteractionResponse struct {
	RequestID   string
	Outcome     AskUserInteractionOutcome
	Answers     map[string]AnswerSelection
	Annotations map[string]AnnotationEntry
}

// AskUserInteractionRequester is implemented by interactive surfaces. Full-
// screen TUI implementations must update their structured state only; linear
// renderers may use the input/output sink they exclusively own.
type AskUserInteractionRequester interface {
	AskUserQuestions(context.Context, AskUserInteractionRequest) (AskUserInteractionResponse, error)
}

type AskUserInteractionRequesterFunc func(context.Context, AskUserInteractionRequest) (AskUserInteractionResponse, error)

func (f AskUserInteractionRequesterFunc) AskUserQuestions(ctx context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
	return f(ctx, request)
}
