package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillCatalogWiringRoutesImmutableExecutionContextAndFailsClosed(t *testing.T) {
	skill, envelope := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "visible body")
	provider := &mockProvider{name: "mock", modelID: "mock-model"}
	engine, err := New(Config{Provider: provider, Sessions: newMemorySessionManager()})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-routing"
	key := newConversationKey("", sessionID)
	conv := engine.newConvWithRuntime(sessionID, "", engine.defaultRuntimeContext(), "")
	invocation := types.UserMessage(envelope)
	invocation.InternalKind = types.InternalMessageKindSkillInvocation
	var sealed bool
	invocation, sealed = conv.ql.SealRuntimeControlMessage(messagecontrol.Runtime(), invocation)
	if !sealed {
		t.Fatal("seal exact-scope task23 invocation")
	}
	messages := []types.Message{invocation}
	if err := restoreVisibleSkillState(conv.ql, messages, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	engine.convs[key] = conv

	ui := engine.ResolveSkillLoadedLedger(context.Background(), sessionID, skill.ID)
	if ui.LoadedContextEpoch != ui.ContextEpoch || ui.ContentDigest != skill.Digest {
		t.Fatalf("idle UI ledger = %#v", ui)
	}

	conv.mu.Lock()
	conv.running = make(chan struct{})
	conv.mu.Unlock()
	if got := engine.ResolveSkillLoadedLedger(context.Background(), sessionID, skill.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("running UI call read mutable q.messages: %#v", got)
	}

	modelCtx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID: sessionID,
		Messages:  messages,
	})
	model := engine.ResolveSkillLoadedLedger(modelCtx, sessionID, skill.ID)
	if model.ContextEpoch != 0 {
		t.Fatalf("unowned model context bypassed execution capability: %#v", model)
	}
	if got := engine.ResolveSkillLoadedLedger(modelCtx, "other-session", skill.ID); got.ContextEpoch != 0 {
		t.Fatalf("request/execution mismatch did not fail closed: %#v", got)
	}

	engine.convs[newConversationKey("another-project", sessionID)] = engine.newConvWithRuntime(sessionID, "", engine.defaultRuntimeContext(), "another-project")
	if got := engine.ResolveSkillLoadedLedger(modelCtx, sessionID, skill.ID); got.ContextEpoch != 0 {
		t.Fatalf("duplicate project/session identity did not fail closed: %#v", got)
	}
}

