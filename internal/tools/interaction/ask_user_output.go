package interaction

import (
	"strings"

	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
)

type askUserQuestionOutput struct {
	Questions   []interactioncontract.QuestionSpec             `json:"questions"`
	Answers     map[string]string                              `json:"answers"`
	Annotations map[string]interactioncontract.AnnotationEntry `json:"annotations,omitempty"`
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

// isApprovalStyleQuestion returns true if the question reads as a yes/no
// approval gate. AskUserQuestion is not allowed to be used for that in plan
// mode — callers should redirect to ExitPlanMode instead.
func isApprovalStyleQuestion(q interactioncontract.QuestionSpec) bool {
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
