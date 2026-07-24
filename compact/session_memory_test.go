package compact

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type fakeSessionMemoryProvider struct {
	snapshot SessionMemorySnapshot
	reset    bool
	err      error
}

func (p *fakeSessionMemoryProvider) SessionMemorySnapshot(context.Context) (SessionMemorySnapshot, error) {
	if p.err != nil {
		return SessionMemorySnapshot{}, p.err
	}
	return p.snapshot, nil
}

func (p *fakeSessionMemoryProvider) ResetLastSummarizedMessage(context.Context) error {
	p.reset = true
	return nil
}

func smMsg(id string, role types.Role, text string) types.Message {
	msg := types.Message{
		ID:   id,
		Role: role,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: text},
		},
	}
	return msg
}

func smSnapshot(t *testing.T, messages []types.Message, index int, content string) SessionMemorySnapshot {
	t.Helper()
	snapshot, ok := (SessionMemorySnapshot{
		Available: true,
		Content:   content,
	}).WithCapturedLastSummarizedMessageAnchor(messages, index)
	if !ok {
		t.Fatalf("failed to capture session-memory anchor at index %d", index)
	}
	return snapshot
}

func requireSessionMemoryAnchorInvalid(t *testing.T, messages []types.Message, snapshot SessionMemorySnapshot) {
	t.Helper()
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")
	result, ok, err := TrySessionMemoryCompaction(context.Background(), messages, SessionMemoryCompactionOptions{
		Provider: &fakeSessionMemoryProvider{snapshot: snapshot},
	})
	if result != nil || ok || !errors.Is(err, ErrSessionMemoryAnchorInvalid) {
		t.Fatalf("result=%#v ok=%v err=%v, want fail-closed invalid anchor", result, ok, err)
	}
}

func TestSessionMemoryCompactionNoOpWhenUnavailableAndLegacyContinues(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")
	ResetSessionMemoryCompactConfig()

	provider := &fakeSessionMemoryProvider{snapshot: SessionMemorySnapshot{Available: false}}
	msgs := []types.Message{
		smMsg("m0", types.RoleUser, "old"),
		smMsg("m1", types.RoleAssistant, "answer"),
		smMsg("m2", types.RoleUser, "recent"),
	}
	result, ok, err := TrySessionMemoryCompaction(context.Background(), msgs, SessionMemoryCompactionOptions{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	if ok || result != nil {
		t.Fatalf("unavailable session memory should no-op, ok=%v result=%#v", ok, result)
	}

	sc := &SummaryCompactor{
		Summarize: func(context.Context, string, string) (string, error) {
			return "legacy summary", nil
		},
		KeepRecent: 1,
	}
	legacy, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	post := BuildPostCompactMessages(legacy)
	if len(post) == 0 || !strings.Contains(post[1].GetText(), "legacy summary") {
		t.Fatalf("legacy compact did not continue after SM no-op: %#v", post)
	}
}

func TestSessionMemoryCompactionEnvGateDisabled(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")
	t.Setenv("DISABLE_CLAUDE_CODE_SM_COMPACT", "true")

	provider := &fakeSessionMemoryProvider{snapshot: SessionMemorySnapshot{Available: true, Content: "memory"}}
	result, ok, err := TrySessionMemoryCompaction(context.Background(), []types.Message{types.UserMessage("hi")}, SessionMemoryCompactionOptions{Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	if ok || result != nil {
		t.Fatalf("disabled gate should no-op, ok=%v result=%#v", ok, result)
	}
}

func TestSessionMemoryCompactionConfigThresholdsExpandTail(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	msgs := []types.Message{
		smMsg("summarized", types.RoleUser, "summarized"),
		smMsg("a", types.RoleAssistant, "one"),
		smMsg("b", types.RoleUser, "two"),
		smMsg("c", types.RoleAssistant, "three"),
		smMsg("d", types.RoleUser, "four"),
	}
	provider := &fakeSessionMemoryProvider{snapshot: smSnapshot(t, msgs, 3, "session memory summary")}
	cfg := SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 3, MaxTokens: 1_000}
	result, ok, err := TrySessionMemoryCompaction(context.Background(), msgs, SessionMemoryCompactionOptions{Provider: provider, Config: &cfg})
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected session-memory compaction")
	}
	if len(result.MessagesToKeep) != 3 {
		t.Fatalf("kept len = %d, want 3", len(result.MessagesToKeep))
	}
	if result.MessagesToKeep[0].ID != "b" || result.MessagesToKeep[2].ID != "d" {
		t.Fatalf("unexpected bounded tail: %#v", result.MessagesToKeep)
	}
	if !strings.Contains(result.SummaryMessages[0].GetText(), "session memory summary") {
		t.Fatalf("summary missing session memory: %q", result.SummaryMessages[0].GetText())
	}
	if !IsCompactSummaryMessage(result.SummaryMessages[0]) || result.SummaryMessages[0].InternalKind != types.InternalMessageKindCompactSummary {
		t.Fatalf("session-memory summary lacks typed compact provenance: %#v", result.SummaryMessages[0])
	}
}

func TestSessionMemoryCompactionBoundaryFloor(t *testing.T) {
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual"})
	msgs := []types.Message{
		smMsg("pre", types.RoleUser, strings.Repeat("before ", 200)),
		boundary,
		smMsg("summarized", types.RoleUser, "summarized"),
		smMsg("tail1", types.RoleAssistant, "one"),
		smMsg("tail2", types.RoleUser, "two"),
	}
	cfg := SessionMemoryCompactConfig{MinTokens: 10_000, MinTextBlockMessages: 5, MaxTokens: 40_000}

	got := CalculateSessionMemoryMessagesToKeepIndex(msgs, 2, cfg)
	if got != 2 {
		t.Fatalf("start index = %d, want boundary floor 2", got)
	}
}

func TestSessionMemoryCompactionPreservesToolPair(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	msgs := []types.Message{
		smMsg("old", types.RoleUser, "old"),
		{
			ID:   "tool-use",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}},
			},
		},
		types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "tu_1", Content: "result"}),
		smMsg("recent", types.RoleAssistant, "recent text"),
	}
	provider := &fakeSessionMemoryProvider{snapshot: smSnapshot(t, msgs, 1, "session memory")}
	cfg := SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 1, MaxTokens: 1_000}
	result, ok, err := TrySessionMemoryCompaction(context.Background(), msgs, SessionMemoryCompactionOptions{Provider: provider, Config: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected session-memory compaction")
	}
	if len(result.MessagesToKeep) < 2 || !result.MessagesToKeep[0].HasToolUse() {
		t.Fatalf("tool_use was not preserved with tool_result tail: %#v", result.MessagesToKeep)
	}
}

