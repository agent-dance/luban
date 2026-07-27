package compact

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func Microcompact(messages []types.Message, cfg MicrocompactConfig) []types.Message {
	return MicrocompactWithResult(messages, cfg).Messages
}

// helper to build an assistant message with a single tool_use block
func toolUseMsg(id, name string) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    id,
				Name:  name,
				Input: map[string]any{},
			},
		},
	}
}

func TestCachedMicrocompactAddsCacheEditsWithoutLocalClearing(t *testing.T) {
	msgs := []types.Message{types.UserMessage("start")}
	for i := 0; i < 3; i++ {
		id := "cached_" + string(rune('0'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"), toolResultMsg(id, "content "+id))
	}
	cfg := DefaultMicrocompactConfig()
	cfg.QuerySource = MicrocompactSourceMain
	cfg.CachedEnabled = true
	cfg.CachedTriggerThreshold = 2
	cfg.CachedKeepRecent = 1

	result := CachedMicrocompact(msgs, cfg, NewCachedMicrocompactState())
	if !result.Changed {
		t.Fatal("cached microcompact did not trigger")
	}
	if got := findMicrocompactTestToolResult(msgs, "cached_0"); got != "content cached_0" {
		t.Fatalf("input tool result mutated: %q", got)
	}
	if got := findMicrocompactTestToolResult(result.Messages, "cached_0"); got != "content cached_0" {
		t.Fatalf("provider-bound messages locally cleared cached result: %q", got)
	}

	last := result.Messages[len(result.Messages)-1]
	if len(last.Content) < 3 {
		t.Fatalf("last user content too short: %#v", last.Content)
	}
	if _, ok := last.Content[0].(types.ToolResultBlock); !ok {
		t.Fatalf("first block = %#v, want tool_result", last.Content[0])
	}
	cacheEdits, ok := last.Content[1].(types.UnknownBlock)
	if !ok || cacheEdits.Type != ContentTypeCacheEdits {
		t.Fatalf("second block = %#v, want cache_edits after tool_result", last.Content[1])
	}
	if text, ok := last.Content[2].(types.TextBlock); !ok || text.Text != "." {
		t.Fatalf("third block = %#v, want text continuation", last.Content[2])
	}
	var body CacheEditsBlock
	if err := json.Unmarshal(cacheEdits.Raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Edits) != 2 || body.Edits[0].CacheReference != "cached_0" || body.Edits[1].CacheReference != "cached_1" {
		t.Fatalf("cache edits = %#v, want deletes for oldest two", body.Edits)
	}
}

// helper to build a user message with a single tool_result block
func toolResultMsg(toolUseID, content string) types.Message {
	return types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.ToolResultBlock{
				Type:      types.ContentTypeToolResult,
				ToolUseID: toolUseID,
				Content:   content,
			},
		},
	}
}

func findMicrocompactTestToolResult(messages []types.Message, id string) string {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok && tr.ToolUseID == id {
				return tr.Content
			}
		}
	}
	return ""
}

func TestMicrocompact_BasicClearing(t *testing.T) {
	// Build a conversation with 12 compactable tool results
	var msgs []types.Message
	msgs = append(msgs, types.UserMessage("hello"))
	for i := 0; i < 12; i++ {
		id := "tu_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"))
		msgs = append(msgs, toolResultMsg(id, "file content "+string(rune('a'+i))))
	}

	cfg := idleMainConfig(10)
	result := Microcompact(msgs, cfg)

	// Should have same number of messages
	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}

	// First 2 tool results (oldest) should be cleared
	cleared := 0
	preserved := 0
	for _, msg := range result {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.Content == microcompactClearedText() {
					cleared++
				} else {
					preserved++
				}
			}
		}
	}
	if cleared != 2 {
		t.Errorf("expected 2 cleared results, got %d", cleared)
	}
	if preserved != 10 {
		t.Errorf("expected 10 preserved results, got %d", preserved)
	}
}

func TestMicrocompact_KeepRecentPreservesCorrectOnes(t *testing.T) {
	var msgs []types.Message
	msgs = append(msgs, types.UserMessage("start"))
	for i := 0; i < 5; i++ {
		id := "id_" + string(rune('0'+i))
		msgs = append(msgs, toolUseMsg(id, "Bash"))
		msgs = append(msgs, toolResultMsg(id, "output_"+string(rune('0'+i))))
	}

	cfg := idleMainConfig(3)
	result := Microcompact(msgs, cfg)

	// The first 2 results should be cleared, last 3 preserved
	var clearedIDs, preservedIDs []string
	for _, msg := range result {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.Content == microcompactClearedText() {
					clearedIDs = append(clearedIDs, tr.ToolUseID)
				} else {
					preservedIDs = append(preservedIDs, tr.ToolUseID)
				}
			}
		}
	}

	if len(clearedIDs) != 2 {
		t.Errorf("expected 2 cleared, got %d: %v", len(clearedIDs), clearedIDs)
	}
	if len(preservedIDs) != 3 {
		t.Errorf("expected 3 preserved, got %d: %v", len(preservedIDs), preservedIDs)
	}
	// Verify the oldest are cleared
	if len(clearedIDs) >= 2 {
		if clearedIDs[0] != "id_0" || clearedIDs[1] != "id_1" {
			t.Errorf("expected oldest IDs cleared, got %v", clearedIDs)
		}
	}
}

