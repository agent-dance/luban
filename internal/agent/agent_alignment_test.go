package agent

// Alignment tests for the AgentTool. The Go/Codex contract intentionally
// differs from the Claude TS surface for model/cwd/remote: subagents inherit
// the parent model, do not expose cwd through the model-facing schema, and only
// advertise worktree isolation in the external schema.

import (
	"context"
	"encoding/json"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
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
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
	tool := &AgentTool{}
	schema := tool.Schema()

	if _, ok := schema.Properties["model"]; ok {
		t.Fatalf("Agent schema must omit Claude model override; got keys %v", schemaKeys(schema))
	}
}

func TestAgentAlignment_Schema_OmitsCWD(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "")
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
	if len(enum) != 1 || enum[0] != "worktree" {
		t.Errorf("isolation enum must contain only worktree; got %v", enum)
	}
}

func TestAgentAlignment_DescriptionAdvertisesModelInheritance(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "")
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

// -------- Agent progress observer contract --------
func TestAgentAlignment_Execute_HasProgressSubscription(t *testing.T) {
	tool := &AgentTool{Registry: registry.New()}

	type progressSubscriber interface {
		SubscribeProgress(func(agentcontract.ProgressEvent)) func()
	}
	if _, ok := any(tool).(progressSubscriber); !ok {
		t.Errorf("AgentTool must expose the progress observer bridge; got no such method")
	}
}

func TestAgentAlignment_ExecuteRejectsUnavailableWorktreeIsolation(t *testing.T) {
	tool := &AgentTool{
		Registry: registry.New(),
		// No provider is configured, so worktree isolation must be rejected
		// before any provider call is attempted.
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

// -------- WaitForMCPReadiness gating --------
//
// Profile-loaded MCP servers should be probed before the subagent loop is
// started. We assert the AgentTool exposes a `MCPReadinessProbe` field /
// setter so the Execute path can wait on probes before starting the subagent.
func TestAgentAlignment_Execute_HasMCPReadinessProbeHook(t *testing.T) {
	type probeHolder interface {
		SetMCPReadinessProbe(MCPReadinessProbe)
	}
	tool := &AgentTool{}
	if _, ok := any(tool).(probeHolder); !ok {
		t.Errorf("AgentTool must accept an MCPReadinessProbe (agent-07) so Execute can call WaitForMCPReadiness; got no setter")
	}
}

// -------- Transcript JSONL contract --------
//
// A completed run records one JSON object per transcript line; every
// non-empty line must therefore parse as valid JSON.
func TestAgentAlignment_Execute_TranscriptIsJSONLines(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "agent.transcript")

	// Drive the production transcript path with a minimal provider response.
	prov := &captureAgentProvider{responses: []string{"hello from subagent"}}
	tool := &AgentTool{
		Registry: registry.New(),
		Provider: prov,
	}
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", transcriptPath)
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
		// The JSONL contract requires the loop to write structured events at
		// the configured observable path.
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
// agent.go:1208 derives fork-subagent gating from env. With LUBAN_CODE_FORK_SUBAGENT=1
// the schema should drop run_in_background AND advertise a `fork`-shaped
// description. We assert both the schema mutation AND the description
// mention forking so the schema and description change together.
func TestAgentAlignment_ForkMode_DescriptionMentionsFork(t *testing.T) {
	t.Setenv("LUBAN_CODE_FORK_SUBAGENT", "1")
	tool := &AgentTool{}
	desc := tool.Description()
	if !strings.Contains(strings.ToLower(desc), "fork") {
		t.Errorf("with LUBAN_CODE_FORK_SUBAGENT=1 description must mention fork semantics; got:\n%s", desc)
	}
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Errorf("with LUBAN_CODE_FORK_SUBAGENT=1 schema must omit run_in_background; got it present")
	}
}

// -------- agentBackgroundTasksDisabled effect --------
//
// When LUBAN_CODE_DISABLE_BACKGROUND_TASKS=1, the
// run_in_background field must vanish AND the description must omit the
// "Background agents:" guidance block.
func TestAgentAlignment_BackgroundDisabled_DescriptionAndSchemaPruned(t *testing.T) {
	t.Setenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS", "1")
	tool := &AgentTool{}
	if _, ok := tool.Schema().Properties["run_in_background"]; ok {
		t.Errorf("schema must hide run_in_background when LUBAN_CODE_DISABLE_BACKGROUND_TASKS=1")
	}
	desc := tool.Description()
	if strings.Contains(desc, "Background agents:") {
		t.Errorf("description must omit Background agents guidance when disabled; got:\n%s", desc)
	}
	if !strings.Contains(desc, "send a single assistant message with multiple Agent tool calls") {
		t.Errorf("description must still teach parallel-Agent invocation pattern (audit row 5)")
	}
}

// -------- team_name guidance must reflect TS rules --------
//
// AgentTool.Description on master TS clarifies that team_name is only valid
// AFTER TeamCreate. The Go description does include the rule — make sure
// the schema field description reproduces it.
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

// -------- Built-in Explore/Plan subagent types --------
func TestAgentAlignment_Description_ListsBuiltinAgents(t *testing.T) {
	tool := &AgentTool{}
	desc := tool.Description()
	for _, want := range []string{"Explore", "Plan"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Agent description must list builtin %s subagent type (agent-11); got:\n%s", want, desc)
		}
	}
}

// -------- Agent progress registry reachability --------
func TestAgentAlignment_RegistryStoredAgent_ExposesProgressSubscription(t *testing.T) {
	reg := registry.New()
	reg.Register(&AgentTool{Registry: reg})
	got := reg.Get("Agent")
	if got == nil {
		t.Fatalf("Agent must be registered")
	}
	type progressSubscriber interface {
		SubscribeProgress(func(agentcontract.ProgressEvent)) func()
	}
	if _, ok := got.(progressSubscriber); !ok {
		t.Errorf("Agent in registry must expose the progress observer bridge")
	}
}
