package compact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type compactionParityManifest struct {
	SchemaVersion            string                     `json:"schema_version"`
	Fixtures                 []string                   `json:"fixtures"`
	P0RegressionSlots        []compactionParityTaskSlot `json:"p0_regression_slots"`
	CompletedFoundationSlots []compactionParityTaskSlot `json:"completed_foundation_slots"`
}

type compactionParityTaskSlot struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Case        string `json:"case"`
	PendingTask string `json:"pending_task"`
}

func TestCompactionParityHarnessDocumentsFixtures(t *testing.T) {
	manifest := loadCompactionParityManifest(t)
	if manifest.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q, want 1", manifest.SchemaVersion)
	}
	if len(manifest.Fixtures) < 5 {
		t.Fatalf("fixture count = %d, want at least 5 documented fixture shapes", len(manifest.Fixtures))
	}

	readmePath := filepath.Join("testdata", "compaction_parity", "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %s: %v", readmePath, err)
	}
	for _, fixture := range manifest.Fixtures {
		if !strings.Contains(string(readme), fixture) {
			t.Fatalf("README does not document fixture %q", fixture)
		}
	}

	p0 := map[string]bool{
		"task_01": false,
		"task_02": false,
		"task_03": false,
		"task_06": false,
		"task_07": false,
		"task_08": false,
		"task_09": false,
		"task_10": false,
		"task_11": false,
		"task_12": false,
		"task_20": false,
	}
	for _, slot := range manifest.P0RegressionSlots {
		if _, ok := p0[slot.TaskID]; ok {
			p0[slot.TaskID] = true
		}
		if slot.Status == "pending" && slot.PendingTask == "" {
			t.Fatalf("pending slot %s must name pending_task", slot.TaskID)
		}
	}
	for taskID, seen := range p0 {
		if !seen {
			t.Fatalf("missing P0 regression slot for %s", taskID)
		}
	}
}

func TestCompactionParityP0RegressionSlots(t *testing.T) {
	tests := []struct {
		taskID string
		name   string
		run    func(*testing.T)
	}{
		{"task_01", "orders boundary summary preserved attachments and hook results", assertTask01BoundaryGolden},
		{"task_02", "preserves API invariants at compact boundaries", assertTask02APIInvariantGolden},
		{"task_03", "structured compact summary path with no-tools enforcement", assertTask03StructuredSummaryGolden},
		{"task_06", "stateful aggregate tool-result budget in main query path", assertTask06ToolResultBudgetGolden},
		{"task_07", "microcompact trigger semantics", assertTask07MicrocompactTriggerSemantics},
		{"task_08", "pre-call and no-tool-turn auto-compact orchestration", skipPendingCompactionParity("task_08")},
		{"task_09", "reactive compact retry on context overflow", skipPendingCompactionParity("task_09")},
		{"task_10", "full post-compact context attachments", assertTask10PostCompactAttachmentGolden},
		{"task_11", "compact hooks progress keepalive and cleanup lifecycle", skipPendingCompactionParity("task_11")},
		{"task_12", "manual compact custom instructions", skipPendingCompactionParity("task_12")},
		{"task_20", "harness manifest and fixture documentation", func(t *testing.T) {
			manifest := loadCompactionParityManifest(t)
			if len(manifest.P0RegressionSlots) == 0 {
				t.Fatal("manifest has no P0 regression slots")
			}
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.taskID+"_"+tt.name, tt.run)
	}
}

func TestCompactionParityCompletedFoundationSlots(t *testing.T) {
	t.Run("task_05_result_store_large_tool_result", func(t *testing.T) {
		rs := NewResultStore(t.TempDir())
		result := rs.ProcessResult(types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: "large_1",
			Content:   strings.Repeat("x", ResultStoreDefaultThreshold+1024),
		})
		if result.Content == "" || !strings.Contains(result.Content, persistedOutputTag) {
			t.Fatalf("large result was not replaced with a compact file reference: %.120q", result.Content)
		}
		if !strings.Contains(result.Content, "Full output saved to:") {
			t.Fatalf("persisted result did not include replay path: %.120q", result.Content)
		}
	})
}