func TestSessionMemoryCompactionPreservesSameAssistantID(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	msgs := []types.Message{
		smMsg("old", types.RoleUser, "old"),
		{
			ID:   "assistant-1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "thinking"},
			},
		},
		{
			ID:   "assistant-1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "visible answer"},
			},
		},
		smMsg("recent", types.RoleUser, "recent"),
	}
	provider := &fakeSessionMemoryProvider{snapshot: smSnapshot(t, msgs, 2, "session memory")}
	cfg := SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 1, MaxTokens: 1_000}
	result, ok, err := TrySessionMemoryCompaction(context.Background(), msgs, SessionMemoryCompactionOptions{Provider: provider, Config: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected session-memory compaction")
	}
	if len(result.MessagesToKeep) != 3 || result.MessagesToKeep[0].ID != "assistant-1" {
		t.Fatalf("same assistant id fragments were not preserved: %#v", result.MessagesToKeep)
	}
}

func TestSessionMemoryAnchorContiguousAssistantFragmentLogicalGroup(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	msgs := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		{
			ID:   "fragmented",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "thinking fragment"},
			},
		},
		smMsg("fragmented", types.RoleAssistant, "visible fragment"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, msgs, 2, "session memory")
	anchor := snapshot.LastSummarizedMessageAnchor
	if anchor == nil || anchor.LogicalOrdinal != 1 || anchor.FragmentCount != 2 || anchor.Role != types.RoleAssistant {
		t.Fatalf("fragment-group anchor = %#v", anchor)
	}
	index, ok := resolveSessionMemoryMessageAnchor(msgs, snapshot)
	if !ok || index != 1 {
		t.Fatalf("resolved fragment group at index %d, ok=%v; want logical start 1", index, ok)
	}

	cfg := SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 1, MaxTokens: 1_000}
	result, compacted, err := TrySessionMemoryCompaction(context.Background(), msgs, SessionMemoryCompactionOptions{
		Provider: &fakeSessionMemoryProvider{snapshot: snapshot},
		Config:   &cfg,
	})
	if err != nil || !compacted || result == nil {
		t.Fatalf("result=%#v compacted=%v err=%v", result, compacted, err)
	}
	fragmentStart := -1
	for index := range result.MessagesToKeep {
		if len(result.MessagesToKeep[index].Content) == 1 && result.MessagesToKeep[index].Content[0].GetType() == types.ContentTypeThinking {
			fragmentStart = index
			break
		}
	}
	if fragmentStart < 0 || fragmentStart+1 >= len(result.MessagesToKeep) ||
		result.MessagesToKeep[fragmentStart].ID != "fragmented" ||
		result.MessagesToKeep[fragmentStart+1].ID != "fragmented" ||
		result.MessagesToKeep[fragmentStart+1].GetText() != "visible fragment" {
		t.Fatalf("logical fragment group was split or reordered: %#v", result.MessagesToKeep)
	}
}

