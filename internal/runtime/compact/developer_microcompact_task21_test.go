package compact

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestDeveloperMicrocompactPreservesOrderRoleContentAndMetadata(t *testing.T) {
	t.Parallel()

	snapshot := task21Developer("snapshot", types.DeveloperMessageKindSkillCatalogSnapshot, 4)
	delta := task21Developer("delta", types.DeveloperMessageKindSkillCatalogDelta, 5)
	messages := []types.Message{
		snapshot,
		types.UserMessage("first user"),
		task21ToolUse("old", "Read"),
		task21ToolResult("old", "old result"),
		task21ToolUse("new", "Read"),
		task21ToolResult("new", "new result"),
		delta,
		types.UserMessage("current user"),
	}

	result := MicrocompactWithResult(messages, task21IdleConfig(1))
	if !result.Changed || result.ToolsCleared != 1 {
		t.Fatalf("result = %#v, want one cleared result", result)
	}
	if len(result.Messages) != len(messages) {
		t.Fatalf("message count = %d, want %d", len(result.Messages), len(messages))
	}
	for index := range messages {
		if result.Messages[index].Role != messages[index].Role {
			t.Fatalf("message %d role = %q, want %q", index, result.Messages[index].Role, messages[index].Role)
		}
	}
	for _, index := range []int{0, 6} {
		if !reflect.DeepEqual(result.Messages[index], messages[index]) {
			t.Fatalf("developer message %d changed:\n got: %#v\nwant: %#v", index, result.Messages[index], messages[index])
		}
	}
	if got := task21ToolResultContent(result.Messages, "old"); got != microcompactClearedText() {
		t.Fatalf("old result = %q", got)
	}
	if got := task21ToolResultContent(messages, "old"); got != "old result" {
		t.Fatalf("input was mutated: %q", got)
	}
}

func TestDeveloperCachedMicrocompactPreservesCatalogAndPinsAfterCurrentUser(t *testing.T) {
	t.Parallel()

	delta := task21Developer("delta", types.DeveloperMessageKindSkillCatalogDelta, 8)
	messages := []types.Message{types.UserMessage("start")}
	for _, id := range []string{"a", "b"} {
		messages = append(messages, task21ToolUse(id, "Read"), task21ToolResult(id, id+" result"))
	}
	messages = append(messages,
		task21ToolUse("c", "Read"),
		delta,
		task21ToolResult("c", "c result"),
	)

	cfg := task21CachedConfig()
	result := CachedMicrocompact(messages, cfg, NewCachedMicrocompactState())
	if !result.Changed || !reflect.DeepEqual(result.DeletedToolIDs, []string{"a", "b"}) {
		t.Fatalf("result = %#v, want deletes for a and b", result)
	}
	developerIndex := len(messages) - 2
	if !reflect.DeepEqual(result.Messages[developerIndex], delta) {
		t.Fatalf("developer message changed:\n got: %#v\nwant: %#v", result.Messages[developerIndex], delta)
	}
	if result.Messages[developerIndex+1].Role != types.RoleUser {
		t.Fatalf("catalog no longer precedes current user: %#v", result.Messages[developerIndex:])
	}
	if !task21HasCacheEdits(result.Messages[developerIndex+1]) {
		t.Fatalf("cache edits were not pinned to the current user: %#v", result.Messages[developerIndex+1].Content)
	}
	if task21HasCacheEdits(result.Messages[3]) {
		t.Fatal("cache edits crossed the developer boundary into an older user message")
	}
}

