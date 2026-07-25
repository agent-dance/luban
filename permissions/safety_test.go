package permissions

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestSafetyCheck(t *testing.T) {
	SetSafetyConfig(SafetyConfig{
		ShellPolicyAnalyzer: func(command string, _ types.PolicyContext) types.PolicyDecision {
			if command == "rm -rf /" || command == "echo x > .git/HEAD" {
				return types.PolicyDecision{
					Disposition: types.PolicyBlock,
					Code:        "test.block",
					PublicKey:   i18n.KeyPermissionSafetyProtectedPath,
					PublicArgs:  []any{command},
				}
			}
			return types.PolicyDecision{Disposition: types.PolicyAllow}
		},
	})
	defer SetSafetyConfig(SafetyConfig{}) // cleanup

	tests := []struct {
		name     string
		tool     string
		input    map[string]any
		wantDec  Decision
		wantDeny bool // true if we expect DecisionDeny
	}{
		{
			name:     "Write .git/HEAD → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".git/HEAD"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .env → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".env"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .ssh/id_rsa → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".ssh/id_rsa"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Read .git/HEAD → Allow",
			tool:     "Read",
			input:    map[string]any{"file_path": ".git/HEAD"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Write src/main.go → Allow",
			tool:     "Write",
			input:    map[string]any{"file_path": "src/main.go"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Edit .bashrc → Deny",
			tool:     "Edit",
			input:    map[string]any{"file_path": ".bashrc"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Edit .zshrc → Deny",
			tool:     "Edit",
			input:    map[string]any{"file_path": "/home/user/.zshrc"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .luban-code/settings.json → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".luban-code/settings.json"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .deepseek-code/settings.json → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".deepseek-code/settings.json"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .claude/settings.json → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".claude/settings.json"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .kube/config → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".kube/config"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Bash echo x > .git/HEAD → Deny (via BashProtectedPathChecker)",
			tool:     "Bash",
			input:    map[string]any{"command": "echo x > .git/HEAD"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Bash cat .git/HEAD → Allow (read operation)",
			tool:     "Bash",
			input:    map[string]any{"command": "cat .git/HEAD"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Bash rm -rf / → Deny (via DangerousCommandChecker)",
			tool:     "Bash",
			input:    map[string]any{"command": "rm -rf /"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Bash ls -la → Allow",
			tool:     "Bash",
			input:    map[string]any{"command": "ls -la"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Write with absolute path /home/user/.env → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": "/home/user/.env"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write nested .git path → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": "/project/.git/objects/pack"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .env.local → Deny (exact match)",
			tool:     "Write",
			input:    map[string]any{"file_path": ".env.local"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .env.production → Deny (exact match)",
			tool:     "Write",
			input:    map[string]any{"file_path": "config/.env.production"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Write .envrc → Allow (not in protected list)",
			tool:     "Write",
			input:    map[string]any{"file_path": ".envrc"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Write .env.example → Allow (not in protected list)",
			tool:     "Write",
			input:    map[string]any{"file_path": ".env.example"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "NotebookEdit .git/notebook.ipynb → Deny",
			tool:     "NotebookEdit",
			input:    map[string]any{"notebook_path": ".git/notebook.ipynb"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "NotebookEdit safe path → Allow",
			tool:     "NotebookEdit",
			input:    map[string]any{"notebook_path": "notebooks/analysis.ipynb"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Write .bash_profile → Deny",
			tool:     "Write",
			input:    map[string]any{"file_path": ".bash_profile"},
			wantDec:  DecisionDeny,
			wantDeny: true,
		},
		{
			name:     "Unknown tool → Allow (passthrough)",
			tool:     "CustomTool",
			input:    map[string]any{"file_path": ".git/HEAD"},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Bash with empty command → Allow",
			tool:     "Bash",
			input:    map[string]any{"command": ""},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
		{
			name:     "Write with no file_path → Allow",
			tool:     "Write",
			input:    map[string]any{},
			wantDec:  DecisionAllow,
			wantDeny: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec, reason := SafetyCheck(tt.tool, tt.input)
			if dec != tt.wantDec {
				t.Errorf("SafetyCheck(%q, %v) decision = %v, want %v (reason: %q)", tt.tool, tt.input, dec, tt.wantDec, reason)
			}
			if tt.wantDeny && reason == "" {
				t.Errorf("SafetyCheck(%q, %v) expected non-empty reason for deny", tt.tool, tt.input)
			}
			if !tt.wantDeny && reason != "" {
				t.Errorf("SafetyCheck(%q, %v) expected empty reason for allow, got %q", tt.tool, tt.input, reason)
			}
		})
	}
}

func TestIsProtectedPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".git/HEAD", true},
		{".git/config", true},
		{".git", true},
		{"/project/.git/objects", true},
		{".env", true},
		{"/home/user/.env", true},
		{"src/.env", true},
		{".bashrc", true},
		{".zshrc", true},
		{".profile", true},
		{".bash_profile", true},
		{".ssh/id_rsa", true},
		{".ssh/authorized_keys", true},
		{".gnupg/pubring.kbx", true},
		{".aws/credentials", true},
		{".aws/config", true},
		{".kube/config", true},
		{".deepseek-code/settings.json", true},
		{".luban-code/settings.json", true},
		{".claude/settings.json", true},

		// Exact enumerated .env variants
		{".env.local", true},
		{".env.production", true},
		{".env.staging", true},
		{".env.development", true},
		{".env.test", true},
		{"config/.env.local", true},
		{"config/.env.production", true},

		// Should NOT match — not in the exact list
		{".envrc", false},       // direnv config, not protected
		{".env.example", false}, // template file, not protected
		{".env.sample", false},  // template file, not protected
		{"src/main.go", false},
		{"README.md", false},
		{".gitignore", false},     // not ".git/" prefix
		{"git/config", false},     // no leading dot
		{"my.env.bak", false},     // basename "my.env.bak" does NOT start with ".env"
		{".kube/contexts", false}, // only .kube/config is protected
		{"src/.gitkeep", false},   // not ".git/" prefix
		{"env", false},            // no leading dot
		{"environment.ts", false}, // doesn't start with ".env"
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("IsProtectedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSafetyCheckWithNoConfig(t *testing.T) {
	// Clear safety config to test fail-closed behavior without injected dependencies.
	SetSafetyConfig(SafetyConfig{})
	defer SetSafetyConfig(SafetyConfig{})

	// File-based checks should still work without config.
	dec, _ := SafetyCheck("Write", map[string]any{"file_path": ".git/HEAD"})
	if dec != DecisionDeny {
		t.Error("expected Deny for Write .git/HEAD even without SafetyConfig")
	}

	// Fail-closed: Bash commands should be denied when checkers are not injected.
	dec, reason := SafetyCheck("Bash", map[string]any{"command": "rm -rf /"})
	if dec != DecisionDeny {
		t.Error("expected Deny for Bash command when no DangerousCommandChecker is configured (fail-closed)")
	}
	if reason == "" {
		t.Error("expected non-empty reason for Bash deny in fail-closed mode")
	}

	// Fail-closed: even safe Bash commands should be denied without config.
	dec, _ = SafetyCheck("Bash", map[string]any{"command": "ls -la"})
	if dec != DecisionDeny {
		t.Error("expected Deny for safe Bash command when no checkers configured (fail-closed)")
	}

	// Empty Bash command should still be allowed.
	dec, _ = SafetyCheck("Bash", map[string]any{"command": ""})
	if dec != DecisionAllow {
		t.Error("expected Allow for empty Bash command")
	}

	// Non-Bash tools should be unaffected by missing config.
	dec, _ = SafetyCheck("Write", map[string]any{"file_path": "src/main.go"})
	if dec != DecisionAllow {
		t.Error("expected Allow for Write to safe path without config")
	}
}

// ── C2: Path traversal tests ────────────────────────────────────────────────

func TestIsProtectedPathTraversal(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		// Path traversal variants that should still match
		{"traversal to .git/HEAD", "foo/../.git/HEAD", true},
		{"deep traversal to .env", "a/b/c/../../../.env", true},
		{"traversal with .ssh", "safe/../../.ssh/id_rsa", true},
		{"dot-slash .git", "./.git/config", true},
		{"double-slash .git", ".git//HEAD", true},

		// These should NOT be protected
		{"safe traversal", "foo/../bar/main.go", false},
		{"safe relative", "./src/main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProtectedPath(tt.path)
			if got != tt.want {
				t.Errorf("IsProtectedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// ── C3: GetProtectedPaths returns a copy ────────────────────────────────────

func TestGetProtectedPathsReturnsCopy(t *testing.T) {
	pp := GetProtectedPaths()
	if len(pp) == 0 {
		t.Fatal("GetProtectedPaths() returned empty list")
	}

	// Mutate the returned slice — should not affect the original.
	original := pp[0]
	pp[0] = "MUTATED"

	pp2 := GetProtectedPaths()
	if pp2[0] != original {
		t.Errorf("GetProtectedPaths() returned mutable reference: got %q after mutation, want %q", pp2[0], original)
	}
}
