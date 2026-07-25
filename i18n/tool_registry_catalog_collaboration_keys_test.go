package i18n

import (
	"strings"
	"testing"
)

func TestToolRegistryCatalogCollaborationKeysCoverEveryRuntimeLanguage(t *testing.T) {
	keys := []Key{
		KeyRegistryToolExecuteFailed,
		KeyRegistryToolUnknown,
		KeyRuntimeToolDisabled,
		KeyRuntimeToolPermissionRequired,
		KeyToolRuntimeRequiredFieldMissing,
		KeyToolSearchCatalogNoMatches,
		KeyToolSearchCatalogRequestedToolsMissing,
		KeyToolSearchCatalogLoadedWithMissing,
		KeyToolSearchCatalogLoadedForQuery,
		KeyToolSearchCatalogMatchScore,
		KeyToolSearchCatalogMatchSnippet,
		KeyMCPVisibilityReconnectAttempt,
		KeyMCPStatePending,
		KeyMCPStateFailed,
		KeyMCPStateNeedsAuth,
		KeyMCPStateDisabled,
		KeyMCPVisibilityStateEntry,
		KeyMCPVisibilityPendingServers,
		KeyMCPVisibilityServerStates,
		KeyToolSendMessageToRequired,
		KeyToolSendMessageAddressSchemeUnsupported,
		KeyToolSendMessageAddressTargetRequired,
		KeyToolSendMessageBareRecipientRequired,
		KeyToolSendMessageSummaryRequired,
		KeyToolSendMessageStructuredBroadcastUnsupported,
		KeyToolSendMessageStructuredCrossSessionUnsupported,
		KeyToolSendMessageStructuredShutdownResponseTarget,
		KeyToolSendMessageStructuredShutdownRejectReasonRequired,
		KeyToolSendMessageAgentResumeFailed,
		KeyToolSendMessageDeliveryQueued,
		KeyToolSendMessageAgentResumed,
		KeyToolSendMessageTeamContextRequired,
		KeyToolSendMessageTeamMissing,
		KeyToolSendMessageInputMessageInvalid,
		KeyToolTeamCreateAlreadyLeading,
		KeyToolTeamDeleteNothingToDelete,
		KeyToolTeamDeleteActiveMembersBlocked,
		KeyToolTeamDeleteCompleted,
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

func TestToolRegistryCatalogCollaborationEnglishContractsAndRawArguments(t *testing.T) {
	if got, want := Format(LangEN, KeyRegistryToolUnknown, "Read", []string{"Write"}), "Error: unknown tool 'Read'. Available tools: [Write]"; got != want {
		t.Fatalf("unknown tool English output = %q, want %q", got, want)
	}
	if got, want := Format(LangEN, KeyToolSearchCatalogLoadedForQuery, 1, "select:Read", "Read", 4), `Loaded 1 tool(s) for "select:Read": Read. Deferred tool pool: 4 tool(s).`; got != want {
		t.Fatalf("ToolSearch English output = %q, want %q", got, want)
	}
	if got, want := Format(LangEN, KeyToolTeamCreateAlreadyLeading, "alpha"), `Already leading team "alpha". A leader can only manage one team at a time. Use TeamDelete to end the current team before creating a new one.`; got != want {
		t.Fatalf("team English output = %q, want %q", got, want)
	}

	got := Format(LangZH, KeyToolSendMessageAgentResumed, "agent-7", "stopped", "/tmp/agent-7.log")
	for _, raw := range []string{`"agent-7"`, "stopped", "/tmp/agent-7.log"} {
		if !strings.Contains(got, raw) {
			t.Fatalf("localized output %q lost raw argument %q", got, raw)
		}
	}
	if got := Format(LangZH, KeyToolSearchCatalogLoadedForQuery, 2, "select:Read", "Read, Write", 9); strings.Contains(got, "%!") || !strings.Contains(got, "select:Read") {
		t.Fatalf("localized ToolSearch format is invalid: %q", got)
	}
}
