package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

func TestClassifyAgentSource_BuiltinForEmpty(t *testing.T) {
	if got := classifyAgentSource(""); got != agentSourceTrustBuiltin {
		t.Fatalf("empty path should be builtin, got %q", got)
	}
}

func TestClassifyAgentSource_ProjectByDefault(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, brand.ConfigDirName, "agents", "foo.md")
	if got := classifyAgentSource(p); got != agentSourceTrustProject {
		t.Fatalf("project agent path should be project, got %q", got)
	}
	if isAgentSourceAdminTrusted(classifyAgentSource(p)) {
		t.Fatalf("project source must not be admin-trusted")
	}
}

func TestClassifyAgentSource_TrustedDirsEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LUBAN_AGENT_TRUSTED_DIRS", tmp)
	p := filepath.Join(tmp, "agents", "secure.md")
	if got := classifyAgentSource(p); got != agentSourceTrustPlugin {
		t.Fatalf("trusted-dir agent should be plugin, got %q", got)
	}
	if !isAgentSourceAdminTrusted(classifyAgentSource(p)) {
		t.Fatalf("plugin source must be admin-trusted")
	}
}

func TestClassifyAgentSource_UserHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no user home dir")
	}
	p := filepath.Join(home, brand.ConfigDirName, "agents", "u.md")
	if got := classifyAgentSource(p); got != agentSourceTrustUser {
		t.Fatalf("user-home agent should be user, got %q", got)
	}
	if isAgentSourceAdminTrusted(classifyAgentSource(p)) {
		t.Fatalf("user source must not be admin-trusted")
	}
}
