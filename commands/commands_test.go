package commands_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// ---------------------------------------------------------------------------
// Minimal stub implementations for testing
// ---------------------------------------------------------------------------

type stubQL struct {
	messages []types.Message
	model    string
	provider provider.Provider
	maxCtx   int
	usedCtx  int
}

func (s *stubQL) SetMessagesPreservingToolUseLedger(msgs []types.Message) {
	s.messages = msgs
}
func (s *stubQL) Messages() []types.Message          { return s.messages }
func (s *stubQL) Model() string                      { return s.model }
func (s *stubQL) SetModel(m string)                  { s.model = m }
func (s *stubQL) ContextUsage() (int, int)           { return s.maxCtx, s.usedCtx }
func (s *stubQL) SetProvider(next provider.Provider) { s.provider = next }

type warningStubQL struct {
	stubQL
	state compact.TokenWarningState
}

type detailedContextStubQL struct {
	stubQL
	usage compact.ContextInputUsage
}

func (s *detailedContextStubQL) ContextUsageDetail() (int, compact.ContextInputUsage) {
	return s.maxCtx, s.usage
}

func (s *warningStubQL) ContextWarningState() compact.TokenWarningState {
	return s.state
}

type stubSessionStore struct {
	entries     []commands.SessionListEntry
	loads       map[string][]types.Message
	loadCalls   []string
	renames     map[string]string
	searchCalls []string
}

func (s *stubSessionStore) Save(_ string, _ []types.Message) error { return nil }
func (s *stubSessionStore) Load(id string) ([]types.Message, error) {
	s.loadCalls = append(s.loadCalls, id)
	if msgs, ok := s.loads[id]; ok {
		return msgs, nil
	}
	return nil, errors.New("not found")
}
func (s *stubSessionStore) List() ([]commands.SessionListEntry, error) { return s.entries, nil }
func (s *stubSessionStore) Search(query, _ string, _ bool) ([]commands.SessionListEntry, error) {
	s.searchCalls = append(s.searchCalls, query)
	var out []commands.SessionListEntry
	for _, e := range s.entries {
		if e.ID == query || e.Title == query {
			out = append(out, e)
		}
	}
	return out, nil
}
func (s *stubSessionStore) Rename(id, title string) error {
	if s.renames == nil {
		s.renames = map[string]string{}
	}
	s.renames[id] = title
	return nil
}

type pingCmd struct{}

func (c *pingCmd) Name() string                                { return "ping" }
func (c *pingCmd) Aliases() []string                           { return []string{"p", "pong"} }
func (c *pingCmd) Description() string                         { return "Test command" }
func (c *pingCmd) Execute(_ *commands.Context, _ string) error { return nil }

func TestFind_ByName(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	cmd := r.Find("ping")
	if cmd == nil || cmd.Name() != "ping" {
		t.Fatal("expected to resolve ping")
	}
}

func TestFind_ByAlias(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	for _, alias := range []string{"p", "pong"} {
		if r.Find(alias) == nil {
			t.Fatalf("expected alias %q", alias)
		}
	}
}

func TestFind_WithLeadingSlash(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	if r.Find("/ping") == nil || r.Find("/p") == nil {
		t.Fatal("expected slash-prefixed aliases to resolve")
	}
}

func TestFind_Unknown(t *testing.T) {
	r := commands.NewRegistry()
	if r.Find("nope") != nil {
		t.Fatal("expected nil for unknown command")
	}
}

func TestIsCommand(t *testing.T) {
	r := commands.NewRegistry()
	cases := []struct {
		input string
		want  bool
	}{{"/model", true}, {"/exit", true}, {"hello", false}, {"", false}, {" /model", false}}
	for _, tc := range cases {
		if got := r.IsCommand(tc.input); got != tc.want {
			t.Errorf("IsCommand(%q)=%v want %v", tc.input, got, tc.want)
		}
	}
}

func TestParse_KnownCommand(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	cmd, args := r.Parse("/ping")
	if cmd == nil || cmd.Name() != "ping" || args != "" {
		t.Fatal("expected /ping to parse")
	}
}

func TestParse_WithArgs(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	cmd, args := r.Parse("/ping   hello world  ")
	if cmd == nil || args != "hello world" {
		t.Fatalf("unexpected parse result: cmd=%v args=%q", cmd, args)
	}
}

func TestParse_Alias(t *testing.T) {
	r := commands.NewRegistry()
	r.Register(&pingCmd{})
	if cmd, _ := r.Parse("/pong"); cmd == nil {
		t.Fatal("expected alias parse")
	}
}

