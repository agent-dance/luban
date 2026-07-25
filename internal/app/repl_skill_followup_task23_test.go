package app

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"

	agent "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"

	"github.com/agent-dance/luban/i18n"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/provider"
)

type task23FollowUpGateEngine struct {
	replHookEngine

	mu            sync.Mutex
	followUpCalls int
	providerCalls int
}

func (e *task23FollowUpGateEngine) QueryFollowUp(_ context.Context, req engine.QueryRequest) (<-chan engine.Event, error) {
	e.mu.Lock()
	e.followUpCalls++
	e.mu.Unlock()
	ch := make(chan engine.Event, 1)
	ch <- engine.Event{SessionID: req.SessionID, Final: true}
	close(ch)
	return ch, nil
}

func (e *task23FollowUpGateEngine) Provider() provider.Provider {
	e.mu.Lock()
	e.providerCalls++
	e.mu.Unlock()
	return e.replHookEngine.Provider()
}

func (e *task23FollowUpGateEngine) calls() (followUps, providers int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.followUpCalls, e.providerCalls
}

func task23MismatchedFollowUpFixture(t *testing.T) (*agentBackgroundPresentationAdapter, agent.RuntimeNotification) {
	t.Helper()
	originRoot := t.TempDir()
	currentRoot := t.TempDir()
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(originRoot))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)

	notification := agent.RuntimeNotification{
		ID:               "task23-notification",
		Kind:             "task-notification",
		TaskID:           "task23-follow-up",
		SessionID:        "task23-session",
		ProjectRoot:      originRoot,
		FollowUpRequired: true,
	}
	record := runtimestore.RuntimeTaskRecord{
		ID:               notification.TaskID,
		Type:             "local_agent",
		Status:           "completed",
		Result:           "retained background result",
		OwnerSessionID:   notification.SessionID,
		OwnerProjectRoot: originRoot,
		Notification:     &notification,
	}
	if err := runtimestore.NewRuntimeTaskStore(originRoot).Save(record); err != nil {
		t.Fatalf("save follow-up fixture: %v", err)
	}
	manager.SetProjectRoot(currentRoot)
	if filepath.Clean(manager.CurrentProjectRoot()) == filepath.Clean(originRoot) {
		t.Fatal("fixture did not move the manager to a different project")
	}
	return manager, notification
}

func assertTask23FollowUpRemainsPending(t *testing.T, notification agent.RuntimeNotification) {
	t.Helper()
	record, ok := runtimestore.NewRuntimeTaskStore(notification.ProjectRoot).Get(notification.TaskID)
	if !ok || record.Notification == nil {
		t.Fatalf("follow-up record missing after rejection: %#v", record)
	}
	if record.Notification.FollowUpDeliveredAt != nil || record.Notification.DeliveredAt != nil {
		t.Fatalf("rejected follow-up was marked delivered: %#v", record.Notification)
	}
}

func TestTask23TUIBackgroundFollowUpRejectsStaleWorkspaceBeforeEngine(t *testing.T) {
	previousLanguage := i18n.DetectOrLoadLanguage()
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("set runtime language: %v", err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(previousLanguage) })

	manager, notification := task23MismatchedFollowUpFixture(t)
	eng := &task23FollowUpGateEngine{}
	err := runTUIBackgroundFollowUp(context.Background(), TUIREPLConfig{
		Engine: eng, BackgroundTasks: manager, SessionTransitionMu: &sync.Mutex{},
	}, nil, ui.NewCostTracker("test-model"), notification)
	if err == nil || err.Error() != i18n.Text(i18n.LangZH, i18n.KeyREPLErrorFollowUpUnavailable) {
		t.Fatalf("TUI stale-workspace error = %v, want %q", err, i18n.Text(i18n.LangZH, i18n.KeyREPLErrorFollowUpUnavailable))
	}
	if followUps, providers := eng.calls(); followUps != 0 || providers != 0 {
		t.Fatalf("TUI stale follow-up reached engine/provider: follow-ups=%d providers=%d", followUps, providers)
	}
	assertTask23FollowUpRemainsPending(t, notification)
}

func TestTask23ScreenReaderBackgroundFollowUpRejectsStaleWorkspaceBeforeEngine(t *testing.T) {
	previousLanguage := i18n.DetectOrLoadLanguage()
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("set runtime language: %v", err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(previousLanguage) })

	manager, notification := task23MismatchedFollowUpFixture(t)
	eng := &task23FollowUpGateEngine{}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	t.Cleanup(func() { _ = renderer.Close() })

	err := runScreenReaderBackgroundFollowUp(context.Background(), TUIREPLConfig{
		Engine: eng, BackgroundTasks: manager, SessionTransitionMu: &sync.Mutex{},
	}, renderer, ui.NewCostTracker("test-model"), notification)
	if err == nil || err.Error() != i18n.Text(i18n.LangZH, i18n.KeyREPLErrorFollowUpUnavailable) {
		t.Fatalf("screen-reader stale-workspace error = %v, want %q", err, i18n.Text(i18n.LangZH, i18n.KeyREPLErrorFollowUpUnavailable))
	}
	if followUps, providers := eng.calls(); followUps != 0 || providers != 0 {
		t.Fatalf("screen-reader stale follow-up reached engine/provider: follow-ups=%d providers=%d", followUps, providers)
	}
	if output.Len() != 0 {
		t.Fatalf("screen-reader announced a follow-up that never started: %q", output.String())
	}
	assertTask23FollowUpRemainsPending(t, notification)
}

func TestTask23BackgroundFollowUpRequiresNonEmptyProjectIdentities(t *testing.T) {
	manager := newAgentBackgroundPresentationAdapter(agentruntime.NewBackgroundTaskManager(t.TempDir()))
	cleanupBackgroundTaskManager(t, manager.BackgroundTaskManager)
	if backgroundFollowUpProjectMatches(manager, "") {
		t.Fatal("empty target project identity was accepted")
	}
	if backgroundFollowUpProjectMatches(nil, manager.CurrentProjectRoot()) {
		t.Fatal("missing manager project identity was accepted")
	}
}