func TestSessionMemoryAnchorDuplicateIDFailsClosed(t *testing.T) {
	original := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("reused", types.RoleAssistant, "summarized original"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, original, 1, "session memory")
	current := append([]types.Message(nil), original...)
	current = append(current, smMsg("reused", types.RoleAssistant, "later non-contiguous reuse"))
	requireSessionMemoryAnchorInvalid(t, current, snapshot)
	if _, ok := CaptureSessionMemoryMessageAnchor(current, 1); ok {
		t.Fatal("captured a new anchor from a history with non-contiguous duplicate IDs")
	}
}

func TestSessionMemoryAnchorRejectsStaleOrReusedID(t *testing.T) {
	original := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("reused", types.RoleAssistant, "summarized original"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, original, 1, "session memory")

	t.Run("same ordinal and ID but replacement content", func(t *testing.T) {
		current := append([]types.Message(nil), original...)
		current[1] = smMsg("reused", types.RoleAssistant, "different message reusing the ID")
		requireSessionMemoryAnchorInvalid(t, current, snapshot)
	})

	t.Run("exact content moved to another logical ordinal", func(t *testing.T) {
		current := append([]types.Message{smMsg("inserted", types.RoleUser, "inserted")}, original...)
		requireSessionMemoryAnchorInvalid(t, current, snapshot)
	})

	t.Run("target missing", func(t *testing.T) {
		current := []types.Message{original[0], original[2]}
		requireSessionMemoryAnchorInvalid(t, current, snapshot)
	})
}

func TestSessionMemoryAnchorRejectsRoleMutation(t *testing.T) {
	original := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("anchor", types.RoleAssistant, "summarized"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, original, 1, "session memory")
	current := append([]types.Message(nil), original...)
	current[1].Role = types.RoleUser
	requireSessionMemoryAnchorInvalid(t, current, snapshot)
}

func TestSessionMemoryAnchorRejectsContentMutation(t *testing.T) {
	original := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("anchor", types.RoleAssistant, "summarized"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, original, 1, "session memory")
	current := append([]types.Message(nil), original...)
	current[1] = smMsg("anchor", types.RoleAssistant, "summarized but mutated")
	requireSessionMemoryAnchorInvalid(t, current, snapshot)
}