func TestParse_UnknownCommand(t *testing.T) {
	r := commands.NewRegistry()
	cmd, args := r.Parse("/unknown")
	if cmd != nil || args != "" {
		t.Fatal("expected unknown parse to return nil")
	}
}

func TestParse_NotACommand(t *testing.T) {
	r := commands.NewRegistry()
	cmd, args := r.Parse("hello")
	if cmd != nil || args != "" {
		t.Fatal("expected non-command parse to return nil")
	}
}

func TestAll_Order(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	all := r.All()
	if len(all) == 0 || all[0].Name() != "exit" {
		t.Fatal("expected builtins in registration order")
	}
}

func TestRegisterBuiltins_RemovesUnsupportedSlashCommands(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	removed := []string{
		"connect",
		"paste",
		"permissions",
		"cost",
		"version",
		"rename",
		"memory",
		"diff",
	}
	for _, name := range removed {
		if cmd := r.Find(name); cmd != nil {
			t.Fatalf("expected /%s to be unregistered, got %T", name, cmd)
		}
	}
}

func TestHelpIsRegisteredAndDiscoversFullscreenKeyboardPaths(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	var output strings.Builder
	if err := r.Find("help").Execute(&commands.Context{OnCommandPresentation: captureCompletedCommand(&output)}, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/help", "Ctrl+O", "Alt+O", "Ctrl+Home", "PageUp", "Escape"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("help omitted %q:\n%s", want, output.String())
		}
	}
}

func TestRegisterBuiltins_IncludesMCPManagementCommand(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	if cmd := r.Find("mcp"); cmd == nil || cmd.Name() != "mcp" {
		t.Fatalf("expected /mcp builtin command, got %T", cmd)
	}
}

func newCtx(ql commands.QueryLooper) *commands.Context {
	return &commands.Context{QueryLoop: ql, OnEvent: func(string) {}, AppVersion: "1.2.3", SessionID: "sess-1"}
}

func TestExit_ReturnsErrExit(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	err := r.Find("exit").Execute(newCtx(&stubQL{}), "")
	if !errors.Is(err, commands.ErrExit) {
		t.Fatalf("expected ErrExit, got %v", err)
	}
}

func TestQuit_AliasForExit(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	err := r.Find("quit").Execute(newCtx(&stubQL{}), "")
	if !errors.Is(err, commands.ErrExit) {
		t.Fatalf("expected ErrExit, got %v", err)
	}
}

func TestModel_SwitchModel(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	ql := &stubQL{model: "old-model"}
	if err := r.Find("model").Execute(newCtx(ql), "new-model"); err != nil {
		t.Fatal(err)
	}
	if ql.model != "new-model" {
		t.Fatalf("expected new-model, got %q", ql.model)
	}
}

func TestModel_PersistsModelWhenCWDSet(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	ql := &stubQL{model: "old-model"}
	cwd := t.TempDir()
	ctx := newCtx(ql)
	ctx.CWD = cwd

	if err := r.Find("model").Execute(ctx, brand.DeepSeekProModel); err != nil {
		t.Fatal(err)
	}
	if ql.model != brand.DeepSeekProModel {
		t.Fatalf("expected %s, got %q", brand.DeepSeekProModel, ql.model)
	}

	settingsPath := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected model settings at %s: %v", settingsPath, err)
	}
	if !strings.Contains(string(data), brand.DeepSeekProModel) {
		t.Fatalf("settings did not contain persisted model: %s", string(data))
	}
}

func TestStatusLabelsProcessedUsageAndUniqueInput(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	var output strings.Builder
	ctx := &commands.Context{
		QueryLoop: &stubQL{
			messages: []types.Message{types.UserMessage("hello")},
			model:    "gpt-5.4",
		},
		OnCommandPresentation:    captureCompletedCommand(&output),
		TotalInputTokens:         2006,
		TotalOutputTokens:        86,
		TotalCacheReadTokens:     1920,
		TotalCacheCreationTokens: 128,
	}
	cmd := r.Find("status")
	if err := cmd.Execute(ctx, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"New input≈:", "API processed usage:", "Input tokens:    2,006", "Cache read:      1,920"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected status output to contain %q, got:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "Total processed:") || strings.Contains(output.String(), "Total tokens:") {
		t.Fatalf("status should not derive a total that can double-count cached tokens, got:\n%s", output.String())
	}
}

