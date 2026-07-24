# Subagent Permission Inheritance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every subagent enforce the parent permission policy captured at spawn, with no model- or profile-controlled permission-mode input.

**Architecture:** Agent launch defensively copies the parent's `ToolRuntimeContext` into the child registry, permission wrapper, persisted metadata, and resume path. Agent input and custom profiles no longer accept permission modes; internal approval routing controls only where an inherited Ask decision is presented.

**Tech Stack:** Go; existing `tools`, `loop`, `registry`, and `permissions` packages; standard `testing`, `go test`, `go vet`, and `gofmt`.

## Global Constraints

- No new dependencies.
- Mandatory safety, protected paths, sandboxing, and explicit denies remain effective.
- A child captures policy once at spawn and never follows later foreground mode changes.
- Agent/profile inputs may narrow tool visibility but cannot select or grant permission policy.
- Preserve unrelated uncommitted workspace changes.
- Witness a relevant failing test before each production-code change.

## File Responsibilities

- `tools/parse.go`: model-facing Agent invocation shape.
- `tools/agent.go`: schema, profile loaders, child launch, permission wrapper, and registry filtering.
- `tools/agent_cwd.go`: defensive runtime projection into child identity and CWD.
- `tools/agent_definitions.go`: public profile introspection.
- `tools/agent_sessions.go`: retained-session persistence and lifecycle reporting.
- `tools/team.go`: trusted team child launch and approval presentation routing.
- `tools/agent_test.go`, `tools/background_runtime_pin_test.go`, `tools/agent_profile_runtime_test.go`: regression coverage.

---

### Task 1: Remove Permission Policy From Agent and Profile Inputs

**Files:**
- Modify: `tools/parse.go`
- Modify: `tools/agent.go`
- Modify: `tools/agent_definitions.go`
- Test: `tools/agent_test.go`
- Test: `tools/agent_profile_runtime_test.go`

**Interfaces:**
- Produces `AgentInput`, `AgentTool.Schema()`, `agentProfile`, and `AgentDefinition` without permission-mode fields.
- Produces migration error text: `permissionMode is no longer supported; subagents inherit the parent permission policy captured at spawn`.

- [ ] **Step 1: Write failing surface tests**

```go
func TestAgentToolSchemaDoesNotExposePermissionMode(t *testing.T) {
    schema := (&AgentTool{}).Schema()
    if _, ok := schema.Properties["mode"]; ok {
        t.Fatal("Agent schema must not expose permission mode")
    }
}

func TestInlineAgentPermissionModeReturnsMigrationError(t *testing.T) {
    _, err := parseAgentProfilesJSON([]byte(`{"unsafe":{"description":"x","prompt":"y","permissionMode":"bypassPermissions"}}`))
    if err == nil || !strings.Contains(err.Error(), "permissionMode is no longer supported") {
        t.Fatalf("migration error = %v", err)
    }
}
```

Add the equivalent Markdown-profile test for `permissionMode: bypassPermissions` and update classifier tests so only `subagent_type` appears in the tag prefix.

- [ ] **Step 2: Run RED**

Run: `go test ./tools -run 'Test(AgentToolSchemaDoesNotExposePermissionMode|CustomAgentPermissionModeReturnsMigrationError|InlineAgentPermissionModeReturnsMigrationError)' -count=1`

Expected: FAIL because the schema and both profile loaders accept the field.

- [ ] **Step 3: Remove the inputs**

Remove `AgentInput.Mode`, the Agent schema `mode` property, `AgentDefinition.PermissionMode`, `agentProfile.PermissionMode`, built-in profile mode defaults, classifier mode tags, and all mode defaulting/validation based on Agent input.

For Markdown, reject every legacy spelling before building the profile:

```go
if _, present := yamlValueWithPresence(frontmatter,
    "permissionMode", "permission_mode", "permission-mode", "mode"); present {
    return agentProfile{}, false, fmt.Errorf(
        "Agent error: custom agent %q permissionMode is no longer supported; subagents inherit the parent permission policy captured at spawn", name)
}
```

For inline JSON, retain only a `json.RawMessage` legacy detector and return the same migration error when it is non-empty and non-null. It must not populate runtime policy.

- [ ] **Step 4: Run GREEN and compile Agent coverage**

Run: `go test ./tools -run 'Test(AgentToolSchemaDoesNotExposePermissionMode|CustomAgentPermissionModeReturnsMigrationError|InlineAgentPermissionModeReturnsMigrationError)' -count=1`

Run: `go test ./tools -run '^TestAgent' -count=1`

Expected: PASS without restoring a model/profile permission input.

- [ ] **Step 5: Commit with Lore intent `Remove model-controlled subagent permission inputs`**

---

### Task 2: Capture and Enforce the Parent Permission Snapshot

