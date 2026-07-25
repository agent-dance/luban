package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestBashPermissionApprovalReason(t *testing.T) {
	tmp := t.TempDir()

	tests := []struct {
		name      string
		command   string
		want      bool
		reasonKey i18n.Key
		prepare   func(t *testing.T)
	}{
		{
			name:      "multiple cd commands ask",
			command:   "cd one && cd two && pwd",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashMultipleDirectories,
		},
		{
			name:      "cd and git ask",
			command:   "cd repo && git status",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashCDAndGit,
		},
		{
			name:      "cd and redirect ask",
			command:   "cd .luban-code && echo x > settings.json",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashCDAndRedirect,
		},
		{
			name:      "process substitution asks",
			command:   "echo hi > >(tee output.txt)",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashProcessSubstitution,
		},
		{
			name:      "bare repo git asks",
			command:   "git status",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashBareGit,
			prepare: func(t *testing.T) {
				mustMkdirAll(t, "objects")
			},
		},
		{
			name:      "git internal writes ask",
			command:   "mkdir -p hooks && echo '#!/bin/sh' > hooks/pre-commit && git status",
			want:      true,
			reasonKey: i18n.KeyToolPermissionBashGitInternal,
		},
		{
			name:    "plain git status outside bare repo is allowed",
			command: "git status",
			want:    false,
		},
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origWD)
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tmp, strings.ReplaceAll(tt.name, " ", "_"))
			mustMkdirAll(t, testDir)
			if err := os.Chdir(testDir); err != nil {
				t.Fatalf("chdir: %v", err)
			}
			if tt.prepare != nil {
				tt.prepare(t)
			}

			got, reasonKey := bashPermissionApprovalReason(tt.command)
			if got != tt.want {
				t.Fatalf("bashPermissionApprovalReason(%q) = %v, want %v (reason key: %q)", tt.command, got, tt.want, reasonKey)
			}
			if reasonKey != tt.reasonKey {
				t.Fatalf("reason key = %q, want %q", reasonKey, tt.reasonKey)
			}
		})
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
