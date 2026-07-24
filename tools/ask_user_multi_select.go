package tools

import (
	"strings"
)

// AnswerSelection represents a structured answer to a single question.
// Selection holds the chosen option label(s); OtherText holds free-text from
// the auto-injected "Other" path. Both may be set if the user supplemented a
// chosen option with custom text.
type AnswerSelection struct {
	Selection []string `json:"selection,omitempty"`
	OtherText string   `json:"other,omitempty"`
}

// AnnotationEntry mirrors the per-question annotation payload returned by the
// TS UI: free-form notes plus the preview text of the chosen option (when
// applicable). Either may be empty.
type AnnotationEntry struct {
	Notes   string `json:"notes,omitempty"`
	Preview string `json:"preview,omitempty"`
}

// AskUserQuestionResult is the structured result returned to the model when
// the surface needs a typed payload alongside the legacy map[string]any view.
type AskUserQuestionResult struct {
	Answers     map[string]AnswerSelection `json:"answers"`
	Annotations map[string]AnnotationEntry `json:"annotations,omitempty"`
	Metadata    map[string]any             `json:"metadata,omitempty"`
}

// AskUserQuestionOutput mirrors the TS output schema returned by call():
// questions plus string answers, with optional per-question annotations.
type AskUserQuestionOutput struct {
	Questions   []QuestionSpec             `json:"questions"`
	Answers     map[string]string          `json:"answers"`
	Annotations map[string]AnnotationEntry `json:"annotations,omitempty"`
	Metadata    map[string]any             `json:"metadata,omitempty"`
}

// ParseOtherSentinel returns (otherText, true) if the input matches
// "Other:<text>". The match is case-insensitive on the prefix.
func ParseOtherSentinel(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) < len(askUserOtherSentinel) {
		return "", false
	}
	if !strings.EqualFold(trimmed[:len(askUserOtherSentinel)], askUserOtherSentinel) {
		return "", false
	}
	return strings.TrimSpace(trimmed[len(askUserOtherSentinel):]), true
}

// ParseMultiSelectReply takes a comma-separated list of option labels (or the
// "Other:<text>" sentinel) and returns the structured AnswerSelection.
// Unknown labels are returned as the third value for caller-side reporting.
func ParseMultiSelectReply(reply string, options []OptionSpec) (AnswerSelection, []string) {
	parts := strings.Split(reply, ",")
	var sel AnswerSelection
	seen := make(map[string]struct{}, len(parts))
	var unknown []string

	for _, raw := range parts {
		piece := strings.TrimSpace(raw)
		if piece == "" {
			continue
		}
		if other, ok := ParseOtherSentinel(piece); ok {
			if other != "" {
				sel.OtherText = other
			}
			continue
		}
		matched := false
		for _, o := range options {
			if strings.EqualFold(strings.TrimSpace(o.Label), piece) {
				if _, dup := seen[o.Label]; !dup {
					sel.Selection = append(sel.Selection, o.Label)
					seen[o.Label] = struct{}{}
				}
				matched = true
				break
			}
		}
		if !matched {
			unknown = append(unknown, piece)
		}
	}
	return sel, unknown
}

// approvalPhrasePatterns is the canonical set of phrases TS uses to detect
// "Should I proceed?" style questions that belong in ExitPlanMode instead.
var approvalPhrasePatterns = []string{
	"should i proceed",
	"should i continue",
	"may i proceed",
	"may i continue",
	"shall i continue",
	"shall i proceed",
	"ok to proceed",
	"okay to proceed",
	"is it ok to proceed",
	"do you approve",
	"approve this plan",
	"ready to proceed",
	"ready to continue",
}

// IsApprovalStyleQuestion returns true if the question reads as a yes/no
// approval gate. AskUserQuestion is not allowed to be used for that in plan
// mode — callers should redirect to ExitPlanMode instead.
func IsApprovalStyleQuestion(q QuestionSpec) bool {
	text := strings.ToLower(strings.TrimSpace(q.Question))
	if text == "" {
		return false
	}
	for _, phrase := range approvalPhrasePatterns {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
