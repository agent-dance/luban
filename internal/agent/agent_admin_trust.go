package agent

// agent_admin_trust.go implements the admin-trust gate that the TS
// AgentTool/runAgent.ts consults before letting an agent's frontmatter `hooks`
// block register process-level hook handlers.
//
// The TS implementation only registers hooks when the agent file is admin-
// trusted (signed plugin or enterprise-managed install). Without that gate,
// any user-authored .md file dropped into ~/.luban-code/agents could declare a
// hook that runs arbitrary commands inside the host CLI — direct privilege
// escalation. With the gate, admin/plugin-managed agents keep their hook
// surface while user-authored agents have hooks silently dropped.
//
// We classify a file as admin-trusted when its on-disk path matches a known
// trusted root: the enterprise managed-agents directory, plugin-shipped
// agents, or paths explicitly whitelisted via the env var
// LUBAN_AGENT_TRUSTED_DIRS (path-list separator-delimited). Project-level
// (.luban-code/agents in the workspace) and user-level
// (~/.luban-code/agents) are
// considered untrusted and have hooks stripped.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/brand"
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
	for _, dir := range managedAgentDirs() {
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
		userDir := filepath.Join(home, brand.ConfigDirName, "agents")
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

// trustedAgentDirsFromEnv returns directories explicitly whitelisted via the
// LUBAN_AGENT_TRUSTED_DIRS env var. The variable is a path-list:
// colon-separated on POSIX, semicolon-separated on Windows.
func trustedAgentDirsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("LUBAN_AGENT_TRUSTED_DIRS"))
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
