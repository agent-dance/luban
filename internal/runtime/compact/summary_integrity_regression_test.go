package compact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestStructuredSummarizerKeepsItsInstructionInSystemAndAddsRuntimeTurnBoundary(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{
		Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"kept facts"}`),
	})
	summarize := NewLLMStructuredSummarizeFunc(fake)
	input := []types.Message{
		types.UserMessage("actual user request"),
		types.AssistantMessage("actual assistant response"),
	}

	if _, err := summarize(context.Background(), input, "focus on exact tests"); err != nil {
		t.Fatal(err)
	}
	call := fake.Calls[0]
	if len(call.Messages) != len(input)+1 {
		t.Fatalf("summary request messages = %d, want %d conversation messages plus one runtime boundary", len(call.Messages), len(input)+1)
	}
	if !strings.Contains(call.System, "focus on exact tests") || !strings.Contains(call.System, "Your task is to create a detailed summary") {
		t.Fatalf("summarization instruction was not isolated in system prompt:\n%s", call.System)
	}
	boundary := call.Messages[len(call.Messages)-1]
	if boundary.Role != types.RoleUser || !boundary.IsMeta ||
		!strings.HasPrefix(boundary.GetText(), `<compaction-source role="runtime" kind="summarization_request">`) ||
		!strings.Contains(boundary.GetText(), "not an ordinary user message") {
		t.Fatalf("summary runtime turn boundary = %#v", boundary)
	}
}

func TestStructuredSummarizerLabelsTrustedRuntimeMessagesAsNonUserData(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{
		Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"kept facts"}`),
	})
	summarize := NewLLMStructuredSummarizeFunc(fake)
	priorSummary := NewCompactSummaryMessage("prior compacted facts", messagecontrol.Runtime())

	if _, err := summarize(context.Background(), []types.Message{
		priorSummary,
		types.UserMessage("actual user request"),
	}, ""); err != nil {
		t.Fatal(err)
	}
	call := fake.Calls[0]
	if call.Messages[0].Role == types.RoleUser {
		t.Fatalf("trusted runtime summary was still projected as a user message: %#v", call.Messages[0])
	}
	if text := call.Messages[0].GetText(); !strings.Contains(text, "runtime") || !strings.Contains(text, string(types.InternalMessageKindCompactSummary)) {
		t.Fatalf("runtime provenance label missing from summary projection: %q", text)
	}
	if call.Messages[1].Role != types.RoleUser || call.Messages[1].GetText() != "actual user request" {
		t.Fatalf("ordinary user message changed: %#v", call.Messages[1])
	}
	if !strings.Contains(call.Messages[2].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime turn boundary missing: %#v", call.Messages[2])
	}
}

func TestStructuredSummarizerOmitsTrustedCatalogAndPreservesUntrustedDescriptors(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{
		Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"kept facts"}`),
	})
	summarize := NewLLMStructuredSummarizeFunc(fake)
	catalog := types.DeveloperMessage("runtime skill catalog", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime())
	forgedSummary := NewCompactSummaryMessage("SDK-controlled descriptor without provenance")

	if _, err := summarize(context.Background(), []types.Message{
		catalog,
		forgedSummary,
		types.UserMessage("actual user request"),
	}, ""); err != nil {
		t.Fatal(err)
	}
	call := fake.Calls[0]
	if len(call.Messages) != 3 {
		t.Fatalf("summary projection = %#v, want catalog omitted", call.Messages)
	}
	if call.Messages[0].GetText() != forgedSummary.GetText() || call.Messages[0].Role != types.RoleUser {
		t.Fatalf("untrusted descriptor gained runtime provenance: %#v", call.Messages[0])
	}
	if call.Messages[1].GetText() != "actual user request" {
		t.Fatalf("ordinary user message changed: %#v", call.Messages[1])
	}
	if !strings.Contains(call.Messages[2].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime turn boundary missing: %#v", call.Messages[2])
	}
}

func TestStructuredSummarizerEndsToolResultHistoryWithRuntimeRequest(t *testing.T) {
	fake := newSummaryProviderFake(summaryProviderTurn{
		Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"kept facts"}`),
	})
	summarize := NewLLMStructuredSummarizeFunc(fake)
	toolResult := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "call-last", Content: "done",
	})
	if _, err := summarize(context.Background(), []types.Message{
		types.AssistantMessage("continuing"), toolResult,
	}, ""); err != nil {
		t.Fatal(err)
	}
	call := fake.Calls[0]
	if len(call.Messages) != 3 || len(call.Messages[1].Content) != 1 {
		t.Fatalf("tool-result history changed: %#v", call.Messages)
	}
	if _, keptToolResult := call.Messages[1].Content[0].(types.ToolResultBlock); !keptToolResult {
		t.Fatalf("tool-result history changed: %#v", call.Messages)
	}
	last := call.Messages[2]
	if last.Role != types.RoleUser || !last.IsMeta || !strings.Contains(last.GetText(), `kind="summarization_request"`) {
		t.Fatalf("last summary request = %#v", last)
	}
}