func TestContextCommandUsesWarningStateEffectiveInputWindow(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	var output strings.Builder
	ctx := &commands.Context{
		QueryLoop: &warningStubQL{
			stubQL: stubQL{
				messages: []types.Message{types.UserMessage("hello")},
				model:    "model-with-effective-window",
				maxCtx:   128_000,
				usedCtx:  100_000,
			},
			state: compact.TokenWarningState{
				UsedTokens:                 100_000,
				EffectiveInputWindowTokens: 108_000,
				PercentLeft:                7,
				IsAtBlockingLimit:          true,
			},
		},
		OnCommandPresentation: captureCompletedCommand(&output),
	}

	if err := r.Find("context").Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"100,000 / 128,000 tokens",
		"Remaining:              22%",
		"blocking limit reached",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected /context output to contain %q, got:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "100,000 / 108,000 tokens") {
		t.Fatalf("/context must not display the auto-compact capacity as the model context, got:\n%s", output.String())
	}
}

func TestBDDContextCommandPreservesEstimateMeasurement(t *testing.T) {
	for _, test := range []struct {
		name        string
		measurement compact.ContextUsageMeasurement
		wantUsage   string
		wantSource  string
	}{
		{name: "complete estimate", measurement: compact.ContextUsageLocalEstimate, wantUsage: "≈90,000 / 200,000", wantSource: "complete local estimate"},
		{name: "lower bound", measurement: compact.ContextUsageLocalLowerBound, wantUsage: "≥90,000 / 200,000", wantSource: "incomplete local estimate (lower bound)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			ctx := &commands.Context{
				Language: i18n.LangEN,
				QueryLoop: &detailedContextStubQL{
					stubQL: stubQL{model: "fixture", maxCtx: 200_000},
					usage:  compact.ContextInputUsage{UsedTokens: 90_000, Measurement: test.measurement},
				},
				OnCommandPresentation: captureCompletedCommand(&output),
			}
			registry := commands.NewRegistry()
			commands.RegisterBuiltins(registry)
			if err := registry.Find("context").Execute(ctx, ""); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), test.wantUsage) || !strings.Contains(output.String(), test.wantSource) {
				t.Fatalf("measurement was not preserved:\n%s", output.String())
			}
		})
	}
}

func TestSessionCurrent_ShowsMetadata(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	var output strings.Builder
	store := &stubSessionStore{entries: []commands.SessionListEntry{{
		ID:           "sess-1",
		Title:        "fix-login",
		UpdatedAt:    time.Now(),
		CreatedAt:    time.Now().Add(-time.Hour),
		MessageCount: 3,
		GitBranch:    "feature/login",
		CWD:          "/repo/app",
		PreviewText:  "Please fix login",
	}}}
	ctx := &commands.Context{QueryLoop: &stubQL{}, SessionStore: store, SessionID: "sess-1", OnCommandPresentation: captureCompletedCommand(&output)}
	if err := r.Find("session").Execute(ctx, "current"); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || !strings.Contains(output.String(), "fix-login") {
		t.Fatalf("expected session metadata output, got %q", output.String())
	}
}

func TestSessionRename_Delegates(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	store := &stubSessionStore{}
	ctx := &commands.Context{QueryLoop: &stubQL{}, SessionStore: store, SessionID: "sess-1", OnEvent: func(string) {}}
	if err := r.Find("session").Execute(ctx, "rename polished-name"); err != nil {
		t.Fatal(err)
	}
	if store.renames["sess-1"] != "polished-name" {
		t.Fatalf("expected rename delegation, got %#v", store.renames)
	}
}

func TestResume_UsesResumeSessionCallback(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	store := &stubSessionStore{
		entries: []commands.SessionListEntry{{
			ID:    "sess-2",
			Title: "resume-me",
		}},
		loads: map[string][]types.Message{
			"sess-2": {types.UserMessage("ignored")},
		},
	}

	var resumed commands.SessionListEntry
	ctx := &commands.Context{
		QueryLoop:    &stubQL{},
		SessionStore: store,
		SessionID:    "sess-1",
		OnEvent:      func(string) {},
		ResumeSession: func(entry commands.SessionListEntry) error {
			resumed = entry
			return nil
		},
	}

	if err := r.Find("resume").Execute(ctx, "resume-me"); err != nil {
		t.Fatal(err)
	}
	if resumed.ID != "sess-2" {
		t.Fatalf("expected ResumeSession callback to receive sess-2, got %+v", resumed)
	}
}

func TestResume_WithoutTransitionFailsClosedWithoutMutatingMessages(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)

	store := &stubSessionStore{
		entries: []commands.SessionListEntry{{ID: "sess-2", Title: "resume-me"}},
		loads:   map[string][]types.Message{"sess-2": {types.UserMessage("target")}},
	}
	ql := &stubQL{messages: []types.Message{types.UserMessage("current")}}
	ctx := &commands.Context{
		QueryLoop:    ql,
		SessionStore: store,
		SessionID:    "sess-1",
		OnEvent:      func(string) {},
	}

	err := r.Find("resume").Execute(ctx, "resume-me")
	if err == nil || !strings.Contains(err.Error(), "session transition is not configured") {
		t.Fatalf("missing transition error = %v", err)
	}
	if len(store.loadCalls) != 0 {
		t.Fatalf("failed-closed resume loaded target transcript: %v", store.loadCalls)
	}
	if got := ql.Messages(); len(got) != 1 || got[0].GetText() != "current" {
		t.Fatalf("failed-closed resume mutated messages: %#v", got)
	}
}

