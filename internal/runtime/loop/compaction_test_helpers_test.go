package loop

import (
	"context"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
)

func (q *QueryLoop) runCompaction(ctx context.Context, trigger string, turnCount int, onEvent func(stream.Event), run func() (*compact.CompactionResult, error)) (result *compact.CompactionResult, err error) {
	result, _, err = q.runCompactionAgainst(ctx, trigger, turnCount, onEvent, q.messages, run)
	return result, err
}

func (plan SkillCatalogPlan) HasMessage() bool { return plan.Message != nil }