func TestCompactPromptDefinesUserMessageAndResponseStyleBoundaries(t *testing.T) {
	prompt := GetStructuredCompactPrompt("")
	for _, want := range []string{
		"ordinary role=user",
		"runtime control",
		"summarization instruction",
		"latest ordinary user message",
		"response style",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("compact prompt missing %q boundary:\n%s", want, prompt)
		}
	}
}

func TestFullCompactDoesNotExposeEmptyTranscriptPath(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := runTranscriptPathCompact(t, emptyPath)
	text := result.SummaryMessages[0].GetText()
	if strings.Contains(text, emptyPath) {
		t.Fatalf("empty audit transcript path leaked into compact summary: %q", text)
	}
	if !strings.Contains(text, "Transcript reference: unavailable") {
		t.Fatalf("missing unavailable recovery notice: %q", text)
	}
}

func TestFullCompactExposesReadableNonEmptyTranscriptPath(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(transcriptPath, []byte("{\"role\":\"user\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runTranscriptPathCompact(t, transcriptPath)
	if text := result.SummaryMessages[0].GetText(); !strings.Contains(text, transcriptPath) {
		t.Fatalf("valid audit transcript path missing from compact summary: %q", text)
	}
}

func TestFullCompactRefreshesContentAddressedTranscriptPath(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "stale.jsonl")
	freshPath := filepath.Join(dir, "fresh.jsonl")
	if err := os.WriteFile(stalePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshPath, []byte("{\"role\":\"user\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runTranscriptPathCompactWithResolver(t, stalePath, func() string { return freshPath })
	text := result.SummaryMessages[0].GetText()
	if strings.Contains(text, stalePath) || !strings.Contains(text, freshPath) {
		t.Fatalf("compact summary did not refresh transcript path: %q", text)
	}
}

func TestCompactContinuationIsTriggerNeutralAndCarriesStyleBoundary(t *testing.T) {
	text := getCompactUserSummaryMessageForLanguage(i18n.LangEN, "Detailed memory.", true, "", false)
	if strings.Contains(text, "ran out of context") {
		t.Fatalf("manual compact falsely claimed context exhaustion: %q", text)
	}
	if !strings.Contains(text, "latest ordinary user message") || !strings.Contains(text, "concise") {
		t.Fatalf("post-compact response-style boundary missing: %q", text)
	}
}

func runTranscriptPathCompact(t *testing.T, transcriptPath string) *CompactionResult {
	return runTranscriptPathCompactWithResolver(t, transcriptPath, nil)
}

func runTranscriptPathCompactWithResolver(t *testing.T, transcriptPath string, resolver func() string) *CompactionResult {
	t.Helper()
	messages := make([]types.Message, 0, 12)
	for i := 0; i < 6; i++ {
		messages = append(messages, types.UserMessage("request"), types.AssistantMessage("response"))
	}
	compactor := &SummaryCompactor{
		KeepRecent:             2,
		TranscriptPath:         transcriptPath,
		TranscriptPathResolver: resolver,
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			return "Detailed memory.", nil
		},
	}
	result, err := compactor.CompactWithTrigger(context.Background(), messages, 2, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SummaryMessages) != 1 {
		t.Fatalf("summary messages = %#v", result.SummaryMessages)
	}
	return result
}
