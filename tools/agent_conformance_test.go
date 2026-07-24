package tools

// agent_conformance_test.go covers tasks/agent.json subtask agent-11. It bundles
// at least 30 conformance cases across the agent runtime+output surface,
// exercising the discriminated-union output schema (agent-01), the agent
// definition registry (agent-02), forkSubagent guards (agent-03), progress
// streaming (agent-04), isolation modes (agent-05/-06), MCP readiness gating
// (agent-07), background launches (agent-08), transcript persistence (agent-09)
// and the AutoClassifierInput / UserFacingName methods (agent-10).
//
// These are unit-level conformance assertions; deeper integration coverage is
// provided by the per-subtask test files (agent_lifecycle_test.go,
// agent_fork_test.go, agent_send_message_test.go, etc.).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// ---------------------------------------------------------------------------
// agent-01: discriminated-union output schema
// ---------------------------------------------------------------------------

func TestConformance_OutputUnion_CompletedKindRoundtrips(t *testing.T) {
	completed := AgentCompleted{
		AgentResultBase: AgentResultBase{
			Kind:        AgentResultKindCompleted,
			DurationMs:  1234,
			TotalTokens: 100,
		},
		AgentID:   "a-1",
		AgentType: "general-purpose",
		Content:   []agentToolContentBlock{{Type: "text", Text: "ok"}},
	}
	data, err := MarshalAgentResult(completed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"kind":"completed"`) {
		t.Fatalf("expected kind=completed in payload, got %s", data)
	}
	got, err := UnmarshalAgentResult(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ResultKind() != AgentResultKindCompleted {
		t.Fatalf("expected completed kind, got %q", got.ResultKind())
	}
}

func TestConformance_OutputUnion_ErrorKindRoundtrips(t *testing.T) {
	in := AgentError{
		AgentResultBase: AgentResultBase{Kind: AgentResultKindError, DurationMs: 5},
		Message:         "boom",
	}
	data, err := MarshalAgentResult(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalAgentResult(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ResultKind() != AgentResultKindError {
		t.Fatalf("expected error kind, got %q", got.ResultKind())
	}
}

func TestConformance_OutputUnion_AbortedKindRoundtrips(t *testing.T) {
	in := AgentAborted{AgentResultBase: AgentResultBase{Kind: AgentResultKindAborted}, Reason: "user"}
	data, err := MarshalAgentResult(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalAgentResult(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ResultKind() != AgentResultKindAborted {
		t.Fatalf("expected aborted kind, got %q", got.ResultKind())
	}
}

func TestConformance_OutputUnion_PartialKindRoundtrips(t *testing.T) {
	in := AgentResultFromAsyncLaunch("a-2", "general", "desc", "prompt", "/tmp/x.jsonl", true)
	data, err := MarshalAgentResult(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalAgentResult(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ResultKind() != AgentResultKindPartial {
		t.Fatalf("expected partial kind, got %q", got.ResultKind())
	}
}

func TestConformance_OutputUnion_IncompleteKindsRoundtrip(t *testing.T) {
	for _, outcome := range []AgentRunOutcome{
		AgentRunOutcomePartial,
		AgentRunOutcomeTimedOut,
		AgentRunOutcomeCancelled,
		AgentRunOutcomeInterrupted,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			in := AgentResultFromIncomplete(agentRunSummary{AgentID: "agent", Outcome: outcome, TerminalReason: string(outcome)}, "transcript")
			data, err := MarshalAgentResult(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got, err := UnmarshalAgentResult(data)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			incomplete, ok := got.(AgentIncomplete)
			if !ok || incomplete.Outcome != outcome || incomplete.ResultKind() != agentResultKindForRunOutcome(outcome) {
				t.Fatalf("roundtrip=%+v (%T)", got, got)
			}
		})
	}
}

func TestConformance_OutputUnion_StatusFallbackToKind(t *testing.T) {
	// Wire payloads from the TS reference use "status" instead of "kind".
	payload := []byte(`{"status":"completed","durationMs":1,"totalTokens":2}`)
	got, err := UnmarshalAgentResult(payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ResultKind() != AgentResultKindCompleted {
		t.Fatalf("status fallback failed: got %q", got.ResultKind())
	}
}

func TestConformance_OutputUnion_UnknownKindIsError(t *testing.T) {
	_, err := UnmarshalAgentResult([]byte(`{"kind":"frobnicated"}`))
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

// ---------------------------------------------------------------------------
// agent-02: agent definitions registry
// ---------------------------------------------------------------------------

func TestConformance_Definitions_BuiltInsExposed(t *testing.T) {
	defs, err := LoadAgentDefinitions("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected at least one built-in definition")
	}
	hasGeneral := false
	for _, d := range defs {
		if strings.EqualFold(d.Name, "general-purpose") {
			hasGeneral = true
			break
		}
	}
	if !hasGeneral {
		t.Fatal("expected general-purpose to be present in built-in definitions")
	}
}

func TestConformance_Definitions_RuntimeOptOutHidesBuiltins(t *testing.T) {
	tool := &AgentTool{NonInteractive: true}
	t.Setenv("CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS", "1")
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
// agent-03: forkSubagent guard
// ---------------------------------------------------------------------------

func TestConformance_Fork_DepthGuardReached(t *testing.T) {
	tool := &AgentTool{Depth: DefaultMaxAgentDepth}
	res, err := tool.Execute(context.Background(), agentExecuteInput("nested", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "maximum agent nesting depth") {
		t.Fatalf("expected depth guard error, got %#v", res)
	}
}

// ---------------------------------------------------------------------------
// agent-04: progress streaming emitter
// ---------------------------------------------------------------------------

func TestConformance_Progress_EmitsAndCloses(t *testing.T) {
	em := NewAgentProgressEmitter("a", "general", 4)
	em.EmitPhase(AgentPhaseStart, 0, "")
	em.EmitPhase(AgentPhaseRunning, 1, "Bash")
	em.Finish(AgentPhaseCompleted, "done")
	events := CollectAgentProgress(em)
	if len(events) < 2 {
		t.Fatalf("expected >=2 events, got %d", len(events))
	}
	last := events[len(events)-1]
	if last.Phase != AgentPhaseCompleted {
		t.Fatalf("expected terminal phase completed, got %q", last.Phase)
	}
}

func TestConformance_Progress_FinishIsIdempotent(t *testing.T) {
	em := NewAgentProgressEmitter("a", "g", 2)
	if !em.Finish(AgentPhaseCompleted, "first") {
		t.Fatal("first Finish should return true")
	}
	if em.Finish(AgentPhaseError, "second") {
		t.Fatal("second Finish must be a no-op")
	}
	if !em.Closed() {
		t.Fatal("emitter must be closed after Finish")
	}
}

func TestConformance_Progress_EmitDropsWhenClosed(t *testing.T) {
	em := NewAgentProgressEmitter("a", "g", 1)
	em.Finish(AgentPhaseAborted, "")
	if em.Emit(AgentProgressEvent{Phase: AgentPhaseRunning}) {
		t.Fatal("emit after Finish must return false")
	}
}

func TestConformance_Progress_BackpressureDropsOldest(t *testing.T) {
	em := NewAgentProgressEmitter("a", "g", 2)
	em.EmitPhase(AgentPhaseStart, 0, "")
	em.EmitPhase(AgentPhaseRunning, 1, "")
	em.EmitPhase(AgentPhaseRunning, 2, "")
	// channel capacity is 2 so at least the third emit forced a drop;
	// the emitter should still be open and non-panicking.
	if em.Closed() {
		t.Fatal("backpressure must not close the channel")
	}
	em.Finish(AgentPhaseCompleted, "")
}

// ---------------------------------------------------------------------------
// agent-05: PrepareIsolation
// ---------------------------------------------------------------------------

func TestConformance_Isolation_NoneIsCheap(t *testing.T) {
	res, err := PrepareIsolation(context.Background(), AgentIsolationNone, "a", "/tmp", types.ToolRuntimeContext{PermissionMode: "default"}, nil)
	if err != nil {
		t.Fatalf("none isolation should not error: %v", err)
	}
	if res.Mode != AgentIsolationNone {
		t.Fatalf("expected mode none, got %q", res.Mode)
	}
	if res.Cleanup == nil {
		t.Fatal("cleanup must always be non-nil")
	}
	res.Cleanup() // must be a no-op
}

func TestConformance_Isolation_RemoteRequiresProvider(t *testing.T) {
	_, err := PrepareIsolation(context.Background(), AgentIsolationRemote, "a", "/tmp", types.ToolRuntimeContext{PermissionMode: "default"}, nil)
	if err == nil {
		t.Fatal("expected error when remote isolation has no provider")
	}
	if !strings.Contains(err.Error(), "RemoteRuntimeProvider") {
		t.Fatalf("expected provider message, got %v", err)
	}
}

func TestConformance_Isolation_NormalizeMode(t *testing.T) {
	cases := map[string]AgentIsolationMode{
		"":         AgentIsolationNone,
		"none":     AgentIsolationNone,
		" None ":   AgentIsolationNone,
		"worktree": AgentIsolationWorktree,
		"REMOTE":   AgentIsolationRemote,
	}
	for in, want := range cases {
		if got := NormalizeIsolationMode(in); got != want {
			t.Fatalf("NormalizeIsolationMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// agent-06: RemoteRuntimeProvider
// ---------------------------------------------------------------------------

type fakeRemoteRuntime struct {
	spawnCalls   int
	pollCalls    int
	cleanupCalls int
	pollPhases   []string
	pollErr      error
	spawnErr     error
}

func (*fakeRemoteRuntime) EnforcesPermissionSnapshot() bool { return true }
func (*fakeRemoteRuntime) EnforcesFailClosedPrompts() bool  { return true }

func (f *fakeRemoteRuntime) Spawn(ctx context.Context, req RemoteAgentSpawnRequest) (RemoteAgentLaunch, error) {
	f.spawnCalls++
	if f.spawnErr != nil {
		return RemoteAgentLaunch{}, f.spawnErr
	}
	return RemoteAgentLaunch{TaskID: "task-1", SessionURL: "https://x/y"}, nil
}

func (f *fakeRemoteRuntime) Poll(ctx context.Context, taskID string) (RemoteAgentStatus, error) {
	if f.pollErr != nil {
		return RemoteAgentStatus{}, f.pollErr
	}
	idx := f.pollCalls
	f.pollCalls++
	phase := "running"
	if idx < len(f.pollPhases) {
		phase = f.pollPhases[idx]
	}
	return RemoteAgentStatus{TaskID: taskID, Phase: phase}, nil
}

func (f *fakeRemoteRuntime) Cleanup(taskID string) error {
	f.cleanupCalls++
	return nil
}

func TestConformance_Remote_Validation_AcceptsRemoteWithProvider(t *testing.T) {
	tool := &AgentTool{RemoteRuntime: &fakeRemoteRuntime{}}
	if err := tool.validateAgentInvocation(AgentInput{
		Description: "x", Prompt: "y", Isolation: "remote",
	}); err != nil {
		t.Fatalf("expected remote acceptance, got %v", err)
	}
}

func TestConformance_Remote_Validation_RejectsWithoutProvider(t *testing.T) {
	tool := &AgentTool{}
	err := tool.validateAgentInvocation(AgentInput{
		Description: "x", Prompt: "y", Isolation: "remote",
	})
	if err == nil || !strings.Contains(err.Error(), "RemoteRuntimeProvider") {
		t.Fatalf("expected provider missing error, got %v", err)
	}
}

func TestConformance_Remote_ExecuteSpawnsAndFormatsResult(t *testing.T) {
	fake := &fakeRemoteRuntime{}
	tool := &AgentTool{RemoteRuntime: fake}
	root := t.TempDir()
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault}})
	res, err := tool.Execute(context.Background(), agentExecuteInput("hello", map[string]any{
		"isolation":     "remote",
		"subagent_type": "general-purpose",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"status":"remote_launched"`) {
		t.Fatalf("expected remote_launched status, got %s", res.Content)
	}
	if !strings.Contains(res.Content, `"taskId":"task-1"`) {
		t.Fatalf("expected taskId in payload, got %s", res.Content)
	}
	if fake.spawnCalls != 1 {
		t.Fatalf("expected single spawn call, got %d", fake.spawnCalls)
	}
}

