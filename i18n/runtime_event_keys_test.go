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
		KeyRuntimeStreamInterruptedPartial, KeyRuntimeStreamRetryFullHistory, KeyRuntimeStreamTransportFallback,
		KeyRuntimeResponseTruncated, KeyRuntimeResponseRetryMaxTokens, KeyRuntimeResponseRecovery,
		KeyRuntimeTokenBudgetContinuation, KeyRuntimeTokenBudgetDiminishing,
		KeyRuntimeGoalLoadMaxTokens, KeyRuntimeGoalLoadFailed, KeyRuntimeGoalEvaluatorUnavailable,
		KeyRuntimeGoalEvaluatorFailed, KeyRuntimeGoalLoadStopHook, KeyRuntimeGoalLoadToolExecution,
		KeyRuntimeGoalBudgetReached, KeyRuntimeGoalChangedStale,
		KeyRuntimeGoalChangedDuringSave, KeyRuntimeGoalUsageSaveFailed,
		KeyRuntimeAutoCompactFailed, KeyRuntimePostCompactCleanupFailed, KeyRuntimeCompactionCommitFailed,
		KeyRuntimeProviderRejectionRetry,
		KeyRuntimeReactiveCompact, KeyRuntimeMediaStrip,
		KeyRuntimeToolInputJSONFailed, KeyRuntimeToolInputJSONFlushFailed, KeyRuntimeToolSkippedMalformed,
		KeyRuntimeToolDisabled, KeyRuntimeToolPlanDenied, KeyRuntimeToolRuleDenied,
		KeyRuntimeToolPermissionRequired, KeyRuntimeResponsesStreamIncomplete,
		KeyRuntimePermissionActionExecute, KeyRuntimePermissionRuleToolContract,
		KeyRuntimePermissionScopeInvocation, KeyRuntimePermissionImpactDefault,
		KeyRuntimePermissionRuleRequired, KeyRuntimePlanActionExecute,
		KeyRuntimePlanImpactExecute, KeyRuntimePlanRiskExecute, KeyRuntimePlanRuleGate,
		KeyRuntimePlanScopeTransition, KeyRuntimePlanAllowedPrompts,
		KeyRuntimePermissionTargetInput,
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

func TestRuntimeEventEnglishContractContracts(t *testing.T) {
	if got := Format(LangEN, KeyRuntimePermissionDenied, "Read"); got != "Permission denied for tool: Read" {
		t.Fatalf("permission denial = %q", got)
	}
	if got := Format(LangEN, KeyRuntimeToolPlanDenied, "Write"); !strings.Contains(got, "plan mode") {
		t.Fatalf("plan denial = %q", got)
	}
	if got := Format(LangZH, KeyRuntimeStreamRetryFullHistory, 2, 5, "400ms"); got != "stream 失败；第 2/5 次重连将在 400ms 后使用完整消息历史进行" {
		t.Fatalf("stream retry = %q", got)
	}
}
