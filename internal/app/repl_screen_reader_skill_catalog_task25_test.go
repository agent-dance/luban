package app

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestScreenReaderSkillManagementCommandsStayLinear(t *testing.T) {
	backend := newTask25SkillsBackend(t, task25EffectiveSkill("skill:project:alpha", "alpha", skills.SourceProject, skills.VisibilityAuto))
	sessionID := "task25-management"
	cfg := TUIREPLConfig{SessionID: &sessionID, SkillManager: backend}
	tracker := ui.NewCostTracker("test")

	tests := []struct {
		input string
		want  []string
	}{
		{input: "/skills list", want: []string{"Catalog revision:", "alpha", "skill:project:alpha"}},
		{input: "/skills show alpha", want: []string{"Skill: alpha", "Visibility: auto"}},
		{input: "/skills set alpha manual-only --scope session", want: []string{"manual-only", "session"}},
		{input: "/skills reset alpha --scope session", want: []string{"auto", "default"}},
		{input: "/skills refresh", want: []string{"Refreshed", "Catalog revision:"}},
	}
	for _, test := range tests {
		t.Run(strings.ReplaceAll(test.input, " ", "_"), func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, tracker, test.input)
			if err != nil || !handled || exit {
				t.Fatalf("handle %q = handled %t exit %t err %v", test.input, handled, exit, err)
			}
			text := output.String()
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Errorf("%q output omitted %q:\n%s", test.input, want, text)
				}
			}
			assertTask25NoTerminalControls(t, text)
		})
	}
	backend.mu.Lock()
	resolveRequests := append([]skills.SkillResolveRequest(nil), backend.resolveRequests...)
	backend.mu.Unlock()
	if len(resolveRequests) != 1 {
		t.Fatalf("/skills show resolve requests = %#v", resolveRequests)
	}
	request := resolveRequests[0]
	if request.Selector != "skill:project:alpha" || request.ExpectedRevision != 1 ||
		request.ExpectedProjectGeneration != 1 || request.Origin != skills.InvocationOriginUser {
		t.Fatalf("/skills show did not pin the selected catalog row: %#v", request)
	}
}

func TestScreenReaderSkillInvocationUsesUserOriginAndHidesEnvelope(t *testing.T) {
	manual := task25EffectiveSkill("skill:project:task25-review", "task25-review", skills.SourceProject, skills.VisibilityManualOnly)
	backend := newTask25SkillsBackend(t, manual)
	queryEngine := &task25ScreenReaderEngine{}
	sessionID := "task25-invoke"

	var mu sync.Mutex
	var captured []commands.SkillInvocationRequest
	invoker := commands.SkillInvokerFunc(func(_ context.Context, request commands.SkillInvocationRequest) (types.ToolResult, error) {
		mu.Lock()
		captured = append(captured, request)
		mu.Unlock()
		return types.ToolResult{Content: "task25 hidden versioned SKILL body"}, nil
	})
	cfg := TUIREPLConfig{
		Engine: queryEngine, SessionID: &sessionID,
		SkillManager: backend, SkillInvoker: invoker,
	}

	tests := []struct {
		name     string
		input    string
		wantArgs *string
	}{
		{name: "name with arguments", input: "/task25-review audit --strict", wantArgs: task25String("audit --strict")},
		{name: "name with omitted arguments", input: "/task25-review"},
		{name: "name with explicit empty arguments", input: "/task25-review ", wantArgs: task25String("")},
		{name: "stable id", input: "/skill:project:task25-review", wantArgs: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()
			handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), test.input)
			if err != nil || !handled || exit {
				t.Fatalf("handle %q = handled %t exit %t err %v", test.input, handled, exit, err)
			}

			mu.Lock()
			request := captured[len(captured)-1]
			mu.Unlock()
			if request.Selector != string(manual.ID) || request.ExpectedRevision != manual.Revision || request.ExpectedProjectGeneration != 1 ||
				request.Origin != skills.InvocationOriginUser || request.SessionID != sessionID {
				t.Fatalf("invocation request = %#v", request)
			}
			if !task25EqualStringPointers(request.Arguments, test.wantArgs) {
				t.Fatalf("arguments = %#v, want %#v", request.Arguments, test.wantArgs)
			}
			queries := queryEngine.snapshotQueries()
			if len(queries) == 0 || queries[len(queries)-1].Message != "task25 hidden versioned SKILL body" {
				t.Fatalf("model query = %#v", queries)
			}
			if strings.Contains(output.String(), "task25 hidden versioned SKILL body") {
				t.Fatalf("screen reader narrated hidden invocation envelope: %q", output.String())
			}
			assertTask25NoTerminalControls(t, output.String())
		})
	}
}