func TestDeveloperCachedMicrocompactWaitsForUserAfterCatalog(t *testing.T) {
	t.Parallel()

	messages := []types.Message{types.UserMessage("start")}
	for _, id := range []string{"a", "b", "c"} {
		messages = append(messages, task21ToolUse(id, "Read"), task21ToolResult(id, id+" result"))
	}
	messages = append(messages, task21Developer("pending delta", types.DeveloperMessageKindSkillCatalogDelta, 9))
	state := NewCachedMicrocompactState()
	result := CachedMicrocompact(messages, task21CachedConfig(), state)
	if result.Changed || len(result.DeletedToolIDs) != 0 || len(state.DeletedRefs) != 0 || len(state.PinnedEdits) != 0 {
		t.Fatalf("cached microcompact crossed pending developer boundary: result=%#v state=%#v", result, state)
	}
	if !reflect.DeepEqual(result.Messages, messages) {
		t.Fatal("messages changed while waiting for a current user")
	}

	withCurrentUser := append(append([]types.Message(nil), messages...), types.UserMessage("current user"))
	resumed := CachedMicrocompact(withCurrentUser, task21CachedConfig(), state)
	if !resumed.Changed || !reflect.DeepEqual(resumed.DeletedToolIDs, []string{"a", "b"}) {
		t.Fatalf("resumed result = %#v, want deletes for a and b", resumed)
	}
	if !reflect.DeepEqual(resumed.Messages[len(messages)-1], messages[len(messages)-1]) {
		t.Fatal("developer message changed when cached microcompact resumed")
	}
	if !task21HasCacheEdits(resumed.Messages[len(resumed.Messages)-1]) {
		t.Fatal("cache edits were not pinned to the first user after the developer boundary")
	}
	if task21HasCacheEdits(resumed.Messages[0]) {
		t.Fatal("cache edits crossed the developer boundary after cached microcompact resumed")
	}
}

func TestDeveloperMicrocompactCachedAndUncachedKeepSameCatalogProjection(t *testing.T) {
	t.Parallel()

	snapshot := task21Developer("snapshot", types.DeveloperMessageKindSkillCatalogSnapshot, 2)
	delta := task21Developer("delta", types.DeveloperMessageKindSkillCatalogDelta, 3)
	messages := []types.Message{snapshot, types.UserMessage("start")}
	for _, id := range []string{"a", "b", "c"} {
		messages = append(messages, task21ToolUse(id, "Read"), task21ToolResult(id, id+" result"))
	}
	messages = append(messages, delta, types.UserMessage("current"))

	uncached := Microcompact(messages, task21IdleConfig(1))
	cached := CachedMicrocompact(messages, task21CachedConfig(), NewCachedMicrocompactState()).Messages
	if got, want := task21DeveloperProjection(uncached), task21DeveloperProjection(messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("uncached developer projection = %#v, want %#v", got, want)
	}
	if got, want := task21DeveloperProjection(cached), task21DeveloperProjection(messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("cached developer projection = %#v, want %#v", got, want)
	}
}

func TestDeveloperContentReplacementIsAnAggregateBarrier(t *testing.T) {
	t.Parallel()

	developer := task21Developer("delta", types.DeveloperMessageKindSkillCatalogDelta, 12)
	messages := []types.Message{
		task21ToolUseMany("a", "b"),
		task21ToolResult("a", strings.Repeat("a", 120_000)),
		developer,
		task21ToolResult("b", strings.Repeat("b", 120_000)),
	}
	calls := 0
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		calls++
		return "replacement-" + toolUseID, nil
	})
	got, records, errs := ApplyToolResultBudget(messages, NewContentReplacementState(), store, nil)
	if len(errs) != 0 || len(records) != 0 || calls != 0 {
		t.Fatalf("developer boundary was crossed: calls=%d records=%#v errors=%v", calls, records, errs)
	}
	if !reflect.DeepEqual(got, messages) || !reflect.DeepEqual(got[2], developer) {
		t.Fatal("content replacement changed messages across the developer boundary")
	}
}