func TestConformance_Remote_PollUntilTerminal_StopsOnCompleted(t *testing.T) {
	fake := &fakeRemoteRuntime{pollPhases: []string{"running", "running", "completed"}}
	em := NewAgentProgressEmitter("a", "g", 4)
	defer em.Finish(AgentPhaseCompleted, "")
	status, err := PollUntilTerminal(context.Background(), fake, RemoteAgentLaunch{TaskID: "task-1"}, em, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if status.Phase != "completed" {
		t.Fatalf("expected completed, got %q", status.Phase)
	}
}

func TestConformance_Remote_PollUntilTerminal_PropagatesErrors(t *testing.T) {
	fake := &fakeRemoteRuntime{pollErr: errors.New("network down")}
	_, err := PollUntilTerminal(context.Background(), fake, RemoteAgentLaunch{TaskID: "x"}, nil, 1*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected network error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// agent-07: MCP readiness gate
// ---------------------------------------------------------------------------

type fakeMCPReadiness struct {
	servers      []string
	connectErrs  map[string]error
	connectCalls map[string]int
	failUntil    map[string]int
}

func (f *fakeMCPReadiness) ServerNames() []string { return f.servers }

func (f *fakeMCPReadiness) Connect(name string) (*MCPServerConn, error) {
	if f.connectCalls == nil {
		f.connectCalls = map[string]int{}
	}
	f.connectCalls[name]++
	if cnt, ok := f.failUntil[name]; ok && f.connectCalls[name] <= cnt {
		return nil, errors.New("not ready yet")
	}
	if err, ok := f.connectErrs[name]; ok {
		return nil, err
	}
	return &MCPServerConn{}, nil
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
	if !report.IsReady() {
		t.Fatalf("report should be ready: %+v", report)
	}
}

func TestConformance_MCPReadiness_MissingConfigErrors(t *testing.T) {
	probe := &fakeMCPReadiness{servers: []string{"github"}}
	_, err := WaitForMCPReadiness(context.Background(), probe, []string{"missing"}, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
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
	if !report.IsReady() {
		t.Fatalf("report should be ready: %+v", report)
	}
	if probe.connectCalls["slow"] < 2 {
		t.Fatalf("expected at least one retry, got %d calls", probe.connectCalls["slow"])
	}
}

func TestConformance_MCPReadiness_EmptyRequiredIsTrivial(t *testing.T) {
	report, err := WaitForMCPReadiness(context.Background(), nil, nil, 0)
	if err != nil || !report.IsReady() {
		t.Fatalf("empty required should succeed; got err=%v report=%+v", err, report)
	}
}

func TestConformance_MCPReadiness_NilProbeWithRequirementsErrors(t *testing.T) {
	_, err := WaitForMCPReadiness(context.Background(), nil, []string{"a"}, 0)
	if err == nil || !strings.Contains(err.Error(), "no MCP manager") {
		t.Fatalf("expected manager error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// agent-08: run_in_background
// ---------------------------------------------------------------------------

func TestConformance_Background_DisabledFlagShortCircuits(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "1")
	if !agentBackgroundTasksDisabled() {
		t.Fatal("expected background tasks disabled")
	}
}

// ---------------------------------------------------------------------------
// agent-09: transcript persistence (path round-trip)
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

// ---------------------------------------------------------------------------
// agent-10: AutoClassifierInput / UserFacingName
// ---------------------------------------------------------------------------

func TestConformance_AutoClassifier_TaglineFormat(t *testing.T) {
	tool := &AgentTool{}
	got := tool.AutoClassifierInput(AgentInput{
		Description:  "x",
		Prompt:       "do something",
		SubagentType: "general-purpose",
	})
	want := "(general-purpose): do something"
	if got != want {
		t.Fatalf("auto classifier mismatch: got %q want %q", got, want)
	}
}

func TestConformance_AutoClassifier_BareTaglineWhenNoTags(t *testing.T) {
	tool := &AgentTool{}
	got := tool.AutoClassifierInput(AgentInput{Prompt: "hello"})
	if got != ": hello" {
		t.Fatalf("expected bare colon prefix, got %q", got)
	}
}

func TestConformance_UserFacingName_MapsToAgent(t *testing.T) {
	tool := &AgentTool{}
	cases := map[string]string{
		"":                "Agent",
		"general-purpose": "Agent",
		"worker":          "Agent",
		"explorer":        "explorer",
		"plan":            "plan",
	}
	for in, want := range cases {
		got := tool.UserFacingName(&AgentInput{SubagentType: in})
		if got != want {
			t.Fatalf("UserFacingName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConformance_UserFacingName_NilInputDefault(t *testing.T) {
	tool := &AgentTool{}
	if tool.UserFacingName(nil) != "Agent" {
		t.Fatal("nil input must yield Agent")
	}
}

// ---------------------------------------------------------------------------
// Cross-cutting: completed + remote launch payloads share JSON shape contract
// ---------------------------------------------------------------------------

func TestConformance_RemoteLaunch_PayloadSchema(t *testing.T) {
	out := formatRemoteLaunchResult(RemoteAgentLaunch{TaskID: "t", SessionURL: "u", OutputFile: "/tmp/x"}, "desc", "prompt")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("payload not valid JSON: %v\n%s", err, out)
	}
	for _, k := range []string{"status", "kind", "taskId", "sessionUrl", "description", "prompt", "outputFile"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("expected key %q in payload, got %s", k, out)
		}
	}
	if got["status"] != "remote_launched" || got["kind"] != "partial" {
		t.Fatalf("expected status=remote_launched kind=partial, got %s", out)
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
	err := tool.validateAgentInvocation(AgentInput{
		Description: "x", Prompt: "y", CWD: "/tmp", Isolation: "worktree",
	})
	if err == nil || !strings.Contains(err.Error(), "cwd cannot be combined") {
		t.Fatalf("expected cwd+worktree rejection, got %v", err)
	}
}

func TestConformance_Validation_RejectsUnknownIsolation(t *testing.T) {
	tool := &AgentTool{}
	err := tool.validateAgentInvocation(AgentInput{
		Description: "x", Prompt: "y", Isolation: "frobnicate",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported isolation") {
		t.Fatalf("expected unsupported isolation error, got %v", err)
	}
}
