package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillsSurfaceAcceptanceComposedExactREPLAppAndScreenReaderParity(t *testing.T) {
	manager, row, root := task28SurfaceManager(t)
	sessionID := "task28-surface-session"
	app := task28SurfaceTUIApp(t)
	t.Cleanup(func() { _ = app.Close() })
	app.State().SessionID.Set(sessionID)
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	cfg := TUIREPLConfig{
		SessionID:           &sessionID,
		CWD:                 &root,
		SkillManager:        manager,
		SkillsMenuLauncher:  app,
		CommandMu:           &sync.Mutex{},
		SessionTransitionMu: &sync.Mutex{},
	}

	// This is the production exact-command composition: REPL routing invokes
	// the real App launcher, which immediately starts the direct checklist read.
	handleTUIInput(context.Background(), cfg, app, registry, nil, nil, ui.NewCostTracker("task28"), "/skills", nil)
	task28SurfaceWaitApp(t, app, func() bool {
		menu := app.State().SkillsMenu.Get()
		return menu != nil && menu.Toggle.HasSnapshot && !menu.Toggle.Loading
	})
	menu := app.State().SkillsMenu.Get()
	if got, found := menu.Toggle.Snapshot.Find(row.ID); !found || got.Visibility != skills.VisibilityAuto {
		t.Fatalf("exact REPL/App checklist row=%#v found=%t", got, found)
	}
	if len(app.State().Messages.Get()) != 1 || app.State().Messages.Get()[0].Text != "/skills" {
		t.Fatalf("exact /skills REPL projection=%#v", app.State().Messages.Get())
	}

	// Explicit subcommands must stay on the command backend and never open the
	// App checklist. RouteExactSkillsMenu is the production splitter called by
	// handleTUIInput immediately before command-registry dispatch.
	app.State().SkillsMenu.Set(nil)
	routed, routeErr := tui.RouteExactSkillsMenu("/skills list", app, tui.SkillsMenuOpenRequest{
		SessionID: func() string { return sessionID },
		Language:  func() i18n.Language { return app.State().Language.Get() },
		Backend:   manager,
	})
	if routeErr != nil || routed || app.State().SkillsMenu.Get() != nil {
		t.Fatalf("/skills list route handled=%t menu=%#v err=%v", routed, app.State().SkillsMenu.Get(), routeErr)
	}

	// Screen-reader mutation and TUI reread share the exact Manager instance.
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	handled, exit, err := handleScreenReaderCommand(
		context.Background(), cfg, renderer, ui.NewCostTracker("task28"),
		"/skills set "+string(row.ID)+" manual-only --scope session",
	)
	if err != nil || !handled || exit || !strings.Contains(output.String(), string(row.ID)) {
		t.Fatalf("screen-reader mutation handled=%t exit=%t output=%q err=%v", handled, exit, output.String(), err)
	}
	handleTUIInput(context.Background(), cfg, app, registry, nil, nil, ui.NewCostTracker("task28"), "/skills", nil)
	task28SurfaceWaitApp(t, app, func() bool {
		current := app.State().SkillsMenu.Get()
		if current == nil || current.Toggle.Loading || !current.Toggle.HasSnapshot {
			return false
		}
		got, found := current.Toggle.Snapshot.Find(row.ID)
		return found && got.Visibility == skills.VisibilityManualOnly && got.VisibilitySource == skills.SkillScopeSession
	})
	if authoritative := task28SurfaceSnapshot(t, manager, sessionID); authoritative.Revision != app.State().SkillsMenu.Get().Toggle.Snapshot.Revision {
		t.Fatalf("TUI did not reread screen-reader authority: manager=%d tui=%d", authoritative.Revision, app.State().SkillsMenu.Get().Toggle.Snapshot.Revision)
	}
}

