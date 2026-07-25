package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

func TestRepositoryForkCreatesIndependentConversationSnapshot(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	source := Ref{ID: "source-session", ProjectDir: projectDir}
	store := repo.StoreForProjectDir(projectDir)
	sourceArtifacts := store.ArtifactsDir(source.ID)
	if err := os.MkdirAll(filepath.Join(sourceArtifacts, "tool-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceResult := filepath.Join(sourceArtifacts, "tool-results", "toolu_1.txt")
	if err := os.WriteFile(sourceResult, []byte("full result"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceArtifacts, "tool-results", "after-fork.txt"), []byte("must not copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	prefixOnlyResult := strings.TrimSuffix(sourceResult, ".txt")
	if err := os.WriteFile(prefixOnlyResult, []byte("must not copy by prefix"), 0o644); err != nil {
		t.Fatal(err)
	}

	assistantTool := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "toolu_1", Name: "Read", Input: map[string]any{"file_path": sourceResult}},
	}}
	toolResult := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "toolu_1", Content: "saved at " + sourceResult,
	})
	toolSearchUse := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-search-1", Name: "ToolSearch", Input: map[string]any{"query": "TaskCreate"}},
	}}
	toolSearchResult := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "tool-search-1",
		ContentBlocks: []types.ContentBlock{
			types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "TaskCreate"},
		},
	})
	messages := []types.Message{
		types.UserMessage("first question"),
		assistantTool,
		toolResult,
		toolSearchUse,
		toolSearchResult,
		types.AssistantMessage("first answer"),
		types.UserMessage("later question"),
		types.AssistantMessage("later answer"),
	}
	if err := repo.Save(source.ID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	activeGoal := goal.Goal{Objective: "do not inherit me", Status: goal.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := repo.SaveMeta(source.ID, projectDir, SessionMeta{
		Title: "source title", CWD: "/workspace", GitBranch: "feature/source",
		Provider: "openai", Model: "gpt-test", Goal: &activeGoal, CacheLineageID: "root-cache-lineage",
		Usage:           &SessionUsageMeta{InputTokens: 99},
		Presentation:    &SessionPresentationMeta{PermissionMode: "plan"},
		SeenToolUseIDs:  []string{"toolu_1", "tool-search-1", "toolu_after_fork"},
		LoadedToolNames: []string{"TaskCreate", "TaskUpdate"},
	}); err != nil {
		t.Fatal(err)
	}

	fork, err := repo.Fork(source, messages[:6])
	if err != nil {
		t.Fatal(err)
	}
	if fork.ID == "" || fork.ID == source.ID || fork.ProjectDir != projectDir {
		t.Fatalf("fork ref = %+v", fork)
	}

	forkedMessages, err := repo.Load(fork)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkedMessages) != 6 || forkedMessages[0].GetText() != "first question" || forkedMessages[5].GetText() != "first answer" {
		t.Fatalf("forked messages = %#v", forkedMessages)
	}
	originalMessages, err := repo.Load(source)
	if err != nil || len(originalMessages) != len(messages) {
		t.Fatalf("source changed: len=%d err=%v", len(originalMessages), err)
	}

	forkMeta, _, err := repo.GetMeta(fork.ID, fork.ProjectDir)
	if err != nil {
		t.Fatal(err)
	}
	if forkMeta.CWD != "/workspace" || forkMeta.GitBranch != "feature/source" || forkMeta.Provider != "openai" || forkMeta.Model != "gpt-test" {
		t.Fatalf("safe metadata was not inherited: %+v", forkMeta)
	}
	if forkMeta.CacheLineageID != "root-cache-lineage" {
		t.Fatalf("fork CacheLineageID = %q, want inherited root lineage", forkMeta.CacheLineageID)
	}
	if forkMeta.Goal != nil || forkMeta.Usage != nil || forkMeta.Presentation != nil {
		t.Fatalf("session-local lifecycle leaked into fork: %+v", forkMeta)
	}
	if len(forkMeta.SeenToolUseIDs) != 2 || forkMeta.SeenToolUseIDs[0] != "tool-search-1" || forkMeta.SeenToolUseIDs[1] != "toolu_1" {
		t.Fatalf("fork ledger = %v, want selected-prefix IDs only", forkMeta.SeenToolUseIDs)
	}
	if len(forkMeta.LoadedToolNames) != 1 || forkMeta.LoadedToolNames[0] != "TaskCreate" {
		t.Fatalf("fork loaded tools = %v, want selected-prefix references only", forkMeta.LoadedToolNames)
	}

	targetArtifacts := store.ArtifactsDir(fork.ID)
	targetResult := filepath.Join(targetArtifacts, "tool-results", "toolu_1.txt")
	if content, readErr := os.ReadFile(targetResult); readErr != nil || string(content) != "full result" {
		t.Fatalf("fork artifact = %q err=%v", content, readErr)
	}
	for path, want := range map[string]os.FileMode{
		targetArtifacts: 0o700,
		filepath.Join(targetArtifacts, "tool-results"): 0o700,
		targetResult: 0o600,
	} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatalf("stat fork artifact %s: %v", path, statErr)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("fork artifact mode %s = %#o, want %#o", path, got, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(targetArtifacts, "tool-results", "after-fork.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("unreferenced post-fork artifact was copied: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(targetArtifacts, "tool-results", filepath.Base(prefixOnlyResult))); !os.IsNotExist(statErr) {
		t.Fatalf("artifact referenced only as another filename's prefix was copied: %v", statErr)
	}
	serializedText := forkedMessages[2].Content[0].(types.ToolResultBlock).Content
	if !strings.Contains(serializedText, targetResult) || strings.Contains(serializedText, sourceResult) {
		t.Fatalf("forked result path = %q, want rewritten target path", serializedText)
	}
	toolInput := forkedMessages[1].Content[0].(types.ToolUseBlock).Input["file_path"]
	if toolInput != targetResult {
		t.Fatalf("forked tool input path = %v, want %s", toolInput, targetResult)
	}
}

func TestRepositoryForkRejectsEmptySnapshot(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	source := Ref{ID: "source-session", ProjectDir: projectDir}
	if err := repo.Save(source.ID, projectDir, []types.Message{types.UserMessage("keep")}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Fork(source, nil); err == nil {
		t.Fatal("empty fork snapshot unexpectedly succeeded")
	}
}

func TestRepositoryForkResealsOnlySourceScopedContentReplacements(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	source := Ref{ID: "replacement-fork-source", ProjectDir: projectDir}
	scope, err := repo.StoreForProjectDir(projectDir).MessageControlScope(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages := replacementHistoryForScopeTest(scope, "fork replacement")
	if err := repo.Save(source.ID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Load(source)
	if err != nil {
		t.Fatal(err)
	}

	fork, err := repo.Fork(source, loaded)
	if err != nil {
		t.Fatal(err)
	}
	forked, err := repo.Load(fork)
	if err != nil {
		t.Fatal(err)
	}
	forkScope, err := repo.StoreForProjectDir(fork.ProjectDir).MessageControlScope(fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacements := compact.ReconstructContentReplacementStateForScope(forked, forkScope).Replacements
	if len(replacements) != 1 || replacements["tool-scope"] != "fork replacement" {
		t.Fatalf("fork replacements=%#v", replacements)
	}
	block := forked[1].Content[1].(types.ContentReplacementBlock)
	scope, bound := block.InternalReplacementProvenanceScope()
	if !bound || scope.SessionID() != fork.ID || scope.ContextGeneration() == 0 {
		t.Fatalf("fork replacement scope=%#v bound=%t", scope, bound)
	}

	untrusted := append([]types.Message(nil), loaded...)
	untrusted[1].Content = append([]types.ContentBlock(nil), loaded[1].Content...)
	untrustedBlock := untrusted[1].Content[1].(types.ContentReplacementBlock)
	untrusted[1].Content[1] = untrustedBlock.WithInternalReplacementProvenance(messagecontrol.Capability{})
	ordinaryFork, err := repo.Fork(source, untrusted)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := repo.Load(ordinaryFork)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryScope, err := repo.StoreForProjectDir(ordinaryFork.ProjectDir).MessageControlScope(ordinaryFork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := compact.ReconstructContentReplacementStateForScope(ordinary, ordinaryScope).Replacements; len(got) != 0 {
		t.Fatalf("fork promoted untrusted replacement: %#v", got)
	}
}
