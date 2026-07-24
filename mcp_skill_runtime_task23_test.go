package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/skills"
)

func TestTask23MCPRuntimeBridgeLifecycleAndWorkspaceIsolation(t *testing.T) {
	t.Setenv(skills.FeatureFlagMCPSkills, "1")
	rootA := t.TempDir()
	rootB := t.TempDir()
	skillManager := task23NewMCPRuntimeSkillManager(t, rootA)
	fixture := newTask23MCPRuntimeFixture()
	mcpManager := svcmcp.NewManager(svcmcp.WithTransportFactory(fixture.transportFactory))
	t.Cleanup(func() { _ = mcpManager.Shutdown(context.Background()) })
	projectConfig := svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fixture", Scope: svcmcp.ScopeProject}
	mcpManager.AddConfig("project-mcp", projectConfig)
	if _, err := mcpManager.GetOrConnect(context.Background(), "project-mcp"); err != nil {
		t.Fatal(err)
	}

	bridge := newMCPSkillRuntimeBridge(skillManager, mcpManager)
	if bridge == nil {
		t.Fatal("bridge was not created")
	}
	t.Cleanup(bridge.close)
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	initialInputs := task23WaitForMCPInputs(t, skillManager, 2)
	initialIDs := task23MCPInputIDs(initialInputs)
	initialSnapshot := task23MCPSnapshot(t, skillManager, "task23-mcp")

	fixture.update("resource-v2", "prompt-v1 $topic")
	fixture.notify(t, svcmcp.NotificationResourcesListChanged)
	task23WaitForMCPInputBodies(t, skillManager, "resource-v2", "<user>\nprompt-v1 $topic")
	fixture.update("resource-v2", "prompt-v2 $topic")
	fixture.notify(t, svcmcp.NotificationPromptsListChanged)
	updatedInputs := task23WaitForMCPInputBodies(t, skillManager, "resource-v2", "<user>\nprompt-v2 $topic")
	if got := task23MCPInputIDs(updatedInputs); !reflect.DeepEqual(got, initialIDs) {
		t.Fatalf("content update changed stable IDs: before=%v after=%v", initialIDs, got)
	}
	updatedSnapshot := task23MCPSnapshot(t, skillManager, "task23-mcp")
	updatedDelta, err := skills.DiffCatalog(initialSnapshot, updatedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedDelta.Upserts) != 2 || len(updatedDelta.Revokes) != 0 {
		t.Fatalf("prompt/resource update delta = %#v", updatedDelta)
	}

	// A transient read failure must not publish the concurrently fetched prompt
	// update as a partial catalog. The previous prompt+resource pair survives.
	epochBeforeFailure := task23MCPBridgeEpoch(bridge)
	fixture.update("resource-invalid", "prompt-invalid $topic")
	fixture.setResourceReadError(true)
	fixture.notify(t, svcmcp.NotificationResourcesListChanged)
	task23WaitForMCPBridgeEpoch(t, bridge, epochBeforeFailure)
	fixture.setResourceReadError(false)
	retainedAfterFailure := skillManager.MCPCatalogInputs()
	if got := task23MCPInputIDs(retainedAfterFailure); !reflect.DeepEqual(got, initialIDs) {
		t.Fatalf("failed refresh changed identities: %v", got)
	}
	for _, input := range retainedAfterFailure {
		if input.Skill.Content == "resource-invalid" || input.Skill.Content == "<user>\nprompt-invalid $topic" {
			t.Fatalf("failed refresh partially published input: %#v", input)
		}
	}
	fixture.update("resource-v2", "prompt-v2 $topic")

	if _, err := mcpManager.ToggleEnabled(context.Background(), "project-mcp", false); err != nil {
		t.Fatal(err)
	}
	task23WaitForMCPInputs(t, skillManager, 0)
	disabledSnapshot := task23MCPSnapshot(t, skillManager, "task23-mcp")
	disabledDelta, err := skills.DiffCatalog(updatedSnapshot, disabledSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(disabledDelta.Revokes) != 2 {
		t.Fatalf("disconnect delta = %#v", disabledDelta)
	}

	if _, err := mcpManager.ToggleEnabled(context.Background(), "project-mcp", true); err != nil {
		t.Fatal(err)
	}
	reconnected := task23WaitForMCPInputBodies(t, skillManager, "resource-v2", "<user>\nprompt-v2 $topic")
	if got := task23MCPInputIDs(reconnected); !reflect.DeepEqual(got, initialIDs) {
		t.Fatalf("reconnect changed IDs: before=%v after=%v", initialIDs, got)
	}
	reconnectedSnapshot := task23MCPSnapshot(t, skillManager, "task23-mcp")
	reconnectedDelta, err := skills.DiffCatalog(disabledSnapshot, reconnectedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconnectedDelta.Upserts) != 2 || len(reconnectedDelta.Revokes) != 0 {
		t.Fatalf("reconnect delta = %#v", reconnectedDelta)
	}

	// Same-workspace epoch mismatch must retain the newer projection, not clear
	// it or overwrite it with the projection captured by prepare.
	planSame, err := skillManager.PrepareProjectSources(rootA)
	if err != nil {
		t.Fatal(err)
	}
	preparedSame := bridge.prepare(context.Background(), map[string]svcmcp.MCPServerConfig{"project-mcp": projectConfig}, true)
	if err := skillManager.StageMCPCatalogInputs(planSame, preparedSame.inputs); err != nil {
		t.Fatal(err)
	}
	fixture.update("resource-same-new", "prompt-same-new $topic")
	fixture.notify(t, svcmcp.NotificationResourcesListChanged)
	fixture.notify(t, svcmcp.NotificationPromptsListChanged)
	task23WaitForMCPInputBodies(t, skillManager, "resource-same-new", "<user>\nprompt-same-new $topic")
	if preparedSame.epoch == task23MCPBridgeEpoch(bridge) {
		t.Fatal("same-workspace test did not advance bridge epoch")
	}
	if err := bridge.withPrepared(preparedSame, planSame, func() error {
		return skillManager.ApplyProjectSources(planSame)
	}); err != nil {
		t.Fatal(err)
	}
	task23WaitForMCPInputBodies(t, skillManager, "resource-same-new", "<user>\nprompt-same-new $topic")

	// A callback/transition failure is pre-publication: generation, directories,
	// unified MCP inputs, and bridge epoch all stay byte-for-byte unchanged.
	failedPlan, err := skillManager.PrepareProjectSources(rootA)
	if err != nil {
		t.Fatal(err)
	}
	failedProjection := bridge.prepare(context.Background(), map[string]svcmcp.MCPServerConfig{"project-mcp": projectConfig}, true)
	if err := skillManager.StageMCPCatalogInputs(failedPlan, failedProjection.inputs); err != nil {
		t.Fatal(err)
	}
	beforeFailureGeneration := skillManager.ProjectGeneration()
	beforeFailureInputs := skillManager.MCPCatalogInputs()
	beforeFailureNames := task23SnapshotNamesForRootTest(t, skillManager)
	beforeFailureEpoch := task23MCPBridgeEpoch(bridge)
	sentinel := errors.New("task23 transition failure")
	if err := bridge.withPrepared(failedProjection, failedPlan, func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("transition error = %v, want sentinel", err)
	}
	task23AssertMCPRuntimeUnchanged(t, skillManager, bridge, beforeFailureGeneration, beforeFailureInputs, beforeFailureNames, beforeFailureEpoch)

	// A project settings revision conflict is also rejected before the atomic
	// project+MCP plan publishes.
	rootConflict := t.TempDir()
	conflictPlan, err := skillManager.PrepareProjectSources(rootConflict)
	if err != nil {
		t.Fatal(err)
	}
	conflictProjection := bridge.prepare(context.Background(), nil, false)
	if err := skillManager.StageMCPCatalogInputs(conflictPlan, conflictProjection.inputs); err != nil {
		t.Fatal(err)
	}
	paths, err := skills.DefaultOverrideStorePaths(rootConflict)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.ProjectSettings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ProjectSettings, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeConflictGeneration := skillManager.ProjectGeneration()
	beforeConflictInputs := skillManager.MCPCatalogInputs()
	beforeConflictNames := task23SnapshotNamesForRootTest(t, skillManager)
	beforeConflictEpoch := task23MCPBridgeEpoch(bridge)
	if err := bridge.withPrepared(conflictProjection, conflictPlan, func() error {
		return skillManager.ApplyProjectSources(conflictPlan)
	}); !errors.Is(err, skills.ErrOverrideRevisionConflict) {
		t.Fatalf("revision conflict error = %v", err)
	}
	task23AssertMCPRuntimeUnchanged(t, skillManager, bridge, beforeConflictGeneration, beforeConflictInputs, beforeConflictNames, beforeConflictEpoch)

	// Prepare B, then consume a list_changed refresh before commit. The bridge
	// epoch must restage the latest successful projection rather than publishing
	// the older prepared one. Because the server is project-scoped, B still gets
	// an empty projection: neither prompt nor resource may leak from A.
	planB, err := skillManager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	preparedB := bridge.prepare(context.Background(), nil, false)
	if err := skillManager.StageMCPCatalogInputs(planB, preparedB.inputs); err != nil {
		t.Fatal(err)
	}
	fixture.update("resource-v3", "prompt-v3 $topic")
	fixture.notify(t, svcmcp.NotificationResourcesListChanged)
	task23WaitForMCPInputBodies(t, skillManager, "resource-v3", "<user>\nprompt-v3 $topic")
	if preparedB.epoch == task23MCPBridgeEpoch(bridge) {
		t.Fatal("test did not advance bridge epoch between prepare and commit")
	}
	if err := bridge.withPrepared(preparedB, planB, func() error {
		if err := skillManager.ApplyProjectSources(planB); err != nil {
			return err
		}
		mcpManager.SetWorkingDirectory(rootB)
		mcpManager.ReplaceWorkspaceConfigs(nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := skillManager.MCPCatalogInputs(); len(got) != 0 {
		t.Fatalf("workspace B retained project MCP prompts/resources: %#v", got)
	}
	if snapshot := task23MCPSnapshot(t, skillManager, "task23-mcp"); len(snapshot.Skills) != 0 {
		t.Fatalf("workspace B catalog leaked MCP rows: %#v", snapshot.Skills)
	}
}

func TestTask23EligibleMCPStatesExcludeOldWorkspaceAndTargetShadow(t *testing.T) {
	states := []svcmcp.MCPServerConnection{
		{Name: "old-project", Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "old", Scope: svcmcp.ScopeProject}},
		{Name: "old-local", Config: svcmcp.MCPServerConfig{Scope: svcmcp.ScopeLocal}},
		{Name: "shadowed", Config: svcmcp.MCPServerConfig{Scope: svcmcp.ScopeUser}},
		{Name: "retained", Config: svcmcp.MCPServerConfig{Scope: svcmcp.ScopeManaged}},
		{Name: "same-project", Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "same", Scope: svcmcp.ScopeProject}},
		{Name: "changed-project", Config: svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "before", Scope: svcmcp.ScopeProject}},
	}
	target := map[string]svcmcp.MCPServerConfig{
		"shadowed":        {Type: svcmcp.TransportStdio, Command: "target", Scope: svcmcp.ScopeProject},
		"same-project":    {Type: svcmcp.TransportStdio, Command: "same", Scope: svcmcp.ScopeProject},
		"changed-project": {Type: svcmcp.TransportStdio, Command: "after", Scope: svcmcp.ScopeProject},
	}
	eligible := eligibleMCPStatesForTarget(states, target, false)
	if len(eligible) != 1 || eligible[0].Name != "retained" {
		t.Fatalf("cross-workspace eligible states = %#v", eligible)
	}
	same := eligibleMCPStatesForTarget(states, target, true)
	gotNames := make([]string, 0, len(same))
	for _, state := range same {
		gotNames = append(gotNames, state.Name)
	}
	if want := []string{"retained", "same-project"}; !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("same-workspace eligible names = %v, want %v", gotNames, want)
	}
}

