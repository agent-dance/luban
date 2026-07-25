// Package interaction defines surface-neutral contracts for structured user
// interaction. It is shared by model-facing tools and presentation surfaces;
// neither side owns these DTOs.
package interaction

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

// OptionSpec describes a single selectable option.
type OptionSpec struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// QuestionSpec describes a single question with its options.
type QuestionSpec struct {
	Question    string       `json:"question"`
	Header      string       `json:"header"`
	Options     []OptionSpec `json:"options"`
	MultiSelect bool         `json:"multiSelect"`
}

// AnswerSelection represents a structured answer to a single question.
type AnswerSelection struct {
	Selection []string `json:"selection,omitempty"`
	OtherText string   `json:"other,omitempty"`
}

// AnnotationEntry carries trusted notes and the selected option preview.
type AnnotationEntry struct {
	Notes   string `json:"notes,omitempty"`
	Preview string `json:"preview,omitempty"`
}

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

// AskUserInteractionRequester is implemented by interactive surfaces.
type AskUserInteractionRequester interface {
	AskUserQuestions(context.Context, AskUserInteractionRequest) (AskUserInteractionResponse, error)
}
