package tools

// agent_isolation.go implements the PrepareIsolation API described by
// tasks/agent.json subtask agent-05. It is a thin facade over the existing
// worktree helpers (createAgentWorktree, cleanupAgentWorktreeIfClean) plus a
// pluggable RemoteRuntimeProvider for the isolation="remote" mode.

import (
	"context"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// AgentIsolationMode names a supported isolation strategy.
type AgentIsolationMode string

const (
	AgentIsolationNone     AgentIsolationMode = "none"
	AgentIsolationWorktree AgentIsolationMode = "worktree"
	AgentIsolationRemote   AgentIsolationMode = "remote"
)

// NormalizeIsolationMode coerces a raw user/profile string into a known mode.
// An empty string maps to AgentIsolationNone.
func NormalizeIsolationMode(raw string) AgentIsolationMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return AgentIsolationNone
	case "worktree":
		return AgentIsolationWorktree
	case "remote":
		return AgentIsolationRemote
	default:
		return AgentIsolationMode(strings.ToLower(strings.TrimSpace(raw)))
	}
}

// AgentIsolationResult captures the side-effects of preparing isolation: a CWD
// override for the child runtime, a metadata snapshot to persist on the
// session, and a cleanup function to call when the run is finished.
type AgentIsolationResult struct {
	Mode      AgentIsolationMode
	ChildCWD  string
	Metadata  agentSessionMetadata
	Cleanup   func()
	Worktree  *agentWorktree
	RemoteRef RemoteAgentLaunch // populated for AgentIsolationRemote
}

// PrepareIsolation prepares the environment for a sub-agent according to the
// requested mode. It returns an AgentIsolationResult whose Cleanup MUST be
// invoked on completion (idempotent / no-op for AgentIsolationNone). When
// mode == AgentIsolationRemote the call delegates to the configured
// RemoteRuntimeProvider; if no provider is configured it returns a clear
// error so callers can fall back to a local mode or surface the failure.
func PrepareIsolation(
	ctx context.Context,
	mode AgentIsolationMode,
	agentID string,
	parentCWD string,
	permissionSnapshot types.ToolRuntimeContext,
	provider RemoteRuntimeProvider,
) (AgentIsolationResult, error) {
	noop := func() {}
	trustedCWD := firstNonEmpty(parentCWD, permissionSnapshot.ProjectRoot)
	if strings.TrimSpace(trustedCWD) != "" {
		validated, err := validateAgentCWD(trustedCWD, permissionSnapshot)
		if err != nil {
			return AgentIsolationResult{}, err
		}
		trustedCWD = validated
	}
	switch NormalizeIsolationMode(string(mode)) {
	case AgentIsolationNone:
		cwd := strings.TrimSpace(trustedCWD)
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return AgentIsolationResult{
			Mode:     AgentIsolationNone,
			ChildCWD: cwd,
			Metadata: agentSessionMetadata{Isolation: ""},
			Cleanup:  noop,
		}, nil
	case AgentIsolationWorktree:
		wt, err := createAgentWorktree(agentID, firstNonEmpty(permissionSnapshot.ProjectRoot, trustedCWD))
		if err != nil {
			return AgentIsolationResult{}, err
		}
		md := agentSessionMetadata{
			Isolation:          string(AgentIsolationWorktree),
			WorktreeRepoRoot:   wt.RepoRoot,
			WorktreePath:       wt.Path,
			WorktreeBranch:     wt.Branch,
			WorktreeHeadCommit: wt.HeadCommit,
		}
		cleanup := func() {
			if md.WorktreePath == "" {
				return
			}
			_, _ = cleanupAgentWorktreeIfClean(md)
		}
		return AgentIsolationResult{
			Mode:     AgentIsolationWorktree,
			ChildCWD: wt.Path,
			Metadata: md,
			Cleanup:  cleanup,
			Worktree: wt,
		}, nil
	case AgentIsolationRemote:
		if provider == nil {
			return AgentIsolationResult{}, i18n.NewError(i18n.KeyToolAgentDeepRemoteRuntimeRequired)
		}
		if err := requireRemotePermissionSnapshotEnforcement(provider); err != nil {
			return AgentIsolationResult{}, err
		}
		if err := requireRemoteFailClosedPromptEnforcement(provider); err != nil {
			return AgentIsolationResult{}, err
		}
		remoteSnapshot := (agentRuntimeContextProvider{snapshot: permissionSnapshot, agentID: agentID, cwd: trustedCWD}).ToolRuntimeContext()
		launch, err := provider.Spawn(ctx, RemoteAgentSpawnRequest{
			AgentID:            agentID,
			ParentCWD:          trustedCWD,
			PermissionSnapshot: remoteSnapshot,
			AvoidPrompts:       true,
		})
		if err != nil {
			return AgentIsolationResult{}, err
		}
		md := agentSessionMetadata{
			Isolation: string(AgentIsolationRemote),
		}
		cleanup := func() {
			if provider != nil && launch.TaskID != "" {
				_ = provider.Cleanup(launch.TaskID)
			}
		}
		return AgentIsolationResult{
			Mode:      AgentIsolationRemote,
			ChildCWD:  trustedCWD,
			Metadata:  md,
			Cleanup:   cleanup,
			RemoteRef: launch,
		}, nil
	default:
		return AgentIsolationResult{}, i18n.NewError(i18n.KeyToolAgentDeepIsolationUnsupported, mode)
	}
}
