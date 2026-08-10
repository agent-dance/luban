package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	toolinspect "github.com/agent-dance/luban/internal/agentic/inspect"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/runtime/engine"
	ui "github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

type printModeInspectProbeEngine struct {
	engine.Engine
	inspect        *toolinspect.Tool
	runtime        types.ToolRuntimeContext
	root           string
	request        engine.QueryRequest
	resumed        string
	querySawResume bool
	decision       types.ToolPermissionResult
	checkErr       error
}

func (e *printModeInspectProbeEngine) Resume(_ context.Context, sessionID string) (int, error) {
	e.resumed = sessionID
	return 7, nil
}

func (e *printModeInspectProbeEngine) Query(ctx context.Context, request engine.QueryRequest) (<-chan engine.Event, error) {
	e.querySawResume = e.resumed == request.SessionID
	e.request = request
	executionCtx := executioncontract.WithToolExecutionContext(ctx, executioncontract.ToolExecutionContext{
		SessionID:         request.SessionID,
		SessionProjectDir: request.SessionProjectDir,
		ProjectRoot:       request.ProjectRoot,
		CWD:               request.CWD,
	})
	e.decision, e.checkErr = e.inspect.CheckPermissions(executionCtx, map[string]any{
		"requests": []any{map[string]any{
			"id": "probe", "kind": toolinspect.KindRead, "path": "fixture.txt",
		}},
	}, types.ToolPermissionRequest{SessionID: request.SessionID, Runtime: e.runtime})

	events := make(chan engine.Event, 1)
	events <- engine.Event{SessionID: request.SessionID, Final: true}
	close(events)
	return events, nil
}

func TestRunPrintModePreservesResolvedSessionForInspectRuntimeOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil, false)
	t.Cleanup(func() { stopScheduleForTest(t, deps) })

	const sessionID = "print-session"
	const sessionProjectDir = "print-session-project"
	deps.BindSessionIdentity(sessionID)
	probe := &printModeInspectProbeEngine{
		inspect: deps.InspectTool,
		runtime: deps.RuntimeScope.ToolRuntimeContext(),
		root:    root,
	}
	if exitCode := RunPrintMode(probe, ui.NewQuietRenderer(io.Discard), PrintModeConfig{
		SessionID:         sessionID,
		SessionProjectDir: sessionProjectDir,
		ProjectRoot:       root,
		CWD:               root,
		Query:             "inspect fixture",
		Resume:            true,
	}); exitCode != 0 {
		t.Fatalf("RunPrintMode exit code = %d", exitCode)
	}
	if probe.request.SessionID != sessionID {
		t.Fatalf("query session ID = %q, want %q", probe.request.SessionID, sessionID)
	}
	if probe.resumed != sessionID || !probe.querySawResume {
		t.Fatalf("print mode queried before resume: resumed=%q request=%+v", probe.resumed, probe.request)
	}
	if probe.request.SessionProjectDir != sessionProjectDir || probe.request.ProjectRoot != root || probe.request.CWD != root {
		t.Fatalf("query workspace identity = %+v", probe.request)
	}
	if probe.checkErr != nil {
		t.Fatalf("Inspect permission check failed: %v", probe.checkErr)
	}
	if probe.decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("Inspect permission behavior = %q, message = %q", probe.decision.Behavior, probe.decision.Message)
	}
}