func TestSkillsSurfaceAcceptanceRealTUIRegistryTextCommandsAndScreenReaderShow(t *testing.T) {
	manager, row, projectRoot := task28SurfaceManager(t)
	const sessionID = "task28-text-command-session"
	activeSession := sessionID
	activeProject, activeCWD := projectRoot, projectRoot
	repo := session.NewRepository(t.TempDir())
	eng, err := engine.New(engine.Config{
		Provider:     task28SurfaceProvider{},
		Sessions:     engine.NewRepositorySessionManager(repo, func() string { return activeProject }),
		SkillManager: manager,
		ProjectRoot:  projectRoot,
		CWD:          projectRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	app := task28SurfaceTUIApp(t)
	t.Cleanup(func() { _ = app.Close() })
	app.State().SessionID.Set(sessionID)
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	ql := &engineQueryLooper{eng: eng, sessionID: func() string { return activeSession }, model: eng.Provider().ModelID()}
	cfg := TUIREPLConfig{
		Engine: eng, Repo: repo, SessionID: &activeSession,
		SessionProjectDir: &activeProject, CWD: &activeCWD,
		SkillManager: manager, SkillsMenuLauncher: app,
		CommandMu: &sync.Mutex{}, SessionTransitionMu: &sync.Mutex{},
	}
	tracker := ui.NewCostTracker("task28")

	tests := []struct {
		input string
		want  []string
	}{
		{input: "/skills list", want: []string{"Catalog revision:", row.Name, string(row.ID)}},
		{input: "/skills show " + string(row.ID), want: []string{"Skill: " + row.Name, "Visibility: auto", string(row.Locator)}},
		{input: "/skills set " + string(row.ID) + " name-only --scope project", want: []string{"name-only", "project", string(row.ID)}},
	}
	for _, test := range tests {
		t.Run(strings.ReplaceAll(test.input, " ", "_"), func(t *testing.T) {
			app.State().SkillsMenu.Set(nil)
			before := len(app.State().Messages.Get())
			handleTUIInput(context.Background(), cfg, app, registry, nil, ql, tracker, test.input, nil)
			task28SurfaceWaitApp(t, app, func() bool {
				return task28SurfaceContainsAll(task28SurfaceMessagesText(app.State().Messages.Get()[before:]), test.want)
			})
			if app.State().SkillsMenu.Get() != nil {
				t.Fatalf("text command %q opened interactive checklist", test.input)
			}
			text := task28SurfaceMessagesText(app.State().Messages.Get()[before:])
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Errorf("%q TUI output omitted %q: %q", test.input, want, text)
				}
			}
		})
	}

	authoritative := task28SurfaceSnapshot(t, manager, sessionID)
	updated, found := authoritative.Find(row.ID)
	if !found || updated.Visibility != skills.VisibilityNameOnly || updated.VisibilitySource != skills.SkillScopeProject {
		t.Fatalf("real TUI registry set did not mutate shared authority: %#v found=%t", updated, found)
	}

	app.State().SkillsMenu.Set(nil)
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	t.Cleanup(func() { _ = renderer.Close() })
	handled, exit, err := handleScreenReaderCommand(
		context.Background(), cfg, renderer, ui.NewCostTracker("task28"),
		"/skills show "+string(row.ID),
	)
	if err != nil || !handled || exit {
		t.Fatalf("screen-reader show handled=%t exit=%t err=%v output=%q", handled, exit, err, output.String())
	}
	for _, want := range []string{"Skill: " + row.Name, "Visibility: name-only", "State source: project", string(row.ID)} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("screen-reader show omitted %q: %q", want, output.String())
		}
	}
	if app.State().SkillsMenu.Get() != nil {
		t.Fatal("screen-reader show opened TUI checklist")
	}
}