func TestActivityAndDetailCommandsExposeKeyboardControlPath(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	var opened bool
	var activityID, action, disclosureID, level string
	ctx := &commands.Context{
		OnEvent:           func(string) {},
		OpenActivityView:  func() string { opened = true; return "opened" },
		CloseActivityView: func() string { opened = false; return "closed" },
		ActivityAction: func(id, requested string) (string, error) {
			activityID, action = id, requested
			return "done", nil
		},
		SetDisclosure: func(id, requested string) (string, error) {
			disclosureID, level = id, requested
			return "done", nil
		},
	}
	if err := r.Find("activity").Execute(ctx, "list"); err != nil || !opened {
		t.Fatalf("activity list = opened %v, err %v", opened, err)
	}
	if err := r.Find("activity").Execute(ctx, "background:task-7 cancel"); err != nil {
		t.Fatal(err)
	}
	if activityID != "background:task-7" || action != "cancel" {
		t.Fatalf("activity action = %q/%q", activityID, action)
	}
	if err := r.Find("activity").Execute(ctx, "close"); err != nil || opened {
		t.Fatalf("activity close = opened %v, err %v", opened, err)
	}
	if err := r.Find("detail").Execute(ctx, "tool:session:toolu evidence"); err != nil {
		t.Fatal(err)
	}
	if disclosureID != "tool:session:toolu" || level != "evidence" {
		t.Fatalf("disclosure action = %q/%q", disclosureID, level)
	}
}

func TestEditorCommandOpensTranscriptOrSingleObservationEvidence(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	var transcriptPath, observationID string
	ctx := &commands.Context{
		OpenTranscriptEditor: func(path string) error { transcriptPath = path; return nil },
		OpenDetailEditor:     func(id string) error { observationID = id; return nil },
	}
	if err := r.Find("editor").Execute(ctx, "audit.txt"); err != nil || transcriptPath != "audit.txt" {
		t.Fatalf("transcript editor = path %q err %v", transcriptPath, err)
	}
	if err := r.Find("editor").Execute(ctx, "detail observation-7"); err != nil || observationID != "observation-7" {
		t.Fatalf("detail editor = observation %q err %v", observationID, err)
	}
}

func TestBuiltinCallbackErrorsUseActiveLanguageAndPreserveCause(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cause := errors.New("backend offline")
	tests := []struct {
		name    string
		command string
		args    string
		want    string
		context *commands.Context
	}{
		{
			name:    "activity",
			command: "activity",
			args:    "background:task-7 cancel",
			want:    "无法执行活动操作“取消”",
			context: &commands.Context{ActivityAction: func(string, string) (string, error) {
				return "", cause
			}},
		},
		{
			name:    "detail",
			command: "detail",
			args:    "observation-7 evidence",
			want:    "无法更改 observation-7 的详细信息",
			context: &commands.Context{SetDisclosure: func(string, string) (string, error) {
				return "", cause
			}},
		},
		{
			name:    "search",
			command: "search",
			args:    "needle",
			want:    "无法搜索对话记录",
			context: &commands.Context{SearchTranscript: func(string) (string, error) {
				return "", cause
			}},
		},
		{
			name:    "export",
			command: "export",
			want:    "无法导出对话记录",
			context: &commands.Context{ExportTranscript: func(string) (string, error) {
				return "", cause
			}},
		},
		{
			name:    "editor",
			command: "editor",
			want:    "无法打开编辑器",
			context: &commands.Context{OpenTranscriptEditor: func(string) error {
				return cause
			}},
		},
		{
			name:    "mouse",
			command: "mouse",
			args:    "on",
			want:    "无法更新鼠标捕获设置",
			context: &commands.Context{SetMouseCapture: func(string) (bool, error) {
				return false, cause
			}},
		},
		{
			name:    "session delete",
			command: "session",
			args:    "delete sess-7",
			want:    "无法删除会话 sess-7",
			context: &commands.Context{DeleteHistory: func(string) error {
				return cause
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.context.Language = i18n.LangZH
			tt.context.OnEvent = func(string) {}
			err := r.Find(tt.command).Execute(tt.context, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want localized prefix %q", err, tt.want)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("error must preserve callback cause: %v", err)
			}
		})
	}
}
