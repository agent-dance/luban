package app

import (
	"bytes"
	"context"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/types"
)

type replHookCaptureRenderer struct {
	ui.QuietRenderer
	mu        sync.Mutex
	contexts  []presentation.ToolEventContext
	summaries []presentation.HookSummary
}

func (r *replHookCaptureRenderer) RenderHookSummary(ctx presentation.ToolEventContext, summary presentation.HookSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.contexts = append(r.contexts, ctx)
	r.summaries = append(r.summaries, summary)
}

func TestFullscreenREPLHookRendersEveryConfigWithCausalEvidence(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookUserPromptSubmit, Command: `printf '%s' '{"system_reminder":"allowed by policy"}'`},
		{Type: hooks.HookUserPromptSubmit, Command: `printf 'raw block reason' >&2; exit 2`},
	})
	renderer := &replHookCaptureRenderer{}
	hookCtx := newREPLHookContext("session-fullscreen", hooks.HookUserPromptSubmit, "user", "local")
	result := runObservedREPLHooks(context.Background(), runner, renderer, hooks.HookUserPromptSubmit, hooks.HookInput{
		UserInput: "explain causality",
	}, hookCtx)

	if !result.Blocked || result.Reason != "raw block reason" {
		t.Fatalf("hook result = %#v, want blocked with raw reason", result)
	}
	if len(renderer.summaries) != 2 || len(renderer.contexts) != 2 {
		t.Fatalf("rendered summaries=%d contexts=%d, want two actual configs", len(renderer.summaries), len(renderer.contexts))
	}
	if renderer.summaries[0].ExecutionID == renderer.summaries[1].ExecutionID {
		t.Fatalf("actual configs reused execution ID %q", renderer.summaries[0].ExecutionID)
	}
	for index, summary := range renderer.summaries {
		ctx := renderer.contexts[index]
		if ctx.SessionID != "session-fullscreen" || ctx.TurnID == "" || ctx.WorkUnitID == "" || ctx.ActorID != "user" || ctx.ActorType != "local" {
			t.Fatalf("summary %d lost causal context: %#v", index, ctx)
		}
		wantConfig := "config-" + string(rune('1'+index))
		if summary.Metadata["config_id"] != wantConfig {
			t.Fatalf("summary %d config identity = %#v", index, summary.Metadata)
		}
		input, ok := summary.Metadata["hook_input"].(hooks.HookInput)
		if !ok || input.SessionID != ctx.SessionID || input.TurnID != ctx.TurnID || input.WorkUnitID != ctx.WorkUnitID || input.AgentID != ctx.ActorID || input.AgentType != ctx.ActorType || input.UserInput != "explain causality" {
			t.Fatalf("summary %d hook input lost identity: %#v", index, summary.Metadata["hook_input"])
		}
		if input.HookConfigID != wantConfig || input.HookExecutionID != summary.ExecutionID {
			t.Fatalf("summary %d config/execution input identity = %#v", index, input)
		}
	}
	passedOutput, ok := renderer.summaries[0].Metadata["hook_output"].(hooks.HookOutput)
	if !ok || passedOutput.Stdout == "" || passedOutput.StdoutBytes == 0 {
		t.Fatalf("pass summary lost raw stdout evidence: %#v", renderer.summaries[0].Metadata)
	}
	blockedOutput, ok := renderer.summaries[1].Metadata["hook_output"].(hooks.HookOutput)
	if !ok || blockedOutput.Stderr != "raw block reason" || blockedOutput.StderrBytes != int64(len("raw block reason")) {
		t.Fatalf("block summary lost raw stderr evidence: %#v", renderer.summaries[1].Metadata)
	}
	if renderer.summaries[0].Status != "passed" || renderer.summaries[1].Status != "blocked" || renderer.summaries[1].Summary != "raw block reason" {
		t.Fatalf("summary status/reason = %#v", renderer.summaries)
	}
}

func TestFullscreenBlockedUserPromptRefusesQueryAfterRenderingEvidence(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookUserPromptSubmit, Command: `printf '%s' '{"block":true,"system_reminder":"fullscreen policy refusal"}'`,
	}})
	renderer := &replHookCaptureRenderer{}
	queries := 0
	result := runObservedREPLHooks(context.Background(), runner, renderer, hooks.HookUserPromptSubmit, hooks.HookInput{UserInput: "blocked"},
		newREPLHookContext("session-fullscreen-block", hooks.HookUserPromptSubmit, "user", "local"))
	if !result.Blocked {
		queries++
	}
	if queries != 0 {
		t.Fatalf("engine query calls=%d after blocked fullscreen prompt", queries)
	}
	if len(renderer.summaries) != 1 || renderer.summaries[0].Summary != "fullscreen policy refusal" {
		t.Fatalf("blocked evidence was not rendered before refusal: %#v", renderer.summaries)
	}
}

type replHookSessionManager struct{}

