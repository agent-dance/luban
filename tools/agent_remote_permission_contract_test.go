package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type permissionSnapshotRemoteRuntime struct {
	enforcesSnapshot bool
	enforcesProfile  bool
	spawnCalls       int
	spawnRequest     RemoteAgentSpawnRequest
}

func (r *permissionSnapshotRemoteRuntime) EnforcesPermissionSnapshot() bool {
	return r.enforcesSnapshot
}

func (*permissionSnapshotRemoteRuntime) EnforcesFailClosedPrompts() bool { return true }

func (r *permissionSnapshotRemoteRuntime) EnforcesProfileRestrictions() bool {
	return r.enforcesProfile
}

func (r *permissionSnapshotRemoteRuntime) Spawn(_ context.Context, req RemoteAgentSpawnRequest) (RemoteAgentLaunch, error) {
	r.spawnCalls++
	r.spawnRequest = req
	return RemoteAgentLaunch{TaskID: "remote-task"}, nil
}

func (*permissionSnapshotRemoteRuntime) Poll(context.Context, string) (RemoteAgentStatus, error) {
	return RemoteAgentStatus{}, nil
}

func (*permissionSnapshotRemoteRuntime) Cleanup(string) error { return nil }

type legacyRemoteRuntime struct {
	spawnCalls int
}

func (r *legacyRemoteRuntime) Spawn(context.Context, RemoteAgentSpawnRequest) (RemoteAgentLaunch, error) {
	r.spawnCalls++
	return RemoteAgentLaunch{TaskID: "legacy-task"}, nil
}

func (*legacyRemoteRuntime) Poll(context.Context, string) (RemoteAgentStatus, error) {
	return RemoteAgentStatus{}, nil
}

func (*legacyRemoteRuntime) Cleanup(string) error { return nil }

func TestPrepareIsolationPassesParentPermissionSnapshotToRemoteRuntime(t *testing.T) {
	root := t.TempDir()
	snapshot := types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    root,
		PermissionMode: "bypassPermissions",
		AllowedDirs:    []string{root},
		AllowedTools:   map[string]bool{"Read": true, "Bash": true},
		DeniedTools:    map[string]bool{"Write": true},
	}
	runtime := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true}

	result, err := PrepareIsolation(
		context.Background(),
		AgentIsolationRemote,
		"agent-1",
		root,
		snapshot,
		runtime,
	)
	if err != nil {
		t.Fatalf("PrepareIsolation(remote) error: %v", err)
	}
	defer result.Cleanup()

	if runtime.spawnCalls != 1 {
		t.Fatalf("Spawn calls = %d, want 1", runtime.spawnCalls)
	}
	expected := cloneToolRuntimeContext(snapshot)
	expected.AgentID = "agent-1"
	expected.Interactive = false
	if !reflect.DeepEqual(runtime.spawnRequest.PermissionSnapshot, expected) {
		t.Fatalf("remote permission snapshot = %#v, want %#v", runtime.spawnRequest.PermissionSnapshot, expected)
	}
	runtime.spawnRequest.PermissionSnapshot.AllowedTools["Write"] = true
	if snapshot.AllowedTools["Write"] {
		t.Fatal("remote provider mutated the caller-owned permission snapshot")
	}
	if result.Metadata.Mode != "" {
		t.Fatalf("isolation metadata must not carry a permission mode side channel, got %q", result.Metadata.Mode)
	}
}

func TestPrepareIsolationRejectsRemoteRuntimeWithoutPermissionSnapshotEnforcement(t *testing.T) {
	runtime := &legacyRemoteRuntime{}
	root := t.TempDir()

	_, err := PrepareIsolation(
		context.Background(),
		AgentIsolationRemote,
		"agent-1",
		root,
		types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "default"},
		runtime,
	)
	if err == nil {
		t.Fatal("expected a fail-closed capability error")
	}
	if !strings.Contains(err.Error(), "permission snapshot") {
		t.Fatalf("error = %q, want permission snapshot capability guidance", err)
	}
	if runtime.spawnCalls != 0 {
		t.Fatalf("legacy runtime Spawn calls = %d, want 0", runtime.spawnCalls)
	}
}

func TestPrepareIsolationRejectsRemoteRuntimeThatDeclinesSnapshotEnforcement(t *testing.T) {
	runtime := &permissionSnapshotRemoteRuntime{enforcesSnapshot: false}
	root := t.TempDir()

	_, err := PrepareIsolation(
		context.Background(),
		AgentIsolationRemote,
		"agent-1",
		root,
		types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits"},
		runtime,
	)
	if err == nil {
		t.Fatal("expected a fail-closed capability error")
	}
	if runtime.spawnCalls != 0 {
		t.Fatalf("declining runtime Spawn calls = %d, want 0", runtime.spawnCalls)
	}
}
