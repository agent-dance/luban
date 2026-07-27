package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestRuntimeArtifactsDoNotDirtyGitWorkspace(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	runArtifactIsolationGit(t, project, "init", "-b", "main")
	runArtifactIsolationGit(t, project, "config", "user.email", "runtime@example.invalid")
	runArtifactIsolationGit(t, project, "config", "user.name", "Runtime Test")
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runArtifactIsolationGit(t, project, "add", "tracked.txt")
	runArtifactIsolationGit(t, project, "commit", "-m", "fixture")
	before := runArtifactIsolationGit(t, project, "status", "--porcelain=v1", "--untracked-files=all")

	deps := SetupRegistry(provider.NewProviderRef(nil), project, []string{project}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() { _ = deps.BackgroundTasks.Shutdown(context.Background()) })
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID: "git-clean-session", ProjectRoot: project, CWD: project,
	})
	snapshot, err := deps.BackgroundTasks.StartAgentTask(ctx, "runtime isolation", "runtime isolation", func(context.Context, io.Writer) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, status := deps.BackgroundTasks.Wait(snapshot.ID, 5*time.Second)
	if status != "success" || completed.Status != "completed" {
		t.Fatalf("background status=%q task=%#v", status, completed)
	}
	if strings.HasPrefix(filepath.Clean(completed.OutputPath), filepath.Clean(project)+string(filepath.Separator)) {
		t.Fatalf("task output remained inside project: %q", completed.OutputPath)
	}
	ctxB := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID: "git-clean-session-b", ProjectRoot: project, CWD: project,
	})
	snapshotB, err := deps.BackgroundTasks.StartAgentTask(ctxB, "runtime isolation b", "runtime isolation b", func(context.Context, io.Writer) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	completedB, status := deps.BackgroundTasks.Wait(snapshotB.ID, 5*time.Second)
	if status != "success" || completedB.Status != "completed" {
		t.Fatalf("background B status=%q task=%#v", status, completedB)
	}
	if filepath.Clean(filepath.Dir(completed.OutputPath)) == filepath.Clean(filepath.Dir(completedB.OutputPath)) {
		t.Fatalf("different sessions share task-output directory %q", filepath.Dir(completed.OutputPath))
	}
	for path, want := range map[string]os.FileMode{
		filepath.Dir(completed.OutputPath):  0o700,
		completed.OutputPath:                0o600,
		filepath.Dir(completedB.OutputPath): 0o700,
		completedB.OutputPath:               0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode(%s)=%04o, want %04o", path, got, want)
		}
	}

	after := runArtifactIsolationGit(t, project, "status", "--porcelain=v1", "--untracked-files=all")
	if after != before || after != "" {
		t.Fatalf("runtime changed git status: before=%q after=%q", before, after)
	}
}

func runArtifactIsolationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