func TestScreenReaderSkillInvocationRejectsAmbiguousOffManagedAndManualPolicy(t *testing.T) {
	winner := task25EffectiveSkill("skill:project:task25-collision", "task25-collision", skills.SourceProject, skills.VisibilityAuto)
	shadowed := task25EffectiveSkill("skill:user:task25-collision", "task25-collision", skills.SourceUser, skills.VisibilityAuto)
	shadowed.ShadowedBy = winner.ID
	shadowed.ModelVisible = false
	shadowed.DescriptionVisible = false
	shadowed.Executable = false
	off := task25EffectiveSkill("skill:project:off", "off-skill", skills.SourceProject, skills.VisibilityOff)
	managed := task25EffectiveSkill("skill:managed:locked", "locked", skills.SourceManaged, skills.VisibilityOff)
	managed.Mutable = false
	managed.ReadOnlyReason = string(skills.CatalogPolicyReasonManagedDeny)
	managed.VisibilitySource = skills.SkillScopeManaged
	manualDisabled := task25EffectiveSkill("skill:project:no-user", "no-user", skills.SourceProject, skills.VisibilityManualOnly)
	manualDisabled.UserInvocable = false
	manualDisabled.Executable = false

	tests := []struct {
		name    string
		rows    []skills.EffectiveSkill
		input   string
		want    []string
		invoked bool
	}{
		{
			name: "same-name collision is ambiguous", rows: []skills.EffectiveSkill{winner, shadowed}, input: "/task25-collision",
			want: []string{"ambiguous", string(winner.ID), string(shadowed.ID)},
		},
		{name: "off", rows: []skills.EffectiveSkill{off}, input: "/off-skill", want: []string{"not available", "/off-skill"}},
		{name: "managed deny", rows: []skills.EffectiveSkill{managed}, input: "/locked", want: []string{"not available", "/locked"}},
		{name: "not user invocable", rows: []skills.EffectiveSkill{manualDisabled}, input: "/no-user", want: []string{"not available", "/no-user"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newTask25SkillsBackend(t, test.rows...)
			var invokeCount int
			var invokeMu sync.Mutex
			invoker := commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
				invokeMu.Lock()
				invokeCount++
				invokeMu.Unlock()
				return types.ToolResult{Content: "must not be queried"}, nil
			})
			sessionID := "task25-denied"
			queryEngine := &task25ScreenReaderEngine{}
			cfg := TUIREPLConfig{Engine: queryEngine, SessionID: &sessionID, SkillManager: backend, SkillInvoker: invoker}
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()

			handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), test.input)
			if err != nil || !handled || exit {
				t.Fatalf("handle %q = handled %t exit %t err %v", test.input, handled, exit, err)
			}
			invokeMu.Lock()
			gotInvokeCount := invokeCount
			invokeMu.Unlock()
			if gotInvokeCount != 0 || len(queryEngine.snapshotQueries()) != 0 {
				t.Fatalf("denied route invoked=%d queries=%#v", gotInvokeCount, queryEngine.snapshotQueries())
			}
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("output omitted %q: %q", want, output.String())
				}
			}
			assertTask25NoTerminalControls(t, output.String())
		})
	}
}

