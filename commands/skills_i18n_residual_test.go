package commands

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestNilSkillInvokerUsesSemanticError(t *testing.T) {
	_, err := SkillInvokerFunc(nil).InvokeSkill(context.Background(), SkillInvocationRequest{})
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyCommandSkillInvokerNotConfigured || info.Cause != nil {
		t.Fatalf("nil skill invoker error = %v, semantic info = %+v, %v", err, info, ok)
	}
	if got := err.Error(); got != "skill invoker is not configured" {
		t.Fatalf("English compatibility error = %q", got)
	}
}
