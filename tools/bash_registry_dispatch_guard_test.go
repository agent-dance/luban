package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestRegisteredBashGetCannotBypassRegistryDispatch(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(staticToolRuntimeProvider{runtime: types.ToolRuntimeContext{
		DeniedTools: map[string]bool{"Bash": true},
	}})
	reg.Register(tool)

	got := reg.Get("Bash")
	if got != tool {
		t.Fatalf("Get(Bash) = %T %p, want original concrete tool %p", got, got, tool)
	}

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "missing receipt", ctx: context.Background()},
		{
			name: "public pending value is not a registry commit",
			ctx: approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
				Token: "forged", Binding: types.ToolPermissionBinding{
					SessionID: "session", TurnID: "turn", ToolUseID: "tool-use", ApprovalEpoch: "epoch",
				},
			}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(root, tc.name+"-must-not-execute")
			result, err := got.Execute(tc.ctx, map[string]any{
				"command": `touch "` + marker + `"`,
			})
			if err != nil {
				t.Fatalf("direct registered Execute returned infrastructure error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("direct registered Execute bypassed Registry authorization: %#v", result)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("direct registered Execute created marker: %v", statErr)
			}
		})
	}

	if !reg.Unregister("Bash") {
		t.Fatal("Unregister(Bash) = false, want true")
	}
	marker := filepath.Join(root, "retained-pointer-must-not-execute")
	result, err := got.Execute(context.Background(), map[string]any{
		"command": `touch "` + marker + `"`,
	})
	if err != nil || !result.IsError {
		t.Fatalf("retained pointer after Unregister result=%#v err=%v", result, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("retained pointer after Unregister created marker: %v", statErr)
	}
}

func TestUnregisteredBashDirectExecutionRemainsCompatible(t *testing.T) {
	tool := &BashTool{CWD: t.TempDir()}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": `printf standalone-direct-ok`,
	})
	if err != nil || result.IsError || result.Content != "standalone-direct-ok" {
		t.Fatalf("standalone direct Execute result=%#v err=%v", result, err)
	}
}

func TestRegisteredBashStandardRegistryDispatchStillExecutesAllowedCommand(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.Register(tool)

	result, err := reg.ExecuteToolWithError(context.Background(), "Bash", map[string]any{
		"command": `printf registry-dispatch-ok`,
	})
	if err != nil || result.IsError || result.Content != "registry-dispatch-ok" {
		t.Fatalf("standard Registry dispatch result=%#v err=%v", result, err)
	}
}

func TestRegisteredBashStandardLoopDispatchStillExecutesApprovedCommand(t *testing.T) {
	root := t.TempDir()
	reg := registry.New()
	reg.Register(&BashTool{CWD: root, AllowedDirs: []string{root}})

	input := map[string]any{"command": `printf loop-dispatch-ok`}
	result, err := executeApprovedRegistryToolForTest(t, reg, "Bash", input)
	if err != nil || result.IsError || result.Content != "loop-dispatch-ok" {
		t.Fatalf("loop-style Registry dispatch result=%#v err=%v", result, err)
	}
}

func TestRegisteredBashDispatchBindingIsConcurrentAndMonotonic(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "concurrent-direct-execute-must-not-run")
	tool := &BashTool{CWD: root, AllowedDirs: []string{root}}
	reg := registry.New()
	reg.Register(tool)

	const (
		registerWorkers = 8
		directWorkers   = 16
		iterations      = 32
	)
	start := make(chan struct{})
	failures := make(chan string, directWorkers*iterations)
	var wg sync.WaitGroup
	for worker := 0; worker < registerWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				reg.Register(tool)
			}
		}()
	}
	for worker := 0; worker < directWorkers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				got := reg.Get("Bash")
				if got == nil {
					failures <- fmt.Sprintf("worker %d iteration %d: Get(Bash) returned nil", worker, i)
					continue
				}
				result, err := got.Execute(context.Background(), map[string]any{
					"command": `touch "` + marker + `"`,
				})
				if err != nil || !result.IsError {
					failures <- fmt.Sprintf("worker %d iteration %d: result=%#v err=%v", worker, i, result, err)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("concurrent direct Execute created marker: %v", statErr)
	}

	// Re-registering is allowed for registry generation management, but it must
	// never undo the Bash binding or break the ordinary approved dispatch path.
	result, err := executeApprovedRegistryToolForTest(t, reg, "Bash", map[string]any{
		"command": `printf post-race-loop-ok`,
	})
	if err != nil || result.IsError || result.Content != "post-race-loop-ok" {
		t.Fatalf("post-race loop-style dispatch result=%#v err=%v", result, err)
	}
}

type staticToolRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p staticToolRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.runtime
}

func executeApprovedRegistryToolForTest(t *testing.T, reg *registry.Registry, name string, input map[string]any) (types.ToolResultBlock, error) {
	t.Helper()
	preflight, err := reg.CheckToolPermissions(context.Background(), name, input, types.ToolPermissionRequest{
		SessionID: "test-session", TurnID: "test-turn", ToolUseID: "test-tool-use", ApprovalEpoch: "test-epoch",
	})
	if err != nil {
		t.Fatalf("%s permission preflight: %v", name, err)
	}
	if preflight.PermissionGrant == "" {
		t.Fatalf("%s permission preflight returned no grant (behavior=%s)", name, preflight.Behavior)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, name, input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatalf("%s permission authorization returned no execution grant", name)
	}
	ctx := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	return reg.ExecuteToolWithError(ctx, name, input)
}