func TestScreenReaderSkillInvocationFailuresDoNotStartModelTurn(t *testing.T) {
	row := task25EffectiveSkill("skill:project:task25-review", "task25-review", skills.SourceProject, skills.VisibilityAuto)
	backend := newTask25SkillsBackend(t, row)
	tests := []struct {
		name    string
		invoker commands.SkillInvoker
		want    string
	}{
		{
			name: "infrastructure error",
			invoker: commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
				return types.ToolResult{}, errors.New("task25 invoke offline")
			}),
			want: "task25 invoke offline",
		},
		{
			name: "authoritative rejection",
			invoker: commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
				return types.ToolResult{Content: "task25 policy changed", IsError: true}, nil
			}),
			want: "task25 policy changed",
		},
		{
			name: "empty envelope",
			invoker: commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
				return types.ToolResult{}, nil
			}),
			want: "no invocation instructions",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sessionID := "task25-failure"
			queryEngine := &task25ScreenReaderEngine{}
			cfg := TUIREPLConfig{Engine: queryEngine, SessionID: &sessionID, SkillManager: backend, SkillInvoker: test.invoker}
			var output bytes.Buffer
			renderer := ui.NewScreenReaderRenderer(&output, nil)
			defer renderer.Close()

			handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/task25-review")
			if err != nil || !handled || exit {
				t.Fatalf("handle = handled %t exit %t err %v", handled, exit, err)
			}
			if len(queryEngine.snapshotQueries()) != 0 {
				t.Fatalf("failed invocation started model query: %#v", queryEngine.snapshotQueries())
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("output omitted %q: %q", test.want, output.String())
			}
			assertTask25NoTerminalControls(t, output.String())
		})
	}
}

func TestScreenReaderSkillFailuresUseActiveRuntimeLanguage(t *testing.T) {
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })

	sessionID := "task25-language"
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handled, exit, err := handleScreenReaderCommand(
		context.Background(), TUIREPLConfig{SessionID: &sessionID}, renderer, ui.NewCostTracker("test"), "/missing-skill",
	)
	if err != nil || !handled || exit {
		t.Fatalf("handle = handled %t exit %t err %v", handled, exit, err)
	}
	if !strings.Contains(output.String(), "无法使用实时技能目录") {
		t.Fatalf("output did not use active Chinese language: %q", output.String())
	}
}

func TestScreenReaderSkillCatalogAndDeveloperProjectionNeverEmitControlsOrDeveloperText(t *testing.T) {
	backend := newTask25SkillsBackend(t, task25EffectiveSkill("skill:project:alpha", "alpha", skills.SourceProject, skills.VisibilityAuto))
	backend.snapshotErr = errors.New("catalog\x1b[2J\x00offline")
	sessionID := "task25-controls"
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()

	handled, exit, err := handleScreenReaderCommand(
		context.Background(), TUIREPLConfig{SessionID: &sessionID, SkillManager: backend}, renderer, ui.NewCostTracker("test"), "/alpha",
	)
	if err != nil || !handled || exit {
		t.Fatalf("handle = handled %t exit %t err %v", handled, exit, err)
	}
	assertTask25NoTerminalControls(t, output.String())

	output.Reset()
	controlScope := messagecontrol.NewScope(sessionID, "task25-screen-reader", 1)
	developer := types.DeveloperMessage("task25 secret catalog metadata", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 7,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), controlScope)
	malformed := types.UserMessage("task25 malformed developer metadata")
	malformed.IsMeta = true
	malformed.DeveloperMetadata = &types.DeveloperMessageMetadata{Kind: types.DeveloperMessageKindSkillCatalogDelta, Revision: 8}
	renderScreenReaderTranscript(renderer, []types.Message{
		developer,
		malformed,
		types.UserMessage("task25 visible user"),
		types.AssistantMessage("task25 visible assistant"),
	}, presentation.ToolEventContext{SessionID: sessionID}, controlScope)
	text := output.String()
	for _, hidden := range []string{"task25 secret catalog metadata", "developer:"} {
		if strings.Contains(text, hidden) {
			t.Errorf("developer-only message leaked %q: %q", hidden, text)
		}
	}
	for _, visible := range []string{"task25 malformed developer metadata", "task25 visible user", "task25 visible assistant"} {
		if !strings.Contains(text, visible) {
			t.Errorf("visible transcript omitted %q: %q", visible, text)
		}
	}
	assertTask25NoTerminalControls(t, text)
}