func (replHookSessionManager) Save(string, []types.Message) error { return nil }
func (replHookSessionManager) Load(string) ([]types.Message, error) {
	return nil, engine.ErrSessionNotFound
}
func (replHookSessionManager) List() ([]engine.SessionInfo, error) { return nil, nil }
func (replHookSessionManager) Latest() (string, error)             { return "", engine.ErrSessionNotFound }
func (replHookSessionManager) Delete(string) error                 { return nil }

type replHookEngine struct {
	screenReaderLifecycleEngine
	mu      sync.Mutex
	queries []engine.QueryRequest
}

func (e *replHookEngine) Query(_ context.Context, req engine.QueryRequest) (<-chan engine.Event, error) {
	e.mu.Lock()
	e.queries = append(e.queries, req)
	e.mu.Unlock()
	ch := make(chan engine.Event, 1)
	ch <- engine.Event{SessionID: req.SessionID, Final: true}
	close(ch)
	return ch, nil
}

func (*replHookEngine) Sessions() engine.SessionManager { return replHookSessionManager{} }
func (*replHookEngine) Tools() []string                 { return nil }

func (e *replHookEngine) queryCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.queries)
}

func TestScreenReaderREPLHookLifecycleAndPassingPromptAreStructured(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookSessionStart, Command: `printf '%s' '{"system_reminder":"start pass"}'`},
		{Type: hooks.HookSessionEnd, Command: `printf '%s' '{"system_reminder":"end pass"}'`},
		{Type: hooks.HookUserPromptSubmit, Command: `printf '%s' '{"system_reminder":"prompt pass"}'`},
	})
	sessionID := "session-screen-pass"
	eng := &replHookEngine{}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader("hello hooks\n/exit\n"))
	if err := RunScreenReaderREPL(context.Background(), TUIREPLConfig{
		Engine: eng, SessionID: &sessionID, HookRunnerRef: &runner,
	}, renderer, nil); err != nil {
		t.Fatal(err)
	}
	if eng.queryCount() != 1 {
		t.Fatalf("passing prompt query calls=%d, want 1", eng.queryCount())
	}
	text := output.String()
	for _, want := range []string{
		"Hook finished: SessionStart", "Hook finished: UserPromptSubmit", "Hook finished: SessionEnd",
		"start pass", "prompt pass", "end pass", "Execution ID: hook:", "Turn: ", "Work unit: ", "Actor: ",
		`"config_id":"config-`, `"hook_input"`, `"hook_output"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader lifecycle/prompt output missing %q:\n%s", want, text)
		}
	}
}

func TestScreenReaderBlockedUserPromptRendersReasonAndSkipsEngineQuery(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookUserPromptSubmit, Command: `printf '%s' '{"system_reminder":"first config passed"}'`},
		{Type: hooks.HookUserPromptSubmit, Command: `printf 'screen-reader policy refusal' >&2; exit 2`},
	})
	sessionID := "session-screen-block"
	eng := &replHookEngine{}
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader("blocked prompt\n/exit\n"))
	if err := RunScreenReaderREPL(context.Background(), TUIREPLConfig{
		Engine: eng, SessionID: &sessionID, HookRunnerRef: &runner,
	}, renderer, nil); err != nil {
		t.Fatal(err)
	}
	if eng.queryCount() != 0 {
		t.Fatalf("blocked prompt reached engine Query %d time(s)", eng.queryCount())
	}
	text := output.String()
	first := strings.Index(text, "first config passed")
	refusal := strings.Index(text, "screen-reader policy refusal")
	receipt := strings.Index(text, "Input blocked by hook: screen-reader policy refusal")
	if first < 0 || refusal <= first || receipt <= refusal {
		t.Fatalf("blocked prompt did not render per-config evidence and reason before refusal: pass=%d refusal=%d receipt=%d\n%s", first, refusal, receipt, text)
	}
	if strings.Count(text, "Hook finished: UserPromptSubmit") != 2 {
		t.Fatalf("screen-reader summaries did not preserve both actual configs:\n%s", text)
	}
}

func TestScreenReaderHookSummaryForwardingPreservesSourceToolUseID(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	handler := makeScreenReaderEventHandler(renderer, nil, nil, presentation.ToolEventContext{SessionID: "session-forward"})
	handler(loopEventHookSummaryForTest("tool-source"))
	if !strings.Contains(output.String(), "Source tool use ID: tool-source") {
		t.Fatalf("screen-reader hook forwarding lost ToolUseID: %q", output.String())
	}
}

func loopEventHookSummaryForTest(toolUseID string) stream.Event {
	return stream.Event{
		Type: stream.EventHookSummary, TurnID: "turn-forward", ActorID: "actor-forward", ActorType: "assistant", WorkUnitID: "work-forward",
		HookSummary: &stream.HookSummaryEvent{HookExecutionID: "hook-forward", ToolUseID: toolUseID, HookName: "PostToolUse", Status: "passed"},
	}
}