func TestMicrocompact_NonCompactableToolsUntouched(t *testing.T) {
	var msgs []types.Message
	msgs = append(msgs, types.UserMessage("start"))

	// Add a non-compactable tool result
	msgs = append(msgs, toolUseMsg("custom_1", "CustomTool"))
	msgs = append(msgs, toolResultMsg("custom_1", "custom output"))

	// Add many compactable ones to exceed KeepRecent
	for i := 0; i < 5; i++ {
		id := "read_" + string(rune('0'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"))
		msgs = append(msgs, toolResultMsg(id, "file content"))
	}

	cfg := idleMainConfig(3)
	result := Microcompact(msgs, cfg)

	// The CustomTool result should NOT be cleared
	for _, msg := range result {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok && tr.ToolUseID == "custom_1" {
				if tr.Content == microcompactClearedText() {
					t.Error("non-compactable tool result was incorrectly cleared")
				}
				if tr.Content != "custom output" {
					t.Errorf("expected 'custom output', got %q", tr.Content)
				}
			}
		}
	}
}

func TestMicrocompact_SmallConversationUntouched(t *testing.T) {
	var msgs []types.Message
	msgs = append(msgs, types.UserMessage("hello"))
	msgs = append(msgs, toolUseMsg("t1", "Read"))
	msgs = append(msgs, toolResultMsg("t1", "some content"))

	cfg := DefaultMicrocompactConfig() // KeepRecent=10
	result := Microcompact(msgs, cfg)

	// Should return the original slice (no copy needed)
	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}
	// Content should be unchanged
	for _, msg := range result {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.Content != "some content" {
					t.Errorf("expected unchanged content, got %q", tr.Content)
				}
			}
		}
	}
}

func TestMicrocompact_EmptyConversation(t *testing.T) {
	result := Microcompact(nil, DefaultMicrocompactConfig())
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}

	result = Microcompact([]types.Message{}, DefaultMicrocompactConfig())
	if len(result) != 0 {
		t.Errorf("expected empty for empty input, got %v", result)
	}
}

func TestMicrocompact_DoesNotMutateInput(t *testing.T) {
	var msgs []types.Message
	msgs = append(msgs, types.UserMessage("start"))
	for i := 0; i < 12; i++ {
		id := "tu_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Grep"))
		msgs = append(msgs, toolResultMsg(id, "original content"))
	}

	cfg := idleMainConfig(10)
	_ = Microcompact(msgs, cfg)

	// Verify original messages are not mutated
	for _, msg := range msgs {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.Content != "original content" {
					t.Errorf("input was mutated: got %q", tr.Content)
				}
			}
		}
	}
}

// ── Idle / time-triggered microcompact tests ─────────────────────────────────

func TestMicrocompact_TimeBasedIdleMainClearsOlderResults(t *testing.T) {
	// Build 8 compactable results
	var msgs []types.Message
	msgs = append(msgs, toolUseMsg("start_tu", "Read"))
	msgs = append(msgs, toolResultMsg("start_tu", "start"))
	for i := 0; i < 7; i++ {
		id := "idle_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"))
		msgs = append(msgs, toolResultMsg(id, "content_"+string(rune('a'+i))))
	}

	resultInfo := MicrocompactWithResult(msgs, idleMainConfig(3))
	result := resultInfo.Messages
	if !resultInfo.TimeBasedTriggered {
		t.Fatal("expected time-based microcompact to trigger")
	}

	cleared, preserved := 0, 0
	for _, msg := range result {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.Content == microcompactClearedText() {
					cleared++
				} else {
					preserved++
				}
			}
		}
	}
	if preserved != 3 {
		t.Errorf("expected 3 preserved, got %d", preserved)
	}
	if cleared != 5 {
		t.Errorf("expected 5 cleared, got %d", cleared)
	}
}

func TestMicrocompact_NotIdleLeavesMessagesUnchanged(t *testing.T) {
	var msgs []types.Message
	for i := 0; i < 8; i++ {
		id := "act_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Bash"))
		msgs = append(msgs, toolResultMsg(id, "out_"+string(rune('a'+i))))
	}

	cfg := MicrocompactConfig{
		KeepRecent:       2,
		TimeBasedEnabled: true,
		QuerySource:      MicrocompactSourceMain,
		IdleThreshold:    60 * time.Minute,
		LastActivity:     time.Now(),
	}
	resultInfo := MicrocompactWithResult(msgs, cfg)
	if resultInfo.Changed {
		t.Fatal("normal non-idle request should not microcompact")
	}
	if !messagesEqual(resultInfo.Messages, msgs) {
		t.Fatal("normal non-idle request changed messages")
	}
}