**Files:**
- Modify: `tools/agent.go`
- Modify: `tools/agent_cwd.go`
- Test: `tools/agent_test.go`
- Test: `tools/agent_cwd_test.go`

**Interfaces:**
- Produces `agentLoopOptions.PermissionSnapshot *types.ToolRuntimeContext`.
- Produces `agentSessionMetadata.InheritedPermissionMode string` plus `PermissionSnapshot *types.ToolRuntimeContext`.
- Produces internal `agentApprovalRouting` values `approvalRouteFailClosed` and `approvalRouteParentSession`.

- [ ] **Step 1: Write failing snapshot tests**

```go
func TestAgentToolInheritsParentPermissionModeAtSpawn(t *testing.T) {
    for _, mode := range []string{"bypassPermissions", "default", "plan"} {
        parent := &captureToolPermissionHandler{}
        handler := newAgentPermissionSnapshotHandler(
            types.ToolRuntimeContext{PermissionMode: mode},
            parent,
            approvalRouteParentSession,
            agentProfile{},
        )
        _, err := handler.Check(context.Background(), loop.PermissionRequest{ToolName: "Read"})
        if err != nil {
            t.Fatal(err)
        }
        if len(parent.requests) != 1 || parent.requests[0].Mode != mode {
            t.Fatalf("mode %q produced request %#v", mode, parent.requests)
        }
    }
}

func TestAgentProfileAllowedRuleDoesNotGrantBeyondParent(t *testing.T) {
    parent := &modeGatePermissionHandler{}
    handler := newAgentPermissionSnapshotHandler(
        types.ToolRuntimeContext{PermissionMode: "default"},
        parent,
        approvalRouteParentSession,
        agentProfile{AllowedToolRules: toolPermissionRulesFromYAML([]any{"Bash(git status)"})},
    )
    decision, err := handler.Check(context.Background(), loop.PermissionRequest{
        ToolName: "Bash", Input: map[string]any{"command": "git status"},
    })
    if err != nil {
        t.Fatal(err)
    }
    if decision != loop.PermissionDeny || parent.requests[0].Mode != "default" {
        t.Fatalf("profile rule granted or rewrote permission: decision=%v requests=%#v", decision, parent.requests)
    }
}
```

Add a test that mutates parent maps and slices after launch and verifies the child snapshot is unchanged.

- [ ] **Step 2: Run RED**

Run: `go test ./tools -run 'TestAgentToolInheritsParentPermissionModeAtSpawn|TestAgentProfileAllowedRuleDoesNotGrantBeyondParent|TestAgentPermissionSnapshotIsDefensiveCopy' -count=1`

Expected: FAIL because child mode currently comes from Agent/profile input and profile allowed rules grant `acceptEdits`.

- [ ] **Step 3: Implement trusted snapshot selection**

```go
permissionSnapshot := cloneToolRuntimeContext(launch.session.ToolRuntime)
if opts.PermissionSnapshot != nil {
    permissionSnapshot = cloneToolRuntimeContext(*opts.PermissionSnapshot)
}
inheritedMode := canonicalAgentMode(permissionSnapshot.PermissionMode)
if inheritedMode == "" {
    inheritedMode = permissionModeDefault
}
permissionSnapshot.PermissionMode = inheritedMode
```

Use `inheritedMode` for registry filtering, child runtime, prompt metadata, permission requests, and persisted metadata. Never read it from Agent input or a profile.

Replace the mutable mode wrapper with a snapshot wrapper:

```go
type agentPermissionSnapshotHandler struct {
    parent                loop.PermissionHandler
    snapshot              types.ToolRuntimeContext
    presentationSessionID string
    approvalRouting       agentApprovalRouting
    profile               agentProfile
}
```

`Check` denies profile deny rules, sets `req.Mode` from the snapshot, derives `AvoidPrompts` only from internal routing, preserves presentation/execution identity, and delegates. Delete the profile allowed-rule rewrite to `acceptEdits` and delete mutable `setMode`.

- [ ] **Step 4: Pin child runtime and remove plan-transition exposure**

Rename `agentRuntimeContextProvider.permissionMode` to `inheritedPermissionMode`; initialize it only from the trusted snapshot. Remove the registry exception that exposes `ExitPlanMode` based on child-requested mode.

- [ ] **Step 5: Run GREEN**

Run: `go test ./tools -run 'TestAgentToolInheritsParentPermissionModeAtSpawn|TestAgentProfileAllowedRuleDoesNotGrantBeyondParent|TestAgentPermissionSnapshotIsDefensiveCopy' -count=1`

Run: `go test ./tools ./permissions -count=1`

Expected: PASS, including zero ordinary prompt for an Auto child.

- [ ] **Step 6: Commit with Lore intent `Keep child authority fixed to the parent spawn snapshot`**

---

### Task 3: Persist the Snapshot and Separate Approval Routing

