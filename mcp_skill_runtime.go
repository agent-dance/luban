package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/skills"
)

const mcpSkillCatalogRefreshTimeout = 10 * time.Second

type preparedMCPSkillProjection struct {
	inputs        []skills.MCPCatalogInput
	targetConfigs map[string]svcmcp.MCPServerConfig
	sameWorkspace bool
	epoch         uint64
}

// mcpSkillRuntimeBridge is the single production bridge between one
// services/mcp Manager and the RegistryDeps-owned skill Manager. Its mutex
// serializes remote discovery with workspace publication, closing the window
// where an old MCP callback could sample A after the skill authority became B.
type mcpSkillRuntimeBridge struct {
	mu           sync.Mutex
	skillManager *skills.Manager
	mcpManager   *svcmcp.Manager
	unregister   func()
	epoch        uint64
	closed       bool
	closeOnce    sync.Once
}

func (b *mcpSkillRuntimeBridge) close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		// Mark the bridge closed while sharing the same mutex as refresh. This
		// waits for an in-flight discovery/publication to finish and makes a
		// hook already copied by the manager dispatcher inert before close
		// returns. Merely unregistering the hook is insufficient because the
		// dispatcher intentionally invokes its snapshot outside Manager.mu.
		b.mu.Lock()
		b.closed = true
		unregister := b.unregister
		b.unregister = nil
		b.mu.Unlock()
		if unregister != nil {
			unregister()
		}
	})
}

func newMCPSkillRuntimeBridge(skillManager *skills.Manager, mcpManager *svcmcp.Manager) *mcpSkillRuntimeBridge {
	if skillManager == nil || mcpManager == nil {
		return nil
	}
	bridge := &mcpSkillRuntimeBridge{skillManager: skillManager, mcpManager: mcpManager}
	bridge.unregister = mcpManager.RegisterCatalogChangeHook(func() {
		ctx, cancel := context.WithTimeout(context.Background(), mcpSkillCatalogRefreshTimeout)
		defer cancel()
		_ = bridge.refresh(ctx)
	})
	return bridge
}

func (b *mcpSkillRuntimeBridge) refresh(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	// Capture the workspace capability before performing remote I/O. A
	// project retarget that does not share this bridge mutex (for example an
	// embedder using Manager directly) then makes the final publication fail
	// closed instead of letting an A discovery populate B.
	generation := b.skillManager.ProjectGeneration()
	states := b.mcpManager.Snapshot()
	inputs, discoverErr := skills.DiscoverMCPCatalogInputsFromConnections(ctx, states)
	if discoverErr != nil {
		inputs = b.retainedInputsForStates(states)
	}
	if err := b.skillManager.ReplaceMCPCatalogInputsAtGeneration(generation, inputs); err != nil {
		return err
	}
	b.epoch++
	return discoverErr
}

// prepare stages all fallible MCP reads before a workspace publication. A
// cross-workspace stage excludes project/local connections from the old root
// and any persistent server shadowed by a target workspace config.
func (b *mcpSkillRuntimeBridge) prepare(ctx context.Context, targetConfigs map[string]svcmcp.MCPServerConfig, sameWorkspace bool) *preparedMCPSkillProjection {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	states := b.mcpManager.Snapshot()
	eligible := eligibleMCPStatesForTarget(states, targetConfigs, sameWorkspace)
	inputs, err := skills.DiscoverMCPCatalogInputsFromConnections(ctx, eligible)
	if err != nil {
		inputs = b.retainedInputsForStates(eligible)
	}
	return &preparedMCPSkillProjection{
		inputs: cloneMCPCatalogInputs(inputs), targetConfigs: cloneMCPServerConfigs(targetConfigs),
		sameWorkspace: sameWorkspace, epoch: b.epoch,
	}
}