func TestMicrocompact_IdleThresholdZero_Disabled(t *testing.T) {
	var msgs []types.Message
	for i := 0; i < 8; i++ {
		id := "dis_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Grep"))
		msgs = append(msgs, toolResultMsg(id, "found"))
	}

	cfg := MicrocompactConfig{
		KeepRecent:       1,
		TimeBasedEnabled: true,
		QuerySource:      MicrocompactSourceMain,
		IdleThreshold:    0,
		LastActivity:     time.Now().Add(-24 * time.Hour),
	}
	resultInfo := MicrocompactWithResult(msgs, cfg)

	if resultInfo.Changed {
		t.Fatal("IdleThreshold=0 should disable time-based microcompact")
	}
}

func TestMicrocompact_UndefinedAndNonMainNeverTrigger(t *testing.T) {
	msgs := []types.Message{types.UserMessage("start")}
	for i := 0; i < 4; i++ {
		id := "src_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"))
		msgs = append(msgs, toolResultMsg(id, "content"))
	}

	for _, source := range []MicrocompactQuerySource{MicrocompactSourceUndefined, MicrocompactSourceNonMain} {
		cfg := idleMainConfig(1)
		cfg.QuerySource = source
		resultInfo := MicrocompactWithResult(msgs, cfg)
		if resultInfo.Changed {
			t.Fatalf("source %q should not trigger time-based microcompact", source)
		}
	}
}

func TestMicrocompact_TimeBasedKeepsAtLeastOneResult(t *testing.T) {
	var msgs []types.Message
	for i := 0; i < 3; i++ {
		id := "keep_" + string(rune('a'+i))
		msgs = append(msgs, toolUseMsg(id, "Read"))
		msgs = append(msgs, toolResultMsg(id, "content"))
	}

	cfg := idleMainConfig(0)
	resultInfo := MicrocompactWithResult(msgs, cfg)
	if resultInfo.ToolsKept != 1 {
		t.Fatalf("ToolsKept = %d, want 1", resultInfo.ToolsKept)
	}
	if got := findResultContent(resultInfo.Messages, "keep_c"); got == microcompactClearedText() {
		t.Fatal("most recent compactable result was cleared")
	}
}

func TestMicrocompact_DefaultConfigRequiresExplicitMainSource(t *testing.T) {
	cfg := DefaultMicrocompactConfig()
	if cfg.IdleThreshold != 60*time.Minute {
		t.Errorf("expected 60min IdleThreshold, got %v", cfg.IdleThreshold)
	}
	if cfg.KeepRecent != 5 {
		t.Errorf("expected KeepRecent=5, got %d", cfg.KeepRecent)
	}
	if !cfg.TimeBasedEnabled {
		t.Error("expected time-based microcompact enabled by default")
	}
	if cfg.QuerySource != MicrocompactSourceUndefined {
		t.Errorf("expected undefined default source, got %q", cfg.QuerySource)
	}
	if !cfg.AgenticV2ProofsEnabled {
		t.Error("expected Agentic V2 proof-preserving microcompact enabled by default")
	}
}

func idleMainConfig(keepRecent int) MicrocompactConfig {
	return MicrocompactConfig{
		KeepRecent:       keepRecent,
		TimeBasedEnabled: true,
		QuerySource:      MicrocompactSourceMain,
		IdleThreshold:    60 * time.Minute,
		LastActivity:     time.Now().Add(-2 * time.Hour),
	}
}

func messagesEqual(a, b []types.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || len(a[i].Content) != len(b[i].Content) {
			return false
		}
		for j := range a[i].Content {
			if !reflect.DeepEqual(a[i].Content[j], b[i].Content[j]) {
				return false
			}
		}
	}
	return true
}

func findResultContent(messages []types.Message, id string) string {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == id {
				return result.Content
			}
		}
	}
	return ""
}

func assertTask07MicrocompactTriggerSemantics(t *testing.T) {
	t.Run("non-idle main unchanged", TestMicrocompact_NotIdleLeavesMessagesUnchanged)
	t.Run("idle main clears older results", TestMicrocompact_TimeBasedIdleMainClearsOlderResults)
	t.Run("undefined and non-main do not trigger", TestMicrocompact_UndefinedAndNonMainNeverTrigger)
	t.Run("keeps at least one result", TestMicrocompact_TimeBasedKeepsAtLeastOneResult)
	t.Run("does not mutate input", TestMicrocompact_DoesNotMutateInput)
}
