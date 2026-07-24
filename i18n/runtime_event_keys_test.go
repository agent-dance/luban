package i18n

import (
	"strings"
	"testing"
)

func TestRuntimeEventKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyRuntimeWarningPublicSummary,
		KeyRuntimeHookEvidenceRetentionFailed, KeyRuntimeUserInterrupted,
		KeyRuntimePermissionCheckFailed, KeyRuntimePermissionDenied,
		KeyRuntimeToolInputNormalization, KeyRuntimeToolExecutionFailed,
		KeyRuntimeToolExecutionCancelled, KeyRuntimeToolBlockedByHook,
		KeyRuntimeToolInputValidation, KeyRuntimeStreamingToolDiscarded,
		KeyRuntimeParallelToolCancelled, KeyRuntimeParallelNamedToolCancelled,
		KeyRuntimePromptTooLong, KeyRuntimeModelFallback, KeyRuntimeTransientAPIError,
		KeyRuntimeStreamInterruptedPartial, KeyRuntimeStreamRetryFullHistory,
		KeyRuntimeResponseTruncated, KeyRuntimeResponseRetryMaxTokens, KeyRuntimeResponseRecovery,
		KeyRuntimeTokenBudgetContinuation, KeyRuntimeTokenBudgetDiminishing,
		KeyRuntimeGoalLoadMaxTokens, KeyRuntimeGoalLoadFailed, KeyRuntimeGoalEvaluatorUnavailable,
		KeyRuntimeGoalEvaluatorFailed, KeyRuntimeGoalLoadStopHook, KeyRuntimeGoalLoadToolExecution,
		KeyRuntimeGoalBudgetReached, KeyRuntimeGoalChangedStale,
		KeyRuntimeGoalChangedDuringSave, KeyRuntimeGoalUsageSaveFailed,
		KeyRuntimeAutoCompactFailed, KeyRuntimePostCompactCleanupFailed, KeyRuntimeCompactionCommitFailed,
		KeyRuntimeContextOverflowDrain, KeyRuntimeProviderRejectionRetry,
		KeyRuntimeReactiveCompact, KeyRuntimeMediaStrip,
		KeyRuntimeToolInputJSONFailed, KeyRuntimeToolInputJSONFlushFailed, KeyRuntimeToolSkippedMalformed,
		KeyRuntimeToolDisabled, KeyRuntimeToolPlanDenied, KeyRuntimeToolRuleDenied,
		KeyRuntimeToolPermissionRequired, KeyRuntimeResponsesStreamIncomplete,
		KeyRuntimePermissionActionExecute, KeyRuntimePermissionRuleToolContract,
		KeyRuntimePermissionScopeInvocation, KeyRuntimePermissionImpactDefault,
		KeyRuntimePermissionRuleRequired, KeyRuntimePlanActionExecute,
		KeyRuntimePlanImpactExecute, KeyRuntimePlanRiskExecute, KeyRuntimePlanRuleGate,
		KeyRuntimePlanScopeTransition, KeyRuntimePlanAllowedPrompts,
		KeyRuntimePlanAutoModeFallback, KeyRuntimePermissionTargetInput,
		KeyRuntimeMissingToolResult,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestRuntimeEventEnglishCompatibilityContracts(t *testing.T) {
	if got := Format(LangEN, KeyRuntimePermissionDenied, "Read"); got != "Permission denied for tool: Read" {
		t.Fatalf("permission denial = %q", got)
	}
	if got := Format(LangEN, KeyRuntimeToolPlanDenied, "Write"); !strings.Contains(got, "plan mode") {
		t.Fatalf("plan denial = %q", got)
	}
}