func TestScreenReaderSkillResumeSummarizesStrictInvocationEnvelopeWithoutBody(t *testing.T) {
	row := task25EffectiveSkill("skill:project:task25-resume", "task25-resume", skills.SourceProject, skills.VisibilityAuto)
	canonicalID, err := skills.ComputeSkillID(row.Source, row.Locator)
	if err != nil {
		t.Fatal(err)
	}
	row.ID = canonicalID
	argument := "task25 secret resume argument"
	body := "task25 secret persisted SKILL body\nwith another line"
	full, err := skills.RenderFullInvocationEnvelope(row, body, skills.NewInvocationArguments(&argument))
	if err != nil {
		t.Fatal(err)
	}
	acknowledgement, err := skills.RenderLoadedDigestAcknowledgement(
		row, row.Digest, skills.DigestInvocationPayload(body), body, skills.NewInvocationArguments(nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	// This looks related but is not a complete canonical v1 envelope. It is
	// ordinary user text and must not be hidden by a prefix heuristic.
	looseUserJSON := `{"type":"skill_invocation","version":1,"body":"task25 ordinary visible JSON"}`
	fullMessage := types.UserMessage(full)
	fullMessage.InternalKind = types.InternalMessageKindSkillInvocation
	controlScope := messagecontrol.NewScope("task25-resume", "task25-screen-reader", 1)
	fullMessage = fullMessage.WithInternalControlProvenance(messagecontrol.Runtime(), controlScope)
	ackMessage := types.UserMessage(acknowledgement)
	ackMessage.InternalKind = types.InternalMessageKindSkillInvocation
	ackMessage = ackMessage.WithInternalControlProvenance(messagecontrol.Runtime(), controlScope)

	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	renderScreenReaderTranscript(renderer, []types.Message{
		fullMessage,
		ackMessage,
		types.UserMessage(looseUserJSON),
		types.AssistantMessage("task25 visible response after invocation"),
	}, presentation.ToolEventContext{SessionID: "task25-resume"}, controlScope)
	text := output.String()
	for _, hidden := range []string{body, argument, `"payload_digest"`, `"skill":{"id"`} {
		if strings.Contains(text, hidden) {
			t.Errorf("persisted invocation detail leaked %q: %q", hidden, text)
		}
	}
	for _, visible := range []string{
		"Explicit skill invocation: /task25-resume (arguments provided)",
		"Explicit skill invocation: /task25-resume (arguments omitted)",
		looseUserJSON,
		"task25 visible response after invocation",
	} {
		if !strings.Contains(text, visible) {
			t.Errorf("resume transcript omitted %q: %q", visible, text)
		}
	}
	assertTask25NoTerminalControls(t, text)
}

type task25ScreenReaderEngine struct {
	engine.Engine
	mu      sync.Mutex
	queries []engine.QueryRequest
}

func (e *task25ScreenReaderEngine) Query(_ context.Context, request engine.QueryRequest) (<-chan engine.Event, error) {
	e.mu.Lock()
	e.queries = append(e.queries, request)
	e.mu.Unlock()
	result := make(chan engine.Event, 1)
	result <- engine.Event{SessionID: request.SessionID, Final: true}
	close(result)
	return result, nil
}

func (e *task25ScreenReaderEngine) snapshotQueries() []engine.QueryRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]engine.QueryRequest(nil), e.queries...)
}

type task25SkillsBackend struct {
	mu              sync.Mutex
	snapshot        skills.CatalogSnapshot
	snapshotErr     error
	resolveRequests []skills.SkillResolveRequest
}

