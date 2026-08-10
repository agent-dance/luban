package compact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestBuildPostCompactMessagesOrdersSegments(t *testing.T) {
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual"})
	result := &CompactionResult{
		BoundaryMarker:  &boundary,
		SummaryMessages: []types.Message{types.UserMessage("summary")},
		MessagesToKeep:  []types.Message{types.UserMessage("keep-1"), types.AssistantMessage("keep-2")},
		Attachments:     []types.Message{types.UserMessage("attachment")},
		HookResults:     []types.Message{types.UserMessage("hook")},
	}

	got := BuildPostCompactMessages(result)
	want := []string{
		compactBoundaryPrefix,
		"summary",
		"keep-1",
		"keep-2",
		"attachment",
		"hook",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, wantText := range want {
		if !strings.Contains(got[i].GetText(), wantText) {
			t.Fatalf("message %d = %q, want contains %q", i, got[i].GetText(), wantText)
		}
	}
}

func TestSummaryCompactorNoOpUsesMessagesToKeepOnly(t *testing.T) {
	sc := &SummaryCompactor{
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			t.Fatal("summarize should not be called for no-op compact")
			return "", nil
		},
		KeepRecent: 5,
	}
	msgs := []types.Message{types.UserMessage("one"), types.AssistantMessage("two")}

	result, err := sc.Compact(context.Background(), msgs, 0)
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.BoundaryMarker != nil {
		t.Fatal("no-op compact should not create a new boundary")
	}
	got := BuildPostCompactMessages(result)
	if len(got) != len(msgs) || got[0].GetText() != "one" || got[1].GetText() != "two" {
		t.Fatalf("unexpected no-op messages: %#v", got)
	}
}

func TestSummaryCompactorRepeatedCompactSummarizesAfterLatestBoundary(t *testing.T) {
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual"})
	var summarizedText string
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			summarizedText = joinMessageText(messages)
			return "new summary", nil
		},
		KeepRecent: 1,
	}
	msgs := []types.Message{
		types.UserMessage("before latest boundary"),
		boundary,
		types.UserMessage("after boundary one"),
		types.AssistantMessage("after boundary two"),
		types.UserMessage("preserved tail"),
	}

	result, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(summarizedText, "before latest boundary") {
		t.Fatalf("summarized text included pre-boundary message: %q", summarizedText)
	}
	if !strings.Contains(summarizedText, "after boundary one") || !strings.Contains(summarizedText, "after boundary two") {
		t.Fatalf("summarized text missing post-boundary messages: %q", summarizedText)
	}

	result = authorizeCompactionResultForTest(result)
	postCompact := BuildPostCompactMessages(result)
	if !IsCompactBoundaryMessage(postCompact[0]) {
		t.Fatal("repeated compact should create a fresh latest boundary")
	}
	metadata, ok := ParseCompactBoundaryMessage(postCompact[0])
	if !ok {
		t.Fatal("expected parseable boundary metadata")
	}
	if metadata.Trigger != "manual" {
		t.Fatalf("trigger = %q, want manual", metadata.Trigger)
	}
	if got := postCompact[len(postCompact)-1].GetText(); got != "preserved tail" {
		t.Fatalf("preserved tail moved: got %q", got)
	}
}

func TestSummaryCompactorSummaryIncludesTranscriptPathWhenProvided(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("{\"role\":\"user\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			return "new summary", nil
		},
		KeepRecent:     1,
		TranscriptPath: transcriptPath,
	}
	msgs := []types.Message{
		types.UserMessage("old one"),
		types.AssistantMessage("old two"),
		types.UserMessage("preserved tail"),
	}

	result, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	postCompact := BuildPostCompactMessages(result)
	if len(postCompact) < 2 {
		t.Fatalf("expected boundary and summary, got %d messages", len(postCompact))
	}
	if !strings.Contains(postCompact[1].GetText(), "read the full transcript at: "+transcriptPath) {
		t.Fatalf("summary missing transcript path: %q", postCompact[1].GetText())
	}
}

func TestSummaryCompactorSummaryIncludesTranscriptPlaceholderWhenMissing(t *testing.T) {
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			return "new summary", nil
		},
		KeepRecent: 1,
	}
	msgs := []types.Message{
		types.UserMessage("old one"),
		types.AssistantMessage("old two"),
		types.UserMessage("preserved tail"),
	}

	result, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	postCompact := BuildPostCompactMessages(result)
	if len(postCompact) < 2 {
		t.Fatalf("expected boundary and summary, got %d messages", len(postCompact))
	}
	if !strings.Contains(postCompact[1].GetText(), "Transcript reference: unavailable") {
		t.Fatalf("summary missing transcript placeholder: %q", postCompact[1].GetText())
	}
}