// withPrepared serializes lifecycle refresh across the Manager's atomic plan
// commit and the subsequent ServiceMCP retarget. The project directories and
// staged MCP projection are atomic inside the skill Manager; this bridge does
// not claim that unrelated direct ServiceMCP callers share that transaction.
func (b *mcpSkillRuntimeBridge) withPrepared(plan *preparedMCPSkillProjection, projectPlan *skills.ProjectSourcePlan, transition func() error) error {
	if b == nil {
		return transition()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return transition()
	}
	if plan != nil && projectPlan != nil && plan.epoch != b.epoch {
		states := eligibleMCPStatesForTarget(b.mcpManager.Snapshot(), plan.targetConfigs, plan.sameWorkspace)
		latest := b.retainedInputsForStates(states)
		if err := b.skillManager.StageMCPCatalogInputs(projectPlan, latest); err != nil {
			return err
		}
		plan.inputs = cloneMCPCatalogInputs(latest)
		plan.epoch = b.epoch
	}
	if err := transition(); err != nil {
		return err
	}
	b.epoch++
	return nil
}

func (b *mcpSkillRuntimeBridge) retainedInputsForStates(states []svcmcp.MCPServerConnection) []skills.MCPCatalogInput {
	connected := make(map[string]struct{})
	for _, state := range states {
		if state.Type == svcmcp.MCPStateConnected && state.Client != nil {
			connected[state.Name] = struct{}{}
		}
	}
	current := b.skillManager.MCPCatalogInputs()
	retained := make([]skills.MCPCatalogInput, 0, len(current))
	for _, input := range current {
		server, err := skills.MCPServerNameForCatalogInput(input)
		if err != nil {
			continue
		}
		if _, ok := connected[server]; ok {
			retained = append(retained, input.Clone())
		}
	}
	return retained
}

func eligibleMCPStatesForTarget(states []svcmcp.MCPServerConnection, targetConfigs map[string]svcmcp.MCPServerConfig, sameWorkspace bool) []svcmcp.MCPServerConnection {
	eligible := make([]svcmcp.MCPServerConnection, 0, len(states))
	for _, state := range states {
		target, targetOwnsName := targetConfigs[state.Name]
		workspaceOwned := state.Config.Scope == svcmcp.ScopeProject || state.Config.Scope == svcmcp.ScopeLocal
		if workspaceOwned {
			// A workspace-owned connection is reusable only for a same-root
			// refresh whose target settings still describe the exact same
			// authority. A removed or edited config must not remain visible
			// merely because the filesystem root did not change.
			if !sameWorkspace || !targetOwnsName || !sameMCPConfigAuthority(state.Name, state.Config, target) {
				continue
			}
			eligible = append(eligible, state)
			continue
		}
		// Project/local target configs shadow persistent user/managed configs
		// with the same server name. Preserve the persistent config in the
		// service manager for later restoration, but never project its skills
		// into the target workspace.
		if targetOwnsName {
			continue
		}
		eligible = append(eligible, state)
	}
	return eligible
}

func sameMCPConfigAuthority(name string, left, right svcmcp.MCPServerConfig) bool {
	left = normalizeMCPConfigForAuthority(name, left)
	right = normalizeMCPConfigForAuthority(name, right)
	return left.Scope == right.Scope && svcmcp.HashMCPConfig(left) == svcmcp.HashMCPConfig(right)
}

func normalizeMCPConfigForAuthority(name string, config svcmcp.MCPServerConfig) svcmcp.MCPServerConfig {
	if config.Type == "" {
		config.Type = svcmcp.TransportStdio
	}
	if config.Type == svcmcp.TransportStdio && config.Args == nil {
		config.Args = []string{}
	}
	if strings.TrimSpace(config.Name) == "" {
		config.Name = strings.TrimSpace(name)
	}
	return config
}

func cloneMCPCatalogInputs(inputs []skills.MCPCatalogInput) []skills.MCPCatalogInput {
	out := make([]skills.MCPCatalogInput, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, input.Clone())
	}
	return out
}

func cloneMCPServerConfigs(configs map[string]svcmcp.MCPServerConfig) map[string]svcmcp.MCPServerConfig {
	out := make(map[string]svcmcp.MCPServerConfig, len(configs))
	for name, config := range configs {
		out[name] = config
	}
	return out
}

func sameWorkspaceRoot(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}
