package agent

// These tests exercise the Agent runtime's current public behavior. Deeper
// integration coverage lives beside each runtime component.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

// ---------------------------------------------------------------------------
// Agent definitions registry.
// ---------------------------------------------------------------------------

func TestConformance_Definitions_RuntimeOptOutHidesBuiltins(t *testing.T) {
	tool := &AgentTool{NonInteractive: true}
	t.Setenv("LUBAN_AGENT_SDK_DISABLE_BUILTIN_AGENTS", "1")
	defs, err := tool.LoadAgentDefinitionsForRuntime("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, d := range defs {
		if d.Source == "builtin" {
			t.Fatalf("expected built-ins to be suppressed, found %q", d.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// forkSubagent guard.
// ---------------------------------------------------------------------------

func TestConformance_Fork_DepthGuardReached(t *testing.T) {
	tool := &AgentTool{Depth: DefaultMaxAgentDepth}
	res, err := tool.Execute(context.Background(), agentExecuteInput("nested", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := toolRuntimeFormat(i18n.KeyToolAgentMaxDepth, DefaultMaxAgentDepth)
	if !res.IsError || res.Content != want {
		t.Fatalf("expected depth guard error, got %#v", res)
	}
}

// ---------------------------------------------------------------------------
// Progress streaming emitter.
// ---------------------------------------------------------------------------

func TestConformance_Progress_EmitsAndCloses(t *testing.T) {
	em := newAgentProgressEmitter("a", "general")
	var events []agentcontract.ProgressEvent
	em.AddObserver(func(event agentcontract.ProgressEvent) {
		events = append(events, event)
	})
	em.EmitPhase(agentcontract.ProgressStart, 0, "")
	em.EmitPhase(agentcontract.ProgressRunning, 1, "Bash")
	em.Finish(agentcontract.ProgressCompleted, "done")
	if len(events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(events))
	}
	last := events[len(events)-1]
	if last.Phase != agentcontract.ProgressCompleted {
		t.Fatalf("expected terminal phase completed, got %q", last.Phase)
	}
}

func TestConformance_Progress_FinishIsIdempotent(t *testing.T) {
	em := newAgentProgressEmitter("a", "g")
	if !em.Finish(agentcontract.ProgressCompleted, "first") {
		t.Fatal("first Finish should return true")
	}
	if em.Finish(agentcontract.ProgressError, "second") {
		t.Fatal("second Finish must be a no-op")
	}
	if !em.Closed() {
		t.Fatal("emitter must be closed after Finish")
	}
}

func TestConformance_Progress_EmitDropsWhenClosed(t *testing.T) {
	em := newAgentProgressEmitter("a", "g")
	em.Finish(agentcontract.ProgressAborted, "")
	if em.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressRunning}) {
		t.Fatal("emit after Finish must return false")
	}
}

// ---------------------------------------------------------------------------
// MCP readiness gate.
// ---------------------------------------------------------------------------

type fakeMCPReadiness struct {
	servers      []string
	connectErrs  map[string]error
	connectCalls map[string]int
	failUntil    map[string]int
}

func (f *fakeMCPReadiness) ServerNames() []string { return f.servers }

func (f *fakeMCPReadiness) GetOrConnect(_ context.Context, name string) (mcpmanager.MCPServerConnection, error) {
	if f.connectCalls == nil {
		f.connectCalls = map[string]int{}
	}
	f.connectCalls[name]++
	if cnt, ok := f.failUntil[name]; ok && f.connectCalls[name] <= cnt {
		return mcpmanager.MCPServerConnection{}, errors.New("not ready yet")
	}
	if err, ok := f.connectErrs[name]; ok {
		return mcpmanager.MCPServerConnection{}, err
	}
	return mcpmanager.MCPServerConnection{Name: name, Type: mcpmanager.MCPStateConnected}, nil
}

func TestConformance_MCPReadiness_RequiredServersFromAllowList(t *testing.T) {
	got := RequiredMCPServersFromAllowList([]string{
		"Bash", "mcp__github__*", "mcp__notion__search", "mcp__*",
	})
	want := map[string]struct{}{"github": {}, "notion": {}}
	if len(got) != len(want) {
		t.Fatalf("expected %d servers, got %v", len(want), got)
	}
	for _, s := range got {
		if _, ok := want[s]; !ok {
			t.Fatalf("unexpected server %q", s)
		}
	}
}

func TestConformance_MCPReadiness_AllReady(t *testing.T) {
	probe := &fakeMCPReadiness{servers: []string{"github", "notion"}}
	report, err := WaitForMCPReadiness(context.Background(), probe, []string{"github", "notion"}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected ready, got %v", err)
	}
	if len(report.Failed) != 0 || len(report.Ready) != len(report.Required) {
		t.Fatalf("report should be ready: %+v", report)
	}
}

func TestConformance_MCPReadiness_MissingConfigErrors(t *testing.T) {
	probe := &fakeMCPReadiness{servers: []string{"github"}}
	_, err := WaitForMCPReadiness(context.Background(), probe, []string{"missing"}, 50*time.Millisecond)
	want := toolRuntimeFormat(i18n.KeyToolAgentMCPRequiredServersNotConfigured, "missing")
	if err == nil || err.Error() != want {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestConformance_MCPReadiness_RetryUntilReady(t *testing.T) {
	probe := &fakeMCPReadiness{
		servers:   []string{"slow"},
		failUntil: map[string]int{"slow": 1},
	}
	report, err := WaitForMCPReadiness(context.Background(), probe, []string{"slow"}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected eventual ready: %v", err)
	}
	if len(report.Failed) != 0 || len(report.Ready) != len(report.Required) {
		t.Fatalf("report should be ready: %+v", report)
	}
	if probe.connectCalls["slow"] < 2 {
		t.Fatalf("expected at least one retry, got %d calls", probe.connectCalls["slow"])
	}
}

func TestConformance_MCPReadiness_EmptyRequiredIsTrivial(t *testing.T) {
	report, err := WaitForMCPReadiness(context.Background(), nil, nil, 0)
	if err != nil || len(report.Failed) != 0 || len(report.Ready) != len(report.Required) {
		t.Fatalf("empty required should succeed; got err=%v report=%+v", err, report)
	}
}

func TestConformance_MCPReadiness_NilProbeWithRequirementsErrors(t *testing.T) {
	_, err := WaitForMCPReadiness(context.Background(), nil, []string{"a"}, 0)
	want := toolRuntimeText(i18n.KeyToolAgentMCPManagerNotConfigured)
	if err == nil || err.Error() != want {
		t.Fatalf("expected manager error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// run_in_background contract.
// ---------------------------------------------------------------------------

func TestConformance_Background_DisabledFlagShortCircuits(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "1")
	if !agentBackgroundTasksDisabled() {
		t.Fatal("expected background tasks disabled")
	}
}

// ---------------------------------------------------------------------------
// Transcript persistence (path round-trip).
// ---------------------------------------------------------------------------

func TestConformance_Transcript_PathThroughCompletedFormatter(t *testing.T) {
	summary := agentRunSummary{
		AgentID:        "a-3",
		AgentType:      "general-purpose",
		Output:         "ok",
		TranscriptPath: "/tmp/agent-a3.jsonl",
		LatestToolUse:  "Bash",
	}
	out := formatCompletedAgentResult(summary)
	if !strings.Contains(out, `"transcriptPath":"/tmp/agent-a3.jsonl"`) {
		t.Fatalf("expected transcriptPath in output, got %s", out)
	}
	if !strings.Contains(out, `"latestToolUse":"Bash"`) {
		t.Fatalf("expected latestToolUse in output, got %s", out)
	}
	if !strings.Contains(out, `"kind":"completed"`) {
		t.Fatalf("expected kind=completed in output, got %s", out)
	}
}

func TestConformance_AsyncLaunch_PayloadSchema(t *testing.T) {
	out := formatAsyncAgentLaunchResult("a-9", "desc", "prompt", "/tmp/y", true)
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("payload not valid JSON: %v\n%s", err, out)
	}
	if got["status"] != "async_launched" || got["kind"] != "partial" {
		t.Fatalf("expected status=async_launched kind=partial, got %s", out)
	}
	if got["isAsync"] != true {
		t.Fatalf("expected isAsync=true, got %s", out)
	}
}

func TestConformance_Validation_RejectsCWDWithWorktreeIsolation(t *testing.T) {
	tool := &AgentTool{}
	err := tool.validateAgentInvocation(agentcontract.Input{
		Description: "x", Prompt: "y", CWD: "/tmp", Isolation: "worktree",
	})
	want := toolRuntimeText(i18n.KeyToolAgentDeepCWDWorktreeConflict)
	if err == nil || err.Error() != want {
		t.Fatalf("expected cwd+worktree rejection, got %v", err)
	}
}

func TestConformance_Validation_RejectsUnknownIsolation(t *testing.T) {
	tool := &AgentTool{}
	err := tool.validateAgentInvocation(agentcontract.Input{
		Description: "x", Prompt: "y", Isolation: "frobnicate",
	})
	want := toolRuntimeFormat(i18n.KeyToolAgentDeepIsolationUnsupported, "frobnicate")
	if err == nil || err.Error() != want {
		t.Fatalf("expected unsupported isolation error, got %v", err)
	}
}
