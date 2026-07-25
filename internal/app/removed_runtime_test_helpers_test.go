package app

import (
	"context"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func newRegistryTestSkillManager(t testing.TB, cwd string) *skills.Manager {
	t.Helper()
	store := newRegistryTestSkillOverrideStore(t, cwd)
	manager := skills.NewManager()
	manager.SetOverrideStore(store)
	if err := manager.ReplaceProjectSources(cwd); err != nil {
		t.Fatal(err)
	}
	return manager
}

func newRegistryTestSkillOverrideStore(t testing.TB, cwd string) skills.OverrideStore {
	t.Helper()
	store, err := skills.NewFileOverrideStore(cwd, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func saveCanonicalTUISessionCheckpoint(t testing.TB, repo *session.Repository, sessionID, projectDir string, messages []types.Message, epoch uint64, durableView ...tui.DurableSessionView) {
	t.Helper()
	identity := tui.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: epoch}
	scope, err := repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Bound() {
		identity = identity.WithInternalControlScope(messagecontrol.Runtime(), scope)
	}
	projection, err := tui.ProjectPersistedMessages(identity, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	view := tui.DurableSessionView{}
	if len(durableView) > 0 {
		view = durableView[0]
	}
	snapshot := tui.SessionSnapshot{Identity: identity, Projection: projection, DurableSessionView: view}
	if scope.Bound() {
		snapshot.ContextGeneration = scope.ContextGeneration()
		snapshot.ContextGenerationPersisted = true
	}
	state := tui.NewAppState()
	if err := state.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := tui.SaveSessionViewCheckpoint(repo.ArtifactsDir(sessionID, projectDir), state, messages); err != nil {
		t.Fatal(err)
	}
}

func buildConversationForkEntries(messages []types.Message) []tui.ForkEntry {
	return buildConversationForkEntriesInLanguage(messages, i18n.DetectOrLoadLanguage())
}

func presentationActionForFamily(_ tui.CommandFamily, toolName string) string {
	return tui.SemanticToolActionInLanguage(i18n.DetectOrLoadLanguage(), toolName)
}

func releaseTUIQueryAfterUpdates(app tuiActivityApp, generation uint64, tracker *ui.CostTracker) {
	flushTUIUsageUpdates(app, tracker)
	app.State().ClearQueryCancel(generation)
}

func backgroundAgentGroupSummary(snapshots []agentcontract.TaskSnapshot, current agentcontract.TaskSnapshot) string {
	return backgroundAgentGroupSummaryInLanguage(i18n.DetectOrLoadLanguage(), snapshots, current)
}

func backgroundActivityEvent(snapshot agentcontract.TaskSnapshot, sessionID string, epoch uint64, name, actorType, actorID string, detailRefs []tui.DetailRef, jumpTarget string, transcriptState ...bool) tui.ActivityEvent {
	return backgroundActivityEventInLanguage(i18n.DetectOrLoadLanguage(), snapshot, sessionID, epoch, name, actorType, actorID, detailRefs, jumpTarget, transcriptState...)
}

func handleTUIInput(
	ctx context.Context,
	cfg TUIREPLConfig,
	tuiApp *tui.App,
	cmdReg *commands.Registry,
	storeAdapter *sessionStoreAdapter,
	ql *engineQueryLooper,
	tracker *ui.CostTracker,
	inputStr string,
	sigHandler *SignalHandler,
	runTracked ...func(func()) bool,
) {
	handleTUIInputSubmission(ctx, cfg, tuiApp, cmdReg, storeAdapter, ql, tracker, tuiInputSubmission{text: inputStr}, sigHandler, runTracked...)
}

func persistTUISessionLifecycle(cfg TUIREPLConfig, state *tui.AppState) error {
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, state, false, nil)
}

func persistSettledTUISessionLifecycle(cfg TUIREPLConfig, state *tui.AppState) error {
	return persistTUISessionLifecycleAtBoundaryWithUpdate(cfg, state, true, nil)
}

func validateCustomEndpoint(providerName, raw string) error {
	return validateCustomEndpointInLanguage(i18n.DetectOrLoadLanguage(), providerName, raw)
}

func performTUIActivityAction(app tuiActivityApp, stopper tuiActivityStopper, id, action string) (string, error) {
	lang := i18n.DetectOrLoadLanguage()
	if app != nil && app.State() != nil {
		lang = app.State().Language.Get()
	}
	return performTUIActivityActionInLanguage(lang, app, stopper, id, action)
}

func compactionProgressActivity(ctx presentation.ToolEventContext, progress *stream.ProgressEvent) (tui.ActivityEvent, bool) {
	return compactionProgressActivityInLanguage(i18n.DetectOrLoadLanguage(), ctx, progress)
}

func runManualCompactionEvents(ctx context.Context, eng engine.Engine, sessionID, customInstructions string, onEvent func(stream.Event)) error {
	return runManualCompactionEventsInLanguage(ctx, eng, sessionID, customInstructions, i18n.DetectOrLoadLanguage(), onEvent)
}
