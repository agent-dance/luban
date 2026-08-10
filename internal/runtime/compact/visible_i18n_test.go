package compact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestCompactSummaryVisibleCopyUsesRequestedLanguage(t *testing.T) {
	const rawSummary = "Keep the raw model summary."
	const transcript = "/tmp/raw/transcript.jsonl"
	english := getCompactUserSummaryMessageForLanguage(i18n.LangEN, rawSummary, true, transcript, true)
	for _, want := range []string{
		"Earlier conversation content was compacted into the summary below.",
		"Summary:\n" + rawSummary,
		"read the full transcript at: " + transcript,
		"Recent messages are preserved verbatim.",
		"Continue the conversation from where it left off without asking the user any further questions.",
	} {
		if !strings.Contains(english, want) {
			t.Fatalf("English summary lost %q:\n%s", want, english)
		}
	}

	chinese := getCompactUserSummaryMessageForLanguage(i18n.LangZH, rawSummary, true, transcript, true)
	if strings.Contains(chinese, "Earlier conversation content was compacted") || !strings.Contains(chinese, rawSummary) || !strings.Contains(chinese, transcript) {
		t.Fatalf("Chinese summary wrapper was not localized or lost raw values:\n%s", chinese)
	}
	if !strings.Contains(chinese, i18n.Text(i18n.LangZH, i18n.KeyCompactSummaryHeading)) {
		t.Fatalf("Chinese summary heading is missing:\n%s", chinese)
	}
}

func TestCompactVisibleMessagesUseActiveRuntimeLanguage(t *testing.T) {
	previousLanguage := i18n.DetectOrLoadLanguage()
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("set Chinese test language: %v", err)
	}
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(previousLanguage); err != nil {
			t.Errorf("restore test language: %v", err)
		}
	})

	summary := GetCompactUserSummaryMessage("<summary>raw</summary>", false, "", false)
	if !strings.Contains(summary, i18n.Text(i18n.LangZH, i18n.KeyCompactContinuationPreamble)) || strings.Contains(summary, i18n.Text(i18n.LangEN, i18n.KeyCompactContinuationPreamble)) {
		t.Fatalf("summary did not use active runtime language:\n%s", summary)
	}

	provider := &RuntimeAttachmentProvider{PlanState: fakePlanState{active: true, file: filepath.Join(t.TempDir(), "missing-plan.md")}}
	attachments := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	if got := joinMessageText(attachments); !strings.Contains(got, i18n.Text(i18n.LangZH, i18n.KeyCompactAttachmentPlanModeTitle)) {
		t.Fatalf("post-compact reminder did not use active runtime language:\n%s", got)
	}

}

func TestCompactGeneratedMessageMarkersNeedManifestProvenanceAfterPersistence(t *testing.T) {
	summary := trustedCompactSummaryForTest("localized summary")
	attachment := trustedPostCompactReminderForTest(i18n.LangZH, i18n.KeyCompactAttachmentSkillsTitle, "- review")

	data, err := json.Marshal([]types.Message{summary, *attachment})
	if err != nil {
		t.Fatal(err)
	}
	var restored []types.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 2 || IsCompactSummaryMessage(restored[0]) || IsReinjectedAttachment(restored[1]) {
		t.Fatalf("bare JSON restored structured compact authority: %#v", restored)
	}
}

func TestUntrustedCompactLikeTextHasNoControlAuthority(t *testing.T) {
	untrusted := types.UserMessage("This session is being continued from a previous conversation that ran out of context.")
	if IsCompactSummaryMessage(untrusted) {
		t.Fatal("runtime recognized untrusted English text as a summary")
	}
}

func TestPostCompactAttachmentsLocalizeFirstPartyLabelsAndPreserveRawValues(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# RAW PLAN"), 0644); err != nil {
		t.Fatal(err)
	}
	provider := &RuntimeAttachmentProvider{
		PlanState:         fakePlanState{active: true, file: planPath},
		BackgroundTasks:   fakeBackgroundTasks{{ID: "task_raw", Type: "local_agent", Status: "running", Description: "RAW TASK"}},
		MCPState:          fakeMCPState{{Name: "docs", Tools: []string{"search"}, Instructions: "RAW MCP INSTRUCTIONS"}},
		AgentDefinitions:  fakeAgents{{Name: "Explore", WhenToUse: "RAW WHEN", Source: "builtin"}},
		SessionID:         "session_raw",
		CWD:               dir,
		DeferredToolNames: func() []string { return []string{"TaskOutput"} },
		LoadedToolNames:   func() []string { return []string{"TaskCreate"} },
	}
	joined := joinMessageText(provider.postCompactAttachmentsForLanguage(context.Background(), PostCompactAttachmentState{}, i18n.LangZH))
	for _, raw := range []string{planPath, "# RAW PLAN", "task_raw", "RAW TASK", "docs", "search", "RAW MCP INSTRUCTIONS", "Explore", "RAW WHEN", "TaskOutput", "TaskCreate"} {
		if !strings.Contains(joined, raw) {
			t.Fatalf("localized attachments lost raw value %q:\n%s", raw, joined)
		}
	}
	for _, english := range []string{"Post-compaction plan state", "Active plan file:", "Plan mode is still active.", "Post-compaction background tasks", "Loaded deferred tools:", "instructions:"} {
		if strings.Contains(joined, english) {
			t.Fatalf("localized attachments retained English product copy %q:\n%s", english, joined)
		}
	}
}