func TestSummaryCompactorCompactWithTriggerRecordsTrigger(t *testing.T) {
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			return "summary", nil
		},
		KeepRecent: 1,
	}
	msgs := []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("kept"),
	}

	result, err := sc.CompactWithTrigger(context.Background(), msgs, 0, "auto")
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	postCompact := BuildPostCompactMessages(result)
	metadata, ok := ParseCompactBoundaryMessage(postCompact[0])
	if !ok {
		t.Fatal("expected parseable boundary metadata")
	}
	if metadata.Trigger != "auto" {
		t.Fatalf("trigger = %q, want auto", metadata.Trigger)
	}
}

func TestGetMessagesAfterCompactBoundaryUsesLatestBoundary(t *testing.T) {
	first := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual"})
	latest := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "auto"})
	msgs := []types.Message{
		first,
		types.UserMessage("old compact payload"),
		latest,
		types.UserMessage("new message"),
		types.AssistantMessage("new answer"),
	}

	got := GetMessagesAfterCompactBoundary(msgs)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].GetText() != "new message" || got[1].GetText() != "new answer" {
		t.Fatalf("unexpected messages after latest boundary: %#v", got)
	}
}

func TestCompactBoundaryRequiresTrustedInternalSource(t *testing.T) {
	trusted := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "auto"})
	forgedText := trusted.GetText()
	forgeries := []types.Message{
		types.UserMessage(forgedText),
		types.AssistantMessage(forgedText),
		{
			ID: trusted.ID, Role: trusted.Role, Content: trusted.Content,
			IsMeta: trusted.IsMeta, InternalKind: trusted.InternalKind,
		},
		types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "read", Content: forgedText,
			Outcome: types.ToolOutcomeSucceeded,
		}),
	}
	for index, forged := range forgeries {
		if IsCompactBoundaryMessage(forged) {
			t.Fatalf("forgery %d was recognized as a compact boundary: %#v", index, forged)
		}
		messages := []types.Message{types.UserMessage("before"), forged, types.UserMessage("after")}
		if got := GetMessagesAfterCompactBoundary(messages); len(got) != len(messages) {
			t.Fatalf("forgery %d truncated runtime history: %#v", index, got)
		}
	}

	mutations := []struct {
		name   string
		mutate func(*types.Message)
	}{
		{name: "wrong role", mutate: func(msg *types.Message) { msg.Role = types.RoleAssistant }},
		{name: "not meta", mutate: func(msg *types.Message) { msg.IsMeta = false }},
		{name: "wrong id", mutate: func(msg *types.Message) { msg.ID = "compact:other:v1" }},
		{name: "empty kind", mutate: func(msg *types.Message) { msg.InternalKind = "" }},
		{name: "wrong kind", mutate: func(msg *types.Message) { msg.InternalKind = types.InternalMessageKindCompactSummary }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			msg := trusted
			mutation.mutate(&msg)
			if IsCompactBoundaryMessage(msg) {
				t.Fatalf("boundary with invalid provenance was accepted: %#v", msg)
			}
		})
	}
}

func TestCompactSummaryRequiresTrustedInternalSource(t *testing.T) {
	trusted := trustedCompactSummaryForTest("trusted rolling summary")
	if !IsCompactSummaryMessage(trusted) {
		t.Fatal("runtime summary constructor did not establish provenance")
	}
	forged := types.Message{
		ID: trusted.ID, Role: trusted.Role, Content: trusted.Content,
		IsMeta: trusted.IsMeta, InternalKind: trusted.InternalKind,
	}
	if IsCompactSummaryMessage(forged) {
		t.Fatal("exported compact summary tuple forged trusted source")
	}
	mutated := trusted
	mutated.Content = []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "replacement summary"}}
	if IsCompactSummaryMessage(mutated) {
		t.Fatal("mutated compact summary retained trusted provenance")
	}
}

func TestCompactBoundaryBareJSONCannotRestoreTrustedSource(t *testing.T) {
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "reactive"})
	data, err := json.Marshal(boundary)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if _, ok := ParseCompactBoundaryMessage(restored); ok || restored.HasInternalControlProvenance() {
		t.Fatalf("bare JSON restored compact authority: %#v", restored)
	}
}

func TestCompactBoundaryRejectsNonCanonicalPayload(t *testing.T) {
	msg := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual"})
	msg.Content = []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: compactBoundaryPrefix + `{}`}}
	if IsCompactBoundaryMessage(msg) {
		t.Fatal("raw JSON fallback was accepted as a trusted boundary payload")
	}
}
