package app

import (
	"fmt"
	"strings"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/ui/terminal"
)

func usageEventIdentity(event stream.Event) (providerName, model string) {
	if event.Metadata == nil {
		return "", ""
	}
	providerName, _ = event.Metadata["provider"].(string)
	model, _ = event.Metadata["model"].(string)
	return providerName, model
}

func usageAccountingIdentity(event stream.Event) string {
	if event.Metadata != nil {
		for _, key := range []string{"usage_id", "request_id"} {
			if value, ok := event.Metadata[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	// Older producers may not carry an explicit ID. Use only durable semantic
	// lineage; if it is absent, return empty so two legitimate anonymous calls
	// are never collapsed merely because their token counts happen to match.
	if event.TurnID == "" && event.TurnCount == 0 && event.WorkUnitID == "" {
		return ""
	}
	kind, trigger, status, disposition := "", "", "", ""
	if event.Metadata != nil {
		kind, _ = event.Metadata["kind"].(string)
		trigger, _ = event.Metadata["trigger"].(string)
		status, _ = event.Metadata["status"].(string)
		disposition, _ = event.Metadata["disposition"].(string)
	}
	return fmt.Sprintf("usage:%s:%s:%d:%s:%s:%s:%s:%s",
		event.Type, event.TurnID, event.TurnCount, event.WorkUnitID,
		kind, trigger, status, disposition)
}

func recordAuxiliaryUsageEvent(tracker *ui.CostTracker, event stream.Event) (costDelta, cumulativeCost float64, recorded bool) {
	if tracker == nil || event.Usage == nil {
		return 0, 0, false
	}
	providerName, model := usageEventIdentity(event)
	before := tracker.TotalCost()
	if !tracker.RecordAuxiliaryUsageOnceForProviderModel(usageAccountingIdentity(event), providerName, model, *event.Usage) {
		return 0, before, false
	}
	after := tracker.TotalCost()
	return after - before, after, true
}

func recordTurnUsageEvent(tracker *ui.CostTracker, event stream.Event, duration time.Duration) bool {
	if tracker == nil || event.Usage == nil {
		return false
	}
	providerName, model := usageEventIdentity(event)
	return tracker.RecordTurnUsageOnceForProviderModel(usageAccountingIdentity(event), providerName, model, *event.Usage, duration)
}

func backgroundNotificationUsageEvent(notification agentcontract.RuntimeNotification) (stream.Event, bool) {
	if notification.Usage == nil {
		return stream.Event{}, false
	}
	usage := *notification.Usage
	return stream.Event{
		Type:       stream.EventProviderUsage,
		Usage:      &usage,
		WorkUnitID: notification.TaskID,
		Metadata: map[string]any{
			"kind":     "background_agent",
			"usage_id": "background_agent:" + notification.TaskID,
			"provider": notification.Provider,
			"model":    notification.Model,
		},
	}, true
}
