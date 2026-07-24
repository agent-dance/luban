// Package tools — agent_feature_gates.go provides defaults for the
// builtin-set feature gates referenced by agent.go. Mirrors the TS gates
// in src/tools/AgentTool/builtInAgents.ts:
//
//   - areBuiltinExplorePlanAgentsEnabled: BUILTIN_EXPLORE_PLAN_AGENTS feature
//     and tengu_amber_stoat A/B. In Go, env BUILTIN_EXPLORE_PLAN_AGENTS or
//     CLAUDE_CODE_AMBER_STOAT can flip the flag, and the legacy
//     CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS still wins.
//   - isVerificationAgentEnabled: VERIFICATION_AGENT feature and
//     tengu_hive_evidence growthbook. Honour VERIFICATION_AGENT,
//     CLAUDE_CODE_HIVE_EVIDENCE, and the legacy
//     CLAUDE_CODE_DISABLE_VERIFICATION_AGENT.
//
// All gates default to ON when the env is unset, matching pre-feature-gate
// Go behaviour.
package tools

import (
	"os"
	"strings"
)

// areBuiltinExplorePlanAgentsEnabled returns whether the Explore/Plan
// builtin agents should be registered.
func areBuiltinExplorePlanAgentsEnabled() bool {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS")); v != "" {
		if isTruthyAgentEnv(v) {
			return false
		}
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_BUILTIN_EXPLORE_PLAN_AGENTS")); v != "" {
		return isTruthyAgentEnv(v)
	}
	if v := strings.TrimSpace(os.Getenv("BUILTIN_EXPLORE_PLAN_AGENTS")); v != "" {
		return isTruthyAgentEnv(v)
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_AMBER_STOAT")); v != "" {
		return isTruthyAgentEnv(v)
	}
	return true
}

// filterBuiltinAgentProfilesByFeatureGates removes experimental built-ins
// from the list when their corresponding feature flag/env is unset.
// Defaults to "on" so existing test fixtures keep working; operators
// opt out via CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS / VERIFICATION_AGENT=0.
func filterBuiltinAgentProfilesByFeatureGates(profiles []agentProfile) []agentProfile {
	if len(profiles) == 0 {
		return profiles
	}
	exploreEnabled := areBuiltinExplorePlanAgentsEnabled()
	verifyEnabled := isVerificationAgentEnabled()
	guideEnabled := agentCodeGuideEnabledForEntrypoint()
	out := make([]agentProfile, 0, len(profiles))
	for _, p := range profiles {
		switch strings.ToLower(strings.TrimSpace(p.Name)) {
		case "explore", "plan":
			if !exploreEnabled {
				continue
			}
		case "verification":
			if !verifyEnabled {
				continue
			}
		case "claude-code-guide":
			if !guideEnabled {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// isVerificationAgentEnabled returns whether the verification agent
// should be registered.
func isVerificationAgentEnabled() bool {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_VERIFICATION_AGENT")); v != "" {
		if isTruthyAgentEnv(v) {
			return false
		}
	}
	if v := strings.TrimSpace(os.Getenv("VERIFICATION_AGENT")); v != "" {
		if !isTruthyAgentEnv(v) {
			return false
		}
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_HIVE_EVIDENCE")); v != "" {
		if !isTruthyAgentEnv(v) {
			return false
		}
	}
	return true
}
