package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestRemoteAgentProfileRestrictionsPreserveResolvedAllowlistAndDenials(t *testing.T) {
	profile := agentProfile{
		AllowedToolsSpecified: true,
		AllowedTools: map[string]struct{}{
			"read": {},
		},
		AllowedToolSpecs: []string{" Read ", "Read"},
		DisallowedTools: map[string]struct{}{
			"write": {},
		},
		DisallowedToolSpecs: []string{"Write", "Bash(rm *)"},
	}

	restrictions := remoteAgentProfileRestrictionsFromProfile(profile)
	if restrictions == nil {
		t.Fatal("restrictive profile produced no remote restrictions")
	}
	if !restrictions.AllowedToolsSpecified {
		t.Fatal("remote restrictions lost the allowlist-presence bit")
	}
	if want := []string{"Read"}; !reflect.DeepEqual(restrictions.AllowedToolSpecs, want) {
		t.Fatalf("allowed tool specs = %#v, want %#v", restrictions.AllowedToolSpecs, want)
	}
	if want := []string{"Write", "Bash(rm *)"}; !reflect.DeepEqual(restrictions.DisallowedToolSpecs, want) {
		t.Fatalf("disallowed tool specs = %#v, want %#v", restrictions.DisallowedToolSpecs, want)
	}

	profile.AllowedToolSpecs[0] = "Write"
	profile.DisallowedToolSpecs[0] = "Read"
	if got := restrictions.AllowedToolSpecs[0]; got != "Read" {
		t.Fatalf("allowed restrictions alias profile storage: got %q", got)
	}
	if got := restrictions.DisallowedToolSpecs[0]; got != "Write" {
		t.Fatalf("disallowed restrictions alias profile storage: got %q", got)
	}
}

func TestRemoteAgentProfileRestrictionsKeepExplicitEmptyAllowlist(t *testing.T) {
	restrictions := remoteAgentProfileRestrictionsFromProfile(agentProfile{AllowedToolsSpecified: true})
	if restrictions == nil {
		t.Fatal("explicit empty allowlist must be serialized because it denies every tool")
	}
	if !restrictions.AllowedToolsSpecified {
		t.Fatal("explicit empty allowlist became unrestricted")
	}

	req := RemoteAgentSpawnRequest{ProfileRestrictions: restrictions}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal remote request: %v", err)
	}
	if !strings.Contains(string(raw), `"allowedToolsSpecified":true`) {
		t.Fatalf("wire request lost explicit empty allowlist: %s", raw)
	}
}

func TestRemoteProfileRestrictionsRequireExplicitProviderEnforcement(t *testing.T) {
	provider := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true}
	restrictions := remoteAgentProfileRestrictionsFromProfile(agentProfile{
		AllowedToolsSpecified: true,
		AllowedToolSpecs:      []string{"Read"},
	})

	err := requireRemoteProfileRestrictionsEnforcement(provider, restrictions)
	if err == nil {
		t.Fatal("snapshot-only provider must not receive a restrictive remote profile")
	}
	if !strings.Contains(err.Error(), "profile restrictions") {
		t.Fatalf("error = %q, want profile restrictions capability guidance", err)
	}
}

func TestRemoteProfileRestrictionsSkipCapabilityForUnrestrictedProfile(t *testing.T) {
	provider := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true}
	if err := requireRemoteProfileRestrictionsEnforcement(provider, nil); err != nil {
		t.Fatalf("unrestricted profile should not require the optional capability: %v", err)
	}
}

