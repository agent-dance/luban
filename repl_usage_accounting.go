package main

import (
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/ui"
)

func usageEventIdentity(event loop.Event) (providerName, model string) {
	if event.Metadata == nil {
		return "", ""
	}
	providerName, _ = event.Metadata["provider"].(string)
	model, _ = event.Metadata["model"].(string)
	return providerName, model
}

func recordAuxiliaryUsageEvent(tracker *ui.CostTracker, event loop.Event) (costDelta, cumulativeCost float64, recorded bool) {
	if tracker == nil || event.Usage == nil {
		return 0, 0, false
	}
	providerName, model := usageEventIdentity(event)
	before := tracker.TotalCost()
	tracker.RecordAuxiliaryUsageForProviderModel(providerName, model, *event.Usage)
	after := tracker.TotalCost()
	return after - before, after, true
}

func recordTurnUsageEvent(tracker *ui.CostTracker, event loop.Event, duration time.Duration) bool {
	if tracker == nil || event.Usage == nil {
		return false
	}
	providerName, model := usageEventIdentity(event)
	tracker.RecordTurnUsageForProviderModel(providerName, model, *event.Usage, duration)
	return true
}

func backgroundNotificationUsageEvent(notification tools.RuntimeNotification) (loop.Event, bool) {
	if notification.Usage == nil {
		return loop.Event{}, false
	}
	usage := *notification.Usage
	return loop.Event{
		Type:       loop.EventProviderUsage,
		Usage:      &usage,
		WorkUnitID: notification.TaskID,
		Metadata: map[string]any{
			"kind":     "background_agent",
			"provider": notification.Provider,
			"model":    notification.Model,
		},
	}, true
}
