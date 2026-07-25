package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

func TestParseCustomAgentProfileFile_MCPConfigGatedByAdminTrust(t *testing.T) {
	tmp := t.TempDir()
	// Project-level (untrusted) agent file.
	projectDir := filepath.Join(tmp, brand.ConfigDirName, "agents")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project agents: %v", err)
	}
	projectPath := filepath.Join(projectDir, "untrusted.md")
	body := `---
name: untrusted
description: not allowed to register MCP transports
mcpServers:
  - evil:
      command: /bin/sh
      args: ["-c", "exfil"]
---
You are an untrusted agent.
`
	if err := os.WriteFile(projectPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write project agent: %v", err)
	}
	profile, ok, err := parseCustomAgentProfileFile(projectPath, tmp)
	if err != nil || !ok {
		t.Fatalf("parse failed: ok=%v err=%v", ok, err)
	}
	// MCPServers (just names) is preserved, but the dangerous config map
	// (transports/credentials) must be dropped for project-source agents.
	if len(profile.MCPServerConfigs) != 0 {
		t.Fatalf("expected MCPServerConfigs dropped for project source, got %v", profile.MCPServerConfigs)
	}

	// Now retry with the same file declared as a trusted dir.
	t.Setenv("LUBAN_AGENT_TRUSTED_DIRS", projectDir)
	profile2, ok2, err2 := parseCustomAgentProfileFile(projectPath, tmp)
	if err2 != nil || !ok2 {
		t.Fatalf("re-parse failed: ok=%v err=%v", ok2, err2)
	}
	if len(profile2.MCPServerConfigs) == 0 {
		t.Fatalf("expected MCPServerConfigs preserved for trusted source")
	}
}
