package tools

// Alignment tests for the AgentTool. The Go/Codex contract intentionally
// differs from the Claude TS surface for model/cwd/remote: subagents inherit
// the parent model, do not expose cwd through the model-facing schema, and only
// advertise worktree isolation in the external schema.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// agentAlignmentInput builds a minimal AgentTool input map.
func agentAlignmentInput(prompt string, fields map[string]any) map[string]any {
	in := map[string]any{
		"description": prompt,
		"prompt":      prompt,
	}
	for k, v := range fields {
		in[k] = v
	}
	return in
}

// schemaPropertyMap unwraps a schema property as a generic map.
func schemaPropertyMap(t *testing.T, schema types.JSONSchema, key string) map[string]any {
	t.Helper()
	prop, ok := schema.Properties[key]
	if !ok {
		t.Fatalf("schema is missing property %q; got keys %v", key, schemaKeys(schema))
	}
	m, ok := prop.(map[string]any)
	if !ok {
		t.Fatalf("schema property %q is not a map: %#v", key, prop)
	}
	return m
}

func schemaKeys(schema types.JSONSchema) []string {
	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	return keys
}

func TestAgentAlignment_Schema_OmitsClaudeModelOverride(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("FORK_SUBAGENT", "")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{}
	schema := tool.Schema()

	if _, ok := schema.Properties["model"]; ok {
		t.Fatalf("Agent schema must omit Claude model override; got keys %v", schemaKeys(schema))
	}
}

func TestAgentAlignment_Schema_OmitsCWD(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("FORK_SUBAGENT", "")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{}
	schema := tool.Schema()
	if _, ok := schema.Properties["cwd"]; ok {
		t.Fatalf("Agent schema must omit cwd from the external surface; got keys %v", schemaKeys(schema))
	}
}

func TestAgentAlignment_Schema_IsolationEnumOnlyWorktree(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.Schema()
	iso := schemaPropertyMap(t, schema, "isolation")
	enum, ok := iso["enum"].([]string)
	if !ok {
		t.Fatalf("isolation enum must be []string, got %#v", iso["enum"])
	}
	hasWorktree := false
	for _, v := range enum {
		if v == "remote" {
			t.Fatalf("external isolation enum must not expose remote; got %v", enum)
		}
		if v == "worktree" {
			hasWorktree = true
		}
	}
	if !hasWorktree {
		t.Errorf("isolation enum must expose worktree; got %v", enum)
	}
}

func TestAgentAlignment_DescriptionAdvertisesModelInheritance(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("FORK_SUBAGENT", "")
	tool := &AgentTool{}
	desc := tool.Description()
	if strings.Contains(desc, "Model override is not available") {
		t.Errorf("Agent description must NOT advertise model override as unavailable; got:\n%s", desc)
	}
	if strings.Contains(desc, "model=sonnet") || strings.Contains(desc, "model=opus") || strings.Contains(desc, "model=haiku") {
		t.Errorf("Agent description must not advertise Claude model overrides; got:\n%s", desc)
	}
	if !strings.Contains(desc, "Subagents always inherit the current session model") {
		t.Errorf("Agent description should advertise model inheritance; got:\n%s", desc)
	}
}

// -------- agent-04: AgentProgressEmitter must be wired into Execute --------
//
// agent_progress.go fully implements the emitter, but `grep` shows it is only
// referenced from tests + agent_remote.go — the synchronous Execute path
// never emits progress. We surface this by checking that the emitter
// constructor is observed at least once during a sync Execute.
//
// We use a thread-safe counter wrapping the package-level constructor;
// constructor is `NewAgentProgressEmitter`. Without modifying production code
// we cannot patch it directly, so this test instead asserts that the public
// `AgentProgressEmitter` type appears in the agent loop's metadata path —
// the Execute method should expose a `Progress()` accessor matching the
// TS contract. The `Progress` symbol does not exist on AgentTool in Go.
func TestAgentAlignment_Execute_HasProgressEmitterAccessor(t *testing.T) {
	tool := &AgentTool{Registry: registry.New()}

	// reflect over the AgentTool instance: TS exposes a Progress channel
	// the loop can subscribe to before Execute runs. The Go side has the
	// emitter type but no accessor on AgentTool itself.
	type progressAccessor interface {
		Progress() *AgentProgressEmitter
	}
	if _, ok := any(tool).(progressAccessor); !ok {
		t.Errorf("AgentTool must expose Progress() accessor wiring agent_progress.go into Execute (agent-04); got no such method")
	}
}

