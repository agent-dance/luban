package tools

// agent_admin_trust.go implements the admin-trust gate that the TS
// AgentTool/runAgent.ts consults before letting an agent's frontmatter `hooks`
// block register process-level hook handlers.
//
// The TS implementation only registers hooks when the agent file is admin-
// trusted (signed plugin or enterprise-managed install). Without that gate,
// any user-authored .md file dropped into ~/.claude/agents could declare a
// hook that runs arbitrary commands inside the host CLI — direct privilege
// escalation. With the gate, admin/plugin-managed agents keep their hook
// surface while user-authored agents have hooks silently dropped.
//
// We classify a file as admin-trusted when its on-disk path matches a known
// trusted root: the enterprise managed-agents directory, plugin-shipped
// agents, or paths explicitly whitelisted via the env var
// CLAUDE_AGENT_TRUSTED_DIRS (path-list separator-delimited). Project-level
// (.claude/agents in the workspace) and user-level (~/.claude/agents) are
// considered untrusted and have hooks stripped.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// agentSourceTrust labels where the agent definition came from for the
// purpose of the admin-trust gate.
type agentSourceTrust string

const (
	agentSourceTrustUnknown agentSourceTrust = ""
	agentSourceTrustBuiltin agentSourceTrust = "builtin"
	agentSourceTrustManaged agentSourceTrust = "managed"
	agentSourceTrustPlugin  agentSourceTrust = "plugin"
	agentSourceTrustProject agentSourceTrust = "project"
	agentSourceTrustUser    agentSourceTrust = "user"
)

// classifyAgentSource inspects the on-disk path of a custom agent definition
// and returns its trust class. Path may be empty for built-in agents.
func classifyAgentSource(path string) agentSourceTrust {
	if path == "" {
		return agentSourceTrustBuiltin
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	for _, dir := range claudeManagedAgentDirs() {
		if dir == "" {
			continue
		}
		clean, err := filepath.Abs(dir)
		if err != nil {
			clean = dir
		}
		if pathHasPrefix(abs, filepath.Clean(clean)) {
			return agentSourceTrustManaged
		}
	}
	for _, dir := range trustedAgentDirsFromEnv() {
		if pathHasPrefix(abs, dir) {
			return agentSourceTrustPlugin
		}
	}
	if home, err := userHomeDirForAgentProfiles(); err == nil && home != "" {
		userDir := filepath.Join(home, ".claude", "agents")
		if pathHasPrefix(abs, userDir) {
			return agentSourceTrustUser
		}
	}
	return agentSourceTrustProject
}

// isAgentSourceAdminTrusted reports whether a source classification is
// permitted to register frontmatter hooks. Built-in/managed/plugin sources
// are trusted; project- and user-authored agents are not.
func isAgentSourceAdminTrusted(trust agentSourceTrust) bool {
	switch trust {
	case agentSourceTrustBuiltin, agentSourceTrustManaged, agentSourceTrustPlugin:
		return true
	}
	return false
}

// isAgentAdminTrustGateEnabled is the policy switch for the admin-trust
// gate. When false (default for backwards compatibility) the gate is a
// no-op — all sources are trusted, mirroring legacy Go behaviour. When
// true, only admin-trusted sources may register hooks/MCP transports.
//
// The gate is enabled by setting CLAUDE_AGENT_ADMIN_TRUST_GATE=1 (or
// =true). Production deployments that ship plugin-managed agents enable
// this; test fixtures that load project-level agents leave it off.
func isAgentAdminTrustGateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_AGENT_ADMIN_TRUST_GATE"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// isAgentSourceAdminTrustedOrGateOff returns true when either the gate is
// disabled or the source is genuinely admin-trusted. Use this in code
// paths that want to emulate the TS gating without breaking legacy
// callers.
func isAgentSourceAdminTrustedOrGateOff(trust agentSourceTrust) bool {
	if !isAgentAdminTrustGateEnabled() {
		return true
	}
	return isAgentSourceAdminTrusted(trust)
}

// trustedAgentDirsFromEnv returns directories explicitly whitelisted via the
// CLAUDE_AGENT_TRUSTED_DIRS env var. The variable is a path-list:
// colon-separated on POSIX, semicolon-separated on Windows.
func trustedAgentDirsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("CLAUDE_AGENT_TRUSTED_DIRS"))
	if raw == "" {
		return nil
	}
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	parts := strings.Split(raw, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		out = append(out, filepath.Clean(abs))
	}
	return out
}

// pathHasPrefix reports whether `abs` lives inside `prefix`. Both arguments
// must already be absolute and cleaned. Comparison is case-insensitive on
// Windows to match the OS path semantics.
func pathHasPrefix(abs, prefix string) bool {
	if prefix == "" {
		return false
	}
	a, p := abs, prefix
	if runtime.GOOS == "windows" {
		a = strings.ToLower(a)
		p = strings.ToLower(p)
	}
	if !strings.HasPrefix(a, p) {
		return false
	}
	if len(a) == len(p) {
		return true
	}
	c := a[len(p)]
	return c == filepath.Separator || c == '/' || c == '\\'
}