func TestDeveloperContentReplacementHelpersNeverRewriteCatalog(t *testing.T) {
	t.Parallel()

	developer := task21Developer("catalog", types.DeveloperMessageKindSkillCatalogSnapshot, 14)
	developer.Content = append(developer.Content,
		types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "a", Content: "catalog sentinel"},
		types.ContentReplacementBlock{Type: types.ContentTypeReplacement, Kind: "tool-result", ToolUseID: "catalog", Replacement: "catalog replacement"},
	)
	if developer.IsTrustedDeveloperMessage() {
		t.Fatal("mutated developer retained trusted canonical status")
	}
	messages := []types.Message{
		developer,
		task21ToolUseMany("a", "b"),
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "a", Content: strings.Repeat("a", 120_000)},
			types.ToolResultBlock{ToolUseID: "b", Content: strings.Repeat("b", 120_000)},
		),
	}
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		return "replacement-" + toolUseID, nil
	})
	scope := messagecontrol.NewScope("session", "/project", 1)
	replaced, records, errs := ApplyToolResultBudget(messages, NewContentReplacementState(), store, nil)
	if len(errs) != 0 || len(records) != 1 {
		t.Fatalf("records=%#v errors=%v, want one user tool replacement", records, errs)
	}
	if !reflect.DeepEqual(replaced[0], developer) {
		t.Fatalf("developer message was rewritten:\n got: %#v\nwant: %#v", replaced[0], developer)
	}
	if got := StripContentReplacementBlocks(messages); len(got[0].Content) != 2 {
		t.Fatalf("untrusted developer descriptor bypassed ordinary replacement stripping: %#v", got[0])
	}
	if got := contentReplacementRecords([]types.Message{developer}, scope); len(got) != 0 {
		t.Fatalf("developer metadata was treated as replacement state: %#v", got)
	}

	record := ContentReplacementRecord{Kind: "tool-result", ToolUseID: "a", Replacement: "replacement-a"}
	beforeCatalog := []types.Message{types.UserMessage("older user"), developer}
	if got := AppendContentReplacementRecordsForScope(beforeCatalog, []ContentReplacementRecord{record}, messagecontrol.Runtime(), scope); len(contentReplacementRecords(got, scope)) != 1 {
		t.Fatal("untrusted developer descriptor was treated as a replacement boundary")
	}

	afterCatalog := []types.Message{developer, types.UserMessage("current user")}
	appended := AppendContentReplacementRecordsForScope(afterCatalog, []ContentReplacementRecord{record}, messagecontrol.Runtime(), scope)
	if !reflect.DeepEqual(appended[0], developer) {
		t.Fatal("appending a replacement record changed the preceding developer message")
	}
	if got := contentReplacementRecords(appended, scope); !reflect.DeepEqual(got, []ContentReplacementRecord{record}) {
		t.Fatalf("replacement records = %#v, want only the current-user record", got)
	}
	reconstructed := ReconstructContentReplacementStateForScope(appended, scope)
	if got := reconstructed.Replacements["a"]; got != record.Replacement {
		t.Fatalf("reconstructed replacement = %q, want %q", got, record.Replacement)
	}
	if _, ok := reconstructed.Replacements["catalog"]; ok {
		t.Fatal("developer catalog content was reconstructed as replacement state")
	}
}

func task21Developer(text string, kind types.DeveloperMessageKind, revision uint64) types.Message {
	message := types.DeveloperMessage(text, types.DeveloperMessageMetadata{Kind: kind, Revision: revision})
	message.ID = "developer-" + text
	return message.WithInternalControlProvenance(messagecontrol.Runtime())
}

func task21ToolUse(id, name string) types.Message {
	return types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: name, Input: map[string]any{}},
	}}
}

func task21ToolUseMany(ids ...string) types.Message {
	blocks := make([]types.ContentBlock, 0, len(ids))
	for _, id := range ids {
		blocks = append(blocks, types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "Read", Input: map[string]any{}})
	}
	return types.Message{Role: types.RoleAssistant, Content: blocks}
}

func task21ToolResult(id, content string) types.Message {
	return types.ToolResultMessage(types.ToolResultBlock{ToolUseID: id, Content: content})
}

func task21IdleConfig(keepRecent int) MicrocompactConfig {
	return MicrocompactConfig{
		KeepRecent:       keepRecent,
		TimeBasedEnabled: true,
		QuerySource:      MicrocompactSourceMain,
		IdleThreshold:    time.Minute,
		LastActivity:     time.Now().Add(-2 * time.Hour),
	}
}

func task21CachedConfig() MicrocompactConfig {
	return MicrocompactConfig{
		KeepRecent:             1,
		QuerySource:            MicrocompactSourceMain,
		CachedEnabled:          true,
		CachedTriggerThreshold: 2,
		CachedKeepRecent:       1,
	}
}

func task21HasCacheEdits(message types.Message) bool {
	for _, block := range message.Content {
		if unknown, ok := block.(types.UnknownBlock); ok && unknown.Type == ContentTypeCacheEdits {
			return true
		}
	}
	return false
}

func task21ToolResultContent(messages []types.Message, id string) string {
	for _, message := range messages {
		for _, block := range message.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == id {
				return result.TextContent()
			}
		}
	}
	return ""
}

func task21DeveloperProjection(messages []types.Message) []types.Message {
	var result []types.Message
	for _, message := range messages {
		if message.Role == types.RoleDeveloper {
			result = append(result, message)
		}
	}
	return result
}
