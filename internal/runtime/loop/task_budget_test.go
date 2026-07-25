package loop

import (
	"context"
	"fmt"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type taskBudgetCompactor struct {
	preCompactCounts []int
	calls            int
}

func (c *taskBudgetCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*compact.CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "auto")
}

func (c *taskBudgetCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*compact.CompactionResult, error) {
	preCompactCount := 0
	if c.calls < len(c.preCompactCounts) {
		preCompactCount = c.preCompactCounts[c.calls]
	}
	c.calls++
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{
		Trigger:              trigger,
		PreCompactTokenCount: preCompactCount,
	})
	return &compact.CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{types.UserMessage(fmt.Sprintf("summary %d", c.calls))},
		MessagesToKeep:       messages[len(messages)-1:],
		PreCompactTokenCount: preCompactCount,
	}, nil
}

func TestTaskBudgetRequestTotalOnlyBeforeCompaction(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, TaskBudget: 5000})

	if err := ql.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	budget := prov.requests[0].TaskBudget
	if budget == nil || budget.Total != 5000 {
		t.Fatalf("TaskBudget = %#v, want total 5000", budget)
	}
	if budget.Remaining != nil {
		t.Fatalf("TaskBudget.Remaining = %d, want nil before compaction", *budget.Remaining)
	}
}

func TestTaskBudgetRequestIncludesRemainingAfterCompaction(t *testing.T) {
	prov := &aggregateBudgetProvider{}
	ql := New(prov, registry.New(), Config{MaxTurns: 1, MaxContextTokens: 100, TaskBudget: 5000})
	ql.compactor = &taskBudgetCompactor{preCompactCounts: []int{1200}}
	ql.messages = manyUserMessages(30)

	if err := ql.runLoop(context.Background(), func(stream.Event) {}); err != nil {
		t.Fatalf("runLoop: %v", err)
	}
	if len(prov.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(prov.requests))
	}
	budget := prov.requests[0].TaskBudget
	if budget == nil || budget.Total != 5000 {
		t.Fatalf("TaskBudget = %#v, want total 5000", budget)
	}
	if budget.Remaining == nil || *budget.Remaining != 3800 {
		t.Fatalf("TaskBudget.Remaining = %#v, want 3800", budget.Remaining)
	}
}

func TestTaskBudgetRemainingMonotonicAndNonNegativeAcrossCompactions(t *testing.T) {
	ql := New(&aggregateBudgetProvider{}, registry.New(), Config{MaxContextTokens: 100, TaskBudget: 1000})
	ql.compactor = &taskBudgetCompactor{preCompactCounts: []int{300, 900}}
	state := newQueryState(manyUserMessages(30))
	snapshot := newQueryConfigSnapshot(ql.config, nil)

	if _, err := ql.prepareMessagesForQuery(context.Background(), state, 1, snapshot.TaskBudget, false, func(stream.Event) {}); err != nil {
		t.Fatalf("first prepareMessagesForQuery: %v", err)
	}
	first := ql.providerParams(state, snapshot, state.Messages).TaskBudget
	if first == nil || first.Remaining == nil || *first.Remaining != 700 {
		t.Fatalf("first TaskBudget = %#v, want remaining 700", first)
	}

	state.Messages = append(state.Messages, manyUserMessages(30)...)
	if _, err := ql.prepareMessagesForQuery(context.Background(), state, 2, snapshot.TaskBudget, false, func(stream.Event) {}); err != nil {
		t.Fatalf("second prepareMessagesForQuery: %v", err)
	}
	second := ql.providerParams(state, snapshot, state.Messages).TaskBudget
	if second == nil || second.Remaining == nil || *second.Remaining != 0 {
		t.Fatalf("second TaskBudget = %#v, want remaining 0", second)
	}
	if *second.Remaining > *first.Remaining {
		t.Fatalf("remaining increased across compactions: first=%d second=%d", *first.Remaining, *second.Remaining)
	}
}

func TestTaskBudgetUnsetOmitsProviderParam(t *testing.T) {
	q := New(newParityFakeProvider(nil), registry.New(), Config{})
	state := newQueryState([]types.Message{types.UserMessage("hello")})
	params := q.providerParams(state, QueryConfigSnapshot{MaxTokens: 1024}, state.Messages)
	if params.TaskBudget != nil {
		t.Fatalf("TaskBudget = %#v, want nil when unset", params.TaskBudget)
	}
}

var _ provider.Provider = (*aggregateBudgetProvider)(nil)