func newTask25SkillsBackend(t *testing.T, rows ...skills.EffectiveSkill) *task25SkillsBackend {
	t.Helper()
	snapshot, err := skills.NewCatalogSnapshot(1, rows)
	if err != nil {
		t.Fatalf("new task25 snapshot: %v", err)
	}
	return &task25SkillsBackend{snapshot: snapshot}
}

func (backend *task25SkillsBackend) Snapshot(string) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.snapshot.Clone(), backend.snapshotErr
}

func (backend *task25SkillsBackend) SnapshotBinding(string) (skills.CatalogBinding, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.snapshotErr != nil {
		return skills.CatalogBinding{}, backend.snapshotErr
	}
	return skills.CatalogBinding{
		ProjectGeneration: 1,
		Snapshot:          backend.snapshot.Clone(),
	}, nil
}

func (backend *task25SkillsBackend) ResolveLatest(request skills.SkillResolveRequest, consume func(skills.ResolvedSkill) error) (skills.SkillResolveResult, error) {
	if err := request.Validate(); err != nil {
		return skills.SkillResolveResult{}, err
	}
	if request.ExpectedProjectGeneration != 0 && request.ExpectedProjectGeneration != 1 {
		return skills.SkillResolveResult{}, skills.ErrSkillProjectGenerationChanged
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.resolveRequests = append(backend.resolveRequests, request)
	result := skills.SkillResolveResult{CatalogRevision: backend.snapshot.Revision}
	var row skills.EffectiveSkill
	var found bool
	if id := skills.SkillID(request.Selector); id.IsValid() {
		row, found = backend.snapshot.Find(id)
		if found {
			result.Candidates = []skills.SkillID{id}
		}
	} else {
		for _, candidate := range backend.snapshot.Skills {
			if candidate.Name != request.Selector {
				continue
			}
			result.Candidates = append(result.Candidates, candidate.ID)
			if candidate.ShadowedBy == "" {
				if found {
					result.Outcome = skills.SkillResolveAmbiguous
					sort.Slice(result.Candidates, func(i, j int) bool { return result.Candidates[i] < result.Candidates[j] })
					return result, nil
				}
				row, found = candidate, true
			}
		}
		sort.Slice(result.Candidates, func(i, j int) bool { return result.Candidates[i] < result.Candidates[j] })
	}
	if !found {
		result.Outcome = skills.SkillResolveNotFound
		return result, nil
	}
	resolved := skills.ResolvedSkill{
		Effective: row,
		Skill: &skills.Skill{
			Name: row.Name, Description: row.Summary, Source: row.Source,
			FilePath: string(row.Locator), SkillDir: "/task25/skills/" + row.Name,
			RawContent: "task25 raw", Content: "task25 body",
		},
	}
	result.Resolved = &resolved
	if request.ExpectedRevision != 0 && request.ExpectedRevision != row.Revision {
		result.Outcome = skills.SkillResolveStale
		return result, nil
	}
	if row.ShadowedBy != "" {
		result.Outcome = skills.SkillResolveShadowed
		return result, nil
	}
	allowed := row.Executable && row.Visibility != skills.VisibilityOff
	if request.Origin == skills.InvocationOriginModel {
		allowed = allowed && row.ModelVisible
	} else {
		allowed = allowed && row.UserInvocable
	}
	if !allowed {
		result.Outcome = skills.SkillResolvePolicyDenied
		return result, nil
	}
	result.Outcome = skills.SkillResolveResolved
	if consume != nil {
		if err := consume(resolved); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (backend *task25SkillsBackend) SetVisibility(_ string, override skills.VisibilityOverride) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.setVisibilityLocked(override.SkillID, override.Scope, override.Visibility)
}

func (backend *task25SkillsBackend) ResetVisibility(_ string, scope skills.SkillScope, id skills.SkillID) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.setVisibilityLocked(id, skills.SkillScopeDefault, skills.VisibilityAuto)
}

func (backend *task25SkillsBackend) RefreshSnapshot(string) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	next, err := skills.NewCatalogSnapshot(backend.snapshot.Revision+1, backend.snapshot.Skills)
	if err != nil {
		return skills.CatalogSnapshot{}, err
	}
	backend.snapshot = next
	return next.Clone(), nil
}

