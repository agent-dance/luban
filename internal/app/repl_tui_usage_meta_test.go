package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	ui "github.com/agent-dance/luban/internal/ui/terminal"
	tuiapp "github.com/agent-dance/luban/internal/ui/tui"
)

func setCompleteTUISessionUsage(state *tuiapp.AppState) session.SessionUsageMeta {
	state.SessionUsageKnown.Set(true)
	state.SessionRoundUsageKnown.Set(true)
	state.SessionTotalInputTokens.Set(321)
	state.SessionTotalOutputTokens.Set(45)
	state.SessionTotalCacheReadTokens.Set(210)
	state.SessionTotalCacheCreateTokens.Set(19)
	state.SessionHasCompacted.Set(true)
	state.SessionCompactionBaselineKnown.Set(true)
	state.SessionCompactionCount.Set(2)
	state.SessionCompletedRoundInputTokens.Set(200)
	state.SessionCompletedRoundOutputTokens.Set(30)
	state.SessionInputTokensAtCompact.Set(250)
	state.SessionCacheReadAtCompact.Set(175)
	state.SessionInputTokens.Set(121)
	state.SessionOutputTokens.Set(15)
	state.SessionCacheReadTokens.Set(35)
	state.SessionCacheCreateTokens.Set(7)
	state.SessionWebSearchRequests.Set(3)
	state.CumulativeCost.Set(1.75)
	state.SessionCostKnown.Set(false)
	state.UsedTokens.Set(64)
	state.MaxTokens.Set(1024)
	return session.SessionUsageMeta{
		InputTokens: 321, OutputTokens: 45, CacheReadTokens: 210, CacheCreateTokens: 19,
		HasCompacted: true, CompactionBaselineKnown: true, RoundUsageKnown: true, CompactionCount: 2,
		CompletedRoundInputTokens: 200, CompletedRoundOutputTokens: 30,
		InputTokensAtCompact: 250, CacheReadAtCompact: 175,
		LastInputTokens: 121, LastOutputTokens: 15, LastCacheReadTokens: 35, LastCacheCreateTokens: 7,
		WebSearchRequests: 3, CumulativeCost: 1.75, CostKnown: false,
		UsedTokens: 64, MaxTokens: 1024,
	}
}

func newTUIUsageMetaFixture(t *testing.T, sessionID string) (*session.Repository, string, *tuiapp.AppState) {
	t.Helper()
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatal(err)
	}
	state := tuiapp.NewAppState()
	state.SessionID.Set(sessionID)
	state.SessionNS.Set(projectDir)
	return repo, projectDir, state
}

