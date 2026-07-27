package loop

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
	"github.com/google/uuid"
)

// NewSystemWarningEvent creates a warning with semantic public copy and
// private diagnostic material.
func NewSystemWarningEvent(publicKey i18n.Key, publicArgs []any, cause error, metadata map[string]any, turnCount int) stream.Event {
	warning := runtimeevent.NewWarningEvent(types.RuntimeIdentity{}, publicKey, publicArgs, cause, metadata)
	return stream.Event{Type: stream.EventSystemWarning, TurnCount: turnCount, RuntimeEvent: &warning}
}

func newMaxTurnsReachedEvent(maxTurns, turnCount int) stream.Event {
	return stream.Event{
		Type:           stream.EventMaxTurnsReached,
		TurnCount:      turnCount,
		TerminalReason: "max_turns_reached",
		MaxTurns:       &stream.MaxTurnsEvent{MaxTurns: maxTurns, TurnCount: turnCount},
		Metadata: map[string]any{
			"max_turns":  maxTurns,
			"turn_count": turnCount,
		},
	}
}

func newAgenticFlightDispositionEvent(controller *agenticFlightController, decision agenticFlightTerminalDecision, turnCount int) stream.Event {
	if controller == nil {
		return stream.Event{}
	}
	return stream.Event{
		Type:      stream.EventProgress,
		TurnCount: turnCount,
		Progress: &stream.ProgressEvent{
			Stage:         "agentic_flight",
			Disposition:   decision.Disposition,
			Blocker:       string(decision.Blocker),
			MutationEpoch: controller.mutationEpoch(),
			VerifiedEpoch: controller.verifiedEpoch(),
		},
	}
}

func newGoalEvaluationEvent(usage *types.Usage, turnCount int, model string) stream.Event {
	usageCopy := *usage
	metadata := map[string]any{
		"kind":     "goal_evaluator",
		"usage_id": "goal_evaluation:" + uuid.NewString(),
	}
	if model = strings.TrimSpace(model); model != "" {
		metadata["model"] = model
	}
	return stream.Event{Type: stream.EventGoalEvaluation, Usage: &usageCopy, TurnCount: turnCount, Metadata: metadata}
}

func newGoalStatusEvent(current *goal.Goal, turnCount int) stream.Event {
	projection := &stream.GoalStatusEvent{}
	if current != nil {
		normalized := goal.Normalize(*current)
		projection.Status = string(normalized.Status)
		projection.Objective = normalized.Objective
		projection.Revision = normalized.Revision
		results := make(map[string]goal.AcceptanceCriterionEvaluation)
		if normalized.LastAcceptanceEvaluation != nil && normalized.LastAcceptanceEvaluation.Revision == normalized.Revision {
			for _, result := range normalized.LastAcceptanceEvaluation.Criteria {
				results[strings.ToUpper(result.CriterionID)] = result
			}
		}
		for _, criterion := range normalized.AcceptanceCriteria {
			item := stream.GoalCriterionStatusEvent{ID: criterion.ID, Text: criterion.Text, Status: "pending"}
			if result, ok := results[strings.ToUpper(criterion.ID)]; ok {
				item.Reason = result.Reason
				item.Status = "unmet"
				if result.Met {
					item.Status = "met"
				}
			}
			projection.Criteria = append(projection.Criteria, item)
		}
	}
	return stream.Event{Type: stream.EventGoalStatus, TurnCount: turnCount, GoalStatus: projection}
}

func newCompactBoundaryEvent(result *compact.CompactionResult, trigger string, turnCount int) stream.Event {
	return newCompactBoundaryEventWithID(result, trigger, turnCount, "")
}

func newCompactBoundaryEventWithID(result *compact.CompactionResult, trigger string, turnCount int, boundaryID string) stream.Event {
	event := stream.Event{
		Type:      stream.EventCompactBoundary,
		TurnCount: turnCount,
		Compact:   &stream.CompactBoundaryEvent{BoundaryID: boundaryID, Trigger: trigger},
	}
	if result != nil {
		event.Compact.PostCompactTokenCount = result.PostCompactTokenCount
		event.Compact.TruePostCompactTokenCount = result.TruePostCompactTokenCount
		event.Compact.Summary = compactMessagesText(result.SummaryMessages)
		event.Compact.UserDisplayMessage = result.UserDisplayMessage
		if result.BoundaryMarker != nil {
			if metadata, ok := compact.ParseCompactBoundaryMessage(*result.BoundaryMarker); ok {
				event.Compact.Trigger = metadata.Trigger
				event.Compact.PreCompactTokenCount = metadata.PreCompactTokenCount
				event.Compact.PreviousTailIdentifier = metadata.PreviousTailIdentifier
				event.Compact.PreCompactDiscoveredTools = append([]string(nil), metadata.PreCompactDiscoveredTools...)
				if metadata.PreservedSegment != nil {
					event.Compact.PreservedSegment = &stream.PreservedSegmentMetadata{
						StartIndex: metadata.PreservedSegment.StartIndex,
						Count:      metadata.PreservedSegment.Count,
						Anchor:     metadata.PreservedSegment.Anchor,
						Direction:  metadata.PreservedSegment.Direction,
					}
				}
			}
		}
	}
	if event.Compact.Trigger == "" {
		event.Compact.Trigger = trigger
	}
	event.Metadata = map[string]any{"trigger": event.Compact.Trigger}
	return event
}

func compactMessagesText(messages []types.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if text := strings.TrimSpace(message.GetText()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}
