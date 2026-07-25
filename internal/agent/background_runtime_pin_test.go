package agent

import (
	"context"
	"sync"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/permission"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestBackgroundAgentPinsExplicitDefaultPermissionMode(t *testing.T) {
	parent := &captureToolPermissionHandler{}
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{PermissionMode: "default"}, parent, agentcontract.ApprovalFailClosed, agentProfile{}, "")
	if handler == nil {
		t.Fatal("expected permission handler")
	}
	if _, err := handler.Check(context.Background(), permission.PermissionRequest{ToolName: "Read"}); err != nil {
		t.Fatal(err)
	}
	if len(parent.requests) != 1 || parent.requests[0].Mode != "default" {
		t.Fatalf("background permission request did not pin default mode: %#v", parent.requests)
	}
}

func TestAgentLaunchRuntimeSnapshotKeepsWorkspaceFieldsTogether(t *testing.T) {
	origin := t.TempDir()
	next := t.TempDir()
	barrier := &sync.RWMutex{}
	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: origin, AllowedDirs: []string{origin}})
	tool := &AgentTool{Registry: reg}
	tool.SetSessionBarrier(barrier)
	tool.SetSessionRuntime(AgentSessionRuntime{
		System: "origin-system",
		ToolRuntime: types.ToolRuntimeContext{
			SessionID: "origin-session", ProjectRoot: origin, AllowedDirs: []string{origin}, PermissionMode: "default",
		},
	})

	snapshot := tool.captureLaunchRuntime()
	barrier.Lock()
	tool.SetSessionRuntime(AgentSessionRuntime{
		System: "next-system",
		ToolRuntime: types.ToolRuntimeContext{
			SessionID: "next-session", ProjectRoot: next, AllowedDirs: []string{next}, PermissionMode: "bypassPermissions",
		},
	})
	reg.Register(&shell.BashTool{CWD: next, AllowedDirs: []string{next}})
	barrier.Unlock()

	if snapshot.session.System != "origin-system" || snapshot.session.ToolRuntime.SessionID != "origin-session" || snapshot.session.ToolRuntime.ProjectRoot != origin {
		t.Fatalf("launch snapshot mixed workspace fields: %+v", snapshot.session)
	}
	childBash, ok := snapshot.registry.Get("Bash").(*shell.BashTool)
	if !ok || childBash.CWD != origin || len(childBash.AllowedDirs) != 1 || childBash.AllowedDirs[0] != origin {
		t.Fatalf("launch registry followed later workspace: %+v", childBash)
	}
}

func TestAgentLaunchRuntimeSnapshotIsConsistentDuringSessionPublication(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	barrier := &sync.RWMutex{}
	reg := registry.New()
	reg.Register(&shell.BashTool{CWD: rootA, AllowedDirs: []string{rootA}})
	tool := &AgentTool{Registry: reg}
	tool.SetSessionBarrier(barrier)
	tool.SetSessionRuntime(AgentSessionRuntime{System: "A", ToolRuntime: types.ToolRuntimeContext{ProjectRoot: rootA, AllowedDirs: []string{rootA}}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			marker, root := "A", rootA
			if i%2 == 1 {
				marker, root = "B", rootB
			}
			barrier.Lock()
			tool.SetSessionRuntime(AgentSessionRuntime{System: marker, ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}}})
			reg.Register(&shell.BashTool{CWD: root, AllowedDirs: []string{root}})
			barrier.Unlock()
		}
	}()
	for i := 0; i < 100; i++ {
		snapshot := tool.captureLaunchRuntime()
		bash, ok := snapshot.registry.Get("Bash").(*shell.BashTool)
		if !ok {
			t.Fatalf("snapshot Bash = %T", snapshot.registry.Get("Bash"))
		}
		wantRoot := rootA
		if snapshot.session.System == "B" {
			wantRoot = rootB
		}
		if snapshot.session.ToolRuntime.ProjectRoot != wantRoot || bash.CWD != wantRoot || len(bash.AllowedDirs) != 1 || bash.AllowedDirs[0] != wantRoot {
			t.Fatalf("mixed launch snapshot: session=%+v bash=%+v", snapshot.session, bash)
		}
	}
	<-done
}