func TestTask23MCPRuntimeBridgeTargetShadowAndPersistentRestore(t *testing.T) {
	t.Setenv(skills.FeatureFlagMCPSkills, "1")
	rootA := t.TempDir()
	rootB := t.TempDir()
	rootC := t.TempDir()
	skillManager := task23NewMCPRuntimeSkillManager(t, rootA)
	fixture := newTask23MCPRuntimeFixture()
	mcpManager := svcmcp.NewManager(svcmcp.WithTransportFactory(fixture.transportFactory))
	t.Cleanup(func() { _ = mcpManager.Shutdown(context.Background()) })
	userConfig := svcmcp.MCPServerConfig{Type: svcmcp.TransportHTTP, URL: "https://user.example.test/mcp", Scope: svcmcp.ScopeUser}
	projectConfig := svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "project", Scope: svcmcp.ScopeProject}
	mcpManager.AddConfig("shared", userConfig)
	if _, err := mcpManager.GetOrConnect(context.Background(), "shared"); err != nil {
		t.Fatal(err)
	}
	bridge := newMCPSkillRuntimeBridge(skillManager, mcpManager)
	t.Cleanup(bridge.close)
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	task23WaitForMCPInputBodies(t, skillManager, "resource-v1", "<user>\nprompt-v1 $topic")

	planB, err := skillManager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	preparedB := bridge.prepare(context.Background(), map[string]svcmcp.MCPServerConfig{"shared": projectConfig}, false)
	if err := skillManager.StageMCPCatalogInputs(planB, preparedB.inputs); err != nil {
		t.Fatal(err)
	}
	if len(preparedB.inputs) != 0 {
		t.Fatalf("target-shadowed user projection was staged: %#v", preparedB.inputs)
	}
	if err := bridge.withPrepared(preparedB, planB, func() error {
		if err := skillManager.ApplyProjectSources(planB); err != nil {
			return err
		}
		mcpManager.SetWorkingDirectory(rootB)
		mcpManager.ReplaceWorkspaceConfigs(map[string]svcmcp.MCPServerConfig{"shared": projectConfig})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := skillManager.MCPCatalogInputs(); len(got) != 0 {
		t.Fatalf("target workspace retained shadowed user skills: %#v", got)
	}
	if state, ok := mcpManager.State("shared"); !ok || state.Config.Scope != svcmcp.ScopeProject || state.Type != svcmcp.MCPStatePending {
		t.Fatalf("target shadow state = %#v ok=%v", state, ok)
	}

	fixture.update("project-resource", "project-prompt $topic")
	if _, err := mcpManager.GetOrConnect(context.Background(), "shared"); err != nil {
		t.Fatal(err)
	}
	task23WaitForMCPInputBodies(t, skillManager, "project-resource", "<user>\nproject-prompt $topic")

	planC, err := skillManager.PrepareProjectSources(rootC)
	if err != nil {
		t.Fatal(err)
	}
	preparedC := bridge.prepare(context.Background(), nil, false)
	if err := skillManager.StageMCPCatalogInputs(planC, preparedC.inputs); err != nil {
		t.Fatal(err)
	}
	if err := bridge.withPrepared(preparedC, planC, func() error {
		if err := skillManager.ApplyProjectSources(planC); err != nil {
			return err
		}
		mcpManager.SetWorkingDirectory(rootC)
		mcpManager.ReplaceWorkspaceConfigs(nil)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := skillManager.MCPCatalogInputs(); len(got) != 0 {
		t.Fatalf("leaving target retained project skills: %#v", got)
	}
	if state, ok := mcpManager.State("shared"); !ok || state.Config.Scope != svcmcp.ScopeUser || state.Type != svcmcp.MCPStatePending {
		t.Fatalf("restored persistent state = %#v ok=%v", state, ok)
	}

	fixture.update("user-restored", "user-restored $topic")
	if _, err := mcpManager.GetOrConnect(context.Background(), "shared"); err != nil {
		t.Fatal(err)
	}
	task23WaitForMCPInputBodies(t, skillManager, "user-restored", "<user>\nuser-restored $topic")
}

func TestTask23MCPRuntimeBridgeAutomaticDisconnectReconnectAndUnregister(t *testing.T) {
	t.Setenv(skills.FeatureFlagMCPSkills, "1")
	root := t.TempDir()
	skillManager := task23NewMCPRuntimeSkillManager(t, root)
	fixture := newTask23MCPRuntimeFixture()
	mcpManager := svcmcp.NewManager(
		svcmcp.WithReconnectPolicy(svcmcp.ReconnectPolicy{
			RemoteMaxAttempts: 2, RemoteInitialDelay: time.Millisecond, RemoteMaxDelay: time.Millisecond,
			ConnectionLostThreshold: 2, StdioCooldowns: []time.Duration{time.Millisecond, time.Millisecond},
		}),
		svcmcp.WithTransportFactory(fixture.transportFactory),
	)
	t.Cleanup(func() { _ = mcpManager.Shutdown(context.Background()) })
	mcpManager.AddConfig("project-mcp", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fixture", Scope: svcmcp.ScopeProject})
	if _, err := mcpManager.GetOrConnect(context.Background(), "project-mcp"); err != nil {
		t.Fatal(err)
	}
	bridge := newMCPSkillRuntimeBridge(skillManager, mcpManager)
	t.Cleanup(bridge.close)
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	initial := task23WaitForMCPInputs(t, skillManager, 2)
	initialIDs := task23MCPInputIDs(initial)

	connectStarted, connectRelease := fixture.blockNextConnect()
	fixture.update("reconnected-resource", "reconnected-prompt $topic")
	if err := fixture.closeCurrentServer(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-connectStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("automatic reconnect did not reach the transport factory")
	}
	task23WaitForMCPInputs(t, skillManager, 0)
	close(connectRelease)
	reconnected := task23WaitForMCPInputBodies(t, skillManager, "reconnected-resource", "<user>\nreconnected-prompt $topic")
	if got := task23MCPInputIDs(reconnected); !reflect.DeepEqual(got, initialIDs) {
		t.Fatalf("automatic reconnect changed stable IDs: before=%v after=%v", initialIDs, got)
	}

	bridge.close()
	before := skillManager.MCPCatalogInputs()
	beforeEpoch := task23MCPBridgeEpoch(bridge)
	dispatched := make(chan struct{}, 1)
	unregisterProbe := mcpManager.RegisterCatalogChangeHook(func() {
		select {
		case dispatched <- struct{}{}:
		default:
		}
	})
	defer unregisterProbe()
	if _, err := mcpManager.ToggleEnabled(context.Background(), "project-mcp", false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-dispatched:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not dispatch the post-close catalog change")
	}
	if got := skillManager.MCPCatalogInputs(); !reflect.DeepEqual(got, before) {
		t.Fatalf("unregistered bridge observed later manager change:\ngot=%#v\nwant=%#v", got, before)
	}
	if got := task23MCPBridgeEpoch(bridge); got != beforeEpoch {
		t.Fatalf("unregistered bridge epoch = %d, want %d", got, beforeEpoch)
	}
}

func TestTask23MCPRuntimeBridgeRejectsLateDiscoveryAfterProjectGenerationChange(t *testing.T) {
	t.Setenv(skills.FeatureFlagMCPSkills, "1")
	rootA := t.TempDir()
	rootB := t.TempDir()
	skillManager := task23NewMCPRuntimeSkillManager(t, rootA)
	fixture := newTask23MCPRuntimeFixture()
	mcpManager := svcmcp.NewManager(svcmcp.WithTransportFactory(fixture.transportFactory))
	t.Cleanup(func() { _ = mcpManager.Shutdown(context.Background()) })
	mcpManager.AddConfig("project-mcp", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fixture", Scope: svcmcp.ScopeProject})
	if _, err := mcpManager.GetOrConnect(context.Background(), "project-mcp"); err != nil {
		t.Fatal(err)
	}
	bridge := newMCPSkillRuntimeBridge(skillManager, mcpManager)
	t.Cleanup(bridge.close)
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	started, release := fixture.blockNextPromptGet()
	fixture.update("late-resource-a", "late-prompt-a $topic")
	refreshDone := make(chan error, 1)
	go func() { refreshDone <- bridge.refresh(context.Background()) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("late A discovery did not reach prompts/get")
	}

	planB, err := skillManager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := skillManager.StageMCPCatalogInputs(planB, nil); err != nil {
		t.Fatal(err)
	}
	if err := skillManager.ApplyProjectSources(planB); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-refreshDone:
		if !errors.Is(err, skills.ErrSkillProjectGenerationChanged) {
			t.Fatalf("late discovery error = %v, want project generation rejection", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("late discovery did not finish")
	}
	if got := skillManager.MCPCatalogInputs(); len(got) != 0 {
		t.Fatalf("late A discovery populated B: %#v", got)
	}
}

func TestTask23MCPRuntimeBridgeCloseWaitsForInflightAndMakesRefreshInert(t *testing.T) {
	t.Setenv(skills.FeatureFlagMCPSkills, "1")
	root := t.TempDir()
	skillManager := task23NewMCPRuntimeSkillManager(t, root)
	fixture := newTask23MCPRuntimeFixture()
	mcpManager := svcmcp.NewManager(svcmcp.WithTransportFactory(fixture.transportFactory))
	t.Cleanup(func() { _ = mcpManager.Shutdown(context.Background()) })
	mcpManager.AddConfig("project-mcp", svcmcp.MCPServerConfig{Type: svcmcp.TransportStdio, Command: "fixture", Scope: svcmcp.ScopeProject})
	if _, err := mcpManager.GetOrConnect(context.Background(), "project-mcp"); err != nil {
		t.Fatal(err)
	}
	bridge := newMCPSkillRuntimeBridge(skillManager, mcpManager)
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	started, release := fixture.blockNextPromptGet()
	fixture.update("before-close", "before-close $topic")
	refreshDone := make(chan error, 1)
	go func() { refreshDone <- bridge.refresh(context.Background()) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight refresh did not reach prompts/get")
	}
	closeDone := make(chan struct{})
	go func() {
		bridge.close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("bridge close returned while discovery could still publish")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-refreshDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight refresh did not finish")
	}
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("bridge close did not wait and finish")
	}

	before := skillManager.MCPCatalogInputs()
	beforeEpoch := task23MCPBridgeEpoch(bridge)
	fixture.update("after-close", "after-close $topic")
	if err := bridge.refresh(context.Background()); err != nil {
		t.Fatalf("closed bridge refresh = %v", err)
	}
	if got := skillManager.MCPCatalogInputs(); !reflect.DeepEqual(got, before) {
		t.Fatalf("closed bridge mutated catalog:\ngot=%#v\nwant=%#v", got, before)
	}
	if got := task23MCPBridgeEpoch(bridge); got != beforeEpoch {
		t.Fatalf("closed bridge epoch = %d, want %d", got, beforeEpoch)
	}
}

func task23NewMCPRuntimeSkillManager(t *testing.T, root string) *skills.Manager {
	t.Helper()
	sessionLayer := skills.NewMemorySessionOverrideLayer()
	store, err := skills.NewFileOverrideStore(root, nil, sessionLayer)
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManagerWithOverrideStore(store)
	if err := manager.ReplaceProjectSources(root); err != nil {
		t.Fatal(err)
	}
	return manager
}

func task23WaitForMCPInputs(t *testing.T, manager *skills.Manager, count int) []skills.MCPCatalogInput {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		inputs := manager.MCPCatalogInputs()
		if len(inputs) == count {
			return inputs
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP input count = %d, want %d: %#v", len(inputs), count, inputs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func task23WaitForMCPInputBodies(t *testing.T, manager *skills.Manager, bodies ...string) []skills.MCPCatalogInput {
	t.Helper()
	want := append([]string(nil), bodies...)
	sort.Strings(want)
	deadline := time.Now().Add(3 * time.Second)
	for {
		inputs := manager.MCPCatalogInputs()
		got := make([]string, 0, len(inputs))
		for _, input := range inputs {
			got = append(got, input.Skill.Content)
		}
		sort.Strings(got)
		if reflect.DeepEqual(got, want) {
			return inputs
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP bodies = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func task23MCPBridgeEpoch(bridge *mcpSkillRuntimeBridge) uint64 {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	return bridge.epoch
}

func task23WaitForMCPBridgeEpoch(t *testing.T, bridge *mcpSkillRuntimeBridge, previous uint64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if task23MCPBridgeEpoch(bridge) > previous {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge epoch did not advance beyond %d", previous)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func task23MCPInputIDs(inputs []skills.MCPCatalogInput) []skills.SkillID {
	ids := make([]skills.SkillID, 0, len(inputs))
	for _, input := range inputs {
		ids = append(ids, input.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func task23SnapshotNamesForRootTest(t *testing.T, manager *skills.Manager) []string {
	t.Helper()
	snapshot, err := manager.Snapshot("task23-runtime-state")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(snapshot.Skills))
	for _, row := range snapshot.Skills {
		names = append(names, row.Name)
	}
	sort.Strings(names)
	return names
}

func task23AssertMCPRuntimeUnchanged(
	t *testing.T,
	manager *skills.Manager,
	bridge *mcpSkillRuntimeBridge,
	wantGeneration skills.ProjectSourceGeneration,
	wantInputs []skills.MCPCatalogInput,
	wantNames []string,
	wantEpoch uint64,
) {
	t.Helper()
	if got := manager.ProjectGeneration(); got != wantGeneration {
		t.Fatalf("project generation changed: got %d want %d", got, wantGeneration)
	}
	if got := manager.MCPCatalogInputs(); !reflect.DeepEqual(got, wantInputs) {
		t.Fatalf("MCP inputs changed:\ngot=%#v\nwant=%#v", got, wantInputs)
	}
	if got := task23SnapshotNamesForRootTest(t, manager); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("catalog names changed: got %v want %v", got, wantNames)
	}
	if got := task23MCPBridgeEpoch(bridge); got != wantEpoch {
		t.Fatalf("bridge epoch changed: got %d want %d", got, wantEpoch)
	}
}

func task23MCPSnapshot(t *testing.T, manager *skills.Manager, sessionID string) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]skills.EffectiveSkill, 0, len(snapshot.Skills))
	for _, row := range snapshot.Skills {
		if row.Source == skills.SourceMCP {
			rows = append(rows, row)
		}
	}
	filtered, err := skills.NewCatalogSnapshot(snapshot.Revision, rows)
	if err != nil {
		t.Fatal(err)
	}
	return filtered
}

type task23MCPRuntimeFixture struct {
	mu             sync.Mutex
	resourceBody   string
	promptBody     string
	resourceErr    bool
	server         svcmcp.Transport
	promptStarted  chan struct{}
	promptRelease  chan struct{}
	connectStarted chan struct{}
	connectRelease chan struct{}
}

func (f *task23MCPRuntimeFixture) setResourceReadError(fail bool) {
	f.mu.Lock()
	f.resourceErr = fail
	f.mu.Unlock()
}

func newTask23MCPRuntimeFixture() *task23MCPRuntimeFixture {
	return &task23MCPRuntimeFixture{resourceBody: "resource-v1", promptBody: "prompt-v1 $topic"}
}

func (f *task23MCPRuntimeFixture) transportFactory(_ context.Context, _ string, _ svcmcp.MCPServerConfig, _ svcmcp.TransportBuildOptions) (svcmcp.Transport, error) {
	f.mu.Lock()
	connectStarted := f.connectStarted
	connectRelease := f.connectRelease
	f.connectStarted = nil
	f.connectRelease = nil
	f.mu.Unlock()
	if connectStarted != nil {
		close(connectStarted)
		<-connectRelease
	}
	client, server := svcmcp.CreateLinkedTransportPair()
	f.mu.Lock()
	f.server = server
	f.mu.Unlock()
	go f.serve(server)
	return client, nil
}

func (f *task23MCPRuntimeFixture) blockNextConnect() (<-chan struct{}, chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	started := make(chan struct{})
	release := make(chan struct{})
	f.connectStarted = started
	f.connectRelease = release
	return started, release
}

func (f *task23MCPRuntimeFixture) closeCurrentServer() error {
	f.mu.Lock()
	server := f.server
	f.mu.Unlock()
	if server == nil {
		return errors.New("task23 fixture has no current server")
	}
	return server.Close()
}

func (f *task23MCPRuntimeFixture) update(resourceBody, promptBody string) {
	f.mu.Lock()
	f.resourceBody = resourceBody
	f.promptBody = promptBody
	f.mu.Unlock()
}

func (f *task23MCPRuntimeFixture) blockNextPromptGet() (<-chan struct{}, chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	started := make(chan struct{})
	release := make(chan struct{})
	f.promptStarted = started
	f.promptRelease = release
	return started, release
}

func (f *task23MCPRuntimeFixture) notify(t *testing.T, method string) {
	t.Helper()
	f.mu.Lock()
	server := f.server
	f.mu.Unlock()
	message, err := svcmcp.NewNotificationMessage(method, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
}

func (f *task23MCPRuntimeFixture) serve(server svcmcp.Transport) {
	for {
		message, err := server.Receive(context.Background())
		if err != nil {
			return
		}
		var result any
		switch message.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": svcmcp.MCPProtocolVersion,
				"capabilities": map[string]any{
					"resources": map[string]any{"listChanged": true},
					"prompts":   map[string]any{"listChanged": true},
				},
				"serverInfo": map[string]any{"name": "task23", "version": "1"},
			}
		case "notifications/initialized":
			continue
		case "resources/list":
			result = svcmcp.ListResourcesResult{Resources: []svcmcp.Resource{{URI: "skill://task23/SKILL.md", Name: "resource"}}}
		case "resources/read":
			f.mu.Lock()
			body := f.resourceBody
			fail := f.resourceErr
			f.mu.Unlock()
			if fail {
				response := svcmcp.JSONRPCMessage{JSONRPC: svcmcp.JSONRPCVersion, ID: message.ID, Error: &svcmcp.RPCError{Code: -32000, Message: "transient read failure"}}
				_ = server.Send(context.Background(), response)
				continue
			}
			result = svcmcp.ReadResourceResult{Contents: []svcmcp.ResourceContent{{URI: "skill://task23/SKILL.md", MimeType: "text/markdown", Text: "---\ndescription: Task 23 resource\n---\n" + body}}}
		case "prompts/list":
			result = svcmcp.ListPromptsResult{Prompts: []svcmcp.PromptDefinition{{
				Name: "prompt", Description: "Task 23 prompt",
				Arguments: []svcmcp.PromptArgument{{Name: "topic", Required: true}},
			}}}
		case "prompts/get":
			f.mu.Lock()
			body := f.promptBody
			started := f.promptStarted
			release := f.promptRelease
			f.promptStarted = nil
			f.promptRelease = nil
			f.mu.Unlock()
			if started != nil {
				close(started)
				<-release
			}
			result = svcmcp.GetPromptResult{Description: "Task 23 prompt", Messages: []svcmcp.PromptMessage{{
				Role: "user", Content: svcmcp.PromptContent{Type: "text", Text: body},
			}}}
		default:
			if len(message.ID) == 0 {
				continue
			}
			response := svcmcp.JSONRPCMessage{JSONRPC: svcmcp.JSONRPCVersion, ID: message.ID, Error: &svcmcp.RPCError{Code: -32601, Message: fmt.Sprintf("unexpected %s", message.Method)}}
			_ = server.Send(context.Background(), response)
			continue
		}
		if len(message.ID) == 0 {
			continue
		}
		response, err := svcmcp.NewResultMessage(message.ID, result)
		if err != nil {
			return
		}
		_ = server.Send(context.Background(), response)
	}
}