// -------- agent-05: PrepareIsolation must be invoked from Execute --------
//
// PrepareIsolation lives in agent_isolation.go and is referenced only from
// agent_conformance_test.go. Execute should consult it whenever the caller
// passes isolation="worktree" or "remote". We assert that Execute fails
// with an isolation-mode error when the registry has no worktree manager AND
// the caller asks for isolation — proving Execute actually went through the
// isolation gate.
func TestAgentAlignment_Execute_RejectsUnsupportedIsolationViaPrepareIsolation(t *testing.T) {
	tool := &AgentTool{
		Registry: registry.New(),
		// no Provider, no WorktreeManager, no RemoteRuntime — every isolation
		// mode must be rejected before any provider call is attempted.
	}
	in := agentAlignmentInput("do isolated work", map[string]any{"isolation": "worktree"})
	res, err := tool.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected infra error: %v", err)
	}
	if !res.IsError {
		t.Errorf("Execute must surface an isolation error when worktree manager is missing (agent-05); got %#v", res)
	}
	if !strings.Contains(strings.ToLower(res.Content), "isolation") &&
		!strings.Contains(strings.ToLower(res.Content), "worktree") {
		t.Errorf("isolation error should mention isolation/worktree; got %q", res.Content)
	}
}

// -------- agent-07: WaitForMCPReadiness gating --------
//
// Profile-loaded MCP servers should be probed before the subagent loop is
// started. We assert the AgentTool exposes a `MCPReadinessProbe` field /
// setter so the Execute path can wait on probes — Go currently has the
// helper but no plumbing on AgentTool.
func TestAgentAlignment_Execute_HasMCPReadinessProbeHook(t *testing.T) {
	type probeHolder interface {
		SetMCPReadinessProbe(MCPReadinessProbe)
	}
	tool := &AgentTool{}
	if _, ok := any(tool).(probeHolder); !ok {
		t.Errorf("AgentTool must accept an MCPReadinessProbe (agent-07) so Execute can call WaitForMCPReadiness; got no setter")
	}
}

// -------- agent-09: transcript must be JSONL, not free-text --------
//
// agent.go:1001-1005 writes free-form Text to the transcript file via
// io.WriteString. TS writes one JSON object per line. We exercise a minimal
// completed run and parse the transcript: every non-empty line must be valid
// JSON.
func TestAgentAlignment_Execute_TranscriptIsJSONLines(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "agent.transcript")

	// Produce a synthetic transcript through the helper used by Execute.
	// agent.go writes free-form text; we cannot easily call into the
	// production path without a provider, so we drive transcript writing
	// directly via the public agent transcript writer if exposed.
	// Fallback: assert that the helper for opening transcripts exists and
	// produces a JSONL writer by name. We rely on the symbol
	// `openAgentTranscriptWriter` returning a writer whose first emitted
	// line for an event is JSON.
	prov := &captureAgentProvider{responses: []string{"hello from subagent"}}
	tool := &AgentTool{
		Registry: registry.New(),
		Provider: prov,
	}
	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", transcriptPath)
	res, err := tool.Execute(context.Background(), agentAlignmentInput("write transcript", nil))
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	if res.IsError {
		t.Logf("Execute reported business error (still proceeding to inspect transcript): %s", res.Content)
	}
	if completed, ok := res.Data.(AgentCompleted); ok && strings.TrimSpace(completed.TranscriptPath) != "" {
		transcriptPath = completed.TranscriptPath
	}
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		// If no transcript was produced via env, this still fails the
		// JSONL contract: agent-09 requires the loop to write structured
		// events somewhere observable.
		t.Errorf("expected transcript file at %s after Execute (agent-09): %v", transcriptPath, err)
		return
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var probe map[string]any
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Errorf("transcript line %d is not JSON (agent-09): %q (%v)", i, line, err)
		}
	}
}

// -------- isForkSubagentEnabled & schema variant --------
//
// agent.go:1208 derives fork-subagent gating from env. With FORK_SUBAGENT=1
// the schema should drop run_in_background AND advertise a `fork`-shaped
// description. We assert both the schema mutation AND the description
// mention forking — the latter is currently missing.
func TestAgentAlignment_ForkMode_DescriptionMentionsFork(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "1")
	tool := &AgentTool{}
	desc := tool.Description()
	if !strings.Contains(strings.ToLower(desc), "fork") {
		t.Errorf("with FORK_SUBAGENT=1 description must mention fork semantics; got:\n%s", desc)
	}
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Errorf("with FORK_SUBAGENT=1 schema must omit run_in_background; got it present")
	}
}

