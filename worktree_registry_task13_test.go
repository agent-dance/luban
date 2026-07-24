package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/tools"
)

func task13RegistryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestTask13RegistryWorktreeRetargetsSessionTools(t *testing.T) {
	repo := t.TempDir()
	task13RegistryGit(t, repo, "init", "-b", "main")
	task13RegistryGit(t, repo, "config", "user.email", "task13@example.invalid")
	task13RegistryGit(t, repo, "config", "user.name", "Task 13")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("registry worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task13RegistryGit(t, repo, "add", "README.md")
	task13RegistryGit(t, repo, "commit", "-m", "fixture")
	t.Setenv("CLAUDE_SESSION_ID", "registry-task13")
	processCWD, _ := os.Getwd()

	deps := SetupRegistry(nil, repo, []string{repo}, nil, nil)
	if deps.EnterWorktreeTool == nil || deps.ExitWorktreeTool == nil || deps.WorktreeManager == nil || deps.WorktreeRuntime == nil {
		t.Fatal("registry did not expose fully-wired worktree dependencies")
	}
	result := deps.Registry.ExecuteTool(context.Background(), "EnterWorktree", map[string]any{"name": "registry", "base_ref": "head"})
	if result.IsError {
		t.Fatalf("EnterWorktree through registry failed: %#v", result)
	}
	data, ok := result.Data.(tools.EnterWorktreeOutput)
	if !ok {
		t.Fatalf("registry result data = %T, want EnterWorktreeOutput", result.Data)
	}
	want := data.WorktreePath
	if deps.WorktreeRuntime.CurrentCWD() != want || deps.RuntimeScope.ProjectRoot() != want || deps.BashTool.CWD != want || deps.PowerShellTool.CWD != want {
		t.Fatalf("session tools not retargeted: runtime=%q scope=%q bash=%q powershell=%q want=%q", deps.WorktreeRuntime.CurrentCWD(), deps.RuntimeScope.ProjectRoot(), deps.BashTool.CWD, deps.PowerShellTool.CWD, want)
	}
	if cwd, _ := os.Getwd(); cwd != processCWD {
		t.Fatalf("registry EnterWorktree changed process cwd: got %q want %q", cwd, processCWD)
	}

	kept := deps.Registry.ExecuteTool(context.Background(), "ExitWorktree", map[string]any{"action": "keep"})
	if kept.IsError {
		t.Fatalf("ExitWorktree keep through registry failed: %#v", kept)
	}
	wantRepo := repo
	if resolved, err := filepath.EvalSymlinks(repo); err == nil {
		wantRepo = resolved
	}
	if deps.WorktreeRuntime.CurrentCWD() != wantRepo || deps.RuntimeScope.ProjectRoot() != wantRepo || deps.BashTool.CWD != wantRepo || deps.PowerShellTool.CWD != wantRepo {
		t.Fatalf("session tools not restored: runtime=%q scope=%q bash=%q powershell=%q want=%q", deps.WorktreeRuntime.CurrentCWD(), deps.RuntimeScope.ProjectRoot(), deps.BashTool.CWD, deps.PowerShellTool.CWD, wantRepo)
	}

	metadata := deps.Registry.ToolMetadata("EnterWorktree", map[string]any{"name": "x"})
	if !metadata.Write || metadata.Destructive {
		t.Fatalf("EnterWorktree metadata = %#v, want write/non-destructive", metadata)
	}
	discovery := registry.DiscoveryMetadata(deps.EnterWorktreeTool)
	if !discovery.ShouldDefer || discovery.SearchHint != "create an isolated git worktree and switch into it" {
		t.Fatalf("EnterWorktree discovery metadata = %#v", discovery)
	}

	_, _ = task13RegistryGitCleanup(repo, want, data.WorktreeBranch)
}

func task13RegistryGitCleanup(repo, path, branch string) (string, error) {
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if branch != "" {
		deleteCmd := exec.Command("git", "branch", "-D", branch)
		deleteCmd.Dir = repo
		_, _ = deleteCmd.CombinedOutput()
	}
	return string(out), err
}

func TestTask13RegistryLoadsAndRunsConfiguredWorktreeHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	hookPath := filepath.Join(t.TempDir(), "registry-hook-worktree")
	if err := os.MkdirAll(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "registry-hooks.log")
	script := filepath.Join(t.TempDir(), "registry-hook.sh")
	body := "#!/bin/sh\ninput=$(cat)\nprintf '%s\\n' \"$input\" >> \"$1\"\ncase \"$input\" in\n  *WorktreeCreate*) printf '{\"path\":\"%s\",\"branch\":\"hook-registry\"}\\n' \"$2\" ;;\n  *) printf '{}\\n' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	command := strings.Join([]string{shellQuoteTask13(script), shellQuoteTask13(logPath), shellQuoteTask13(hookPath)}, " ")
	settings := map[string]any{
		"hooks": map[string]any{
			"WorktreeCreate": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}},
			},
			"WorktreeRemove": []any{
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}},
			},
		},
	}
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	settingsDir := filepath.Join(root, ".deepseek-code")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), settingsData, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_SESSION_ID", "registry-hook-task13")
	deps := SetupRegistry(nil, root, []string{root}, nil, nil)
	if _, ok := deps.EnterWorktreeTool.HookBridge.Lookup("WorktreeCreate"); !ok {
		t.Fatal("registry did not inject configured WorktreeCreate hook")
	}
	created := deps.Registry.ExecuteTool(context.Background(), "EnterWorktree", map[string]any{"name": "hooked"})
	if created.IsError {
		t.Fatalf("configured registry hook create failed: %#v", created)
	}
	wantHookPath := hookPath
	if resolved, err := filepath.EvalSymlinks(hookPath); err == nil {
		wantHookPath = resolved
	}
	if deps.WorktreeRuntime.CurrentCWD() != wantHookPath {
		t.Fatalf("hook registry cwd = %q, want %q", deps.WorktreeRuntime.CurrentCWD(), hookPath)
	}
	removed := deps.Registry.ExecuteTool(context.Background(), "ExitWorktree", map[string]any{"action": "remove", "discard_changes": true})
	if removed.IsError {
		t.Fatalf("configured registry hook remove failed: %#v", removed)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(logData)), "\n"); len(lines) != 2 {
		t.Fatalf("configured hook calls = %d, want 2: %q", len(lines), logData)
	}
}

func shellQuoteTask13(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
