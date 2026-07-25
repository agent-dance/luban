package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/types"
)

type renderStructuredRecorder struct {
	callContext   presentation.ToolEventContext
	resultContext presentation.ToolEventContext
	call          types.ToolUseBlock
	result        types.ToolResultBlock
	hookContext   presentation.ToolEventContext
	hook          presentation.HookSummary
	errorContext  presentation.ToolEventContext
	errorToolID   string
	errorMessage  string
}

func (r *renderStructuredRecorder) Text(string)                                          {}
func (r *renderStructuredRecorder) Thinking(string)                                      {}
func (r *renderStructuredRecorder) Error(string)                                         {}
func (r *renderStructuredRecorder) Info(string)                                          {}
func (r *renderStructuredRecorder) Success(string)                                       {}
func (r *renderStructuredRecorder) Warning(string)                                       {}
func (r *renderStructuredRecorder) Bold(string)                                          {}
func (r *renderStructuredRecorder) Usage(*types.Usage)                                   {}
func (r *renderStructuredRecorder) Banner(string, string)                                {}
func (r *renderStructuredRecorder) SessionInfo(string, []string)                         {}
func (r *renderStructuredRecorder) Prompt() string                                       { return "" }
func (r *renderStructuredRecorder) Newline()                                             {}
func (r *renderStructuredRecorder) Goodbye()                                             {}
func (r *renderStructuredRecorder) CostSummary(float64, float64, int, int)               {}
func (r *renderStructuredRecorder) ContextBar(int, int)                                  {}
func (r *renderStructuredRecorder) SpinnerStart(string) func()                           { return func() {} }
func (r *renderStructuredRecorder) PermissionRequest(string, map[string]any, int) string { return "n" }
func (r *renderStructuredRecorder) RenderToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	r.callContext, r.call = ctx, call
}
func (r *renderStructuredRecorder) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	r.resultContext, r.result = ctx, result
}
func (r *renderStructuredRecorder) RenderHookSummary(ctx presentation.ToolEventContext, summary presentation.HookSummary) {
	r.hookContext, r.hook = ctx, summary
}
func (r *renderStructuredRecorder) RuntimeErrorEvent(ctx presentation.ToolEventContext, toolUseID, message string, _ *types.APIError, _ map[string]any) {
	r.errorContext, r.errorToolID, r.errorMessage = ctx, toolUseID, message
}

func TestDropTextInBriefTurnEventRenderer(t *testing.T) {
	var rendered bytes.Buffer
	handler := makeEventHandler(ui.NewTermRenderer(&rendered), false)
	handler(stream.Event{Type: stream.EventText, Text: "duplicate model text"})
	handler(stream.Event{Type: stream.EventToolUse, ToolUse: &types.ToolUseBlock{
		Name: "SendUserMessage", Input: map[string]any{"message": "visible message", "status": "normal"},
	}})
	handler(stream.Event{Type: stream.EventToolResult, ToolResult: &types.ToolResultBlock{
		Content: "Message delivered to user.",
		Data:    interaction.SendUserMessageOutput{Message: "visible message"},
		Outcome: types.ToolOutcomeSucceeded,
	}})
	handler(stream.Event{Type: stream.EventTurnEnd})

	text := rendered.String()
	if strings.Contains(text, "duplicate model text") {
		t.Fatalf("same-turn assistant text was duplicated: %q", text)
	}
	if strings.Count(text, "visible message") != 1 {
		t.Fatalf("visible Brief count = %d in %q", strings.Count(text, "visible message"), text)
	}
	for _, unwanted := range []string{"SendUserMessage", "Message delivered to user.", "⚡", "↳"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("Brief event render contains generic chrome %q in %q", unwanted, text)
		}
	}
}

func TestNonBriefTurnEventRendererFlushesAssistantText(t *testing.T) {
	var rendered bytes.Buffer
	handler := makeEventHandler(ui.NewTermRenderer(&rendered), false)
	handler(stream.Event{Type: stream.EventText, Text: "ordinary response"})
	if rendered.Len() != 0 {
		t.Fatalf("text must remain buffered until turn shape is known: %q", rendered.String())
	}
	handler(stream.Event{Type: stream.EventTurnEnd})
	if rendered.String() != "ordinary response" {
		t.Fatalf("ordinary response = %q", rendered.String())
	}
}

