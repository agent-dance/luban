package presentation

const (
	ToolPresentationStateStarted   = "started"
	ToolPresentationStateRunning   = "running"
	ToolPresentationStateSucceeded = "succeeded"
	ToolPresentationStateFailed    = "failed"
	ToolPresentationStatePartial   = "partial"
	ToolPresentationStateDenied    = "denied"
	ToolPresentationStateCancelled = "cancelled"
	ToolPresentationStateTimedOut  = "timed_out"
	ToolPresentationStateUnknown   = "unknown"

	ToolPresentationLevelHidden     = "hidden_member"
	ToolPresentationLevelFolded     = "folded"
	ToolPresentationLevelStructured = "structured"
	ToolPresentationLevelEvidence   = "evidence"
)

// ToolPresentation is the renderer-neutral, already-redacted projection of a
// semantic tool transition. State is an execution fact supplied by the caller;
// renderers must not infer it from Result or DetailLines.
//
// Raw tool input and output deliberately do not belong in this DTO. HasMore
// preserves the path to retained evidence without flooding append-only output.
type ToolPresentation struct {
	ToolName          string
	ToolUseID         string
	WorkUnitID        string
	Actor             string
	Action            string
	Object            string
	State             string
	Result            string
	NextAction        string
	DetailLines       []string
	PresentationLevel string
	ReasonCodes       []string
	HasMore           bool
	Redacted          bool
}

// SemanticToolRenderer is the optional renderer capability for already-redacted
// semantic tool transitions.
type SemanticToolRenderer interface {
	RenderToolPresentation(ToolPresentation)
}
