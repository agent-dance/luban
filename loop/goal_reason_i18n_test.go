package loop

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestGoalEvaluatorFailureReasonStoresSemanticKeyAndRawCause(t *testing.T) {
	cause := errors.New("raw provider detail")
	err := i18n.WrapError(i18n.KeyLoopGoalEvaluatorProviderCallFailed, cause)
	reason, key, detail := goalEvaluatorFailureReason(err)
	if reason == "" || key != string(i18n.KeyLoopGoalEvaluatorProviderCallFailed) || detail != cause.Error() {
		t.Fatalf("goalEvaluatorFailureReason() = %q, %q, %q", reason, key, detail)
	}
}

func TestGoalEvaluatorFailureReasonPreservesCustomEvaluatorText(t *testing.T) {
	err := errors.New("custom evaluator detail")
	_, key, detail := goalEvaluatorFailureReason(err)
	if key != "" || detail != err.Error() {
		t.Fatalf("custom failure metadata = %q, %q", key, detail)
	}
}
