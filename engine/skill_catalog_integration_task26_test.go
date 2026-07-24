package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task26EngineReceiptSkillTool struct {
	skill skills.EffectiveSkill
	body  string
}

func (tool task26EngineReceiptSkillTool) Name() string        { return "Skill" }
func (tool task26EngineReceiptSkillTool) Description() string { return "task26 visible skill receipt" }
func (tool task26EngineReceiptSkillTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}

func (tool task26EngineReceiptSkillTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	execution, ok := loop.ToolExecutionContextFromContext(ctx)
	if !ok {
		return types.ToolResult{}, fmt.Errorf("task26 skill execution context missing")
	}
	ledger, resolved := execution.ResolveSkillLoadedLedger(tool.skill.ID)
	if !resolved || ledger.ContextEpoch == 0 {
		return types.ToolResult{}, fmt.Errorf("task26 skill ledger capability missing")
	}
	envelope, err := skills.RenderFullInvocationEnvelope(tool.skill, tool.body, skills.InvocationArguments{})
	if err != nil {
		return types.ToolResult{}, err
	}
	metadata, err := skills.EncodeSkillExecutionReceiptMetadata(skills.SkillExecutionReceipt{
		ContextEpoch:            ledger.ContextEpoch,
		SkillID:                 tool.skill.ID,
		ContentDigest:           tool.skill.Digest,
		InvocationPayloadDigest: skills.DigestInvocationPayload(tool.body),
		InvocationEnvelopeKind:  skills.InvocationEnvelopeFull,
	})
	if err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: envelope, Metadata: metadata}, nil
}

