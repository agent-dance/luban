package main

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/session"
	tuiapp "github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
)

func TestProvisionalActivityPersistsRestartsAndAcceptsAuthoritativeResult(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "provisional-activity-restart"
	toolUseID := "read-provisional"
	messages := []types.Message{
		types.UserMessage("inspect"),
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: toolUseID, Name: "Read", Input: map[string]any{"file_path": "state.go"}},
		}},
	}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	identity := tuiapp.SessionIdentity{Namespace: projectDir, SessionID: sessionID, Epoch: 1}
	projection, err := tuiapp.ProjectPersistedMessages(identity, messages, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(tuiapp.SessionSnapshot{Identity: identity, Projection: projection}); err != nil {
		t.Fatal(err)
	}
	ctx := tuiapp.ToolEventContext{SessionID: sessionID, TurnID: "turn-1", WorkUnitID: "inspect", ActorID: "assistant", ActorType: "assistant"}
	if err := state.ApplyActivity(tuiapp.ActivityEvent{
		ID: "tool:" + toolUseID, SessionID: sessionID, Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID,
		Actor: tuiapp.ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Kind: tuiapp.ActivityTool, Name: "Read",
		State: tuiapp.ActivityRunning, Outcome: tuiapp.OutcomeRunning,
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyRuntimeError(ctx, toolUseID, "transport disconnected", nil, map[string]any{"attempt": 1}); err != nil {
		t.Fatal(err)
	}
	assertSingleProvisionalFailureWithoutAttention(t, state)

	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir}
	if err := persistTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Activities) != 1 || meta.Activities[0].Version != session.SessionActivityMetaVersionProvisional || !meta.Activities[0].Provisional {
		t.Fatalf("persisted provisional activity = %+v", meta.Activities)
	}

	// Reopen the serialized sidecar in a fresh repository without copying the
	// richer exact view checkpoint. This forces restart through both conversion
	// functions while leaving the source repository internally consistent.
	restartRepo := session.NewRepository(t.TempDir())
	restartProjectDir := restartRepo.ProjectDirForCWD(t.TempDir())
	if err := restartRepo.Save(sessionID, restartProjectDir, messages); err != nil {
		t.Fatal(err)
	}
	if err := restartRepo.SaveMeta(sessionID, restartProjectDir, meta); err != nil {
		t.Fatal(err)
	}
	restartCfg := TUIREPLConfig{Repo: restartRepo, SessionID: &sessionID, SessionProjectDir: &restartProjectDir}
	restartedSnapshot, err := prepareTUISessionSnapshot(restartCfg, sessionID, restartProjectDir, 2, messages)
	if err != nil {
		t.Fatal(err)
	}
	restarted := tuiapp.NewAppState()
	if err := restarted.ApplySessionSnapshot(restartedSnapshot); err != nil {
		t.Fatal(err)
	}
	assertSingleProvisionalFailureWithoutAttention(t, restarted)

	resultCtx := ctx
	resultCtx.Outcome = tuiapp.OutcomeSucceeded
	if err := restarted.ApplyToolResult(resultCtx, types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: "ok", Outcome: types.ToolOutcomeSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	activity, ok := restarted.GetActivity("tool:" + toolUseID)
	if !ok || activity.Provisional || activity.State != tuiapp.ActivityCompleted || activity.Outcome != tuiapp.OutcomeSucceeded {
		t.Fatalf("authoritative result did not correct restored provisional activity: %+v", activity)
	}
	counts := restarted.ActivitySnapshot().Counts
	if counts.Total != 1 || counts.Completed != 1 || counts.Failed != 0 || counts.Unread != 0 {
		t.Fatalf("corrected footer counts = %+v", counts)
	}

	if err := persistTUISessionLifecycle(restartCfg, restarted); err != nil {
		t.Fatal(err)
	}
	correctedMeta, _, err := restartRepo.GetMeta(sessionID, restartProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(correctedMeta.Activities) != 1 || correctedMeta.Activities[0].Provisional {
		t.Fatalf("corrected authority did not persist: %+v", correctedMeta.Activities)
	}
}

func TestLegacyRuntimeErrorActivityMigrationIsConservativeAndReconcilable(t *testing.T) {
	genericZH := i18n.Text(i18n.LangZH, i18n.KeyRuntimeErrorPublicSummary)
	legacy := session.SessionActivityMeta{
		ID: "tool:legacy-read", Kind: string(tuiapp.ActivityTool), Name: "Read",
		State: string(tuiapp.ActivityFailed), Lifecycle: string(tuiapp.ActivityLifecycleFailed), Outcome: tuiapp.OutcomeFailed.String(),
		ProgressMessage: genericZH, JumpTarget: "runtime-error:legacy-event", LastSequence: 4,
	}
	if restored := activityFromSessionMeta(tuiapp.NewMemoryDetailStore(), legacy); !restored.Provisional {
		t.Fatalf("legacy runtime-error activity was not migrated as provisional: %+v", restored)
	}

	for _, test := range []struct {
		name   string
		mutate func(*session.SessionActivityMeta)
	}{
		{name: "real tool observation", mutate: func(meta *session.SessionActivityMeta) { meta.JumpTarget = "tool:session:legacy-read" }},
		{name: "specific failure", mutate: func(meta *session.SessionActivityMeta) { meta.ProgressMessage = "permission denied" }},
		{name: "non-tool activity", mutate: func(meta *session.SessionActivityMeta) { meta.Kind = string(tuiapp.ActivityBackground) }},
		{name: "conflicting lifecycle", mutate: func(meta *session.SessionActivityMeta) { meta.Lifecycle = string(tuiapp.ActivityLifecycleCompleted) }},
		{name: "versioned authority", mutate: func(meta *session.SessionActivityMeta) { meta.Version = session.SessionActivityMetaVersionProvisional }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := legacy
			test.mutate(&candidate)
			if restored := activityFromSessionMeta(tuiapp.NewMemoryDetailStore(), candidate); restored.Provisional {
				t.Fatalf("authoritative legacy failure was downgraded to provisional: %+v", restored)
			}
		})
	}

	unversionedExplicit := legacy
	unversionedExplicit.ProgressMessage = "specific failure"
	unversionedExplicit.Provisional = true
	if restored := activityFromSessionMeta(tuiapp.NewMemoryDetailStore(), unversionedExplicit); !restored.Provisional {
		t.Fatalf("explicit unversioned provisional bit was lost: %+v", restored)
	}

	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "legacy-session"
	messages := []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "legacy-read", Name: "Read", Input: map[string]any{"file_path": "legacy.go"}},
	}}}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Activities: []session.SessionActivityMeta{legacy}}); err != nil {
		t.Fatal(err)
	}
	cfg := TUIREPLConfig{Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir}
	snapshot, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 7, messages)
	if err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	if err := state.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	restored, ok := state.GetActivity("tool:legacy-read")
	if !ok || !restored.Provisional {
		t.Fatalf("legacy sidecar did not restore provisional authority: %+v", restored)
	}
	ctx := tuiapp.ToolEventContext{SessionID: sessionID, TurnID: "turn-legacy", WorkUnitID: "inspect", ActorID: "assistant", ActorType: "assistant", Outcome: tuiapp.OutcomeSucceeded}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "legacy-read", Content: "ok", Outcome: types.ToolOutcomeSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	corrected, ok := state.GetActivity("tool:legacy-read")
	if !ok || corrected.Provisional || corrected.Outcome != tuiapp.OutcomeSucceeded || corrected.State != tuiapp.ActivityCompleted {
		t.Fatalf("legacy provisional activity was not reconcilable: %+v", corrected)
	}
}

func assertSingleProvisionalFailureWithoutAttention(t *testing.T, state *tuiapp.AppState) {
	t.Helper()
	snapshot := state.ActivitySnapshot()
	if len(snapshot.Activities) != 1 || !snapshot.Activities[0].Provisional || snapshot.Activities[0].State != tuiapp.ActivityFailed {
		t.Fatalf("provisional activity snapshot = %+v", snapshot)
	}
	if snapshot.Counts.Total != 1 || snapshot.Counts.Failed != 1 || snapshot.Counts.Unread != 0 || snapshot.Counts.NeedsInput != 0 || snapshot.Counts.ReadyReview != 0 {
		t.Fatalf("provisional footer/attention counts = %+v", snapshot.Counts)
	}
}