func TestCompactionSummaryProviderFakes(t *testing.T) {
	tests := []struct {
		name        string
		fake        *summaryProviderFake
		wantSummary string
		wantErr     string
	}{
		{
			name: "successful summary",
			fake: newSummaryProviderFake(summaryProviderTurn{
				Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"kept facts"}`),
			}),
			wantSummary: "kept facts",
		},
		{
			name: "incomplete summary response is modelled",
			fake: newSummaryProviderFake(summaryProviderTurn{
				Events: compactTextEventsWithStopReason("<summary>partial", types.StopReasonMaxTokens),
			}),
			wantErr: "Compaction interrupted",
		},
		{
			name: "prompt too long on summary request",
			fake: newSummaryProviderFake(summaryProviderTurn{
				Err: &types.APIError{Type: "prompt_too_long", Message: "prompt is too long", Status: 400},
			}),
			wantErr: "prompt is too long",
		},
		{
			name: "empty assistant response",
			fake: newSummaryProviderFake(summaryProviderTurn{
				Events: []types.StreamEvent{{Type: types.EventMessageStart}, {Type: types.EventMessageStop}},
			}),
			wantErr: "Failed to generate conversation summary",
		},
		{
			name: "media size error",
			fake: newSummaryProviderFake(summaryProviderTurn{
				Err: errors.New("media size exceeds provider limit"),
			}),
			wantErr: "media size exceeds provider limit",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			summarize := NewLLMStructuredSummarizeFunc(tt.fake)
			got, err := summarize(context.Background(), []types.Message{types.UserMessage("conversation")}, "")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.wantSummary {
				t.Fatalf("summary = %q, want %q", got, tt.wantSummary)
			}
			if len(tt.fake.Calls) != 1 {
				t.Fatalf("provider calls = %d, want 1", len(tt.fake.Calls))
			}
			call := tt.fake.Calls[0]
			if !strings.HasPrefix(call.System, CompactSystemPrompt) || !strings.Contains(call.System, "Your task is to create a detailed summary") {
				t.Fatalf("system prompt is missing the compact contract: %q", call.System)
			}
			if call.MaxTokens != CompactMaxOutputTokens {
				t.Fatalf("max tokens = %d, want %d", call.MaxTokens, CompactMaxOutputTokens)
			}
			if len(call.Tools) != 0 {
				t.Fatalf("summary provider call exposed tools: %#v", call.Tools)
			}
		})
	}
}

func TestCompactionParityFixtureShapes(t *testing.T) {
	t.Run("long conversation keeps tool_use and matching tool_result pairs", func(t *testing.T) {
		msgs := longToolConversationFixture(3)
		if len(msgs) != 9 {
			t.Fatalf("fixture len = %d, want 9", len(msgs))
		}
		for i := 0; i < 3; i++ {
			assistant := msgs[i*3+1]
			result := msgs[i*3+2]
			if len(assistant.GetToolUses()) == 0 || !hasToolResultBlock(result) {
				t.Fatalf("turn %d missing tool pair: %#v %#v", i, assistant, result)
			}
		}
	})

	t.Run("parallel results fixture keeps sibling tool results in one user message", func(t *testing.T) {
		msg := parallelToolResultsFixture(3, 128)
		if msg.Role != types.RoleUser || len(msg.Content) != 3 {
			t.Fatalf("parallel result fixture = %#v, want one user message with 3 blocks", msg)
		}
		for _, block := range msg.Content {
			if _, ok := block.(types.ToolResultBlock); !ok {
				t.Fatalf("parallel result block = %T, want ToolResultBlock", block)
			}
		}
	})

	t.Run("media fixture includes top-level and nested media", func(t *testing.T) {
		msgs := mediaAttachmentFixture()
		if len(msgs) != 2 {
			t.Fatalf("media fixture len = %d, want 2", len(msgs))
		}
		if !messageHasMedia(msgs[0]) || !messageHasNestedMedia(msgs[1]) {
			t.Fatalf("media fixture missing top-level or nested media: %#v", msgs)
		}
	})

	t.Run("session metadata fixture records provider model cwd and branch", func(t *testing.T) {
		meta := sessionMetadataFixture()
		if meta.ID == "" || meta.Provider == "" || meta.Model == "" || meta.CWD == "" || meta.GitBranch == "" {
			t.Fatalf("session metadata fixture is incomplete: %+v", meta)
		}
	})
}

func assertTask01BoundaryGolden(t *testing.T) {
	t.Helper()
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{
		Trigger:                   "manual",
		PreCompactTokenCount:      1234,
		PreviousTailIdentifier:    "assistant:tail",
		PreCompactDiscoveredTools: []string{"Read", "ToolSearch"},
		PreservedSegment:          &PreservedSegmentMetadata{StartIndex: 3, Count: 2, Anchor: "tail"},
	})
	result := &CompactionResult{
		BoundaryMarker:  &boundary,
		SummaryMessages: []types.Message{types.UserMessage("summary message")},
		MessagesToKeep:  []types.Message{types.AssistantMessage("preserved assistant")},
		Attachments:     []types.Message{types.UserMessage("plan attachment")},
		HookResults:     []types.Message{types.UserMessage("hook result")},
	}

	got := BuildPostCompactMessages(result)
	wantTexts := []string{compactBoundaryPrefix, "summary message", "preserved assistant", "plan attachment", "hook result"}
	if len(got) != len(wantTexts) {
		t.Fatalf("post-compact length = %d, want %d", len(got), len(wantTexts))
	}
	for i, want := range wantTexts {
		if !strings.Contains(got[i].GetText(), want) {
			t.Fatalf("message %d text = %q, want containing %q", i, got[i].GetText(), want)
		}
	}
	metadata, ok := ParseCompactBoundaryMessage(got[0])
	if !ok {
		t.Fatal("first message is not parseable compact boundary")
	}
	if metadata.Trigger != "manual" || metadata.PreCompactTokenCount != 1234 || metadata.PreservedSegment.Count != 2 {
		t.Fatalf("unexpected boundary metadata: %+v", metadata)
	}
}

func assertTask03StructuredSummaryGolden(t *testing.T) {
	t.Helper()
	fake := newSummaryProviderFake(summaryProviderTurn{
		Events: compactTextEvents(`{"schema":"compact-summary/v2","summary":"structured summary"}`),
	})
	summarize := NewLLMStructuredSummarizeFunc(fake)
	input := longToolConversationFixture(1)
	got, err := summarize(context.Background(), input, "focus on tool contracts")
	if err != nil {
		t.Fatal(err)
	}
	if got != "structured summary" {
		t.Fatalf("summary = %q, want structured summary", got)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fake.Calls))
	}
	call := fake.Calls[0]
	if len(call.Tools) != 0 {
		t.Fatalf("structured summary exposed tools: %#v", call.Tools)
	}
	if call.ToolChoice != nil {
		t.Fatalf("structured summary set tool choice: %#v", call.ToolChoice)
	}
	if call.Thinking == nil || call.Thinking.Enabled {
		t.Fatalf("structured summary should explicitly disable thinking, got %#v", call.Thinking)
	}
	if len(call.Messages) != len(input)+1 {
		t.Fatalf("structured summary messages = %d, want %d conversation messages plus runtime request", len(call.Messages), len(input))
	}
	if !strings.Contains(call.Messages[len(call.Messages)-1].GetText(), `kind="summarization_request"`) {
		t.Fatalf("runtime summary request missing: %#v", call.Messages[len(call.Messages)-1])
	}
	if !strings.Contains(call.System, "Additional Instructions") {
		t.Fatalf("custom instructions missing from isolated compact prompt: %q", call.System)
	}
}

func assertTask06ToolResultBudgetGolden(t *testing.T) {
	t.Helper()
	state := NewContentReplacementState()
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		return "<persisted-output>\n" + toolUseID + "\n</persisted-output>", nil
	})
	messages := []types.Message{
		assistantWithTools("parallel_a", "parallel_b"),
		parallelToolResultsFixture(2, 120_000),
	}

	got, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 {
		t.Fatalf("ApplyToolResultBudget errors = %v", errs)
	}
	if len(records) != 1 {
		t.Fatalf("replacement records = %d, want 1 over-budget sibling result", len(records))
	}
	replaced := records[0].ToolUseID
	if replaced != "parallel_a" && replaced != "parallel_b" {
		t.Fatalf("replaced id = %q, want one parallel tool id", replaced)
	}
	if !strings.Contains(extractToolResultContent(got, replaced), "<persisted-output>") {
		t.Fatalf("replacement was not visible in compacted messages")
	}
}

func assertTask10PostCompactAttachmentGolden(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan\n\nKeep the compact parity harness extendable."), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &RuntimeAttachmentProvider{
		PlanState:       fakePlanState{active: true, file: planPath},
		BackgroundTasks: fakeBackgroundTasks{{ID: "bg_1", Type: "agent", Status: "running", Description: "verify compact suite"}},
		MCPState:        fakeMCPState{{Name: "docs", Tools: []string{"search", "read"}, Instructions: "Use official docs."}},
		AgentDefinitions: fakeAgents{{
			Name:      "test-engineer",
			WhenToUse: "Write regression tests.",
			Source:    "builtin",
		}},
		SessionID:         "session-fixture",
		CWD:               dir,
		DeferredToolNames: func() []string { return []string{"DeferredSearch"} },
		LoadedToolNames:   func() []string { return []string{"Read"} },
	}
	attachments := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	got := joinMessageText(attachments)
	for _, want := range []string{
		"Post-compaction plan state",
		"Keep the compact parity harness extendable.",
		"Post-compaction plan mode reminder",
		"Post-compaction background tasks",
		"verify compact suite",
		"Post-compaction deferred tools",
		"DeferredSearch",
		"Post-compaction agent listing",
		"test-engineer",
		"Post-compaction MCP state",
		"Use official docs.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("attachment golden missing %q:\n%s", want, got)
		}
	}
}

func assertTask02APIInvariantGolden(t *testing.T) {
	t.Helper()
	msgs := sameIDAssistantFragmentFixture()
	if got := AdjustIndexToPreserveAPIInvariants(msgs, 1); got != 0 {
		t.Fatalf("adjusted index = %d, want 0 to keep same-id assistant fragments together", got)
	}

}

func skipPendingCompactionParity(taskID string) func(*testing.T) {
	return func(t *testing.T) {
		t.Skipf("TODO(%s): parity behavior not implemented yet; slot is reserved in compaction parity harness", taskID)
	}
}

func loadCompactionParityManifest(t *testing.T) compactionParityManifest {
	t.Helper()
	path := filepath.Join("testdata", "compaction_parity", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var manifest compactionParityManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return manifest
}

func longToolConversationFixture(turns int) []types.Message {
	msgs := make([]types.Message, 0, turns*3)
	for i := 0; i < turns; i++ {
		id := "tool_" + string(rune('a'+i))
		msgs = append(msgs,
			types.UserMessage("request "+id),
			types.Message{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{types.ToolUseBlock{
					Type:  types.ContentTypeToolUse,
					ID:    id,
					Name:  "Read",
					Input: map[string]any{"file_path": "/tmp/" + id + ".go"},
				}},
			},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: id, Content: strings.Repeat("result "+id+" ", 20)}),
		)
	}
	return msgs
}

func parallelToolResultsFixture(count int, bytesPerResult int) types.Message {
	results := make([]types.ToolResultBlock, 0, count)
	for i := 0; i < count; i++ {
		id := "parallel_" + string(rune('a'+i))
		results = append(results, types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: id,
			Content:   strings.Repeat(id+" ", bytesPerResult/max(1, len(id)+1)+1),
		})
	}
	return types.ToolResultMessage(results...)
}

func sameIDAssistantFragmentFixture() []types.Message {
	return []types.Message{
		{ID: "assistant_1", Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "thinking"},
		}},
		{ID: "assistant_1", Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Read", Input: map[string]any{"file_path": "/tmp/a.go"}},
		}},
		types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "tu_1", Content: "ok"}),
	}
}

func mediaAttachmentFixture() []types.Message {
	return []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "inspect media"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "iVBORw0KGgo="}},
				types.DocumentBlock{Type: types.ContentTypeDocument, Source: &types.DocumentSource{Type: "base64", MediaType: "application/pdf", Data: "JVBERi0="}},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "media_tool",
			ContentBlocks: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "tool media"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "iVBORw0KGgo="}},
			},
		}),
	}
}

func sessionMetadataFixture() session.SessionMeta {
	return session.SessionMeta{
		ID:           "session-parity-fixture",
		Title:        "Compaction parity fixture",
		CreatedAt:    time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 7, 10, 9, 30, 0, 0, time.UTC),
		MessageCount: 42,
		CWD:          "/workspace/project",
		GitBranch:    "compaction-parity",
		PreviewText:  "long compactable conversation",
		Provider:     "parity-fake",
		Model:        "parity-model",
	}
}

func hasToolResultBlock(msg types.Message) bool {
	for _, block := range msg.Content {
		if _, ok := block.(types.ToolResultBlock); ok {
			return true
		}
	}
	return false
}

func messageHasMedia(msg types.Message) bool {
	for _, block := range msg.Content {
		if block.GetType() == types.ContentTypeImage || block.GetType() == types.ContentTypeDocument {
			return true
		}
	}
	return false
}

func messageHasNestedMedia(msg types.Message) bool {
	for _, block := range msg.Content {
		result, ok := block.(types.ToolResultBlock)
		if !ok {
			continue
		}
		for _, nested := range result.ContentBlocks {
			if nested.GetType() == types.ContentTypeImage || nested.GetType() == types.ContentTypeDocument {
				return true
			}
		}
	}
	return false
}

type summaryProviderFake struct {
	turns []summaryProviderTurn
	Calls []provider.Params
}

type summaryProviderTurn struct {
	Events []types.StreamEvent
	Err    error
}

func newSummaryProviderFake(turns ...summaryProviderTurn) *summaryProviderFake {
	return &summaryProviderFake{turns: turns}
}

func (p *summaryProviderFake) Name() string { return "summary-parity-fake" }

func (p *summaryProviderFake) ModelID() string { return "summary-parity-model" }

func (p *summaryProviderFake) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.Calls = append(p.Calls, params)
	if len(p.turns) == 0 {
		ch := make(chan types.StreamEvent)
		close(ch)
		return ch, nil
	}
	turn := p.turns[0]
	p.turns = p.turns[1:]
	if turn.Err != nil {
		return nil, turn.Err
	}
	ch := make(chan types.StreamEvent, len(turn.Events))
	for _, event := range turn.Events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func compactTextEvents(text string) []types.StreamEvent {
	return compactTextEventsWithStopReason(text, types.StopReasonEndTurn)
}

func compactTextEventsWithStopReason(text string, reason types.StopReason) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReason(reason)},
		{Type: types.EventMessageStop},
	}
}

func stopReason(reason types.StopReason) *types.StopReason {
	return &reason
}