func TestSkillRegistrySessionResumeRestoresOverridesAndClearsUnprovenLedger(t *testing.T) {
	dir := t.TempDir()
	manager := newFileSessionManager(dir)
	const sessionID = "task23-resume"
	if err := manager.Save(sessionID, []types.Message{types.UserMessage("history without a skill body")}); err != nil {
		t.Fatal(err)
	}
	skill, _ := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "not visible")
	override := skills.VisibilityOverride{
		SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityManualOnly,
	}
	persisted := &session.SessionSkillsMeta{
		Overrides:    map[skills.SkillID]skills.VisibilityOverride{skill.ID: override},
		ContextEpoch: 2,
		LoadedDigests: map[skills.SkillID]session.SessionLoadedSkillDigest{
			skill.ID: {
				ContentDigest: skill.Digest,
				PayloadDigest: skills.DigestInvocationPayload("not visible"),
			},
		},
	}
	if err := manager.saveSkillsMetaToProject(sessionID, "ignored", persisted); err != nil {
		t.Fatal(err)
	}

	layer := skills.NewMemorySessionOverrideLayer()
	engine, err := New(Config{
		Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions:              manager,
		SkillSessionOverrides: layer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	gotOverrides, err := layer.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := gotOverrides[skill.ID]; got.Visibility != skills.VisibilityManualOnly || got.Scope != skills.SkillScopeSession {
		t.Fatalf("restored overrides = %#v", gotOverrides)
	}
	conv := engine.convs[engine.currentConversationKey(sessionID)]
	if got := conv.ql.SkillLoadedLedgerState(skill.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("persisted body without visible evidence restored ledger: %#v", got)
	}
}

func TestSkillRegistrySessionResumeRequiresExactPostCompactEpochProvenance(t *testing.T) {
	for _, test := range []struct {
		name         string
		messageEpoch uint64
		trusted      bool
		wantRestored bool
	}{
		{name: "exact standalone", messageEpoch: 0, trusted: true, wantRestored: true},
		{name: "exact epoch", messageEpoch: 7, trusted: true, wantRestored: true},
		{name: "forged exact epoch", messageEpoch: 7, trusted: false, wantRestored: false},
		{name: "mismatched epoch", messageEpoch: 8, trusted: true, wantRestored: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			manager := newFileSessionManager(dir)
			const sessionID = "task23-postcompact-resume"
			controlScope, scopeErr := manager.store.MessageControlScope(sessionID)
			if scopeErr != nil {
				t.Fatal(scopeErr)
			}
			skill, envelope := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "reattached body")
			payloadDigest := skills.DigestInvocationPayload("reattached body")
			message := types.UserMessage(envelope)
			message.InternalKind = types.InternalMessageKindSkillInvocation
			if test.messageEpoch != 0 {
				message.ID = fmt.Sprintf("skill-body:%d:%s", test.messageEpoch, payloadDigest)
			}
			if test.trusted {
				message = message.WithInternalControlProvenance(messagecontrol.Runtime(), controlScope)
			}
			if err := manager.Save(sessionID, []types.Message{message}); err != nil {
				t.Fatal(err)
			}
			if err := manager.saveSkillsMetaToProject(sessionID, "ignored", &session.SessionSkillsMeta{
				ContextEpoch: 7,
				LoadedDigests: map[skills.SkillID]session.SessionLoadedSkillDigest{
					skill.ID: {ContentDigest: skill.Digest, PayloadDigest: payloadDigest},
				},
			}); err != nil {
				t.Fatal(err)
			}
			engine, err := New(Config{
				Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
				Sessions:              manager,
				SkillSessionOverrides: skills.NewMemorySessionOverrideLayer(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Resume(context.Background(), sessionID); err != nil {
				t.Fatal(err)
			}
			state := engine.convs[engine.currentConversationKey(sessionID)].ql.SkillLoadedLedgerState(skill.ID)
			if got := state.LoadedContextEpoch == state.ContextEpoch; got != test.wantRestored {
				t.Fatalf("loaded state = %#v, restored=%t want %t", state, got, test.wantRestored)
			}
		})
	}
}

func TestSkillRegistrySessionResumeRollsBackOverrideWhenConversationIsActive(t *testing.T) {
	dir := t.TempDir()
	manager := newFileSessionManager(dir)
	const sessionID = "task23-rollback"
	if err := manager.Save(sessionID, []types.Message{types.UserMessage("persisted")}); err != nil {
		t.Fatal(err)
	}
	skill, _ := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "body")
	oldOverride := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly}
	newOverride := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityManualOnly}
	if err := manager.saveSkillsMetaToProject(sessionID, "ignored", &session.SessionSkillsMeta{
		Overrides: map[skills.SkillID]skills.VisibilityOverride{skill.ID: newOverride},
	}); err != nil {
		t.Fatal(err)
	}
	layer := skills.NewMemorySessionOverrideLayer()
	if err := layer.ReplaceSession(sessionID, map[skills.SkillID]skills.VisibilityOverride{skill.ID: oldOverride}); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Config{
		Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions:              manager,
		SkillSessionOverrides: layer,
	})
	if err != nil {
		t.Fatal(err)
	}
	conv := engine.newConvWithRuntime(sessionID, "", engine.defaultRuntimeContext(), "")
	conv.ql.SetMessages([]types.Message{types.UserMessage("live conversation A")})
	conv.running = make(chan struct{})
	engine.convs[engine.currentConversationKey(sessionID)] = conv

	if _, err := engine.Resume(context.Background(), sessionID); err == nil {
		t.Fatal("Resume succeeded for an active conversation")
	}
	got, err := layer.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if restored := got[skill.ID]; restored.Visibility != oldOverride.Visibility {
		t.Fatalf("failed resume left replacement overlay behind: %#v", got)
	}
	if messages := conv.ql.Messages(); len(messages) != 1 || messages[0].GetText() != "live conversation A" {
		t.Fatalf("failed resume installed detached visible state: %#v", messages)
	}

	prepared, err := engine.PrepareRuntimeContextResume(context.Background(), sessionID, "", RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Commit(); err == nil {
		t.Fatal("prepared runtime resume replaced an active conversation")
	}
	got, err = layer.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if restored := got[skill.ID]; restored.Visibility != oldOverride.Visibility {
		t.Fatalf("failed prepared resume left replacement overlay behind: %#v", got)
	}
	if messages := conv.ql.Messages(); len(messages) != 1 || messages[0].GetText() != "live conversation A" {
		t.Fatalf("failed prepared commit replaced live visible state: %#v", messages)
	}
}