func TestSkillCatalogIntegrationEngineCompactNeverReusesInvisibleBody(t *testing.T) {
	const sessionID = "task26-engine-compact"
	projectRoot := t.TempDir()
	task26WriteEngineSkill(t, projectRoot, "compact-skill", "compact-skill", "compact body must disappear")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26EngineManager(t, projectRoot, layer)
	row := task26OnlyEngineSkill(t, manager, sessionID)

	provider := &mockProvider{
		name:    "task26-engine",
		modelID: "task26-model",
		responses: [][]types.StreamEvent{
			toolCallEvents("task26-compact-load", "Skill", map[string]any{
				"skill": string(row.ID), "revision": uint64(row.Revision),
			}),
			textEvents("skill loaded"),
			textEvents(`{"schema":"compact-summary/v2","summary":"compact summary"}`),
		},
	}
	reg := registry.New()
	reg.Register(task26EngineReceiptSkillTool{skill: row, body: "compact body must disappear"})
	sessions := newFileSessionManager(t.TempDir())
	engine, err := New(Config{
		Provider: provider, Registry: reg, Sessions: sessions,
		SkillManager: manager, SkillSessionOverrides: layer,
		ProjectRoot: projectRoot, CWD: projectRoot,
		MaxTokens: 1024, MaxTurns: 3, MaxContextTokens: 100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Shutdown(context.Background()) })

	stream, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "load compact skill"})
	if err != nil {
		t.Fatal(err)
	}
	task26AssertEngineQuerySucceeded(t, drainEvents(t, stream, 5*time.Second))
	conv := engine.convs[engine.currentConversationKey(sessionID)]
	if conv == nil {
		t.Fatal("engine did not retain the compact test conversation")
	}
	loaded := conv.ql.SkillLoadedLedgerState(row.ID)
	if loaded.LoadedContextEpoch != loaded.ContextEpoch || loaded.ContentDigest != row.Digest {
		t.Fatalf("pre-compact body was not visibly loaded: %#v", loaded)
	}
	before := conv.ql.SkillCatalogState()
	beforeGeneration, err := engine.ContextGeneration(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	// Revoke before compacting. A compactor is allowed to reattach an exact,
	// still-authorized body, so disabling first makes any retained body proof a
	// genuine stale-authorization bug rather than valid progressive disclosure.
	if _, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: row.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
	}); err != nil {
		t.Fatal(err)
	}
	var compactEvents []loop.Event
	if err := engine.CompactWithEvents(context.Background(), sessionID, "", func(event loop.Event) {
		compactEvents = append(compactEvents, event)
	}); err != nil {
		t.Fatal(err)
	}
	afterGeneration, err := engine.ContextGeneration(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterGeneration != beforeGeneration+1 {
		t.Fatalf("catalog-only compact generation = %d, want %d", afterGeneration, beforeGeneration+1)
	}
	boundaries, terminals := compactCASTestEvents(compactEvents)
	if boundaries != 0 || !reflect.DeepEqual(terminals, []string{"compact_end"}) {
		t.Fatalf("catalog-only compact publication boundaries/terminals = %d/%v, want 0/[compact_end]; events=%+v", boundaries, terminals, compactEvents)
	}
	after := conv.ql.SkillCatalogState()
	if after.ContextEpoch <= before.ContextEpoch || len(after.LoadedDigests) != 0 {
		t.Fatalf("compact reused a revoked body ledger: before=%#v after=%#v", before, after)
	}
	if err := after.Validate(); err != nil {
		t.Fatalf("post-compact runtime state is invalid: %v\n%#v", err, after)
	}
	wantCursorEpoch := loop.SkillCatalogContextEpoch(fmt.Sprintf("context-%d", after.ContextEpoch))
	if after.Cursor.Empty() || after.Cursor.ContextEpoch != wantCursorEpoch {
		t.Fatalf("compact did not bind the current cursor to the new epoch: want=%q state=%#v", wantCursorEpoch, after)
	}
	visibleMessages := compact.GetMessagesAfterCompactBoundary(conv.ql.Messages())
	visibleSnapshot := false
	for _, message := range visibleMessages {
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil &&
			message.DeveloperMetadata.Kind == types.DeveloperMessageKindSkillCatalogSnapshot &&
			message.DeveloperMetadata.Revision == uint64(after.Cursor.AnnouncedRevision()) {
			visibleSnapshot = true
			break
		}
	}
	if !visibleSnapshot {
		t.Fatalf("post-compact current full snapshot is not visible: state=%#v messages=%#v", after, visibleMessages)
	}
	reconstructed := conv.ql.VisibleSkillCatalogState(conv.ql.Messages())
	if reconstructed.ContextEpoch != after.ContextEpoch || !reflect.DeepEqual(reconstructed.Cursor, after.Cursor) ||
		len(reconstructed.LoadedDigests) != len(after.LoadedDigests) {
		t.Fatalf("post-compact state cannot be reconstructed from current visible history\n runtime: %#v\nvisible: %#v", after, reconstructed)
	}
	for id, entry := range after.LoadedDigests {
		if reconstructed.LoadedDigests[id] != entry {
			t.Fatalf("post-compact loaded entry %s is not visible\n runtime: %#v\nvisible: %#v", id, after, reconstructed)
		}
	}
	if strings.Contains(task26EngineMessagesText(conv.ql.Messages()), "compact body must disappear") {
		t.Fatalf("revoked body survived visible compact history: %#v", conv.ql.Messages())
	}

	reenabled, err := manager.ResetVisibility(sessionID, skills.SkillScopeSession, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	reenabledRow, found := reenabled.Find(row.ID)
	if !found {
		t.Fatal("re-enabled skill disappeared from the registry")
	}
	if reenabledRow.Visibility != skills.VisibilityAuto {
		t.Fatalf("re-enabled row = %#v", reenabledRow)
	}
	if state := engine.ResolveSkillLoadedLedger(context.Background(), sessionID, row.ID); state.LoadedContextEpoch != 0 {
		t.Fatalf("post-compact idle resolver reused invisible body: %#v", state)
	}
}

func TestSkillCatalogIntegrationEngineResumeRejectsPersistedInvisibleCursorAndLedger(t *testing.T) {
	const sessionID = "task26-engine-resume"
	projectRoot := t.TempDir()
	task26WriteEngineSkill(t, projectRoot, "resume-skill", "resume-skill", "resume body must be proven visible")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26EngineManager(t, projectRoot, layer)
	row := task26OnlyEngineSkill(t, manager, sessionID)
	sessions := newFileSessionManager(t.TempDir())

	firstProvider := &mockProvider{
		name:    "task26-engine-first",
		modelID: "task26-model",
		responses: [][]types.StreamEvent{
			toolCallEvents("task26-resume-load", "Skill", map[string]any{
				"skill": string(row.ID), "revision": uint64(row.Revision),
			}),
			textEvents("skill loaded before save"),
		},
	}
	reg := registry.New()
	reg.Register(task26EngineReceiptSkillTool{skill: row, body: "resume body must be proven visible"})
	firstEngine, err := New(Config{
		Provider: firstProvider, Registry: reg, Sessions: sessions,
		SkillManager: manager, SkillSessionOverrides: layer,
		ProjectRoot: projectRoot, CWD: projectRoot, MaxTokens: 1024, MaxTurns: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := firstEngine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "load before resume"})
	if err != nil {
		t.Fatal(err)
	}
	task26AssertEngineQuerySucceeded(t, drainEvents(t, stream, 5*time.Second))
	firstConv := firstEngine.convs[firstEngine.currentConversationKey(sessionID)]
	firstState := firstConv.ql.SkillCatalogState()
	if firstState.Cursor.Empty() || len(firstState.LoadedDigests) != 1 {
		t.Fatalf("test precondition did not persist a visible cursor/body: %#v", firstState)
	}
	if err := firstEngine.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Keep the skills sidecar produced by the engine, but replace the model
	// history with text that proves neither its catalog cursor nor its loaded
	// body. Resume must reconcile against messages, not trust the sidecar.
	if err := sessions.Save(sessionID, []types.Message{types.UserMessage("replacement history without catalog or body")}); err != nil {
		t.Fatal(err)
	}
	meta, err := sessions.store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Skills == nil || meta.Skills.ContextEpoch == 0 || len(meta.Skills.LoadedDigests) != 1 {
		t.Fatalf("test did not retain the persisted stale sidecar: %#v", meta.Skills)
	}

	resumedProvider := &mockProvider{
		name: "task26-engine-resumed", modelID: "task26-model",
		responses: [][]types.StreamEvent{textEvents("resumed answer")},
	}
	resumedEngine, err := New(Config{
		Provider: resumedProvider, Sessions: sessions,
		SkillManager: manager, SkillSessionOverrides: layer,
		ProjectRoot: projectRoot, CWD: projectRoot, MaxTokens: 1024, MaxTurns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resumedEngine.Shutdown(context.Background()) })
	if _, err := resumedEngine.Resume(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	resumedConv := resumedEngine.convs[resumedEngine.currentConversationKey(sessionID)]
	resumedState := resumedConv.ql.SkillCatalogState()
	if !resumedState.Cursor.Empty() || len(resumedState.LoadedDigests) != 0 {
		t.Fatalf("resume trusted a cursor/body absent from visible messages: %#v", resumedState)
	}
	if loaded := resumedConv.ql.SkillLoadedLedgerState(row.ID); loaded.LoadedContextEpoch != 0 {
		t.Fatalf("resume exposed an invisible loaded body: %#v", loaded)
	}

	stream, err = resumedEngine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "current user after resume"})
	if err != nil {
		t.Fatal(err)
	}
	task26AssertEngineQuerySucceeded(t, drainEvents(t, stream, 5*time.Second))
	messages := resumedProvider.lastParams.Messages
	if len(messages) < 3 {
		t.Fatalf("resumed provider messages = %#v", messages)
	}
	last := len(messages) - 1
	if messages[last].Role != types.RoleUser || messages[last].GetText() != "current user after resume" {
		t.Fatalf("resumed current user tail = %#v", messages[last])
	}
	catalog := messages[last-1]
	if catalog.Role != types.RoleDeveloper || catalog.DeveloperMetadata == nil || catalog.DeveloperMetadata.Kind != types.DeveloperMessageKindSkillCatalogSnapshot {
		t.Fatalf("resume did not rebuild a full catalog before the current user: %#v", messages)
	}
}

