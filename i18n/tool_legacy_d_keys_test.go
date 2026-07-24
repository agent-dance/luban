package i18n

import (
	"strings"
	"testing"
)

func TestToolLegacyDKeysCoverEveryRuntimeLanguage(t *testing.T) {
	keys := []Key{
		KeyToolLegacyDRegistryExecuteFailed,
		KeyToolLegacyDRegistryUnknownTool,
		KeyToolLegacyDRegistryToolDisabled,
		KeyToolLegacyDRegistryPermissionRequired,
		KeyToolLegacyDTestingPermissionSucceeded,
		KeyToolLegacyDTodoInputRequired,
		KeyToolLegacyDTodoLimitExceeded,
		KeyToolLegacyDTodoContentRequired,
		KeyToolLegacyDTodoActiveFormRequired,
		KeyToolLegacyDTodoStatusInvalid,
		KeyToolLegacyDTodoContentDuplicate,
		KeyToolLegacyDTodoSaveFailed,
		KeyToolLegacyDTodoRegressionWarning,
		KeyToolLegacyDTodoTypedResultInvalid,
		KeyToolLegacyDTodoModified,
		KeyToolLegacyDRequiredFieldQuoted,
		KeyToolLegacyDToolSearchNoMatches,
		KeyToolLegacyDToolSearchRequestedMissing,
		KeyToolLegacyDToolSearchLoadedWithMissing,
		KeyToolLegacyDToolSearchLoadedForQuery,
		KeyToolLegacyDToolSearchScore,
		KeyToolLegacyDToolSearchSnippet,
		KeyToolLegacyDMCPReconnect,
		KeyToolLegacyDMCPStatePending,
		KeyToolLegacyDMCPStateFailed,
		KeyToolLegacyDMCPStateNeedsAuth,
		KeyToolLegacyDMCPStateDisabled,
		KeyToolLegacyDMCPStateEntry,
		KeyToolLegacyDMCPPendingServers,
		KeyToolLegacyDMCPServerStates,
		KeyToolLegacyDSendToRequired,
		KeyToolLegacyDSendSchemeUnsupported,
		KeyToolLegacyDSendAddressRequired,
		KeyToolLegacyDSendBridgeConsent,
		KeyToolLegacyDSendBridgeUnavailable,
		KeyToolLegacyDSendBareRecipientRequired,
		KeyToolLegacyDSendDecodeFailed,
		KeyToolLegacyDSendSummaryRequired,
		KeyToolLegacyDSendStructuredBroadcast,
		KeyToolLegacyDSendStructuredCrossSession,
		KeyToolLegacyDSendShutdownTarget,
		KeyToolLegacyDSendShutdownRejectReason,
		KeyToolLegacyDSendAgentResumeFailed,
		KeyToolLegacyDSendQueued,
		KeyToolLegacyDSendAgentResumed,
		KeyToolLegacyDSendNoTeamContext,
		KeyToolLegacyDSendTeamMissing,
		KeyToolLegacyDSendMessageTypeInvalid,
		KeyToolLegacyDTeamAlreadyLeading,
		KeyToolLegacyDTeamAgentIDRequired,
		KeyToolLegacyDTeamAgentNoOutput,
		KeyToolLegacyDTeamAgentCompleted,
		KeyToolLegacyDTeamNothingToDelete,
		KeyToolLegacyDTeamActiveMembers,
		KeyToolLegacyDTeamDeleted,
		KeyToolLegacyDTeamTasksRequired,
		KeyToolLegacyDTeamNotFound,
		KeyToolLegacyDTeamTaskDescriptionRequired,
		KeyToolLegacyDTeamDispatchEmpty,
		KeyToolLegacyDTeamDispatchComplete,
		KeyToolLegacyDTeamDispatchTaskHeader,
		KeyToolLegacyDTeamDispatchError,
	}

	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolLegacyDEnglishCompatibilityAndRawArguments(t *testing.T) {
	if got, want := Format(LangEN, KeyToolLegacyDRegistryUnknownTool, "Read", []string{"Write"}), "Error: unknown tool 'Read'. Available tools: [Write]"; got != want {
		t.Fatalf("unknown tool English output = %q, want %q", got, want)
	}
	if got, want := Format(LangEN, KeyToolLegacyDToolSearchLoadedForQuery, 1, "select:Read", "Read", 4), `Loaded 1 tool(s) for "select:Read": Read. Deferred tool pool: 4 tool(s).`; got != want {
		t.Fatalf("ToolSearch English output = %q, want %q", got, want)
	}
	if got, want := Format(LangEN, KeyToolLegacyDTeamAlreadyLeading, "alpha"), `Already leading team "alpha". A leader can only manage one team at a time. Use TeamDelete to end the current team before creating a new one.`; got != want {
		t.Fatalf("team English output = %q, want %q", got, want)
	}

	got := Format(LangZH, KeyToolLegacyDSendAgentResumed, "agent-7", "stopped", "/tmp/agent-7.log")
	for _, raw := range []string{`"agent-7"`, "stopped", "/tmp/agent-7.log"} {
		if !strings.Contains(got, raw) {
			t.Fatalf("localized output %q lost raw argument %q", got, raw)
		}
	}
	if got := Format(LangZH, KeyToolLegacyDToolSearchLoadedForQuery, 2, "select:Read", "Read, Write", 9); strings.Contains(got, "%!") || !strings.Contains(got, "select:Read") {
		t.Fatalf("localized ToolSearch format is invalid: %q", got)
	}
}