// -------- agentBackgroundTasksDisabled effect --------
//
// audit P1-3 / row 5: when CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1 the
// run_in_background field must vanish AND the description must omit the
// "Background agents:" guidance block.
func TestAgentAlignment_BackgroundDisabled_DescriptionAndSchemaPruned(t *testing.T) {
	t.Setenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS", "1")
	tool := &AgentTool{}
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Errorf("schema must hide run_in_background when CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1")
	}
	desc := tool.Description()
	if strings.Contains(desc, "Background agents:") {
		t.Errorf("description must omit Background agents guidance when disabled; got:\n%s", desc)
	}
	if !strings.Contains(desc, "send a single assistant message with multiple Agent tool calls") {
		t.Errorf("description must still teach parallel-Agent invocation pattern (audit row 5)")
	}
}

// -------- agent-08: Profile loading must be exposed --------
//
// audit row 5 + tasks/agent.json agent-08: Profile-driven subagents need an
// accessor for the loaded profiles map (so the loop can echo allowed tools).
// AgentTool only has private `InlineProfiles` and no method to enumerate the
// final resolved set.
func TestAgentAlignment_LoadedProfiles_AreExposed(t *testing.T) {
	type profilesAccessor interface {
		LoadedProfiles() []string
	}
	tool := &AgentTool{}
	if _, ok := any(tool).(profilesAccessor); !ok {
		t.Errorf("AgentTool must expose LoadedProfiles() to surface resolved profile names (agent-08)")
	}
}

// -------- agent-10: Hooks must be invoked from Execute --------
//
// AgentTool.HookRunner exists but there is no observable signal that
// Execute actually fires before/after lifecycle hooks. We assert that the
// AgentTool provides an `HookEvents()` accessor returning the recorded
// hook invocations from the most recent Execute. This is the contract the
// audit report calls out and is not yet implemented.
func TestAgentAlignment_Execute_RecordsHookEvents(t *testing.T) {
	type hookObserver interface {
		HookEvents() []string
	}
	tool := &AgentTool{Registry: registry.New()}
	if _, ok := any(tool).(hookObserver); !ok {
		t.Errorf("AgentTool must expose HookEvents() so callers can verify hook firing (agent-10)")
	}
}

// -------- team_name guidance must reflect TS rules --------
//
// AgentTool.Description on master TS clarifies that team_name is only valid
// AFTER TeamCreate. The Go description does include the rule — make sure
// the schema field description reproduces it. We currently put a different
// short blurb into the schema property description.
func TestAgentAlignment_TeamName_SchemaDescriptionMentionsTeamCreate(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.Schema()
	prop, ok := schema.Properties["team_name"].(map[string]any)
	if !ok {
		t.Fatalf("schema must contain team_name property")
	}
	desc, _ := prop["description"].(string)
	if !strings.Contains(desc, "TeamCreate") {
		t.Errorf("team_name schema description must reference TeamCreate; got %q", desc)
	}
	if !strings.Contains(strings.ToLower(desc), "do not set") {
		t.Errorf("team_name description should warn against setting it for ordinary subagents; got %q", desc)
	}
}

// -------- agent-11: Subagent type listing must include builtin Explore/Plan --------
func TestAgentAlignment_Description_ListsBuiltinAgents(t *testing.T) {
	tool := &AgentTool{}
	desc := tool.Description()
	for _, want := range []string{"Explore", "Plan"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Agent description must list builtin %s subagent type (agent-11); got:\n%s", want, desc)
		}
	}
}

// -------- agent-04 alt: AgentProgressEmitter must be reachable from registry --------
//
// Even if Progress() is not on AgentTool, the audit asks that the loop be
// able to discover the active emitter from the registry-stored AgentTool.
// We surface the symptom: the registry-stored Agent tool is bare types.Tool
// and offers no path to the emitter; this test asserts the cast.
func TestAgentAlignment_RegistryStoredAgent_ExposesProgressChannel(t *testing.T) {
	reg := registry.New()
	reg.Register(&AgentTool{Registry: reg})
	got := reg.Get("Agent")
	if got == nil {
		t.Fatalf("Agent must be registered")
	}
	type channelExposer interface {
		ProgressChannel() <-chan AgentProgressEvent
	}
	if _, ok := got.(channelExposer); !ok {
		t.Errorf("Agent in registry must expose ProgressChannel() so the loop can observe phase events (agent-04)")
	}
}