func TestSkillCatalogMetadataSaveUsesConversationProjectNotMutableCurrentProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "task23-project-meta"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("origin")}); err != nil {
		t.Fatal(err)
	}
	currentProject := projectA
	layer := skills.NewMemorySessionOverrideLayer()
	skill, _ := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "body")
	override := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly}
	if err := layer.ReplaceSession(sessionID, map[skills.SkillID]skills.VisibilityOverride{skill.ID: override}); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Config{
		Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions:              newRepositorySessionManager(repo, func() string { return currentProject }),
		SkillSessionOverrides: layer,
	})
	if err != nil {
		t.Fatal(err)
	}
	conv := engine.newConvWithRuntime(sessionID, "", RuntimeContext{CWD: rootA}, projectA)
	if err := engine.installConversationControlScope(sessionID, conv); err != nil {
		t.Fatal(err)
	}
	conv.ql.SetMessages([]types.Message{types.UserMessage("origin")})
	currentProject = projectB
	if err := engine.saveConversation(sessionID, conv); err != nil {
		t.Fatal(err)
	}
	meta, _, err := repo.GetMeta(sessionID, projectA)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Skills == nil || meta.Skills.Overrides[skill.ID].Visibility != skills.VisibilityNameOnly {
		t.Fatalf("origin-project skills metadata = %#v", meta.Skills)
	}
	// Repository.GetMeta intentionally falls back to global ID resolution when
	// the requested project has no match, so inspect the exact project store.
	if _, err := repo.StoreForProjectDir(projectB).GetMeta(sessionID); err == nil {
		t.Fatal("skills metadata followed mutable current project")
	}
}

func TestSkillRegistryExplicitProjectResumeNeverFallsBackAcrossProjects(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "task23-exact-project-resume"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("project A only")}); err != nil {
		t.Fatal(err)
	}
	skill, _ := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "body")
	aOverride := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityManualOnly}
	if err := repo.SaveMeta(sessionID, projectA, session.SessionMeta{Skills: &session.SessionSkillsMeta{
		Overrides: map[skills.SkillID]skills.VisibilityOverride{skill.ID: aOverride},
	}}); err != nil {
		t.Fatal(err)
	}

	currentProject := projectB
	manager := newRepositorySessionManager(repo, func() string { return currentProject })
	if _, err := manager.loadToolUseLedgerFromProject(sessionID, projectB); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("project B tool ledger error = %v, want ErrSessionNotFound", err)
	}
	if _, err := manager.loadSkillsMetaFromProject(sessionID, projectB); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("project B skills metadata error = %v, want ErrSessionNotFound", err)
	}

	layer := skills.NewMemorySessionOverrideLayer()
	oldOverride := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly}
	if err := layer.ReplaceSession(sessionID, map[skills.SkillID]skills.VisibilityOverride{skill.ID: oldOverride}); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Config{
		Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions:              manager,
		SkillSessionOverrides: layer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.PrepareRuntimeContextResume(context.Background(), sessionID, projectB, RuntimeContext{CWD: rootB}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("project B prepare error = %v, want ErrSessionNotFound", err)
	}
	if _, published := engine.convs[newConversationKey(projectB, sessionID)]; published {
		t.Fatal("failed project B prepare published a conversation")
	}
	unchanged, err := layer.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := unchanged[skill.ID]; got.Visibility != oldOverride.Visibility {
		t.Fatalf("failed project B prepare changed overlay: %#v", unchanged)
	}
	if _, err := repo.StoreForProjectDir(projectB).Load(sessionID); err == nil {
		t.Fatal("failed project B prepare created history")
	}

	currentProject = projectA
	if _, err := engine.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("resume project A: %v", err)
	}
	conv := engine.convs[newConversationKey(projectA, sessionID)]
	if conv == nil || len(conv.ql.Messages()) != 1 || conv.ql.Messages()[0].GetText() != "project A only" {
		t.Fatalf("project A history = %#v", conv)
	}
	restored, err := layer.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored[skill.ID]; got.Visibility != aOverride.Visibility {
		t.Fatalf("project A overlay = %#v", restored)
	}
}