func task26EngineManager(t *testing.T, projectRoot string, layer *skills.MemorySessionOverrideLayer) *skills.Manager {
	t.Helper()
	settings := t.TempDir()
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(settings, "user", "settings.json"),
		ProjectSettings: filepath.Join(settings, "project", "settings.json"),
	}, nil, layer)
	if err != nil {
		t.Fatal(err)
	}
	return skills.NewManagerWithOverrideStore(store, skills.DirSource{Dir: projectRoot, Source: skills.SourceProject})
}

func task26WriteEngineSkill(t *testing.T, root, directory, name, body string) {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: task26 %s\n---\n# %s\n\n%s\n", name, name, name, body)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task26OnlyEngineSkill(t *testing.T, manager *skills.Manager, sessionID string) skills.EffectiveSkill {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("engine fixture catalog = %#v", snapshot.Skills)
	}
	return snapshot.Skills[0]
}

func task26AssertEngineQuerySucceeded(t *testing.T, events []Event) {
	t.Helper()
	for _, event := range events {
		if event.Final {
			if event.Error != nil {
				t.Fatalf("query failed: %v", event.Error)
			}
			return
		}
	}
	t.Fatal("query stream ended without a final event")
}

func task26EngineMessagesText(messages []types.Message) string {
	encoded, _ := json.Marshal(messages)
	return string(encoded)
}
