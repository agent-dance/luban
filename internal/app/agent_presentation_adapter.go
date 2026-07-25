package app

import (
	"context"

	"github.com/agent-dance/luban/i18n"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

// agentBackgroundPresentationAdapter is the application-owned projection from
// the Agent runtime into the narrower TUI/screen-reader presentation port.
type agentBackgroundPresentationAdapter struct {
	*agentruntime.BackgroundTaskManager
}

var _ tuiBackgroundTaskPresentation = (*agentBackgroundPresentationAdapter)(nil)

func newAgentBackgroundPresentationAdapter(manager *agentruntime.BackgroundTaskManager) *agentBackgroundPresentationAdapter {
	return &agentBackgroundPresentationAdapter{BackgroundTaskManager: manager}
}

func agentBackgroundPresentationPort(manager *agentruntime.BackgroundTaskManager) tuiBackgroundTaskPresentation {
	if manager == nil {
		return nil
	}
	return newAgentBackgroundPresentationAdapter(manager)
}

func (a *agentBackgroundPresentationAdapter) NotificationFollowUpTarget(notification agentcontract.RuntimeNotification) (tuiBackgroundFollowUpTarget, bool) {
	if a == nil || a.BackgroundTaskManager == nil {
		return tuiBackgroundFollowUpTarget{}, false
	}
	target, ok := a.BackgroundTaskManager.NotificationFollowUpTarget(notification)
	if !ok {
		return tuiBackgroundFollowUpTarget{}, false
	}
	return tuiBackgroundFollowUpTarget{
		SessionID:         target.SessionID,
		SessionProjectDir: target.SessionProjectDir,
		ProjectRoot:       target.ProjectRoot,
		Message:           target.Message,
	}, true
}

func (a *agentBackgroundPresentationAdapter) LocalizeRuntimeNotification(lang i18n.Language, notification agentcontract.RuntimeNotification, snapshot agentcontract.TaskSnapshot) agentcontract.RuntimeNotification {
	return agentruntime.LocalizeRuntimeNotification(lang, notification, snapshot)
}

func (a *agentBackgroundPresentationAdapter) SetNotificationConsumers(observer, followUp tuiRuntimeNotificationSink) {
	if a == nil || a.BackgroundTaskManager == nil {
		return
	}
	a.BackgroundTaskManager.SetNotificationConsumers(agentNotificationSink(observer), agentNotificationSink(followUp))
}

func agentNotificationSink(sink tuiRuntimeNotificationSink) agentruntime.RuntimeNotificationSink {
	if sink == nil {
		return nil
	}
	return agentruntime.RuntimeNotificationSinkFunc(func(ctx context.Context, notification agentcontract.RuntimeNotification) error {
		return sink.DeliverRuntimeNotification(ctx, notification)
	})
}