**Files:**
- Modify: `tools/agent.go`
- Modify: `tools/agent_sessions.go`
- Modify: `tools/team.go`
- Modify: `tools/background_tasks.go`
- Modify: `tools/agent_remote.go`
- Test: `tools/background_runtime_pin_test.go`
- Test: `tools/agent_sessions_test.go`
- Test: `tools/team_test.go`

**Interfaces:**
- Consumes Task 2 snapshot metadata.
- Produces restore through `agentLoopOptions.PermissionSnapshot`.
- Produces internal approval routing without policy mutation.

- [ ] **Step 1: Write failing lifecycle tests**

```go
func assertInheritedMode(t *testing.T, handler loop.PermissionHandler, parent *captureToolPermissionHandler, want string) {
    t.Helper()
    if _, err := handler.Check(context.Background(), loop.PermissionRequest{ToolName: "Read"}); err != nil {
        t.Fatal(err)
    }
    if len(parent.requests) != 1 || parent.requests[0].Mode != want {
        t.Fatalf("permission request = %#v, want mode %q", parent.requests, want)
    }
}
```

Use this assertion in three complete fixtures: the first spawns in `default`, switches foreground to `bypassPermissions`, and expects `default`; the second restores persisted `default` under a current Auto foreground and expects `default`; the third invokes a fork-routed handler with child session `agent-session` and presentation session `parent-session`, then asserts `SessionID == "parent-session"`, `ExecutionSessionID == "agent-session"`, `AvoidPrompts == false`, and `Mode` equals the captured parent mode.

Add a remote-runtime capture fixture that asserts `RemoteAgentSpawnRequest` contains a defensive copy of the parent permission snapshot. A remote provider that reports it cannot enforce permission snapshots must make Agent launch return an error before spawning.

- [ ] **Step 2: Run RED**

Run: `go test ./tools -run 'Test(BackgroundAgentKeepsSpawnPermissionSnapshotAfterForegroundSwitch|RestoredAgentUsesPersistedPermissionSnapshot|ForkApprovalRoutingDoesNotChangeInheritedPermissionMode)' -count=1`

Expected: FAIL because restore and fork routing still depend on Agent/profile mode fields.

- [ ] **Step 3: Persist and restore defensive snapshots**

Store a cloned `ToolRuntimeContext` pointer in metadata. Clone the pointed value whenever task metadata is copied. Restore with:

```go
snapshot := cloneToolRuntimeContext(*metadata.PermissionSnapshot)
opts.PermissionSnapshot = &snapshot
```

For legacy metadata without a complete trusted snapshot, fail closed. Its recorded mode may have originated from the removed model/profile-controlled field, so it cannot become a permission authority during migration.

- [ ] **Step 4: Separate routing**

Fork/team trusted launches with a parent presentation session use `approvalRouteParentSession`; unattended background launches use `approvalRouteFailClosed`. Routing may set `AvoidPrompts` and presentation IDs but never `req.Mode`. Remove retained-session code that mutates permission mode after plan approval. Lifecycle reporting reads inherited metadata, not Agent input.

Extend the remote spawn request and provider capability contract with the captured snapshot. Reject the launch when the provider cannot enforce it; do not send `default` as a fallback.

- [ ] **Step 5: Run GREEN**

Run: `go test ./tools -run 'Test(BackgroundAgentKeepsSpawnPermissionSnapshotAfterForegroundSwitch|RestoredAgentUsesPersistedPermissionSnapshot|ForkApprovalRoutingDoesNotChangeInheritedPermissionMode)' -count=1`

Run: `go test ./tools -count=1`

Expected: PASS with foreground transitions isolated from existing children.

- [ ] **Step 6: Commit with Lore intent `Preserve inherited child policy across background lifecycle`**

---

### Task 4: Full Verification and Review

**Files:**
- Modify only implementation files required to resolve failures caused by Tasks 1-3.
- Review: `docs/superpowers/specs/2026-07-14-subagent-permission-inheritance-design.md`

- [ ] **Step 1: Format and inspect**

Run: `gofmt -w` on every modified Go file.

Run: `git diff --check` and `git diff --stat`.

Expected: no whitespace errors and no unrelated provider/catalog or tool-results files staged.

- [ ] **Step 2: Static and targeted verification**

Run: `go vet ./tools ./permissions ./registry ./loop`

Run: `go test ./tools ./permissions ./registry ./loop -count=1`

Run: `go test -race ./tools ./permissions -count=1`

Expected: every command exits 0.

- [ ] **Step 3: Full suite**

Run: `go test ./... -count=1`

Expected: all packages pass. Document any independently reproducible pre-existing failure instead of changing unrelated user work.

- [ ] **Step 4: Request focused review**

Dispatch a read-only reviewer with the approved spec, this plan, base SHA `d1a33a5`, current HEAD, and the permission diff. Fix every Critical or Important issue and rerun affected verification.

- [ ] **Step 5: Commit verification fixes with Lore trailers**