func TestSessionMemoryAnchorRejectsFragmentGroupMutation(t *testing.T) {
	original := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("fragmented", types.RoleAssistant, "first fragment"),
		smMsg("fragmented", types.RoleAssistant, "second fragment"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	snapshot := smSnapshot(t, original, 2, "session memory")

	t.Run("fragment content", func(t *testing.T) {
		current := append([]types.Message(nil), original...)
		current[2] = smMsg("fragmented", types.RoleAssistant, "mutated second fragment")
		requireSessionMemoryAnchorInvalid(t, current, snapshot)
	})
	t.Run("fragment count", func(t *testing.T) {
		current := append([]types.Message(nil), original[:2]...)
		current = append(current, original[3])
		requireSessionMemoryAnchorInvalid(t, current, snapshot)
	})
}

func TestSessionMemoryAnchorJSONRestart(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")
	originalMessages := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		{
			ID:   "assistant",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-1", Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}},
			},
		},
		smMsg("assistant", types.RoleAssistant, "visible fragment"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	originalSnapshot := smSnapshot(t, originalMessages, 2, "durable session memory")

	persisted, err := json.Marshal(struct {
		Snapshot SessionMemorySnapshot `json:"snapshot"`
		Messages []types.Message       `json:"messages"`
	}{Snapshot: originalSnapshot, Messages: originalMessages})
	if err != nil {
		t.Fatal(err)
	}
	var restored struct {
		Snapshot SessionMemorySnapshot `json:"snapshot"`
		Messages []types.Message       `json:"messages"`
	}
	if err := json.Unmarshal(persisted, &restored); err != nil {
		t.Fatal(err)
	}
	index, ok := resolveSessionMemoryMessageAnchor(restored.Messages, restored.Snapshot)
	if !ok || index != 1 {
		t.Fatalf("restart resolved index=%d ok=%v snapshot=%#v", index, ok, restored.Snapshot)
	}
	cfg := SessionMemoryCompactConfig{MinTokens: 1, MinTextBlockMessages: 1, MaxTokens: 1_000}
	result, compacted, err := TrySessionMemoryCompaction(context.Background(), restored.Messages, SessionMemoryCompactionOptions{
		Provider: &fakeSessionMemoryProvider{snapshot: restored.Snapshot},
		Config:   &cfg,
	})
	if err != nil || !compacted || result == nil {
		t.Fatalf("JSON-restarted compaction result=%#v compacted=%v err=%v", result, compacted, err)
	}
}

func TestSessionMemoryFreshCaptureRejectsExistingStrongAnchorRebind(t *testing.T) {
	messages := []types.Message{
		smMsg("summarized", types.RoleAssistant, "content covered by the summary"),
		smMsg("middle", types.RoleUser, "not summarized"),
		smMsg("later", types.RoleAssistant, "must not become summarized"),
	}
	original := smSnapshot(t, messages, 0, "summary of only the first message")
	originalAnchor := original.LastSummarizedMessageAnchor
	if originalAnchor == nil {
		t.Fatal("missing original strong anchor")
	}

	rebound, ok := original.WithCapturedLastSummarizedMessageAnchor(messages, 2)
	if ok {
		t.Fatal("fresh capture API rebound an existing strong snapshot")
	}
	if rebound.Content != original.Content || rebound.LastSummarizedMessageID != original.LastSummarizedMessageID ||
		rebound.LastSummarizedMessageAnchor != originalAnchor {
		t.Fatalf("rejected rebind mutated snapshot: original=%#v rebound=%#v", original, rebound)
	}
	resolved, valid := resolveSessionMemoryMessageAnchor(messages, rebound)
	if !valid || resolved != 0 {
		t.Fatalf("rejected rebind advanced summary authorization: index=%d valid=%v", resolved, valid)
	}
	if _, err := json.Marshal(rebound); err != nil {
		t.Fatalf("unchanged original strong snapshot no longer persists: %v", err)
	}
}

func TestSessionMemoryLegacyIDOnlyFailsClosed(t *testing.T) {
	legacyJSON := []byte(`{"Available":true,"Content":"legacy memory","LastSummarizedMessageID":"anchor"}`)
	preexistingMessages := []types.Message{smMsg("anchor", types.RoleAssistant, "old strong content")}
	snapshot := smSnapshot(t, preexistingMessages, 0, "old strong memory")
	if err := json.Unmarshal(legacyJSON, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.LastSummarizedMessageAnchor != nil || snapshot.LastSummarizedMessageID != "anchor" {
		t.Fatalf("legacy snapshot decode = %#v", snapshot)
	}
	messages := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("anchor", types.RoleAssistant, "possibly stale"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	requireSessionMemoryAnchorInvalid(t, messages, snapshot)
	if _, err := json.Marshal(snapshot); !errors.Is(err, ErrSessionMemoryAnchorInvalid) {
		t.Fatalf("re-persist legacy ID-only snapshot error = %v, want invalid anchor", err)
	}
}

func TestSessionMemoryLegacyAnchorMigratesOnlyWithAuthoritativeSaveTimeHistory(t *testing.T) {
	messages := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("anchor", types.RoleAssistant, "authoritative summarized content"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	legacy := SessionMemorySnapshot{
		Available:               true,
		Content:                 "session memory",
		LastSummarizedMessageID: "anchor",
	}
	if _, ok := legacy.WithCapturedLastSummarizedMessageAnchor(messages, 1); ok {
		t.Fatal("fresh-extraction binding accepted a legacy ID-only snapshot")
	}
	migrated, ok := legacy.WithLastSummarizedMessageAnchor(messages, 1)
	if !ok || migrated.LastSummarizedMessageAnchor == nil || migrated.LastSummarizedMessageID != "" {
		t.Fatalf("migrated snapshot = %#v ok=%v", migrated, ok)
	}
	persisted, err := json.Marshal(migrated)
	if err != nil {
		t.Fatalf("persist migrated snapshot: %v", err)
	}
	var restored SessionMemorySnapshot
	if err := json.Unmarshal(persisted, &restored); err != nil {
		t.Fatal(err)
	}
	if index, ok := resolveSessionMemoryMessageAnchor(messages, restored); !ok || index != 1 {
		t.Fatalf("restored migrated anchor index=%d ok=%v", index, ok)
	}
}

func TestSessionMemoryLegacyAnchorMigrationRejectsUnprovenIDOrIndex(t *testing.T) {
	t.Run("empty legacy ID", func(t *testing.T) {
		legacy := SessionMemorySnapshot{Available: true, Content: "legacy session memory"}
		messages := []types.Message{smMsg("authoritative", types.RoleAssistant, "summarized")}
		migrated, ok := legacy.WithLastSummarizedMessageAnchor(messages, 0)
		if ok || migrated.LastSummarizedMessageAnchor != nil || migrated.LastSummarizedMessageID != "" {
			t.Fatalf("migration result=%#v ok=%v, want unchanged empty-ID legacy snapshot", migrated, ok)
		}
		if _, err := json.Marshal(migrated); !errors.Is(err, ErrSessionMemoryAnchorInvalid) {
			t.Fatalf("empty-ID legacy migration became persistable: %v", err)
		}
	})

	legacy := SessionMemorySnapshot{
		Available:               true,
		Content:                 "legacy session memory",
		LastSummarizedMessageID: "expected-anchor",
	}
	tests := []struct {
		name     string
		messages []types.Message
		index    int
	}{
		{
			name: "mismatched authoritative ID",
			messages: []types.Message{
				smMsg("before", types.RoleUser, "before"),
				smMsg("different-anchor", types.RoleAssistant, "summarized"),
			},
			index: 1,
		},
		{
			name: "empty authoritative ID",
			messages: []types.Message{
				smMsg("before", types.RoleUser, "before"),
				types.AssistantMessage("summarized"),
			},
			index: 1,
		},
		{
			name: "wrong in-range index",
			messages: []types.Message{
				smMsg("wrong", types.RoleUser, "wrong"),
				smMsg("expected-anchor", types.RoleAssistant, "summarized"),
			},
			index: 0,
		},
		{
			name: "negative index",
			messages: []types.Message{
				smMsg("expected-anchor", types.RoleAssistant, "summarized"),
			},
			index: -1,
		},
		{
			name: "past-end index",
			messages: []types.Message{
				smMsg("expected-anchor", types.RoleAssistant, "summarized"),
			},
			index: 1,
		},
		{
			name: "non-contiguous duplicate ID",
			messages: []types.Message{
				smMsg("expected-anchor", types.RoleAssistant, "first"),
				smMsg("separator", types.RoleUser, "separator"),
				smMsg("expected-anchor", types.RoleAssistant, "second"),
			},
			index: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			migrated, ok := legacy.WithLastSummarizedMessageAnchor(tc.messages, tc.index)
			if ok || migrated.LastSummarizedMessageAnchor != nil || migrated.LastSummarizedMessageID != legacy.LastSummarizedMessageID {
				t.Fatalf("migration result=%#v ok=%v, want unchanged weak legacy snapshot", migrated, ok)
			}
			if _, err := json.Marshal(migrated); !errors.Is(err, ErrSessionMemoryAnchorInvalid) {
				t.Fatalf("invalid migration became persistable: %v", err)
			}
		})
	}
}

func TestSessionMemoryAnchorMissingMalformedOrConflictingFailsClosed(t *testing.T) {
	messages := []types.Message{
		smMsg("before", types.RoleUser, "before"),
		smMsg("anchor", types.RoleAssistant, "summarized"),
		smMsg("tail", types.RoleUser, "tail"),
	}
	strong := smSnapshot(t, messages, 1, "session memory")

	t.Run("missing", func(t *testing.T) {
		requireSessionMemoryAnchorInvalid(t, messages, SessionMemorySnapshot{Available: true, Content: "session memory"})
	})
	t.Run("malformed digest", func(t *testing.T) {
		mutated := strong
		anchor := *strong.LastSummarizedMessageAnchor
		anchor.ContentDigest = "sha256:not-a-digest"
		mutated.LastSummarizedMessageAnchor = &anchor
		requireSessionMemoryAnchorInvalid(t, messages, mutated)
	})
	t.Run("conflicting legacy mirror", func(t *testing.T) {
		mutated := strong
		mutated.LastSummarizedMessageID = "another-id"
		requireSessionMemoryAnchorInvalid(t, messages, mutated)
	})
}

func TestSessionMemoryCompactionResetTracking(t *testing.T) {
	provider := &fakeSessionMemoryProvider{}
	if err := ResetSessionMemoryCompactionTracking(context.Background(), provider); err != nil {
		t.Fatal(err)
	}
	if !provider.reset {
		t.Fatal("expected reset hook to be called")
	}
}

func TestSessionMemoryCompactionProviderError(t *testing.T) {
	t.Setenv("ENABLE_CLAUDE_CODE_SM_COMPACT", "1")

	wantErr := errors.New("boom")
	provider := &fakeSessionMemoryProvider{err: wantErr}
	_, _, err := TrySessionMemoryCompaction(context.Background(), nil, SessionMemoryCompactionOptions{Provider: provider})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