func TestDropTextInBriefTurnWithConcurrentSiblingTool(t *testing.T) {
	var rendered bytes.Buffer
	handler := makeEventHandler(ui.NewTermRenderer(&rendered), false)
	handler(stream.Event{Type: stream.EventText, Text: "same-turn duplicate", TurnCount: 4})
	handler(stream.Event{Type: stream.EventToolUse, TurnCount: 4, ToolUse: &types.ToolUseBlock{Name: "Read"}})
	handler(stream.Event{Type: stream.EventToolResult, TurnCount: 4, ToolResult: &types.ToolResultBlock{Content: "read result", Outcome: types.ToolOutcomeSucceeded}})
	handler(stream.Event{Type: stream.EventToolUse, TurnCount: 4, ToolUse: &types.ToolUseBlock{Name: "SendUserMessage"}})
	handler(stream.Event{Type: stream.EventToolResult, TurnCount: 4, ToolResult: &types.ToolResultBlock{
		Data:    interaction.SendUserMessageOutput{Message: "visible once"},
		Outcome: types.ToolOutcomeSucceeded,
	}})
	handler(stream.Event{Type: stream.EventTurnEnd, TurnCount: 4})
	text := rendered.String()
	if strings.Contains(text, "same-turn duplicate") || strings.Count(text, "visible once") != 1 {
		t.Fatalf("concurrent sibling Brief render = %q", text)
	}
}

func TestEventRendererPreservesStructuredLoopIdentity(t *testing.T) {
	recorder := &renderStructuredRecorder{}
	handler := makeEventHandler(recorder, false)
	identity := stream.Event{
		ProjectRoot: "/workspace/project", TurnID: "session-render:query-9:turn-3", ActorID: "agent-7", ActorType: "executor", WorkUnitID: "work-7",
	}
	call := identity
	call.Type = stream.EventToolUse
	call.ToolUse = &types.ToolUseBlock{ID: "toolu-render", Name: "Bash", Input: map[string]any{"command": "pwd"}}
	handler(call)
	result := identity
	result.Type = stream.EventToolResult
	result.ToolResult = &types.ToolResultBlock{ToolUseID: "toolu-render", Content: "done", Outcome: types.ToolOutcomeSucceeded}
	handler(result)

	want := presentation.ToolEventContext{SessionID: "session-render", ProjectRoot: identity.ProjectRoot, TurnID: identity.TurnID, ActorID: identity.ActorID, ActorType: identity.ActorType, WorkUnitID: identity.WorkUnitID}
	if recorder.callContext != want || recorder.resultContext != want {
		t.Fatalf("contexts = call %+v result %+v, want %+v", recorder.callContext, recorder.resultContext, want)
	}
	if recorder.call.ID != "toolu-render" || recorder.result.ToolUseID != "toolu-render" || recorder.result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("structured tool payload was downgraded: call=%+v result=%+v", recorder.call, recorder.result)
	}
}

func TestEventRendererPreservesHookAndRuntimeErrorIdentity(t *testing.T) {
	recorder := &renderStructuredRecorder{}
	handler := makeEventHandler(recorder, false)
	identity := stream.Event{ProjectRoot: "/workspace/project", TurnID: "session-render:query-3:turn-8", ActorID: "agent-8", ActorType: "reviewer", WorkUnitID: "review-8"}
	hook := identity
	hook.Type = stream.EventHookSummary
	hook.HookSummary = &stream.HookSummaryEvent{HookExecutionID: "hook-8", ToolUseID: "toolu-8", HookName: "PostToolUse", Status: "blocked", Summary: "policy blocked"}
	handler(hook)
	runtimeErr := identity
	runtimeErr.Type, runtimeErr.ToolUseID, runtimeErr.Text = stream.EventError, "toolu-8", "tool failed"
	handler(runtimeErr)

	want := presentation.ToolEventContext{SessionID: "session-render", ProjectRoot: identity.ProjectRoot, TurnID: identity.TurnID, ActorID: identity.ActorID, ActorType: identity.ActorType, WorkUnitID: identity.WorkUnitID}
	if recorder.hookContext != want || recorder.hook.ToolUseID != "toolu-8" || recorder.hook.ExecutionID != "hook-8" || recorder.hook.Status != "blocked" {
		t.Fatalf("hook identity = ctx %+v summary %+v", recorder.hookContext, recorder.hook)
	}
	if recorder.errorContext != want || recorder.errorToolID != "toolu-8" || recorder.errorMessage != "tool failed" {
		t.Fatalf("runtime error identity = ctx %+v tool %q message %q", recorder.errorContext, recorder.errorToolID, recorder.errorMessage)
	}
}