func TestProjectScopedGoalLoadDoesNotFallBackToAnotherProject(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	const sessionID = "task23-exact-project-goal"
	if err := repo.Save(sessionID, projectA, []types.Message{types.UserMessage("project A")}); err != nil {
		t.Fatal(err)
	}
	projectAGoal, err := goal.CreateWithCriteria("must stay in project A", []string{"must stay in project A"}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(sessionID, projectA, session.SessionMeta{Goal: &projectAGoal}); err != nil {
		t.Fatal(err)
	}
	manager := newRepositorySessionManager(repo, func() string { return projectB })
	loaded, err := manager.loadGoalFromProject(sessionID, projectB)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Fatalf("project B loaded project A goal: %#v", loaded)
	}
	provider := &mockProvider{name: "mock", modelID: "mock-model"}
	engine, err := New(Config{Provider: provider, Sessions: manager})
	if err != nil {
		t.Fatal(err)
	}
	events, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "project B", CWD: rootB, ProjectRoot: rootB})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	provider.mu.Lock()
	callCount := provider.callCount
	provider.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("project B query inherited a goal continuation: provider calls=%d", callCount)
	}
}

func TestSkillRegistryQueryRejectsSecondProjectForBareSessionID(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	engine, err := New(Config{
		Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions:              newRepositorySessionManager(repo, func() string { return projectA }),
		SkillSessionOverrides: skills.NewMemorySessionOverrideLayer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-query-project-collision"
	first, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "project A", CWD: rootA, ProjectRoot: rootA})
	if err != nil {
		t.Fatal(err)
	}
	for range first {
	}
	if _, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "project B", CWD: rootB, ProjectRoot: rootB}); err == nil {
		t.Fatal("second project created a conversation for the same bare session ID")
	}
	if _, exists := engine.convs[newConversationKey(projectB, sessionID)]; exists {
		t.Fatal("rejected project B query published a conversation")
	}

	again, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "project A continues", CWD: rootA, ProjectRoot: rootA})
	if err != nil {
		t.Fatalf("project A could not continue: %v", err)
	}
	for range again {
	}
	if _, exists := engine.convs[newConversationKey(projectA, sessionID)]; !exists {
		t.Fatal("project A conversation disappeared after collision rejection")
	}
}

func TestProjectScopedQueryKeepsDuplicateSessionsIndependentWithoutSkillsState(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	rootA := t.TempDir()
	rootB := t.TempDir()
	projectA := repo.ProjectDirForCWD(rootA)
	projectB := repo.ProjectDirForCWD(rootB)
	engine, err := New(Config{
		Provider: &mockProvider{name: "mock", modelID: "mock-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return projectA }),
	})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-project-duplicate"
	for _, request := range []QueryRequest{
		{SessionID: sessionID, Message: "project A history", CWD: rootA, ProjectRoot: rootA},
		{SessionID: sessionID, Message: "project B history", CWD: rootB, ProjectRoot: rootB},
	} {
		events, queryErr := engine.Query(context.Background(), request)
		if queryErr != nil {
			t.Fatalf("project-scoped query: %v", queryErr)
		}
		for range events {
		}
	}
	if engine.convs[newConversationKey(projectA, sessionID)] == nil || engine.convs[newConversationKey(projectB, sessionID)] == nil {
		t.Fatalf("duplicate project conversations were not independently created: %#v", engine.convs)
	}
	messagesA, err := repo.StoreForProjectDir(projectA).Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	messagesB, err := repo.StoreForProjectDir(projectB).Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messagesA) == 0 || messagesA[0].GetText() != "project A history" {
		t.Fatalf("project A routing = %#v", messagesA)
	}
	if len(messagesB) == 0 || messagesB[0].GetText() != "project B history" {
		t.Fatalf("project B routing = %#v", messagesB)
	}
}