func (backend *task25SkillsBackend) ToggleProjectVisibility(_ string, id skills.SkillID, expected skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	row, found := backend.snapshot.Find(id)
	if !found {
		return skills.ProjectVisibilityToggleResult{}, errors.New("task25 skill not found")
	}
	visibility := skills.VisibilityOff
	if row.Visibility == skills.VisibilityOff {
		visibility = skills.VisibilityAuto
	}
	next, err := backend.setVisibilityLocked(id, skills.SkillScopeProject, visibility)
	if err != nil {
		return skills.ProjectVisibilityToggleResult{}, err
	}
	updated, _ := next.Find(id)
	return skills.ProjectVisibilityToggleResult{
		Outcome: skills.ProjectVisibilityToggleCommitted, RequestedSkillID: id,
		ObservedRevision: expected, CurrentRevision: next.Revision, Skill: &updated, Snapshot: next,
	}, nil
}

func (backend *task25SkillsBackend) setVisibilityLocked(id skills.SkillID, scope skills.SkillScope, visibility skills.Visibility) (skills.CatalogSnapshot, error) {
	rows := append([]skills.EffectiveSkill(nil), backend.snapshot.Skills...)
	found := false
	for index := range rows {
		if rows[index].ID != id {
			continue
		}
		found = true
		row := &rows[index]
		row.Visibility = visibility
		row.VisibilitySource = scope
		row.Revision++
		switch visibility {
		case skills.VisibilityAuto:
			row.ModelVisible, row.DescriptionVisible = true, true
			row.UserInvocable, row.Executable = true, true
		case skills.VisibilityNameOnly:
			row.ModelVisible, row.DescriptionVisible = true, false
			row.UserInvocable, row.Executable = true, true
		case skills.VisibilityManualOnly:
			row.ModelVisible, row.DescriptionVisible = false, false
			row.UserInvocable, row.Executable = true, true
		case skills.VisibilityOff:
			row.ModelVisible, row.DescriptionVisible = false, false
			row.UserInvocable, row.Executable = false, false
		}
		break
	}
	if !found {
		return backend.snapshot.Clone(), errors.New("task25 skill not found")
	}
	next, err := skills.NewCatalogSnapshot(backend.snapshot.Revision+1, rows)
	if err != nil {
		return skills.CatalogSnapshot{}, err
	}
	backend.snapshot = next
	return next.Clone(), nil
}

func task25EffectiveSkill(id skills.SkillID, name string, source skills.SkillSource, visibility skills.Visibility) skills.EffectiveSkill {
	row := skills.EffectiveSkill{
		ID: id, Name: name, Summary: "Task 25 summary", Source: source,
		Locator: skills.SkillLocator("/task25/" + string(source) + "/" + name + "/SKILL.md"),
		Digest:  skills.ComputeSkillDigest("task25 raw"), Revision: 1,
		Visibility: visibility, VisibilitySource: skills.SkillScopeDefault,
		Mutable: true,
	}
	switch visibility {
	case skills.VisibilityAuto:
		row.ModelVisible, row.DescriptionVisible = true, true
		row.UserInvocable, row.Executable = true, true
	case skills.VisibilityNameOnly:
		row.ModelVisible, row.UserInvocable, row.Executable = true, true, true
	case skills.VisibilityManualOnly:
		row.UserInvocable, row.Executable = true, true
	case skills.VisibilityOff:
	}
	return row
}

func task25String(value string) *string { return &value }

func task25EqualStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func assertTask25NoTerminalControls(t *testing.T, text string) {
	t.Helper()
	for _, char := range text {
		if char != '\n' && (char < 0x20 || char == 0x7f || (char >= 0x80 && char <= 0x9f)) {
			t.Fatalf("screen-reader output retained terminal control U+%04X: %q", char, text)
		}
	}
}

var _ commands.SkillsBackend = (*task25SkillsBackend)(nil)