func TestAgentRemoteSpawnCarriesResolvedProfileRestrictions(t *testing.T) {
	runtime := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true, enforcesProfile: true}
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := &AgentTool{
		RemoteRuntime: runtime,
		InlineProfiles: map[string]agentProfile{
			"reader": {
				Name:                  "reader",
				AllowedToolsSpecified: true,
				AllowedTools:          map[string]struct{}{"read": {}},
				AllowedToolSpecs:      []string{"Read"},
				DisallowedTools:       map[string]struct{}{"write": {}},
				DisallowedToolSpecs:   []string{"Write"},
			},
		},
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "bypassPermissions",
	}})

	result, err := tool.Execute(context.Background(), agentExecuteInput("remote restricted", map[string]any{
		"isolation":     "remote",
		"subagent_type": "reader",
		"cwd":           child,
	}))
	if err != nil || result.IsError {
		t.Fatalf("remote Execute result=%#v err=%v", result, err)
	}
	if runtime.spawnCalls != 1 || runtime.spawnRequest.ProfileRestrictions == nil {
		t.Fatalf("remote request omitted profile restrictions: %#v", runtime.spawnRequest)
	}
	if got := runtime.spawnRequest.ProfileRestrictions.AllowedToolSpecs; !reflect.DeepEqual(got, []string{"Read"}) {
		t.Fatalf("remote allowed tool specs = %#v", got)
	}
	if got := runtime.spawnRequest.ProfileRestrictions.DisallowedToolSpecs; !reflect.DeepEqual(got, []string{"Write"}) {
		t.Fatalf("remote disallowed tool specs = %#v", got)
	}
	if runtime.spawnRequest.ParentCWD != child || runtime.spawnRequest.PermissionSnapshot.ProjectRoot != child || !reflect.DeepEqual(runtime.spawnRequest.PermissionSnapshot.AllowedDirs, []string{child}) {
		t.Fatalf("remote cwd did not narrow parent scope: %#v", runtime.spawnRequest)
	}
}

func TestAgentRemoteSpawnRejectsCWDOutsideParentScope(t *testing.T) {
	runtime := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true, enforcesProfile: true}
	root := t.TempDir()
	tool := &AgentTool{RemoteRuntime: runtime}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "bypassPermissions",
	}})
	result, err := tool.Execute(context.Background(), agentExecuteInput("remote escape", map[string]any{
		"isolation": "remote", "cwd": t.TempDir(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "outside the parent permission scope") {
		t.Fatalf("remote cwd escape result = %#v", result)
	}
	if runtime.spawnCalls != 0 {
		t.Fatalf("remote cwd escape reached provider: %d calls", runtime.spawnCalls)
	}
}

func TestAgentRemoteSpawnRejectsMissingParentPermissionScope(t *testing.T) {
	runtime := &permissionSnapshotRemoteRuntime{enforcesSnapshot: true, enforcesProfile: true}
	tool := &AgentTool{RemoteRuntime: runtime}
	result, err := tool.Execute(context.Background(), agentExecuteInput("remote without scope", map[string]any{"isolation": "remote"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "complete parent permission snapshot") {
		t.Fatalf("missing remote snapshot result = %#v", result)
	}
	if runtime.spawnCalls != 0 {
		t.Fatalf("missing remote snapshot reached provider: %d calls", runtime.spawnCalls)
	}
}

func TestHTTPRemoteRuntimeUsesCamelCaseSnapshotAndRequiresServerAck(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"taskId":"remote-wire","permissionSnapshotEnforced":true,"profileRestrictionsEnforced":true,"promptRoutingEnforced":true}`))
	}))
	defer server.Close()
	runtime := &HTTPRemoteRuntime{BaseURL: server.URL, AccessToken: "token"}
	_, err := runtime.Spawn(context.Background(), RemoteAgentSpawnRequest{
		Prompt: "run",
		PermissionSnapshot: types.ToolRuntimeContext{
			SessionID: "parent", PermissionMode: "bypassPermissions", AllowedDirs: []string{"/workspace"},
		},
		ProfileRestrictions: &RemoteAgentProfileRestrictions{AllowedToolsSpecified: true, AllowedToolSpecs: []string{"Read"}},
		AvoidPrompts:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := payload["permissionSnapshot"].(map[string]any)
	if !ok || snapshot["permissionMode"] != "bypassPermissions" || snapshot["sessionId"] != "parent" {
		t.Fatalf("remote snapshot wire contract = %#v", payload["permissionSnapshot"])
	}
	if _, leaked := snapshot["PermissionMode"]; leaked {
		t.Fatalf("remote snapshot used Go field names: %#v", snapshot)
	}
	if payload["avoidPrompts"] != true {
		t.Fatalf("remote request did not require fail-closed prompts: %#v", payload)
	}
}
