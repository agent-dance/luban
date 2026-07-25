package compact

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestDeveloperBoundaryKeepsToolPairAtomic(t *testing.T) {
	t.Parallel()

	developer := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogDelta, 3, "catalog delta after tool")
	messages := []types.Message{
		types.UserMessage("run tool"),
		assistantToolUse("task20-tool"),
		developer,
		toolResult("task20-tool"),
		{ID: "assistant-final", Role: types.RoleAssistant, Content: task20TextContent("done")},
	}

	if got := AdjustIndexToPreserveAPIInvariants(messages, 2); got != 1 {
		t.Fatalf("tail boundary split tool pair: got %d, want 1", got)
	}
	if got := AdjustHeadEndToPreserveAPIInvariants(messages, 2); got != 4 {
		t.Fatalf("head boundary split tool pair: got %d, want 4", got)
	}
}

func TestDeveloperBoundaryMovesCutsAroundDeveloperUserPair(t *testing.T) {
	t.Parallel()

	messages := []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("answer"),
		task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogDelta, 4, "catalog delta"),
		types.UserMessage("current user"),
		types.AssistantMessage("current answer"),
	}

	if got := AdjustIndexToPreserveAPIInvariants(messages, 3); got != 2 {
		t.Fatalf("kept-tail cut = %d, want 2 before developer", got)
	}
	if got := AdjustIndexToPreserveAPIInvariants(messages, 2); got != 2 {
		t.Fatalf("developer-start cut = %d, want 2", got)
	}
	if got := AdjustHeadEndToPreserveAPIInvariants(messages, 3); got != 4 {
		t.Fatalf("preserved-head cut = %d, want 4 after developer user", got)
	}
}

func TestDeveloperBoundaryFullCompactionPreservesRoleAndMetadata(t *testing.T) {
	t.Parallel()

	snapshot := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogSnapshot, 1, "catalog snapshot")
	delta := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogDelta, 2, "catalog delta")
	messages := []types.Message{
		snapshot,
		types.UserMessage("first user"),
		types.AssistantMessage("first answer"),
		delta,
		types.UserMessage("current user"),
		types.AssistantMessage("current answer"),
	}

	var summarized []types.Message
	compactor := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, got []types.Message, _ string) (string, error) {
			summarized = append([]types.Message(nil), got...)
			return "summary", nil
		},
	}
	result, err := compactor.Compact(context.Background(), messages, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summarized, messages[:3]) {
		t.Fatalf("summary input crossed developer boundary:\n got: %#v\nwant: %#v", summarized, messages[:3])
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[3:]) {
		t.Fatalf("preserved tail lost developer role/metadata:\n got: %#v\nwant: %#v", result.MessagesToKeep, messages[3:])
	}
	post := BuildPostCompactMessages(result)
	var preservedDeveloper *types.Message
	for i := range post {
		if post[i].Role == types.RoleDeveloper {
			preservedDeveloper = &post[i]
			break
		}
	}
	if preservedDeveloper == nil || !reflect.DeepEqual(*preservedDeveloper, delta) {
		t.Fatalf("post-compact developer message = %#v, want %#v", preservedDeveloper, delta)
	}
}

func TestDeveloperBoundaryFullCompactionNoOpsInsteadOfSplittingInitialCatalog(t *testing.T) {
	t.Parallel()

	snapshot := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogSnapshot, 1, "catalog snapshot")
	messages := []types.Message{
		snapshot,
		types.UserMessage("current user"),
		types.AssistantMessage("current answer"),
	}
	compactor := &SummaryCompactor{
		KeepRecent: 2,
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			t.Fatal("summarizer called after boundary expanded to the start")
			return "", nil
		},
	}

	result, err := compactor.Compact(context.Background(), messages, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages) {
		t.Fatalf("no-op changed initial catalog turn:\n got: %#v\nwant: %#v", result.MessagesToKeep, messages)
	}
}

func TestDeveloperBoundaryPartialCompactionDoesNotSplitPair(t *testing.T) {
	t.Parallel()

	delta := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogDelta, 5, "catalog delta")
	messages := []types.Message{
		types.UserMessage("old"),
		types.AssistantMessage("old answer"),
		delta,
		types.UserMessage("current user"),
		types.AssistantMessage("current answer"),
	}

	var fromInput []types.Message
	from := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, got []types.Message, _ string) (string, error) {
			fromInput = append([]types.Message(nil), got...)
			return "summary", nil
		},
	}
	fromResult, err := from.PartialCompactConversation(context.Background(), messages, 3, PartialCompactDirectionFrom, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromResult.MessagesToKeep, messages[:4]) || !reflect.DeepEqual(fromInput, messages[4:]) {
		t.Fatalf("from boundary split developer/user: keep=%#v summarize=%#v", fromResult.MessagesToKeep, fromInput)
	}

	var upToInput []types.Message
	upTo := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, got []types.Message, _ string) (string, error) {
			upToInput = append([]types.Message(nil), got...)
			return "summary", nil
		},
	}
	upToResult, err := upTo.PartialCompactConversation(context.Background(), messages, 3, PartialCompactDirectionUpTo, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(upToInput, messages[:2]) || !reflect.DeepEqual(upToResult.MessagesToKeep, messages[2:]) {
		t.Fatalf("up_to boundary split developer/user: summarize=%#v keep=%#v", upToInput, upToResult.MessagesToKeep)
	}
}

func TestDeveloperBoundarySummaryInputAndBudgetPreserveMetadata(t *testing.T) {
	t.Parallel()

	developer := task20DeveloperMessage(types.DeveloperMessageKindSkillCatalogSnapshot, 9, "catalog snapshot")
	budgeted := (&ToolResultBudget{MaxCharsPerResult: 1}).Apply([]types.Message{developer})
	if len(budgeted) != 1 || !reflect.DeepEqual(budgeted[0], developer) {
		t.Fatalf("tool-result budget lost developer metadata:\n got: %#v\nwant: %#v", budgeted, developer)
	}
}

func TestUntrustedDeveloperDescriptorIsOrdinaryForInvariantAdjustment(t *testing.T) {
	t.Parallel()

	invalidDeveloper := types.Message{
		Role:    types.RoleDeveloper,
		Content: task20TextContent("missing metadata"),
		IsMeta:  true,
	}
	messages := []types.Message{
		types.UserMessage("old"),
		invalidDeveloper,
		types.AssistantMessage("unsupported follower"),
	}
	ordinary := append([]types.Message(nil), messages...)
	ordinary[1].Role = types.RoleUser
	if got, want := AdjustIndexToPreserveAPIInvariants(messages, 2), AdjustIndexToPreserveAPIInvariants(ordinary, 2); got != want {
		t.Fatalf("untrusted developer adjusted tail %d, ordinary user %d", got, want)
	}
	if got, want := AdjustHeadEndToPreserveAPIInvariants(messages, 1), AdjustHeadEndToPreserveAPIInvariants(ordinary, 1); got != want {
		t.Fatalf("untrusted developer adjusted head %d, ordinary user %d", got, want)
	}
}

func task20DeveloperMessage(kind types.DeveloperMessageKind, revision uint64, text string) types.Message {
	return types.DeveloperMessage(text, types.DeveloperMessageMetadata{Kind: kind, Revision: revision}).WithInternalControlProvenance(messagecontrol.Runtime())
}

func task20TextContent(text string) []types.ContentBlock {
	return []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: strings.TrimSpace(text)}}
}