func TestSkillRegistryConcurrentCrossProjectResumeCommitsOneOverlayWinner(t *testing.T) {
	for _, mode := range []string{"prepared", "resume-with-runtime"} {
		t.Run(mode, func(t *testing.T) {
			repo := session.NewRepository(t.TempDir())
			rootA := t.TempDir()
			rootB := t.TempDir()
			projectA := repo.ProjectDirForCWD(rootA)
			projectB := repo.ProjectDirForCWD(rootB)
			const sessionID = "task23-concurrent-project-resume"
			skill, _ := task23InvocationEnvelope(t, filepath.Join(t.TempDir(), "SKILL.md"), "body")
			overrideA := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly}
			overrideB := skills.VisibilityOverride{SkillID: skill.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityManualOnly}
			for _, target := range []struct {
				project  string
				override skills.VisibilityOverride
			}{{projectA, overrideA}, {projectB, overrideB}} {
				if err := repo.Save(sessionID, target.project, []types.Message{types.UserMessage(target.project)}); err != nil {
					t.Fatal(err)
				}
				if err := repo.SaveMeta(sessionID, target.project, session.SessionMeta{Skills: &session.SessionSkillsMeta{
					Overrides: map[skills.SkillID]skills.VisibilityOverride{skill.ID: target.override},
				}}); err != nil {
					t.Fatal(err)
				}
			}
			layer := skills.NewMemorySessionOverrideLayer()
			engine, err := New(Config{
				Provider:              &mockProvider{name: "mock", modelID: "mock-model"},
				Sessions:              newRepositorySessionManager(repo, func() string { return projectA }),
				SkillSessionOverrides: layer,
			})
			if err != nil {
				t.Fatal(err)
			}

			type resumeTarget struct {
				project string
				root    string
			}
			targets := []resumeTarget{{projectA, rootA}, {projectB, rootB}}
			prepared := make([]PreparedRuntimeContextResume, len(targets))
			if mode == "prepared" {
				for index, target := range targets {
					prepared[index], err = engine.PrepareRuntimeContextResume(context.Background(), sessionID, target.project, RuntimeContext{CWD: target.root})
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			start := make(chan struct{})
			results := make(chan struct {
				project string
				err     error
			}, len(targets))
			var wg sync.WaitGroup
			for index, target := range targets {
				wg.Add(1)
				go func(index int, target resumeTarget) {
					defer wg.Done()
					<-start
					var resumeErr error
					if mode == "prepared" {
						resumeErr = prepared[index].Commit()
					} else {
						_, resumeErr = engine.ResumeWithRuntimeContext(context.Background(), sessionID, target.project, RuntimeContext{CWD: target.root})
					}
					results <- struct {
						project string
						err     error
					}{project: target.project, err: resumeErr}
				}(index, target)
			}
			close(start)
			wg.Wait()
			close(results)

			winner := ""
			successes := 0
			for result := range results {
				if result.err == nil {
					successes++
					winner = result.project
				}
			}
			if successes != 1 {
				t.Fatalf("successful resumes = %d, want 1", successes)
			}
			engine.convsMu.RLock()
			_, winnerPublished := engine.convs[newConversationKey(winner, sessionID)]
			conversationCount := len(engine.convs)
			engine.convsMu.RUnlock()
			if !winnerPublished || conversationCount != 1 {
				t.Fatalf("published conversations = %#v, winner=%q", engine.convs, winner)
			}
			finalOverrides, err := layer.Snapshot(sessionID)
			if err != nil {
				t.Fatal(err)
			}
			want := overrideA.Visibility
			if winner == projectB {
				want = overrideB.Visibility
			}
			if got := finalOverrides[skill.ID].Visibility; got != want {
				t.Fatalf("final overlay = %s, want winner %s overlay %s", got, winner, want)
			}
		})
	}
}

func task23InvocationEnvelope(t *testing.T, rawLocator, body string) (skills.EffectiveSkill, string) {
	t.Helper()
	locator, err := skills.CanonicalSkillLocator(skills.SourceUser, rawLocator)
	if err != nil {
		t.Fatal(err)
	}
	id, err := skills.ComputeSkillID(skills.SourceUser, locator)
	if err != nil {
		t.Fatal(err)
	}
	skill := skills.EffectiveSkill{
		ID: id, Name: "task23", Source: skills.SourceUser, Locator: locator,
		Digest: skills.ComputeSkillDigest(body), Revision: 1,
		Visibility: skills.VisibilityAuto, VisibilitySource: skills.SkillScopeDefault,
		ModelVisible: true, DescriptionVisible: true, UserInvocable: true,
		Executable: true, Mutable: true,
	}
	envelope, err := skills.RenderFullInvocationEnvelope(skill, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	return skill, envelope
}