func TestTUILifecyclePersistsOneUsageCaptureForResumeAndMetadata(t *testing.T) {
	const sessionID = "tui-usage-meta"
	repo, projectDir, state := newTUIUsageMetaFixture(t, sessionID)
	want := setCompleteTUISessionUsage(state)
	generation := state.SetQueryCancel(func() {})
	activeSessionID := sessionID
	cfg := TUIREPLConfig{
		Engine: screenReaderLifecycleEngine{}, Repo: repo, SessionID: &activeSessionID, SessionProjectDir: &projectDir,
	}
	if err := persistSettledTUISessionLifecycle(cfg, state); err != nil {
		t.Fatal(err)
	}
	if !state.HasActiveQuery() {
		t.Fatal("settled lifecycle save released the active query before persistence completed")
	}
	state.ClearQueryCancel(generation)

	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Usage == nil || !reflect.DeepEqual(*meta.Usage, want) {
		t.Fatalf("persisted usage metadata = %+v, want %+v", meta.Usage, want)
	}

	resumed, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionUsageMetaFromTUIView(resumed.DurableSessionView); !reflect.DeepEqual(*got, want) {
		t.Fatalf("checkpoint usage = %+v, want %+v", *got, want)
	}

	tracker := ui.NewCostTracker("test-model")
	if err := restoreScreenReaderLifecycle(cfg, tracker); err != nil {
		t.Fatal(err)
	}
	restored := tracker.Snapshot()
	if restored.SessionInput != want.InputTokens || restored.SessionOutput != want.OutputTokens ||
		restored.SessionCacheRead != want.CacheReadTokens || restored.SessionCacheCreate != want.CacheCreateTokens ||
		restored.SessionWebSearchRequests != want.WebSearchRequests || restored.SessionCost != want.CumulativeCost || restored.CostKnown != want.CostKnown ||
		!restored.HasCompacted || !restored.CompactionBaselineKnown || restored.InputAtCompact != want.InputTokensAtCompact || restored.CacheReadAtCompact != want.CacheReadAtCompact ||
		!restored.Conversation.Known || restored.Conversation.CompactionCount != want.CompactionCount ||
		restored.Conversation.CompletedInputTokens != want.CompletedRoundInputTokens || restored.Conversation.CompletedOutputTokens != want.CompletedRoundOutputTokens ||
		restored.Conversation.LastInputTokens != want.LastInputTokens || restored.Conversation.LastOutputTokens != want.LastOutputTokens ||
		restored.Conversation.LastCacheReadTokens != want.LastCacheReadTokens || restored.Conversation.LastCacheMakeTokens != want.LastCacheCreateTokens {
		t.Fatalf("screen-reader resume usage = %+v, want metadata %+v", restored, want)
	}
}

func TestTUILifecyclePersistenceReportsEitherPublicationFailure(t *testing.T) {
	t.Run("checkpoint failure leaves metadata at last restorable boundary", func(t *testing.T) {
		const sessionID = "checkpoint-failure"
		repo, projectDir, state := newTUIUsageMetaFixture(t, sessionID)
		if err := repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Usage: &session.SessionUsageMeta{InputTokens: 7}}); err != nil {
			t.Fatal(err)
		}
		state.SessionTotalInputTokens.Set(99)
		viewRoot := filepath.Join(repo.ArtifactsDir(sessionID, projectDir), "tui-view")
		if err := os.WriteFile(viewRoot, []byte("blocked"), 0o600); err != nil {
			t.Fatal(err)
		}
		activeSessionID := sessionID
		cfg := TUIREPLConfig{Repo: repo, SessionID: &activeSessionID, SessionProjectDir: &projectDir}
		if err := persistTUISessionLifecycle(cfg, state); err == nil {
			t.Fatal("checkpoint publication failure was swallowed")
		}
		meta, _, err := repo.GetMeta(sessionID, projectDir)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Usage == nil || meta.Usage.InputTokens != 7 {
			t.Fatalf("checkpoint failure advanced metadata usage: %+v", meta.Usage)
		}
	})

	t.Run("metadata failure retains the exact checkpoint and returns an error", func(t *testing.T) {
		const sessionID = "metadata-failure"
		repo, projectDir, state := newTUIUsageMetaFixture(t, sessionID)
		state.SessionUsageKnown.Set(true)
		state.SessionTotalInputTokens.Set(99)
		metaPath := filepath.Join(projectDir, sessionID+".meta.json")
		if err := os.WriteFile(metaPath, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		activeSessionID := sessionID
		cfg := TUIREPLConfig{Repo: repo, SessionID: &activeSessionID, SessionProjectDir: &projectDir}
		if err := persistTUISessionLifecycle(cfg, state); err == nil {
			t.Fatal("metadata publication failure was swallowed")
		}
		resumed, err := prepareTUISessionSnapshot(cfg, sessionID, projectDir, 2, nil)
		if err != nil {
			t.Fatalf("checkpoint was not retained after metadata failure: %v", err)
		}
		if resumed.Usage.InputTokens != 99 {
			t.Fatalf("retained checkpoint input tokens = %d, want 99", resumed.Usage.InputTokens)
		}
	})
}
