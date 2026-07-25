package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type ledgerSideEffectTool struct {
	executions atomic.Int32
}

func (*ledgerSideEffectTool) Name() string        { return "LedgerSideEffect" }
func (*ledgerSideEffectTool) Description() string { return "persistent tool identity ledger probe" }
func (*ledgerSideEffectTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *ledgerSideEffectTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executions.Add(1)
	return types.ToolResult{Content: "executed"}, nil
}
func (*ledgerSideEffectTool) IsConcurrentSafe() bool { return true }

type ledgerPermissionProbe struct {
	checks atomic.Int32
}

func (p *ledgerPermissionProbe) Check(context.Context, permission.PermissionRequest) (permission.PermissionDecision, error) {
	p.checks.Add(1)
	return permission.PermissionAllowOnce, nil
}

func readMetaJSON(t *testing.T, projectDir, sessionID string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectDir, sessionID+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}

func writeRawSeenToolUseIDs(t *testing.T, projectDir, sessionID string, ids []string) {
	t.Helper()
	meta := readMetaJSON(t, projectDir, sessionID)
	meta["seen_tool_use_ids"] = ids
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".meta.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutoSavePersistsCompleteStableToolUseIdentityLedger(t *testing.T) {
	configHome := t.TempDir()
	projectRoot := filepath.Join(t.TempDir(), "project")
	projectDir := session.NewRepository(configHome).ProjectDirForCWD(projectRoot)
	repo := session.NewRepository(configHome)
	manager := newRepositorySessionManager(repo, func() string { return projectDir })
	tool := &ledgerSideEffectTool{}
	reg := registry.New()
	reg.Register(tool)
	provider := &mockProvider{
		name: "ledger-save", modelID: "ledger-save-model",
		responses: [][]types.StreamEvent{
			toolCallEvents("tool-z", "LedgerSideEffect", map[string]any{}),
			toolCallEvents("tool-a", "LedgerSideEffect", map[string]any{}),
			textEvents("done"),
		},
	}
	eng, err := New(Config{Provider: provider, Registry: reg, Sessions: manager, ProjectRoot: projectRoot, CWD: projectRoot, MaxTurns: 4})
	if err != nil {
		t.Fatal(err)
	}

	events, err := eng.Query(context.Background(), QueryRequest{SessionID: "ledger-save", Message: "run", ProjectRoot: projectRoot, CWD: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	drained := drainEvents(t, events, 5*time.Second)
	final := drained[len(drained)-1]
	if final.Error != nil {
		t.Fatal(final.Error)
	}
	if got := tool.executions.Load(); got != 2 {
		t.Fatalf("tool executions = %d, want 2", got)
	}
	if err := eng.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	meta := readMetaJSON(t, projectDir, "ledger-save")
	raw, ok := meta["seen_tool_use_ids"].([]any)
	if !ok {
		t.Fatalf("seen_tool_use_ids missing from auto-save metadata: %#v", meta)
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		got = append(got, value.(string))
	}
	if want := []string{"tool-a", "tool-z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted ledger = %v, want stable sorted %v", got, want)
	}
}

func TestResumeLoadsCompactedToolUseIdentityLedgerBeforeDownstreamSideEffects(t *testing.T) {
	configHome := t.TempDir()
	repo := session.NewRepository(configHome)
	projectRoot := filepath.Join(t.TempDir(), "project")
	projectDir := repo.ProjectDirForCWD(projectRoot)
	const sessionID = "ledger-resume"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("compacted context without the old tool")}); err != nil {
		t.Fatal(err)
	}
	writeRawSeenToolUseIDs(t, projectDir, sessionID, []string{"tool-old"})

	tool := &ledgerSideEffectTool{}
	permission := &ledgerPermissionProbe{}
	reg := registry.New()
	reg.Register(tool)
	hookMarker := filepath.Join(t.TempDir(), "downstream-hook-ran")
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookPostSampling, Command: "touch " + hookMarker, Timeout: 5},
		{Type: hooks.HookPreToolUse, Command: "touch " + hookMarker, Timeout: 5},
	})
	provider := &mockProvider{
		name: "ledger-resume", modelID: "ledger-resume-model",
		responses: [][]types.StreamEvent{toolCallEvents("tool-old", "LedgerSideEffect", map[string]any{})},
	}
	manager := newRepositorySessionManager(repo, func() string { return projectDir })
	eng, err := New(Config{
		Provider: provider, Registry: reg, Sessions: manager, Permission: permission,
		HookRunner: runner, ProjectRoot: projectRoot, CWD: projectRoot, MaxTurns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown(context.Background())
	if _, err := eng.ResumeWithRuntimeContext(context.Background(), sessionID, projectDir, RuntimeContext{
		ProjectRoot: projectRoot, CWD: projectRoot, HookRunner: runner,
	}); err != nil {
		t.Fatalf("ResumeWithRuntimeContext: %v", err)
	}

	events, err := eng.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "continue", ProjectRoot: projectRoot, CWD: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	drained := drainEvents(t, events, 5*time.Second)
	final := drained[len(drained)-1]
	if final.Error == nil || !strings.Contains(final.Error.Error(), "reused_tool_use_id") {
		t.Fatalf("final error = %v, want persisted identity refusal", final.Error)
	}
	if got := tool.executions.Load(); got != 0 {
		t.Fatalf("tool executions = %d, want 0", got)
	}
	if got := permission.checks.Load(); got != 0 {
		t.Fatalf("permission checks = %d, want 0", got)
	}
	if _, err := os.Stat(hookMarker); !os.IsNotExist(err) {
		t.Fatalf("downstream hook ran or marker stat failed: %v", err)
	}
}

func TestCompactedSessionSaveRestartResumeRejectsHistoricalToolUseID(t *testing.T) {
	configHome := t.TempDir()
	repo := session.NewRepository(configHome)
	projectRoot := filepath.Join(t.TempDir(), "project")
	projectDir := repo.ProjectDirForCWD(projectRoot)
	const sessionID = "ledger-compact-restart"

	firstTool := &ledgerSideEffectTool{}
	firstRegistry := registry.New()
	firstRegistry.Register(firstTool)
	first, err := New(Config{
		Provider: &mockProvider{
			name: "ledger-first", modelID: "ledger-first-model",
			responses: [][]types.StreamEvent{
				toolCallEvents("tool-before-compact", "LedgerSideEffect", map[string]any{}),
				textEvents("first done"),
				textEvents("filler 1"),
				textEvents("filler 2"),
				textEvents("filler 3"),
				textEvents("filler 4"),
				textEvents("filler 5"),
				textEvents("filler 6"),
				textEvents("filler 7"),
				textEvents("filler 8"),
				textEvents("filler 9"),
				textEvents("filler 10"),
				textEvents("filler 11"),
				textEvents(`{"schema":"compact-summary/v2","summary":"compacted summary without old tool"}`),
			},
		},
		Registry: firstRegistry, Sessions: newRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: projectRoot, CWD: projectRoot, MaxTurns: 3, MaxContextTokens: 100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := first.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "first", ProjectRoot: projectRoot, CWD: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	drained := drainEvents(t, events, 5*time.Second)
	if final := drained[len(drained)-1]; final.Error != nil {
		t.Fatal(final.Error)
	}
	key := newConversationKey(projectDir, sessionID)
	conv := first.convs[key]
	if conv == nil {
		t.Fatal("first engine did not retain conversation")
	}
	for index := 1; index <= 11; index++ {
		events, err = first.Query(context.Background(), QueryRequest{
			SessionID: sessionID, Message: fmt.Sprintf("filler question %d", index), ProjectRoot: projectRoot, CWD: projectRoot,
		})
		if err != nil {
			t.Fatal(err)
		}
		drained = drainEvents(t, events, 5*time.Second)
		if final := drained[len(drained)-1]; final.Error != nil {
			t.Fatal(final.Error)
		}
	}
	if _, err := first.Compact(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Load(session.Ref{ID: sessionID, ProjectDir: projectDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 || stored[0].InternalKind != types.InternalMessageKindCompactBoundary {
		t.Fatalf("stored transcript has no canonical compact boundary: %#v", stored)
	}
	for _, message := range stored {
		for _, block := range message.Content {
			switch typed := block.(type) {
			case types.ToolUseBlock:
				if typed.ID == "tool-before-compact" {
					t.Fatalf("compacted transcript retained historical tool use: %#v", stored)
				}
			case types.ToolResultBlock:
				if typed.ToolUseID == "tool-before-compact" {
					t.Fatalf("compacted transcript retained historical tool result: %#v", stored)
				}
			}
		}
	}

	secondTool := &ledgerSideEffectTool{}
	secondPermission := &ledgerPermissionProbe{}
	secondRegistry := registry.New()
	secondRegistry.Register(secondTool)
	second, err := New(Config{
		Provider: &mockProvider{
			name: "ledger-second", modelID: "ledger-second-model",
			responses: [][]types.StreamEvent{toolCallEvents("tool-before-compact", "LedgerSideEffect", map[string]any{})},
		},
		Registry: secondRegistry, Sessions: newRepositorySessionManager(repo, func() string { return projectDir }), Permission: secondPermission,
		ProjectRoot: projectRoot, CWD: projectRoot, MaxTurns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Shutdown(context.Background())
	if _, err := second.ResumeWithRuntimeContext(context.Background(), sessionID, projectDir, RuntimeContext{ProjectRoot: projectRoot, CWD: projectRoot}); err != nil {
		t.Fatal(err)
	}
	events, err = second.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "resume", ProjectRoot: projectRoot, CWD: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	drained = drainEvents(t, events, 5*time.Second)
	if final := drained[len(drained)-1]; final.Error == nil || !strings.Contains(final.Error.Error(), "reused_tool_use_id") {
		t.Fatalf("restart reuse error = %v, want persisted refusal", final.Error)
	}
	if secondTool.executions.Load() != 0 || secondPermission.checks.Load() != 0 {
		t.Fatalf("restart reuse reached side effects: tool=%d permission=%d", secondTool.executions.Load(), secondPermission.checks.Load())
	}
}

func TestPrepareResumeFailsClosedOnCorruptIdentityLedgerMetadata(t *testing.T) {
	configHome := t.TempDir()
	repo := session.NewRepository(configHome)
	projectRoot := filepath.Join(t.TempDir(), "project")
	projectDir := repo.ProjectDirForCWD(projectRoot)
	const sessionID = "ledger-corrupt"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("stored")}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".meta.json"), []byte("{broken metadata\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng, err := New(Config{
		Provider: &mockProvider{name: "ledger-corrupt", modelID: "ledger-corrupt-model"},
		Sessions: newRepositorySessionManager(repo, func() string { return projectDir }), ProjectRoot: projectRoot, CWD: projectRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown(context.Background())
	if _, err := eng.PrepareRuntimeContextResume(context.Background(), sessionID, projectDir, RuntimeContext{ProjectRoot: projectRoot, CWD: projectRoot}); err == nil {
		t.Fatal("prepare resume accepted corrupt identity ledger metadata")
	}
	if _, ok := eng.convs[newConversationKey(projectDir, sessionID)]; ok {
		t.Fatal("corrupt metadata published a conversation")
	}
}