func TestSkillsResumeAcceptanceRealCoreEngineRestoresSidecarSessionOverride(t *testing.T) {
	_, row, projectRoot := task28SurfaceManager(t)
	const sessionID = "task28-engine-resume"
	configHome := t.TempDir()
	repo := session.NewRepository(configHome)
	if err := repo.Save(sessionID, projectRoot, []types.Message{types.UserMessage("persisted visible history")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(sessionID, projectRoot, session.SessionMeta{Skills: &session.SessionSkillsMeta{
		Overrides: map[skills.SkillID]skills.VisibilityOverride{row.ID: {
			SkillID: row.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
		}},
		ContextEpoch: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	layer := skills.NewMemorySessionOverrideLayer()
	// Rebuild the Manager against the same source but a fresh, empty process
	// session layer. Only CoreEngine.Resume may populate it from the sidecar.
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(projectRoot, "resume-settings", "user.json"),
		ProjectSettings: filepath.Join(projectRoot, "resume-settings", "project.json"),
	}, nil, layer)
	if err != nil {
		t.Fatal(err)
	}
	resumeManager := skills.NewManager(skills.DirSource{Dir: filepath.Join(projectRoot, "skills"), Source: skills.SourceProject})
	resumeManager.SetOverrideStore(store)
	before := task28SurfaceSnapshot(t, resumeManager, sessionID)
	if got, _ := before.Find(row.ID); got.Visibility != skills.VisibilityAuto {
		t.Fatalf("fresh process unexpectedly had sidecar override=%#v", got)
	}
	eng, err := engine.New(engine.Config{
		Provider:              task28SurfaceProvider{},
		Sessions:              engine.NewRepositorySessionManager(repo, func() string { return projectRoot }),
		SkillManager:          resumeManager,
		SkillSessionOverrides: layer,
		ProjectRoot:           projectRoot,
		CWD:                   projectRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	count, err := eng.Resume(context.Background(), sessionID)
	if err != nil || count != 1 {
		t.Fatalf("CoreEngine.Resume count=%d err=%v", count, err)
	}
	after := task28SurfaceSnapshot(t, resumeManager, sessionID)
	got, found := after.Find(row.ID)
	if !found || got.Visibility != skills.VisibilityOff || got.VisibilitySource != skills.SkillScopeSession {
		t.Fatalf("CoreEngine.Resume did not restore sidecar override=%#v found=%t", got, found)
	}
}

func TestSkillsCompactAcceptanceRealQueryLoopForceCompactAtomicallyFencesEpochSeven(t *testing.T) {
	manager, row, _ := task28SurfaceManager(t)
	provider := &task28SurfaceBlockingCompactProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	query := loop.New(provider, registry.New(), loop.Config{
		MaxTokens: 1024, MaxTurns: 2, MaxContextTokens: 100_000,
		SessionID: "task28-query-compact", SkillManager: manager,
	})
	messages := make([]types.Message, 0, 26)
	for index := 0; index < 13; index++ {
		messages = append(messages,
			types.UserMessage(fmt.Sprintf("task28 old user %02d", index)),
			types.AssistantMessage(fmt.Sprintf("task28 old assistant %02d", index)),
		)
	}
	query.SetMessages(messages)
	loaded := loop.SkillLoadedLedgerEntry{
		ContentDigest: row.Digest,
		PayloadDigest: skills.DigestInvocationPayload("task28 epoch-seven body"),
	}
	if err := query.SetSkillCatalogState(loop.SkillCatalogRuntimeState{
		ContextEpoch:  7,
		LoadedDigests: map[skills.SkillID]loop.SkillLoadedLedgerEntry{row.ID: loaded},
	}); err != nil {
		t.Fatal(err)
	}

	compactErr := make(chan error, 1)
	go func() {
		_, err := query.ForceCompact(context.Background())
		compactErr <- err
	}()
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("real QueryLoop.ForceCompact did not reach the summary provider")
	}
	blocked := query.SkillCatalogState()
	if blocked.ContextEpoch != 7 || len(blocked.LoadedDigests) != 1 || blocked.LoadedDigests[row.ID] != loaded {
		t.Fatalf("in-flight ForceCompact published partial state: %#v", blocked)
	}

	stopSampling := make(chan struct{})
	invalid := make(chan loop.SkillCatalogRuntimeState, 1)
	var sampler sync.WaitGroup
	sampler.Add(1)
	go func() {
		defer sampler.Done()
		for {
			select {
			case <-stopSampling:
				return
			default:
			}
			state := query.SkillCatalogState()
			oldComplete := state.ContextEpoch == 7 && len(state.LoadedDigests) == 1 && state.LoadedDigests[row.ID] == loaded
			newComplete := state.ContextEpoch == 8 && len(state.LoadedDigests) == 0
			if !oldComplete && !newComplete {
				select {
				case invalid <- state:
				default:
				}
				return
			}
			runtime.Gosched()
		}
	}()
	close(provider.release)
	select {
	case err := <-compactErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("real QueryLoop.ForceCompact did not finish")
	}
	close(stopSampling)
	sampler.Wait()
	select {
	case state := <-invalid:
		t.Fatalf("epoch and loaded ledger were not published atomically: %#v", state)
	default:
	}

	after := query.SkillCatalogState()
	if after.ContextEpoch != 8 || len(after.LoadedDigests) != 0 || after.Cursor.Empty() ||
		after.Cursor.ContextEpoch != loop.SkillCatalogContextEpoch("context-8") {
		t.Fatalf("ForceCompact did not rebuild epoch 8 atomically: %#v", after)
	}
	resolved := query.SkillLoadedLedgerState(row.ID)
	if resolved.ContextEpoch != 8 || resolved.LoadedContextEpoch != 0 || resolved.ContentDigest != "" || resolved.PayloadDigest != "" {
		t.Fatalf("ForceCompact exposed epoch-seven body after replacement: %#v", resolved)
	}
	visible := query.Messages()
	if len(visible) == 0 || !compact.IsCompactBoundaryMessage(visible[0]) {
		t.Fatalf("ForceCompact did not install a real compact boundary: %#v", visible)
	}
}

func TestSkillsCompactAcceptanceRealCoreEngineCompactPersistsEpochSevenToEightAtomically(t *testing.T) {
	manager, row, projectRoot := task28SurfaceManager(t)
	const sessionID = "task28-engine-compact-epoch-seven"
	provider := &task28SurfaceScriptProvider{turns: [][]types.StreamEvent{
		task28SurfaceTextEvents("task28 bootstrap"),
		task28SurfaceToolEvents("task28-load", "Skill", map[string]any{
			"skill": string(row.ID), "revision": uint64(row.Revision),
		}),
		task28SurfaceTextEvents("task28 loaded"),
	}}
	tools := registry.New()
	tools.Register(task28SurfaceReceiptSkillTool{skill: row, body: "task28 exact loaded body"})
	repo := session.NewRepository(t.TempDir())
	eng, err := engine.New(engine.Config{
		Provider: provider, Registry: tools,
		Sessions:     engine.NewRepositorySessionManager(repo, func() string { return projectRoot }),
		SkillManager: manager, ProjectRoot: projectRoot, CWD: projectRoot,
		MaxTokens: 1024, MaxTurns: 3, MaxContextTokens: 100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	stream, err := eng.Query(context.Background(), engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectRoot,
		ProjectRoot: projectRoot, CWD: projectRoot, Message: "bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	task28SurfaceDrainEngine(t, stream)
	for index := 0; index < 6; index++ {
		if _, err := eng.Compact(context.Background(), sessionID); err != nil {
			t.Fatalf("preparing epoch 7 compact %d: %v", index+1, err)
		}
	}
	beforeLoad := eng.ResolveSkillLoadedLedger(context.Background(), sessionID, row.ID)
	if beforeLoad.ContextEpoch != 7 || beforeLoad.LoadedContextEpoch != 0 {
		t.Fatalf("six real CoreEngine compacts did not establish epoch 7: %#v", beforeLoad)
	}

	stream, err = eng.Query(context.Background(), engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: projectRoot,
		ProjectRoot: projectRoot, CWD: projectRoot, Message: "load at epoch seven",
	})
	if err != nil {
		t.Fatal(err)
	}
	task28SurfaceDrainEngine(t, stream)
	loaded := eng.ResolveSkillLoadedLedger(context.Background(), sessionID, row.ID)
	if loaded.ContextEpoch != 7 || loaded.LoadedContextEpoch != 7 || loaded.ContentDigest != row.Digest ||
		loaded.PayloadDigest != skills.DigestInvocationPayload("task28 exact loaded body") {
		t.Fatalf("real engine did not commit exact epoch-seven body receipt: %#v", loaded)
	}

	// Revocation makes retaining that exact body across compaction invalid.
	// CoreEngine.Compact must publish the epoch and cleared ledger together and
	// save the same tuple to the session sidecar.
	if _, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: row.ID, Scope: skills.SkillScopeProject, Visibility: skills.VisibilityOff,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Compact(context.Background(), sessionID); err != nil {
		t.Fatal(err)
	}
	after := eng.ResolveSkillLoadedLedger(context.Background(), sessionID, row.ID)
	if after.ContextEpoch != 8 || after.LoadedContextEpoch != 0 || after.ContentDigest != "" || after.PayloadDigest != "" {
		t.Fatalf("CoreEngine.Compact mixed epoch 8 with epoch-seven ledger: %#v", after)
	}
	meta, _, err := repo.GetMeta(sessionID, projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Skills == nil || meta.Skills.ContextEpoch != 8 || len(meta.Skills.LoadedDigests) != 0 {
		t.Fatalf("CoreEngine.Compact persisted non-atomic epoch/ledger sidecar: %#v", meta.Skills)
	}
}

type task28SurfaceProvider struct{}

func (task28SurfaceProvider) Name() string    { return "task28" }
func (task28SurfaceProvider) ModelID() string { return "task28-model" }
func (task28SurfaceProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	stream := make(chan types.StreamEvent)
	close(stream)
	return stream, nil
}

type task28SurfaceBlockingCompactProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*task28SurfaceBlockingCompactProvider) Name() string    { return "task28-compact" }
func (*task28SurfaceBlockingCompactProvider) ModelID() string { return "task28-compact-model" }
func (provider *task28SurfaceBlockingCompactProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	provider.once.Do(func() { close(provider.started) })
	select {
	case <-provider.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return task28SurfaceEventStream(task28SurfaceTextEvents(`{"schema":"compact-summary/v2","summary":"task28 compact summary"}`)), nil
}

type task28SurfaceScriptProvider struct {
	mu    sync.Mutex
	calls int
	turns [][]types.StreamEvent
}

func (*task28SurfaceScriptProvider) Name() string    { return "task28-script" }
func (*task28SurfaceScriptProvider) ModelID() string { return "task28-script-model" }
func (provider *task28SurfaceScriptProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	// CoreEngine compaction uses a tool-free summary request. Keep that traffic
	// separate from the scripted conversation turns so this acceptance fixture
	// remains correct if the retained history later grows past KeepRecent.
	if len(params.Tools) == 0 {
		return task28SurfaceEventStream(task28SurfaceTextEvents(`{"schema":"compact-summary/v2","summary":"task28 compact summary"}`)), nil
	}
	provider.mu.Lock()
	index := provider.calls
	provider.calls++
	var events []types.StreamEvent
	if index < len(provider.turns) {
		events = append([]types.StreamEvent(nil), provider.turns[index]...)
	} else {
		events = task28SurfaceTextEvents("task28 default")
	}
	provider.mu.Unlock()
	return task28SurfaceEventStream(events), nil
}

type task28SurfaceReceiptSkillTool struct {
	skill skills.EffectiveSkill
	body  string
}

func (task28SurfaceReceiptSkillTool) Name() string        { return "Skill" }
func (task28SurfaceReceiptSkillTool) Description() string { return "task28 exact skill receipt" }
func (task28SurfaceReceiptSkillTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (tool task28SurfaceReceiptSkillTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	execution, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return types.ToolResult{}, errors.New("task28 tool execution context missing")
	}
	ledger, resolved := execution.ResolveSkillLoadedLedger(string(tool.skill.ID))
	if !resolved || ledger.ContextEpoch == 0 {
		return types.ToolResult{}, errors.New("task28 skill ledger capability missing")
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

func task28SurfaceTextEvents(value string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: value}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

func task28SurfaceToolEvents(id, name string, input map[string]any) []types.StreamEvent {
	raw, _ := json.Marshal(input)
	return authorizeAppTestToolStreams([]types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: id, Name: name,
		}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "input_json_delta", PartialJSON: string(raw),
		}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	})
}

func task28SurfaceEventStream(events []types.StreamEvent) <-chan types.StreamEvent {
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream
}

func task28SurfaceDrainEngine(t *testing.T, stream <-chan engine.Event) {
	t.Helper()
	seenFinal := false
	for event := range stream {
		if event.Final {
			seenFinal = true
			if event.Error != nil {
				t.Fatalf("engine query final error: %v", event.Error)
			}
		}
	}
	if !seenFinal {
		t.Fatal("engine query stream closed without final event")
	}
}

func task28SurfaceMessagesText(messages []tui.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Text)
	}
	return strings.Join(parts, "\n")
}

func task28SurfaceContainsAll(text string, values []string) bool {
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}

func task28SurfaceManager(t *testing.T) (*skills.Manager, skills.EffectiveSkill, string) {
	t.Helper()
	root := t.TempDir()
	skillRoot := filepath.Join(root, "skills")
	dir := filepath.Join(skillRoot, "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review carefully\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "settings", "user.json"),
		ProjectSettings: filepath.Join(root, "settings", "project.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: skillRoot, Source: skills.SourceProject})
	manager.SetOverrideStore(store)
	snapshot := task28SurfaceSnapshot(t, manager, "task28-surface-session")
	if len(snapshot.Skills) != 1 {
		t.Fatalf("surface snapshot=%#v", snapshot)
	}
	return manager, snapshot.Skills[0], root
}

func task28SurfaceSnapshot(t *testing.T, manager *skills.Manager, sessionID string) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func task28SurfaceWaitApp(t *testing.T, app *tui.App, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.GoTuiApp().DispatchEvents()
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for composed skills surface")
}

func task28SurfaceTUIApp(t *testing.T) *tui.App {
	t.Helper()
	master, slave := task28SurfacePTY(t)
	oldStdin, oldStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = slave, slave
	app, err := tui.NewTUIAppWithAdmission(func(string) bool { return false }, "task28", "task28-model", nil, nil)
	os.Stdin, os.Stdout = oldStdin, oldStdout
	if err != nil {
		_ = slave.Close()
		_ = master.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = slave.Close()
		_ = master.Close()
	})
	return app
}
