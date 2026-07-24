package loop

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type projectIdentityProvider struct{}

func (*projectIdentityProvider) Name() string    { return "project-identity" }
func (*projectIdentityProvider) ModelID() string { return "project-identity-model" }
func (*projectIdentityProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return makeStreamChan(parityTextEvents("done")...), nil
}

type projectToolIdentityProvider struct{ calls atomic.Int32 }

func (*projectToolIdentityProvider) Name() string    { return "project-tool-identity" }
func (*projectToolIdentityProvider) ModelID() string { return "project-tool-identity-model" }
func (p *projectToolIdentityProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	if p.calls.Add(1) == 1 {
		return makeStreamChan(aggregateToolUseEvents(types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-project", Name: "ProjectIdentityProbe", Input: map[string]any{}})...), nil
	}
	return makeStreamChan(parityTextEvents("done")...), nil
}

type projectIdentityProbeTool struct{ contexts chan ToolExecutionContext }

func (*projectIdentityProbeTool) Name() string { return "ProjectIdentityProbe" }
func (*projectIdentityProbeTool) Description() string {
	return "captures immutable project execution identity"
}
func (*projectIdentityProbeTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *projectIdentityProbeTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	exec, _ := ToolExecutionContextFromContext(ctx)
	t.contexts <- exec
	return types.ToolResult{Content: "captured"}, nil
}

func TestQueryLoopStampsImmutableProjectRootOnEveryEvent(t *testing.T) {
	const projectRoot = "/workspace/project"
	cfg := Config{SessionID: "session-project", ProjectRoot: projectRoot, CWD: "/workspace/project/nested", MaxTurns: 1}
	query := New(&projectIdentityProvider{}, registry.New(), cfg)

	var events []Event
	if err := query.Run(context.Background(), "hello", func(event Event) { events = append(events, event) }); err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("query emitted no events")
	}
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(encoded, &payload); err != nil {
			t.Fatal(err)
		}
		if got := payload["project_root"]; got != projectRoot {
			t.Fatalf("event %s project_root = %#v, want %q; cwd must not substitute for project root", event.Type, got, projectRoot)
		}
	}
}

func TestToolExecutionContextKeepsProjectRootSeparateFromNestedCWD(t *testing.T) {
	const projectRoot = "/workspace/project"
	const sessionProjectDir = "/session-store/legacy-project"
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "after_message", true: "streaming"}[streaming], func(t *testing.T) {
			tool := &projectIdentityProbeTool{contexts: make(chan ToolExecutionContext, 1)}
			reg := registry.New()
			reg.Register(tool)
			query := New(&projectToolIdentityProvider{}, reg, Config{
				SessionID: "session-project-tool", CacheLineageID: "root-cache-lineage", SessionProjectDir: sessionProjectDir,
				ProjectRoot: projectRoot, CWD: "/workspace/project/nested", MaxTurns: 2,
				StreamingToolExecution: streaming,
			})
			if err := query.Run(context.Background(), "run probe", func(Event) {}); err != nil {
				t.Fatal(err)
			}
			exec := <-tool.contexts
			if exec.SessionProjectDir != sessionProjectDir || exec.ProjectRoot != projectRoot || exec.CWD != "/workspace/project/nested" {
				t.Fatalf("tool execution identity = project dir %q root %q cwd %q, want independent persisted/workspace/execution identities",
					exec.SessionProjectDir, exec.ProjectRoot, exec.CWD)
			}
			if exec.CacheLineageID != "root-cache-lineage" {
				t.Fatalf("tool execution cache lineage = %q, want root-cache-lineage", exec.CacheLineageID)
			}
		})
	}
}
